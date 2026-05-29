package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	apiserver "github.com/lambdawp-567/sharedgpupower/backend/internal/api"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/db"
	pb "github.com/lambdawp-567/sharedgpupower/backend/internal/grpc/pb"
	grpcserver "github.com/lambdawp-567/sharedgpupower/backend/internal/grpc"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/jobs"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/registry"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/scheduler"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/tokens"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatal("db connect failed", zap.Error(err))
	}
	defer pool.Close()

	// Apply schema
	schemaSQL, err := os.ReadFile("internal/db/schema.sql")
	if err == nil {
		if _, err = pool.Exec(ctx, string(schemaSQL)); err != nil {
			log.Warn("schema apply failed (may already exist)", zap.Error(err))
		}
	}

	reg := registry.New(pool, log)
	store := jobs.NewStore(pool, log)
	ledger := tokens.New(pool)
	sched := scheduler.New(reg, store, ledger, log)
	hub := apiserver.NewHub(log)

	// Stale-agent sweeper
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				reg.SweepStale(ctx)
			}
		}
	}()

	go sched.Run(ctx)
	hub.StartPing(30 * time.Second)

	// gRPC server
	grpcPort := getenv("GRPC_PORT", "9090")
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatal("grpc listen failed", zap.Error(err))
	}

	grpcSrv := grpc.NewServer(
		grpc.MaxRecvMsgSize(64 * 1024 * 1024),
	)
	agentSrv := grpcserver.NewAgentServer(reg, sched, log)
	sigSrv := grpcserver.NewSignalingServer(log)
	pb.RegisterAgentServiceServer(grpcSrv, agentSrv)
	pb.RegisterSignalingServiceServer(grpcSrv, sigSrv)

	go func() {
		log.Info("gRPC server listening", zap.String("port", grpcPort))
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error("grpc serve error", zap.Error(err))
		}
	}()

	// HTTP server
	handler := apiserver.NewHandler(reg, store, ledger, sched, log)
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Mount("/", handler.Router())
	r.Get("/ws", hub.ServeWS)

	httpPort := getenv("HTTP_PORT", "8080")
	httpSrv := &http.Server{
		Addr:    ":" + httpPort,
		Handler: cors.AllowAll().Handler(r),
	}

	go func() {
		log.Info("HTTP server listening", zap.String("port", httpPort))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http serve error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down...")

	grpcSrv.GracefulStop()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	httpSrv.Shutdown(shutCtx)

	fmt.Println("bye")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
