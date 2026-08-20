package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

// TestBeginValueSummaryShapesTheSchemaCoordinateWidth pins the fold's opening
// state: the accumulator is exactly as wide as the schema's sealed coordinate
// range, opens with every coordinate absent, and carries no row yet.
func TestBeginValueSummaryShapesTheSchemaCoordinateWidth(t *testing.T) {
	schema := &Schema{coordinateCount: 3}
	begin := BeginValueSummary(schema)
	if !begin.Valid {
		t.Fatal("a sealed coordinate range opens a valid accumulator")
	}
	if len(begin.Values) != 3 || len(begin.Present) != 3 {
		t.Fatalf("accumulator width = %d values / %d presence bits, want 3/3", len(begin.Values), len(begin.Present))
	}
	if begin.Rows != 0 {
		t.Fatalf("opening rows = %d, want 0", begin.Rows)
	}
	if begin.owner != schema {
		t.Fatal("opening accumulator lost its exact schema owner")
	}
	for index := range begin.Present {
		if begin.Present[index] {
			t.Fatalf("coordinate %d opens present", index)
		}
	}
	// Each fold opens on its own storage: a second query must not observe the
	// first query's writes.
	other := BeginValueSummary(schema)
	begin.Present[1], begin.Values[1] = true, Value{schema: schema, top: true}
	if other.Present[1] || other.Values[1].top {
		t.Fatal("two folds share accumulator storage")
	}
}

// TestBeginValueSummaryWithoutCoordinatesIsTheZeroObservation pins the two
// inputs that carry no coordinate range at all. Both must open the invalid
// zero accumulator rather than an empty valid one, so a later Accumulate
// rejects instead of folding a width the schema never sealed.
func TestBeginValueSummaryWithoutCoordinatesIsTheZeroObservation(t *testing.T) {
	for name, schema := range map[string]*Schema{
		"nil schema":            nil,
		"zero coordinate range": {},
	} {
		begin := BeginValueSummary(schema)
		if begin.Valid || begin.Values != nil || begin.Present != nil || begin.Rows != 0 {
			t.Fatalf("%s opens %+v, want the zero observation", name, begin)
		}
	}
}

// TestAccumulateValueSummaryRejectsMalformedFoldInput pins every rejection the
// fold owns that is reachable without a live engine query frame. A rejection
// must return the zero observation, never a partially folded one.
func TestAccumulateValueSummaryRejectsMalformedFoldInput(t *testing.T) {
	schema := &Schema{coordinateCount: 2}
	// OrderedCells is an engine capability: a domain can name it but cannot
	// construct one, so the reachable vector here is the empty observation.
	var empty engine.OrderedCells[Value]
	if empty.Count() != 0 {
		t.Fatalf("unbound cell vector reports width %d, want 0", empty.Count())
	}
	for name, input := range map[string]struct {
		schema *Schema
		result ValueSummaryObservation
	}{
		"nil schema":           {schema: nil, result: BeginValueSummary(schema)},
		"invalid running fold": {schema: schema, result: ValueSummaryObservation{Values: make([]Value, 2), Present: make([]bool, 2)}},
		"no observed cells":    {schema: schema, result: BeginValueSummary(schema)},
	} {
		result, ok := AccumulateValueSummary(input.schema, input.result, empty)
		if ok {
			t.Fatalf("%s accumulated", name)
		}
		if result.Valid || result.Values != nil || result.Present != nil || result.Rows != 0 {
			t.Fatalf("%s returned %+v, want the zero observation", name, result)
		}
	}
}
