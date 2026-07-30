package key

import (
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Value identifies one value cell in an abstract state. It is a packed,
// comparable identity (a kind tag plus a number), not a string: value cells are
// only ever compared and used as map keys, never serialized or displayed, so an
// integer identity removes the decimal-string encoding and string-keyed map
// hashing the previous representation paid on every lookup. The zero Value is
// the empty/invalid cell.
type Value uint64

const (
	valueKindShift  = 61
	valueNumberMask = (uint64(1) << valueKindShift) - 1

	valueKindSymbol     uint64 = 1
	valueKindReturn     uint64 = 2
	valueKindExpression uint64 = 3
	valueKindCallResult uint64 = 4

	callResultSlotBits = 29
	callResultSlotMask = (uint64(1) << callResultSlotBits) - 1
)

func packValue(kind, number uint64) Value {
	return Value(kind<<valueKindShift | (number & valueNumberMask))
}

func (v Value) kind() uint64 { return uint64(v) >> valueKindShift }

func (v Value) number() uint64 { return uint64(v) & valueNumberMask }

// SymbolValue identifies the current point-local value of a symbol.
func SymbolValue(sym symbol.ID) Value {
	if sym == 0 {
		return 0
	}
	return packValue(valueKindSymbol, uint64(sym))
}

// ReturnSlot identifies a non-symbol return value slot.
func ReturnSlot(index int) Value {
	if index < 0 {
		return 0
	}
	return packValue(valueKindReturn, uint64(index))
}

// ExpressionValue identifies the transient result cell of one structurally
// certified expression. The cell is written on every incoming edge of the
// expression's CFG join, so consumers read a single ordinary State value
// without evaluating an untaken short-circuit operand.
func ExpressionValue(expression uint32) Value {
	if expression == 0 {
		return 0
	}
	return packValue(valueKindExpression, uint64(expression))
}

// CallResult identifies one result coordinate produced by one exact CFG call
// point. Point ownership is part of the cell identity: adjacent calls writing
// the same result ordinal must never alias through a function-wide register.
func CallResult(point, slot uint32) Value {
	if point == 0 || uint64(slot) > callResultSlotMask {
		return 0
	}
	return packValue(valueKindCallResult, uint64(point)<<callResultSlotBits|uint64(slot))
}

// ParseSymbolValue inverts SymbolValue.
func ParseSymbolValue(value Value) (symbol.ID, bool) {
	if value.kind() != valueKindSymbol {
		return 0, false
	}
	n := value.number()
	if n == 0 {
		return 0, false
	}
	return symbol.ID(n), true
}

// ParseReturnSlot inverts ReturnSlot.
func ParseReturnSlot(value Value) (int, bool) {
	if value.kind() != valueKindReturn {
		return 0, false
	}
	return int(value.number()), true
}

// ParseExpressionValue inverts ExpressionValue.
func ParseExpressionValue(value Value) (uint32, bool) {
	if value.kind() != valueKindExpression {
		return 0, false
	}
	n := value.number()
	if n == 0 || n > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(n), true
}

// ParseCallResult inverts CallResult.
func ParseCallResult(value Value) (point, slot uint32, ok bool) {
	if value.kind() != valueKindCallResult {
		return 0, 0, false
	}
	n := value.number()
	point = uint32(n >> callResultSlotBits)
	if point == 0 {
		return 0, 0, false
	}
	return point, uint32(n & callResultSlotMask), true
}
