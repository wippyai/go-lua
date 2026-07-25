package engine

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func foldTestTemp(id uint32) wir.Operand {
	return wir.Operand{Kind: wir.OperandTemp, Ref: id}
}

func foldTestNumber(body *wir.Body, text string) wir.Operand {
	return wir.Operand{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstNumber, Number: text}))}
}

func foldTestWord(t *testing.T, folded map[int]nativeConstantWord, index int, representation, text string) {
	t.Helper()
	word, ok := folded[index]
	if !ok {
		t.Fatalf("op %d carries no constant, want %s %s", index, representation, text)
	}
	if word.representation != representation || word.text != text {
		t.Fatalf("op %d = %s %s, want %s %s", index, word.representation, word.text, representation, text)
	}
}

func TestConstantLatticeFoldsThroughSingleAssignmentBindings(t *testing.T) {
	body := wir.NewBody("fold")
	body.Emit(wir.Instruction{Op: wir.OpAssign, Dst: foldTestTemp(1), A: foldTestNumber(body, "10")})
	body.Emit(wir.Instruction{Op: wir.OpBinOp, Operator: wir.BinAdd, Dst: foldTestTemp(2), A: foldTestTemp(1), B: foldTestNumber(body, "5")})
	body.Emit(wir.Instruction{Op: wir.OpAssign, Dst: foldTestTemp(3), A: foldTestTemp(2)})
	body.Emit(wir.Instruction{Op: wir.OpUnOp, Operator: wir.UnNeg, Dst: foldTestTemp(4), A: foldTestTemp(3)})

	folded := nativeFoldedConstants(body)
	foldTestWord(t, folded, 0, "integer", "10")
	foldTestWord(t, folded, 1, "integer", "15")
	foldTestWord(t, folded, 2, "integer", "15")
	foldTestWord(t, folded, 3, "integer", "-15")
}

func TestConstantLatticeWithholdsRewrittenDestination(t *testing.T) {
	body := wir.NewBody("mutate")
	body.Emit(wir.Instruction{Op: wir.OpAssign, Dst: foldTestTemp(1), A: foldTestNumber(body, "10")})
	body.Emit(wir.Instruction{Op: wir.OpBinOp, Operator: wir.BinAdd, Dst: foldTestTemp(2), A: foldTestTemp(1), B: foldTestNumber(body, "5")})
	body.Emit(wir.Instruction{Op: wir.OpAssign, Dst: foldTestTemp(1), A: foldTestNumber(body, "20")})

	if folded := nativeFoldedConstants(body); len(folded) != 0 {
		t.Fatalf("rewritten binding folded to %v, want every row withheld", folded)
	}
}

func TestConstantLatticeWithholdsCapturedBinding(t *testing.T) {
	body := wir.NewBody("capture")
	body.Emit(wir.Instruction{Op: wir.OpAssign, Dst: foldTestTemp(1), A: foldTestNumber(body, "10")})
	body.Emit(wir.Instruction{Op: wir.OpBinOp, Operator: wir.BinAdd, Dst: foldTestTemp(2), A: foldTestTemp(1), B: foldTestNumber(body, "5")})
	body.Emit(wir.Instruction{Op: wir.OpClosure, Dst: foldTestTemp(3), List: body.AppendOperands([]wir.Operand{foldTestTemp(1)})})

	folded := nativeFoldedConstants(body)
	if _, ok := folded[0]; ok {
		t.Fatalf("captured binding published %v, want the row withheld", folded[0])
	}
	if _, ok := folded[1]; ok {
		t.Fatalf("captured binding folded into %v, want the fold stopped", folded[1])
	}
}

func TestConstantLatticeWithholdsUnresolvedOperand(t *testing.T) {
	body := wir.NewBody("opaque")
	body.Emit(wir.Instruction{Op: wir.OpBinOp, Operator: wir.BinAdd, Dst: foldTestTemp(2), A: foldTestTemp(1), B: foldTestNumber(body, "5")})

	if folded := nativeFoldedConstants(body); len(folded) != 0 {
		t.Fatalf("unresolved operand folded to %v, want every row withheld", folded)
	}
}

