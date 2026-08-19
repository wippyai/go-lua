package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/domain/composite"
)

// TestProgramArtifactIndexBaseCarriesEvaluationSpan is the artifact-side law
// behind index access geometry: the base of an index access is an evaluated
// value occurrence, so it owns an exact evaluation Span, and the compiled Heap
// index row carries that Span for every lens shape.
//
// The lens shape is what varies here. A dotted field name is static metadata
// with no runtime key evaluation, a bracketed literal is a scalar key
// occurrence, and a bracketed expression is a dynamic key. All three describe
// the same base evaluation, so a Program that expresses one and not the others
// has published a lens-dependent base geometry, which no consumer of the
// sealed artifact can order.
func TestProgramArtifactIndexBaseCarriesEvaluationSpan(t *testing.T) {
	for _, fixture := range []struct {
		name, text string
		reads      int
		writes     int
	}{
		{name: "dotted-field-read", reads: 1, text: `
local function run(row: { name: string })
    return row.name
end
return run
`},
		{name: "bracket-literal-read", reads: 1, text: `
local function run(row: { name: string })
    return row["name"]
end
return run
`},
		{name: "dynamic-key-read", reads: 1, text: `
local function run(rows: { number }, index: number)
    return rows[index]
end
return run
`},
		{name: "dotted-field-write", writes: 1, text: `
local function run(row: { name: string })
    row.name = "next"
    return row
end
return run
`},
		{name: "bracket-literal-write", writes: 1, text: `
local function run(row: { name: string })
    row["name"] = "next"
    return row
end
return run
`},
		{name: "dotted-field-chain-read", reads: 2, text: `
local function run(outer: { inner: { name: string } })
    return outer.inner.name
end
return run
`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			published, err := lower.Lower(lower.Source{Name: fixture.name + ".lua", Text: []byte(fixture.text)})
			if err != nil {
				t.Fatal(err)
			}
			compilation, compilationOK := composite.Global()
			if !compilationOK {
				t.Fatal("Program artifact grammar unavailable")
			}
			artifact, failure := composite.CompileArtifactDetailed(published, compilation)
			if failure.Available() || artifact == nil || !artifact.Available() {
				t.Fatalf("compile %s: %s", fixture.name, failure.Error())
			}
			frozen, catalog, coldPublished := artifact.ColdPublication()
			if !coldPublished {
				t.Fatalf("compile %s: cold publication unavailable", fixture.name)
			}
			reads, writes := 0, 0
			indexCount, indexPublished := cold.HeapIndexFamily().Count(&frozen, catalog)
			if !indexPublished {
				t.Fatalf("compile %s: heap index family unavailable", fixture.name)
			}
			for index := 0; index < indexCount; index++ {
				row, rowOK := cold.HeapIndexFamily().At(&frozen, catalog, index)
				if !rowOK || !row.Available() {
					t.Fatalf("heap index row %d unavailable", index)
				}
				if !row.BaseSpan().Available() {
					t.Fatalf("heap index row %d carries no base evaluation Span", index)
				}
				if row.Read() {
					reads++
					continue
				}
				writes++
			}
			if reads != fixture.reads || writes != fixture.writes {
				t.Fatalf("%s published reads=%d writes=%d, want reads=%d writes=%d", fixture.name, reads, writes, fixture.reads, fixture.writes)
			}
		})
	}
}
