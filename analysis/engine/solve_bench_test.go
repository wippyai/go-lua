package engine

import (
	"context"
	"fmt"
	"testing"
)

// BenchmarkSolve measures a full Solve over the sealed receipt query matrix
// fixture shared by TestConstructedQueryMatrixScaleInvariance and the
// solve_law_test.go suite. Solve caches its completed state on the solver
// (see solver.completedState in runtime_executor.go), so a second call on the
// same solver returns the warm path instead of re-executing. Each iteration
// therefore builds a fresh fixture outside the timed section and times only
// the first, real Solve call on it.
func BenchmarkSolve(b *testing.B) {
	for _, width := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("width=%d", width), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				fixture := newReceiptQueryMatrixFixture(b, width, nil, nil)
				b.StartTimer()
				state, status := fixture.solver.Solve(context.Background())
				if status != SolveComplete || state == nil {
					b.Fatalf("solve width=%d state=%t status=%v", width, state != nil, status)
				}
			}
		})
	}
}
