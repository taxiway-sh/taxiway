// Package config holds shared configuration: env-var resolution, lab naming,
// orchestrator-name validation, and state-dir paths.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultTaxiwayDir       = ".taxiway"
	DefaultAuthDir          = "auth"
	DefaultObservabilityDir = "observability"
	DefaultProxyDir         = "proxy"
	DefaultStateDir         = "lab-state"
	DefaultRuntime          = "runtime"
	DefaultPrefix           = "taxiway-"
	DefaultDriver           = "lima"
	MaxLabNameLength        = 48
)

var orchNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var labHostSlugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// reservedLabNames are words that Cobra treats as built-in sub-commands or
// flags when passed as positional arguments (e.g. "taxiway up help").
var reservedLabNames = map[string]bool{
	"help":    true,
	"version": true,
}

// ValidateOrchName returns an error if name is empty or contains invalid chars.
func ValidateOrchName(name string) error {
	if name == "" {
		return fmt.Errorf("orchestrator name is required")
	}
	if !orchNameRe.MatchString(name) {
		return fmt.Errorf("invalid orchestrator name %q — must match ^[a-zA-Z0-9_-]+$", name)
	}
	return nil
}

// ValidateLabName returns an error if name is empty, contains invalid chars,
// or is a reserved word ("help", "version").
func ValidateLabName(name string) error {
	if name == "" {
		return fmt.Errorf("lab name is required")
	}
	if len(name) > MaxLabNameLength {
		return fmt.Errorf("invalid lab name: must be %d characters or fewer", MaxLabNameLength)
	}
	if !orchNameRe.MatchString(name) {
		return fmt.Errorf("invalid lab name %q — must match ^[a-zA-Z0-9_-]+$", name)
	}
	if reservedLabNames[name] {
		return fmt.Errorf("invalid lab name %q — reserved word", name)
	}
	return nil
}

// IDOf returns the host driver-facing lab identifier: "taxiway-<lab>".
func IDOf(lab string) string {
	return DefaultPrefix + lab
}

// RuntimeIDOf returns the driver-facing lab identifier for the current Taxiway
// runtime context. Host installs keep the historical "taxiway-<lab>" shape;
// dev/e2e contexts add the context and context id so runtime resources remain
// isolated across worktrees and test runs.
func RuntimeIDOf(lab string) string {
	base := strings.TrimPrefix(lab, DefaultPrefix)
	context := strings.TrimSpace(os.Getenv("TAXIWAY_CONTEXT"))
	contextID := strings.TrimSpace(os.Getenv("TAXIWAY_CONTEXT_ID"))
	if context == "" || context == "host" || contextID == "" {
		return IDOf(base)
	}
	contextPrefix := context + "-" + contextID + "-"
	if strings.HasPrefix(base, contextPrefix) {
		return DefaultPrefix + base
	}
	return DefaultPrefix + contextPrefix + base
}

// LabNameFromID extracts the lab name from a driver-facing lab identifier.
func LabNameFromID(id string) (string, error) {
	if !strings.HasPrefix(id, DefaultPrefix) {
		return "", fmt.Errorf("invalid lab id %q: expected prefix %q", id, DefaultPrefix)
	}
	return LabDirOf(id), nil
}

// LabDirOf returns the state-directory key for a lab identifier.
// Strips DefaultPrefix when present, so both "taxiway-gastown" and "gastown" return "gastown".
// Use this wherever a path under stateDir is constructed — never NameOf.
func LabDirOf(id string) string {
	lab := strings.TrimPrefix(id, DefaultPrefix)
	context := strings.TrimSpace(os.Getenv("TAXIWAY_CONTEXT"))
	contextID := strings.TrimSpace(os.Getenv("TAXIWAY_CONTEXT_ID"))
	if context == "" || context == "host" || contextID == "" {
		return lab
	}
	return strings.TrimPrefix(lab, context+"-"+contextID+"-")
}

