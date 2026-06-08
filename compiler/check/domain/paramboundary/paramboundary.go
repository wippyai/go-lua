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

// ParameterSlot is the parameter-boundary identity exported to consumers that
// need both slot index and declaration point for one root symbol.
type ParameterSlot struct {
	Index     int
	DeclPoint cfg.Point
}

// ParameterSlots is the normalized symbol -> parameter-boundary lookup for one
// function. It preserves canonical slot indexes, including implicit receivers.
type ParameterSlots struct {
	bySymbol map[cfg.SymbolID]ParameterSlot
}

// ParameterSlotsFromGraph returns the normalized parameter-boundary lookup for g.
func ParameterSlotsFromGraph(g *cfg.Graph) ParameterSlots {
	if g == nil {
		return ParameterSlots{}
	}
	return ParameterSlotsFromSlots(g.ParamSlotsReadOnly())
}

// ParameterSlotsFromSlots normalizes a canonical parameter-slot slice into a
// symbol-keyed boundary lookup.
func ParameterSlotsFromSlots(slots []cfg.ParamSlot) ParameterSlots {
	if len(slots) == 0 {
		return ParameterSlots{}
	}
	out := ParameterSlots{bySymbol: make(map[cfg.SymbolID]ParameterSlot, len(slots))}
	for idx, slot := range slots {
		if slot.Symbol == 0 {
			continue
		}
		out.bySymbol[slot.Symbol] = ParameterSlot{
			Index:     idx,
			DeclPoint: slot.DeclPoint,
		}
	}
	return out
}

// Lookup returns the canonical parameter slot for sym, if sym is a parameter.
func (s ParameterSlots) Lookup(sym cfg.SymbolID) (ParameterSlot, bool) {
	if sym == 0 || s.bySymbol == nil {
		return ParameterSlot{}, false
	}
	slot, ok := s.bySymbol[sym]
	return slot, ok
}

// IsEmpty reports whether the lookup has no valid parameter roots.
func (s ParameterSlots) IsEmpty() bool {
	return len(s.bySymbol) == 0
}

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
