// Package provenance owns the one source-coordinate law shared by exact
// parser-to-Program witnesses.
package provenance

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Exact requires a canonical Program Term to retain the complete parser-owned
// source coordinate of one AST occurrence. The coordinate includes both ends
// of the occurrence, so it remains an ordering witness as well as a location:
// two distinct authored occurrences cannot be silently conflated behind a
// marker/base embedding.
//
// Callers must already have selected term through its typed Program relation.
// Exact intentionally does not search a generic Term plane or infer a source
// relation from coincident spans.
func Exact(identity source.Identity, term keyspace.Term, occurrence ast.PositionHolder, file string) error {
	if term == 0 || occurrence == nil {
		return fmt.Errorf("provenance: missing Source identity, Term, or source occurrence")
	}
	want := source.Span{
		File:      file,
		StartLine: uint32(occurrence.Line()),
		StartCol:  uint32(occurrence.Column()),
		EndLine:   uint32(occurrence.LastLine()),
		EndCol:    uint32(occurrence.LastColumn()),
	}
	got, ok := identity.Span(term)
	if !ok || got != want {
		return fmt.Errorf("provenance: Source span = %#v/%v, want parser source order %#v", got, ok, want)
	}
	return nil
}
