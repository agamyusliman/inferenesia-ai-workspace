package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agamyusliman/inferenesia-app/internal/git"
)

// Git tool names for the agent (VAL-GIT-005/006/007/009).
const (
	NameGitStatus       = "git_status"
	NameGitDiff         = "git_diff"
	NameGitLog          = "git_log"
	NameGitStage        = "git_stage"
	NameGitUnstage      = "git_unstage"
	NameGitCommit       = "git_commit"
	NameGitFetch        = "git_fetch"
	NameGitPull         = "git_pull"
	NameGitPush         = "git_push"
	NameGitBranchList   = "git_branch_list"
	NameGitCheckout     = "git_checkout"
	NameGitResetHard    = "git_reset_hard"
	NameGitBranchDelete = "git_branch_delete"
)

// GitBackend is the agent-facing git surface (agent-mode GitService with hooks isolation).
type GitBackend interface {
	Status() (git.Status, error)
	Diff(path string, staged bool) (git.DiffResult, error)
	LogOneline(n int) (string, error)
	Stage(paths ...string) (git.OpResult, error)
	Unstage(paths ...string) (git.OpResult, error)
	CommitAgent(message string, approved bool) (git.OpResult, error)
	Fetch() (git.OpResult, error)
	Pull() (git.OpResult, error)
	Push(opts git.PushOptions) (git.OpResult, error)
	Branches() (git.BranchList, error)
	Checkout(name string, create bool) (git.OpResult, error)
	ResetHard(opts git.DestructiveOptions) (git.OpResult, error)
	DeleteBranch(opts git.DestructiveOptions) (git.OpResult, error)
	IsAgentMode() bool
}

// Approver requests user confirmation for gated git ops (NeedsApproval).
// Returns true when the user approves; false when rejected/cancelled.
type Approver interface {
	RequestApproval(ctx context.Context, req git.ApprovalRequest) (bool, error)
}

// SetGitBackend wires the agent GitService (AgentMode=true) and optional Approver.
func (r *Registry) SetGitBackend(backend GitBackend, approver Approver) {
	if r == nil {
		return
	}
	r.git = backend
	r.gitApprover = approver
}

func gitDefs() []Definition {
	return []Definition{
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitStatus,
				Description: "Show git status (branch + porcelain-style changed files) for the workspace repo.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitDiff,
				Description: "Show unified diff for a workspace-relative path. Use staged=true for index vs HEAD.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Path relative to repo root" },
    "staged": { "type": "boolean", "description": "If true, show staged (cached) diff" }
  },
  "required": ["path"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitLog,
				Description: "Show recent commits (oneline). Optional limit (default 10).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "limit": { "type": "integer", "description": "Max commits (default 10)" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitStage,
				Description: "Stage one or more paths (git add).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "paths": { "type": "array", "items": { "type": "string" }, "description": "Paths to stage" },
    "path": { "type": "string", "description": "Single path (convenience)" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitUnstage,
				Description: "Unstage one or more paths (git restore --staged).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "paths": { "type": "array", "items": { "type": "string" } },
    "path": { "type": "string" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitCommit,
				Description: "Create a git commit from the index. REQUIRES user confirmation (NeedsApproval). Do not force-push or use --no-verify.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "message": { "type": "string", "description": "Commit message" }
  },
  "required": ["message"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitFetch,
				Description: "Run git fetch (no force).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitPull,
				Description: "Run git pull (no force; prefers ff-only).",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitPush,
				Description: "Push to remote. Default is non-force and still needs confirmation when policy requires it. force or force_with_lease ALWAYS need explicit user confirmation (NeedsApproval). Never silent force-push.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "remote": { "type": "string", "description": "Remote name (default origin)" },
    "force": { "type": "boolean", "description": "Use --force (requires confirmation)" },
    "force_with_lease": { "type": "boolean", "description": "Use --force-with-lease (requires confirmation)" },
    "refspec": { "type": "string", "description": "Optional refspec" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitBranchList,
				Description: "List local and remote branches.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitCheckout,
				Description: "Checkout a branch (or create with create=true).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "create": { "type": "boolean" }
  },
  "required": ["name"]
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitResetHard,
				Description: "DESTRUCTIVE: git reset --hard. ALWAYS requires explicit user confirmation (NeedsApproval). Cancel leaves the tree unchanged.",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "ref": { "type": "string", "description": "Reset target (default HEAD)" }
  }
}`),
			},
		},
		{
			Type: "function",
			Function: FunctionSpec{
				Name:        NameGitBranchDelete,
				Description: "DESTRUCTIVE: delete a local branch. ALWAYS requires explicit user confirmation (NeedsApproval).",
				Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "force": { "type": "boolean", "description": "Use -D instead of -d" }
  },
  "required": ["name"]
}`),
			},
		},
	}
}

