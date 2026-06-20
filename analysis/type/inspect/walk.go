package inspect

import "github.com/wippyai/go-lua/analysis/type/typ"

// ScanOptions configures a Scanner.
type ScanOptions struct {
	Seen ScanSeen

	// MaxEnters limits distinct nodes accepted by Enter after seen pruning.
	MaxEnters int
}

// Scanner holds reusable traversal state for recursive type scans.
type Scanner struct {
	seen      ScanSeen
	maxEnters int
	enters    int
}

// NewScanner creates a traversal scanner.
func NewScanner(opts ScanOptions) *Scanner {
	return &Scanner{
		seen:      opts.Seen,
		maxEnters: opts.MaxEnters,
	}
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
		return false
	}
	if s.seen != nil {
		s.seen.Remember(t)
	}
	s.enters++
	return true
}

// WalkChildren visits the canonical child type slots of t in stable order.
// It returns true when visit reports a match and traversal should stop.
func WalkChildren(t typ.Type, visit func(typ.Type) bool) bool {
	return typ.WalkChildren(t, visit)
}

func unwrapTransparent(t typ.Type) typ.Type {
	return typ.UnwrapTransparentWrappers(t)
}
