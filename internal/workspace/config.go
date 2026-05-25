package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/danew/devsync/internal/apperrors"
	devssh "github.com/danew/devsync/internal/ssh"
	"gopkg.in/yaml.v3"
)

const (
	LocalOverrideFile = ".devsync.yaml"
	GlobalConfigFile  = "config.yaml"
)

var SupportedWorkspaceConfigFiles = []string{LocalOverrideFile}

var internalDefaultIgnores = []string{".git", "node_modules", "dist", "build", ".cache", ".next", "coverage"}

type Config struct {
	Workspace WorkspaceIdentity
	Remote    RemoteConfig
	Sync      SyncConfig
	Forward   ForwardConfig
	Mapping   MappingConfig
	Sources   ConfigSources
}

type WorkspaceIdentity struct {
	Name string
}

type RemoteConfig struct {
	Node   string
	Host   string
	SSH    SSHConfig
	Path   string
	Target devssh.Target
}

type SSHConfig struct {
	User string `yaml:"user"`
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

type SyncConfig struct {
	Mode    string
	Ignores []string
}

type ForwardConfig struct {
	Ports []PortForward `yaml:"ports"`
}

func (f ForwardConfig) IsZero() bool {
	return len(f.Ports) == 0
}

type PortForward struct {
	LocalHost  string `yaml:"local_host,omitempty"`
	LocalPort  string `yaml:"local,omitempty"`
	RemoteHost string `yaml:"host,omitempty"`
	RemotePort string `yaml:"remote,omitempty"`
}

type MappingConfig struct {
	LocalRoot       string
	RemoteRoot      string
	RelativePath    string
	ConventionBased bool
}

type ConfigSources struct {
	GlobalPath         string
	GlobalLoaded       bool
	LocalOverridePath  string
	LocalOverrideFound bool
	RemoteNodeSource   string
	RemotePathSource   string
	IgnoreSource       string
}

type GlobalConfig struct {
	Nodes       map[string]NodeConfig `yaml:"nodes"`
	DefaultNode string                `yaml:"default_node"`
	Defaults    DefaultsConfig        `yaml:"defaults"`
	Mapping     GlobalMappingConfig   `yaml:"mapping"`
}

func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Nodes: map[string]NodeConfig{
			"core-dev": {SSH: "core-dev", WorkspaceRoot: "~/workspace"},
		},
		DefaultNode: "core-dev",
		Defaults:    DefaultsConfig{Ignores: append([]string{}, internalDefaultIgnores...)},
		Mapping:     GlobalMappingConfig{LocalRoot: "~/remote"},
	}
}

type NodeConfig struct {
	SSH           string `yaml:"ssh"`
	WorkspaceRoot string `yaml:"workspace_root"`
}

type DefaultsConfig struct {
	Ignores []string `yaml:"ignores"`
}

type GlobalMappingConfig struct {
	LocalRoot string `yaml:"local_root"`
}

type LocalConfig struct {
	Workspace string            `yaml:"workspace"`
	Remote    LocalRemoteConfig `yaml:"remote"`
	Sync      LocalSyncConfig   `yaml:"sync"`
	Forward   ForwardConfig     `yaml:"forward,omitempty"`
	Ignore    []string          `yaml:"ignore"`
}

type LocalRemoteConfig struct {
	Node string    `yaml:"node"`
	Host string    `yaml:"host"`
	SSH  SSHConfig `yaml:"ssh"`
	Path string    `yaml:"path"`
}

type LocalSyncConfig struct {
	Mode    string   `yaml:"mode"`
	Ignores []string `yaml:"ignores"`
}

func DefaultConfig(name string) Config {
	return Config{
		Workspace: WorkspaceIdentity{Name: name},
		Remote:    RemoteConfig{Node: "core-dev", Host: "core-dev"},
		Sync:      SyncConfig{Mode: "mutagen", Ignores: append([]string{}, internalDefaultIgnores...)},
		Mapping:   MappingConfig{LocalRoot: "~/remote", RemoteRoot: "~/workspace", ConventionBased: true},
	}
}

