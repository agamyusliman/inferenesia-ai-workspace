package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agamyusliman/inferenesia-app/internal/git"
)

func TestGitApproverBridgeApprove(t *testing.T) {
	b := newGitApproverBridge()
	b.timeout = 2 * time.Second
	var got ChatEvent
	b.setEmit(func(ev ChatEvent) { got = ev })

	var wg sync.WaitGroup
	wg.Add(1)
	var approved bool
	var err error
	go func() {
		defer wg.Done()
		approved, err = b.RequestApproval(context.Background(), git.ApprovalRequest{
			Kind:    git.OpCommit,
			Summary: "test commit",
			Detail:  "msg",
		})
	}()
	// Wait until pending appears
	deadline := time.Now().Add(time.Second)
	var pending *PendingApproval
	for time.Now().Before(deadline) {
		pending = b.Pending()
		if pending != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pending == nil {
		t.Fatal("expected pending approval")
	}
	if got.Type != ChatEventNeedsApproval || got.ApprovalID != pending.ID {
		t.Fatalf("event: %+v pending=%+v", got, pending)
	}
	if err := b.Resolve(pending.ID, true); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err != nil || !approved {
		t.Fatalf("approved=%v err=%v", approved, err)
	}
}

func TestGitApproverBridgeReject(t *testing.T) {
	b := newGitApproverBridge()
	b.timeout = 2 * time.Second
	done := make(chan bool, 1)
	go func() {
		ok, err := b.RequestApproval(context.Background(), git.ApprovalRequest{
			Kind:    git.OpForcePush,
			Summary: "force",
			Force:   true,
		})
		if err != nil {
			t.Errorf("err: %v", err)
		}
		done <- ok
	}()
	var pending *PendingApproval
	for i := 0; i < 50; i++ {
		pending = b.Pending()
		if pending != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pending == nil {
		t.Fatal("no pending")
	}
	if err := b.Resolve(pending.ID, false); err != nil {
		t.Fatal(err)
	}
	ok := <-done
	if ok {
		t.Fatal("expected reject")
	}
}
