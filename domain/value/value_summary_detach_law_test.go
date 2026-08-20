package value

import "testing"

// TestCloneValueSummaryOwnsBothInteriorPlanes proves the domain result's
// Clone/Equal contract. A query answer crosses the domain boundary with two
// mutable interior slices, so either side may subsequently change without
// changing the other.
func TestCloneValueSummaryOwnsBothInteriorPlanes(t *testing.T) {
	schema := &Schema{coordinateCount: 2, potential: 1}
	source := ValueSummaryObservation{
		Values:  []Value{{schema: schema, top: true}, {schema: schema, top: true}},
		Present: []bool{true, true},
		Rows:    1,
		Valid:   true,
		owner:   schema,
	}
	detached := CloneValueSummary(source)
	if !EqualValueSummary(schema, detached, source) {
		t.Fatal("a fresh Value summary clone is not equal to its source")
	}

	source.Values[0] = Value{}
	source.Present[1] = false
	if !detached.Values[0].top || !detached.Present[1] {
		t.Fatal("mutating the source changed the detached Value summary")
	}

	detached.Values[1] = Value{}
	detached.Present[0] = false
	if !source.Values[1].top || !source.Present[0] {
		t.Fatal("mutating the detached Value summary changed its source")
	}
}
