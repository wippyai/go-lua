package pathkey

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DeleteSubtree removes finite path refinements at prefix and any descendant.
// ok is false when prefix is not a recognized structural path-key spelling.
func DeleteSubtree(
	in map[pathdom.PathKey]product.Value,
	prefix pathdom.PathKey,
) (out map[pathdom.PathKey]product.Value, changed bool, ok bool) {
	parsedPrefix, ok := parse(prefix)
	if !ok {
		return in, false, false
	}
	out, changed = deleteMatching(in, func(candidate parsedKey) bool {
		return candidate.hasPrefix(parsedPrefix)
	})
	return out, changed, true
}

// DeleteDescendants removes finite path refinements below prefix while keeping
// the exact prefix refinement. ok is false when prefix is not a recognized
// structural path-key spelling.
func DeleteDescendants(
	in map[pathdom.PathKey]product.Value,
	prefix pathdom.PathKey,
) (out map[pathdom.PathKey]product.Value, changed bool, ok bool) {
	parsedPrefix, ok := parse(prefix)
	if !ok {
		return in, false, false
	}
	out, changed = deleteMatching(in, func(candidate parsedKey) bool {
		return candidate.hasStrictPrefix(parsedPrefix)
	})
	return out, changed, true
}

type parsedKey struct {
	versioned bool
	sym       symbol.ID
	version   int
	segments  []segment.Segment
	stable    pathaddr.Stable
}

func parse(pathKey pathdom.PathKey) (parsedKey, bool) {
	sym, version, suffix, ok := pathaddr.ParseResolverPath(pathKey)
	if ok && version > 0 {
		segments, ok := segment.InternFormattedSegments(suffix)
		if !ok {
			return parsedKey{}, false
		}
		return parsedKey{
			versioned: true,
			sym:       sym,
			version:   version,
			segments:  segments,
		}, true
	}
	stable, ok := pathaddr.StableFromKey(pathKey)
	if !ok {
		return parsedKey{}, false
	}
	return parsedKey{stable: stable}, true
}

func deleteMatching(
	in map[pathdom.PathKey]product.Value,
	match func(parsedKey) bool,
) (map[pathdom.PathKey]product.Value, bool) {
	if len(in) == 0 {
		return in, false
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	changed := false
	for pathKey, value := range in {
		parsed, ok := parse(pathKey)
		if ok && match(parsed) {
			changed = true
			continue
		}
		out[pathKey] = value
	}
	if !changed {
		return in, false
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func (k parsedKey) hasPrefix(prefix parsedKey) bool {
	if k.versioned || prefix.versioned {
		return k.versioned &&
			prefix.versioned &&
			k.sym == prefix.sym &&
			k.version == prefix.version &&
			pathaddr.SegmentsHasPrefix(k.segments, prefix.segments)
	}
	return k.stable.HasPrefix(prefix.stable)
}

func (k parsedKey) hasStrictPrefix(prefix parsedKey) bool {
	if k.versioned || prefix.versioned {
		return k.versioned &&
			prefix.versioned &&
			k.sym == prefix.sym &&
			k.version == prefix.version &&
			pathaddr.SegmentsHasStrictPrefix(k.segments, prefix.segments)
	}
	remainder, ok := k.stable.RemainderAfterPrefix(prefix.stable)
	return ok && len(remainder) > 0
}
