package auth

import (
	"context"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const registerFullMethod = "/sharedgpu.agent.v1.AgentService/Register"

// AgentMTLSInterceptor validates client mTLS certs for all non-Register unary RPCs.
// Set MTLS_REQUIRED=true in production; omit for local dev (insecure transport).
func AgentMTLSInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if info.FullMethod == registerFullMethod || !mtlsRequired() {
		return handler(ctx, req)
	}
	if err := requireMTLSCert(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// AgentMTLSStreamInterceptor validates client mTLS certs for all streaming RPCs.
func AgentMTLSStreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if !mtlsRequired() {
		return handler(srv, ss)
	}
	if err := requireMTLSCert(ss.Context()); err != nil {
		return err
	}
	return handler(srv, ss)
}

func requireMTLSCert(ctx context.Context) error {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "no peer info")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
		return status.Error(codes.Unauthenticated, "mTLS client certificate required")
	}
	return nil
}

func mtlsRequired() bool {
	return os.Getenv("MTLS_REQUIRED") == "true"
}
