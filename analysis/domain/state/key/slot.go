package key

import "github.com/wippyai/go-lua/analysis/symbol"

// Slot identifies one logical value slot in abstract state.
type Slot struct {
	symbol symbol.ID
	key    Value
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
