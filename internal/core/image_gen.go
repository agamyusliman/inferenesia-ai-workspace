package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/provider"
)

// chatStreamImageGen handles GenerateImage=true turns via the gateway image
// path (POST /responses + image_generation tool, then /images/generations).
// Anthropic and other non-compat profiles degrade with a clear message (VAL-CHAT-025).
// Ephemeral continue does not apply here; image gen is a single turn.
func (s *Service) chatStreamImageGen(ctx context.Context, req ChatRequest, emit func(ChatEvent)) error {
	if emit == nil {
		emit = func(ChatEvent) {}
	}
	prompt := strings.TrimSpace(req.Prompt)
	images := normalizeChatImages(req.Images)
	if prompt == "" {
		err := fmt.Errorf("core: empty image generation prompt")
		emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
		return err
	}

	if req.NoHistory {
		req.Ephemeral = true
	}

	// Playground-scoped turns (canvas Image Studio / Slides) must not bind or
	// write the coding workspace chat: commitTurn persists them under
	// "pg:"+PlaygroundID only. Mirror that contract here — the workspace id on
	// a playground request is only a scope HINT for the canvas, never a target
	// for the coding transcript (docs/canvas-gallery.md scope rules).
	playgroundID := strings.TrimSpace(req.PlaygroundID)
	isPlayground := playgroundID != ""

	turnWorkspaceID := strings.TrimSpace(req.WorkspaceID)
	if !isPlayground {
		if turnWorkspaceID != "" {
			s.switchChatWorkspace(turnWorkspaceID)
		} else if !req.Ephemeral {
			if active := s.registry.Active(); active != nil {
				turnWorkspaceID = active.ID
				s.switchChatWorkspace(turnWorkspaceID)
			}
		}
	}

	s.resolveChatProfileModel(&req)

	router, err := s.providerRouter(req.Profile)
	if err != nil {
		emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
		return err
	}
	client, err := router.GetProfile(req.Profile)
	if err != nil {
		emit(ChatEvent{Type: ChatEventError, Error: err.Error()})
		return err
	}
	requestHost := client.RequestHost()
	profileName := client.Name()

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = client.DefaultModel()
	}
	if oc, ok := client.(*provider.OpenAICompat); ok {
		if resolved, rerr := oc.ResolveModel(ctx, model); rerr == nil && resolved != "" {
			model = resolved
		}
	}

	root, rootErr := s.activeRoot()
	userText := prompt
	if rootErr == nil && len(req.Mentions) > 0 {
		if built, berr := s.buildUserMessage(root, prompt, req.Mentions); berr == nil {
			userText = built
		}
	}

	refs := make([]provider.ImageRef, 0, len(images))
	for _, img := range images {
		refs = append(refs, provider.ImageRef{
			Name:      img.Name,
			MediaType: img.MediaType,
			DataURL:   img.DataURL,
		})
	}
	refs = provider.CapImageRefs(refs)

	imgReq := provider.ImageGenRequest{
		Prompt:          prompt,
		Model:           model,
		N:               1,
		Size:            strings.TrimSpace(req.ImageSize),
		ReasoningEffort: strings.TrimSpace(req.ImageReasoningEffort),
		ImageOnly:       req.ImageOnly,
		Refs:            refs,
	}
	if req.ImageTemperature != nil {
		imgReq.Temperature = *req.ImageTemperature
		imgReq.HasTemperature = true
	}

	started := time.Now()
	finalText, runErr := s.runImageGeneration(ctx, client, imgReq)
	latencyMS := time.Since(started).Milliseconds()

	// Ephemeral (non-continue) turns are never persisted. Playground turns
	// persist to their own "pg:" thread; only coding turns need a workspace.
	persistTranscript := !(req.Ephemeral && !req.Continue) &&
		(isPlayground || turnWorkspaceID != "")

	if runErr != nil {
		msg := runErr.Error()
		if persistTranscript {
			s.appendImageGenTranscript(req, turnWorkspaceID, userText, images, "(error) "+msg, model, profileName)
		}
		emit(ChatEvent{
			Type:      ChatEventError,
			Error:     msg,
			Profile:   profileName,
			Model:     model,
			Host:      requestHost,
			LatencyMS: latencyMS,
		})
		return runErr
	}

	if persistTranscript {
		s.appendImageGenTranscript(req, turnWorkspaceID, userText, images, finalText, model, profileName)
	}
	tokPrompt, tokComp, tokTotal := estimateTokenUsage(
		[]provider.Message{
			{Role: provider.RoleUser, Content: userText},
			{Role: provider.RoleAssistant, Content: finalText},
		},
		finalText,
	)
	// Stream a single final blob as TokenDelta-like content via Done.
	emit(ChatEvent{
		Type:             ChatEventDone,
		Final:            finalText,
		Text:             finalText,
		Profile:          profileName,
		Model:            model,
		Host:             requestHost,
		Tokens:           tokTotal,
		TokensPrompt:     tokPrompt,
		TokensCompletion: tokComp,
		LatencyMS:        latencyMS,
	})
	return nil
}

