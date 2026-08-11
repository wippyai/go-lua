package binaryprimitive

import (
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func (view Arithmetic) Count() int {
	if view.result == nil || !view.result.available() {
		return 0
	}
	return len(view.result.buckets.arithmetic)
}

func (view Arithmetic) At(index int) (keyspace.Term, bool) {
	return view.result.bucketAt(view.result.buckets.arithmetic, index, kindBinaryArithmetic)
}

func (view Bitwise) Count() int {
	if view.result == nil || !view.result.available() {
		return 0
	}
	return len(view.result.buckets.bitwise)
}

func (view Bitwise) At(index int) (keyspace.Term, bool) {
	return view.result.bucketAt(view.result.buckets.bitwise, index, kindBinaryBitwise)
}

func (view Equality) Count() int {
	if view.result == nil || !view.result.available() {
		return 0
	}
	return len(view.result.buckets.equality)
}

func (view Equality) At(index int) (keyspace.Term, bool) {
	return view.result.bucketAt(view.result.buckets.equality, index, kindBinaryEquality)
}

func (view Order) Count() int {
	if view.result == nil || !view.result.available() {
		return 0
	}
	return len(view.result.buckets.order)
}

func (view Order) At(index int) (keyspace.Term, bool) {
	return view.result.bucketAt(view.result.buckets.order, index, kindBinaryOrder)
}

func (r *Result) bucketAt(bucket []keyspace.Term, index int, category binaryCategory) (keyspace.Term, bool) {
	if !r.available() || index < 0 || index >= len(bucket) {
		return 0, false
	}
	term := bucket[index]
	if !keyspace.ValidTerm(term, keyspace.FamilyBinary, len(r.slots)) {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.slots)) || r.slots[ordinal] == 0 {
		return 0, false
	}
	rowIndex := r.slots[ordinal] - 1
	if uint64(rowIndex) >= uint64(len(r.primitives)) || r.primitives[rowIndex].source != term ||
		binaryCategoryFor(r.primitives[rowIndex].operation.Op) != category {
		return 0, false
	}
	return term, true
}

// Primitive returns the opaque retained handle for one executable primitive
// Binary. Non-primitive candidates, dead Binaries, wrong families, and rows
// outside the sealed denominator fail closed.
func (r *Result) Primitive(binary keyspace.Term) (Primitive, bool) {
	if !r.available() || keyspace.TermFamily(binary) != keyspace.FamilyBinary {
		return Primitive{}, false
	}
	ordinal := keyspace.TermOrdinal(binary)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.slots)) {
		return Primitive{}, false
	}
	slot := r.slots[ordinal]
	if slot == 0 || uint64(slot-1) >= uint64(len(r.primitives)) || r.primitives[slot-1].source != binary {
		return Primitive{}, false
	}
	return Primitive{result: r, slot: slot}, true
}

// Source returns the authored Binary identity denoted by this handle.
func (primitive Primitive) Source() (keyspace.Term, bool) {
	row, ok := primitive.row()
	if !ok {
		return 0, false
	}
	return row.source, true
}

// Operation returns the raw authored operation row.
func (primitive Primitive) Operation() (Operation, bool) {
	row, ok := primitive.row()
	if !ok {
		return Operation{}, false
	}
	return row.operation, true
}

// Comparison returns the exact branch interpretation when this Binary is a
// branch condition. Branchless primitives return ok=false.
func (primitive Primitive) Comparison() (Comparison, bool) {
	row, ok := primitive.row()
	if !ok || !row.hasCompare {
		return Comparison{}, false
	}
	return row.comparison, true
}

func (primitive Primitive) row() (primitiveRow, bool) {
	if primitive.result == nil || !primitive.result.available() || primitive.slot == 0 ||
		uint64(primitive.slot-1) >= uint64(len(primitive.result.primitives)) {
		return primitiveRow{}, false
	}
	row := primitive.result.primitives[primitive.slot-1]
	if row.source == 0 || keyspace.TermFamily(row.source) != keyspace.FamilyBinary ||
		keyspace.TermOrdinal(row.source) == 0 {
		return primitiveRow{}, false
	}
	ordinal := keyspace.TermOrdinal(row.source)
	if uint64(ordinal) >= uint64(len(primitive.result.slots)) || primitive.result.slots[ordinal] != primitive.slot {
		return primitiveRow{}, false
	}
	if !validOperation(row.operation) {
		return primitiveRow{}, false
	}
	if row.hasCompare && !validComparison(row.operation, row.comparison) {
		return primitiveRow{}, false
	}
	return row, true
}

type binaryCategory uint8

const (
	binaryCategoryInvalid binaryCategory = iota
	kindBinaryArithmetic
	kindBinaryBitwise
	kindBinaryEquality
	kindBinaryOrder
)

func binaryCategoryFor(op kind.BinaryOp) binaryCategory {
	switch op {
	case kind.BinaryAdd, kind.BinarySub, kind.BinaryMul, kind.BinaryDiv,
		kind.BinaryIDiv, kind.BinaryMod, kind.BinaryPow:
		return kindBinaryArithmetic
	case kind.BinaryBitAnd, kind.BinaryBitOr, kind.BinaryBitXor,
		kind.BinaryShiftLeft, kind.BinaryShiftRight:
		return kindBinaryBitwise
	case kind.BinaryEqual, kind.BinaryNotEqual:
		return kindBinaryEquality
	case kind.BinaryLess, kind.BinaryLessEqual, kind.BinaryGreater, kind.BinaryGreaterEqual:
		return kindBinaryOrder
	default:
		return binaryCategoryInvalid
	}
}

func validOperation(operation Operation) bool {
	return keyspace.TermFamily(operation.Owner) == keyspace.FamilyBody &&
		keyspace.TermOrdinal(operation.Owner) != 0 && validBinaryOperands(operation.Left, operation.Right) &&
		binaryCategoryFor(operation.Op) != binaryCategoryInvalid
}

func validComparison(operation Operation, comparison Comparison) bool {
	if keyspace.TermFamily(comparison.Branch) != keyspace.FamilyBranch ||
		keyspace.TermOrdinal(comparison.Branch) == 0 ||
		keyspace.TermFamily(comparison.TrueBody) != keyspace.FamilyBody ||
		keyspace.TermOrdinal(comparison.TrueBody) == 0 ||
		keyspace.TermFamily(comparison.FalseBody) != keyspace.FamilyBody ||
		keyspace.TermOrdinal(comparison.FalseBody) == 0 ||
		comparison.TrueBody == comparison.FalseBody || !validOperation(operation) {
		return false
	}
	switch operation.Op {
	case kind.BinaryEqual:
		return !comparison.Invert && comparison.Left == operation.Left && comparison.Right == operation.Right
	case kind.BinaryNotEqual:
		return comparison.Invert && comparison.Left == operation.Left && comparison.Right == operation.Right
	case kind.BinaryLess, kind.BinaryLessEqual:
		return !comparison.Invert && comparison.Left == operation.Left && comparison.Right == operation.Right
	case kind.BinaryGreater, kind.BinaryGreaterEqual:
		return !comparison.Invert && comparison.Left == operation.Right && comparison.Right == operation.Left
	default:
		return false
	}
}
