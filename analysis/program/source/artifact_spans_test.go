package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactSpansRenderThroughTheSourceOwner(t *testing.T) {
	input, index := sourceFixture(1)
	component := finalizeSource(t, input, index)
	view := component.View()
	term := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	span, ok := view.Identity().Span(term)
	if !ok || span.File != input.Name || span.StartLine != 1 || span.StartCol != 1 {
		t.Fatalf("Span(%v) = %#v/%v, want fixture.lua:1:1", term, span, ok)
	}
	coordinate, ok := CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	if !ok {
		t.Fatal("source span did not round-trip to a coordinate")
	}
	if rendered, ok := view.Identity().Render(coordinate); !ok || rendered != span {
		t.Fatalf("Render(%#v) = %#v/%v, want %#v/true", coordinate, rendered, ok, span)
	}
	if _, ok := view.Identity().Span(keyspace.MakeTerm(keyspace.FamilyNil, 99)); ok {
		t.Fatal("Span accepted an unavailable ordinal")
	}
}
