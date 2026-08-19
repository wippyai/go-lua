package factbinding

import (
	"math/rand"
	"sort"
	"testing"
)

// TestPointFoldRunMergeLaw states the admission order law consumed by the
// point RHS fold: the merged row vector is exactly the ascending
// (position, operand) order of the per-operand runs, and every run is
// admitted already ascending in position.
func TestPointFoldRunMergeLaw(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	work := &bindingWork[uint64, uint64]{}
	for trial := 0; trial < 500; trial++ {
		operands := random.Intn(6) + 1
		work.pointFoldRows = work.pointFoldRows[:0]
		work.pointFoldRuns = work.pointFoldRuns[:0]
		var expected []pointFoldCoverageRegion
		for operand := 0; operand < operands; operand++ {
			start := len(work.pointFoldRows)
			position := -1
			for row := random.Intn(8); row > 0; row-- {
				position += 1 + random.Intn(4)
				entry := pointFoldCoverageRegion{position: position, operand: operand}
				work.pointFoldRows = append(work.pointFoldRows, entry)
				expected = append(expected, entry)
			}
			work.pointFoldRuns = append(work.pointFoldRuns, pointFoldRun{next: start, end: len(work.pointFoldRows)})
		}
		sort.SliceStable(expected, func(left, right int) bool {
			if expected[left].position != expected[right].position {
				return expected[left].position < expected[right].position
			}
			return expected[left].operand < expected[right].operand
		})
		work.mergePointFoldRuns()
		if len(work.pointFoldRows) != len(expected) {
			t.Fatalf("trial %d: merged %d rows, expected %d", trial, len(work.pointFoldRows), len(expected))
		}
		for index := range expected {
			if work.pointFoldRows[index] != expected[index] {
				t.Fatalf("trial %d: row %d is %+v, expected %+v", trial, index, work.pointFoldRows[index], expected[index])
			}
		}
	}
}
