package flow

import "github.com/wippyai/go-lua/types/cfg"

// ValueSlot identifies one logical value slot in PointState.
//
// A symbol slot may live in Env or Cells depending on the transfer-owned
// lexical storage policy. A key slot is an ordinary non-symbol Env key such as a
// return slot. This keeps consumers from decoding ValueKey strings to recover
// storage identity.
type ValueSlot struct {
	symbol cfg.SymbolID
	key    ValueKey
}

// SymbolValueSlot identifies the logical slot for a CFG symbol.
func SymbolValueSlot(sym cfg.SymbolID) (ValueSlot, bool) {
	if sym == 0 {
		return ValueSlot{}, false
	}
	return ValueSlot{symbol: sym}, true
}

// ValueKeySlot identifies the logical slot named by key. Symbol-shaped keys are
// normalized to symbol slots so callers can apply their symbol storage policy.
func ValueKeySlot(key ValueKey) (ValueSlot, bool) {
	if key == "" {
		return ValueSlot{}, false
	}
	if sym, ok := ParseSymbolValueKey(key); ok {
		return SymbolValueSlot(sym)
	}
	return ValueSlot{key: key}, true
}

// Symbol returns the symbol for a symbol slot.
func (s ValueSlot) Symbol() (cfg.SymbolID, bool) {
	return s.symbol, s.symbol != 0
}

// Key returns the non-symbol Env key for a key slot.
func (s ValueSlot) Key() (ValueKey, bool) {
	return s.key, s.symbol == 0 && s.key != ""
}

// ValueKey returns the canonical Env key spelling of the slot.
func (s ValueSlot) ValueKey() (ValueKey, bool) {
	if s.symbol != 0 {
		return SymbolValueKey(s.symbol), true
	}
	return s.Key()
}

// Equal reports whether two slots identify the same logical value.
func (s ValueSlot) Equal(other ValueSlot) bool {
	return s.symbol == other.symbol && s.key == other.key
}
