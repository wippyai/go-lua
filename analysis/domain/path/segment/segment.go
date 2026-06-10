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

// FormatSegments converts path segments to a canonical suffix string.
// This is the single canonical implementation for segment serialization.
// Format: .field for SegmentField, ["key"] for SegmentIndexString, [123] for SegmentIndexInt.
func FormatSegments(segs []Segment) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range segs {
		switch seg.Kind {
		case SegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case SegmentIndexString:
			writeQuotedPathIndex(&b, seg.Name)
		case SegmentIndexInt:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(']')
		}
	}
	return b.String()
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
