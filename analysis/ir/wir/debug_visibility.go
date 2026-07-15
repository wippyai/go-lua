package wir

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type debugVisibilityAtPoint struct {
	before []SymbolID
	after  []SymbolID
}

// SetDebugLocalVisibility records the source-local symbols lexically visible
// at an observable before or after phase. Lowering supplies this structural
// projection as symbol IDs only; names, kinds, and backend slots remain the
// existing DbgLocal projection over SymbolInfo and codegen allocation.
func (b *Body) SetDebugLocalVisibility(point cfg.Point, phase DebugPhase, symbols []SymbolID) {
	if b == nil || (phase != DebugPhaseBefore && phase != DebugPhaseAfter) {
		return
	}
	out := append([]SymbolID(nil), symbols...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	write := 0
	for _, symbol := range out {
		if symbol == 0 || (write != 0 && out[write-1] == symbol) {
			continue
		}
		out[write] = symbol
		write++
	}
	b.setDebugLocalVisibilitySnapshot(point, phase, out[:write])
}

// SetDebugLocalVisibilitySnapshot records an immutable canonical visibility
// snapshot. The lowering producer uses this ownership-transfer form so all
// unchanged points can share one sorted symbol slice. Non-canonical input is
// normalized through SetDebugLocalVisibility instead of weakening the table's
// deterministic representation.
func (b *Body) SetDebugLocalVisibilitySnapshot(point cfg.Point, phase DebugPhase, symbols []SymbolID) {
	if b == nil || (phase != DebugPhaseBefore && phase != DebugPhaseAfter) {
		return
	}
	for i, symbol := range symbols {
		if symbol == 0 || (i != 0 && symbols[i-1] >= symbol) {
			b.SetDebugLocalVisibility(point, phase, symbols)
			return
		}
	}
	b.setDebugLocalVisibilitySnapshot(point, phase, symbols)
}

func (b *Body) setDebugLocalVisibilitySnapshot(point cfg.Point, phase DebugPhase, symbols []SymbolID) {
	needed := int(point) + 1
	if len(b.debugLocalVisibility) < needed {
		b.debugLocalVisibility = append(b.debugLocalVisibility, make([]debugVisibilityAtPoint, needed-len(b.debugLocalVisibility))...)
	}
	if phase == DebugPhaseBefore {
		b.debugLocalVisibility[point].before = symbols
	} else {
		b.debugLocalVisibility[point].after = symbols
	}
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
	if int(point) >= len(b.debugLocalVisibility) {
		return nil
	}
	visible := b.debugLocalVisibility[point].before
	if phase == DebugPhaseAfter {
		visible = b.debugLocalVisibility[point].after
	}
	return append([]SymbolID(nil), visible...)
}
