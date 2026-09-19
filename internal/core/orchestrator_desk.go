package core

import (
	"sync"

	"github.com/agamyusliman/inferenesia-app/internal/orchestrator"
	"github.com/agamyusliman/inferenesia-app/internal/tools"
)

// ChatEventTaskSpawned / ChatEventTaskDone map orchestrator lifecycle to desktop SSE.
const (
	ChatEventTaskSpawned ChatEventType = "TaskSpawned"
	ChatEventTaskDone    ChatEventType = "TaskDone"
	ChatEventBlocked     ChatEventType = "Blocked"
	ChatEventNeedsAppr   ChatEventType = "NeedsApproval"
)

// orchState holds the per-Service orchestrator Manager (lazy).
type orchState struct {
	mu  sync.Mutex
	mgr *orchestrator.Manager
	// emit bridges TaskSpawned/TaskDone to ChatEvent for the active stream.
	emit func(ChatEvent)
}

func (s *Service) ensureOrch() *orchState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.orch == nil {
		s.orch = &orchState{}
	}
	if s.orch.mgr == nil {
		os := s.orch
		s.orch.mgr = orchestrator.New(orchestrator.Options{
			Emitter: func(ev orchestrator.Event) {
				os.mu.Lock()
				emit := os.emit
				os.mu.Unlock()
				if emit == nil {
					return
				}
				ce := ChatEvent{
					Detail: ev.Detail,
					Text:   ev.Summary,
					Error:  ev.Error,
					Name:   string(ev.Type),
					Path:   ev.TaskID,
				}
				switch ev.Type {
				case orchestrator.EventTaskSpawned:
					ce.Type = ChatEventTaskSpawned
					// Structured detail for task panel: task_id|category|status|label
					ce.Detail = ev.TaskID + "|" + ev.Category + "|" + string(ev.Status) + "|" + ev.Detail
					ce.Text = string(ev.Status)
				case orchestrator.EventTaskDone:
					ce.Type = ChatEventTaskDone
					ce.Detail = ev.TaskID + "|" + ev.Category + "|" + string(ev.Status) + "|" + ev.Detail
					ce.Text = string(ev.Status)
					if ev.SynthesisConsumed {
						ce.Detail += "|synthesis_consumed=true"
						ce.Text = ev.Summary
					}
				case orchestrator.EventBlocked:
					ce.Type = ChatEventBlocked
					ce.Detail = ev.Detail
				case orchestrator.EventNeedsApproval:
					ce.Type = ChatEventNeedsAppr
					ce.Detail = ev.Detail
				default:
					return
				}
				emit(ce)
			},
		})
		// Persist TaskRun rows for cancel/resume (VAL-ORCH-006/007).
		s.wireTaskPersist(s.orch.mgr)
	}
	return s.orch
}

// setOrchEmit registers the chat stream callback for orchestrator events.
func (o *orchState) setEmit(emit func(ChatEvent)) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.emit = emit
	o.mu.Unlock()
}

// wireTaskBackend attaches task() to a tools.Registry for workspace root.
// explore uses built-in ExploreRunner; write locks go through Manager.
// Category filter is applied per Spec.Category on sub-task spawn via adapter runner.
func (s *Service) wireTaskBackend(reg *tools.Registry, root string, emit func(ChatEvent)) {
	if s == nil || reg == nil {
		return
	}
	os := s.ensureOrch()
	os.setEmit(emit)
	runner := s.defaultTaskRunner(root)
	adapter := tools.NewTaskAdapter(os.mgr, runner)
	if active := s.GetActiveWorkspace(); active != nil {
		adapter.SetWorkspaceID(active.ID)
	}
	reg.SetTaskBackend(adapter)
}

// OrchestratorManager returns the shared Manager (for tests / later task panel API).
func (s *Service) OrchestratorManager() *orchestrator.Manager {
	if s == nil {
		return nil
	}
	return s.ensureOrch().mgr
}
