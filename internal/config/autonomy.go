package config

import "strings"

const (
	AutonomyOff    = "off"
	AutonomyLow    = "low"
	AutonomyMedium = "medium"
	AutonomyHigh   = "high"
)

func DefaultAutonomy() string {
	return AutonomyLow
}

func IsValidAutonomyLevel(level string) bool {
	switch strings.TrimSpace(level) {
	case AutonomyOff, AutonomyLow, AutonomyMedium, AutonomyHigh:
		return true
	}
	return false
}

func NormalizeAutonomyLevel(level string) string {
	level = strings.TrimSpace(level)
	if !IsValidAutonomyLevel(level) {
		return DefaultAutonomy()
	}
	return level
}

func LoadAutonomy(home string) (string, error) {
	cfg, err := LoadFromDir(home)
	if err != nil {
		return DefaultAutonomy(), err
	}
	if strings.TrimSpace(cfg.Autonomy) == "" {
		return DefaultAutonomy(), nil
	}
	return NormalizeAutonomyLevel(cfg.Autonomy), nil
}

func SaveAutonomy(home string, level string) (FileConfig, error) {
	level = NormalizeAutonomyLevel(level)
	cfg, err := LoadFromDir(home)
	if err != nil {
		return FileConfig{}, err
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	cfg.Autonomy = level
	if err := SaveToDir(home, cfg); err != nil {
		return FileConfig{}, err
	}
	return cfg, nil
}
