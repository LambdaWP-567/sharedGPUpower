package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/auth"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/jobs"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/registry"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/scheduler"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/tokens"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/users"
	"go.uber.org/zap"
)

type Handler struct {
	reg       *registry.Registry
	store     *jobs.Store
	ledger    *tokens.Ledger
	sched     *scheduler.Scheduler
	userStore *users.Store
	authMW    *auth.Middleware
	log       *zap.Logger
}

func NewHandler(
	reg *registry.Registry,
	store *jobs.Store,
	ledger *tokens.Ledger,
	sched *scheduler.Scheduler,
	userStore *users.Store,
	authMW *auth.Middleware,
	log *zap.Logger,
) *Handler {
	return &Handler{
		reg:       reg,
		store:     store,
		ledger:    ledger,
		sched:     sched,
		userStore: userStore,
		authMW:    authMW,
		log:       log,
	}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()

	r.Get("/health", h.health)

	// Public agent/network endpoints (read-only telemetry, no auth required)
	r.Get("/api/v1/agents", h.listAgents)
	r.Get("/api/v1/agents/{id}/balance", h.agentBalance)
	r.Get("/api/v1/network/capacity", h.networkCapacity)

	// Authenticated endpoints
	if h.authMW != nil {
		r.Group(func(r chi.Router) {
			r.Use(h.authMW.Authenticate)
			r.Use(auth.RequireActive)

			r.Post("/api/v1/jobs", h.submitJob)
			r.Get("/api/v1/jobs/{id}", h.getJob)
			r.Get("/api/v1/jobs", h.listJobs)

			// User profile
			r.Get("/api/v1/users/me", h.me)
			r.Post("/api/v1/users/me/regenerate-key", h.regenerateAPIKey)
			r.Get("/api/v1/users/me/agents", h.myAgents)
			r.Get("/api/v1/users/me/balance", h.myBalance)

			// Admin panel
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAdmin)
				r.Get("/api/v1/admin/users", h.adminListUsers)
				r.Get("/api/v1/admin/users/pending", h.adminListPendingUsers)
				r.Post("/api/v1/admin/users/{id}/approve", h.adminApproveUser)
				r.Post("/api/v1/admin/users/{id}/disable", h.adminDisableUser)
				r.Get("/api/v1/admin/agents/pending", h.adminListPendingAgents)
				r.Post("/api/v1/admin/agents/{id}/approve", h.adminApproveAgent)
				r.Post("/api/v1/admin/agents/{id}/disable", h.adminDisableAgent)
			})
		})
	} else {
		// No auth middleware — all routes open (dev mode)
		r.Post("/api/v1/jobs", h.submitJob)
		r.Get("/api/v1/jobs/{id}", h.getJob)
		r.Get("/api/v1/jobs", h.listJobs)
	}

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ---- Agent / Network ----

type agentInfo struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	ApprovalStatus string  `json:"approval_status"`
	Arch           string  `json:"arch"`
	OS             string  `json:"os"`
	GPUModel       string  `json:"gpu_model"`
	CPUCores       int32   `json:"cpu_cores"`
	RAMGB          float32 `json:"ram_gb"`
	BenchScore     float64 `json:"bench_score"`
	UserID         string  `json:"user_id,omitempty"`
	LastSeen       string  `json:"last_seen"`
}

func toAgentInfo(a *registry.Agent) agentInfo {
	return agentInfo{
		ID:             a.ID,
		Name:           a.Name,
		Status:         a.Status,
		ApprovalStatus: a.ApprovalStatus,
		Arch:           a.Arch,
		OS:             a.OS,
		GPUModel:       a.GPUModel,
		CPUCores:       a.CPUCores,
		RAMGB:          a.RAMGB,
		BenchScore:     a.BenchScore,
		UserID:         a.UserID,
		LastSeen:       a.LastSeen.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.reg.All()
	out := make([]agentInfo, 0, len(agents))
	for _, a := range agents {
		out = append(out, toAgentInfo(a))
	}
	jsonOK(w, out)
}

func (h *Handler) agentBalance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	bal, err := h.ledger.Balance(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]float64{"balance": bal})
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
	jsonOK(w, resp)
}

// ---- Jobs ----

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
	jsonOK(w, j)
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
	jsonOK(w, js)
}

// ---- User profile ----

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	jsonOK(w, map[string]any{
		"id":           u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"avatar_url":   u.AvatarURL,
		"role":         u.Role,
		"status":       u.Status,
		"api_key":      u.APIKey,
		"created_at":   u.CreatedAt,
	})
}

func (h *Handler) regenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	newKey, err := h.userStore.RegenerateAPIKey(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"api_key": newKey})
}

func (h *Handler) myAgents(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	agents := h.reg.ListByUser(u.ID)
	out := make([]agentInfo, 0, len(agents))
	for _, a := range agents {
		out = append(out, toAgentInfo(a))
	}
	jsonOK(w, out)
}

func (h *Handler) myBalance(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	bal, err := h.ledger.UserBalance(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]float64{"balance": bal})
}

// ---- Admin ----

func (h *Handler) adminListUsers(w http.ResponseWriter, r *http.Request) {
	list, err := h.userStore.ListAll(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, list)
}

func (h *Handler) adminListPendingUsers(w http.ResponseWriter, r *http.Request) {
	list, err := h.userStore.ListPending(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, list)
}

func (h *Handler) adminApproveUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.userStore.SetStatus(r.Context(), id, "active"); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "active"})
}

func (h *Handler) adminDisableUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.userStore.SetStatus(r.Context(), id, "disabled"); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "disabled"})
}

func (h *Handler) adminListPendingAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.reg.AllPendingApproval()
	out := make([]agentInfo, 0, len(agents))
	for _, a := range agents {
		out = append(out, toAgentInfo(a))
	}
	jsonOK(w, out)
}

func (h *Handler) adminApproveAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.reg.SetApprovalStatus(r.Context(), id, "active"); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"approval_status": "active"})
}

func (h *Handler) adminDisableAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.reg.SetApprovalStatus(r.Context(), id, "disabled"); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"approval_status": "disabled"})
}

// ---- helpers ----

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
