package config

import "strings"

// LiveBlocksConfig holds the opt-in toggle for Live Blocks (P9-live) —
// generative UI previews rendered as sandboxed iframes in chat.
//
// Product name is always Live Blocks (not Flux / enowX*). Default is off.
// Persisted under ~/.inferenesia/config.yaml (mode 0600 via SaveToDir).
type LiveBlocksConfig struct {
	// Enabled turns on sandboxed iframe previews for Live Block fences in chat.
	// Default false. StripBloat still replaces live-block bodies before the
	// model call regardless of this toggle (context discipline is always-on).
	Enabled bool `yaml:"enabled"`
}

// DefaultLiveBlocks returns the product default: Live Blocks off.
func DefaultLiveBlocks() LiveBlocksConfig {
	return LiveBlocksConfig{Enabled: false}
}

// Normalize is a no-op placeholder kept for symmetry with other config sections.
func (c *LiveBlocksConfig) Normalize() {
	if c == nil {
		return
	}
}

// IsLiveBlockLanguage reports whether a fenced code-block language should be
// treated as a Live Block when the feature is enabled. Match is case-insensitive
// and tolerates surrounding whitespace. Recognised languages:
//   - live-block   (canonical)
//   - yura-live    (product alias)
//   - html:preview (legacy)
//
// This helper MUST stay in sync with the regex in internal/core/context_discipline.go
// (liveBlockFenceRe) and web/src/features/chat/liveBlocks.ts (isLiveBlockLanguage).
func IsLiveBlockLanguage(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "live-block", "yura-live", "html:preview":
		return true
	default:
		return false
	}
}

// LoadLiveBlocks reads live_blocks from config.yaml under home (defaults when absent).
// Missing config or missing key returns DefaultLiveBlocks with a nil error so
// first-run never crashes the UI.
func LoadLiveBlocks(home string) (LiveBlocksConfig, error) {
	cfg, err := LoadFromDir(home)
	if err != nil {
		return DefaultLiveBlocks(), err
	}
	out := cfg.LiveBlocks
	out.Normalize()
	return out, nil
}

// SaveLiveBlocks merges live_blocks into config.yaml under home (mode 0600).
func SaveLiveBlocks(home string, lb LiveBlocksConfig) (FileConfig, error) {
	lb.Normalize()
	cfg, err := LoadFromDir(home)
	if err != nil {
		return FileConfig{}, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	cfg.LiveBlocks = lb
	if err := SaveToDir(home, cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}
