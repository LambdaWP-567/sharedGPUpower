package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/jobs"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/registry"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/scheduler"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/tokens"
	"go.uber.org/zap"
)

type Handler struct {
	reg    *registry.Registry
	store  *jobs.Store
	ledger *tokens.Ledger
	sched  *scheduler.Scheduler
	log    *zap.Logger
}

func NewHandler(
	reg *registry.Registry,
	store *jobs.Store,
	ledger *tokens.Ledger,
	sched *scheduler.Scheduler,
	log *zap.Logger,
) *Handler {
	return &Handler{reg: reg, store: store, ledger: ledger, sched: sched, log: log}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", h.health)
	r.Get("/api/v1/agents", h.listAgents)
	r.Get("/api/v1/agents/{id}/balance", h.agentBalance)
	r.Get("/api/v1/network/capacity", h.networkCapacity)
	r.Post("/api/v1/jobs", h.submitJob)
	r.Get("/api/v1/jobs/{id}", h.getJob)
	r.Get("/api/v1/jobs", h.listJobs)

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type agentInfo struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Arch       string  `json:"arch"`
	OS         string  `json:"os"`
	GPUModel   string  `json:"gpu_model"`
	CPUCores   int32   `json:"cpu_cores"`
	RAMGB      float32 `json:"ram_gb"`
	BenchScore float64 `json:"bench_score"`
	LastSeen   string  `json:"last_seen"`
}

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.reg.All()
	out := make([]agentInfo, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentInfo{
			ID:         a.ID,
			Name:       a.Name,
			Status:     a.Status,
			Arch:       a.Arch,
			OS:         a.OS,
			GPUModel:   a.GPUModel,
			CPUCores:   a.CPUCores,
			RAMGB:      a.RAMGB,
			BenchScore: a.BenchScore,
			LastSeen:   a.LastSeen.Format("2006-01-02T15:04:05Z"),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) agentBalance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bal, err := h.ledger.Balance(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]float64{"balance": bal})
}

type capacityResponse struct {
	TotalAgents    int     `json:"total_agents"`
	OnlineAgents   int     `json:"online_agents"`
	TotalCPUCores  int32   `json:"total_cpu_cores"`
	TotalRAMGB     float32 `json:"total_ram_gb"`
	TotalGPULayers int32   `json:"total_gpu_layers"`
	AvgBenchScore  float64 `json:"avg_bench_score"`
}

func (h *Handler) networkCapacity(w http.ResponseWriter, r *http.Request) {
	all := h.reg.All()
	online := h.reg.GetAvailable()

	resp := capacityResponse{
		TotalAgents:  len(all),
		OnlineAgents: len(online),
	}
	for _, a := range online {
		resp.TotalCPUCores += a.CPUCores
		resp.TotalRAMGB += a.RAMGB
		resp.TotalGPULayers += a.GPULayersLimit
		resp.AvgBenchScore += a.BenchScore
	}
	if len(online) > 0 {
		resp.AvgBenchScore /= float64(len(online))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type submitJobRequest struct {
	SubmitterAgentID  string         `json:"submitter_agent_id"`
	Type              string         `json:"type"`
	Model             string         `json:"model"`
	Prompt            string         `json:"prompt"`
	MaxTokens         int            `json:"max_tokens"`
	RequiredGPULayers int32          `json:"required_gpu_layers"`
	Extra             map[string]any `json:"extra"`
}

func (h *Handler) submitJob(w http.ResponseWriter, r *http.Request) {
	var req submitJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SubmitterAgentID == "" || req.Prompt == "" {
		http.Error(w, "submitter_agent_id and prompt required", http.StatusBadRequest)
		return
	}
	jobType := req.Type
	if jobType == "" {
		jobType = "llm_inference"
	}

	payload := map[string]any{
		"model":      req.Model,
		"prompt":     req.Prompt,
		"max_tokens": req.MaxTokens,
	}
	for k, v := range req.Extra {
		payload[k] = v
	}

	j, err := h.store.Create(r.Context(), req.SubmitterAgentID, jobType, payload)
	if err != nil {
		h.log.Error("create job failed", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	payload["job_id"] = j.ID
	payload["type"] = jobType

	h.sched.Enqueue(&scheduler.JobRequest{
		JobID:             j.ID,
		SubmitterAgentID:  req.SubmitterAgentID,
		Type:              jobType,
		Payload:           payload,
		RequiredGPULayers: req.RequiredGPULayers,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": j.ID, "status": "pending"})
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	j, err := h.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}

func (h *Handler) listJobs(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id query param required", http.StatusBadRequest)
		return
	}
	js, err := h.store.ListBySubmitter(r.Context(), agentID, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(js)
}
