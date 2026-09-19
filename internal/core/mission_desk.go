package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/mission"
	"github.com/agamyusliman/inferenesia-app/internal/secrethyg"
	"github.com/agamyusliman/inferenesia-app/internal/workspace"
)

// ChatEventMissionStatus is emitted when a mission status or gates change (VAL-MISSION-006).
// Desktop mission panel re-fetches / applies without manual refresh.
const ChatEventMissionStatus ChatEventType = "MissionStatus"

// MissionView is the JSON shape for the desktop mission panel.
type MissionView struct {
	ID                 string                `json:"id"`
	WorkspaceID        string                `json:"workspace_id,omitempty"`
	Goal               string                `json:"goal"`
	BudgetTokens       int64                 `json:"budget_tokens,omitempty"`
	TokensUsed         int64                 `json:"tokens_used,omitempty"`
	Status             string                `json:"status"`
	BlockerReason      string                `json:"blocker_reason,omitempty"`
	AcceptanceCriteria []string              `json:"acceptance_criteria,omitempty"`
	Evidence           []mission.Evidence    `json:"evidence"`
	Gates              []mission.GateResult  `json:"gates"`
	// PlanID references the approved Plan Mode proposal this mission was
	// created from (empty for direct-created missions). VAL-CROSS-004.
	PlanID string `json:"plan_id,omitempty"`
	// PlanSnapshot is the approved plan body captured at approve time so the
	// mission always carries the plan that fed it (VAL-CROSS-004).
	PlanSnapshot string `json:"plan_snapshot,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Product     string `json:"product,omitempty"`
}

// MissionListView is the mission panel list payload.
type MissionListView struct {
	Missions []MissionView `json:"missions"`
	Count    int           `json:"count"`
	Product  string        `json:"product,omitempty"`
}

// MissionCreateRequest is the body for create mission (VAL-MISSION-001).
type MissionCreateRequest struct {
	Goal               string   `json:"goal"`
	BudgetTokens       int64    `json:"budget_tokens,omitempty"`
	WorkspaceID        string   `json:"workspace_id,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
}

