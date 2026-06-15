package path

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// NewPath creates a path with the given symbol and display name.
// This is the primary constructor for resolved paths.
//
// Example:
//
//	path := NewPath(sym, "x")           // x
//	path := NewPath(sym, "x").Field("y") // x.y
func NewPath(sym symbol.ID, name string) Path {
	return Path{Root: name, Symbol: sym}
}

// Field returns a new path with a field access segment appended.
//
// Example:
//
//	path.Field("name")  // path.name
//	path.Field("a").Field("b") // path.a.b
func (p Path) Field(name string) Path {
	return p.Append(segment.Segment{Kind: segment.SegmentField, Name: name})
}

// IndexStr returns a new path with a string index segment appended.
//
// Example:
//
//	path.IndexStr("key")  // path["key"]
func (p Path) IndexStr(key string) Path {
	return p.Append(segment.Segment{Kind: segment.SegmentIndexString, Name: key})
}

// IndexInt returns a new path with an integer index segment appended.
//
// Example:
//
//	path.IndexInt(0)  // path[0]
//	path.IndexInt(1)  // path[1]
func (p Path) IndexInt(index int) Path {
	return p.Append(segment.Segment{Kind: segment.SegmentIndexInt, Index: index})
}

// Parent returns the path without its last segment.
// Returns an empty path if there are no segments.
//
// Example:
//
//	path.Field("a").Field("b").Parent() // path.a
func (p Path) Parent() Path {
	if len(p.Segments) == 0 {
		return Path{}
	}
	// Copy segments to avoid slice aliasing
	parentSegs := make([]segment.Segment, len(p.Segments)-1)
	copy(parentSegs, p.Segments[:len(p.Segments)-1])
	return Path{
		Root:     p.Root,
		Symbol:   p.Symbol,
		Version:  p.Version,
		Segments: parentSegs,
	}
}

// LastSegment returns the final segment of the path, if any.
// Returns (segment, true) if the path has segments, (zero, false) otherwise.
func (p Path) LastSegment() (segment.Segment, bool) {
	if len(p.Segments) == 0 {
		return segment.Segment{}, false
	}
	return p.Segments[len(p.Segments)-1], true
}

// DirectFieldName returns the field name for a root.field path with exactly one
// string-valued segment. It accepts both dot fields and static string indexes.
func (p Path) DirectFieldName() (string, bool) {
	if len(p.Segments) != 1 {
		return "", false
	}
	seg := p.Segments[0]
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

// DirectIntIndex returns the index for a root[integer] path with exactly one
// integer-valued segment.
func (p Path) DirectIntIndex() (int, bool) {
	if len(p.Segments) != 1 {
		return 0, false
	}
	seg := p.Segments[0]
	if seg.Kind != segment.SegmentIndexInt {
		return 0, false
	}
	return seg.Index, true
}

// Append returns a new path with the given segment appended.
// Returns an empty path if the receiver is empty.
func (p Path) Append(seg segment.Segment) Path {
	if p.IsEmpty() {
		return Path{}
	}

	next := Path{Root: p.Root, Symbol: p.Symbol, Version: p.Version}
	if len(p.Segments) > 0 {
		next.Segments = append(next.Segments, p.Segments...)
	}

	next.Segments = append(next.Segments, seg)

	return next
}
