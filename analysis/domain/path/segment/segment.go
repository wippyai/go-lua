// Package segment owns the leaf vocabulary for path suffix segments.
package segment

import (
	"strconv"
	"strings"
	"sync"
)

var formattedSegmentsCache sync.Map

// SegmentKind describes how a path accesses a nested value.
type SegmentKind uint8

const (
	SegmentField SegmentKind = iota
	SegmentIndexString
	SegmentIndexInt
)

// Segment identifies a single field or index access step on a path.
// For field access (.name), Kind is SegmentField and Name holds the field name.
// For string index (["key"]), Kind is SegmentIndexString and Name holds the key.
// For integer index ([1]), Kind is SegmentIndexInt and Index holds the value.
type Segment struct {
	Kind  SegmentKind
	Name  string
	Index int
}

// DirectFieldName reports whether segs is exactly one dot-field segment and
// returns that field name. It centralizes the common "plain object field"
// suffix check used by object-literal projections.
func DirectFieldName(segs []Segment) (string, bool) {
	if len(segs) != 1 {
		return "", false
	}
	seg := segs[0]
	if seg.Kind != SegmentField {
		return "", false
	}
	return seg.Name, true
}

// FormatSegments converts path segments to a canonical suffix string.
// This is the single canonical implementation for segment serialization.
// Format: .field for SegmentField, ["key"] for SegmentIndexString, [123] for SegmentIndexInt.
func FormatSegments(segs []Segment) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(FormattedLen(segs))
	WriteFormattedSegments(&b, segs)
	return b.String()
}

// FormattedLen returns the exact byte length FormatSegments will emit.
func FormattedLen(segs []Segment) int {
	total := 0
	for _, seg := range segs {
		switch seg.Kind {
		case SegmentField:
			total += 1 + len(seg.Name)
		case SegmentIndexString:
			total += quotedPathIndexLen(seg.Name)
		case SegmentIndexInt:
			total += 2 + signedDecimalLen(seg.Index)
		}
	}
	return total
}

// WriteFormattedSegments appends the canonical FormatSegments suffix to b.
func WriteFormattedSegments(b *strings.Builder, segs []Segment) {
	for _, seg := range segs {
		switch seg.Kind {
		case SegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case SegmentIndexString:
			writeQuotedPathIndex(b, seg.Name)
		case SegmentIndexInt:
			b.WriteByte('[')
			writeInt(b, seg.Index)
			b.WriteByte(']')
		}
	}
}

// InternFormattedSegments parses the canonical suffix emitted by FormatSegments.
// The returned slice is interned and immutable; callers that expose or mutate
// segment slices must use ParseFormattedSegments instead.
func InternFormattedSegments(suffix string) ([]Segment, bool) {
	if suffix == "" {
		return nil, true
	}
	if cached, ok := formattedSegmentsCache.Load(suffix); ok {
		return cached.([]Segment), true
	}
	segs, ok := parseFormattedSegmentsSlow(suffix)
	if !ok {
		return nil, false
	}
	if cached, loaded := formattedSegmentsCache.LoadOrStore(suffix, segs); loaded {
		return cached.([]Segment), true
	}
	return segs, true
}

// ParseFormattedSegments parses the canonical suffix emitted by FormatSegments
// and returns a defensive copy for public consumers.
func ParseFormattedSegments(suffix string) ([]Segment, bool) {
	segs, ok := InternFormattedSegments(suffix)
	if !ok {
		return nil, false
	}
	return cloneFormattedSegments(segs), true
}

// ValidFormattedSegments reports whether suffix is a canonical FormatSegments
// suffix without projecting segment values.
func ValidFormattedSegments(suffix string) bool {
	_, ok := InternFormattedSegments(suffix)
	return ok
}

func parseFormattedSegmentsSlow(suffix string) ([]Segment, bool) {
	var segs []Segment
	for len(suffix) > 0 {
		switch suffix[0] {
		case '.':
			end := 1
			for end < len(suffix) && suffix[end] != '.' && suffix[end] != '[' {
				end++
			}
			if end == 1 {
				return nil, false
			}
			segs = append(segs, Segment{Kind: SegmentField, Name: suffix[1:end]})
			suffix = suffix[end:]
		case '[':
			if len(suffix) < 3 {
				return nil, false
			}
			if suffix[1] == '"' {
				end, ok := formattedQuotedSegmentEnd(suffix)
				if !ok || end+1 >= len(suffix) || suffix[end+1] != ']' {
					return nil, false
				}
				name, ok := unquoteFormattedSegment(suffix[2:end])
				if !ok {
					return nil, false
				}
				segs = append(segs, Segment{Kind: SegmentIndexString, Name: name})
				suffix = suffix[end+2:]
				continue
			}
			end := 1
			if suffix[end] == '-' {
				end++
			}
			digitStart := end
			for end < len(suffix) && suffix[end] >= '0' && suffix[end] <= '9' {
				end++
			}
			if end == digitStart || end >= len(suffix) || suffix[end] != ']' {
				return nil, false
			}
			v, err := strconv.Atoi(suffix[1:end])
			if err != nil {
				return nil, false
			}
			segs = append(segs, Segment{Kind: SegmentIndexInt, Index: v})
			suffix = suffix[end+1:]
		default:
			return nil, false
		}
	}
	return segs, true
}

func formattedQuotedSegmentEnd(s string) (int, bool) {
	for i := 2; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
			if i >= len(s) || (s[i] != '\\' && s[i] != '"') {
				return 0, false
			}
		case '"':
			return i, true
		}
	}
	return 0, false
}

func unquoteFormattedSegment(s string) (string, bool) {
	if !strings.ContainsAny(s, `\"`) {
		return s, true
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) || (s[i] != '\\' && s[i] != '"') {
			return "", false
		}
		b.WriteByte(s[i])
	}
	return b.String(), true
}

func cloneFormattedSegments(segs []Segment) []Segment {
	if len(segs) == 0 {
		return nil
	}
	return append([]Segment(nil), segs...)
}

func writeQuotedPathIndex(b *strings.Builder, key string) {
	b.WriteString("[\"")
	for i := 0; i < len(key); i++ {
		switch key[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(key[i])
		default:
			b.WriteByte(key[i])
		}
	}
	b.WriteString("\"]")
}

func quotedPathIndexLen(key string) int {
	n := 4 + len(key)
	for i := 0; i < len(key); i++ {
		switch key[i] {
		case '\\', '"':
			n++
		}
	}
	return n
}

func writeInt(b *strings.Builder, n int) {
	var buf [20]byte
	u := uint64(n)
	if n < 0 {
		b.WriteByte('-')
		u = uint64(-(n + 1)) + 1
	}
	out := buf[:0]
	out = appendUint(out, u)
	b.Write(out)
}

func signedDecimalLen(n int) int {
	if n < 0 {
		return 1 + unsignedDecimalLen(uint64(-(n+1))+1)
	}
	return unsignedDecimalLen(uint64(n))
}

func unsignedDecimalLen(n uint64) int {
	digits := 1
	for n >= 10 {
		n /= 10
		digits++
	}
	return digits
}

func appendUint(out []byte, n uint64) []byte {
	var rev [20]byte
	i := 0
	for {
		rev[i] = byte('0' + n%10)
		i++
		n /= 10
		if n == 0 {
			break
		}
	}
	for i > 0 {
		i--
		out = append(out, rev[i])
	}
	return out
}
