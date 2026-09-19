package core

import (
	"fmt"

	"github.com/agamyusliman/inferenesia-app/internal/config"
)

// LiveBlocksSystemFragment is appended to the chat system prompt only when
// Live Blocks is enabled (P9-live). Short, no secrets. Tells the model the
// fence format and the CDN allowlist. Off-by-default → not appended when off.
const LiveBlocksSystemFragment = `[Live Blocks]
When useful, emit interactive HTML widgets as fenced ` + "`live-block" + ` with self-contained HTML.
Prefer CDN: cdn.jsdelivr.net, cdnjs.cloudflare.com, unpkg.com, esm.sh. Keep widgets small.
Never put secrets, API keys, or credentials inside a Live Block.`

const WebPreviewSystemFragment = `[Web Preview]
Inferenesia chat renders fenced HTML with an inline live preview + source code, and a Canvas button opens the full Web Preview panel (device frames, rotate).
When the user asks for a simple UI / landing page / mockup (no npm install, no framework project):
1. Prefer ONE self-contained HTML document (inline CSS/JS). Put the FULL markup in a fenced ` + "`html" + ` block in the reply so chat shows live preview + code immediately.
2. Optionally also write index.html via tools for the session/workspace.
3. Never tell the user to run python -m http.server, open an external browser, or install dependencies just to preview. Do not paste long bash "how to run" blocks.
4. Mention they can click Canvas on the code block (or rail Preview) for mobile/tablet/desktop frames.
For real multi-file projects that need package install / bundlers, use normal project tools — still avoid offloading server startup to the user when a static HTML preview would suffice.`

// LiveBlocksView is the Settings → Live Blocks payload (P9-live).
type LiveBlocksView struct {
	// Product is the brand string (Inferenesia).
	Product string `json:"product"`
	// Enabled is the persisted toggle (default off).
	Enabled bool `json:"enabled"`
	// Note documents default-off + opt-in nature for the UI.
	Note string `json:"note"`
}

// SetLiveBlocksRequest toggles Live Blocks on/off.
type SetLiveBlocksRequest struct {
	Enabled bool `json:"enabled"`
}

// GetLiveBlocks returns the Settings Live Blocks view (default off; never errors).
func (s *Service) GetLiveBlocks() LiveBlocksView {
	home := s.ConfigHome()
	lb, err := config.LoadLiveBlocks(home)
	if err != nil {
		lb = config.DefaultLiveBlocks()
	}
	return buildLiveBlocksView(s.BrandName(), lb)
}

// SetLiveBlocks persists the enabled toggle under ~/.inferenesia/config.yaml (mode 0600).
func (s *Service) SetLiveBlocks(req SetLiveBlocksRequest) (LiveBlocksView, error) {
	home := s.ConfigHome()
	lb := config.LiveBlocksConfig{Enabled: req.Enabled}
	if _, err := config.SaveLiveBlocks(home, lb); err != nil {
		return LiveBlocksView{}, fmt.Errorf("save live blocks: %w", err)
	}
	return buildLiveBlocksView(s.BrandName(), lb), nil
}

// LiveBlocksConfig returns the raw toggle for chat / pipeline hooks.
// Failures return defaults (off) so chat never panics.
func (s *Service) LiveBlocksConfig() config.LiveBlocksConfig {
	home := s.ConfigHome()
	c, err := config.LoadLiveBlocks(home)
	if err != nil {
		return config.DefaultLiveBlocks()
	}
	return c
}

func buildLiveBlocksView(product string, lb config.LiveBlocksConfig) LiveBlocksView {
	return LiveBlocksView{
		Product: product,
		Enabled: lb.Enabled,
		Note:    "Live Blocks (opt-in). Default off. When on, ```live-block / yura-live / html:preview fences render as sandboxed iframe previews. History is still stripped by context discipline; never put secrets in a Live Block.",
	}
}
