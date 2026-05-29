package registry

import (
	"context"
	"fmt"
	"time"

	pb "github.com/lambdawp-567/sharedgpupower/agent/internal/pb"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/benchmark"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type Client struct {
	conn    *grpc.ClientConn
	svc     pb.AgentServiceClient
	agentID string
	cfg     *config.Config
	bench   *benchmark.Result
	log     *zap.Logger
}

func New(cfg *config.Config, bench *benchmark.Result, log *zap.Logger) (*Client, error) {
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	if cfg.Backend.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		// TODO Phase 4: load mTLS credentials from cert files
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Backend.Endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc connect: %w", err)
	}

	return &Client{
		conn:  conn,
		svc:   pb.NewAgentServiceClient(conn),
		cfg:   cfg,
		bench: bench,
		log:   log,
	}, nil
}

func (c *Client) Register(ctx context.Context) (string, error) {
	hw := &pb.HardwareInfo{
		Arch:           c.bench.Arch,
		Os:             c.bench.OS,
		CpuCores:       int32(c.hardwareCores()),
		RamGb:          0, // populated by resources package
		GpuLayersTotal: c.cfg.Resources.GPULayers * 2, // estimate total as 2× limit
	}

	req := &pb.RegisterRequest{
		Name:      c.cfg.Agent.Name,
		PublicKey: "placeholder", // Phase 4: real mTLS key
		Hardware:  hw,
		Limits: &pb.ResourceLimits{
			CpuPercent: c.cfg.Resources.CPUPercent,
			RamPercent: c.cfg.Resources.RAMPercent,
			GpuLayers:  c.cfg.Resources.GPULayers,
		},
		Benchmark: &pb.BenchmarkResult{
			CpuScore:       c.bench.CPUScore,
			MemBandwidthGbps: c.bench.MemBandwidthGB,
			GpuTflops:      c.bench.GPUTFlops,
			CompositeScore: c.bench.CompositeScore,
		},
	}

	resp, err := c.svc.Register(ctx, req)
	if err != nil {
		return "", err
	}
	c.agentID = resp.AgentId
	c.log.Info("registered with backend", zap.String("agent_id", c.agentID))
	return c.agentID, nil
}

func (c *Client) RunHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := c.svc.Heartbeat(ctx, &pb.HeartbeatRequest{AgentId: c.agentID})
			if err != nil {
				c.log.Warn("heartbeat failed", zap.Error(err))
			}
		}
	}
}

func (c *Client) StreamJobs(ctx context.Context) (pb.AgentService_StreamJobsClient, error) {
	return c.svc.StreamJobs(ctx, &pb.StreamJobsRequest{AgentId: c.agentID})
}

func (c *Client) ReportResult(ctx context.Context, result *pb.JobResult) error {
	_, err := c.svc.ReportResult(ctx, result)
	return err
}

func (c *Client) AgentID() string { return c.agentID }

func (c *Client) Close() { c.conn.Close() }

func (c *Client) hardwareCores() int {
	return 1 // runtime.NumCPU() — imported in main to avoid circular deps
}
