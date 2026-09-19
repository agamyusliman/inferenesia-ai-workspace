package tools

import (
	"context"
	"fmt"

	"github.com/agamyusliman/inferenesia-app/internal/tools/diagram"
	"github.com/agamyusliman/inferenesia-app/internal/writegate"
)

// DiagramBackend is the subset of *diagram.Tools the registry needs to
// advertise and dispatch create_diagram / update_diagram / export_diagram
// (VAL-DIAG-002/003/004).
type DiagramBackend interface {
	Definitions() []diagram.ToolDef
	Execute(ctx context.Context, call diagram.Call) diagram.Result
}

// DiagramAdapter wraps *diagram.Tools to satisfy DiagramBackend.
type DiagramAdapter struct {
	Tools *diagram.Tools
}

// NewDiagramAdapter wraps diagram tools for tools.Registry.SetDiagram.
func NewDiagramAdapter(t *diagram.Tools) *DiagramAdapter {
	return &DiagramAdapter{Tools: t}
}

// Definitions implements DiagramBackend.
func (a *DiagramAdapter) Definitions() []diagram.ToolDef {
	if a == nil || a.Tools == nil {
		return nil
	}
	return a.Tools.Definitions()
}

// Execute implements DiagramBackend.
func (a *DiagramAdapter) Execute(ctx context.Context, call diagram.Call) diagram.Result {
	if a == nil || a.Tools == nil {
		return diagram.Result{Name: call.Name, Content: "diagram: backend not configured", IsError: true}
	}
	return a.Tools.Execute(ctx, call)
}

// SetDiagram wires diagram tools (create/update/export) into the registry.
// Pass nil to clear. Accepts *diagram.Tools, *DiagramAdapter, or any
// DiagramBackend.
func (r *Registry) SetDiagram(d any) {
	if r == nil {
		return
	}
	switch v := d.(type) {
	case nil:
		r.diagram = nil
	case *diagram.Tools:
		r.diagram = NewDiagramAdapter(v)
	case DiagramBackend:
		r.diagram = v
	default:
		// ignore unknown types
	}
}

// Diagram returns the attached diagram backend (may be nil).
func (r *Registry) Diagram() DiagramBackend {
	if r == nil {
		return nil
	}
	return r.diagram
}

// AttachDiagramFromGateway builds diagram tools bound to the registry's
// WriteGateway and wires them in. No-op when the registry has no gateway.
// Returns the adapter (nil when no gateway).
func (r *Registry) AttachDiagramFromGateway() *DiagramAdapter {
	if r == nil || r.gw == nil {
		return nil
	}
	t := diagram.NewTools(r.gw)
	adapter := NewDiagramAdapter(t)
	r.SetDiagram(adapter)
	return adapter
}

// EnsureDiagramFromGateway is a convenience used by CLI/desktop bootstrap to
// guarantee diagram tools are attached once a WriteGateway exists. It never
// overrides an explicitly-attached backend.
func EnsureDiagramFromGateway(r *Registry) {
	if r == nil {
		return
	}
	if r.diagram != nil {
		return
	}
	r.AttachDiagramFromGateway()
}

// diagramDefs returns OpenAI tool definitions for the attached diagram backend.
func diagramDefs(r *Registry) []Definition {
	if r == nil || r.diagram == nil {
		return nil
	}
	var defs []Definition
	for _, d := range r.diagram.Definitions() {
		params := d.Parameters
		if len(params) == 0 {
			params = jsonRawEmpty()
		}
		defs = append(defs, Definition{
			Type: d.Type,
			Function: FunctionSpec{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  params,
			},
		})
	}
	return defs
}

// execDiagram dispatches a diagram tool call to the backend and maps the
// diagram.Result into a tools.Result.
func (r *Registry) execDiagram(ctx context.Context, call Call) Result {
	if r == nil || r.diagram == nil {
		return Result{
			Name:    call.Name,
			Content: fmt.Sprintf("diagram: backend not configured for tool %q", call.Name),
			IsError: true,
		}
	}
	dRes := r.diagram.Execute(ctx, diagram.Call{
		ID:        call.ID,
		Name:      call.Name,
		Arguments: call.Arguments,
	})
	return Result{
		ID:      dRes.ID,
		Name:    dRes.Name,
		Content: dRes.Content,
		IsError: dRes.IsError,
	}
}

// jsonRawEmpty returns an empty object JSON for tools without parameters.
func jsonRawEmpty() []byte {
	return []byte(`{"type":"object","properties":{}}`)
}

// Compile-time assertions that the adapter satisfies the interface.
var _ DiagramBackend = (*DiagramAdapter)(nil)

// _ keeps writegate import referenced when only the adapter path is used.
var _ = writegate.SourceDiagram
