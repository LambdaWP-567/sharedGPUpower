package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lambdawp-567/sharedgpupower/agent/internal/ollama"
	"go.uber.org/zap"
)

type Result struct {
	JobID     string
	Success   bool
	Output    []byte
	Error     string
	Duration  time.Duration
}

type Executor struct {
	ollama *ollama.Client
	log    *zap.Logger
}

func New(ollamaClient *ollama.Client, log *zap.Logger) *Executor {
	return &Executor{ollama: ollamaClient, log: log}
}

func (e *Executor) Execute(ctx context.Context, jobID, jobType string, payload []byte) *Result {
	start := time.Now()
	res := &Result{JobID: jobID}

	switch jobType {
	case "llm_inference", "":
		output, err := e.runLLM(ctx, payload)
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
			res.Output = output
		}
	default:
		res.Error = fmt.Sprintf("unknown job type: %s", jobType)
	}

	res.Duration = time.Since(start)
	return res
}

func (e *Executor) runLLM(ctx context.Context, rawPayload []byte) ([]byte, error) {
	var payload struct {
		Model     string  `json:"model"`
		Prompt    string  `json:"prompt"`
		MaxTokens int     `json:"max_tokens"`
		Temp      float64 `json:"temperature"`
	}
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if payload.Prompt == "" {
		return nil, fmt.Errorf("prompt is empty")
	}

	opts := map[string]any{}
	if payload.MaxTokens > 0 {
		opts["num_predict"] = payload.MaxTokens
	}
	if payload.Temp > 0 {
		opts["temperature"] = payload.Temp
	}

	resp, err := e.ollama.Generate(ctx, ollama.GenerateRequest{
		Model:   payload.Model,
		Prompt:  payload.Prompt,
		Options: opts,
	})
	if err != nil {
		return nil, err
	}

	out, _ := json.Marshal(map[string]any{
		"response":         resp.Response,
		"eval_count":       resp.EvalCount,
		"eval_duration_ns": resp.EvalDurationNanos,
	})
	return out, nil
}
