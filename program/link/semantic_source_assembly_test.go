package link

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/internal/schema/relations"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestSourcePublicationsSealCompleteProject(t *testing.T) {
	sealed, contract, _, _ := semanticSourceFixture(t, false)
	got, err := sealed.SourcePublications()
	if err != nil {
		t.Fatalf("source catalog: %v", err)
	}
	if got.Count() != 111 || got.Count() != semanticsource.CatalogSchema().Count() {
		t.Fatalf("publication count = %d, want exact 111/generated denominator %d", got.Count(), semanticsource.CatalogSchema().Count())
	}

	want := make(map[semanticsource.Token]int, got.Count())
	for index := 0; index < sealed.Project().Mounts().Count(); index++ {
		shard, ok := sealed.Project().Mounts().At(index)
		if !ok {
			t.Fatalf("ShardAt(%d)", index)
		}
		p, ok := sealed.Project().Mounts().Program(shard)
		if !ok || p == nil {
			t.Fatalf("Program(%v)", shard)
		}
		addSemanticSourceFragment(t, want, programReceiptPublications(t, p), true)
	}
	targetReceipt, ok := contract.SemanticSourceReceipt()
	if !ok {
		t.Fatal("target receipt")
	}
	targetRows := targetReceipt.Publications()
	if len(targetRows) == 0 {
		t.Fatal("target receipt publications")
	}
	addSemanticSourceFragment(t, want, targetRows, false)
	linkRows, ok := sealed.sourcePublications()
	if !ok {
		t.Fatal("link fragment")
	}
	addSemanticSourceFragment(t, want, linkRows, false)

	seen := make(map[semanticsource.Token]struct{}, got.Count())
	for index := 0; index < got.Count(); index++ {
		measure, ok := got.At(index)
		if !ok {
			t.Fatalf("At(%d)", index)
		}
		token := measure.Token()
		if _, duplicate := seen[token]; duplicate {
			t.Fatalf("duplicate sealed token %v", token)
		}
		seen[token] = struct{}{}
		if count, present := want[token]; !present || count != measure.Count() {
			t.Fatalf("sealed token %v = %d/%t, want %d", token, measure.Count(), present, count)
		}
	}
	if len(seen) != len(want) || len(seen) != 111 || len(seen) != semanticsource.CatalogSchema().Count() {
		t.Fatalf("sealed vocabulary = %d, want exact 111/generated denominator %d", len(seen), semanticsource.CatalogSchema().Count())
	}
}

func TestSourcePublicationsCountDuplicateProgramMounts(t *testing.T) {
	p := source(t, `local value = 1; return value`)
	sealed := linked(t, contract(t),
		linkproject.Module{Name: "first", Program: p},
		linkproject.Module{Name: "second", Program: p},
	)
	got, err := sealed.SourcePublications()
	if err != nil {
		t.Fatalf("source catalog: %v", err)
	}
	counts := semanticSourceCatalogCounts(t, got)
	fragment := programReceiptPublications(t, p)
	for _, row := range fragment {
		token := row.Definition().Token()
		if actual := counts[token]; actual != 2*row.Count() {
			t.Fatalf("duplicate mounted Program token %v = %d, want %d", token, actual, 2*row.Count())
		}
	}
}

func TestSourcePublicationsRetainZeroRowsAndReplay(t *testing.T) {
	sealed := linked(t, contract(t), linkproject.Module{Name: "main", Program: source(t, ``)})
	receipt, ok := sealed.SemanticSourceReceipt()
	if !ok || receipt.OwnerID() != sealed.ContentID() {
		t.Fatalf("Link aggregate receipt = %x/%t, want owner %x/true", receipt.OwnerID(), ok, sealed.ContentID())
	}
	detached, ok := receipt.Publications()
	if !ok || detached.Count() != 111 || detached.Count() != semanticsource.CatalogSchema().Count() {
		t.Fatalf("detached Link receipt = %d/%t, want exact 111/generated denominator %d", detached.Count(), ok, semanticsource.CatalogSchema().Count())
	}
	first, err := sealed.SourcePublications()
	if err != nil {
		t.Fatalf("first source catalog: %v", err)
	}
	second, err := sealed.SourcePublications()
	if err != nil {
		t.Fatalf("second source catalog: %v", err)
	}
	if !sameSemanticSourceMeasures(first, second) {
		t.Fatal("source catalog replay changed")
	}
	counts := semanticSourceCatalogCounts(t, first)
	for _, key := range []struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
	}{
		{semanticsource.OriginLinkProjectBaseApplication, 0},
		{semanticsource.OriginLinkBoundary, 0},
		{semanticsource.OriginLinkModule, 0},
		{semanticsource.OriginLinkHost, 0},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget},
	} {
		definition, ok := semanticsource.Definition(key.origin, key.facet)
		if !ok {
			t.Fatalf("definition %v/%v", key.origin, key.facet)
		}
		if count, present := counts[definition.Token()]; !present || count != 0 {
			t.Fatalf("zero row %v/%v = %d/%t, want 0/true", key.origin, key.facet, count, present)
		}
	}
}

