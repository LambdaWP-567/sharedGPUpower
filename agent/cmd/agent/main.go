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

	// Connect to backend
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

	// Start heartbeat
	go client.RunHeartbeat(ctx)

	// Ollama client
	ollamaClient := ollama.New(cfg.Ollama.Host, cfg.Resources.GPULayers)

	// Ping Ollama
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := ollamaClient.Ping(pingCtx); err != nil {
		log.Warn("ollama not reachable (will retry per job)", zap.Error(err))
	} else {
		log.Info("ollama connected", zap.String("host", cfg.Ollama.Host))
	}
	pingCancel()

	exec := executor.New(ollamaClient, log)

	// Stream and execute jobs
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := runJobStream(ctx, client, exec, log); err != nil {
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

func runJobStream(ctx context.Context, client *registry.Client, exec *executor.Executor, log *zap.Logger) error {
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

		go func(job *pb.JobPayload) {
			log.Info("executing job", zap.String("job_id", job.JobId), zap.String("type", job.Type))
			start := time.Now()

			// Merge job_id into payload if not present
			var payloadMap map[string]any
			if json.Unmarshal(job.Payload, &payloadMap) == nil {
				if _, ok := payloadMap["job_id"]; !ok {
					payloadMap["job_id"] = job.JobId
				}
				job.Payload, _ = json.Marshal(payloadMap)
			}

			result := exec.Execute(ctx, job.JobId, job.Type, job.Payload)
			elapsed := time.Since(start).Seconds()

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
