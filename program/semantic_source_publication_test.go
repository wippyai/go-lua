package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/semanticsource"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
	"github.com/wippyai/go-lua/program/testfixture"
)

// TestSemanticSourceFragmentsCoverEveryProgramDefinition checks the direct
// owner boundary. Program is only the immutable quartet; the four child
// views are the sole source of its semantic-source contribution.
func TestSemanticSourceFragmentsCoverEveryProgramDefinition(t *testing.T) {
	p, err := testfixture.Minimal()
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	publications := programReceiptPublications(t, p)

	want := make(map[semanticsource.Token]struct{})
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatalf("canonical schema: %v", err)
	}
	for _, row := range schema.Rows() {
		switch row.Owner {
		case relations.OwnerProgramSource, relations.OwnerProgramFlow, relations.OwnerProgramStatic, relations.OwnerProgramModule:
			want[row.Definition.Token()] = struct{}{}
		}
	}
	if len(publications) != len(want) {
		t.Fatalf("direct child publication count = %d, want %d", len(publications), len(want))
	}

	seen := make(map[semanticsource.Token]struct{}, len(publications))
	for _, publication := range publications {
		token := publication.Definition().Token()
		if _, ok := want[token]; !ok {
			t.Fatalf("unexpected direct child publication: %v", token)
		}
		if _, duplicate := seen[token]; duplicate {
			t.Fatalf("duplicate direct child publication: %v", token)
		}
		seen[token] = struct{}{}
		if publication.Count() < 0 {
			t.Fatalf("negative direct child publication count for %v", token)
		}
	}
	for token := range want {
		if _, ok := seen[token]; !ok {
			t.Fatalf("missing direct child publication: %v", token)
		}
	}
}

func TestSemanticSourceFragmentsMatchCanonicalChildCardinalities(t *testing.T) {
	p, err := lower.Lower(lower.Source{
		Name: "semantic-source-fragment.lua",
		Text: []byte("local a = 1\nlocal t = { value = a }\na = a + 1\nif a then return t.value end\nreturn a\n"),
	})
	if err != nil {
		t.Fatalf("lower: %v", err)
	}

	sourceRows, err := source.SemanticSourceFragment(p.Source())
	if err != nil {
		t.Fatalf("Source fragment: %v", err)
	}
	flowRows, err := flow.SemanticSourceFragment(p.Flow())
	if err != nil {
		t.Fatalf("Flow fragment: %v", err)
	}
	staticRows, err := static.SemanticSourceFragment(p.Static())
	if err != nil {
		t.Fatalf("Static fragment: %v", err)
	}
	moduleRows, err := module.SemanticSourceFragment(p.Module())
	if err != nil {
		t.Fatalf("Module fragment: %v", err)
	}

	if len(sourceRows) != 8 {
		t.Fatalf("Source fragment rows = %d, want 8", len(sourceRows))
	}
	if len(flowRows) != 33 {
		t.Fatalf("Flow fragment rows = %d, want 33", len(flowRows))
	}
	if len(staticRows) != 10 {
		t.Fatalf("Static fragment rows = %d, want 10", len(staticRows))
	}
	if len(moduleRows) != 6 {
		t.Fatalf("Module fragment rows = %d, want 6", len(moduleRows))
	}

	// Each direct child emits its own complete fixed owner slice. Check a few
	// rows whose cardinalities are directly visible through those same typed
	// views, without reintroducing a root-level publication projection.
	sourceCounts := publicationCounts(sourceRows)
	if got := sourceCounts[semanticSourceDefinition(t, semanticsource.OriginProgramSourceKey, 0).Token()]; got != p.Source().Keys().Count() {
		t.Fatalf("SourceKey = %d, want %d", got, p.Source().Keys().Count())
	}
	if got := sourceCounts[semanticSourceDefinition(t, semanticsource.OriginProgramSourceExactKey, 0).Token()]; got != p.Source().Keys().ExactCount() {
		t.Fatalf("SourceExactKey = %d, want %d", got, p.Source().Keys().ExactCount())
	}
	moduleCounts := publicationCounts(moduleRows)
	if got := moduleCounts[semanticSourceDefinition(t, semanticsource.OriginProgramModuleImport, 0).Token()]; got != p.Module().Count() {
		t.Fatalf("ModuleImport = %d, want %d", got, p.Module().Count())
	}
}

func programReceiptPublications(t *testing.T, p *program.Program) []semanticsource.Publication {
	t.Helper()
	receipt, ok := p.SemanticSourceReceipt()
	if !ok {
		t.Fatal("Program semantic-source receipt")
	}
	rows := receipt.Publications()
	if len(rows) != 57 {
		t.Fatalf("Program receipt publications = %d, want 57", len(rows))
	}
	return rows
}

func publicationCounts(rows []semanticsource.Publication) map[semanticsource.Token]int {
	counts := make(map[semanticsource.Token]int, len(rows))
	for _, row := range rows {
		counts[row.Definition().Token()] = row.Count()
	}
	return counts
}

func semanticSourceDefinition(t *testing.T, origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.RelationDef {
	t.Helper()
	definition, ok := semanticsource.Definition(origin, facet)
	if !ok {
		t.Fatalf("missing semantic-source definition origin=%d facet=%d", origin, facet)
	}
	return definition
}
