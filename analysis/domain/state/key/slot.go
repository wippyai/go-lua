package key

import "github.com/wippyai/go-lua/analysis/symbol"

// Slot identifies one logical value slot in abstract state.
type Slot struct {
	symbol symbol.ID
	key    Value
}

// SymbolSlot identifies the logical slot for a symbol.
func SymbolSlot(sym symbol.ID) (Slot, bool) {
	if sym == 0 {
		return Slot{}, false
	}
	return Slot{symbol: sym}, true
}

// ValueSlot identifies the logical slot named by key. Symbol-shaped keys are
// normalized to symbol slots so storage policy can stay symbol-aware.
func ValueSlot(key Value) (Slot, bool) {
	if key == "" {
		return Slot{}, false
	}
	if sym, ok := ParseSymbolValue(key); ok {
		return SymbolSlot(sym)
	}
	return Slot{key: key}, true
}

// Symbol returns the symbol for a symbol slot.
func (s Slot) Symbol() (symbol.ID, bool) {
	return s.symbol, s.symbol != 0
}

// Key returns the non-symbol key for a key slot.
func (s Slot) Key() (Value, bool) {
	return s.key, s.symbol == 0 && s.key != ""
}

// Value returns the canonical value key spelling of the slot.
func (s Slot) Value() (Value, bool) {
	if s.symbol != 0 {
		return SymbolValue(s.symbol), true
	}
	return s.Key()
}

// Equal reports whether two slots identify the same logical value.
func (s Slot) Equal(other Slot) bool {
	return s.symbol == other.symbol && s.key == other.key
}
