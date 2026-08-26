package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

// globalEntryAdmissions answers the cells one lowered source admits at
// callable entry, and the total number of body entry points those rows span.
func globalEntryAdmissions(t *testing.T, text string) (map[string]int, int) {
	t.Helper()
	published, err := lower.Lower(lower.Source{Name: "global-entry-admission.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile: %s", failure.Error())
	}
	program := artifact.Program()
	count, countOK := program.OccurrenceKindCount(programschema.OccurrenceGlobalEntry)
	if !countOK {
		t.Fatal("global-entry family is unpublished")
	}
	admitted, points := make(map[string]int, count), 0
	for index := 0; index < count; index++ {
		row, rowOK := program.OccurrenceKindAt(programschema.OccurrenceGlobalEntry, index)
		if !rowOK || !row.Available() {
			t.Fatalf("global-entry row %d is unavailable", index)
		}
		if _, named := row.BodyID(); named {
			t.Fatalf("global-entry row %d names one body, but it admits a cell at every entry", index)
		}
		_, pointCount, spanOK := row.PointSpan()
		if !spanOK || pointCount == 0 {
			t.Fatalf("global-entry row %d names no entry point", index)
		}
		admitted[row.ID().String()] = int(pointCount)
		points += int(pointCount)
	}
	return admitted, points
}

// TestGlobalEntryAdmitsOnlyCellsThisProgramDoesNotWrite states the admission's
// own contract, at the level that decides it.
//
// A cell no code writes holds the value it was bound with at every entry, so a
// body entered by an unknown caller may rely on it and the row is issued. A
// cell this program assigns holds the join over those writes at an arbitrary
// entry, which one binding cannot state, so no row is issued and the cell
// keeps its flow-derived value. Issuing one there would put a single path's
// value where every path's value belongs.
func TestGlobalEntryAdmitsOnlyCellsThisProgramDoesNotWrite(t *testing.T) {
	unwritten, unwrittenPoints := globalEntryAdmissions(t, `
local function reads(value: string | number): string
    if type(value) == "string" then return value end
    return ""
end
return reads
`)
	if len(unwritten) == 0 {
		t.Fatal("a global cell this program never writes was not admitted at callable entry")
	}
	if unwrittenPoints == 0 {
		t.Fatal("an admitted cell was issued at no entry point")
	}

	written, _ := globalEntryAdmissions(t, `
local function reads(value: string | number): string
    if type(value) == "string" then return value end
    return ""
end
type = reads
return reads
`)
	if len(written) != 0 {
		t.Fatalf("a cell this program writes was admitted at callable entry with a single binding; admitted=%v", written)
	}
}
