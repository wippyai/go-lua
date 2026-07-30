package transformer

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

// TestGuardPossibilitiesAmortizesVariantOriginReconstruction proves that
// evaluating the same guard leaf repeatedly across a deep and/or chain reuses
// one run-scoped typevalue derivation instead of rebuilding the discriminated
// union's structural type from scratch at every leaf. Each leaf visit calls
// CanBeTruthy/CanBeFalsy on the identical variant-origin value, so a run-scoped
// cache must amortize the reconstruction cost across the whole chain.
func TestGuardPossibilitiesAmortizesVariantOriginReconstruction(t *testing.T) {
	reg := standard.Registry()

	const caseCount = 24
	members := make([]typ.Type, caseCount)
	for i := range members {
		members[i] = typetable.NewRecord().
			Field("kind", typ.LiteralString(fmt.Sprintf("case%d", i))).
			Field("value", typ.Number).
			Build()
	}
	union := typeexpr.Union(members...)
	family, cases, ok := variant.OriginOfType(union)
	if !ok {
		t.Fatal("union of distinctly tagged records was not recognized as a discriminated variant family")
	}
	value := product.Set(reg, typevalue.FromType(reg, union), variantorigin.Key, variantorigin.Of(family, cases))

	arena := NewArena(reg)
	v := arena.Constant(value)
	w := arena.Constant(typevalue.LiteralBool(reg, true))
	truthyV := arena.Truthy(v)
	guardW := arena.Truthy(w)

	const depth = 64
	chain := guardW
	for i := 0; i < depth; i++ {
		chain = arena.And(truthyV, arena.Or(guardW, chain))
	}

	cursor, err := NewBindingCursor(Shape{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	eval := func() {
		if _, _, ok := arena.evalGuardPossibilities(chain, cursor, SpecializationContext{}); !ok {
			t.Fatal("guard chain did not evaluate exactly")
		}
	}

	eval() // warm up any run-scoped derivation cache before measuring.

	// A cache-backed lookup pays a small constant per leaf visit (key
	// construction), never a rebuild proportional to the union's case count.
	// The uncached baseline measured here before threading the cache was
	// ~192 allocs/leaf (union reconstruction on every visit); a cache-backed
	// lookup measures ~4 allocs/leaf (key hashing only). Budget generous
	// headroom above that floor while staying far below the uncached cost.
	const maxAllocsPerLeaf = 16
	amortized := testing.AllocsPerRun(20, eval)
	if perLeaf := amortized / float64(depth); perLeaf > maxAllocsPerLeaf {
		t.Fatalf("evalGuardPossibilities allocs/leaf = %.2f, want <= %d: "+
			"the %d-deep guard chain re-derives the same variant-origin union at every leaf "+
			"instead of consulting a run-scoped typevalue cache", perLeaf, maxAllocsPerLeaf, depth)
	}
}
