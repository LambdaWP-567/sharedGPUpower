package tokens

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Ledger struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Ledger {
	return &Ledger{db: db}
}

// Credit adds tokens to an agent for contributing compute.
func (l *Ledger) Credit(ctx context.Context, agentID, jobID string, amount float64) error {
	_, err := l.db.Exec(ctx,
		`INSERT INTO token_ledger (agent_id, delta, reason, job_id) VALUES ($1, $2, 'job_completed', $3)`,
		agentID, amount, jobID)
	return err
}

// Debit removes tokens from an agent when consuming compute.
func (l *Ledger) Debit(ctx context.Context, agentID, jobID string, amount float64) error {
	_, err := l.db.Exec(ctx,
		`INSERT INTO token_ledger (agent_id, delta, reason, job_id) VALUES ($1, $2, 'job_consumed', $3)`,
		agentID, -amount, jobID)
	return err
}

// Balance returns the current token balance for an agent.
func (l *Ledger) Balance(ctx context.Context, agentID string) (float64, error) {
	var bal float64
	err := l.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(delta), 0) FROM token_ledger WHERE agent_id=$1`, agentID).Scan(&bal)
	return bal, err
}

// HasSufficientBalance checks if agent can afford a job.
func (l *Ledger) HasSufficientBalance(ctx context.Context, agentID string, cost float64) (bool, error) {
	bal, err := l.Balance(ctx, agentID)
	if err != nil {
		return false, err
	}
	return bal >= cost, nil
}

// CalcCost computes token cost: benchScore × durationMinutes × resourceFraction.
func CalcCost(benchScore, durationMinutes, resourceFraction float64) float64 {
	return benchScore * durationMinutes * resourceFraction
}
