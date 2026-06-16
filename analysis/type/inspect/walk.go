package inspect

import "github.com/wippyai/go-lua/analysis/type/typ"

// ScanOptions configures a Scanner.
type ScanOptions struct {
	Seen ScanSeen

	// MaxSteps limits explicit Step calls. Use this for bounded scans where
	// nil leaves and already-seen nodes should still count against the budget.
	MaxSteps int

	// MaxEnters limits distinct nodes accepted by Enter after seen pruning.
	MaxEnters int
}

// Scanner holds reusable traversal state for recursive type scans.
type Scanner struct {
	seen      ScanSeen
	maxSteps  int
	maxEnters int
	steps     int
	enters    int
	exceeded  bool
}

// NewScanner creates a traversal scanner.
func NewScanner(opts ScanOptions) *Scanner {
	return &Scanner{
		seen:      opts.Seen,
		maxSteps:  opts.MaxSteps,
		maxEnters: opts.MaxEnters,
	}
}

// Step records a scan step and reports whether traversal may continue.
func (s *Scanner) Step() bool {
	if s == nil || s.maxSteps <= 0 {
		return true
	}
	s.steps++
	if s.steps <= s.maxSteps {
		return true
	}
	s.exceeded = true
	return false
}

// Enter applies the scanner's seen and unique-node budget policies.
func (s *Scanner) Enter(t typ.Type) bool {
	if s == nil {
		return true
	}
	if s.seen != nil {
		if s.seen.Contains(t) {
			return false
		}
	}
	if s.maxEnters > 0 && s.enters >= s.maxEnters {
		s.exceeded = true
		return false
	}
	if s.seen != nil {
		s.seen.Remember(t)
	}
	s.enters++
	return true
}

// Exceeded reports whether any scanner budget was exceeded.
func (s *Scanner) Exceeded() bool {
	return s != nil && s.exceeded
}

// Complete reports whether the scanner finished without exceeding a budget.
func (s *Scanner) Complete() bool {
	return s == nil || !s.exceeded
}

// WalkChildren visits the canonical child type slots of t in stable order.
func (s *Scanner) WalkChildren(t typ.Type, visit func(typ.Type) bool) bool {
	return WalkChildren(t, visit)
}

// WalkChildren visits the canonical child type slots of t in stable order.
// It returns true when visit reports a match and traversal should stop.
func WalkChildren(t typ.Type, visit func(typ.Type) bool) bool {
	return typ.WalkChildren(t, visit)
}
