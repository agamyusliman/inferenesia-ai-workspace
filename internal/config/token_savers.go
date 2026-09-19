package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TokenSaverIDs are the four optional token savers (Settings → Token Saver).
const (
	TokenSaverRTK      = "rtk"
	TokenSaverHeadroom = "headroom"
	TokenSaverCaveman  = "caveman"
	TokenSaverPonytail = "ponytail"
)

// TokenSaversConfig holds independent on/off toggles for optional token savers.
// Defaults are all off. Persisted under ~/.inferenesia/config.yaml (VAL-SAVER-001/002).
type TokenSaversConfig struct {
	RTK      TokenSaverRTKConfig      `yaml:"rtk"`
	Headroom TokenSaverHeadroomConfig `yaml:"headroom"`
	Caveman  TokenSaverFlagConfig     `yaml:"caveman"`
	Ponytail TokenSaverFlagConfig     `yaml:"ponytail"`
}

// TokenSaverRTKConfig compresses tool outputs (git/grep/ls/tree/logs) when available.
type TokenSaverRTKConfig struct {
	Enabled bool   `yaml:"enabled"`
	Command string `yaml:"command,omitempty"` // default "rtk" or absolute path
}

// TokenSaverHeadroomConfig compresses prompts via a configured base_url (e.g. /v1/compress).
type TokenSaverHeadroomConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url,omitempty"`
}

// TokenSaverFlagConfig is a simple enable flag (Caveman / Ponytail prompt bias).
type TokenSaverFlagConfig struct {
	Enabled bool `yaml:"enabled"`
}

// DefaultTokenSavers returns all savers disabled (product default).
func DefaultTokenSavers() TokenSaversConfig {
	return TokenSaversConfig{
		RTK:      TokenSaverRTKConfig{Enabled: false, Command: "rtk"},
		Headroom: TokenSaverHeadroomConfig{Enabled: false},
		Caveman:  TokenSaverFlagConfig{Enabled: false},
		Ponytail: TokenSaverFlagConfig{Enabled: false},
	}
}

// Normalize fills empty RTK command with the default binary name.
func (c *TokenSaversConfig) Normalize() {
	if c == nil {
		return
	}
	if strings.TrimSpace(c.RTK.Command) == "" {
		c.RTK.Command = "rtk"
	}
	c.Headroom.BaseURL = strings.TrimSpace(c.Headroom.BaseURL)
}

// IsEnabled reports whether the named saver is toggled on (ignores install status).
func (c TokenSaversConfig) IsEnabled(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case TokenSaverRTK:
		return c.RTK.Enabled
	case TokenSaverHeadroom:
		return c.Headroom.Enabled
	case TokenSaverCaveman:
		return c.Caveman.Enabled
	case TokenSaverPonytail:
		return c.Ponytail.Enabled
	default:
		return false
	}
}

// SetEnabled flips one saver toggle and returns the updated config.
// Unknown ids return an error. Enabling a missing install is allowed (no crash).
func (c TokenSaversConfig) SetEnabled(id string, enabled bool) (TokenSaversConfig, error) {
	out := c
	out.Normalize()
	switch strings.ToLower(strings.TrimSpace(id)) {
	case TokenSaverRTK:
		out.RTK.Enabled = enabled
	case TokenSaverHeadroom:
		out.Headroom.Enabled = enabled
	case TokenSaverCaveman:
		out.Caveman.Enabled = enabled
	case TokenSaverPonytail:
		out.Ponytail.Enabled = enabled
	default:
		return c, fmt.Errorf("unknown token saver %q (use rtk, headroom, caveman, ponytail)", id)
	}
	return out, nil
}

// LoadTokenSavers reads token_savers from config.yaml under home (defaults when absent).
func LoadTokenSavers(home string) (TokenSaversConfig, error) {
	cfg, err := LoadFromDir(home)
	if err != nil {
		return DefaultTokenSavers(), err
	}
	out := cfg.TokenSavers
	// Detect zero-value vs explicit load: FileConfig always unmarshals; empty means defaults.
	if !cfg.hasTokenSaversYAML {
		out = DefaultTokenSavers()
	}
	out.Normalize()
	return out, nil
}

// SaveTokenSavers merges token_savers into config.yaml under home (mode 0600).
func SaveTokenSavers(home string, savers TokenSaversConfig) (FileConfig, error) {
	savers.Normalize()
	cfg, err := LoadFromDir(home)
	if err != nil {
		return FileConfig{}, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	cfg.TokenSavers = savers
	cfg.hasTokenSaversYAML = true
	if err := SaveToDir(home, cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}

// RTKCommand returns the configured rtk binary name/path (default "rtk").
func (c TokenSaversConfig) RTKCommand() string {
	cmd := strings.TrimSpace(c.RTK.Command)
	if cmd == "" {
		return "rtk"
	}
	return cmd
}

// ProbeRTKAvailable returns true when the RTK binary is on PATH or at an absolute path.
// Never logs secrets.
func ProbeRTKAvailable(command string) (available bool, detail string) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		cmd = "rtk"
	}
	// Absolute or relative path with a separator: check existence + executable.
	if strings.Contains(cmd, string(os.PathSeparator)) || strings.HasPrefix(cmd, ".") {
		info, err := os.Stat(cmd)
		if err != nil || info.IsDir() {
			return false, "binary not found"
		}
		// Best-effort execute bit check (Unix); Windows relies on extension.
		if info.Mode()&0o111 == 0 {
			// Still treat regular files as potentially runnable (Windows, etc.).
			if !info.Mode().IsRegular() {
				return false, "not an executable file"
			}
		}
		abs, _ := filepath.Abs(cmd)
		if abs == "" {
			abs = cmd
		}
		return true, abs
	}
	path, err := exec.LookPath(cmd)
	if err != nil {
		return false, "not installed"
	}
	return true, path
}

// ProbeHeadroomAvailable is true when a base_url is configured (endpoint target present).
// Actual health ping is deferred to the pipeline worker; missing URL → Not installed.
func ProbeHeadroomAvailable(baseURL string) (available bool, detail string) {
	u := strings.TrimSpace(baseURL)
	if u == "" {
		return false, "base_url not configured"
	}
	return true, MaskBaseURL(u)
}
