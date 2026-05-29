package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// StepCAClient issues mTLS certificates for agents via step-ca's sign endpoint.
type StepCAClient struct {
	url         string
	provisioner string
	privateKey  *ecdsa.PrivateKey
	httpClient  *http.Client
}

func NewStepCAClient(url, provisioner string, privateKey *ecdsa.PrivateKey) *StepCAClient {
	return &StepCAClient{
		url:         url,
		provisioner: provisioner,
		privateKey:  privateKey,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// NewStepCAClientFromEnv creates a client from STEPCA_URL, STEPCA_PROVISIONER, STEPCA_PROVISIONER_KEY.
func NewStepCAClientFromEnv() (*StepCAClient, error) {
	url := os.Getenv("STEPCA_URL")
	if url == "" {
		url = "http://localhost:9000"
	}
	provisioner := os.Getenv("STEPCA_PROVISIONER")
	if provisioner == "" {
		provisioner = "backend@sharedgpupower"
	}
	keyPEM := os.Getenv("STEPCA_PROVISIONER_KEY")
	if keyPEM == "" {
		return nil, fmt.Errorf("STEPCA_PROVISIONER_KEY env var required")
	}
	key, err := parseECPrivateKeyPEM([]byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("parse provisioner key: %w", err)
	}
	return NewStepCAClient(url, provisioner, key), nil
}

// RequestCert sends a CSR to step-ca and returns the signed cert PEM and CA cert PEM.
func (c *StepCAClient) RequestCert(ctx context.Context, csrPEM []byte, agentID string) (certPEM, caCertPEM []byte, err error) {
	ott, err := c.makeOTT(agentID)
	if err != nil {
		return nil, nil, fmt.Errorf("sign OTT: %w", err)
	}

	body, _ := json.Marshal(map[string]string{
		"csr": string(csrPEM),
		"ott": ott,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.url+"/1.0/sign", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("step-ca sign: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("step-ca status %d: %s", resp.StatusCode, b)
	}

	var result struct {
		CRT string `json:"crt"`
		CA  string `json:"ca"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decode step-ca response: %w", err)
	}
	return []byte(result.CRT), []byte(result.CA), nil
}

// makeOTT creates a signed one-time token JWT for the step-ca JWK provisioner.
func (c *StepCAClient) makeOTT(agentID string) (string, error) {
	now := time.Now()
	claims := map[string]any{
		"iss":  c.provisioner,
		"sub":  agentID,
		"aud":  c.url + "/1.0/sign",
		"iat":  now.Unix(),
		"nbf":  now.Unix(),
		"exp":  now.Add(5 * time.Minute).Unix(),
		"sans": []string{agentID},
	}
	return signES256JWT(c.privateKey, claims)
}

// signES256JWT signs claims with ECDSA P-256 / SHA-256 (ES256) and returns a compact JWT.
func signES256JWT(key *ecdsa.PrivateKey, claims map[string]any) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256","typ":"JWT"}`))
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sigInput := header + "." + payload

	h := sha256.Sum256([]byte(sigInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, h[:])
	if err != nil {
		return "", err
	}

	// IEEE P1363 format: 32-byte r || 32-byte s
	sig := make([]byte, 64)
	rb, sb := r.Bytes(), s.Bytes()
	copy(sig[32-len(rb):32], rb)
	copy(sig[64-len(sb):64], sb)

	return sigInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseECPrivateKeyPEM(pemData []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}
