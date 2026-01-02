package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Domain        string `yaml:"domain"`
	ProjectsDir   string `yaml:"projects_dir"`
	CertPath      string `yaml:"cert_path"`
	KeyPath       string `yaml:"key_path"`
	CaddyfilePath string `yaml:"caddyfile_path"`
}

func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "dev-router", "config.yaml")
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

	// Expand ~ in paths
	cfg.ProjectsDir = expandHome(cfg.ProjectsDir)
	cfg.CertPath = expandHome(cfg.CertPath)
	cfg.KeyPath = expandHome(cfg.KeyPath)
	cfg.CaddyfilePath = expandHome(cfg.CaddyfilePath)

	return &cfg, nil
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