// LabLiteLLMHost returns the lab-specific hostname used by the gateway proxy.
func LabLiteLLMHost(lab string) string {
	lower := strings.ToLower(lab)
	slug := strings.Trim(labHostSlugRe.ReplaceAllString(lower, "-"), "-")
	if slug == "" {
		slug = "lab"
	}
	return slug + ".litellm.localhost"
}

// CreatedAtPath returns the lab creation timestamp path for a lab identifier.
func CreatedAtPath(stateDir, id string) string {
	return filepath.Join(stateDir, LabDirOf(id), "created_at")
}

// EnsureCreatedAt writes created_at if it does not exist yet.
func EnsureCreatedAt(stateDir, id string) error {
	path := CreatedAtPath(stateDir, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n")
	return err
}

// ReadCreatedAt reads created_at for a lab identifier when present.
func ReadCreatedAt(stateDir, id string) (time.Time, bool) {
	raw, err := os.ReadFile(CreatedAtPath(stateDir, id))
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// LabRef identifies a lab and its orchestrator type.
type LabRef struct {
	Lab                 string               // lab name (e.g. "mon-lab")
	Orch                string               // orchestrator type (e.g. "claude-code")
	Driver              string               // driver used to create the lab (e.g. "docker", "lima", "mock")
	Workspace           *Workspace           // optional workspace config (nil = no repo configured)
	OrchestratorProfile *OrchestratorProfile // optional orchestrator profile (nil = use orchestrator defaults)
	Settings            map[string]string    // optional orchestrator-scoped --set values
}

// OrchestratorProfile identifies an optional named profile selected for an orchestrator.
type OrchestratorProfile struct {
	Name string `json:"name"`
}

// ID returns the driver-facing lab identifier for this LabRef.
func (r LabRef) ID() string {
	return IDOf(r.Lab)
}

// StateDir resolves the lab state directory. Precedence:
// 1. override argument (non-empty)
// 2. TAXIWAY_LAB_STATE_DIR env var
// 3. ~/.taxiway/lab-state
func StateDir(override, repoDir string) string {
	if override != "" {
		return override
	}
	if v := os.Getenv("TAXIWAY_LAB_STATE_DIR"); v != "" {
		return v
	}
	return filepath.Join(userTaxiwayDir(), DefaultStateDir)
}

// RuntimeDir resolves the directory containing taxiway runtime assets.
// Precedence:
// 1. override argument (non-empty)
// 2. TAXIWAY_RUNTIME_DIR env var
// 3. ~/.taxiway/runtime
func RuntimeDir(override, _ string) string {
	if override != "" {
		return override
	}
	if v := os.Getenv("TAXIWAY_RUNTIME_DIR"); v != "" {
		return v
	}
	return filepath.Join(userTaxiwayDir(), DefaultRuntime)
}

// ObservabilityDir resolves the user-level mutable observability directory.
func ObservabilityDir() string {
	return filepath.Join(userTaxiwayDir(), DefaultObservabilityDir)
}

// AuthDir resolves the user-level mutable auth directory.
func AuthDir() string {
	return filepath.Join(userTaxiwayDir(), DefaultAuthDir)
}

// ProxyDir resolves the user-level mutable proxy directory.
func ProxyDir() string {
	return filepath.Join(userTaxiwayDir(), DefaultProxyDir)
}

func userTaxiwayDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return DefaultTaxiwayDir
	}
	return filepath.Join(home, DefaultTaxiwayDir)
}

// InstallScript returns the path to orchestrators/<orch>/install.sh and
// verifies it exists.
func InstallScript(repoDir, orch string) (string, error) {
	p := filepath.Join(repoDir, "orchestrators", orch, "install.sh")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("unknown orchestrator %q — no orchestrators/%s/install.sh found", orch, orch)
	}
	return p, nil
}

// VerifyScript returns the path to orchestrators/<orch>/verify.sh and verifies it exists.
func VerifyScript(repoDir, orch string) (string, error) {
	p := filepath.Join(repoDir, "orchestrators", orch, "verify.sh")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("no verify.sh for orchestrator %q — expected orchestrators/%s/verify.sh", orch, orch)
	}
	return p, nil
}

