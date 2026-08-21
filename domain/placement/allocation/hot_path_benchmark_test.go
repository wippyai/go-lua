package allocation

import (
	"testing"

	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

// BenchmarkAllocationRejectForeignSchema measures the cheap rejection path
// used when an operand arrives from a different or unavailable binding.
func BenchmarkAllocationRejectForeignSchema(b *testing.B) {
	var result operand
	var ok bool
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, ok = allocationOperandForSchema(placement.Schema{}, heap.Key{})
	}
	if ok || result.key.Valid() {
		b.Fatal("foreign allocation accepted")
	}
}
