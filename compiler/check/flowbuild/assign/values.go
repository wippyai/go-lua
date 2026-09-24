package assign

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func expandedAssignValues(synthAPI api.SynthAPI, info *cfg.AssignInfo, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	if synthAPI == nil || info == nil || len(info.Targets) == 0 || len(info.Sources) == 0 {
		return nil
	}
	return synthAPI.ExpandValuesWithSpecTypes(info.Sources, len(info.Targets), p, specTypes)
}

// rhsSpecTypesAtAssignPoint returns the overlay as the RHS of the assignment at
// p observes it. Assignment target symbols take their pre-assignment types
// (joined across predecessors), which preserves Lua's RHS evaluation order for
// `x = f(x, ...)`. Every symbol the RHS reads is narrowed by the branch
// conditions reaching p, so `if not x then return end; local y = f(x)` calls f
// with x present.
func rhsSpecTypesAtAssignPoint(
	graph *cfg.Graph,
	info *cfg.AssignInfo,
	p cfg.Point,
	base api.SpecTypes,
	resolve checkcallsite.SymbolTypeAtPoint,
	preflow *flow.Solution,
) api.SpecTypes {
	if graph == nil || info == nil || len(info.Targets) == 0 {
		return base
	}
	return narrowReadSymbolsAtPoint(graph, info, p, preAssignmentTargetTypes(graph, info, p, base, resolve), preflow)
}

// preAssignmentTargetTypes overlays assignment target symbols with their types
// before the assignment at p.
func preAssignmentTargetTypes(
	graph *cfg.Graph,
	info *cfg.AssignInfo,
	p cfg.Point,
	base api.SpecTypes,
	resolve checkcallsite.SymbolTypeAtPoint,
) api.SpecTypes {
	if resolve == nil {
		return base
	}

	targetSyms := make(map[cfg.SymbolID]bool, len(info.Targets))
	for _, target := range info.Targets {
		if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
			targetSyms[target.Symbol] = true
		}
	}
	if len(targetSyms) == 0 {
		return base
	}

	var out api.SpecTypes
	override := func(sym cfg.SymbolID, t typ.Type) {
		if t == nil || t.Kind().IsPlaceholder() {
			return
		}
		if out == nil {
			if len(base) == 0 {
				out = make(api.SpecTypes, len(targetSyms))
			} else {
				out = make(api.SpecTypes, len(base)+len(targetSyms))
				for k, v := range base {
					out[k] = v
				}
			}
		}
		out[sym] = typ.PruneSoftUnionMembers(t)
	}

	for sym := range targetSyms {
		joined := checkcallsite.PreAssignmentTypeAtJoinOrPoint(graph, p, sym, checkcallsite.SymbolTypeAtPoint(resolve))
		override(sym, joined)
	}

	if out != nil {
		return out
	}
	return base
}

// narrowReadSymbolsAtPoint narrows the overlay types of the symbols read by the
// sources of info by the branch conditions reaching p.
func narrowReadSymbolsAtPoint(graph *cfg.Graph, info *cfg.AssignInfo, p cfg.Point, overlay api.SpecTypes, preflow *flow.Solution) api.SpecTypes {
	if preflow == nil || len(overlay) == 0 {
		return overlay
	}
	bindings := graph.Bindings()
	if bindings == nil {
		return overlay
	}
	var refs []cfg.SymbolID
	for _, src := range info.Sources {
		collectExprSymbols(src, bindings, &refs)
	}
	var out api.SpecTypes
	for _, sym := range refs {
		t, ok := overlay[sym]
		if !ok || t == nil {
			continue
		}
		name := bindings.Name(sym)
		if name == "" {
			continue
		}
		path := constraint.Path{Root: name, Symbol: sym}
		narrowed := preflow.NarrowTypeAt(p, path, t)
		if narrowed == nil || typ.TypeEquals(narrowed, t) {
			continue
		}
		if out == nil {
			out = make(api.SpecTypes, len(overlay))
			for k, v := range overlay {
				out[k] = v
			}
		}
		out[sym] = narrowed
	}
	if out != nil {
		return out
	}
	return overlay
}

func assignValueAt(values []typ.Type, i int) typ.Type {
	if i < 0 || i >= len(values) {
		return nil
	}
	return values[i]
}
