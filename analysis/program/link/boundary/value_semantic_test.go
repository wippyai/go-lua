package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestBoundaryMountedSemanticDirectoryReturnsIssuedValues(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	p := boundaryProgram(t)
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: p}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	if !component.Values().VisitMountedSemantics(func(moduleID, occurrenceID identity.ContentID, value Value) bool {
		if !moduleID.Available() || !occurrenceID.Available() {
			t.Fatal("semantic directory returned unavailable identity")
		}
		if _, ok := component.Values().ID(value); !ok {
			t.Fatal("semantic directory returned an unissued Value")
		}
		seen++
		return true
	}) {
		t.Fatal("semantic directory traversal aborted")
	}
	if seen == 0 {
		t.Fatal("mounted Program published no semantic Value rows")
	}
}

// assertMountedSpanSemantic is the law every computation, Return, and
// IndexRead pass in sealValueSemanticIDs now states: a family row is
// admitted only when its own Span already resolves to a Boundary ordinal
// AND the span-directory pass published that exact context at that exact
// ordinal. sealValueSemanticIDs itself cannot express the omitted-context
// defect this proves: the span-directory pass and every later family pass
// draw the same context from the same table.spans entry in the same
// construction, so a real seal can never omit one while carrying the other.
// The defect is reproduced directly against the extracted assertion instead,
// using a genuine Span pulled from a real Program rather than a fabricated
// one, with only the surrounding valueTable state assembled by hand.
func TestBoundarySpanSemanticAssertionRefusesUnpublishedContext(t *testing.T) {
	p := boundaryProgram(t)
	binaries := p.Flow().Authored().Operators().Binaries()
	if binaries.Count() == 0 {
		t.Fatal("fixture declares no binary operator")
	}
	term, termOK := binaries.At(0)
	if !termOK {
		t.Fatal("binary row publishes no term")
	}
	span, spanOK := p.Span(term)
	if !spanOK || !p.OwnsSpan(span) || !span.ContextID().Available() {
		t.Fatal("binary term owns no span identity")
	}

	table := &valueTable{
		rows:     []valueRow{{shard: 1, term: term}},
		spans:    map[valueSpanKey]uint32{{mount: 1, context: span.ContextID()}: 0},
		semantic: map[valueSemanticKey]uint32{},
	}
	if err := assertMountedSpanSemantic(table, 1, "Binary", span); err == nil {
		t.Fatal("assertion admitted a Span context the semantic directory never published")
	}

	table.semantic[valueSemanticKey{mount: 1, id: span.ContextID()}] = 0
	if err := assertMountedSpanSemantic(table, 1, "Binary", span); err != nil {
		t.Fatalf("assertion refused a Span context the semantic directory holds at the exact ordinal: %v", err)
	}

	table.semantic[valueSemanticKey{mount: 1, id: span.ContextID()}] = 1
	if err := assertMountedSpanSemantic(table, 1, "Binary", span); err == nil {
		t.Fatal("assertion admitted a Span context published at a different ordinal")
	}
}
