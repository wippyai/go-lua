package flow

import "github.com/wippyai/go-lua/types/cfg"

// AnnotatedSymbols is the finite root-symbol set with explicit source
// annotations. It is separate from DeclaredTypes because not every declared
// type is annotation authority: binding values and inferred declarations must
// not seal mutable shapes or suppress dynamic defaults.
type AnnotatedSymbols struct {
	symbols cfgSymbolSet
}

// AnnotatedSymbolsFromMap normalizes a legacy symbol -> bool map into the
// annotation-authority carrier.
func AnnotatedSymbolsFromMap(in map[cfg.SymbolID]bool) AnnotatedSymbols {
	var out AnnotatedSymbols
	for sym, ok := range in {
		if ok {
			out.Add(sym)
		}
	}
	return out
}

// Add records sym as an explicitly annotated root symbol.
func (s *AnnotatedSymbols) Add(sym cfg.SymbolID) bool {
	return s.symbols.Add(sym)
}

// Contains reports whether sym has explicit source annotation authority.
func (s AnnotatedSymbols) Contains(sym cfg.SymbolID) bool {
	return s.symbols.Contains(sym)
}

func annotatedSymbolsEqual(a, b AnnotatedSymbols) bool {
	if a.symbols.Len() != b.symbols.Len() {
		return false
	}
	for sym := range a.symbols.seen {
		if !b.Contains(sym) {
			return false
		}
	}
	return true
}
