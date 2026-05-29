package router

import (
	"strings"
)

type Decision int

const (
	RunLocal  Decision = iota
	RunRemote
)

// Route decides whether a prompt should be handled locally or offloaded to the network.
// Rules (in priority order):
//  1. Explicit [OFFLOAD] marker in prompt → remote
//  2. Prompt length > contextThreshold tokens → remote
//  3. Otherwise → local
func Route(prompt, model string, contextThreshold int) Decision {
	if strings.Contains(prompt, "[OFFLOAD]") {
		return RunRemote
	}
	// Rough token estimate: 1 token ≈ 4 chars
	estimatedTokens := len(prompt) / 4
	if estimatedTokens > contextThreshold {
		return RunRemote
	}
	return RunLocal
}