// StartScript returns the path to orchestrators/<orch>/start.sh and verifies it exists.
func StartScript(repoDir, orch string) (string, error) {
	p := filepath.Join(repoDir, "orchestrators", orch, "start.sh")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("no start.sh for orchestrator %q — expected orchestrators/%s/start.sh", orch, orch)
	}
	return p, nil
}

// BootstrapScript returns the path to infra/commands/bootstrap.sh.
func BootstrapScript(repoDir string) string {
	return filepath.Join(repoDir, "infra", "commands", "bootstrap.sh")
}

// DoctorScript returns the path to infra/commands/doctor.sh.
func DoctorScript(repoDir string) string {
	return filepath.Join(repoDir, "infra", "commands", "doctor.sh")
}

// ResetScript returns the path to infra/commands/reset.sh.
func ResetScript(repoDir string) string {
	return filepath.Join(repoDir, "infra", "commands", "reset.sh")
}

// OrchManifest represents the optional orchestrators/<orch>/manifest.yaml.
// It intentionally excludes a 'secrets' field. Agent authentication is declared
// in agent manifests and runtime gateway environment is generated by Taxiway.
type OrchManifest struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	DocsURL     string         `yaml:"docs_url"`
	Agents      []string       `yaml:"agents,omitempty"`
	Shell       *ShellManifest `yaml:"shell,omitempty"`
	Settings    []OrchSetting  `yaml:"settings,omitempty"`
}

// AgentManifest represents agents/<agent>/manifest.yaml.
type AgentManifest struct {
	Name    string                `yaml:"name"`
	Command string                `yaml:"command"`
	Auth    *AgentAuthManifest    `yaml:"auth,omitempty"`
	LiteLLM *AgentLiteLLMManifest `yaml:"litellm,omitempty"`
}

// AgentLiteLLMManifest declares the LiteLLM providers an agent can consume.
type AgentLiteLLMManifest struct {
	Providers []string `yaml:"providers,omitempty"`
}

// AgentAuthManifest declares how an agent authenticates before start.
type AgentAuthManifest struct {
	DefaultMode string                   `yaml:"default_mode"`
	Modes       map[string]AgentAuthMode `yaml:"modes,omitempty"`
}

// AgentAuthMode describes one authentication path for an agent.
type AgentAuthMode struct {
	Scope          string                   `yaml:"scope"`
	Description    string                   `yaml:"description,omitempty"`
	CredentialFile *AgentAuthCredentialFile `yaml:"credential_file,omitempty"`
}

// AgentAuthCredentialFile describes an auth credential file owned by one auth mode.
type AgentAuthCredentialFile struct {
	HostPath string `yaml:"host_path"`
	LabPath  string `yaml:"lab_path"`
	Mode     string `yaml:"mode,omitempty"`
}

// OrchSetting documents one orchestrator-scoped --set key.
type OrchSetting struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Default     string   `yaml:"default,omitempty"`
	Examples    []string `yaml:"examples,omitempty"`
	Phases      []string `yaml:"phases,omitempty"`
}

// ShellManifest defines a custom shell command for `taxiway shell <lab>`.
// When present in manifest.yaml, taxiway shell runs Command instead of
// the default `tmux attach-session`.
type ShellManifest struct {
	// Command is the argv to execute in the lab (e.g. ["gt", "mayor", "attach"]).
	// Must be non-empty when shell: is present.
	Command []string `yaml:"command"`
}