func TestSourcePublicationsRejectsNilAndStaleAggregate(t *testing.T) {
	if _, err := (*Link)(nil).SourcePublications(); !errors.Is(err, errSemanticSourceAssemblyUnavailable) {
		t.Fatalf("nil Link source catalog = %v, want unavailable", err)
	}
	sealed := linked(t, contract(t), linkproject.Module{Name: "main", Program: source(t, `return 1`)})
	stale := *sealed
	stale.semanticReceipt = SemanticSourceReceipt{}
	if _, err := stale.SourcePublications(); !errors.Is(err, errSemanticSourceAssemblyUnavailable) {
		t.Fatalf("stale Link receipt = %v, want unavailable", err)
	}
	detached, err := sealed.SourcePublications()
	if err != nil {
		t.Fatalf("source catalog: %v", err)
	}
	measures := detached.Measures()
	if len(measures) == 0 {
		t.Fatal("source catalog has no measures")
	}
	measures[0] = semanticsource.Measure{}
	replayed, err := sealed.SourcePublications()
	if err != nil {
		t.Fatalf("replayed source catalog: %v", err)
	}
	if _, ok := replayed.At(0); !ok {
		t.Fatal("detached mutation invalidated cached Link receipt")
	}
}

func TestSemanticSourceAssemblyRejectsForeignIncompleteAndOverflowFragments(t *testing.T) {
	assembly := testSemanticSourceAssembly(t)
	p := source(t, `return 1`)
	programRows := programReceiptPublications(t, p)
	if err := assembly.acceptProgram(programRows[:len(programRows)-1]); !errors.Is(err, errSemanticSourceAssemblyFragment) {
		t.Fatalf("incomplete Program fragment = %v, want fragment rejection", err)
	}

	contract := contract(t)
	targetReceipt, ok := contract.SemanticSourceReceipt()
	if !ok {
		t.Fatal("target receipt")
	}
	targetRows := targetReceipt.Publications()
	if len(targetRows) == 0 {
		t.Fatal("target receipt publications")
	}
	if err := assembly.acceptProgram(targetRows); !errors.Is(err, errSemanticSourceAssemblyFragment) {
		t.Fatalf("foreign Target fragment as Program = %v, want fragment rejection", err)
	}

	overflow := testSemanticSourceAssembly(t)
	maxRows := completeSemanticSourceFragment(t, overflow, semanticSourceProgram, maxInt())
	if err := overflow.acceptProgram(maxRows); err != nil {
		t.Fatalf("first max Program fragment: %v", err)
	}
	if err := overflow.acceptProgram(maxRows); !errors.Is(err, errSemanticSourceAssemblyOverflow) {
		t.Fatalf("second max Program fragment = %v, want overflow rejection", err)
	}
}

func testSemanticSourceAssembly(t testing.TB) *semanticSourceAssembly {
	t.Helper()
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatalf("relation schema: %v", err)
	}
	assembly, err := newSemanticSourceAssembly(schema)
	if err != nil {
		t.Fatalf("new assembly: %v", err)
	}
	return assembly
}

func completeSemanticSourceFragment(t testing.TB, assembly *semanticSourceAssembly, contributor semanticSourceContributor, firstCount int) []semanticsource.Publication {
	t.Helper()
	rows := make([]semanticsource.Publication, 0, assembly.expectedCount[contributor])
	for _, expected := range assembly.schemaExpected {
		if expected.contributor != contributor {
			continue
		}
		count := 0
		if len(rows) == 0 {
			count = firstCount
		}
		row, err := semanticsource.SealPublication(expected.definition, count)
		if err != nil {
			t.Fatalf("seal publication: %v", err)
		}
		rows = append(rows, row)
	}
	if len(rows) != assembly.expectedCount[contributor] {
		t.Fatalf("complete fragment rows = %d, want %d", len(rows), assembly.expectedCount[contributor])
	}
	return rows
}

func programReceiptPublications(t testing.TB, p *program.Program) []semanticsource.Publication {
	t.Helper()
	receipt, ok := p.SemanticSourceReceipt()
	if !ok {
		t.Fatal("Program semantic-source receipt")
	}
	rows := receipt.Publications()
	if len(rows) != 57 {
		t.Fatalf("Program receipt rows = %d, want 57", len(rows))
	}
	return rows
}

func addSemanticSourceFragment(t testing.TB, counts map[semanticsource.Token]int, rows []semanticsource.Publication, sum bool) {
	t.Helper()
	for _, row := range rows {
		token := row.Definition().Token()
		if prior, duplicate := counts[token]; duplicate && !sum {
			t.Fatalf("duplicate fragment token %v", token)
		} else if duplicate {
			counts[token] = prior + row.Count()
			continue
		}
		counts[token] = row.Count()
	}
}

func semanticSourceCatalogCounts(t testing.TB, publications semanticsource.Publications) map[semanticsource.Token]int {
	t.Helper()
	counts := make(map[semanticsource.Token]int, publications.Count())
	for index := 0; index < publications.Count(); index++ {
		measure, ok := publications.At(index)
		if !ok {
			t.Fatalf("At(%d)", index)
		}
		if _, duplicate := counts[measure.Token()]; duplicate {
			t.Fatalf("duplicate measure %v", measure.Token())
		}
		counts[measure.Token()] = measure.Count()
	}
	return counts
}

func sameSemanticSourceMeasures(left, right semanticsource.Publications) bool {
	if left.Count() != right.Count() {
		return false
	}
	for index := 0; index < left.Count(); index++ {
		leftMeasure, leftOK := left.At(index)
		rightMeasure, rightOK := right.At(index)
		if !leftOK || !rightOK || leftMeasure.Token() != rightMeasure.Token() || leftMeasure.Count() != rightMeasure.Count() {
			return false
		}
	}
	return true
}