// MissionEvidenceRequest attaches verification evidence (VAL-MISSION-003).
type MissionEvidenceRequest struct {
	ID       string `json:"id"`
	Kind     string `json:"kind,omitempty"`
	Summary  string `json:"summary"`
	Content  string `json:"content,omitempty"`
	Command  string `json:"command,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
	GateName string `json:"gate_name,omitempty"`
}

// MissionCompleteResult is returned by CompleteMission (success or blocked attempt).
type MissionCompleteResult struct {
	OK      bool        `json:"ok"`
	Mission MissionView `json:"mission"`
	// Message is human-readable: success or "evidence required" denial (VAL-MISSION-002).
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// MissionGateRunRequest runs one or more verification gates (VAL-MISSION-005).
type MissionGateRunRequest struct {
	ID    string   `json:"id"`
	Gates []string `json:"gates,omitempty"` // empty = all default gates
}

// missionState holds the mission store + optional stream emit on Service.
type missionState struct {
	store *mission.Store
	// emit hooks the active chat stream for MissionStatus events.
	emitMu sync.Mutex
	emit   func(ChatEvent)
}

func (s *Service) ensureMissions() *missionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.missions == nil {
		s.missions = &missionState{
			store: mission.NewStore(),
		}
	}
	return s.missions
}

func (ms *missionState) setEmit(emit func(ChatEvent)) {
	if ms == nil {
		return
	}
	ms.emitMu.Lock()
	defer ms.emitMu.Unlock()
	ms.emit = emit
}

func (ms *missionState) emitEvent(ev ChatEvent) {
	if ms == nil {
		return
	}
	ms.emitMu.Lock()
	emit := ms.emit
	ms.emitMu.Unlock()
	if emit == nil {
		return
	}
	defer func() { _ = recover() }()
	emit(ev)
}

func (s *Service) emitMissionStatus(m mission.Mission) {
	ms := s.ensureMissions()
	detail := fmt.Sprintf("status=%s", m.Status)
	if m.BlockerReason != "" {
		detail += " blocked=" + secrethyg.Redact(m.BlockerReason)
	}
	ms.emitEvent(ChatEvent{
		Type:   ChatEventMissionStatus,
		Name:   m.ID,
		Detail: detail,
		Text:   secrethyg.Redact(m.Goal),
		Final:  string(m.Status),
		Path:   m.WorkspaceID,
	})
	s.setStatus(fmt.Sprintf("Mission %s → %s", shortID(m.ID), m.Status))
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

// CreateMission creates a mission with goal and optional budget (VAL-MISSION-001).
func (s *Service) CreateMission(req MissionCreateRequest) (MissionView, error) {
	if s == nil {
		return MissionView{}, fmt.Errorf("core: service is nil")
	}
	wsID := strings.TrimSpace(req.WorkspaceID)
	if wsID == "" {
		if active := s.GetActiveWorkspace(); active != nil {
			wsID = active.ID
		} else {
			// Bind session scratch so missions always have a workspace_id.
			if w, err := s.EnsureSessionWorkspace(); err == nil {
				wsID = w.ID
			}
		}
	}
	ms := s.ensureMissions()
	m, err := ms.store.Create(req.Goal, req.BudgetTokens, wsID, req.AcceptanceCriteria)
	if err != nil {
		return MissionView{}, err
	}
	s.emitMissionStatus(m)
	return missionToView(m, s.BrandName()), nil
}

// ListMissions returns missions for the active workspace.
// No active workspace → empty list (panels clear when project closed).
// Missions with empty workspace_id are only shown for Session scratch.
func (s *Service) ListMissions() MissionListView {
	if s == nil {
		return MissionListView{Missions: []MissionView{}}
	}
	active := s.GetActiveWorkspace()
	if active == nil {
		return MissionListView{Missions: []MissionView{}, Count: 0, Product: s.BrandName()}
	}
	wsID := active.ID
	ms := s.ensureMissions()
	raw := ms.store.ListByWorkspace(wsID)
	if wsID == workspace.ScratchSessionID {
		for _, m := range ms.store.List() {
			if strings.TrimSpace(m.WorkspaceID) != "" {
				continue
			}
			found := false
			for _, x := range raw {
				if x.ID == m.ID {
					found = true
					break
				}
			}
			if !found {
				raw = append(raw, m)
			}
		}
	}
	out := make([]MissionView, 0, len(raw))
	for _, m := range raw {
		out = append(out, missionToView(m, s.BrandName()))
	}
	return MissionListView{
		Missions: out,
		Count:    len(out),
		Product:  s.BrandName(),
	}
}

// CancelMission marks a mission cancelled (terminal).
func (s *Service) CancelMission(id string) (MissionView, error) {
	if s == nil {
		return MissionView{}, fmt.Errorf("core: service is nil")
	}
	m, err := s.ensureMissions().store.Cancel(id)
	if err != nil {
		return MissionView{}, err
	}
	s.emitMissionStatus(m)
	return missionToView(m, s.BrandName()), nil
}

// DeleteMission removes a mission from the in-memory store.
func (s *Service) DeleteMission(id string) error {
	if s == nil {
		return fmt.Errorf("core: service is nil")
	}
	return s.ensureMissions().store.Delete(id)
}

// GetMission returns one mission by id.
func (s *Service) GetMission(id string) (MissionView, error) {
	if s == nil {
		return MissionView{}, fmt.Errorf("core: service is nil")
	}
	m, err := s.ensureMissions().store.Get(id)
	if err != nil {
		return MissionView{}, err
	}
	return missionToView(m, s.BrandName()), nil
}

// ActivateMission sets status active.
func (s *Service) ActivateMission(id string) (MissionView, error) {
	if s == nil {
		return MissionView{}, fmt.Errorf("core: service is nil")
	}
	m, err := s.ensureMissions().store.Activate(id)
	if err != nil {
		return MissionView{}, err
	}
	s.emitMissionStatus(m)
	return missionToView(m, s.BrandName()), nil
}

// BlockMission sets blocked status with reason (VAL-MISSION-004).
func (s *Service) BlockMission(id, reason string) (MissionView, error) {
	if s == nil {
		return MissionView{}, fmt.Errorf("core: service is nil")
	}
	m, err := s.ensureMissions().store.SetBlocked(id, reason)
	if err != nil {
		return MissionView{}, err
	}
	s.emitMissionStatus(m)
	return missionToView(m, s.BrandName()), nil
}

// AttachMissionEvidence adds redacted verification evidence (VAL-MISSION-003/008).
func (s *Service) AttachMissionEvidence(req MissionEvidenceRequest) (MissionView, error) {
	if s == nil {
		return MissionView{}, fmt.Errorf("core: service is nil")
	}
	m, err := s.ensureMissions().store.AttachEvidence(
		req.ID, req.Kind, req.Summary, req.Content, req.Command, req.ExitCode, req.GateName,
	)
	if err != nil {
		return MissionView{}, err
	}
	s.emitMissionStatus(m)
	return missionToView(m, s.BrandName()), nil
}

// CompleteMission marks complete only with evidence and passing gates (VAL-MISSION-002/003/005/007).
// On denial, returns ok=false with message containing "evidence required" (or gate failure).
func (s *Service) CompleteMission(id string) MissionCompleteResult {
	if s == nil {
		return MissionCompleteResult{OK: false, Error: "core: service is nil"}
	}
	m, err := s.ensureMissions().store.Complete(id)
	if err != nil {
		// Still return current mission when possible so UI can show unchanged status.
		view, _ := s.GetMission(id)
		msg := err.Error()
		return MissionCompleteResult{
			OK:      false,
			Mission: view,
			Message: msg,
			Error:   msg,
		}
	}
	s.emitMissionStatus(m)
	return MissionCompleteResult{
		OK:      true,
		Mission: missionToView(m, s.BrandName()),
		Message: "mission completed",
	}
}

// RunMissionGates executes verification gates and records results (VAL-MISSION-005).
// Failing gate blocks completion and sets status blocked.
func (s *Service) RunMissionGates(req MissionGateRunRequest) (MissionView, error) {
	if s == nil {
		return MissionView{}, fmt.Errorf("core: service is nil")
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return MissionView{}, fmt.Errorf("mission: id is required")
	}
	ms := s.ensureMissions()
	// Ensure mission exists.
	if _, err := ms.store.Get(id); err != nil {
		return MissionView{}, err
	}
	if _, err := ms.store.SetVerifying(id); err != nil {
		return MissionView{}, err
	}
	s.emitMissionStatus(mustGet(ms, id))

	workDir := s.workspaceRootForMission(id)
	runner := &mission.GateRunner{WorkDir: workDir, Timeout: 3 * time.Minute}

	names := req.Gates
	if len(names) == 0 {
		for _, g := range mission.DefaultGates {
			names = append(names, g.Name)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var last mission.Mission
	var lastErr error
	for _, name := range names {
		if ctx.Err() != nil {
			break
		}
		gr := runner.RunGate(ctx, name)
		last, lastErr = ms.store.RecordGate(id, gr)
		if lastErr != nil {
			return MissionView{}, lastErr
		}
		// Attach gate output as evidence when passed (helps complete path).
		if gr.Passed {
			code := gr.ExitCode
			_, _ = ms.store.AttachEvidence(
				id, "gate",
				fmt.Sprintf("gate %s passed", gr.Name),
				gr.Output,
				gr.Command,
				&code,
				gr.Name,
			)
			last, _ = ms.store.Get(id)
		}
		s.emitMissionStatus(last)
	}
	if last.ID == "" {
		last, lastErr = ms.store.Get(id)
		if lastErr != nil {
			return MissionView{}, lastErr
		}
	}
	return missionToView(last, s.BrandName()), nil
}

func mustGet(ms *missionState, id string) mission.Mission {
	m, _ := ms.store.Get(id)
	return m
}

func (s *Service) workspaceRootForMission(missionID string) string {
	m, err := s.ensureMissions().store.Get(missionID)
	if err == nil && m.WorkspaceID != "" {
		if root, err2 := s.rootFor(m.WorkspaceID); err2 == nil {
			return root
		}
	}
	if active := s.GetActiveWorkspace(); active != nil {
		return active.RootPath
	}
	return "."
}

// --- MissionBackend for tools ---

// CreateGoal implements tools.MissionBackend.create_goal.
func (s *Service) CreateGoal(objective string, budgetTokens int64) (mission.Mission, error) {
	v, err := s.CreateMission(MissionCreateRequest{
		Goal:         objective,
		BudgetTokens: budgetTokens,
	})
	if err != nil {
		return mission.Mission{}, err
	}
	return s.ensureMissions().store.Get(v.ID)
}

// GetGoal implements tools.MissionBackend.get_goal (latest non-completed, or by id).
func (s *Service) GetGoal(id string) (mission.Mission, error) {
	ms := s.ensureMissions()
	id = strings.TrimSpace(id)
	if id != "" {
		return ms.store.Get(id)
	}
	list := ms.store.List()
	for _, m := range list {
		if m.Status != mission.StatusCompleted {
			return m, nil
		}
	}
	if len(list) == 0 {
		return mission.Mission{}, fmt.Errorf("mission: no missions")
	}
	return list[0], nil
}

// UpdateGoalComplete implements tools.MissionBackend update_goal complete|blocked.
func (s *Service) UpdateGoalComplete(id string) (mission.Mission, error) {
	res := s.CompleteMission(id)
	if !res.OK {
		return mission.Mission{}, fmt.Errorf("%s", res.Error)
	}
	return s.ensureMissions().store.Get(id)
}

// UpdateGoalBlocked implements tools.MissionBackend blocked path.
func (s *Service) UpdateGoalBlocked(id, reason string) (mission.Mission, error) {
	v, err := s.BlockMission(id, reason)
	if err != nil {
		return mission.Mission{}, err
	}
	return s.ensureMissions().store.Get(v.ID)
}

// AttachGoalEvidence implements tools.MissionBackend attach evidence.
func (s *Service) AttachGoalEvidence(id, kind, summary, content, command string, exitCode *int) (mission.Mission, error) {
	v, err := s.AttachMissionEvidence(MissionEvidenceRequest{
		ID: id, Kind: kind, Summary: summary, Content: content, Command: command, ExitCode: exitCode,
	})
	if err != nil {
		return mission.Mission{}, err
	}
	return s.ensureMissions().store.Get(v.ID)
}

func missionToView(m mission.Mission, product string) MissionView {
	v := MissionView{
		ID:                 m.ID,
		WorkspaceID:        m.WorkspaceID,
		Goal:               m.Goal,
		BudgetTokens:       m.BudgetTokens,
		TokensUsed:         m.TokensUsed,
		Status:             string(m.Status),
		BlockerReason:      m.BlockerReason,
		AcceptanceCriteria: m.AcceptanceCriteria,
		Evidence:           m.Evidence,
		Gates:              m.Gates,
		PlanID:             m.PlanID,
		PlanSnapshot:       m.PlanSnapshot,
		Product:            product,
	}
	if v.Evidence == nil {
		v.Evidence = []mission.Evidence{}
	}
	if v.Gates == nil {
		v.Gates = []mission.GateResult{}
	}
	if v.AcceptanceCriteria == nil {
		v.AcceptanceCriteria = []string{}
	}
	if !m.CreatedAt.IsZero() {
		v.CreatedAt = m.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !m.UpdatedAt.IsZero() {
		v.UpdatedAt = m.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if m.CompletedAt != nil {
		v.CompletedAt = m.CompletedAt.UTC().Format(time.RFC3339)
	}
	return v
}

// setStatus is a helper used by several desk packages — defined once if missing.
// (Already used in plan_desk via Service.setStatus.)
