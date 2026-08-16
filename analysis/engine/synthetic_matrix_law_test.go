package engine

import (
	"context"
	"testing"
)

// TestReceiptQueriesRemainBoundToDeclaredRows exercises a bounded complete
// permutation matrix. Every one of the four declared Factor/Rule/Query
// triples is assembled through the receipt directory in all 4! row orders.
// The topology key and semantic query vector must remain invariant.
func TestReceiptQueriesRemainBoundToDeclaredRows(t *testing.T) {
	permutations := receiptQueryPermutations(4)
	var baseline receiptQueryMatrixFixture
	for index, permutation := range permutations {
		fixture := newReceiptQueryMatrixFixture(t, 4, permutation, permutation)
		if index == 0 {
			baseline = fixture
			state, status := fixture.solver.Solve(context.Background())
			if state == nil || status != SolveComplete {
				t.Fatalf("baseline receipt solve = state:%v status:%v", state, status)
			}
			for queryIndex, query := range fixture.queries {
				value, readable := ReceiptQueryResult[uint64](query, fixture.solver, state)
				if !readable || value != baseline.expected[queryIndex] {
					t.Fatalf("baseline query[%d] = %d/%v, want %d/true", queryIndex, value, readable, baseline.expected[queryIndex])
				}
			}
			continue
		}
		if fixture.topologyKey != baseline.topologyKey {
			t.Fatalf("receipt topology key changed under permutation %v", permutation)
		}
		if fixture.schemaID != baseline.schemaID {
			t.Fatalf("receipt schema identity changed under permutation %v", permutation)
		}
		state, status := fixture.solver.Solve(context.Background())
		if state == nil || status != SolveComplete {
			t.Fatalf("permutation %v solve = state:%v status:%v", permutation, state, status)
		}
		for queryIndex, query := range fixture.queries {
			value, readable := ReceiptQueryResult[uint64](query, fixture.solver, state)
			if !readable || value != baseline.expected[queryIndex] {
				t.Fatalf("permutation %v query[%d] = %d/%v, want %d/true", permutation, queryIndex, value, readable, baseline.expected[queryIndex])
			}
		}
	}
}

// TestReceiptQueryMatrixScaleInvariance keeps the larger historical matrix
// widths represented without multiplying the exhaustive four-row test. Each
// width is built in identity, reverse, and rotated declaration/row orders.
func TestReceiptQueryMatrixScaleInvariance(t *testing.T) {
	for _, count := range []int{16, 21, 25} {
		orders := receiptQueryScaleOrders(count)
		var baseline receiptQueryMatrixFixture
		for index, rowOrder := range orders {
			declarationOrder := orders[(index+1)%len(orders)]
			fixture := newReceiptQueryMatrixFixture(t, count, rowOrder, declarationOrder)
			state, status := fixture.solver.Solve(context.Background())
			if state == nil || status != SolveComplete {
				t.Fatalf("count %d order %d solve = state:%v status:%v", count, index, state, status)
			}
			if index == 0 {
				baseline = fixture
			} else if fixture.schemaID != baseline.schemaID || fixture.topologyKey != baseline.topologyKey {
				t.Fatalf("count %d order %d changed schema/topology identity", count, index)
			}
			for queryIndex, query := range fixture.queries {
				value, readable := ReceiptQueryResult[uint64](query, fixture.solver, state)
				if !readable || value != baseline.expected[queryIndex] {
					t.Fatalf("count %d order %d query[%d] = %d/%v, want %d/true", count, index, queryIndex, value, readable, baseline.expected[queryIndex])
				}
			}
		}
	}
}

func receiptQueryScaleOrders(count int) [][]int {
	identity := make([]int, count)
	for index := range identity {
		identity[index] = index
	}
	reverse := append([]int(nil), identity...)
	for left, right := 0, len(reverse)-1; left < right; left, right = left+1, right-1 {
		reverse[left], reverse[right] = reverse[right], reverse[left]
	}
	rotation := append([]int(nil), identity...)
	if count > 1 {
		rotation = append(rotation[1:], rotation[0])
	}
	return [][]int{identity, reverse, rotation}
}