func ResolveConfig(ws Workspace) (Config, error) {
	global, sources, err := LoadGlobalConfig()
	if err != nil {
		return Config{}, err
	}
	local, localFound, localPath, err := LoadLocalConfig(ws.Root)
	if err != nil {
		return Config{}, err
	}
	sources.LocalOverridePath = localPath
	sources.LocalOverrideFound = localFound
	return ResolveConfigFromLayers(ws, global, local, localFound, sources)
}

func ResolveConfigFromLayers(ws Workspace, global GlobalConfig, local LocalConfig, localFound bool, sources ConfigSources) (Config, error) {
	cfg := DefaultConfig(ws.Name)
	cfg.Sources = sources
	cfg.Sources.RemoteNodeSource = "convention"
	cfg.Sources.RemotePathSource = "convention"
	cfg.Sources.IgnoreSource = "internal defaults"

	if global.Mapping.LocalRoot != "" {
		cfg.Mapping.LocalRoot = global.Mapping.LocalRoot
	}
	if global.DefaultNode != "" {
		cfg.Remote.Node = global.DefaultNode
		cfg.Remote.Host = global.DefaultNode
		cfg.Sources.RemoteNodeSource = "global config"
	} else if len(global.Nodes) == 1 {
		for node := range global.Nodes {
			cfg.Remote.Node = node
			cfg.Remote.Host = node
			cfg.Sources.RemoteNodeSource = "global config"
		}
	}

	if node, ok := global.Nodes[cfg.Remote.Node]; ok {
		if node.SSH != "" {
			cfg.Remote.Host = node.SSH
		}
		if node.WorkspaceRoot != "" {
			cfg.Mapping.RemoteRoot = node.WorkspaceRoot
		}
	}

	if localFound {
		if local.Workspace != "" {
			cfg.Workspace.Name = local.Workspace
		}
		if local.Remote.Node != "" {
			cfg.Remote.Node = local.Remote.Node
			cfg.Remote.Host = local.Remote.Node
			cfg.Sources.RemoteNodeSource = "workspace override"
		}
		if local.Remote.Host != "" {
			cfg.Remote.Host = local.Remote.Host
			cfg.Sources.RemoteNodeSource = "workspace override"
		}
		if local.Remote.SSH.Host != "" {
			cfg.Remote.SSH = local.Remote.SSH
			cfg.Remote.Host = formatSSHTarget(local.Remote.SSH)
			cfg.Sources.RemoteNodeSource = "workspace override"
		}
		if node, ok := global.Nodes[cfg.Remote.Node]; ok {
			if node.SSH != "" && local.Remote.Host == "" {
				cfg.Remote.Host = node.SSH
			}
			if node.WorkspaceRoot != "" {
				cfg.Mapping.RemoteRoot = node.WorkspaceRoot
			}
		}
		if local.Sync.Mode != "" {
			cfg.Sync.Mode = local.Sync.Mode
		}
		cfg.Forward = normalizeForwardConfig(local.Forward)
	}

	if localFound && local.Remote.Path != "" {
		cfg.Remote.Path = local.Remote.Path
		cfg.Mapping.ConventionBased = false
		cfg.Sources.RemotePathSource = "workspace override"
	} else {
		path, relative, err := inferRemotePath(ws.Root, cfg.Mapping.LocalRoot, cfg.Mapping.RemoteRoot)
		if err != nil {
			return Config{}, err
		}
		cfg.Remote.Path = path
		cfg.Mapping.RelativePath = relative
		cfg.Mapping.ConventionBased = true
	}

	ignores := append([]string{}, internalDefaultIgnores...)
	if len(global.Defaults.Ignores) > 0 {
		ignores = append(ignores, global.Defaults.Ignores...)
		cfg.Sources.IgnoreSource = "internal defaults + global config"
	}
	if localFound && (len(local.Sync.Ignores) > 0 || len(local.Ignore) > 0) {
		ignores = append(ignores, local.Sync.Ignores...)
		ignores = append(ignores, local.Ignore...)
		cfg.Sources.IgnoreSource = "internal defaults + global config + workspace override"
	}
	cfg.Sync.Ignores = normalizeIgnores(ignores)
	cfg.Remote.Target = normalizeRemoteTarget(cfg.Remote.Host, cfg.Remote.SSH)

	if cfg.Workspace.Name == "" {
		return Config{}, fmt.Errorf("workspace identity could not be resolved")
	}
	if cfg.Remote.Node == "" || cfg.Remote.Host == "" || cfg.Remote.Path == "" {
		return Config{}, fmt.Errorf("remote configuration could not be resolved safely")
	}
	return cfg, nil
}

