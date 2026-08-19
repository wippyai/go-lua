package engine

import (
	"context"
	"testing"
)

func TestConstructedQueriesRemainBoundToDeclaredRows(t *testing.T) {
	baseline := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	state, status := baseline.solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("baseline solve state=%t status=%v", state != nil, status)
	}
	keys := make([][2]byte, len(baseline.queries))
	for index, query := range baseline.queries {
		key, ok := query.PublicationKey()
		if !ok {
			t.Fatalf("baseline query %d has no publication key", index)
		}
		keys[index] = [2]byte{key[0], key[1]}
	}
	for _, permutation := range receiptQueryPermutations(4) {
		fixture := newReceiptQueryMatrixFixture(t, 4, permutation, permutation)
		state, status := fixture.solver.Solve(context.Background())
		if status != SolveComplete || state == nil || fixture.schemaID != baseline.schemaID || fixture.topologyKey != baseline.topologyKey {
			t.Fatalf("permutation %v changed sealed program identity", permutation)
		}
		for index, query := range fixture.queries {
			key, ok := query.PublicationKey()
			value, readable := testSnapshotQueryValue[uint64](fixture.solver, state, key)
			if !ok || !readable || [2]byte{key[0], key[1]} != keys[index] || value != fixture.expected[index] {
				t.Fatalf("permutation %v query %d = %d/%t", permutation, index, value, readable)
			}
		}
	}
}

func TestConstructedQueryMatrixScaleInvariance(t *testing.T) {
	for _, width := range []int{1, 2, 4, 8, 16} {
		fixture := newReceiptQueryMatrixFixture(t, width, nil, nil)
		state, status := fixture.solver.Solve(context.Background())
		if status != SolveComplete || state == nil || len(fixture.queries) != width {
			t.Fatalf("width %d solve state=%t status=%v queries=%d", width, state != nil, status, len(fixture.queries))
		}
		for index, query := range fixture.queries {
			key, keyed := query.PublicationKey()
			value, readable := testSnapshotQueryValue[uint64](fixture.solver, state, key)
			if !keyed || !readable || value != fixture.expected[index] {
				t.Fatalf("width %d query %d = %d/%t", width, index, value, readable)
			}
		}
	}
}
