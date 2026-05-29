package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	pb "github.com/lambdawp-567/sharedgpupower/agent/internal/pb"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/benchmark"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/config"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/executor"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/ollama"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/registry"
	"go.uber.org/zap"
)

const (
	maxConcurrentJobs = 4
	jobTimeout        = 10 * time.Minute
)

func main() {
	cfgPath := flag.String("config", "agent.yaml", "path to config file")
	flag.Parse()

	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal("load config failed", zap.Error(err))
	}

	if cfg.Agent.Name == "" {
		hostname, _ := os.Hostname()
		cfg.Agent.Name = hostname
	}

	log.Info("sharedGPUpower agent starting",
		zap.String("name", cfg.Agent.Name),
		zap.String("backend", cfg.Backend.Endpoint),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Run benchmark
	log.Info("running startup benchmark...")
	bench, err := benchmark.Run(ctx, cfg.Ollama.Host, cfg.Ollama.DefaultModel)
	if err != nil {
		log.Warn("benchmark error (using defaults)", zap.Error(err))
		bench = &benchmark.Result{
			CPUScore:       0.5,
			MemBandwidthGB: 0.5,
			CompositeScore: 0.5,
			Arch:           runtime.GOARCH,
			OS:             runtime.GOOS,
		}
	}
	log.Info("benchmark complete",
		zap.Float32("cpu_score", bench.CPUScore),
		zap.Float32("mem_bw_gbps", bench.MemBandwidthGB),
		zap.Float32("gpu_tflops", bench.GPUTFlops),
		zap.Float32("composite", bench.CompositeScore),
	)

	client, err := registry.New(cfg, bench, log)
	if err != nil {
		log.Fatal("grpc connect failed", zap.Error(err))
	}
	defer client.Close()

	agentID, err := client.Register(ctx)
	if err != nil {
		log.Fatal("register failed", zap.Error(err))
	}
	log.Info("agent registered", zap.String("id", agentID))

	go client.RunHeartbeat(ctx)

	ollamaClient := ollama.New(cfg.Ollama.Host, cfg.Resources.GPULayers)

	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := ollamaClient.Ping(pingCtx); err != nil {
		log.Warn("ollama not reachable (will retry per job)", zap.Error(err))
	} else {
		log.Info("ollama connected", zap.String("host", cfg.Ollama.Host))
	}
	pingCancel()

	exec := executor.New(ollamaClient, log)

	// Semaphore limits concurrent job goroutines.
	sem := make(chan struct{}, maxConcurrentJobs)

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := runJobStream(ctx, client, exec, sem, log); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Warn("job stream ended, reconnecting in 5s", zap.Error(err))
				time.Sleep(5 * time.Second)
			}
		}
	}()

	<-ctx.Done()
	fmt.Println("agent shutting down")
}

func runJobStream(ctx context.Context, client *registry.Client, exec *executor.Executor, sem chan struct{}, log *zap.Logger) error {
	stream, err := client.StreamJobs(ctx)
	if err != nil {
		return err
	}
	log.Info("job stream connected")

	for {
		job, err := stream.Recv()
		if err != nil {
			return err
		}

		// Acquire semaphore slot (blocks if maxConcurrentJobs reached).
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		go func(job *pb.JobPayload) {
			defer func() { <-sem }()

			log.Info("executing job", zap.String("job_id", job.JobId), zap.String("type", job.Type))

			// Per-job timeout so a hanging Ollama call doesn't leak forever.
			jobCtx, jobCancel := context.WithTimeout(ctx, jobTimeout)
			defer jobCancel()

			start := time.Now()

			var payloadMap map[string]any
			if err := json.Unmarshal(job.Payload, &payloadMap); err == nil {
				if _, ok := payloadMap["job_id"]; !ok {
					payloadMap["job_id"] = job.JobId
				}
				if updated, err := json.Marshal(payloadMap); err == nil {
					job.Payload = updated
				}
			}

			result := exec.Execute(jobCtx, job.JobId, job.Type, job.Payload)
			elapsed := time.Since(start).Seconds()

			if jobCtx.Err() == context.DeadlineExceeded {
				result.Success = false
				result.Error = "job timeout exceeded"
			}

			reportCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			client.ReportResult(reportCtx, &pb.JobResult{
				JobId:           job.JobId,
				AgentId:         client.AgentID(),
				Success:         result.Success,
				Result:          result.Output,
				Error:           result.Error,
				DurationSeconds: float32(elapsed),
			})

			log.Info("job done",
				zap.String("job_id", job.JobId),
				zap.Bool("success", result.Success),
				zap.Float64("duration_s", elapsed),
			)
		}(job)
	}
}
