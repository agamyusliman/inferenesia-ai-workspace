package core

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// chatRun is a backend-owned generation. It continues after HTTP client disconnect
// so navigation/reload does not stop the agent. Explicit Stop calls CancelChatRun.
type chatRun struct {
	id     string
	cancel context.CancelFunc
	mu     sync.Mutex
	subs   map[chan ChatEvent]struct{}
	done   chan struct{}
	err    error
}

var chatRunSeq atomic.Uint64

func (r *chatRun) subscribe(buf int) (<-chan ChatEvent, func()) {
	ch := make(chan ChatEvent, buf)
	r.mu.Lock()
	if r.subs == nil {
		r.subs = make(map[chan ChatEvent]struct{})
	}
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	unsub := func() {
		r.mu.Lock()
		if _, ok := r.subs[ch]; ok {
			delete(r.subs, ch)
			close(ch)
		}
		r.mu.Unlock()
	}
	return ch, unsub
}

func (r *chatRun) publish(ev ChatEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for ch := range r.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (r *chatRun) closeSubs() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for ch := range r.subs {
		close(ch)
	}
	r.subs = nil
}

func (s *Service) ensureChatRuns() {
	if s.chatRuns == nil {
		s.chatRuns = make(map[string]*chatRun)
	}
}

// StartChatRun begins a detached agent turn. The run survives HTTP disconnect.
func (s *Service) StartChatRun(req ChatRequest) (string, <-chan ChatEvent, func(), error) {
	if s == nil {
		return "", nil, nil, fmt.Errorf("core: service is nil")
	}
	n := chatRunSeq.Add(1)
	id := fmt.Sprintf("run_%d_%d", timeNowUnixNano(), n)
	ctx, cancel := context.WithCancel(context.Background())
	run := &chatRun{
		id:     id,
		cancel: cancel,
		subs:   make(map[chan ChatEvent]struct{}),
		done:   make(chan struct{}),
	}
	ch, unsub := run.subscribe(256)

	s.chatRunsMu.Lock()
	s.ensureChatRuns()
	s.chatRuns[id] = run
	s.chatRunsMu.Unlock()

	go func() {
		defer close(run.done)
		defer run.closeSubs()
		defer func() {
			s.chatRunsMu.Lock()
			delete(s.chatRuns, id)
			s.chatRunsMu.Unlock()
		}()
		run.publish(ChatEvent{Type: ChatEventRunStarted, RunID: id})
		err := s.ChatStream(ctx, req, func(ev ChatEvent) {
			if ev.RunID == "" {
				ev.RunID = id
			}
			run.publish(ev)
		})
		run.err = err
		if err != nil && ctx.Err() == nil {
			run.publish(ChatEvent{Type: ChatEventError, Error: err.Error(), RunID: id})
		}
	}()

	return id, ch, unsub, nil
}

// CancelChatRun stops a detached run by id (Stop button). Safe if already finished.
func (s *Service) CancelChatRun(runID string) bool {
	if s == nil || runID == "" {
		return false
	}
	s.chatRunsMu.Lock()
	run := s.chatRuns[runID]
	s.chatRunsMu.Unlock()
	if run == nil {
		return false
	}
	run.cancel()
	return true
}

func timeNowUnixNano() int64 {
	return time.Now().UnixNano()
}