func TestConstantLatticePromotesFloatArm(t *testing.T) {
	body := wir.NewBody("promote")
	body.Emit(wir.Instruction{Op: wir.OpAssign, Dst: foldTestTemp(1), A: foldTestNumber(body, "2.5")})
	body.Emit(wir.Instruction{Op: wir.OpBinOp, Operator: wir.BinMul, Dst: foldTestTemp(2), A: foldTestTemp(1), B: foldTestNumber(body, "4")})

	folded := nativeFoldedConstants(body)
	foldTestWord(t, folded, 0, "float", "2.5")
	foldTestWord(t, folded, 1, "float", "10.0")
}

func TestConstantLatticeWithholdsDivisionAndExponentiation(t *testing.T) {
	for _, operator := range []wir.Operator{wir.BinDiv, wir.BinPow} {
		body := wir.NewBody("inexact")
		body.Emit(wir.Instruction{Op: wir.OpBinOp, Operator: operator, Dst: foldTestTemp(1), A: foldTestNumber(body, "8"), B: foldTestNumber(body, "2")})
		if folded := nativeFoldedConstants(body); len(folded) != 0 {
			t.Fatalf("operator %d folded to %v, want the row withheld", operator, folded)
		}
	}
}

func TestIntegerArithmeticFoldWithholdsOutsideTheIntegerRange(t *testing.T) {
	cases := []struct {
		name           string
		operator       wir.Operator
		left, right    int64
		representation string
		text           string
		exact          bool
	}{
		{name: "add overflow", operator: wir.BinAdd, left: math.MaxInt64, right: 1},
		{name: "sub overflow", operator: wir.BinSub, left: math.MinInt64, right: 1},
		{name: "mul overflow", operator: wir.BinMul, left: math.MaxInt64, right: 2},
		{name: "mul most negative", operator: wir.BinMul, left: math.MinInt64, right: -1},
		{name: "idiv by zero", operator: wir.BinIDiv, left: 1, right: 0},
		{name: "idiv most negative", operator: wir.BinIDiv, left: math.MinInt64, right: -1},
		{name: "mod by zero", operator: wir.BinMod, left: 1, right: 0},
		{name: "add", operator: wir.BinAdd, left: 10, right: 5, representation: "integer", text: "15", exact: true},
		{name: "sub", operator: wir.BinSub, left: math.MaxInt64, right: math.MaxInt64, representation: "integer", text: "0", exact: true},
		{name: "mul", operator: wir.BinMul, left: -3, right: 7, representation: "integer", text: "-21", exact: true},
		{name: "floor div negative dividend", operator: wir.BinIDiv, left: -7, right: 2, representation: "integer", text: "-4", exact: true},
		{name: "floor div negative divisor", operator: wir.BinIDiv, left: 7, right: -2, representation: "integer", text: "-4", exact: true},
		{name: "mod negative dividend", operator: wir.BinMod, left: -7, right: 2, representation: "integer", text: "1", exact: true},
		{name: "mod negative divisor", operator: wir.BinMod, left: 7, right: -2, representation: "integer", text: "-1", exact: true},
		{name: "mod most negative", operator: wir.BinMod, left: math.MinInt64, right: -1, representation: "integer", text: "0", exact: true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			word, ok := nativeFoldIntegerArithmetic(item.operator, item.left, item.right)
			if ok != item.exact {
				t.Fatalf("exact=%v, want %v (word %v)", ok, item.exact, word)
			}
			if item.exact && (word.representation != item.representation || word.text != item.text) {
				t.Fatalf("word = %s %s, want %s %s", word.representation, word.text, item.representation, item.text)
			}
		})
	}
}

func TestFloatWordWithholdsNonFiniteResults(t *testing.T) {
	for _, value := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		if word, ok := nativeFloatWord(value); ok {
			t.Fatalf("non-finite %v published %v, want the word withheld", value, word)
		}
	}
	word, ok := nativeFloatWord(3)
	if !ok || word.text != "3.0" {
		t.Fatalf("integral float = %v (%v), want the float spelling 3.0", word.text, ok)
	}
}

func TestFloatValueWithholdsInexactIntegerConversion(t *testing.T) {
	word, _ := nativeIntegerWord(1<<53 + 1)
	if value, ok := word.floatValue(); ok {
		t.Fatalf("integer beyond the mantissa converted to %v, want the conversion withheld", value)
	}
	word, _ = nativeIntegerWord(1 << 52)
	if value, ok := word.floatValue(); !ok || value != float64(1<<52) {
		t.Fatalf("exact conversion = %v (%v), want %v", value, ok, float64(1<<52))
	}
}
