package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/lambdawp-567/sharedgpupower/backend/internal/jobs"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/registry"
	"github.com/lambdawp-567/sharedgpupower/backend/internal/tokens"
	"go.uber.org/zap"
)

type JobRequest struct {
	JobID            string
	SubmitterAgentID string
	Type             string
	Payload          map[string]any
	RequiredGPULayers int32
}

type Scheduler struct {
	mu       sync.Mutex
	queue    []*JobRequest
	reg      *registry.Registry
	store    *jobs.Store
	ledger   *tokens.Ledger
	log      *zap.Logger
	notify   chan struct{}
}

func New(reg *registry.Registry, store *jobs.Store, ledger *tokens.Ledger, log *zap.Logger) *Scheduler {
	return &Scheduler{
		reg:    reg,
		store:  store,
		ledger: ledger,
		log:    log,
		notify: make(chan struct{}, 64),
	}
}

func (s *Scheduler) Enqueue(req *JobRequest) {
	s.mu.Lock()
	s.queue = append(s.queue, req)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.notify:
			s.dispatch(ctx)
		case <-ticker.C:
			s.dispatch(ctx)
		}
	}
}

func (s *Scheduler) dispatch(ctx context.Context) {
	s.mu.Lock()
	if len(s.queue) == 0 {
		s.mu.Unlock()
		return
	}
	req := s.queue[0]
	s.queue = s.queue[1:]
	s.mu.Unlock()

	agent := s.selectAgent(req)
	if agent == nil {
		s.log.Warn("no suitable agent, requeueing", zap.String("job_id", req.JobID))
		s.mu.Lock()
		s.queue = append(s.queue, req)
		s.mu.Unlock()
		return
	}

	if err := s.store.Assign(ctx, req.JobID, agent.ID); err != nil {
		s.log.Error("assign failed", zap.Error(err))
		return
	}

	payload, _ := json.Marshal(req.Payload)
	select {
	case agent.JobCh <- payload:
		s.log.Info("job dispatched",
			zap.String("job_id", req.JobID),
			zap.String("agent_id", agent.ID))
	default:
		s.log.Warn("agent job channel full, requeueing", zap.String("agent_id", agent.ID))
		s.store.Fail(ctx, req.JobID, "agent channel full")
	}
}

// selectAgent picks the best available agent by benchmark score and GPU capability.
func (s *Scheduler) selectAgent(req *JobRequest) *registry.Agent {
	available := s.reg.GetAvailable()
	if len(available) == 0 {
		return nil
	}

	var candidates []*registry.Agent
	for _, a := range available {
		if a.ID == req.SubmitterAgentID {
			continue // don't assign to submitter (avoid loops)
		}
		if req.RequiredGPULayers > 0 && a.GPULayersLimit < req.RequiredGPULayers {
			continue
		}
		if len(a.JobCh) >= cap(a.JobCh) {
			continue // full
		}
		candidates = append(candidates, a)
	}
	if len(candidates) == 0 {
		return nil
	}

	// Sort by benchmark score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].BenchScore > candidates[j].BenchScore
	})
	return candidates[0]
}

func (s *Scheduler) CompleteJob(ctx context.Context, jobID, agentID string, durationSeconds float64) error {
	agent, ok := s.reg.Get(agentID)
	if !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	durationMin := durationSeconds / 60.0
	resourceFraction := float64(agent.GPULayersLimit) / 32.0
	if resourceFraction == 0 {
		resourceFraction = float64(agent.CPULimit) / 100.0
	}

	cost := tokens.CalcCost(agent.BenchScore, durationMin, resourceFraction)

	if err := s.store.Complete(ctx, jobID, cost); err != nil {
		return err
	}
	return s.ledger.Credit(ctx, agentID, jobID, cost)
}
