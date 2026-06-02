package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldTypeMerger merges an incoming field type with an existing field type.
// When prev is nil, next is a new field value. The carriers store interned
// product.AbstractValue; this merger operates on the projected structural types
// so the rich convergence-merge precision is preserved, and the result lifts
// back onto the carrier.
type FieldTypeMerger func(prev typ.Type, next typ.Type) typ.Type

// MergeCapturedFieldSymbolMaps merges captured-field maps keyed by captured symbol.
// Structure: capturedSymbol -> fieldKey -> fieldType.
func MergeCapturedFieldSymbolMaps(
	existing map[cfg.SymbolID]FieldValues,
	next map[cfg.SymbolID]FieldValues,
	merge FieldTypeMerger,
) map[cfg.SymbolID]FieldValues {
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

	merged := make(map[cfg.SymbolID]FieldValues, len(existing)+len(next))
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
		out := make(FieldValues, len(existingFields)+len(fields))
		for _, key := range SortedFieldKeys(existingFields) {
			out[key] = existingFields[key]
		}
		for _, key := range SortedFieldKeys(fields) {
			out[key] = liftCarrier(mergeFn(projectCarrier(out[key]), projectCarrier(fields[key])))
		}
		merged[sym] = out
	}
	return merged
}
