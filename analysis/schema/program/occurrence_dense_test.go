package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestOccurrenceDenseRowsValidateClosedSemanticsAndOperands(t *testing.T) {
	first := identity.ContentID{1}
	second := identity.ContentID{2}
	firstInput, firstInputOK := NewOccurrenceInput(first)
	secondInput, secondInputOK := NewOccurrenceInput(second)
	if !firstInputOK || !secondInputOK {
		t.Fatal("failed to construct occurrence input fixtures")
	}
	inputs := []OccurrenceInput{firstInput, secondInput}
	code, codeOK := OccurrenceBinaryEqualityCode(flowkind.BinaryNotEqual, false, true)
	row, rowOK := NewOccurrence(OccurrenceBinaryEquality, first, identity.ContentID{}, code, 0, 0, 0, 2, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	if !codeOK || !rowOK || !OccurrenceDenseAvailable(row, nil, inputs) {
		t.Fatal("valid dense equality row was rejected")
	}
	left, right, op, operandsOK := OccurrenceBinaryEqualityOperands(row, inputs)
	if !operandsOK || left != first || right != second || op != flowkind.BinaryNotEqual {
		t.Fatal("equality operands did not preserve canonical order and opcode")
	}
	if OccurrenceDenseAvailable(row, nil, inputs[:1]) {
		t.Fatal("dense equality row accepted a truncated operand plane")
	}

	for _, candidate := range []struct {
		op            flowkind.BinaryOp
		hasComparison bool
		invert        bool
	}{
		{flowkind.BinaryEqual, false, true},
		{flowkind.BinaryNotEqual, false, false},
		{flowkind.BinaryGreater, false, false},
	} {
		if _, accepted := OccurrenceBinaryEqualityCode(candidate.op, candidate.hasComparison, candidate.invert); accepted {
			t.Fatalf("accepted non-canonical equality encoding: %+v", candidate)
		}
	}
}

func TestOccurrenceDenseOperandHelpersRejectWrongKind(t *testing.T) {
	id := identity.ContentID{1}
	input, inputOK := NewOccurrenceInput(id)
	if !inputOK {
		t.Fatal("failed to construct occurrence input fixture")
	}
	row, rowOK := NewOccurrence(OccurrenceUnary, id, identity.ContentID{}, 0, 0, 0, 0, 1, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
	if !rowOK {
		t.Fatal("failed to construct occurrence fixture")
	}
	if _, _, _, operandsOK := OccurrenceBinaryArithmeticOperands(row, []OccurrenceInput{input}); operandsOK {
		t.Fatal("binary arithmetic helper accepted a non-arithmetic row")
	}
	if _, _, operandsOK := OccurrenceStorageReadOperands(row, []OccurrenceInput{input}); operandsOK {
		t.Fatal("storage-read helper accepted a non-storage row")
	}
}

func TestOccurrenceOperationPredicateCodeOwnsTruthEncoding(t *testing.T) {
	falseCode, falseOK := OccurrenceOperationPredicateCode(flowkind.BinaryEqual, false)
	trueCode, trueOK := OccurrenceOperationPredicateCode(flowkind.BinaryEqual, true)
	if !falseOK || !trueOK || trueCode != falseCode|(uint64(1)<<8) {
		t.Fatalf("unexpected operation-predicate truth encoding: false=%d true=%d", falseCode, trueCode)
	}
	if _, accepted := OccurrenceOperationPredicateCode(flowkind.BinaryGreater, true); accepted {
		t.Fatal("accepted a non-equality operation-predicate opcode")
	}

	body := identity.ContentID{2}
	valid, validOK := NewOccurrence(
		OccurrenceOperationPredicateRefinement,
		identity.ContentID{1}, body, trueCode,
		0, 1, 0, 5,
		keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	if !validOK || !OccurrenceSemanticAvailable(valid) {
		t.Fatal("schema-owned operation-predicate encoding was rejected")
	}
	invalid := valid
	invalid.code = trueCode | (uint64(1) << 9)
	if OccurrenceSemanticAvailable(invalid) {
		t.Fatal("operation-predicate row accepted an unknown code bit")
	}
}
