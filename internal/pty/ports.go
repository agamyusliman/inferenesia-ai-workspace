package pty

// ControlPortMin/Max are the only allowed local debug/control ports for PTY
// (and the rest of the app). VAL-TERM-010: never open 5000, 7000, 5599.
const (
	ControlPortMin = 4100
	ControlPortMax = 4199
)

// OffLimitsPorts must never be used by PTY or mission-started services.
var OffLimitsPorts = []int{5000, 7000, 5599}

// PortInRange reports whether p is in [4100, 4199].
func PortInRange(p int) bool {
	return p >= ControlPortMin && p <= ControlPortMax
}

// PortAllowed reports whether p is safe to open (in range and not off-limits).
// Note: off-limits ports are already outside 4100–4199; this is belt-and-suspenders.
func PortAllowed(p int) bool {
	if !PortInRange(p) {
		return false
	}
	for _, bad := range OffLimitsPorts {
		if p == bad {
			return false
		}
	}
	return true
}
