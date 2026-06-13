package key

import "github.com/wippyai/go-lua/analysis/symbol"

// Slot identifies one logical value slot in abstract state.
type Slot struct {
	symbol symbol.ID
	key    Value
}

// SymbolSlot returns the canonical slot for a symbol value.
func SymbolSlot(sym symbol.ID) (Slot, bool) {
	if sym == 0 {
		return Slot{}, false
	}
	return Slot{symbol: sym}, true
}

// KeySlot returns a slot for a non-symbol value key.
func KeySlot(key Value) (Slot, bool) {
	if key == "" {
		return Slot{}, false
	}
	if _, ok := ParseSymbolValue(key); ok {
		return Slot{}, false
	}
	return Slot{key: key}, true
}

// SlotOfValue returns the canonical slot for a value key.
func SlotOfValue(value Value) (Slot, bool) {
	if sym, ok := ParseSymbolValue(value); ok {
		return SymbolSlot(sym)
	}
	return KeySlot(value)
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
