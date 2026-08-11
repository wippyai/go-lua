package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestDeclareWeakMergesInterleavedSummaryKeySpansExactly is deliberately
// hostile to a flattened insertion sort: Unit order is by declaration, while
// every summary's keys are interleaved with every other summary. The common
// trailing key also proves that overlap is deduplicated rather than merely
// concatenated.
func TestDeclareWeakMergesInterleavedSummaryKeySpansExactly(t *testing.T) {
	const (
		unitCount  = 96
		blockCount = 96
	)
	spans := interleavedWeakTargetSpans(unitCount, blockCount)
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var weak carrier.Target
	binding, ok := bindTest(testAlgebraInput[uint64, uint64]{
		KeyEnd:      uint64(unitCount*blockCount + 1),
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		LessOrEq: func(left, right uint64) bool { return left <= right },
		declare: func(binding *Binding[uint64, uint64]) bool {
			units := make([]carrier.Unit, unitCount)
			for index, span := range spans {
				unit, declared := binding.DeclareSummary(span)
				if !declared {
					return false
				}
				units[index] = unit
			}
			var declared bool
			weak, declared = binding.DeclareWeak(units)
			return declared
		},
	}, manager)
	if !ok {
		t.Fatal("binding")
	}
	descriptor, declared := binding.targets[weak]
	if !declared {
		t.Fatal("weak target")
	}
	wantCount := unitCount*blockCount + 1
	if len(descriptor.keys) != wantCount {
		t.Fatalf("weak target key count = %d, want %d", len(descriptor.keys), wantCount)
	}
	for index, key := range descriptor.keys {
		if key != uint64(index) {
			t.Fatalf("weak target key[%d] = %d, want %d", index, key, index)
		}
	}
}

func BenchmarkMergeCanonicalKeySpansInterleaved(b *testing.B) {
	spans := interleavedWeakTargetSpans(256, 256)
	var merged []uint64
	b.SetBytes(int64(256 * 257 * 8))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		var ok bool
		merged, ok = mergeCanonicalKeySpans(spans)
		if !ok {
			b.Fatal("merge")
		}
	}
	if len(merged) != 256*256+1 {
		b.Fatalf("merged key count = %d", len(merged))
	}
}

func interleavedWeakTargetSpans(unitCount, blockCount int) [][]uint64 {
	spans := make([][]uint64, unitCount)
	common := uint64(unitCount * blockCount)
	for unit := range spans {
		span := make([]uint64, 0, blockCount+1)
		for block := 0; block < blockCount; block++ {
			span = append(span, uint64(block*unitCount+unit))
		}
		spans[unit] = append(span, common)
	}
	return spans
}
