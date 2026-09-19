// Package config loads app configuration (env > ~/.inferenesia/config.yaml > defaults).
//
// Dual-read legacy support:
//   - INFERENESIA_HOME (preferred) → INFERNESIA_HOME (typo brand) → YURA_AI_HOME → default dir
//   - Default dir: ~/.inferenesia; if only ~/.infernesia exists, use it until migrate
//   - New installs and new writes prefer ~/.inferenesia (product home)
//   - Existing ~/.yura-ai alone is NOT auto-selected (set YURA_AI_HOME to keep using it)
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultHomeDirName is the directory under the user home for app config.
	DefaultHomeDirName = ".inferenesia"
	// TypoHomeDirName is the misspelled product home (Infernesia typo era).
	// Still auto-selected when it exists and the canonical dir does not.
	TypoHomeDirName = ".infernesia"
	// LegacyHomeDirName is the pre-rebrand config directory (still read when present).
	LegacyHomeDirName = ".yura-ai"
	// EnvHome is the preferred override for the config home directory.
	EnvHome = "INFERENESIA_HOME"
	// EnvHomeTypo is the misspelled product override (Infernesia typo era).
	EnvHomeTypo = "INFERNESIA_HOME"
	// EnvHomeLegacy is the pre-rebrand override (still honored for existing users).
	EnvHomeLegacy = "YURA_AI_HOME"
	// ConfigFileName is the primary YAML config file name.
	ConfigFileName = "config.yaml"

	// DirPerm is the filesystem mode for the config home directory (owner-only).
	DirPerm = 0o700
	// FilePerm is the filesystem mode for config.yaml (may hold secrets later).
	FilePerm = 0o600
)

// defaultConfigYAML is written on first run when config.yaml is missing.
// Keep free of secrets; comments document env overrides only.
const defaultConfigYAML = `# Inferenesia configuration
# Secrets should use mode 0600. Do not commit this file with API keys.
#
# Hierarchy: env > this file > defaults
# Provider keys via environment (preferred) or api_key in this file:
#   INFERENESIA_BASE_URL, INFERENESIA_API_KEY, INFERENESIA_MODEL (optional)  — Inferenesia API gateway
#   Typo-brand aliases still accepted: INFERNESIA_* ; older: TEMP_AI_*
#   INFERENESIA_BYOK_BASE_URL, INFERENESIA_BYOK_API_KEY, INFERENESIA_BYOK_MODEL — env BYOK profile "byok"
#   Typo-brand / older aliases: INFERNESIA_BYOK_* , YURA_AI_BYOK_*
#   INFERENESIA_HOME (or INFERNESIA_HOME / YURA_AI_HOME) overrides this directory location.
#   INFERENESIA_MCP (or INFERNESIA_MCP / YURA_AI_MCP) overrides MCP servers YAML.
#
# Multi-profile (openai_compatible BYOK + Inferenesia API). BYOK keys are never sent to the Inferenesia Cloud gateway.
#
# providers:
#   - id: inferenesia
#     type: openai_compatible
#     name: Inferenesia API
#     base_url: https://inferenesia.cloud/v1
#     api_key_env: INFERENESIA_API_KEY
#   - id: my-openai
#     type: openai_compatible
#     name: OpenAI BYOK
#     base_url: https://api.openai.com/v1
#     api_key_env: OPENAI_API_KEY
# default_provider: inferenesia
#
# Optional Token Savers (Settings → Token Saver). Defaults all off.
# Missing binary/endpoint shows "Not installed" and never crashes chat.
# token_savers:
#   rtk: { enabled: false, command: rtk }
#   headroom: { enabled: false, base_url: "" }
#   caveman: { enabled: false }
#   ponytail: { enabled: false }
#
# Optional MCP servers (stdio or HTTP). Tools register as mcp.<name>.<tool>.
# Hierarchy: INFERENESIA_MCP (or legacy YURA_AI_MCP) env YAML > project .inferenesia/config.yaml > this file > defaults.
# Generic host only — no vector/RAG server type. Headers/auth never logged.
# Config with auth/headers uses mode 0600. Stop/remove unregisters tools (unknown tool).
# mcp:
#   echo:
#     type: stdio
#     command: /path/to/mcp-server
#     args: ["--stdio"]
#   docs:
#     type: http
#     base_url: http://127.0.0.1:4115/mcp
#     headers:
#       Authorization: Bearer <token>
#     auth:
#       type: bearer
#       token_env: DOCS_TOKEN

version: 1
`

