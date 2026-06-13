package address

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// StructuralKey is a parsed path-key identity in either point-local or stable
// address space. Local and stable keys are intentionally not comparable.
type StructuralKey struct {
	local    bool
	sym      symbol.ID
	version  int
	segments []segment.Segment
	stable   Stable
}

// StructuralKeyFromPathKey parses recognized structural path-key spellings.
func StructuralKeyFromPathKey(pathKey pathdom.PathKey) (StructuralKey, bool) {
	sym, version, suffix, ok := ParseResolverPath(pathKey)
	if ok && version > 0 {
		segments, ok := segment.InternFormattedSegments(suffix)
		if !ok {
			return StructuralKey{}, false
		}
		return StructuralKey{
			local:    true,
			sym:      sym,
			version:  version,
			segments: cloneSegments(segments),
		}, true
	}
	stable, ok := StableFromKey(pathKey)
	if !ok {
		return StructuralKey{}, false
	}
	return StructuralKey{stable: stable}, true
}

// PathKey returns the canonical key spelling for this structural key.
func (k StructuralKey) PathKey() pathdom.PathKey {
	if k.local {
		local, ok := LocalKeyForVersion(k.sym, k.version, k.segments)
		if !ok {
			return ""
		}
		return local.PathKey()
	}
	return k.stable.Key()
}

// HasPrefix reports whether prefix is this key or one of its ancestors.
func (k StructuralKey) HasPrefix(prefix StructuralKey) bool {
	if k.local || prefix.local {
		return k.local &&
			prefix.local &&
			k.sym == prefix.sym &&
			k.version == prefix.version &&
			SegmentsHasPrefix(k.segments, prefix.segments)
	}
	return k.stable.HasPrefix(prefix.stable)
}

// HasStrictPrefix reports whether prefix is a strict ancestor of this key.
func (k StructuralKey) HasStrictPrefix(prefix StructuralKey) bool {
	remainder, ok := k.RemainderAfterPrefix(prefix)
	return ok && len(remainder) > 0
}

// RemainderAfterPrefix returns the member/index suffix below prefix.
func (k StructuralKey) RemainderAfterPrefix(prefix StructuralKey) ([]segment.Segment, bool) {
	if k.local || prefix.local {
		if !k.local ||
			!prefix.local ||
			k.sym != prefix.sym ||
			k.version != prefix.version ||
			!SegmentsHasPrefix(k.segments, prefix.segments) {
			return nil, false
		}
		return cloneSegments(k.segments[len(prefix.segments):]), true
	}
	return k.stable.RemainderAfterPrefix(prefix.stable)
}

// Append returns a descendant key reached by appending suffix segments.
func (k StructuralKey) Append(suffix []segment.Segment) (StructuralKey, bool) {
	if len(suffix) == 0 {
		return k, k.PathKey() != ""
	}
	if k.local {
		next := make([]segment.Segment, len(k.segments)+len(suffix))
		copy(next, k.segments)
		copy(next[len(k.segments):], suffix)
		return StructuralKey{
			local:    true,
			sym:      k.sym,
			version:  k.version,
			segments: next,
		}, true
	}
	stable, ok := k.stable.Append(suffix)
	if !ok {
		return StructuralKey{}, false
	}
	return StructuralKey{stable: stable}, true
}

// SameKind reports whether keys are in the same structural address space.
func (k StructuralKey) SameKind(other StructuralKey) bool {
	return k.local == other.local
}

// RebasePathKey rewrites pathKey from one structural prefix to another in the
// same address space, preserving segment boundaries.
func RebasePathKey(pathKey, from, to pathdom.PathKey) (pathdom.PathKey, bool) {
	parsedPath, ok := StructuralKeyFromPathKey(pathKey)
	if !ok {
		return "", false
	}
	parsedFrom, ok := StructuralKeyFromPathKey(from)
	if !ok {
		return "", false
	}
	parsedTo, ok := StructuralKeyFromPathKey(to)
	if !ok || !parsedFrom.SameKind(parsedTo) {
		return "", false
	}
	remainder, ok := parsedPath.RemainderAfterPrefix(parsedFrom)
	if !ok {
		return "", false
	}
	rebased, ok := parsedTo.Append(remainder)
	if !ok {
		return "", false
	}
	key := rebased.PathKey()
	if key == "" || key == pathKey {
		return "", false
	}
	return key, true
}
