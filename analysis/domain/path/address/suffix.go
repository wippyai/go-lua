package address

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// Suffix is the structured member/index suffix of an address.
type Suffix struct {
	segments []segment.Segment
}

// SuffixOfSegments builds a defensive structured suffix value.
func SuffixOfSegments(segments []segment.Segment) Suffix {
	return suffixOfOwnedSegments(cloneSegments(segments))
}

func suffixOfOwnedSegments(segments []segment.Segment) Suffix {
	if len(segments) == 0 {
		return Suffix{}
	}
	return Suffix{segments: segments}
}

// Segments returns a defensive copy of the suffix segments.
func (s Suffix) Segments() []segment.Segment {
	return cloneSegments(s.segments)
}

// Len returns the number of member/index steps in the suffix.
func (s Suffix) Len() int { return len(s.segments) }

// Parent returns the suffix without its last step.
func (s Suffix) Parent() (Suffix, bool) {
	if len(s.segments) == 0 {
		return Suffix{}, false
	}
	return suffixOfOwnedSegments(cloneSegments(s.segments[:len(s.segments)-1])), true
}

// Equal reports structural suffix equality.
func (s Suffix) Equal(other Suffix) bool {
	if len(s.segments) != len(other.segments) {
		return false
	}
	for i := range s.segments {
		if s.segments[i] != other.segments[i] {
			return false
		}
	}
	return true
}

// HasPrefix reports whether prefix is this suffix or one of its ancestors.
func (s Suffix) HasPrefix(prefix Suffix) bool {
	if len(prefix.segments) > len(s.segments) {
		return false
	}
	for i := range prefix.segments {
		if s.segments[i] != prefix.segments[i] {
			return false
		}
	}
	return true
}

// RemainderAfterPrefix returns the suffix below prefix.
func (s Suffix) RemainderAfterPrefix(prefix Suffix) ([]segment.Segment, bool) {
	if !s.HasPrefix(prefix) {
		return nil, false
	}
	return cloneSegments(s.segments[len(prefix.segments):]), true
}

// Overlaps reports whether one suffix is a prefix of the other.
func (s Suffix) Overlaps(other Suffix) bool {
	return s.HasPrefix(other) || other.HasPrefix(s)
}

// KeySuffix returns the deterministic segment suffix encoding.
func (s Suffix) KeySuffix() string {
	return segment.FormatSegments(s.segments)
}

// RelativeStaticMemberSuffixKey returns the canonical static-member key for a
// relative suffix. It intentionally encodes only suffix segments so rootless
// member facts do not collapse to an empty path key.
func RelativeStaticMemberSuffixKey(segments []segment.Segment) (pathdom.PathKey, bool) {
	if len(segments) == 0 {
		return "", false
	}
	return pathdom.PathKey(segment.FormatSegments(segments)), true
}

// RelativeStaticMemberSuffixSegments parses a rootless static-member suffix key.
func RelativeStaticMemberSuffixSegments(key pathdom.PathKey) ([]segment.Segment, bool) {
	if key == "" {
		return nil, false
	}
	return segment.ParseFormattedSegments(string(key))
}

func cloneSegments(segments []segment.Segment) []segment.Segment {
	if len(segments) == 0 {
		return nil
	}
	return append([]segment.Segment(nil), segments...)
}
