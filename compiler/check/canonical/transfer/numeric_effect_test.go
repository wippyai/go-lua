package transfer

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
)

func TestNumericEffectAppliesPrimitiveAtomsFromTop(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}
	arrKey := constraint.PathKey("arr")
	idxKey := constraint.PathKey("i")

	if !tr.applyNumericEffect(&out, NumericEffect{Ops: []NumericOp{
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
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}

	if tr.applyNumericEffect(&out, NumericEffect{
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
	tr := New(input.Inputs{}, Config{})
	base := numeric.NewState()
	base.ApplyLenGeConst("arr", 1)
	out := flow.PointState{Num: base}

	if !tr.applyNumericEffect(&out, NumericEffect{
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
	tr := New(input.Inputs{}, Config{})
	num := numeric.NewState()
	num.ApplyLenGeConst("arr", 1)
	out := flow.PointState{Num: num}

	if !tr.applyNumericEffect(&out, NumericEffect{
		Ops: []NumericOp{{Kind: NumericDropLenBound, Key: "arr"}},
	}) {
		t.Fatalf("numeric effect did not report dropped length fact")
	}
	if out.Num != nil {
		t.Fatalf("empty numeric state was not canonicalized to Top: %v", out.Num)
	}
}

func TestNumericComparisonOpsAvoidStrictBoundOverflow(t *testing.T) {
	if ops := numericConstComparisonOps("i", "<", math.MinInt64); len(ops) != 0 {
		t.Fatalf("x < MinInt64 ops = %#v, want no over-approximating overflow op", ops)
	}
	if ops := numericConstComparisonOps("i", ">", math.MaxInt64); len(ops) != 0 {
		t.Fatalf("x > MaxInt64 ops = %#v, want no over-approximating overflow op", ops)
	}
	if ops := numericLengthBoundOps("arr", "<", math.MinInt64); len(ops) != 0 {
		t.Fatalf("#arr < MinInt64 ops = %#v, want no overflow op", ops)
	}
	if ops := numericLengthBoundOps("arr", ">", math.MaxInt64); len(ops) != 0 {
		t.Fatalf("#arr > MaxInt64 ops = %#v, want no overflow op", ops)
	}
}
