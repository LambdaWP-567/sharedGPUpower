package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Backend   BackendConfig   `yaml:"backend"`
	Resources ResourceConfig  `yaml:"resources"`
	Ollama    OllamaConfig    `yaml:"ollama"`
	Agent     AgentConfig     `yaml:"agent"`
}

type BackendConfig struct {
	Endpoint string `yaml:"endpoint"`
	TLSCert  string `yaml:"tls_cert"`
	TLSKey   string `yaml:"tls_key"`
	Insecure bool   `yaml:"insecure"`
}

type ResourceConfig struct {
	CPUPercent float32 `yaml:"cpu_percent"`
	RAMPercent float32 `yaml:"ram_percent"`
	GPULayers  int32   `yaml:"gpu_layers"`
}

type OllamaConfig struct {
	Host         string `yaml:"host"`
	DefaultModel string `yaml:"default_model"`
}

type AgentConfig struct {
	Name string `yaml:"name"`
	ID   string `yaml:"id"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// Defaults
	if cfg.Backend.Endpoint == "" {
		cfg.Backend.Endpoint = "localhost:9090"
	}
	if cfg.Ollama.Host == "" {
		cfg.Ollama.Host = "http://localhost:11434"
	}
	if cfg.Ollama.DefaultModel == "" {
		cfg.Ollama.DefaultModel = "llama3.2:3b"
	}
	if cfg.Resources.CPUPercent == 0 {
		cfg.Resources.CPUPercent = 50
	}
	if cfg.Resources.RAMPercent == 0 {
		cfg.Resources.RAMPercent = 25
	}
	if cfg.Resources.GPULayers == 0 {
		cfg.Resources.GPULayers = 20
	}
	return &cfg, nil
}
