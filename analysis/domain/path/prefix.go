package path

import "github.com/wippyai/go-lua/analysis/domain/path/segment"

// SameRoot reports whether two paths refer to the same root identity. Symbol
// identity is authoritative; when either path has a symbol, both symbol and
// version must match. Symbol-less paths fall back to their display root.
func (p Path) SameRoot(other Path) bool {
	if p.Symbol != 0 || other.Symbol != 0 {
		return p.Symbol == other.Symbol && p.Version == other.Version
	}
	return p.Root == other.Root && p.Version == other.Version
}

// HasPrefix reports whether prefix identifies p itself or an ancestor of p.
func (p Path) HasPrefix(prefix Path) bool {
	if !p.SameRoot(prefix) || len(prefix.Segments) > len(p.Segments) {
		return false
	}
	return segmentsHavePrefix(p.Segments, prefix.Segments)
}

// HasStrictPrefix reports whether prefix identifies a proper ancestor of p.
func (p Path) HasStrictPrefix(prefix Path) bool {
	return len(prefix.Segments) < len(p.Segments) && p.HasPrefix(prefix)
}

// Overlaps reports whether either path is the same as, or an ancestor of, the
// other. This is the canonical path-overlap predicate for invalidation checks.
func (p Path) Overlaps(other Path) bool {
	return p.HasPrefix(other) || other.HasPrefix(p)
}

// SuffixAfter returns the suffix segments after prefix when prefix identifies p
// or an ancestor of p. The returned slice is copied and can be mutated.
func (p Path) SuffixAfter(prefix Path) ([]segment.Segment, bool) {
	if !p.HasPrefix(prefix) {
		return nil, false
	}
	remaining := p.Segments[len(prefix.Segments):]
	if len(remaining) == 0 {
		return nil, true
	}
	out := make([]segment.Segment, len(remaining))
	copy(out, remaining)
	return out, true
}

func segmentsHavePrefix(candidate, prefix []segment.Segment) bool {
	for i, seg := range prefix {
		if candidate[i] != seg {
			return false
		}
	}
	return true
}
