package inspect

import "github.com/wippyai/go-lua/analysis/type/typ"

// ScanOptions configures a Scanner.
type ScanOptions struct {
	Seen ScanSeen
}

// Scanner holds reusable traversal state for recursive type scans.
type Scanner struct {
	seen   ScanSeen
	enters int
}

// NewScanner creates a traversal scanner.
func NewScanner(opts ScanOptions) *Scanner {
	return &Scanner{
		seen: opts.Seen,
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
	if s.seen != nil {
		s.seen.Remember(t)
	}
	s.enters++
	return true
}