func normalizeRemoteTarget(host string, ssh SSHConfig) devssh.Target {
	if ssh.Host != "" {
		return devssh.Target{User: ssh.User, Host: ssh.Host, Port: ssh.Port}
	}
	return devssh.ParseTarget(host)
}

func formatSSHTarget(ssh SSHConfig) string {
	return normalizeRemoteTarget("", ssh).String()
}

func (p *PortForward) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		port := strings.TrimSpace(node.Value)
		if port == "" {
			return fmt.Errorf("forward port cannot be empty")
		}
		*p = PortForward{LocalPort: port, RemoteHost: "127.0.0.1", RemotePort: port}
		return nil
	case yaml.MappingNode:
		values := map[string]string{}
		for i := 0; i < len(node.Content); i += 2 {
			key := strings.TrimSpace(node.Content[i].Value)
			value := strings.TrimSpace(node.Content[i+1].Value)
			values[key] = value
		}
		*p = PortForward{
			LocalHost:  values["local_host"],
			LocalPort:  firstNonEmpty(values["local"], values["local_port"]),
			RemoteHost: firstNonEmpty(values["host"], values["remote_host"], "127.0.0.1"),
			RemotePort: firstNonEmpty(values["remote"], values["remote_port"]),
		}
		if p.LocalPort == "" {
			return fmt.Errorf("forward port mapping requires local")
		}
		if p.RemotePort == "" {
			p.RemotePort = p.LocalPort
		}
		return nil
	default:
		return fmt.Errorf("forward port must be a port number or mapping")
	}
}

func normalizeForwardConfig(cfg ForwardConfig) ForwardConfig {
	ports := []PortForward{}
	for _, port := range cfg.Ports {
		port.LocalHost = strings.TrimSpace(port.LocalHost)
		port.LocalPort = strings.TrimSpace(port.LocalPort)
		port.RemoteHost = strings.TrimSpace(port.RemoteHost)
		port.RemotePort = strings.TrimSpace(port.RemotePort)
		if port.RemoteHost == "" {
			port.RemoteHost = "127.0.0.1"
		}
		if port.RemotePort == "" {
			port.RemotePort = port.LocalPort
		}
		if port.LocalPort == "" {
			continue
		}
		ports = append(ports, port)
	}
	return ForwardConfig{Ports: ports}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func LoadGlobalConfig() (GlobalConfig, ConfigSources, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return GlobalConfig{}, ConfigSources{}, err
	}
	sources := ConfigSources{GlobalPath: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GlobalConfig{}, sources, nil
		}
		return GlobalConfig{}, sources, fmt.Errorf("read global config: %w", err)
	}
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return GlobalConfig{}, sources, fmt.Errorf("parse global config %s: %w", path, err)
	}
	sources.GlobalLoaded = true
	return cfg, sources, nil
}

func EnsureGlobalConfig() (string, bool, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return "", false, err
	}
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, err
	}
	data, err := yaml.Marshal(DefaultGlobalConfig())
	if err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", false, err
	}
	return path, true, nil
}

