package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	host       string
	maxLayers  int32
	httpClient *http.Client
}

func New(host string, maxGPULayers int32) *Client {
	return &Client{
		host:      host,
		maxLayers: maxGPULayers,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type GenerateRequest struct {
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type GenerateResponse struct {
	Model              string `json:"model"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	EvalCount          int    `json:"eval_count"`
	EvalDurationNanos  int64  `json:"eval_duration"`
}

func (c *Client) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if req.Options == nil {
		req.Options = map[string]any{}
	}
	// Enforce GPU layer limit
	if c.maxLayers > 0 {
		req.Options["num_gpu"] = c.maxLayers
	}
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama status %d", resp.StatusCode)
	}

	var result GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.host+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama ping: status %d", resp.StatusCode)
	}
	return nil
}
