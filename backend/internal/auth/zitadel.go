package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// ZitadelIntrospector validates JWTs issued by a Zitadel instance using the
// OIDC UserInfo endpoint. This is simpler than full local JWT validation and
// works without configuring JWKS keys locally.
type ZitadelIntrospector struct {
	domain     string
	httpClient *http.Client
	cache      map[string]cachedUserInfo
	mu         sync.RWMutex
}

type cachedUserInfo struct {
	zitadelID string
	email     string
	name      string
	avatarURL string
	expiresAt time.Time
}

func NewZitadelIntrospector(domain string) *ZitadelIntrospector {
	return &ZitadelIntrospector{
		domain: domain,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		cache: make(map[string]cachedUserInfo),
	}
}

// NewZitadelIntrospectorFromEnv creates an introspector using ZITADEL_DOMAIN env var.
func NewZitadelIntrospectorFromEnv() *ZitadelIntrospector {
	domain := os.Getenv("ZITADEL_DOMAIN")
	if domain == "" {
		domain = "localhost:8081"
	}
	return NewZitadelIntrospector(domain)
}

// Introspect validates a Bearer token via Zitadel's OIDC UserInfo endpoint.
func (z *ZitadelIntrospector) Introspect(ctx context.Context, token string) (zitadelID, email, name, avatarURL string, err error) {
	// Check cache first (5-minute TTL)
	z.mu.RLock()
	if cached, ok := z.cache[token]; ok && time.Now().Before(cached.expiresAt) {
		z.mu.RUnlock()
		return cached.zitadelID, cached.email, cached.name, cached.avatarURL, nil
	}
	z.mu.RUnlock()

	scheme := "https"
	if isLocalHost(z.domain) {
		scheme = "http"
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s://%s/oidc/v1/userinfo", scheme, z.domain), nil)
	if err != nil {
		return "", "", "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := z.httpClient.Do(req)
	if err != nil {
		return "", "", "", "", fmt.Errorf("zitadel userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", "", "", "", fmt.Errorf("invalid token")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", "", fmt.Errorf("userinfo status %d: %s", resp.StatusCode, body)
	}

	var info struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", "", "", err
	}

	// Cache for 5 minutes
	z.mu.Lock()
	z.cache[token] = cachedUserInfo{
		zitadelID: info.Sub,
		email:     info.Email,
		name:      info.Name,
		avatarURL: info.Picture,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	z.mu.Unlock()

	return info.Sub, info.Email, info.Name, info.Picture, nil
}

func isLocalHost(domain string) bool {
	for _, prefix := range []string{"localhost", "127.", "::1"} {
		if len(domain) >= len(prefix) && domain[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
