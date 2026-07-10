package wir

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type debugVisibilityKey struct {
	point cfg.Point
	phase DebugPhase
}

// SetDebugLocalVisibility records the source-local symbols lexically visible
// at an observable before or after phase. Lowering supplies this structural
// projection as symbol IDs only; names, kinds, and backend slots remain the
// existing DbgLocal projection over SymbolInfo and codegen allocation.
func (b *Body) SetDebugLocalVisibility(point cfg.Point, phase DebugPhase, symbols []SymbolID) {
	if b == nil || (phase != DebugPhaseBefore && phase != DebugPhaseAfter) {
		return
	}
	if b.debugLocalVisibility == nil {
		b.debugLocalVisibility = make(map[debugVisibilityKey][]SymbolID)
	}
	seen := make(map[SymbolID]struct{}, len(symbols))
	out := make([]SymbolID, 0, len(symbols))
	for _, symbol := range symbols {
		if symbol == 0 {
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	b.debugLocalVisibility[debugVisibilityKey{point: point, phase: phase}] = out
}

// DebugLocalVisibility returns a detached, deterministic DbgLocal-symbol
// projection for an observable phase. Call and suspend use before-point
// visibility; return uses after-point visibility.
func (b *Body) DebugLocalVisibility(point cfg.Point, phase DebugPhase) []SymbolID {
	if b == nil {
		return nil
	}
	switch phase {
	case DebugPhaseCall, DebugPhaseSuspend:
		phase = DebugPhaseBefore
	case DebugPhaseReturn:
		phase = DebugPhaseAfter
	}
	if phase != DebugPhaseBefore && phase != DebugPhaseAfter {
		return nil
	}
	return append([]SymbolID(nil), b.debugLocalVisibility[debugVisibilityKey{point: point, phase: phase}]...)
}
