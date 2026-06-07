package captured

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// PathProjection is the captured environment view of parent flow state.
// Captured inference consumes the normalized flow path-shape projection surface
// instead of interpreting path facts locally.
type PathProjection = flow.PathShapeProjection

// FromParentFactsAtPoint computes captured types for a nested graph from parent
// facts at the point where the closure environment is observed.
func FromParentFactsAtPoint(
	parentFacts flow.TypeFacts,
	childGraph *cfg.Graph,
	point cfg.Point,
	bindingsOverride *bind.BindingTable,
	projection PathProjection,
) map[cfg.SymbolID]typ.Type {
	if parentFacts == nil || childGraph == nil || point == 0 {
		return nil
	}
	bindings := bindingsOverride
	if bindings == nil {
		bindings = childGraph.Bindings()
	}
	if bindings == nil {
		return nil
	}
	fn := childGraph.Func()
	if fn == nil {
		return nil
	}
	capturedSyms := bindings.CapturedSymbols(fn)
	if len(capturedSyms) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]typ.Type, len(capturedSyms))
	for _, sym := range capturedSyms {
		if sym == 0 {
			continue
		}
		if t := capturedTypeAtPoint(parentFacts, point, sym, projection); !typ.IsAbsentOrUnknown(t) {
			out[sym] = t
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func capturedTypeAtPoint(
	parentFacts flow.TypeFacts,
	point cfg.Point,
	sym cfg.SymbolID,
	projection PathProjection,
) typ.Type {
	if parentFacts == nil || point == 0 || sym == 0 {
		return nil
	}
	tv := parentFacts.EffectiveTypeAt(point, sym)
	if tv.State != flow.StateResolved || typ.IsAbsentOrUnknown(tv.Type) {
		return nil
	}
	root := constraint.Path{Symbol: sym}
	return flow.ProjectObservedPathShape(point, root, tv.Type, projection)
}

// MergeCapturedTypes merges captured types into declared types as hints.
func MergeCapturedTypes(declared flow.DeclaredTypes, captured map[cfg.SymbolID]typ.Type) flow.DeclaredTypes {
	if len(captured) == 0 {
		return declared
	}
	if declared == nil {
		declared = make(flow.DeclaredTypes, len(captured))
	}
	for _, sym := range cfg.SortedSymbolIDs(captured) {
		t := captured[sym]
		if sym == 0 || t == nil {
			continue
		}
		if prev := declared[sym]; prev != nil {
			declared[sym] = value.JoinPrecise(prev, t)
		} else {
			declared[sym] = t
		}
	}
	return declared
}
