package grpcserver

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/lambdawp-567/sharedgpupower/backend/internal/grpc/pb"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/registry"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/scheduler"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	reg   *registry.Registry
	sched *scheduler.Scheduler
	log   *zap.Logger
}

func NewAgentServer(reg *registry.Registry, sched *scheduler.Scheduler, log *zap.Logger) *AgentServer {
	return &AgentServer{reg: reg, sched: sched, log: log}
}

func (s *AgentServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name required")
	}

	benchScore := 1.0
	if req.Benchmark != nil && req.Benchmark.CompositeScore > 0 {
		benchScore = float64(req.Benchmark.CompositeScore)
	}

	a := &registry.Agent{
		Name:           req.Name,
		PublicKey:      req.PublicKey,
		BenchScore:     benchScore,
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

	s.log.Info("agent registered", zap.String("id", id), zap.String("name", req.Name))
	return &pb.RegisterResponse{AgentId: id, Token: id}, nil
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
	defer s.reg.SetOffline(req.AgentId)

	s.log.Info("agent streaming jobs", zap.String("agent_id", req.AgentId))

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case payload, ok := <-agent.JobCh:
			if !ok {
				return nil
			}
			jobPayload := &pb.JobPayload{
				Payload: payload,
			}
			// Extract job_id from payload if present
			var m map[string]any
			if json.Unmarshal(payload, &m) == nil {
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
		// Store result in jobs table
		_ = req.Error
	}

	s.log.Info("job result received",
		zap.String("job_id", req.JobId),
		zap.Bool("success", req.Success),
		zap.Float32("duration_s", req.DurationSeconds))

	return &pb.Ack{Ok: true}, nil
}

type SignalingServer struct {
	pb.UnimplementedSignalingServiceServer
	sessions map[string]chan *pb.ICESignal
	log      *zap.Logger
}

func NewSignalingServer(log *zap.Logger) *SignalingServer {
	return &SignalingServer{
		sessions: make(map[string]chan *pb.ICESignal),
		log:      log,
	}
}

func (s *SignalingServer) Exchange(stream pb.SignalingService_ExchangeServer) error {
	ctx := stream.Context()

	// First message identifies the sender
	first, err := stream.Recv()
	if err != nil {
		return err
	}

	sessionKey := first.FromAgentId + ":" + first.JobId
	ch := make(chan *pb.ICESignal, 16)
	s.sessions[sessionKey] = ch
	defer delete(s.sessions, sessionKey)

	// Forward received signals to target
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			targetKey := msg.ToAgentId + ":" + msg.JobId
			if targetCh, ok := s.sessions[targetKey]; ok {
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

	// Forward initial signal
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
