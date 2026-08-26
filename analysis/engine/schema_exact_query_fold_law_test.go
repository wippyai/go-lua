package engine

import (
	"context"
	"testing"
)

func TestSchemaExactQueryFoldPreservesCanonicalMultiplicity(t *testing.T) {
	_, factor, query := exactQuerySchemaFixture(t)
	spec := HotExactQuerySpec[uint64, uint64]{
		Fold: QueryFold[OrderedCells[uint64], uint64]{
			Begin: func() uint64 { return 0 },
			Accumulate: func(result uint64, cells OrderedCells[uint64]) (uint64, bool) {
				if cells.Count() != 1 {
					return 0, false
				}
				value, present, valid := cells.At(0)
				if !valid {
					return 0, false
				}
				if !present {
					return result, true
				}
				return result + value, true
			},
		},
		Result: FrozenResult[uint64]{
			Semantic:    coldKey(953_100),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
			Present:     func(uint64) bool { return true },
		},
	}
	binding := NewSchemaBinding(factor.Schema())
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, spec) || !binding.Seal() {
		t.Fatal("exact query fold binding")
	}
	implementation, ok := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ok || implementation == nil {
		t.Fatal("exact query fold implementation")
	}
	row, rowOK := implementation.sealedRow()
	begin, accumulate := row.projection.begin, row.projection.accumulate
	if !rowOK || begin == nil || accumulate == nil {
		t.Fatal("exact query fold authority")
	}
	result := begin()
	for _, row := range []OrderedCells[uint64]{
		{record: newOrderedCellsRecord([]summaryCell[uint64]{{value: 0, present: false}})},
		{record: newOrderedCellsRecord([]summaryCell[uint64]{{value: 3, present: true}})},
		{record: newOrderedCellsRecord([]summaryCell[uint64]{{value: 5, present: true}})},
	} {
		result, ok = accumulate(result, row)
		if !ok {
			t.Fatal("exact fold rejected a canonical row")
		}
	}
	if result != 8 {
		t.Fatalf("exact fold result=%d, want 8", result)
	}
}

func TestSchemaExactQueryFoldRejectsMixedProjectAndFoldAuthority(t *testing.T) {
	_, factor, query := exactQuerySchemaFixture(t)
	spec := hotExactQuerySpec()
	spec.Fold = QueryFold[OrderedCells[uint64], uint64]{
		Begin:      func() uint64 { return 0 },
		Accumulate: func(result uint64, _ OrderedCells[uint64]) (uint64, bool) { return result, true },
	}
	binding := NewSchemaBinding(factor.Schema())
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || BindExactQuery(binding, query, factor, spec) || !binding.Poisoned() {
		t.Fatal("mixed exact Project/Fold authority was accepted")
	}
}

// TestSchemaExactQueryFoldMaterializesThroughCommittedProgram executes the
// Fold after ConstructProgram and CommittedProgram.Seal.  The fold law is
// therefore checked at the published query surface, not only by calling its
// cell accumulator directly.
func TestSchemaExactQueryFoldMaterializesThroughCommittedProgram(t *testing.T) {
	fixture := newFoldQueryMatrixFixture(t, 3)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("fold solve = state:%t status:%v", state != nil, status)
	}
	for index, query := range fixture.queries {
		key, keyed := query.PublicationKey()
		value, readable := testSnapshotQueryValue[uint64](fixture.solver, state, key)
		// The fold begins at its seed and accumulates the cell the query's own
		// point wrote, which the matrix rule staged as one.
		if !keyed || !readable || value != 38 {
			t.Fatalf("fold query[%d] = %d/%t keyed=%t, want fold seed 37 over the staged cell", index, value, readable, keyed)
		}
	}
}
