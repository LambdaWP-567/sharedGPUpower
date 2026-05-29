package grpcserver

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	pb "github.com/lambdawp-567/sharedgpupower/backend/internal/grpc/pb"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/auth"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/registry"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/scheduler"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/users"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	reg          *registry.Registry
	sched        *scheduler.Scheduler
	userStore    *users.Store
	stepca       *auth.StepCAClient
	log          *zap.Logger
	streamsMu    sync.Mutex
	activeStreams map[string]struct{}
}

func NewAgentServer(
	reg *registry.Registry,
	sched *scheduler.Scheduler,
	userStore *users.Store,
	stepca *auth.StepCAClient,
	log *zap.Logger,
) *AgentServer {
	return &AgentServer{
		reg:          reg,
		sched:        sched,
		userStore:    userStore,
		stepca:       stepca,
		log:          log,
		activeStreams: make(map[string]struct{}),
	}
}

func (s *AgentServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name required")
	}

	// Validate API key and get user
	var userID string
	if req.UserApiKey != "" && s.userStore != nil {
		u, err := s.userStore.GetByAPIKey(ctx, req.UserApiKey)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid api key")
		}
		if u.Status == "disabled" {
			return nil, status.Error(codes.PermissionDenied, "account disabled")
		}
		userID = u.ID
	}

	benchScore := 1.0
	if req.Benchmark != nil && req.Benchmark.CompositeScore > 0 {
		benchScore = float64(req.Benchmark.CompositeScore)
	}

	a := &registry.Agent{
		Name:           req.Name,
		PublicKey:      req.PublicKey,
		BenchScore:     benchScore,
		UserID:         userID,
		ApprovalStatus: "pending",
	}
	if req.Hardware != nil {
		a.Arch = req.Hardware.Arch
		a.OS = req.Hardware.Os
		a.GPUModel = req.Hardware.GpuModel
		a.CPUCores = req.Hardware.CpuCores
		a.RAMGB = req.Hardware.RamGb
		a.GPULayersTotal = req.Hardware.GpuLayersTotal
	}
	if req.Limits != nil {
		a.CPULimit = req.Limits.CpuPercent
		a.RAMLimit = req.Limits.RamPercent
		a.GPULayersLimit = req.Limits.GpuLayers
	}

	id, err := s.reg.Register(ctx, a)
	if err != nil {
		s.log.Error("register failed", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "register: %v", err)
	}

	resp := &pb.RegisterResponse{AgentId: id, Token: id}

	// Issue mTLS cert if CSR provided and step-ca is configured
	if len(req.CsrPem) > 0 && s.stepca != nil {
		certPEM, caCertPEM, err := s.stepca.RequestCert(ctx, req.CsrPem, id)
		if err != nil {
			s.log.Warn("step-ca cert request failed (agent proceeds without mTLS)", zap.Error(err))
		} else {
			resp.CertPem = certPEM
			resp.CaCertPem = caCertPEM
		}
	}

	s.log.Info("agent registered",
		zap.String("id", id),
		zap.String("name", req.Name),
		zap.String("user_id", userID),
	)
	return resp, nil
}

func (s *AgentServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	if err := s.reg.Heartbeat(ctx, req.AgentId); err != nil {
		return nil, status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
	}
	return &pb.HeartbeatResponse{Ok: true}, nil
}

func (s *AgentServer) StreamJobs(req *pb.StreamJobsRequest, stream pb.AgentService_StreamJobsServer) error {
	agent, ok := s.reg.Get(req.AgentId)
	if !ok {
		return status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
	}

	// Gate on approval status
	if agent.ApprovalStatus != "active" {
		return status.Errorf(codes.PermissionDenied, "agent pending approval")
	}

	// Prevent duplicate streams for the same agent.
	s.streamsMu.Lock()
	if _, active := s.activeStreams[req.AgentId]; active {
		s.streamsMu.Unlock()
		return status.Errorf(codes.AlreadyExists, "stream already active for agent %s", req.AgentId)
	}
	s.activeStreams[req.AgentId] = struct{}{}
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()
		delete(s.activeStreams, req.AgentId)
		s.streamsMu.Unlock()
		s.reg.SetOffline(req.AgentId)
	}()

	s.log.Info("agent streaming jobs", zap.String("agent_id", req.AgentId))

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case payload, ok := <-agent.JobCh:
			if !ok {
				return nil
			}
			jobPayload := &pb.JobPayload{Payload: payload}
			var m map[string]any
			if err := json.Unmarshal(payload, &m); err == nil {
				if jid, ok := m["job_id"].(string); ok {
					jobPayload.JobId = jid
				}
				if jtype, ok := m["type"].(string); ok {
					jobPayload.Type = jtype
				}
			}
			if err := stream.Send(jobPayload); err != nil {
				s.log.Error("stream send failed", zap.Error(err))
				return err
			}
		}
	}
}

func (s *AgentServer) ReportResult(ctx context.Context, req *pb.JobResult) (*pb.Ack, error) {
	if req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id required")
	}

	if req.Success {
		if err := s.sched.CompleteJob(ctx, req.JobId, req.AgentId, float64(req.DurationSeconds)); err != nil {
			s.log.Error("complete job failed", zap.Error(err))
		}
	} else {
		if err := s.sched.FailJob(ctx, req.JobId, req.Error); err != nil {
			s.log.Error("fail job update failed", zap.Error(err))
		}
	}

	s.log.Info("job result received",
		zap.String("job_id", req.JobId),
		zap.Bool("success", req.Success),
		zap.Float32("duration_s", req.DurationSeconds))

	return &pb.Ack{Ok: true}, nil
}

type SignalingServer struct {
	pb.UnimplementedSignalingServiceServer
	mu       sync.RWMutex
	sessions map[string]chan *pb.ICESignal
	log      *zap.Logger
}

func NewSignalingServer(log *zap.Logger) *SignalingServer {
	return &SignalingServer{
		sessions: make(map[string]chan *pb.ICESignal),
		log:      log,
	}
}

func (s *SignalingServer) getSession(key string) (chan *pb.ICESignal, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch, ok := s.sessions[key]
	return ch, ok
}

func (s *SignalingServer) Exchange(stream pb.SignalingService_ExchangeServer) error {
	ctx := stream.Context()

	first, err := stream.Recv()
	if err != nil {
		return err
	}

	sessionKey := first.FromAgentId + ":" + first.JobId
	ch := make(chan *pb.ICESignal, 16)

	s.mu.Lock()
	s.sessions[sessionKey] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.sessions, sessionKey)
		s.mu.Unlock()
	}()

	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			targetKey := msg.ToAgentId + ":" + msg.JobId
			if targetCh, ok := s.getSession(targetKey); ok {
				select {
				case targetCh <- msg:
				case <-ctx.Done():
					return
				default:
					s.log.Warn("signaling target channel full")
				}
			}
		}
	}()

	if err := stream.Send(first); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case sig, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(sig); err != nil {
				return err
			}
		case <-time.After(5 * time.Minute):
			return nil
		}
	}
}
