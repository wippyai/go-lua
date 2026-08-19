// Package kind owns the closed semantic vocabulary shared by Flow's typed
// construction and later sealed projections.
//
// This leaf contains vocabulary only. It does not own rows, storage, identity,
// builders, components, or views.
package kind

// CellRole is the closed lexical-storage definition vocabulary. Every sealed
// Cell has exactly one role; roles are derived from typed relations rather
// than supplied as mutable flags.
type CellRole uint8

const (
	CellGlobal CellRole = iota + 1
	CellLocal
	CellFormal
	CellFunctionVararg
	CellLoop
	CellCapture
	CellChunkVararg
)

// FieldKind is the sole closed Lua table-constructor field vocabulary.
type FieldKind uint8

const (
	FieldList FieldKind = iota + 1
	FieldName
	FieldExact
	FieldKey
)

// LoopKind is the closed authored Lua loop vocabulary. Its execution edges
// and exits are derived only by the whole-Flow finalizer.
type LoopKind uint8

const (
	LoopWhile LoopKind = iota + 1
	LoopRepeat
	LoopNumericFor
	LoopGenericFor
)

// UnaryOp is the closed Lua unary operator vocabulary.
type UnaryOp uint8

const (
	UnaryNeg UnaryOp = iota + 1
	UnaryNot
	UnaryLen
	UnaryBitNot
)

// BinaryOp is the closed Lua binary operator vocabulary.
type BinaryOp uint8

const (
	BinaryAdd BinaryOp = iota + 1
	BinarySub
	BinaryMul
	BinaryDiv
	BinaryIDiv
	BinaryMod
	BinaryPow
	BinaryConcat
	BinaryBitAnd
	BinaryBitOr
	BinaryBitXor
	BinaryShiftLeft
	BinaryShiftRight
	BinaryEqual
	BinaryNotEqual
	BinaryLess
	BinaryLessEqual
	BinaryGreater
	BinaryGreaterEqual
)

// IsBinaryArithmetic reports whether op is one of the closed primitive
// arithmetic operators. The arithmetic members occupy the first contiguous
// segment of the BinaryOp vocabulary; all other members are distinct binary
// operation families.
func IsBinaryArithmetic(op BinaryOp) bool {
	return op >= BinaryAdd && op <= BinaryPow
}

// IsBinaryOrder reports whether op is one of the closed primitive relational
// order operators.
func IsBinaryOrder(op BinaryOp) bool {
	return op >= BinaryLess && op <= BinaryGreaterEqual
}

// SelectOp is Lua's short-circuit value-selection vocabulary.
type SelectOp uint8

const (
	SelectAnd SelectOp = iota + 1
	SelectOr
)

// ValueClaimKind preserves the exact authored spelling of a scalar value
// claim. Validation, narrowing, guards, and outcome behavior are later
// derived relations, not authored Flow content.
type ValueClaimKind uint8

const (
	ValueClaimTypeAs ValueClaimKind = iota + 1
	ValueClaimTypeColonColon
	ValueClaimNonNil
)

// OutcomeKind is the closed authored control-outcome vocabulary. Its numeric
// ordinals are part of the canonical Flow representation; keep them explicit
// and append-only rather than deriving them from declaration order.
type OutcomeKind uint8

const (
	OutcomeNormal OutcomeKind = 1
	OutcomeReturn OutcomeKind = 2
	OutcomeThrow  OutcomeKind = 3
	OutcomeBreak  OutcomeKind = 4
	OutcomeGoto   OutcomeKind = 5
	OutcomeYield  OutcomeKind = 6
	OutcomeCancel OutcomeKind = 7
)
