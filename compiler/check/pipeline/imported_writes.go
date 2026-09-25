package pipeline

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// importedModuleCallWrites applies a function's exported writes to the
// caller's alias of the same imported module. The effect is attached to the
// evaluated call point, so a skipped call cannot change the caller's shape.
func (r *Runner) importedModuleCallWrites(store api.StoreView, graph *cfg.Graph, bindings *bind.BindingTable) []flow.FieldWriteEffect {
	if r == nil || r.manifests == nil || store == nil || graph == nil {
		return nil
	}
	aliases := modules.MergeAliases(store.ModuleAliases(), modules.CollectAliases(graph))
	stableAlias := func(sym cfg.SymbolID) bool {
		if sym == 0 || aliases[sym] == "" {
			return false
		}
		return (bindings == nil || !bindings.IsReassigned(sym)) &&
			(store.ModuleBindings() == nil || !store.ModuleBindings().IsReassigned(sym))
	}
	// A field replaced anywhere in this graph may no longer be the function
	// described by the imported manifest.
	replaced := make(map[cfg.SymbolID]map[string]bool)
	graph.EachAssign(func(_ cfg.Point, assignment *cfg.AssignInfo) {
		if assignment == nil {
			return
		}
		for _, target := range assignment.Targets {
			if target.Kind != cfg.TargetField || !stableAlias(target.BaseSymbol) || len(target.FieldPath) != 1 {
				continue
			}
			if replaced[target.BaseSymbol] == nil {
				replaced[target.BaseSymbol] = make(map[string]bool)
			}
			replaced[target.BaseSymbol][target.FieldPath[0]] = true
		}
	})
	aliasTargets := make(map[string][]cfg.SymbolID)
	for sym := range graph.AllSymbolIDs() {
		if stableAlias(sym) {
			aliasTargets[aliases[sym]] = append(aliasTargets[aliases[sym]], sym)
		}
	}
	var effects []flow.FieldWriteEffect
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || !returns.CallEvaluatedAtPoint(graph, p, info) || len(info.CalleePath.Segments) != 1 {
			return
		}
		segment := info.CalleePath.Segments[0]
		if segment.Kind != constraint.SegmentField && segment.Kind != constraint.SegmentIndexString {
			return
		}
		calleeSym := info.CalleePath.Symbol
		if !stableAlias(calleeSym) || replaced[calleeSym][segment.Name] {
			return
		}
		calleeModule := aliases[calleeSym]
		manifest := r.manifests.Manifest(calleeModule)
		if manifest == nil || !manifest.BodyBacked {
			return
		}
		writes := manifest.CallWrites[segment.Name]
		if len(writes) == 0 {
			return
		}
		for _, write := range writes {
			if write.Module == "" || write.Field == "" || write.Type == nil {
				continue
			}
			key := api.FieldWriteKey{Path: write.Path, Field: write.Field}
			for _, sym := range aliasTargets[write.Module] {
				effects = append(effects, flow.FieldWriteEffect{
					Point:  p,
					Target: constraint.Path{Root: graph.NameOf(sym), Symbol: sym, Segments: key.Segments()},
					Field:  write.Field,
					Type:   write.Type,
				})
			}
		}
	})
	return effects
}
