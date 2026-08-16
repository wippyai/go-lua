// Package candidate owns the closed source-Term candidate contract. Candidate
// relations are Program projections on existing Unary/Binary/Read/Write/Call
// Terms; no dispatch handle, second Call, or runtime allocation fact exists.
package candidate

import flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"

// Family identifies one closed Lua candidate relation with its own exact
// operand and outcome shape.
type Family uint8

const (
	FamilyInvalid Family = iota
	FamilyUnaryNumeric
	FamilyLength
	FamilyArithmetic
	FamilyBitwise
	FamilyConcat
	FamilyEquality
	FamilyOrder
	FamilyIndexGet
	FamilyIndexSet
	FamilyCallable
)

// Branch identifies one semantically distinct candidate route. Raw routes are
// deliberately split where Lua's language law differs; there is no generic
// "raw" label that erases length's string/table or assignment's present-key
// distinction.
type Branch uint8

const (
	BranchInvalid Branch = iota
	BranchPrimitive
	BranchStringRaw
	BranchTableRaw
	BranchRawPresent
	BranchMeta
	BranchFallback
	BranchError
	BranchDirect
)

// Subject is the exact authored Program operation that owns a candidate
// family. Unary and Binary coordinates are populated only by their matching
// families; index and callable families are already unique by Term kind.
type Subject struct {
	Family Family
	Unary  flowkind.UnaryOp
	Binary flowkind.BinaryOp
}

// Requirement is one exact source-Term candidate branch. A branch helper must
// retain the same source Term and exact typed operands/outcomes; it cannot
// re-lower or construct a second application occurrence.
type Requirement struct {
	Subject Subject
	Branch  Branch
}

var unaryNumeric = [...]flowkind.UnaryOp{flowkind.UnaryNeg, flowkind.UnaryBitNot}
var arithmetic = [...]flowkind.BinaryOp{
	flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv,
	flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow,
}
var bitwise = [...]flowkind.BinaryOp{
	flowkind.BinaryBitAnd, flowkind.BinaryBitOr, flowkind.BinaryBitXor,
	flowkind.BinaryShiftLeft, flowkind.BinaryShiftRight,
}
var equality = [...]flowkind.BinaryOp{flowkind.BinaryEqual, flowkind.BinaryNotEqual}
var order = [...]flowkind.BinaryOp{
	flowkind.BinaryLess, flowkind.BinaryLessEqual,
	flowkind.BinaryGreater, flowkind.BinaryGreaterEqual,
}

// Requirements derives the finite candidate contract from Lua's closed
// source-operation vocabulary. It is independent of fixtures and Program
// counts. Logical `not`, `and`, and `or` do not appear because they are
// non-dispatching language operations, covered by ordinary Program laws.
func Requirements() []Requirement {
	rows := make([]Requirement, 0, 79)
	appendBranches := func(subject Subject, branches ...Branch) {
		for _, branch := range branches {
			rows = append(rows, Requirement{Subject: subject, Branch: branch})
		}
	}
	for _, op := range unaryNumeric {
		appendBranches(Subject{Family: FamilyUnaryNumeric, Unary: op}, BranchPrimitive, BranchMeta, BranchError)
	}
	appendBranches(Subject{Family: FamilyLength, Unary: flowkind.UnaryLen}, BranchStringRaw, BranchTableRaw, BranchMeta, BranchError)
	for _, op := range arithmetic {
		appendBranches(Subject{Family: FamilyArithmetic, Binary: op}, BranchPrimitive, BranchMeta, BranchError)
	}
	for _, op := range bitwise {
		appendBranches(Subject{Family: FamilyBitwise, Binary: op}, BranchPrimitive, BranchMeta, BranchError)
	}
	appendBranches(Subject{Family: FamilyConcat, Binary: flowkind.BinaryConcat}, BranchPrimitive, BranchMeta, BranchError)
	for _, op := range equality {
		appendBranches(Subject{Family: FamilyEquality, Binary: op}, BranchPrimitive, BranchMeta)
	}
	for _, op := range order {
		appendBranches(Subject{Family: FamilyOrder, Binary: op}, BranchPrimitive, BranchMeta, BranchError)
		if op == flowkind.BinaryLessEqual || op == flowkind.BinaryGreaterEqual {
			appendBranches(Subject{Family: FamilyOrder, Binary: op}, BranchFallback)
		}
	}
	appendBranches(Subject{Family: FamilyIndexGet}, BranchRawPresent, BranchMeta, BranchFallback, BranchError)
	appendBranches(Subject{Family: FamilyIndexSet}, BranchRawPresent, BranchPrimitive, BranchMeta, BranchFallback, BranchError)
	appendBranches(Subject{Family: FamilyCallable}, BranchDirect, BranchMeta, BranchError)
	return rows
}
