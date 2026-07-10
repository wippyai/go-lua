package keyspace

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/keycodec"
)

// symbolPathKeyParses reports whether raw is a valid stable symbol key s<id><suffix>.
func symbolPathKeyParses(raw string) bool {
	n, end, ok := keycodec.ParsePrefixedNonZeroDecimal(raw, "s")
	if !ok || n == 0 {
		return false
	}
	_, ok = segment.InternFormattedSegments(raw[end:])
	return ok
}

// resolverPathKeyParses reports whether raw is a valid resolver key
// sym<id>[@<ver>]<suffix>.
func resolverPathKeyParses(raw string) bool {
	n, suffixStart, ok := keycodec.ParsePrefixedNonZeroDecimal(raw, "sym")
	if !ok || n == 0 {
		return false
	}
	if suffixStart < len(raw) && raw[suffixStart] == '@' {
		ver, next, vok := keycodec.ParsePositiveIntAfterAt(raw, suffixStart+1)
		if !vok || ver == 0 {
			return false
		}
		suffixStart = next
	}
	_, ok = segment.InternFormattedSegments(raw[suffixStart:])
	return ok
}

// parsePlainNamedRoot mirrors address.parsePlainNamedRootSuffix, returning the
// root spelling and its parsed segments.
func parsePlainNamedRoot(s string) (string, []segment.Segment, bool) {
	if s == "" {
		return "", nil, false
	}
	if s[0] == '$' {
		end := 1
		for end < len(s) && keycodec.IsDecimalDigit(s[end]) {
			end++
		}
		if end == 1 {
			return "", nil, false
		}
		return parsePlainRootSuffix(s, end)
	}
	if strings.HasPrefix(s, "ret[") {
		end := 4
		for end < len(s) && keycodec.IsDecimalDigit(s[end]) {
			end++
		}
		if end == 4 || end >= len(s) || s[end] != ']' {
			return "", nil, false
		}
		end++
		return parsePlainRootSuffix(s, end)
	}
	end := 0
	for end < len(s) && s[end] != '.' && s[end] != '[' {
		end++
	}
	if end == 0 {
		return "", nil, false
	}
	return parsePlainRootSuffix(s, end)
}

func parsePlainRootSuffix(s string, rootEnd int) (string, []segment.Segment, bool) {
	segments, ok := segment.InternFormattedSegments(s[rootEnd:])
	if !ok {
		return "", nil, false
	}
	return s[:rootEnd], segments, true
}
