package path

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/keycodec"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ParseKey is the sole parser for the in-memory PathKey grammar.
//
// A key is either:
//
//	sym<symbol>[@<version>]<segments>
//	n<byte-length>:<named-root><segments>
//
// Symbols and positive SSA versions use canonical decimal spelling. Named
// roots are always length-prefixed, including placeholders and return slots,
// so root text can never be mistaken for segment syntax. n0: is reserved for
// a non-empty rootless suffix carried by address.SuffixKey.
func ParseKey(key PathKey) (Path, bool) {
	text := string(key)
	if text == "" {
		return Path{}, false
	}
	if sym, suffixStart, ok := keycodec.ParsePrefixedNonZeroDecimal(text, "sym"); ok {
		version := 0
		if suffixStart < len(text) && text[suffixStart] == '@' {
			parsed, next, valid := keycodec.ParsePositiveIntAfterAt(text, suffixStart+1)
			if !valid {
				return Path{}, false
			}
			version = parsed
			suffixStart = next
		}
		segments, ok := segment.ParseFormattedSegments(text[suffixStart:])
		if !ok {
			return Path{}, false
		}
		path := Path{Symbol: symbol.ID(sym), Version: version, Segments: segments}
		return path, FormatKey(path) == key
	}

	root, suffix, ok := parseLengthPrefixedRoot(text)
	if !ok {
		return Path{}, false
	}
	segments, ok := segment.ParseFormattedSegments(suffix)
	if !ok || root == "" && len(segments) == 0 {
		return Path{}, false
	}
	path := Path{Root: root, Segments: segments}
	return path, FormatKey(path) == key
}

// FormatKey is the sole formatter for the in-memory PathKey grammar.
func FormatKey(path Path) PathKey {
	if !validKeySegments(path.Segments) {
		return ""
	}
	if path.Symbol != 0 {
		if path.Version < 0 {
			return ""
		}
		var b strings.Builder
		b.Grow(3 + unsignedDecimalLen(uint64(path.Symbol)) + 1 + signedDecimalLen(path.Version) + segment.FormattedLen(path.Segments))
		b.WriteString("sym")
		writeUint(&b, uint64(path.Symbol))
		if path.Version > 0 {
			b.WriteByte('@')
			writeInt(&b, path.Version)
		}
		segment.WriteFormattedSegments(&b, path.Segments)
		return PathKey(b.String())
	}
	if path.Version != 0 {
		return ""
	}
	if path.Root == "" && len(path.Segments) == 0 {
		return ""
	}

	var b strings.Builder
	b.Grow(2 + unsignedDecimalLen(uint64(len(path.Root))) + len(path.Root) + segment.FormattedLen(path.Segments))
	b.WriteByte('n')
	b.WriteString(strconv.Itoa(len(path.Root)))
	b.WriteByte(':')
	b.WriteString(path.Root)
	segment.WriteFormattedSegments(&b, path.Segments)
	return PathKey(b.String())
}

func validKeySegments(segments []segment.Segment) bool {
	for _, item := range segments {
		switch item.Kind {
		case segment.SegmentField:
			if item.Name == "" || strings.ContainsAny(item.Name, ".[") {
				return false
			}
		case segment.SegmentIndexString, segment.SegmentIndexInt:
		default:
			return false
		}
	}
	return true
}

func parseLengthPrefixedRoot(text string) (root, suffix string, ok bool) {
	if len(text) < 3 || text[0] != 'n' {
		return "", "", false
	}
	colon := 1
	for colon < len(text) && keycodec.IsDecimalDigit(text[colon]) {
		colon++
	}
	length, valid := keycodec.ParseUnsignedDecimal(text[1:colon])
	if !valid || colon >= len(text) || text[colon] != ':' || length > uint64(len(text)-colon-1) {
		return "", "", false
	}
	rootStart := colon + 1
	rootEnd := rootStart + int(length)
	return text[rootStart:rootEnd], text[rootEnd:], true
}
