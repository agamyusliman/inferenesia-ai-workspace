package core

import (
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

type AutonomyView struct {
	Level       string `json:"level"`
	Description string `json:"description"`
}

var autonomyDescriptions = map[string]string{
	config.AutonomyOff:    "Read-only only. Mutating tools are blocked until you raise the level (next chat turn).",
	config.AutonomyLow:    "Read-only + file/doc edits (WriteGateway). Git, terminal, MCP, and shell are blocked.",
	config.AutonomyMedium: "Adds git (commit/push/checkout), terminal control, and MCP tools. Shell and destructive git stay blocked. Git may still ask NeedsApproval for commit/force-push.",
	config.AutonomyHigh:   "All tool risk classes allowed. Separate git NeedsApproval dialogs may still appear for commit/force-push/reset.",
}

func buildAutonomyView(level string) AutonomyView {
	level = config.NormalizeAutonomyLevel(level)
	desc := autonomyDescriptions[level]
	if desc == "" {
		desc = autonomyDescriptions[config.DefaultAutonomy()]
	}
	return AutonomyView{Level: level, Description: desc}
}

func (s *Service) GetAutonomy() AutonomyView {
	home := s.ConfigHome()
	level, err := config.LoadAutonomy(home)
	if err != nil {
		level = config.DefaultAutonomy()
	}
	return buildAutonomyView(level)
}

// SetAutonomy validates and persists the level. Invalid input is rejected
// before any write so a bad request never mutates the tool gate. The returned
// view reflects what was persisted, not what was requested, so Settings cannot
// report a permission change the next chat turn would not honour.
func (s *Service) SetAutonomy(level string) (AutonomyView, error) {
	level = strings.TrimSpace(level)
	if !config.IsValidAutonomyLevel(level) {
		return AutonomyView{}, fmt.Errorf("invalid autonomy level %q (use off, low, medium, or high)", level)
	}
	home := s.ConfigHome()
	cfg, err := config.SaveAutonomy(home, level)
	if err != nil {
		return AutonomyView{}, fmt.Errorf("save autonomy: %w", err)
	}
	return buildAutonomyView(cfg.Autonomy), nil
}

func (s *Service) AutonomyLevel() string {
	home := s.ConfigHome()
	level, err := config.LoadAutonomy(home)
	if err != nil {
		return config.DefaultAutonomy()
	}
	return level
}
