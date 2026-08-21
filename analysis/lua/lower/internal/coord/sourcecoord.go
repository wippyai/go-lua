// Package coord converts compiler's signed source positions into the
// canonical Source coordinate type. It contains no lowering or semantic
// authority; callers decide whether an invalid result is fatal.
package coord

import (
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Build validates a signed source span before converting it to Source's
// uint32 coordinate representation. A zero span is the explicit no-position
// value. Otherwise the start must be positive, and the end is either absent
// as exactly (0, 0) or a non-reversed positive coordinate.
func Build(file string, startLine, startCol, endLine, endCol int) (source.Span, bool) {
	if !fitsUint32(startLine) || !fitsUint32(startCol) || !fitsUint32(endLine) || !fitsUint32(endCol) {
		return source.Span{}, false
	}
	if _, ok := source.CoordinateFromParts(uint32(startLine), uint32(startCol), uint32(endLine), uint32(endCol)); !ok {
		return source.Span{}, false
	}
	return source.Span{
		File:      file,
		StartLine: uint32(startLine),
		StartCol:  uint32(startCol),
		EndLine:   uint32(endLine),
		EndCol:    uint32(endCol),
	}, true
}

// Invalid returns a non-zero malformed span suitable for fixed-signature
// writers. Collector validation rejects it, unlike the lawful all-zero span.
func Invalid(file string) source.Span {
	return source.Span{File: file, StartCol: 1}
}

func fitsUint32(value int) bool {
	return value >= 0 && uint64(value) <= uint64(^uint32(0))
}

// Span converts one AST holder's position into the canonical Source span,
// applying the shared nil/malformed fallback: a nil holder is the explicit
// no-position value, and a non-nil holder that fails Build yields Invalid.
func Span(file string, holder ast.PositionHolder) source.Span {
	if holder == nil {
		return source.Span{File: file}
	}
	span, ok := Build(file, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return Invalid(file)
	}
	return span
}

// PositionSpan converts one AST Position value into the canonical Source
// span. Position is a value type, not a PositionHolder: the all-zero value
// is the explicit no-position value, and any other invalid Position yields
// Invalid.
func PositionSpan(file string, position ast.Position) source.Span {
	if !position.Valid() {
		if position.Line == 0 && position.Column == 0 && position.EndLine == 0 && position.EndColumn == 0 {
			return source.Span{File: file}
		}
		return Invalid(file)
	}
	span, ok := Build(file, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return Invalid(file)
	}
	return span
}