// Home resolves the config home directory with legacy dual-read:
//  1. INFERENESIA_HOME if set (non-empty)
//  2. INFERNESIA_HOME if set (non-empty, typo brand)
//  3. YURA_AI_HOME if set (non-empty, pre-rebrand)
//  4. ~/.inferenesia when it exists
//  5. ~/.infernesia when it exists (typo brand data)
//  6. ~/.inferenesia (product default — created by EnsureHome)
//
// Relative overrides are resolved against the process working directory.
// A pre-existing ~/.yura-ai directory alone no longer wins; set YURA_AI_HOME
// to keep using that path during migration.
func Home() (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvHome)); override != "" {
		return cleanOverride(EnvHome, override)
	}
	if typo := strings.TrimSpace(os.Getenv(EnvHomeTypo)); typo != "" {
		return cleanOverride(EnvHomeTypo, typo)
	}
	if legacy := strings.TrimSpace(os.Getenv(EnvHomeLegacy)); legacy != "" {
		return cleanOverride(EnvHomeLegacy, legacy)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	preferred := filepath.Join(userHome, DefaultHomeDirName)
	if st, err := os.Stat(preferred); err == nil && st.IsDir() {
		return preferred, nil
	}
	typoDir := filepath.Join(userHome, TypoHomeDirName)
	if st, err := os.Stat(typoDir); err == nil && st.IsDir() {
		return typoDir, nil
	}
	return preferred, nil
}

func cleanOverride(envName, override string) (string, error) {
	path := filepath.Clean(override)
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve %s %q: %w", envName, override, err)
		}
		path = abs
	}
	return path, nil
}

// ConfigPath returns the absolute path to config.yaml under the resolved home.
func ConfigPath() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigFileName), nil
}

// EnsureHome creates the config home directory (mode 0700) and a default
// config.yaml (mode 0600) if missing. Returns the absolute home path.
//
// When an env override is set, only that path is touched; the default
// ~/.inferenesia is never created or modified by this function.
func EnsureHome() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return EnsureHomeAt(home)
}

// EnsureHomeAt creates the given config home directory (mode 0700) and a default
// config.yaml (mode 0600) if missing. Used by tests and desktop isolation.
func EnsureHomeAt(home string) (string, error) {
	home = filepath.Clean(home)
	if home == "" || home == "." {
		return "", fmt.Errorf("empty config home")
	}
	if err := ensureDir(home); err != nil {
		return "", err
	}
	if err := ensureDefaultConfig(filepath.Join(home, ConfigFileName)); err != nil {
		return "", err
	}
	return home, nil
}

// MkdirHome creates a directory with DirPerm (0700). Exported for core bootstrap.
func MkdirHome(home string) error {
	return ensureDir(home)
}

func ensureDir(home string) error {
	if err := os.MkdirAll(home, DirPerm); err != nil {
		return fmt.Errorf("create config home %s: %w", home, err)
	}
	// Tighten perms if the directory already existed with a looser mode.
	if err := os.Chmod(home, DirPerm); err != nil {
		return fmt.Errorf("chmod config home %s: %w", home, err)
	}
	return nil
}

func ensureDefaultConfig(cfgPath string) error {
	info, err := os.Stat(cfgPath)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("config path is a directory: %s", cfgPath)
		}
		// Leave existing content and mode intact (idempotent bootstrap).
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat config %s: %w", cfgPath, err)
	}
	if err := os.WriteFile(cfgPath, []byte(defaultConfigYAML), FilePerm); err != nil {
		return fmt.Errorf("write default config: %w", err)
	}
	// Explicit chmod in case the process umask altered the written mode.
	if err := os.Chmod(cfgPath, FilePerm); err != nil {
		return fmt.Errorf("chmod config %s: %w", cfgPath, err)
	}
	return nil
}
