package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	apiserver "github.com/lambdawp-567/sharedgpupower/backend/internal/api"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/auth"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/db"
	pb "github.com/lambdawp-567/sharedgpupower/backend/internal/grpc/pb"
	grpcserver "github.com/lambdawp-567/sharedgpupower/backend/internal/grpc"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/jobs"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/registry"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/scheduler"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/tokens"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/users"
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

	userStore := users.New(pool)
	reg := registry.New(pool, log)
	store := jobs.NewStore(pool, log)
	ledger := tokens.New(pool)
	sched := scheduler.New(reg, store, ledger, log)
	hub := apiserver.NewHub(log)

	// Auth middleware (Zitadel)
	zitadelDomain := os.Getenv("ZITADEL_DOMAIN")
	var authMW *auth.Middleware
	if zitadelDomain != "" {
		introspector := auth.NewZitadelIntrospector(zitadelDomain)
		authMW = auth.NewMiddleware(introspector, userStore, log)
		log.Info("Zitadel auth enabled", zap.String("domain", zitadelDomain))
	} else {
		log.Warn("ZITADEL_DOMAIN not set — auth middleware disabled (dev mode)")
	}

	// step-ca client (optional)
	var stepca *auth.StepCAClient
	if os.Getenv("STEPCA_PROVISIONER_KEY") != "" {
		var caErr error
		stepca, caErr = auth.NewStepCAClientFromEnv()
		if caErr != nil {
			log.Warn("step-ca client init failed", zap.Error(caErr))
		} else {
			log.Info("step-ca enabled", zap.String("url", os.Getenv("STEPCA_URL")))
		}
	}

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
		grpc.MaxRecvMsgSize(64*1024*1024),
		grpc.UnaryInterceptor(auth.AgentMTLSInterceptor),
		grpc.StreamInterceptor(auth.AgentMTLSStreamInterceptor),
	)
	agentSrv := grpcserver.NewAgentServer(reg, sched, userStore, stepca, log)
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
	handler := apiserver.NewHandler(reg, store, ledger, sched, userStore, authMW, log)
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Mount("/", handler.Router())
	r.Get("/ws", hub.ServeWS)

	// OIDC callback (Zitadel authorization code exchange)
	if zitadelDomain != "" {
		r.Get("/auth/callback", oidcCallbackHandler(zitadelDomain, userStore, log))
		r.Get("/auth/logout", logoutHandler())
	}

	corsOrigins := getenv("CORS_ORIGINS", "http://localhost:3000")
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   splitComma(corsOrigins),
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})

	httpPort := getenv("HTTP_PORT", "8080")
	httpSrv := &http.Server{
		Addr:    ":" + httpPort,
		Handler: corsHandler.Handler(r),
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

// oidcCallbackHandler handles the Zitadel authorization code callback.
// It exchanges the code for tokens, validates via UserInfo, creates/finds the
// local user, and sets an HttpOnly session cookie.
func oidcCallbackHandler(zitadelDomain string, userStore *users.Store, log *zap.Logger) http.HandlerFunc {
	clientID := os.Getenv("ZITADEL_CLIENT_ID")
	clientSecret := os.Getenv("ZITADEL_CLIENT_SECRET")
	redirectURI := os.Getenv("ZITADEL_REDIRECT_URI")
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	scheme := "https"
	if isLocalHostDomain(zitadelDomain) {
		scheme = "http"
	}
	tokenEndpoint := fmt.Sprintf("%s://%s/oauth/v2/token", scheme, zitadelDomain)
	userInfoEndpoint := fmt.Sprintf("%s://%s/oidc/v1/userinfo", scheme, zitadelDomain)

	httpClient := &http.Client{Timeout: 10 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		// Exchange code for tokens
		form := url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"redirect_uri": {redirectURI},
			"client_id":    {clientID},
		}
		if clientSecret != "" {
			form.Set("client_secret", clientSecret)
		}
		resp, err := httpClient.PostForm(tokenEndpoint, form)
		if err != nil {
			log.Error("token exchange failed", zap.Error(err))
			http.Error(w, "token exchange failed", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var tokenResp struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
		}
		if err := json.Unmarshal(body, &tokenResp); err != nil || tokenResp.AccessToken == "" {
			log.Error("bad token response", zap.String("body", string(body)))
			http.Error(w, "token exchange failed", http.StatusInternalServerError)
			return
		}

		// Fetch user info from Zitadel
		req2, _ := http.NewRequestWithContext(r.Context(), "GET", userInfoEndpoint, nil)
		req2.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		resp2, err := httpClient.Do(req2)
		if err != nil {
			log.Error("userinfo failed", zap.Error(err))
			http.Error(w, "userinfo failed", http.StatusInternalServerError)
			return
		}
		defer resp2.Body.Close()

		var info struct {
			Sub     string `json:"sub"`
			Email   string `json:"email"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		}
		if err := json.NewDecoder(resp2.Body).Decode(&info); err != nil || info.Sub == "" {
			http.Error(w, "invalid userinfo", http.StatusInternalServerError)
			return
		}

		// Find or create local user
		_, _, err = userStore.FindOrCreate(r.Context(), info.Sub, info.Email, info.Name, info.Picture)
		if err != nil {
			log.Error("find or create user failed", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Set session cookie with the Zitadel access token
		http.SetCookie(w, &http.Cookie{
			Name:     "sgpu_session",
			Value:    tokenResp.AccessToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   3600,
		})

		http.Redirect(w, r, frontendURL+"/dashboard", http.StatusFound)
	}
}

func logoutHandler() http.HandlerFunc {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "sgpu_session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		http.Redirect(w, r, frontendURL, http.StatusFound)
	}
}

func isLocalHostDomain(domain string) bool {
	for _, prefix := range []string{"localhost", "127.", "::1"} {
		if strings.HasPrefix(domain, prefix) {
			return true
		}
	}
	return false
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
