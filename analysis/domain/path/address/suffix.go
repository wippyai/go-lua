package address

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// SuffixKey is the typed string carrier for rootless static-member suffix
// facts. It is deliberately not a full path key: it contains only the member
// suffix spelling (for example ".field" or "[\"name\"]") and has no root.
// Hot state maps should still use the interned keyspace.Key representation.
type SuffixKey pathdom.PathKey

// PathKey returns the legacy string carrier for compatibility boundaries.
func (k SuffixKey) PathKey() pathdom.PathKey { return pathdom.PathKey(k) }

func (k SuffixKey) String() string { return string(k) }

// Valid reports whether k is a non-empty rootless static-member suffix.
func (k SuffixKey) Valid() bool {
	_, ok := SuffixKeyFromPathKey(k.PathKey())
	return ok
}

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

// Parent returns the suffix without its last step.
func (s Suffix) Parent() (Suffix, bool) {
	if len(s.segments) == 0 {
		return Suffix{}, false
	}
	return suffixOfOwnedSegments(s.segments[:len(s.segments)-1]), true
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
func RelativeStaticMemberSuffixKey(segments []segment.Segment) (SuffixKey, bool) {
	if len(segments) == 0 {
		return "", false
	}
	return SuffixKey(segment.FormatSegments(segments)), true
}

// SuffixKeyFromPathKey validates and narrows key to the rootless static-member
// suffix grammar.
func SuffixKeyFromPathKey(key pathdom.PathKey) (SuffixKey, bool) {
	if key == "" {
		return "", false
	}
	if !segment.ValidFormattedSegments(string(key)) {
		return "", false
	}
	return SuffixKey(key), true
}

// RelativeStaticMemberSuffixSegments parses a rootless static-member suffix key.
func RelativeStaticMemberSuffixSegments(key SuffixKey) ([]segment.Segment, bool) {
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
