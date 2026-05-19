package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Workspace string       `yaml:"workspace"`
	Remote    RemoteConfig `yaml:"remote"`
	Sync      SyncConfig   `yaml:"sync"`
	Ignore    []string     `yaml:"ignore"`
}

type RemoteConfig struct {
	Host string `yaml:"host"`
	Path string `yaml:"path"`
}

type SyncConfig struct {
	Mode string `yaml:"mode"`
}

func DefaultConfig(name string) Config {
	return Config{
		Workspace: name,
		Sync:      SyncConfig{Mode: "mutagen"},
		Ignore:    []string{".git", "node_modules", "dist", "build", ".cache", ".next", "coverage"},
	}
}

func LoadConfig(workspaceName string) (Config, error) {
	path, err := ConfigPath(workspaceName)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("workspace config not found: %s (run devsync init)", path)
		}
		return Config{}, fmt.Errorf("read workspace config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse workspace config %s: %w", path, err)
	}
	if cfg.Workspace == "" {
		cfg.Workspace = workspaceName
	}
	if cfg.Remote.Host == "" {
		return Config{}, fmt.Errorf("workspace config missing remote.host")
	}
	if cfg.Remote.Path == "" {
		return Config{}, fmt.Errorf("workspace config missing remote.path")
	}
	if cfg.Sync.Mode == "" {
		cfg.Sync.Mode = "mutagen"
	}
	if !contains(cfg.Ignore, ".git") {
		cfg.Ignore = append([]string{".git"}, cfg.Ignore...)
	}
	return cfg, nil
}

func WriteConfig(cfg Config) (string, error) {
	path, err := ConfigPath(cfg.Workspace)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("render workspace config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write workspace config: %w", err)
	}
	return path, nil
}

func ConfigPath(workspaceName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "devsync", workspaceName+".yaml"), nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
