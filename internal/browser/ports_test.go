package browser

import (
	"testing"
)

func TestPortAllocatorInRange(t *testing.T) {
	a := NewPortAllocator()
	seen := map[int]bool{}
	for i := 0; i < 20; i++ {
		p, err := a.Allocate()
		if err != nil {
			t.Fatalf("allocate: %v", err)
		}
		if !InRange(p) {
			t.Fatalf("port %d outside 4100–4199", p)
		}
		if seen[p] {
			// May re-use after release only; reserved should be unique until Release.
			t.Fatalf("duplicate reserved port %d", p)
		}
		seen[p] = true
		// Keep reserved so we force different ports; release every other.
		if i%2 == 1 {
			a.Release(p)
			delete(seen, p)
		}
	}
	// Cleanup
	for p := range seen {
		a.Release(p)
	}
}

func TestDebugLaunchArgsPortRange(t *testing.T) {
	args := DebugLaunchArgs(4123)
	found := false
	for _, a := range args {
		if a == "--remote-debugging-port=4123" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected remote-debugging-port arg, got %v", args)
	}
}

func TestInRangeBounds(t *testing.T) {
	if InRange(4099) || InRange(4200) || InRange(5000) || InRange(7000) || InRange(5599) {
		t.Fatal("off-limits / out-of-range ports must be rejected")
	}
	if !InRange(4100) || !InRange(4199) {
		t.Fatal("boundary ports 4100 and 4199 must be accepted")
	}
}
