package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
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
	// The fold reads a vector as its width and its row accessor, so the
	// reachable malformed delivery is the zero-width one: a width no
	// coordinate answers, whose accessor is therefore never reached.
	emptyWidth := 0
	emptyAt := func(int) (Value, bool, bool) { return Value{}, false, false }
	for name, input := range map[string]struct {
		schema *Schema
		result ValueSummaryObservation
	}{
		"nil schema":           {schema: nil, result: BeginValueSummary(schema)},
		"invalid running fold": {schema: schema, result: ValueSummaryObservation{Values: make([]Value, 2), Present: make([]bool, 2)}},
		"no observed cells":    {schema: schema, result: BeginValueSummary(schema)},
	} {
		result, ok := AccumulateValueSummaryRows(input.schema, input.result, emptyWidth, emptyAt)
		if ok {
			t.Fatalf("%s accumulated", name)
		}
		if result.Valid || result.Values != nil || result.Present != nil || result.Rows != 0 {
			t.Fatalf("%s returned %+v, want the zero observation", name, result)
		}
	}
}

// TestValueSummaryProjectsPortableIDsThroughItsOwner pins the C5a boundary:
// readers use the ValueID issued by the sealed owner, while the dense image
// remains private to Value. A foreign or absent ID never becomes a fabricated
// dense coordinate.
func TestValueSummaryProjectsPortableIDsThroughItsOwner(t *testing.T) {
	firstID := identity.ContentID{1}
	secondID := identity.ContentID{2}
	schema := summaryCodecSchema(firstID, secondID)
	observation := ValueSummaryObservation{
		Values:  []Value{{schema: schema, top: true}, {}},
		Present: []bool{true, false}, Rows: 1, Valid: true, owner: schema,
	}
	if _, present, valid := observation.ValueAtID(firstID); !present || !valid {
		t.Fatal("owner did not project the present portable ValueID")
	}
	if _, present, valid := observation.ValueAtID(secondID); present || !valid {
		t.Fatal("owner confused an absent portable ValueID with an invalid one")
	}
	if _, _, valid := observation.ValueAtID(identity.ContentID{3}); valid {
		t.Fatal("owner admitted a foreign portable ValueID")
	}
}
