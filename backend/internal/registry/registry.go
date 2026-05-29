package registry

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Agent struct {
	ID             string
	Name           string
	PublicKey      string
	Arch           string
	OS             string
	GPUModel       string
	CPUCores       int32
	RAMGB          float32
	GPULayersTotal int32
	BenchScore     float64
	CPULimit       float32
	RAMLimit       float32
	GPULayersLimit int32
	Status         string
	LastSeen       time.Time
	JobCh          chan []byte
}

type Registry struct {
	mu     sync.RWMutex
	agents map[string]*Agent
	db     *pgxpool.Pool
	log    *zap.Logger
}

func New(db *pgxpool.Pool, log *zap.Logger) *Registry {
	return &Registry{
		agents: make(map[string]*Agent),
		db:     db,
		log:    log,
	}
}

func (r *Registry) Register(ctx context.Context, a *Agent) (string, error) {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	a.Status = "online"
	a.LastSeen = time.Now()
	a.JobCh = make(chan []byte, 8)

	_, err := r.db.Exec(ctx, `
		INSERT INTO agents (id, name, public_key, arch, os, gpu_model, cpu_cores, ram_gb,
		    gpu_layers, bench_score, cpu_limit, ram_limit, gpu_layers_limit, status, last_seen)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'online',NOW())
		ON CONFLICT (id) DO UPDATE SET
		    name=$2, public_key=$3, bench_score=$10, cpu_limit=$11, ram_limit=$12,
		    gpu_layers_limit=$13, status='online', last_seen=NOW()`,
		a.ID, a.Name, a.PublicKey, a.Arch, a.OS, a.GPUModel,
		a.CPUCores, a.RAMGB, a.GPULayersTotal, a.BenchScore,
		a.CPULimit, a.RAMLimit, a.GPULayersLimit,
	)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	r.agents[a.ID] = a
	r.mu.Unlock()
	return a.ID, nil
}

func (r *Registry) Heartbeat(ctx context.Context, agentID string) error {
	r.mu.Lock()
	if a, ok := r.agents[agentID]; ok {
		a.LastSeen = time.Now()
		a.Status = "online"
	}
	r.mu.Unlock()

	_, err := r.db.Exec(ctx,
		`UPDATE agents SET status='online', last_seen=NOW() WHERE id=$1`, agentID)
	return err
}

func (r *Registry) SetOffline(agentID string) {
	r.mu.Lock()
	if a, ok := r.agents[agentID]; ok {
		a.Status = "offline"
	}
	r.mu.Unlock()
}

func (r *Registry) GetAvailable() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Agent
	for _, a := range r.agents {
		if a.Status == "online" {
			out = append(out, a)
		}
	}
	return out
}

func (r *Registry) Get(id string) (*Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[id]
	return a, ok
}

func (r *Registry) All() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}

// SweepStale marks agents offline if no heartbeat for >30s.
func (r *Registry) SweepStale(ctx context.Context) {
	threshold := time.Now().Add(-30 * time.Second)
	r.mu.Lock()
	var stale []string
	for id, a := range r.agents {
		if a.Status == "online" && a.LastSeen.Before(threshold) {
			a.Status = "offline"
			stale = append(stale, id)
		}
	}
	r.mu.Unlock()

	for _, id := range stale {
		r.db.Exec(ctx, `UPDATE agents SET status='offline' WHERE id=$1`, id)
		r.log.Info("agent marked offline", zap.String("agent_id", id))
	}
}
