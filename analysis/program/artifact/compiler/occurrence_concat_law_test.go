package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

// concatLawSource is one authored program whose only computation is a `..`
// term in an executable position.
const concatLawSource = `
local function join(left: string, right: string): string return left .. right end
return join
`

// TestArtifactOccurrenceCatalogOwnsEveryConcatTerm states the artifact's
// ownership law over the authored concat plane. Concat carries no arithmetic
// representation lattice, so it is an intentional non-member of the binary
// primitive projection; it is still a computation the program evaluates, and
// every executable Concat candidate Flow classifies is therefore named by one
// artifact occurrence at its own evaluation span. Without that row a plane
// observing a concat subject has no mounted owner to resolve it against.
func TestArtifactOccurrenceCatalogOwnsEveryConcatTerm(t *testing.T) {
	lowered, err := lower.Lower(lower.Source{Name: "occurrence-concat.lua", Text: []byte(concatLawSource)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, lowered, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("concat compilation failed: %s", failure.Error())
	}
	terms := concatLawTerms(t, lowered)
	if len(terms) == 0 {
		t.Fatal("authored concat plane is empty")
	}
	rows := artifact.Program()
	count, held := rows.OccurrenceCount()
	if !held {
		t.Fatal("occurrence plane is not published")
	}
	for _, term := range terms {
		span, spanOK := lowered.Span(term)
		if !spanOK || !lowered.OwnsSpan(span) {
			t.Fatalf("concat term %d carries no owned evaluation span", term)
		}
		owner, owned := -1, false
		for index := 0; index < count; index++ {
			row, rowOK := rows.OccurrenceAt(index)
			if !rowOK {
				t.Fatalf("occurrence row %d is unavailable", index)
			}
			if row.ID() != span.ContextID() {
				continue
			}
			if owned {
				t.Fatalf("concat term %d has two artifact occurrence owners", term)
			}
			owner, owned = index, true
		}
		if !owned {
			t.Fatalf("concat term %d has no artifact occurrence owner at its evaluation span", term)
		}
		row, _ := rows.OccurrenceAt(owner)
		if row.Kind() != programschema.OccurrenceBinaryConcat {
			t.Fatalf("concat term %d is owned by occurrence kind %d, want the concat family", term, row.Kind())
		}
		if row.Code() != uint64(flowkind.BinaryConcat) {
			t.Fatalf("concat occurrence carries operation code %d, want %d", row.Code(), flowkind.BinaryConcat)
		}
		if _, inputs, inputsOK := row.InputSpan(); !inputsOK || inputs != 2 {
			t.Fatalf("concat occurrence carries %d operands, want the ordered operand pair", inputs)
		}
		if _, bodyOK := row.BodyID(); !bodyOK {
			t.Fatal("concat occurrence names no containing body")
		}
	}
	kindCount, kindHeld := rows.OccurrenceKindCount(programschema.OccurrenceBinaryConcat)
	if !kindHeld || kindCount != len(terms) {
		t.Fatalf("artifact catalogues %d concat occurrences for %d executable concat terms", kindCount, len(terms))
	}
}

func concatLawTerms(t *testing.T, lowered *program.Program) []keyspace.Term {
	t.Helper()
	candidates := lowered.Flow().Candidates()
	executable := lowered.Flow().Executable()
	if candidates == nil || executable == nil {
		t.Fatal("candidate classification is unavailable")
	}
	concat := candidates.Concat()
	terms := make([]keyspace.Term, 0, concat.Count())
	for index := 0; index < concat.Count(); index++ {
		term, termOK := concat.At(index)
		if !termOK {
			t.Fatalf("concat candidate %d is unavailable", index)
		}
		if !executable.Contains(term) {
			continue
		}
		terms = append(terms, term)
	}
	return terms
}
