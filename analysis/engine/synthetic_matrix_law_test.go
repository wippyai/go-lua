package engine

import "testing"

// TestGenericEngineSemanticFactorMatrix is the Wave-C static semantic matrix.
// Every declared Factor has its own ingress and Query.  Transform Rules cover
// real one, two, and seven input products, with an output-local carry edge.
func TestGenericEngineSemanticFactorMatrix(t *testing.T) {
	for _, count := range []int{16, 21, 25} {
		t.Run("factors", func(t *testing.T) {
			var baseline staticMatrixObservation
			for permutationIndex, permutation := range staticMatrixPermutations(count) {
				observed := runStaticMatrixFixture(t, count, permutation)
				if len(observed.values) != count {
					t.Fatalf("query vector length = %d, want %d", len(observed.values), count)
				}
				if permutationIndex == 0 {
					baseline = observed
					continue
				}
				if observed.composition != baseline.composition {
					t.Fatal("CompositionID changed under static declaration permutation")
				}
				if observed.topology.Key() != baseline.topology.Key() {
					t.Fatal("sealed Topology.Key changed under static topology permutation")
				}
				for index, value := range observed.values {
					if value != baseline.values[index] {
						t.Fatalf("QueryResult[%d] = %d, want %d", index, value, baseline.values[index])
					}
				}
			}
		})
	}
}
