package browser

import (
	"fmt"
	"net"
	"sync"
)

// Mission port range (VAL-BRW-001 / VAL-BRW-010). Never use 5000, 7000, 5599.
const (
	PortMin = 4100
	PortMax = 4199
)

// PortAllocator finds free TCP ports in [PortMin, PortMax].
type PortAllocator struct {
	mu       sync.Mutex
	next     int
	reserved map[int]struct{}
}

// NewPortAllocator starts scanning at PortMin.
func NewPortAllocator() *PortAllocator {
	return &PortAllocator{
		next:     PortMin,
		reserved: make(map[int]struct{}),
	}
}

// Allocate returns an available port in [4100, 4199].
func (a *PortAllocator) Allocate() (int, error) {
	if a == nil {
		a = NewPortAllocator()
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	start := a.next
	for i := 0; i <= (PortMax - PortMin); i++ {
		p := start + i
		if p > PortMax {
			p = PortMin + (p-PortMin)%(PortMax-PortMin+1)
		}
		if _, taken := a.reserved[p]; taken {
			continue
		}
		if portFree(p) {
			a.reserved[p] = struct{}{}
			a.next = p + 1
			if a.next > PortMax {
				a.next = PortMin
			}
			return p, nil
		}
	}
	return 0, fmt.Errorf("browser: no free CDP port in %d–%d", PortMin, PortMax)
}

// Release frees a previously allocated port reservation.
func (a *PortAllocator) Release(port int) {
	if a == nil {
		return
	}
	a.mu.Lock()
	delete(a.reserved, port)
	a.mu.Unlock()
}

// InRange reports whether port is inside the mission CDP window.
func InRange(port int) bool {
	return port >= PortMin && port <= PortMax
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
