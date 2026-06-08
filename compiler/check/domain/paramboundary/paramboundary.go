// Package paramboundary owns function-parameter boundary policy.
//
// In particular, it decides which parameter roots are true dynamic defaults:
// unannotated source parameters that may read as gradual any until body evidence
// pins them. Explicit annotations, including `any`, are not dynamic-default
// proof and must stay subject to ordinary obligations.
package paramboundary

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/types/typ"
)

// UnannotatedRootsFromFacts returns parameter roots that have no explicit
// annotation and no resolved declared type.
func UnannotatedRootsFromFacts(
	paramSyms []cfg.SymbolID,
	declared map[cfg.SymbolID]typ.Type,
	annotated map[cfg.SymbolID]bool,
) functionsymbols.Set {
	var set functionsymbols.Set
	for _, sym := range paramSyms {
		if sym == 0 || annotated[sym] {
			continue
		}
		if t, ok := declared[sym]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
			continue
		}
		set.Add(sym)
	}
	return set
}

// UnannotatedRootsBySlot returns parameter roots whose canonical slot has no
// resolved declared type. The slot map is already aligned for implicit method
// receivers, so callers do not need to inspect source parameter indexes.
func UnannotatedRootsBySlot(
	paramSyms []cfg.SymbolID,
	declaredBySlot map[int]typ.Type,
) functionsymbols.Set {
	var set functionsymbols.Set
	for i, sym := range paramSyms {
		if sym == 0 {
			continue
		}
		if t, ok := declaredBySlot[i]; ok && t != nil && !typ.IsAbsentOrUnknown(t) {
			continue
		}
		set.Add(sym)
	}
	return set
}

// SourceUnannotated reports whether sym is a function parameter with no explicit
// source annotation. isAnnotated lets solved fact carriers veto symbols that
// were normalized as annotated outside the raw CFG slot metadata.
func SourceUnannotated(g *cfg.Graph, sym cfg.SymbolID, isAnnotated func(cfg.SymbolID) bool) bool {
	if g == nil || sym == 0 {
		return false
	}
	if isAnnotated != nil && isAnnotated(sym) {
		return false
	}
	for _, slot := range g.ParamSlotsReadOnly() {
		if slot.Symbol != sym {
			continue
		}
		if slot.IsImplicitSelf {
			return true
		}
		return slot.TypeAnnotation == nil
	}
	return false
}