func (r *Registry) execGit(ctx context.Context, call Call) Result {
	if r.git == nil {
		return Result{
			Name:    call.Name,
			Content: "git tools not configured (no workspace git backend)",
			IsError: true,
		}
	}
	name := strings.TrimSpace(call.Name)
	switch name {
	case NameGitStatus:
		st, err := r.git.Status()
		if err != nil {
			return gitErr(name, err)
		}
		b, _ := json.MarshalIndent(st, "", "  ")
		return Result{Name: name, Content: string(b)}
	case NameGitDiff:
		var args struct {
			Path   string `json:"path"`
			Staged bool   `json:"staged"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return gitErr(name, err)
		}
		d, err := r.git.Diff(args.Path, args.Staged)
		if err != nil {
			return gitErr(name, err)
		}
		if d.Content == "" {
			return Result{Name: name, Content: d.Message}
		}
		return Result{Name: name, Content: d.Content}
	case NameGitLog:
		var args struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &args)
		if args.Limit <= 0 {
			args.Limit = 10
		}
		log, err := r.git.LogOneline(args.Limit)
		if err != nil {
			return gitErr(name, err)
		}
		return Result{Name: name, Content: log}
	case NameGitStage:
		paths, err := pathsFromArgs(call.Arguments)
		if err != nil {
			return gitErr(name, err)
		}
		res, err := r.git.Stage(paths...)
		return gitOpResult(name, res, err)
	case NameGitUnstage:
		paths, err := pathsFromArgs(call.Arguments)
		if err != nil {
			return gitErr(name, err)
		}
		res, err := r.git.Unstage(paths...)
		return gitOpResult(name, res, err)
	case NameGitCommit:
		var args struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return gitErr(name, err)
		}
		msg := strings.TrimSpace(args.Message)
		if msg == "" {
			return Result{Name: name, Content: "git_commit: message is required", IsError: true}
		}
		approved, aerr := r.requestApproval(ctx, git.ApprovalRequest{
			Kind:    git.OpCommit,
			Summary: "Agent wants to create a git commit",
			Detail:  "Message: " + msg,
		})
		if aerr != nil {
			return gitErr(name, aerr)
		}
		if !approved {
			return Result{
				Name:    name,
				Content: "NeedsApproval: user rejected agent commit — commit aborted, no tree change",
				IsError: true,
			}
		}
		res, err := r.git.CommitAgent(msg, true)
		return gitOpResult(name, res, err)
	case NameGitFetch:
		res, err := r.git.Fetch()
		return gitOpResult(name, res, err)
	case NameGitPull:
		res, err := r.git.Pull()
		return gitOpResult(name, res, err)
	case NameGitPush:
		var args struct {
			Remote         string `json:"remote"`
			Force          bool   `json:"force"`
			ForceWithLease bool   `json:"force_with_lease"`
			Refspec        string `json:"refspec"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &args)
		force := args.Force || args.ForceWithLease
		kind := git.OpPush
		summary := "Agent wants to push to remote"
		if force {
			kind = git.OpForcePush
			summary = "Agent wants to FORCE-PUSH (destructive to remote history)"
		}
		approved, aerr := r.requestApproval(ctx, git.ApprovalRequest{
			Kind:    kind,
			Summary: summary,
			Detail:  fmt.Sprintf("remote=%s force=%v force_with_lease=%v", firstNonEmptyStr(args.Remote, "origin"), args.Force, args.ForceWithLease),
			Force:   force,
		})
		if aerr != nil {
			return gitErr(name, aerr)
		}
		if !approved {
			return Result{
				Name:    name,
				Content: "NeedsApproval: user rejected push — no remote change",
				IsError: true,
			}
		}
		opts := git.PushOptions{
			Remote:         args.Remote,
			Force:          args.Force,
			ForceWithLease: args.ForceWithLease,
			Approved:       true,
			RequireConfirm: true,
		}
		if rs := strings.TrimSpace(args.Refspec); rs != "" {
			opts.Refspecs = []string{rs}
		}
		res, err := r.git.Push(opts)
		return gitOpResult(name, res, err)
	case NameGitBranchList:
		list, err := r.git.Branches()
		if err != nil {
			return gitErr(name, err)
		}
		b, _ := json.MarshalIndent(list, "", "  ")
		return Result{Name: name, Content: string(b)}
	case NameGitCheckout:
		var args struct {
			Name   string `json:"name"`
			Create bool   `json:"create"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return gitErr(name, err)
		}
		res, err := r.git.Checkout(args.Name, args.Create)
		return gitOpResult(name, res, err)
	case NameGitResetHard:
		var args struct {
			Ref string `json:"ref"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &args)
		approved, aerr := r.requestApproval(ctx, git.ApprovalRequest{
			Kind:    git.OpResetHard,
			Summary: "DESTRUCTIVE: agent wants git reset --hard",
			Detail:  "ref=" + firstNonEmptyStr(args.Ref, "HEAD"),
		})
		if aerr != nil {
			return gitErr(name, aerr)
		}
		if !approved {
			return Result{
				Name:    name,
				Content: "NeedsApproval: user rejected reset --hard — working tree unchanged",
				IsError: true,
			}
		}
		res, err := r.git.ResetHard(git.DestructiveOptions{Approved: true, Ref: args.Ref})
		return gitOpResult(name, res, err)
	case NameGitBranchDelete:
		var args struct {
			Name  string `json:"name"`
			Force bool   `json:"force"`
		}
		if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
			return gitErr(name, err)
		}
		approved, aerr := r.requestApproval(ctx, git.ApprovalRequest{
			Kind:    git.OpBranchDelete,
			Summary: "DESTRUCTIVE: agent wants to delete branch " + args.Name,
			Detail:  fmt.Sprintf("force=%v", args.Force),
		})
		if aerr != nil {
			return gitErr(name, aerr)
		}
		if !approved {
			return Result{
				Name:    name,
				Content: "NeedsApproval: user rejected branch delete — no tree change",
				IsError: true,
			}
		}
		res, err := r.git.DeleteBranch(git.DestructiveOptions{
			Approved:    true,
			Ref:         args.Name,
			ForceDelete: args.Force,
		})
		return gitOpResult(name, res, err)
	default:
		return Result{Name: name, Content: "unknown git tool " + name, IsError: true}
	}
}

