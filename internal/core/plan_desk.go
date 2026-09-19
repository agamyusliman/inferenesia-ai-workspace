package core

import (
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/plan"
)

// PlanModeState is the JSON shape for desktop Plan Mode UI (VAL-PLAN-001).
type PlanModeState struct {
	Active  bool   `json:"active"`
	Mode    string `json:"mode"`
	Reason  string `json:"reason,omitempty"`
	Product string `json:"product,omitempty"`
}

// ensurePlanMode lazy-inits the Plan Mode controller on Service.
func (s *Service) ensurePlanMode() *plan.State {
	if s == nil {
		return plan.New()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.planMode == nil {
		s.planMode = plan.New()
	}
	return s.planMode
}

// GetPlanMode returns the current Plan Mode snapshot for UI indicator.
func (s *Service) GetPlanMode() PlanModeState {
	st := s.ensurePlanMode().Snapshot()
	return PlanModeState{
		Active:  st.Active,
		Mode:    st.Mode,
		Reason:  st.Reason,
		Product: s.BrandName(),
	}
}

// EnterPlanMode activates read-only Plan Mode (VAL-PLAN-001).
// Agent tool writes are denied until ExitPlanMode.
func (s *Service) EnterPlanMode(reason string) (PlanModeState, error) {
	if s == nil {
		return PlanModeState{}, fmt.Errorf("core: service is nil")
	}
	r := strings.TrimSpace(reason)
	if r == "" {
		r = "user"
	}
	pm := s.ensurePlanMode()
	pm.Enter(r)
	s.setStatus("Plan Mode active (read-only)")
	return s.GetPlanMode(), nil
}

// ExitPlanMode re-enables write tools (VAL-PLAN-007).
// reason should be one of: approve | reject | cancel | user (or free text).
func (s *Service) ExitPlanMode(reason string) (PlanModeState, error) {
	if s == nil {
		return PlanModeState{}, fmt.Errorf("core: service is nil")
	}
	r := strings.TrimSpace(reason)
	if r == "" {
		r = "user"
	}
	pm := s.ensurePlanMode()
	pm.Exit(r)
	s.setStatus("Plan Mode exited — writes re-enabled")
	return s.GetPlanMode(), nil
}

// PlanModeController exposes the shared plan.State for tool registries.
func (s *Service) PlanModeController() *plan.State {
	return s.ensurePlanMode()
}