func (s *Service) runImageGeneration(
	ctx context.Context,
	client provider.Profile,
	req provider.ImageGenRequest,
) (string, error) {
	switch c := client.(type) {
	case *provider.OpenAICompat:
		res, err := c.GenerateImage(ctx, req)
		if err != nil {
			return "", err
		}
		md := provider.FormatGeneratedImagesMarkdown(req.Prompt, res)
		if strings.TrimSpace(md) == "" {
			return "", &provider.ImageGenUnsupportedError{
				Profile: c.Name(),
				Reason:  "empty image result",
			}
		}
		return md, nil
	default:
		// Anthropic and any non-compat adapter: clear degrade (VAL-CHAT-025).
		return "", &provider.ImageGenUnsupportedError{
			Profile: client.Name(),
			Reason:  "adapter has no image generation path (openai_compatible /images/generations only)",
		}
	}
}

// appendImageGenTranscript persists one image-gen turn to the thread that owns
// it. Scope routing mirrors commitTurn (docs/canvas-gallery.md):
//
//   - Playground turn (req.PlaygroundID set): writes ONLY to "pg:<id>". The
//     coding workspace chat and in-memory s.chat are never touched, so a
//     Library/Image Studio generate cannot leak into a session or workspace.
//   - Coding turn (no PlaygroundID): workspace-scoped persist as before.
func (s *Service) appendImageGenTranscript(
	req ChatRequest,
	workspaceID string,
	userText string,
	images []ChatImagePart,
	assistantText string,
	model, profile string,
) {
	playgroundID := strings.TrimSpace(req.PlaygroundID)
	wid := strings.TrimSpace(workspaceID)
	if playgroundID == "" && wid == "" {
		return
	}

	userMsg := provider.Message{Role: provider.RoleUser, Content: userText}
	if len(images) > 0 {
		parts := []provider.ContentPart{{Type: "text", Text: userText}}
		for _, img := range images {
			parts = append(parts, provider.ContentPart{
				Type:      "image_url",
				ImageURL:  img.DataURL,
				MediaType: img.MediaType,
			})
		}
		userMsg.Parts = parts
	}
	assistantMsg := provider.Message{Role: provider.RoleAssistant, Content: assistantText}

	if playgroundID != "" {
		hist, _, _ := s.loadPlaygroundHistory(playgroundID)
		msgs := append([]provider.Message(nil), hist...)
		msgs = append(msgs, userMsg, assistantMsg)
		_ = s.persistPlayground(playgroundID, ChatSession{
			Messages:    msgs,
			Model:       model,
			Profile:     profile,
			WorkspaceID: wid,
		})
		return
	}

	// Load prior history OUTSIDE the service lock — loadProviderHistory
	// takes s.mu itself, which would deadlock if called while we already
	// hold it. Then acquire s.mu once for the swap+snapshot.
	hist, _, _ := s.loadProviderHistory(wid)
	msgs := append([]provider.Message(nil), hist...)
	msgs = append(msgs, userMsg, assistantMsg)

	s.mu.Lock()
	if s.chatWorkspaceID == wid {
		s.chat.Messages = msgs
		s.chat.Model = model
		s.chat.Profile = profile
	}
	snap := ChatSession{
		Messages: append([]provider.Message(nil), msgs...),
		Model:    model,
		Profile:  profile,
	}
	s.mu.Unlock()
	_ = s.persistChat(wid, snap)
}
