package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldTypeMerger merges an incoming type written at key with an existing one.
// When prev is nil, next is a new field value.
type FieldTypeMerger func(key api.FieldWriteKey, prev typ.Type, next typ.Type) typ.Type

// MergeFieldWriteSymbolMaps merges field-write maps keyed by target symbol.
// Structure: targetSymbol -> FieldWriteKey -> fieldType.
func MergeFieldWriteSymbolMaps(
	existing map[cfg.SymbolID]api.FieldWriteSet,
	next map[cfg.SymbolID]api.FieldWriteSet,
	merge FieldTypeMerger,
) map[cfg.SymbolID]api.FieldWriteSet {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}

	mergeFn := merge
	if mergeFn == nil {
		mergeFn = func(_ api.FieldWriteKey, prev typ.Type, n typ.Type) typ.Type {
			if prev != nil {
				return prev
			}
			return n
		}
	}

	merged := make(map[cfg.SymbolID]api.FieldWriteSet, len(existing)+len(next))
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
		out := make(api.FieldWriteSet, len(existingFields)+len(fields))
		for _, key := range api.SortedFieldWriteKeys(existingFields) {
			out[key] = existingFields[key]
		}
		for _, key := range api.SortedFieldWriteKeys(fields) {
			out[key] = mergeFn(key, out[key], fields[key])
		}
		merged[sym] = out
	}
	return merged
}
