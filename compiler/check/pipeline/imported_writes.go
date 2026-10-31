package pipeline

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
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
	aliasTargets := make(map[string][]cfg.SymbolID)
	graph.EachSymbolID(func(sym cfg.SymbolID) bool {
		if stableAlias(sym) {
			aliasTargets[aliases[sym]] = append(aliasTargets[aliases[sym]], sym)
		}
		return false
	})
	var effects []flow.FieldWriteEffect
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil || !returns.CallEvaluatedAtPoint(graph, p, info) {
			return
		}
		writes := r.importedWritesThroughCall(store, graph, bindings, info, make(map[cfg.SymbolID]bool))
		if len(writes) == 0 {
			return
		}
		beforeOperands := callsite.CallsBeforeOperandReads(graph, p)[info.Call]
		for _, write := range writes {
			if write.Module == "" || write.Field == "" || write.Type == nil {
				continue
			}
			key := api.FieldWriteKey{Path: write.Path, Field: write.Field}
			for _, sym := range aliasTargets[write.Module] {
				effects = append(effects, flow.FieldWriteEffect{
					Point:          p,
					Target:         constraint.Path{Root: graph.NameOf(sym), Symbol: sym, Segments: key.Segments()},
					Field:          write.Field,
					Type:           write.Type,
					BeforeOperands: beforeOperands,
				})
			}
		}
	})
	return effects
}

// importedWritesThroughCall follows a local wrapper to the body-backed module
// calls it can execute. It describes possible writes only; the outer call must
// still execute before any effect is applied to the caller.
func (r *Runner) importedWritesThroughCall(store api.StoreView, graph *cfg.Graph, bindings *bind.BindingTable, info *cfg.CallInfo, seen map[cfg.SymbolID]bool) []io.ModuleWrite {
	if info == nil || graph == nil {
		return nil
	}
	aliases := modules.MergeAliases(store.ModuleAliases(), modules.CollectAliases(graph))
	var writes []io.ModuleWrite
	if len(info.CalleePath.Segments) == 1 {
		segment := info.CalleePath.Segments[0]
		calleeSym := info.CalleePath.Symbol
		if segment.Kind == constraint.SegmentField || segment.Kind == constraint.SegmentIndexString {
			stable := calleeSym != 0 && aliases[calleeSym] != "" &&
				(bindings == nil || !bindings.IsReassigned(calleeSym)) &&
				(store.ModuleBindings() == nil || !store.ModuleBindings().IsReassigned(calleeSym)) &&
				!modules.AssignedModuleField(graph, calleeSym, segment.Name)
			if stable {
				manifest := r.manifests.Manifest(aliases[calleeSym])
				if manifest != nil && manifest.BodyBacked {
					writes = append(writes, manifest.CallWrites[segment.Name]...)
					writes = append(writes, manifest.MayCallWrites[segment.Name]...)
				}
			}
		}
	}
	callee := callsite.SelectPreferredSymbol(
		callsite.CallableCalleeSymbolCandidates(info, graph, bindings, store.ModuleBindings()),
		func(sym cfg.SymbolID) bool { return store.FunctionRefBySym(sym) != nil },
	)
	if callee == 0 || seen[callee] {
		return writes
	}
	seen[callee] = true
	ref := store.FunctionRefBySym(callee)
	if ref == nil {
		return writes
	}
	child := store.Graphs()[ref.GraphID]
	if child == nil {
		return writes
	}
	reachable := child.CFG().Reachable()
	child.EachCallSite(func(p cfg.Point, nested *cfg.CallInfo) {
		if reachable[p] {
			writes = append(writes, r.importedWritesThroughCall(store, child, child.Bindings(), nested, seen)...)
		}
	})
	return writes
}
