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
	valueKindShift  = 62
	valueNumberMask = (uint64(1) << valueKindShift) - 1

	valueKindSymbol uint64 = 1
	valueKindReturn uint64 = 2
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
