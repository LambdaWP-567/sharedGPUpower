package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusDone     Status = "done"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)

type Job struct {
	ID               string
	SubmitterAgentID string
	AssignedAgentID  string
	Type             string
	Payload          map[string]any
	Status           Status
	TokenCost        float64
	Error            string
	CreatedAt        time.Time
	CompletedAt      *time.Time
}

type Store struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func NewStore(db *pgxpool.Pool, log *zap.Logger) *Store {
	return &Store{db: db, log: log}
}

func (s *Store) Create(ctx context.Context, submitterID, jobType string, payload map[string]any) (*Job, error) {
	j := &Job{
		ID:               uuid.New().String(),
		SubmitterAgentID: submitterID,
		Type:             jobType,
		Payload:          payload,
		Status:           StatusPending,
		CreatedAt:        time.Now(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO jobs (id, submitter_agent_id, type, payload, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		j.ID, j.SubmitterAgentID, j.Type, raw, string(j.Status))
	return j, err
}

func (s *Store) Assign(ctx context.Context, jobID, agentID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE jobs SET assigned_agent_id=$1, status='running' WHERE id=$2`,
		agentID, jobID)
	return err
}

func (s *Store) Complete(ctx context.Context, jobID string, tokenCost float64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE jobs SET status='done', token_cost=$1, completed_at=NOW() WHERE id=$2`,
		tokenCost, jobID)
	return err
}

func (s *Store) Fail(ctx context.Context, jobID, errMsg string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE jobs SET status='failed', error=$1, completed_at=NOW() WHERE id=$2`,
		errMsg, jobID)
	return err
}

func (s *Store) Get(ctx context.Context, jobID string) (*Job, error) {
	j := &Job{}
	var payload []byte
	var statusStr string
	var completedAt *time.Time

	err := s.db.QueryRow(ctx,
		`SELECT id, submitter_agent_id, COALESCE(assigned_agent_id,''), type, payload, status,
		        token_cost, COALESCE(error,''), created_at, completed_at
		 FROM jobs WHERE id=$1`, jobID).
		Scan(&j.ID, &j.SubmitterAgentID, &j.AssignedAgentID, &j.Type,
			&payload, &statusStr, &j.TokenCost, &j.Error, &j.CreatedAt, &completedAt)
	if err != nil {
		return nil, fmt.Errorf("job not found: %w", err)
	}
	j.Status = Status(statusStr)
	j.CompletedAt = completedAt

	if err := json.Unmarshal(payload, &j.Payload); err != nil {
		s.log.Warn("failed to unmarshal job payload", zap.String("job_id", jobID), zap.Error(err))
		j.Payload = map[string]any{}
	}
	return j, nil
}

func (s *Store) ListBySubmitter(ctx context.Context, agentID string, limit int) ([]*Job, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, submitter_agent_id, COALESCE(assigned_agent_id,''), type, payload, status,
		        token_cost, COALESCE(error,''), created_at, completed_at
		 FROM jobs WHERE submitter_agent_id=$1 ORDER BY created_at DESC LIMIT $2`,
		agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Job
	for rows.Next() {
		j := &Job{}
		var payload []byte
		var statusStr string
		var completedAt *time.Time
		if err := rows.Scan(&j.ID, &j.SubmitterAgentID, &j.AssignedAgentID, &j.Type,
			&payload, &statusStr, &j.TokenCost, &j.Error, &j.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		j.Status = Status(statusStr)
		j.CompletedAt = completedAt
		if err := json.Unmarshal(payload, &j.Payload); err != nil {
			j.Payload = map[string]any{}
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
