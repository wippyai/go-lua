package address

import (
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/keycodec"
)

type parsedRootSuffix struct {
	root     string
	suffix   string
	segments []segment.Segment
}

func parseStableSymbolRootSuffix(key pathdom.PathKey) (uint64, parsedRootSuffix, bool) {
	s := string(key)
	n, end, ok := keycodec.ParsePrefixedNonZeroDecimal(s, "s")
	if !ok {
		return 0, parsedRootSuffix{}, false
	}
	parsed, ok := parseSuffix(s[:end], s[end:])
	if !ok {
		return 0, parsedRootSuffix{}, false
	}
	return n, parsed, true
}

func parseResolverRootSuffix(key pathdom.PathKey) (uint64, int, parsedRootSuffix, bool) {
	s := string(key)
	n, suffixStart, ok := keycodec.ParsePrefixedNonZeroDecimal(s, "sym")
	if !ok {
		return 0, 0, parsedRootSuffix{}, false
	}
	version := 0
	if suffixStart < len(s) && s[suffixStart] == '@' {
		parsedVersion, next, parsedOK := keycodec.ParsePositiveIntAfterAt(s, suffixStart+1)
		if !parsedOK {
			return 0, 0, parsedRootSuffix{}, false
		}
		version = parsedVersion
		suffixStart = next
	}
	parsed, ok := parseSuffix(s[:suffixStart], s[suffixStart:])
	if !ok {
		return 0, 0, parsedRootSuffix{}, false
	}
	return n, version, parsed, true
}

func parseEncodedNamedRootSuffix(key string) (parsedRootSuffix, bool) {
	if len(key) < 3 || key[0] != 'n' {
		return parsedRootSuffix{}, false
	}
	i := 1
	if key[i] == '0' {
		return parsedRootSuffix{}, false
	}
	length := 0
	for i < len(key) {
		ch := key[i]
		if ch < '0' || ch > '9' {
			break
		}
		digit := int(ch - '0')
		if length > (int(^uint(0)>>1)-digit)/10 {
			return parsedRootSuffix{}, false
		}
		length = length*10 + digit
		i++
	}
	if i == 1 || i >= len(key) || key[i] != ':' || length == 0 {
		return parsedRootSuffix{}, false
	}
	rootStart := i + 1
	rootEnd := rootStart + length
	if rootEnd > len(key) {
		return parsedRootSuffix{}, false
	}
	return parseSuffix(key[rootStart:rootEnd], key[rootEnd:])
}

func parsePlainNamedRootSuffix(key pathdom.PathKey) (parsedRootSuffix, bool) {
	s := string(key)
	if s == "" {
		return parsedRootSuffix{}, false
	}
	if s[0] == '$' {
		end := 1
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == 1 {
			return parsedRootSuffix{}, false
		}
		return parseSuffix(s[:end], s[end:])
	}
	if strings.HasPrefix(s, "ret[") {
		end := 4
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == 4 || end >= len(s) || s[end] != ']' {
			return parsedRootSuffix{}, false
		}
		end++
		return parseSuffix(s[:end], s[end:])
	}
	end := 0
	for end < len(s) && s[end] != '.' && s[end] != '[' {
		end++
	}
	if end == 0 {
		return parsedRootSuffix{}, false
	}
	return parseSuffix(s[:end], s[end:])
}

func parseSuffix(root, suffix string) (parsedRootSuffix, bool) {
	segments, ok := segment.InternFormattedSegments(suffix)
	if !ok {
		return parsedRootSuffix{}, false
	}
	return parsedRootSuffix{root: root, suffix: suffix, segments: segments}, true
}
