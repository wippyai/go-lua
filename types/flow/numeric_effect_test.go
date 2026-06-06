package flow

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
)

func TestNumericEffectAppliesPrimitiveAtomsFromTop(t *testing.T) {
	out := PointState{}
	arrKey := constraint.PathKey("arr")
	idxKey := constraint.PathKey("i")

	if !ApplyNumericEffect(&out, NumericEffect{Ops: []NumericOp{
		{Kind: NumericLenGeConst, Key: arrKey, Const: 1},
		{Kind: NumericLenLeConst, Key: arrKey, Const: 4},
		{Kind: NumericVarLeLenOffset, Key: idxKey, Other: arrKey, Offset: -1},
	}}) {
		t.Fatalf("numeric effect did not report a state change")
	}
	if out.Num == nil {
		t.Fatalf("numeric effect from Top left Num at Top")
	}
	lower, upper, ok := out.Num.LenBoundsFor(arrKey)
	if !ok || lower != 1 || upper != 4 {
		t.Fatalf("length bound = [%d,%d], ok=%v; want [1,4]", lower, upper, ok)
	}
	ref, offset, ok := out.Num.LenRefWithOffsetFor(idxKey)
	if !ok || ref != arrKey || offset != -1 {
		t.Fatalf("length ref = (%s,%d), ok=%v; want (%s,-1)", ref, offset, ok, arrKey)
	}
}

func TestNumericEffectRequireExistingPreservesTop(t *testing.T) {
	out := PointState{}

	if ApplyNumericEffect(&out, NumericEffect{
		Ops:             []NumericOp{{Kind: NumericLenGeConst, Key: "arr", Const: 1}},
		RequireExisting: true,
	}) {
		t.Fatalf("RequireExisting numeric effect changed Top")
	}
	if out.Num != nil {
		t.Fatalf("RequireExisting numeric effect materialized Num: %v", out.Num)
	}
}

func TestNumericEffectClonesBeforeApplying(t *testing.T) {
	base := numeric.NewState()
	base.ApplyLenGeConst("arr", 1)
	out := PointState{Num: base}

	if !ApplyNumericEffect(&out, NumericEffect{
		Ops: []NumericOp{{Kind: NumericLenGeConst, Key: "arr", Const: 3}},
	}) {
		t.Fatalf("numeric effect did not report stronger length floor")
	}
	origLower, _, _ := base.LenBoundsFor("arr")
	if origLower != 1 {
		t.Fatalf("numeric effect mutated input state: lower=%d, want 1", origLower)
	}
	nextLower, _, _ := out.Num.LenBoundsFor("arr")
	if nextLower != 3 {
		t.Fatalf("numeric effect lower=%d, want 3", nextLower)
	}
}

func TestNumericEffectCanonicalizesEmptyStateToTop(t *testing.T) {
	num := numeric.NewState()
	num.ApplyLenGeConst("arr", 1)
	out := PointState{Num: num}

	if !ApplyNumericEffect(&out, NumericEffect{
		Ops: []NumericOp{{Kind: NumericDropLenBound, Key: "arr"}},
	}) {
		t.Fatalf("numeric effect did not report dropped length fact")
	}
	if out.Num != nil {
		t.Fatalf("empty numeric state was not canonicalized to Top: %v", out.Num)
	}
}

func TestNumericLenGeConstPathOpUsesSymbolPathKey(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(7), "items").Field("rows")

	op, ok := NumericLenGeConstPathOp(path, 3)
	if !ok {
		t.Fatalf("path length op was not produced")
	}
	want := SymbolPathKey(cfg.SymbolID(7), path.Segments)
	if op.Kind != NumericLenGeConst || op.Key != want || op.Const != 3 {
		t.Fatalf("path length op = %#v, want key=%s lower=3", op, want)
	}
}

func TestNumericLenGeConstIndexedPrefixOps(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(8), "items").
		IndexInt(2).
		Field("child").
		IndexInt(4)

	ops := NumericLenGeConstIndexedPrefixOps(path)
	if len(ops) != 2 {
		t.Fatalf("prefix ops len=%d, want 2: %#v", len(ops), ops)
	}
	if ops[0].Key != SymbolPathKey(cfg.SymbolID(8), nil) || ops[0].Const != 2 {
		t.Fatalf("first prefix op = %#v", ops[0])
	}
	if ops[1].Key != SymbolPathKey(cfg.SymbolID(8), path.Segments[:2]) || ops[1].Const != 4 {
		t.Fatalf("second prefix op = %#v", ops[1])
	}
}

func TestNumericComparisonOpsAvoidStrictBoundOverflow(t *testing.T) {
	if ops := NumericConstComparisonOps("i", "<", math.MinInt64); len(ops) != 0 {
		t.Fatalf("x < MinInt64 ops = %#v, want no over-approximating overflow op", ops)
	}
	if ops := NumericConstComparisonOps("i", ">", math.MaxInt64); len(ops) != 0 {
		t.Fatalf("x > MaxInt64 ops = %#v, want no over-approximating overflow op", ops)
	}
	if ops := NumericLengthBoundOps("arr", "<", math.MinInt64); len(ops) != 0 {
		t.Fatalf("#arr < MinInt64 ops = %#v, want no overflow op", ops)
	}
	if ops := NumericLengthBoundOps("arr", ">", math.MaxInt64); len(ops) != 0 {
		t.Fatalf("#arr > MaxInt64 ops = %#v, want no overflow op", ops)
	}
}
