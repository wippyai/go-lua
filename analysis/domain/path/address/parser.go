package address

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ParseResolverPath decodes the canonical verbose resolver spelling.
//
// Accepted forms are symN, symN@V, and either with a valid segment suffix.
// The suffix is returned exactly as spelled so callers can preserve the
// current point-local spelling while still validating segment boundaries.
func ParseResolverPath(key pathdom.PathKey) (sym symbol.ID, version int, suffix string, ok bool) {
	path, ok := pathdom.ParseKey(key)
	if !ok || path.Symbol == 0 {
		return 0, 0, "", false
	}
	return path.Symbol, path.Version, segment.FormatSegments(path.Segments), true
}

// LocalPathFromKey parses a point-local resolver key into a path. It accepts
// only versioned resolver keys, because unversioned resolver roots are not
// point-local state identities.
func LocalPathFromKey(key pathdom.PathKey) (pathdom.Path, bool) {
	path, ok := pathdom.ParseKey(key)
	if !ok || path.Symbol == 0 || path.Version <= 0 {
		return pathdom.Path{}, false
	}
	return path, true
}

// LocalKeyForVersion formats a point-local key for an explicit SSA version.
func LocalKeyForVersion(sym symbol.ID, version int, segments []segment.Segment) (LocalKey, bool) {
	if sym == 0 || version <= 0 {
		return LocalKey{}, false
	}
	return LocalKey{key: pathdom.FormatKey(pathdom.Path{Symbol: sym, Version: version, Segments: segments})}, true
}

// SegmentsHasPrefix reports whether prefix is this segment list or an ancestor.
func SegmentsHasPrefix(segments, prefix []segment.Segment) bool {
	if len(prefix) > len(segments) {
		return false
	}
	for i := range prefix {
		if !segmentsEquivalent(segments[i], prefix[i]) {
			return false
		}
	}
	return true
}

// SegmentsHasStrictPrefix reports whether prefix is a strict ancestor.
func SegmentsHasStrictPrefix(segments, prefix []segment.Segment) bool {
	return len(prefix) < len(segments) && SegmentsHasPrefix(segments, prefix)
}

func segmentsEquivalent(a, b segment.Segment) bool {
	if a == b {
		return true
	}
	if (a.Kind == segment.SegmentField || a.Kind == segment.SegmentIndexString) &&
		(b.Kind == segment.SegmentField || b.Kind == segment.SegmentIndexString) {
		return a.Name == b.Name
	}
	return false
}

// VersionedRootString returns the canonical verbose resolver root.
func VersionedRootString(sym symbol.ID, version int) string {
	if sym == 0 || version <= 0 {
		return ""
	}
	return "sym" + strconv.FormatUint(uint64(sym), 10) + "@" + strconv.Itoa(version)
}

// RebaseLocalPathKeyToContext rebases a versioned local key to the version of a
// context key with the same symbol root.
func RebaseLocalPathKeyToContext(pathKey, contextKey pathdom.PathKey) (pathdom.PathKey, bool) {
	if pathKey == "" || contextKey == "" {
		return "", false
	}
	if pathKey == contextKey {
		return pathKey, true
	}
	from, ok := LocalPathFromKey(pathKey)
	if !ok {
		return "", false
	}
	to, ok := LocalPathFromKey(contextKey)
	if !ok || from.Symbol == 0 || to.Symbol == 0 || from.Symbol != to.Symbol {
		return "", false
	}
	fromRoot, ok := LocalKeyForVersion(from.Symbol, from.Version, nil)
	if !ok {
		return "", false
	}
	toRoot, ok := LocalKeyForVersion(to.Symbol, to.Version, nil)
	if !ok {
		return "", false
	}
	return RebasePathKey(pathKey, fromRoot.PathKey(), toRoot.PathKey())
}

// PlaceholderPathFromKey parses a canonical placeholder-root key such as
// n2:$0.field into a placeholder path.
func PlaceholderPathFromKey(key pathdom.PathKey) (pathdom.Path, bool) {
	path, ok := pathdom.ParseKey(key)
	if !ok || path.Symbol != 0 {
		return pathdom.Path{}, false
	}
	index := pathdom.PlaceholderIndexFromString(path.Root)
	if index < 0 {
		return pathdom.Path{}, false
	}
	base := pathdom.NewPlaceholder(index)
	if base.Root != path.Root {
		return pathdom.Path{}, false
	}
	base.Segments = path.Segments
	return base, true
}
