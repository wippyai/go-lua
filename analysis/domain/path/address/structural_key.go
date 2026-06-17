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

// FieldCanonicalPathKey returns the equivalent key whose static string-index
// segments use field spelling. It is an auxiliary analysis key: callers should
// keep the original syntax key too, then use this key to make a.b and a["b"]
// share facts.
func FieldCanonicalPathKey(pathKey pathdom.PathKey) (pathdom.PathKey, bool) {
	parsed, ok := StructuralKeyFromPathKey(pathKey)
	if !ok {
		return "", false
	}
	if !parsed.local {
		segments, changed := FieldCanonicalSegments(parsed.stable.suffix.segments)
		if !changed {
			return "", false
		}
		parsed.stable.suffix = suffixOfOwnedSegments(segments)
		key := parsed.PathKey()
		return key, key != "" && key != pathKey
	}
	segments, changed := FieldCanonicalSegments(parsed.segments)
	if !changed {
		return "", false
	}
	parsed.segments = segments
	key := parsed.PathKey()
	return key, key != "" && key != pathKey
}

// FieldCanonicalSegments rewrites static string-index segments to field
// segments. The returned slice is a defensive copy when a rewrite happens.
func FieldCanonicalSegments(segments []segment.Segment) ([]segment.Segment, bool) {
	var out []segment.Segment
	changed := false
	for i, seg := range segments {
		if seg.Kind != segment.SegmentIndexString {
			continue
		}
		if !changed {
			out = cloneSegments(segments)
			changed = true
		}
		out[i] = segment.Segment{Kind: segment.SegmentField, Name: seg.Name}
	}
	if !changed {
		return segments, false
	}
	return out, true
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