// LoadOrchManifest parses orchestrators/<orch>/manifest.yaml if it exists.
// Returns (nil, nil) when the file is absent — that is valid (manifest is optional).
// Returns an error if the file exists but is malformed, or if it contains a
// 'secrets:' field.
func LoadOrchManifest(repoDir, orch string) (*OrchManifest, error) {
	path := filepath.Join(repoDir, "orchestrators", orch, "manifest.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil // manifest is optional
	}
	if err != nil {
		return nil, fmt.Errorf("config: cannot read %s: %w", path, err)
	}

	// Lint pass: reject any manifest that declares a 'secrets:' field.
	// Decode into a generic map first to catch unknown/forbidden keys.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: cannot parse %s: %w", path, err)
	}
	if _, hasSecrets := raw["secrets"]; hasSecrets {
		return nil, fmt.Errorf(
			"config: orchestrators/%s/manifest.yaml must not contain a 'secrets:' field — "+
				"orchestrator secrets are not supported",
			orch,
		)
	}

	var m OrchManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("config: cannot parse %s: %w", path, err)
	}

	// Validate shell.command: if shell: is present it must have a non-empty command.
	if m.Shell != nil && len(m.Shell.Command) == 0 {
		return nil, fmt.Errorf(
			"config: orchestrators/%s/manifest.yaml: shell.command must be non-empty when shell: is present",
			orch,
		)
	}
	for _, agent := range m.Agents {
		if err := ValidateOrchName(agent); err != nil {
			return nil, fmt.Errorf(
				"config: orchestrators/%s/manifest.yaml: invalid agent %q: %w",
				orch, agent, err,
			)
		}
	}

	return &m, nil
}

// LoadAgentManifest parses agents/<agent>/manifest.yaml if it exists.
// Returns (nil, nil) when the file is absent.
func LoadAgentManifest(repoDir, agent string) (*AgentManifest, error) {
	path := filepath.Join(repoDir, "agents", agent, "manifest.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: cannot read %s: %w", path, err)
	}

	var m AgentManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("config: cannot parse %s: %w", path, err)
	}
	if m.Name != "" {
		if err := ValidateOrchName(m.Name); err != nil {
			return nil, fmt.Errorf("config: agents/%s/manifest.yaml: invalid name %q: %w", agent, m.Name, err)
		}
	}
	if m.Auth != nil {
		if m.Auth.DefaultMode == "" {
			return nil, fmt.Errorf("config: agents/%s/manifest.yaml: auth.default_mode is required when auth is present", agent)
		}
		if len(m.Auth.Modes) == 0 {
			return nil, fmt.Errorf("config: agents/%s/manifest.yaml: auth.modes is required when auth is present", agent)
		}
		if _, ok := m.Auth.Modes[m.Auth.DefaultMode]; !ok {
			return nil, fmt.Errorf("config: agents/%s/manifest.yaml: auth.default_mode %q is not declared in auth.modes", agent, m.Auth.DefaultMode)
		}
		for mode, cfg := range m.Auth.Modes {
			if err := ValidateOrchName(mode); err != nil {
				return nil, fmt.Errorf("config: agents/%s/manifest.yaml: invalid auth mode %q: %w", agent, mode, err)
			}
			if cfg.Scope == "" {
				return nil, fmt.Errorf("config: agents/%s/manifest.yaml: auth mode %q must declare scope", agent, mode)
			}
			if cfg.CredentialFile != nil {
				if cfg.Scope != "lab" {
					return nil, fmt.Errorf("config: agents/%s/manifest.yaml: auth mode %q credential_file requires scope=lab", agent, mode)
				}
				if cfg.CredentialFile.HostPath == "" {
					return nil, fmt.Errorf("config: agents/%s/manifest.yaml: auth mode %q credential_file.host_path is required", agent, mode)
				}
				if cfg.CredentialFile.LabPath == "" {
					return nil, fmt.Errorf("config: agents/%s/manifest.yaml: auth mode %q credential_file.lab_path is required", agent, mode)
				}
			}
		}
	}
	if m.LiteLLM != nil {
		for _, provider := range m.LiteLLM.Providers {
			if err := ValidateOrchName(provider); err != nil {
				return nil, fmt.Errorf("config: agents/%s/manifest.yaml: invalid litellm provider %q: %w", agent, provider, err)
			}
		}
	}

	return &m, nil
}
