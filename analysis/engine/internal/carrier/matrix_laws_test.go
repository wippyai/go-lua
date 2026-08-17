package carrier

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
)

// TestCarrierMatrixCarryOnlyProductArity checks the carrier-owned half of the
// generic matrix. Read observation and typed write
// targets are deliberately exercised by their own package contracts: carrier
// only owns the complete predecessor vector and the atomic contribution cut.
func TestCarrierMatrixCarryOnlyProductArity(t *testing.T) {
	for _, factors := range []int{3, 9, 16, 25} {
		for _, inputs := range []int{1, 2, 4} {
			t.Run("factors="+strconv.Itoa(factors)+"/inputs="+strconv.Itoa(inputs), func(t *testing.T) {
				_, whole, composition, operations, initial := contributionFixture(t, factors)
				anchor := shape.Slot(factors / 2)
				zeroRead, ok := composition.SealContribution(inputs, nil, nil, true)
				if !ok {
					t.Fatal("seal zero-read contribution")
				}
				carryOnly, ok := composition.SealContribution(inputs, []shape.Slot{anchor}, []ContributionSource{{Slot: anchor, Input: inputs - 1}}, false)
				if !ok {
					t.Fatal("seal carry-only contribution")
				}
				anchorWrite, ok := composition.SealContribution(0, []shape.Slot{anchor}, nil, false)
				if !ok {
					t.Fatal("seal anchor input publication")
				}
				work, ok := composition.NewWork()
				if !ok {
					t.Fatal("work")
				}
				product := make([]PointState, inputs)
				for index := range product[:inputs-1] {
					product[index] = contributionPoint(t, work, initial)
				}
				product[inputs-1] = contributionWrittenPoint(t, work, anchorWrite, composition.Scope(), whole, contributionSlotWrite{operation: operations[int(anchor)], slot: anchor, root: 2})

				zeroBase, ok := work.BeginRuleContribution(zeroRead, composition.Scope(), product, whole)
				if !ok {
					t.Fatal("begin zero-read contribution")
				}
				zeroResult, ok := work.FinishRuleContribution(zeroBase, nil)
				if !ok || !zeroResult.Valid() || !work.EqualUnder(zeroResult.State(), product[0].State()) {
					t.Fatal("zero-read contribution did not retain exactly its support input")
				}

				carryBase, ok := work.BeginRuleContribution(carryOnly, composition.Scope(), product, whole)
				if !ok {
					t.Fatal("begin carry-only contribution")
				}
				projected := carryBase.State()
				carried, carriedOK := projected.HandleAt(anchor)
				inputRoot, inputOK := product[inputs-1].HandleAt(anchor)
				if !carriedOK || !inputOK || carried != inputRoot {
					t.Fatal("carry-only projection did not retain the declared exact source root")
				}
				for slot := 0; slot < factors; slot++ {
					if shape.Slot(slot) == anchor {
						continue
					}
					got, gotOK := projected.HandleAt(shape.Slot(slot))
					want, wantOK := initial.HandleAt(shape.Slot(slot))
					if !gotOK || !wantOK || got != want {
						t.Fatalf("carry-only leaked source slot %d", slot)
					}
				}
				carriedResult, ok := work.FinishRuleContribution(carryBase, nil)
				if !ok || !carriedResult.Valid() {
					t.Fatal("finish carry-only contribution")
				}
				resultRoot, ok := carriedResult.HandleAt(anchor)
				if !ok || resultRoot != inputRoot {
					t.Fatal("finished carry-only contribution lost its source root")
				}
			})
		}
	}
}

func BenchmarkCarrierMatrixCarryOnly(b *testing.B) {
	for _, factors := range []int{3, 9, 16, 25} {
		for _, inputs := range []int{1, 2, 4} {
			b.Run("factors="+strconv.Itoa(factors)+"/inputs="+strconv.Itoa(inputs), func(b *testing.B) {
				_, whole, composition, operations, initial := contributionFixture(b, factors)
				anchor := shape.Slot(factors / 2)
				plan, ok := composition.SealContribution(inputs, []shape.Slot{anchor}, []ContributionSource{{Slot: anchor, Input: inputs - 1}}, false)
				if !ok {
					b.Fatal("seal contribution")
				}
				anchorWrite, ok := composition.SealContribution(0, []shape.Slot{anchor}, nil, false)
				if !ok {
					b.Fatal("seal anchor input publication")
				}
				work, ok := composition.NewWork()
				if !ok {
					b.Fatal("work")
				}
				product := make([]PointState, inputs)
				for index := range product[:inputs-1] {
					product[index] = contributionPoint(b, work, initial)
				}
				product[inputs-1] = contributionWrittenPoint(b, work, anchorWrite, composition.Scope(), whole, contributionSlotWrite{operation: operations[int(anchor)], slot: anchor, root: 2})
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					base, ok := work.BeginRuleContribution(plan, composition.Scope(), product, whole)
					if !ok {
						b.Fatal("begin")
					}
					result, ok := work.FinishRuleContribution(base, nil)
					if !ok || !result.Valid() {
						b.Fatal("finish")
					}
				}
				b.ReportMetric(float64(factors), "factor-slots/op")
				b.ReportMetric(float64(inputs), "product-inputs/op")
				b.ReportMetric(1, "carry-slots/op")
			})
		}
	}
}
