package key

import (
	"github.com/wippyai/go-lua/analysis/internal/keycodec"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Value identifies one value cell in an abstract state.
type Value string

// SymbolValue identifies the current point-local value of a symbol.
func SymbolValue(sym symbol.ID) Value {
	if sym == 0 {
		return ""
	}
	return Value(keycodec.PrefixedDecimalKey('s', uint64(sym), ""))
}

// ReturnSlot identifies a non-symbol return value slot.
func ReturnSlot(index int) Value {
	if index < 0 {
		return ""
	}
	return Value(keycodec.PrefixedDecimalKey('r', uint64(index), ""))
}

// ParseSymbolValue inverts SymbolValue.
func ParseSymbolValue(value Value) (symbol.ID, bool) {
	s := string(value)
	if len(s) < 2 || s[0] != 's' {
		return 0, false
	}
	n, ok := keycodec.ParseUnsignedDecimal(s[1:])
	if !ok || n == 0 {
		return 0, false
	}
	return symbol.ID(n), true
}

// ParseReturnSlot inverts ReturnSlot.
func ParseReturnSlot(value Value) (int, bool) {
	s := string(value)
	if len(s) < 2 || s[0] != 'r' {
		return 0, false
	}
	n, ok := keycodec.ParseUnsignedDecimal(s[1:])
	if !ok || n > uint64(int(^uint(0)>>1)) {
		return 0, false
	}
	return int(n), true
}
