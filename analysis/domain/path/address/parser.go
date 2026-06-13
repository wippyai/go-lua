package address

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keycodec"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ParseResolverPath decodes the canonical verbose resolver spelling.
//
// Accepted forms are symN, symN@V, and either with a valid segment suffix.
// The suffix is returned exactly as spelled so callers can preserve the
// current point-local spelling while still validating segment boundaries.
func ParseResolverPath(key pathdom.PathKey) (sym symbol.ID, version int, suffix string, ok bool) {
	s := string(key)
	if len(s) < 4 || s[0] != 's' || s[1] != 'y' || s[2] != 'm' {
		return 0, 0, "", false
	}
	i := 3
	for i < len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			break
		}
		i++
	}
	n, parsed := keycodec.ParseUnsignedDecimal(s[3:i])
	if !parsed || n == 0 {
		return 0, 0, "", false
	}
	suffixStart := i
	if i < len(s) && s[i] == '@' {
		parsedVersion, next, parsedOK := keycodec.ParsePositiveIntAfterAt(s, i+1)
		if !parsedOK {
			return 0, 0, "", false
		}
		version = parsedVersion
		suffixStart = next
	}
	suffix = s[suffixStart:]
	if suffix != "" {
		switch suffix[0] {
		case '.', '[':
		default:
			return 0, 0, "", false
		}
		if !segment.ValidFormattedSegments(suffix) {
			return 0, 0, "", false
		}
	}
	return symbol.ID(n), version, suffix, true
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
