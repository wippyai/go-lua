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
	n, version, parsed, ok := parseResolverRootSuffix(key)
	if !ok {
		return 0, 0, "", false
	}
	return symbol.ID(n), version, parsed.suffix, true
}

// LocalPathFromKey parses a point-local resolver key into a path. It accepts
// only versioned resolver keys, because unversioned resolver roots are not
// point-local state identities.
func LocalPathFromKey(key pathdom.PathKey) (pathdom.Path, bool) {
	n, version, parsed, ok := parseResolverRootSuffix(key)
	if !ok || version <= 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{
		Symbol:   symbol.ID(n),
		Version:  version,
		Segments: cloneSegments(parsed.segments),
	}, true
}

// LocalKeyFromPathKey validates and narrows a PathKey to the point-local
// resolver key space used by flow-state facts.
func LocalKeyFromPathKey(key pathdom.PathKey) (LocalKey, bool) {
	if _, ok := LocalPathFromKey(key); !ok {
		return "", false
	}
	return LocalKey(key), true
}

// LocalKeyForVersion formats a point-local key for an explicit SSA version.
func LocalKeyForVersion(sym symbol.ID, version int, segments []segment.Segment) (LocalKey, bool) {
	if sym == 0 || version <= 0 {
		return "", false
	}
	path := pathdom.Path{Symbol: sym, Version: version, Segments: cloneSegments(segments)}
	return LocalKey(path.Key()), true
}

// SegmentsHasPrefix reports whether prefix is this segment list or an ancestor.
func SegmentsHasPrefix(segments, prefix []segment.Segment) bool {
	if len(prefix) > len(segments) {
		return false
	}
	for i := range prefix {
		if segments[i] != prefix[i] {
			return false
		}
	}
	return true
}

// SegmentsHasStrictPrefix reports whether prefix is a strict ancestor.
func SegmentsHasStrictPrefix(segments, prefix []segment.Segment) bool {
	return len(prefix) < len(segments) && SegmentsHasPrefix(segments, prefix)
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

// PlaceholderPathFromKey parses a placeholder-root key such as $0.field into a
// placeholder path.
func PlaceholderPathFromKey(key pathdom.PathKey) (pathdom.Path, bool) {
	parsed, ok := parsePlainNamedRootSuffix(key)
	if !ok {
		return pathdom.Path{}, false
	}
	index := pathdom.PlaceholderIndexFromString(parsed.root)
	if index < 0 {
		return pathdom.Path{}, false
	}
	base := pathdom.NewPlaceholder(index)
	if base.Root != parsed.root {
		return pathdom.Path{}, false
	}
	base.Segments = cloneSegments(parsed.segments)
	return base, true
}
