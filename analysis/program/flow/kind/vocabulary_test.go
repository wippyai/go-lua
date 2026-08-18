package kind

import "testing"

func TestCanonicalVocabularyOrdinals(t *testing.T) {
	cases := []struct {
		name string
		got  uint8
		want uint8
	}{
		{"FieldList", uint8(FieldList), 1},
		{"FieldName", uint8(FieldName), 2},
		{"FieldExact", uint8(FieldExact), 3},
		{"FieldKey", uint8(FieldKey), 4},

		{"LoopWhile", uint8(LoopWhile), 1},
		{"LoopRepeat", uint8(LoopRepeat), 2},
		{"LoopNumericFor", uint8(LoopNumericFor), 3},
		{"LoopGenericFor", uint8(LoopGenericFor), 4},

		{"UnaryNeg", uint8(UnaryNeg), 1},
		{"UnaryNot", uint8(UnaryNot), 2},
		{"UnaryLen", uint8(UnaryLen), 3},
		{"UnaryBitNot", uint8(UnaryBitNot), 4},

		{"BinaryAdd", uint8(BinaryAdd), 1},
		{"BinarySub", uint8(BinarySub), 2},
		{"BinaryMul", uint8(BinaryMul), 3},
		{"BinaryDiv", uint8(BinaryDiv), 4},
		{"BinaryIDiv", uint8(BinaryIDiv), 5},
		{"BinaryMod", uint8(BinaryMod), 6},
		{"BinaryPow", uint8(BinaryPow), 7},
		{"BinaryConcat", uint8(BinaryConcat), 8},
		{"BinaryBitAnd", uint8(BinaryBitAnd), 9},
		{"BinaryBitOr", uint8(BinaryBitOr), 10},
		{"BinaryBitXor", uint8(BinaryBitXor), 11},
		{"BinaryShiftLeft", uint8(BinaryShiftLeft), 12},
		{"BinaryShiftRight", uint8(BinaryShiftRight), 13},
		{"BinaryEqual", uint8(BinaryEqual), 14},
		{"BinaryNotEqual", uint8(BinaryNotEqual), 15},
		{"BinaryLess", uint8(BinaryLess), 16},
		{"BinaryLessEqual", uint8(BinaryLessEqual), 17},
		{"BinaryGreater", uint8(BinaryGreater), 18},
		{"BinaryGreaterEqual", uint8(BinaryGreaterEqual), 19},

		{"SelectAnd", uint8(SelectAnd), 1},
		{"SelectOr", uint8(SelectOr), 2},

		{"ValueClaimTypeAs", uint8(ValueClaimTypeAs), 1},
		{"ValueClaimTypeColonColon", uint8(ValueClaimTypeColonColon), 2},
		{"ValueClaimNonNil", uint8(ValueClaimNonNil), 3},

		{"OutcomeNormal", uint8(OutcomeNormal), 1},
		{"OutcomeReturn", uint8(OutcomeReturn), 2},
		{"OutcomeThrow", uint8(OutcomeThrow), 3},
		{"OutcomeBreak", uint8(OutcomeBreak), 4},
		{"OutcomeGoto", uint8(OutcomeGoto), 5},
		{"OutcomeYield", uint8(OutcomeYield), 6},
		{"OutcomeCancel", uint8(OutcomeCancel), 7},

		{"CellGlobal", uint8(CellGlobal), 1},
		{"CellLocal", uint8(CellLocal), 2},
		{"CellFormal", uint8(CellFormal), 3},
		{"CellFunctionVararg", uint8(CellFunctionVararg), 4},
		{"CellLoop", uint8(CellLoop), 5},
		{"CellCapture", uint8(CellCapture), 6},
		{"CellChunkVararg", uint8(CellChunkVararg), 7},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("%s = %d, want %d", test.name, test.got, test.want)
			}
		})
	}
}

func TestCanonicalVocabularyZeroIsInvalid(t *testing.T) {
	cases := []struct {
		name   string
		zero   uint8
		values []uint8
	}{
		{"FieldKind", uint8(FieldKind(0)), []uint8{uint8(FieldList), uint8(FieldName), uint8(FieldExact), uint8(FieldKey)}},
		{"LoopKind", uint8(LoopKind(0)), []uint8{uint8(LoopWhile), uint8(LoopRepeat), uint8(LoopNumericFor), uint8(LoopGenericFor)}},
		{"UnaryOp", uint8(UnaryOp(0)), []uint8{uint8(UnaryNeg), uint8(UnaryNot), uint8(UnaryLen), uint8(UnaryBitNot)}},
		{"BinaryOp", uint8(BinaryOp(0)), []uint8{uint8(BinaryAdd), uint8(BinarySub), uint8(BinaryMul), uint8(BinaryDiv), uint8(BinaryIDiv), uint8(BinaryMod), uint8(BinaryPow), uint8(BinaryConcat), uint8(BinaryBitAnd), uint8(BinaryBitOr), uint8(BinaryBitXor), uint8(BinaryShiftLeft), uint8(BinaryShiftRight), uint8(BinaryEqual), uint8(BinaryNotEqual), uint8(BinaryLess), uint8(BinaryLessEqual), uint8(BinaryGreater), uint8(BinaryGreaterEqual)}},
		{"SelectOp", uint8(SelectOp(0)), []uint8{uint8(SelectAnd), uint8(SelectOr)}},
		{"ValueClaimKind", uint8(ValueClaimKind(0)), []uint8{uint8(ValueClaimTypeAs), uint8(ValueClaimTypeColonColon), uint8(ValueClaimNonNil)}},
		{"OutcomeKind", uint8(OutcomeKind(0)), []uint8{uint8(OutcomeNormal), uint8(OutcomeReturn), uint8(OutcomeThrow), uint8(OutcomeBreak), uint8(OutcomeGoto), uint8(OutcomeYield), uint8(OutcomeCancel)}},
		{"CellRole", uint8(CellRole(0)), []uint8{uint8(CellGlobal), uint8(CellLocal), uint8(CellFormal), uint8(CellFunctionVararg), uint8(CellLoop), uint8(CellCapture), uint8(CellChunkVararg)}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.zero != 0 {
				t.Fatalf("zero %s = %d, want 0", test.name, test.zero)
			}
			for _, value := range test.values {
				if test.zero == value {
					t.Fatalf("zero %s collides with canonical value %d", test.name, value)
				}
			}
		})
	}
}

func TestBinaryArithmeticMembershipRejectsOtherBinaryFamilies(t *testing.T) {
	for op := BinaryAdd; op <= BinaryPow; op++ {
		if !IsBinaryArithmetic(op) {
			t.Fatalf("arithmetic operator %d was rejected", op)
		}
	}
	for _, op := range []BinaryOp{
		0,
		BinaryConcat,
		BinaryBitAnd,
		BinaryBitOr,
		BinaryBitXor,
		BinaryShiftLeft,
		BinaryShiftRight,
		BinaryEqual,
		BinaryNotEqual,
		BinaryLess,
		BinaryLessEqual,
		BinaryGreater,
		BinaryGreaterEqual,
		BinaryOp(255),
	} {
		if IsBinaryArithmetic(op) {
			t.Fatalf("non-arithmetic operator %d was admitted", op)
		}
	}
}
