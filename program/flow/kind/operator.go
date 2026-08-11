package kind

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

// SelectOp is Lua's short-circuit value-selection vocabulary.
type SelectOp uint8

const (
	SelectAnd SelectOp = iota + 1
	SelectOr
)
