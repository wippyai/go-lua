package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldTypeMerger merges an incoming field type with an existing field type.
// When prev is nil, next is a new field value. The carriers store interned
// product.AbstractValue; this merger operates on the projected structural types
// so the rich convergence-merge precision is preserved, and the result lifts
// back onto the carrier.
type FieldTypeMerger func(prev typ.Type, next typ.Type) typ.Type

// MergeCapturedFieldSymbolMaps merges captured-field maps keyed by captured symbol.
// Structure: capturedSymbol -> fieldName -> fieldType.
func MergeCapturedFieldSymbolMaps(
	existing map[cfg.SymbolID]map[string]product.AbstractValue,
	next map[cfg.SymbolID]map[string]product.AbstractValue,
	merge FieldTypeMerger,
) map[cfg.SymbolID]map[string]product.AbstractValue {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}

	mergeFn := merge
	if mergeFn == nil {
		mergeFn = func(prev typ.Type, n typ.Type) typ.Type {
			if prev != nil {
				return prev
			}
			return n
		}
	}

	merged := make(map[cfg.SymbolID]map[string]product.AbstractValue, len(existing)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(existing) {
		merged[sym] = existing[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		fields := next[sym]
		existingFields := merged[sym]
		if existingFields == nil {
			merged[sym] = fields
			continue
		}
		out := make(map[string]product.AbstractValue, len(existingFields)+len(fields))
		for _, name := range cfg.SortedFieldNames(existingFields) {
			out[name] = existingFields[name]
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			out[name] = liftCarrier(mergeFn(projectCarrier(out[name]), projectCarrier(fields[name])))
		}
		merged[sym] = out
	}
	return merged
}
