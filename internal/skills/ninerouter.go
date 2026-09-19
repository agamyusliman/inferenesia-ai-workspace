package skills

import "strings"

// Optional 9Router skill pack (VAL-KIT-001/002/009).
// Adapted from https://github.com/decolua/9router skills (9router-chat/image/
// embeddings/web-search/web-fetch/stt). Progressive discovery only — bodies and
// tool grants apply after skill load; never hard-require Qdrant/enowx-rag.
//
// Env (tools / HTTP endpoints, never logged):
//   NINEROUTER_URL — base URL (e.g. http://127.0.0.1:20128)
//   NINEROUTER_KEY — optional bearer when 9Router auth is enabled
//
// Chat: point an openai_compatible provider profile base_url at
// $NINEROUTER_URL/v1 (or rely on Bootstrap profile "ninerouter") — no second LLM loop.

// Skill name constants for the 9Router pack.
const (
	NameNineRouterChat       = "9router-chat"
	NameNineRouterImage      = "9router-image"
	NameNineRouterEmbeddings = "9router-embeddings"
	NameNineRouterWebSearch  = "9router-web-search"
	NameNineRouterWebFetch   = "9router-web-fetch"
	NameNineRouterSTT        = "9router-stt"
)

// Tool names granted only while the matching 9Router skill is loaded.
// Implementations live under internal/tools (HTTP when NINEROUTER_* set).
const (
	ToolNineRouterImage      = "ninerouter_image"
	ToolNineRouterEmbeddings = "ninerouter_embeddings"
	ToolNineRouterWebSearch  = "ninerouter_web_search"
	ToolNineRouterWebFetch   = "ninerouter_web_fetch"
	ToolNineRouterSTT        = "ninerouter_stt"
)

// nineRouterChatBody documents chat-via-provider-profile (no parallel client).
const nineRouterChatBody = `# 9Router — Chat (optional)

Use the **existing** openai_compatible ModelRouter path. Do **not** spin up a second LLM client in TS or Go.

## Env

| Variable | Role |
|----------|------|
| NINEROUTER_URL | 9Router base (e.g. http://127.0.0.1:20128) — no trailing path required |
| NINEROUTER_KEY | Optional API key / bearer when auth is enabled |

Never log or commit NINEROUTER_KEY.

## Provider profile (VAL-KIT-009)

Register or select an openai_compatible profile with:

- base_url = $NINEROUTER_URL/v1 (append /v1 if the base has no version segment)
- api_key_env = NINEROUTER_KEY (or inline local key mode 0600)
- type = openai_compatible

Bootstrap also auto-registers profile id **ninerouter** when NINEROUTER_URL is set.

CLI examples:

` + "```" + `
export NINEROUTER_URL=http://127.0.0.1:20128
export NINEROUTER_KEY=your-key   # optional
inferenesia chat --profile ninerouter "Reply with PONG"
inferenesia models --profile ninerouter
` + "```" + `

YAML (~/.inferenesia/config.yaml, mode 0600 when keys present):

` + "```" + `yaml
providers:
  - id: ninerouter
    type: openai_compatible
    name: 9Router
    base_url: http://127.0.0.1:20128/v1
    api_key_env: NINEROUTER_KEY
default_provider: ninerouter
` + "```" + `

## Endpoints (via profile host)

- POST /v1/chat/completions — OpenAI-compatible stream + tools
- GET /v1/models — model list (no hard-coded model required)

Missing NINEROUTER_URL: configure the profile or env before selecting ninerouter.
`

const nineRouterImageBody = `# 9Router — Image generation (optional)

Requires skill load + NINEROUTER_URL (NINEROUTER_KEY if auth). Endpoint shape:
POST $NINEROUTER_URL/v1/images/generations

Granted tool while loaded: ninerouter_image

| Field | Notes |
|-------|-------|
| model | from /v1/models/image when available |
| prompt | required description |
| n, size, quality, response_format | optional |

Missing env → clear blocked error (no crash). Does not require Qdrant.
`

const nineRouterEmbeddingsBody = `# 9Router — Embeddings (optional)

Pure embed call via POST $NINEROUTER_URL/v1/embeddings.

Granted tool: ninerouter_embeddings

**Does not** require Qdrant, Voyage, or enowx-rag. This skill only produces vectors;
storage/index is out of scope.

Missing NINEROUTER_URL → blocked cleanly.
`

const nineRouterWebSearchBody = `# 9Router — Web search (optional)

POST $NINEROUTER_URL/v1/search

Granted tool: ninerouter_web_search

Expect structured results (title / url / snippet) or a clear error.
`

