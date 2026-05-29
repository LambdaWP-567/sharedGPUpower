package registry

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	pb "github.com/lambdawp-567/sharedgpupower/agent/internal/pb"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/benchmark"
	"github.com/lambdawp-567/sharedgpupower/agent/internal/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	hasCert := fileExists(cfg.Auth.CertPath) && fileExists(cfg.Auth.KeyPath)

	var creds grpc.DialOption
	if hasCert && fileExists(cfg.Auth.CACertPath) {
		tlsCreds, err := loadMTLSCredentials(cfg.Auth.CertPath, cfg.Auth.KeyPath, cfg.Auth.CACertPath)
		if err != nil {
			log.Warn("failed to load mTLS creds, falling back to insecure", zap.Error(err))
			creds = grpc.WithTransportCredentials(insecure.NewCredentials())
		} else {
			log.Info("mTLS enabled")
			creds = grpc.WithTransportCredentials(tlsCreds)
		}
	} else {
		creds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.NewClient(cfg.Backend.Endpoint,
		creds,
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	)
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
		RamGb:          0,
		GpuLayersTotal: c.cfg.Resources.GPULayers * 2,
	}

	req := &pb.RegisterRequest{
		Name:       c.cfg.Agent.Name,
		PublicKey:  "ecdsa-p256",
		Hardware:   hw,
		UserApiKey: c.cfg.Auth.UserAPIKey,
		Limits: &pb.ResourceLimits{
			CpuPercent: c.cfg.Resources.CPUPercent,
			RamPercent: c.cfg.Resources.RAMPercent,
			GpuLayers:  c.cfg.Resources.GPULayers,
		},
		Benchmark: &pb.BenchmarkResult{
			CpuScore:         c.bench.CPUScore,
			MemBandwidthGbps: c.bench.MemBandwidthGB,
			GpuTflops:        c.bench.GPUTFlops,
			CompositeScore:   c.bench.CompositeScore,
		},
	}

	// Generate CSR if we don't have a cert yet
	if !fileExists(c.cfg.Auth.CertPath) {
		csrPEM, err := c.generateAndSaveKey()
		if err != nil {
			c.log.Warn("CSR generation failed (continuing without mTLS)", zap.Error(err))
		} else {
			req.CsrPem = csrPEM
		}
	}

	resp, err := c.svc.Register(ctx, req)
	if err != nil {
		return "", err
	}
	c.agentID = resp.AgentId

	// Save cert and CA cert if returned
	if len(resp.CertPem) > 0 {
		if err := savePEMFile(c.cfg.Auth.CertPath, resp.CertPem); err != nil {
			c.log.Warn("save cert failed", zap.Error(err))
		}
	}
	if len(resp.CaCertPem) > 0 {
		if err := savePEMFile(c.cfg.Auth.CACertPath, resp.CaCertPem); err != nil {
			c.log.Warn("save CA cert failed", zap.Error(err))
		}
	}

	c.log.Info("registered with backend", zap.String("agent_id", c.agentID))
	return c.agentID, nil
}

// generateAndSaveKey creates an ECDSA P-256 key pair, saves the private key,
// and returns a PEM-encoded CSR.
func (c *Client) generateAndSaveKey() (csrPEM []byte, err error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Save private key
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := savePEMFile(c.cfg.Auth.KeyPath, keyPEM); err != nil {
		return nil, fmt.Errorf("save private key: %w", err)
	}

	// Create CSR — CN will be set to agent ID after registration, so use hostname for now
	hostname, _ := os.Hostname()
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   hostname,
			Organization: []string{"sharedGPUpower"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		return nil, fmt.Errorf("create CSR: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), nil
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

func (c *Client) hardwareCores() int { return 1 }

func loadMTLSCredentials(certPath, keyPath, caPath string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}

	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("parse CA cert")
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func savePEMFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
