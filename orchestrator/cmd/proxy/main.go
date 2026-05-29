package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lambdawp-567/sharedgpupower/orchestrator/internal/router"
)

type Config struct {
	ListenAddr       string
	OllamaAddr       string
	BackendAddr      string
	AgentID          string
	ContextThreshold int
}

func main() {
	cfg := &Config{}
	flag.StringVar(&cfg.ListenAddr, "listen", ":11435", "listen address (use instead of Ollama :11434)")
	flag.StringVar(&cfg.OllamaAddr, "ollama", "http://localhost:11434", "local Ollama address")
	flag.StringVar(&cfg.BackendAddr, "backend", "http://localhost:8080", "sharedGPUpower backend API")
	flag.StringVar(&cfg.AgentID, "agent-id", "", "local agent ID (from registration)")
	flag.IntVar(&cfg.ContextThreshold, "threshold", 2000, "token count above which tasks are offloaded")
	flag.Parse()

	if cfg.AgentID == "" {
		cfg.AgentID = os.Getenv("SGPU_AGENT_ID")
	}

	fmt.Printf("sharedGPUpower orchestrator proxy\n")
	fmt.Printf("  listening on  : %s\n", cfg.ListenAddr)
	fmt.Printf("  local ollama  : %s\n", cfg.OllamaAddr)
	fmt.Printf("  backend       : %s\n", cfg.BackendAddr)
	fmt.Printf("  offload at    : >%d estimated tokens\n\n", cfg.ContextThreshold)

	ollamaURL, err := url.Parse(cfg.OllamaAddr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid ollama addr:", err)
		os.Exit(1)
	}
	proxy := httputil.NewSingleHostReverseProxy(ollamaURL)

	mux := http.NewServeMux()

	// Intercept /api/generate — everything else passes through to Ollama
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		json.Unmarshal(body, &req)

		decision := router.Route(req.Prompt, req.Model, cfg.ContextThreshold)

		if decision == router.RunLocal || cfg.AgentID == "" {
			proxy.ServeHTTP(w, r)
			return
		}

		// Offload to network
		w.Header().Set("Content-Type", "application/json")
		result, err := submitToNetwork(cfg.BackendAddr, cfg.AgentID, req.Model, req.Prompt)
		if err != nil {
			// Fallback to local on network error
			r.Body = io.NopCloser(bytes.NewReader(body))
			proxy.ServeHTTP(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model":    req.Model,
			"response": result,
			"done":     true,
			"offloaded": true,
		})
	})

	// All other paths → proxy to Ollama
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "proxy server error:", err)
		}
	}()

	<-ctx.Done()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	srv.Shutdown(shutCtx)
	fmt.Println("proxy stopped")
}

func submitToNetwork(backendAddr, agentID, model, prompt string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"submitter_agent_id": agentID,
		"type":               "llm_inference",
		"model":              model,
		"prompt":             prompt,
		"async":              false,
	})

	resp, err := http.Post(backendAddr+"/api/v1/jobs", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var jobResp struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&jobResp)

	// Poll for result (non-time-critical)
	for i := 0; i < 60; i++ {
		time.Sleep(2 * time.Second)
		status, result, err := pollJob(backendAddr, jobResp.JobID)
		if err != nil {
			continue
		}
		if status == "done" {
			return result, nil
		}
		if status == "failed" {
			return "", fmt.Errorf("job failed")
		}
	}
	return "", fmt.Errorf("job timeout")
}

func pollJob(backendAddr, jobID string) (string, string, error) {
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/jobs/%s", backendAddr, jobID))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var j struct {
		Status  string                 `json:"Status"`
		Payload map[string]interface{} `json:"Payload"`
	}
	json.NewDecoder(resp.Body).Decode(&j)

	if j.Status == "done" {
		if result, ok := j.Payload["response"].(string); ok {
			return "done", result, nil
		}
	}
	return j.Status, "", nil
}
