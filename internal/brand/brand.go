// Package brand holds user-facing product identity strings for Inferenesia.
//
// Product brand is always "Inferenesia" (from inference / inferensi — AI inference).
// The misspelling "Infernesia" / "infernesia" is a legacy typo brand: dual-read
// only, never marketed. The string "temp-ai" is a deprecated gateway/provider
// alias label only — never the product name ("temp-ai Agent" is forbidden).
// The primary user-facing gateway label is "Inferenesia API".
//
// Legacy dual-read: profile ids "infernesia" and "tempai" still resolve to the
// canonical "inferenesia" profile; TEMP_AI_* / YURA_AI_* / INFERNESIA_* env
// vars still work alongside INFERENESIA_*. See docs/brand/rebrand-plan.md.
package brand

const (
	// Name is the product display name (VAL-FOUND-005).
	Name = "Inferenesia"
	// Binary is the CLI executable name.
	Binary = "inferenesia"
	// DesktopBinary is the desktop hub executable name.
	DesktopBinary = "inferenesia-desktop"
	// ShortDescription is used in help and banners.
	ShortDescription = "AI workspace (desktop + CLI) — Inferenesia API and/or BYOK."
	// ProviderGateway is the primary user-facing gateway/provider label.
	ProviderGateway = "Inferenesia API"
	// ProviderTempAI is the deprecated gateway provider label alias (still
	// accepted for legacy config/profile lookups; never the product brand).
	// Kept as a constant so tests and compat code can reference it explicitly.
	ProviderTempAI = "temp-ai"
	// StartupBanner is a one-line identity string for CLI session banners.
	StartupBanner = Name + " — AI workspace (CLI)"
	// DomainPublic is the public product domain (planned; DNS cutover later).
	DomainPublic = "inferenesia.id"
)