const nineRouterWebFetchBody = `# 9Router — Web fetch (optional)

POST $NINEROUTER_URL/v1/web/fetch

Granted tool: ninerouter_web_fetch

Returns markdown/text/html for a URL; honor format when provided.
`

const nineRouterSTTBody = `# 9Router — Speech-to-text (optional)

POST $NINEROUTER_URL/v1/audio/transcriptions (multipart)

Granted tool: ninerouter_stt

Accepts an audio file path under the workspace (or absolute readable path).
Missing env → clear blocked.
`

// NineRouterPackMetas returns discovery-only metadata for the optional pack
// (VAL-KIT-001: progressive disclosure — no bodies).
func NineRouterPackMetas() []Meta {
	return []Meta{
		{
			Name:          NameNineRouterChat,
			Description:   "Optional 9Router chat: use openai_compatible profile at NINEROUTER_URL/v1 (no second LLM loop)",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
		{
			Name:          NameNineRouterImage,
			Description:   "Optional 9Router image gen via /v1/images/generations (requires NINEROUTER_URL)",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
		{
			Name:          NameNineRouterEmbeddings,
			Description:   "Optional 9Router embeddings via /v1/embeddings (no Qdrant/enowx-rag required)",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
		{
			Name:          NameNineRouterWebSearch,
			Description:   "Optional 9Router web search via /v1/search (requires NINEROUTER_URL)",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
		{
			Name:          NameNineRouterWebFetch,
			Description:   "Optional 9Router web fetch via /v1/web/fetch (markdown/text/html)",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
		{
			Name:          NameNineRouterSTT,
			Description:   "Optional 9Router speech-to-text via /v1/audio/transcriptions",
			Source:        SourceBuiltin,
			UserInvocable: true,
		},
	}
}

// loadNineRouterBuiltin returns a fully loaded pack skill, or false.
func loadNineRouterBuiltin(name string) (*Skill, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case NameNineRouterChat:
		return &Skill{
			Meta: Meta{
				Name:          NameNineRouterChat,
				Description:   "Optional 9Router chat: use openai_compatible profile at NINEROUTER_URL/v1 (no second LLM loop)",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			// Chat is provider-profile only; no extra agent tool required.
			Body:         nineRouterChatBody,
			AllowedTools: nil,
		}, true
	case NameNineRouterImage:
		return &Skill{
			Meta: Meta{
				Name:          NameNineRouterImage,
				Description:   "Optional 9Router image gen via /v1/images/generations (requires NINEROUTER_URL)",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			Body:         nineRouterImageBody,
			AllowedTools: []string{ToolNineRouterImage},
		}, true
	case NameNineRouterEmbeddings:
		return &Skill{
			Meta: Meta{
				Name:          NameNineRouterEmbeddings,
				Description:   "Optional 9Router embeddings via /v1/embeddings (no Qdrant/enowx-rag required)",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			Body:         nineRouterEmbeddingsBody,
			AllowedTools: []string{ToolNineRouterEmbeddings},
		}, true
	case NameNineRouterWebSearch:
		return &Skill{
			Meta: Meta{
				Name:          NameNineRouterWebSearch,
				Description:   "Optional 9Router web search via /v1/search (requires NINEROUTER_URL)",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			Body:         nineRouterWebSearchBody,
			AllowedTools: []string{ToolNineRouterWebSearch},
		}, true
	case NameNineRouterWebFetch:
		return &Skill{
			Meta: Meta{
				Name:          NameNineRouterWebFetch,
				Description:   "Optional 9Router web fetch via /v1/web/fetch (markdown/text/html)",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			Body:         nineRouterWebFetchBody,
			AllowedTools: []string{ToolNineRouterWebFetch},
		}, true
	case NameNineRouterSTT:
		return &Skill{
			Meta: Meta{
				Name:          NameNineRouterSTT,
				Description:   "Optional 9Router speech-to-text via /v1/audio/transcriptions",
				Source:        SourceBuiltin,
				UserInvocable: true,
			},
			Body:         nineRouterSTTBody,
			AllowedTools: []string{ToolNineRouterSTT},
		}, true
	default:
		return nil, false
	}
}

// IsNineRouterSkill reports whether name is part of the optional 9Router pack.
func IsNineRouterSkill(name string) bool {
	_, ok := loadNineRouterBuiltin(name)
	return ok
}

// NineRouterPackNames lists pack skill ids (stable order).
func NineRouterPackNames() []string {
	return []string{
		NameNineRouterChat,
		NameNineRouterImage,
		NameNineRouterEmbeddings,
		NameNineRouterWebSearch,
		NameNineRouterWebFetch,
		NameNineRouterSTT,
	}
}