func LoadLocalConfig(repoRoot string) (LocalConfig, bool, string, error) {
	path := filepath.Join(repoRoot, LocalOverrideFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LocalConfig{}, false, path, nil
		}
		return LocalConfig{}, false, path, fmt.Errorf("read workspace override: %w", err)
	}
	var cfg LocalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return LocalConfig{}, false, path, fmt.Errorf("parse workspace override %s: %w", path, err)
	}
	return cfg, true, path, nil
}

func HasWorkspaceConfig(repoRoot string) (bool, string, error) {
	for _, name := range SupportedWorkspaceConfigFiles {
		path := filepath.Join(repoRoot, name)
		_, err := os.Stat(path)
		if err == nil {
			return true, path, nil
		}
		if !os.IsNotExist(err) {
			return false, path, err
		}
	}
	return false, filepath.Join(repoRoot, LocalOverrideFile), nil
}

func WriteConfig(cfg Config) (string, error) {
	path, err := ConfigPath(cfg.Workspace.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(LocalConfig{
		Workspace: cfg.Workspace.Name,
		Remote:    LocalRemoteConfig{Node: cfg.Remote.Node, Host: cfg.Remote.Host, SSH: cfg.Remote.SSH, Path: cfg.Remote.Path},
		Sync:      LocalSyncConfig{Mode: cfg.Sync.Mode, Ignores: cfg.Sync.Ignores},
	})
	if err != nil {
		return "", fmt.Errorf("render workspace config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write workspace config: %w", err)
	}
	return path, nil
}

func WriteLocalOverride(repoRoot string, cfg Config) (string, error) {
	path := filepath.Join(repoRoot, LocalOverrideFile)
	data, err := yaml.Marshal(LocalConfig{
		Workspace: cfg.Workspace.Name,
		Remote:    LocalRemoteConfig{Node: cfg.Remote.Node, Host: cfg.Remote.Host, SSH: cfg.Remote.SSH, Path: cfg.Remote.Path},
		Sync:      LocalSyncConfig{Mode: cfg.Sync.Mode, Ignores: cfg.Sync.Ignores},
	})
	if err != nil {
		return "", fmt.Errorf("render workspace override: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write workspace override: %w", err)
	}
	return path, nil
}

func ConfigPath(workspaceName string) (string, error) {
	dir, err := DevSyncConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workspaceName+".yaml"), nil
}

func GlobalConfigPath() (string, error) {
	dir, err := DevSyncConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, GlobalConfigFile), nil
}

func DevSyncConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "devsync"), nil
}

func LoadConfig(workspaceName string) (Config, error) {
	path, err := ConfigPath(workspaceName)
	if err != nil {
		return Config{}, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Config{}, apperrors.New(apperrors.ErrWorkspaceConfigMissing, fmt.Sprintf("workspace config not found: %s (run devsync init)", path))
		}
		return Config{}, err
	}
	return Config{}, fmt.Errorf("legacy per-workspace config loading is no longer used by commands; use ResolveConfig")
}

func inferRemotePath(localPath, localRoot, remoteRoot string) (string, string, error) {
	localAbs, err := expandPath(localPath)
	if err != nil {
		return "", "", err
	}
	rootAbs, err := expandPath(localRoot)
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(rootAbs, localAbs)
	if err != nil {
		return "", "", fmt.Errorf("infer workspace mapping: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", "", fmt.Errorf("local workspace %s is outside configured local root %s", localPath, localRoot)
	}
	return joinRemotePath(remoteRoot, filepath.ToSlash(relative)), filepath.ToSlash(relative), nil
}

func expandPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Abs(path)
}

func joinRemotePath(root, relative string) string {
	root = strings.TrimRight(root, "/")
	if relative == "" || relative == "." {
		return root
	}
	return root + "/" + relative
}

func normalizeIgnores(values []string) []string {
	seen := map[string]bool{".git": true}
	result := []string{".git"}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == ".git" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