func (r *Registry) requestApproval(ctx context.Context, req git.ApprovalRequest) (bool, error) {
	if r.gitApprover == nil {
		// No UI/CLI approver: hard deny gated ops (safe default).
		return false, nil
	}
	return r.gitApprover.RequestApproval(ctx, req)
}

func pathsFromArgs(raw string) ([]string, error) {
	var args struct {
		Paths []string `json:"paths"`
		Path  string   `json:"path"`
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return nil, err
		}
	}
	var out []string
	for _, p := range args.Paths {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if p := strings.TrimSpace(args.Path); p != "" {
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("path or paths required")
	}
	return out, nil
}

func gitErr(name string, err error) Result {
	return Result{Name: name, Content: fmt.Sprintf("%s: %v", name, err), IsError: true}
}

func gitOpResult(name string, res git.OpResult, err error) Result {
	if err != nil {
		msg := res.Message
		if msg == "" {
			msg = err.Error()
		}
		if res.Detail != "" {
			msg += "\n" + res.Detail
		}
		return Result{Name: name, Content: msg, IsError: true}
	}
	msg := res.Message
	if res.Hash != "" {
		msg += " hash=" + res.Hash
	}
	if res.Detail != "" {
		msg += "\n" + res.Detail
	}
	return Result{Name: name, Content: msg, IsError: !res.OK}
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
