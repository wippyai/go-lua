package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/resolve"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// CollectCalledNestedFieldAssignments collects field assignments recorded for
// called nested functions that target symbols from the parent graph (captured variables).
//
// When a nested function assigns fields to a captured variable, those assignments
// affect the type of the variable in the parent scope. This function consumes
// transfer call evidence and reduces the already-recorded captured assignments
// for callees that are proven to run from the parent graph.
//
// Example:
//
//	local t = {}
//	local function setup()
//	    t.name = "hello"  -- This assignment is collected
//	end
//	setup()  -- Because setup() is called, t gains field "name"
//
// The result maps symbols to their assigned field types, which can be merged
// into the symbol's type in the parent scope.
func CollectCalledNestedFieldAssignments(
	parent *cfg.Graph,
	bindings *bind.BindingTable,
	calls []api.CallEvidence,
	capturedByCallee map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type,
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
	if parent == nil || len(capturedByCallee) == 0 {
		return result
	}

	// Gather all symbols known in the parent graph (avoid per-point merges).
	parentSymbols := parent.AllSymbolIDs()

	// Find which local functions are called according to transfer evidence.
	trackedCallees := make(map[cfg.SymbolID]bool, len(capturedByCallee))
	for calleeSym := range capturedByCallee {
		trackedCallees[calleeSym] = true
	}
	calledSyms := make(map[cfg.SymbolID]bool)
	for _, call := range calls {
		for sym := range calledSymbolsFromCall(call.Info, call.Point, parent, bindings, resolveCalleeType, func(sym cfg.SymbolID) bool {
			return trackedCallees[sym]
		}) {
			calledSyms[sym] = true
		}
	}

	// Collect field assignments from called nested functions and merge into result.
	if len(calledSyms) == 0 {
		return result
	}
	for _, sym := range cfg.SortedSymbolIDs(calledSyms) {
		nestedFields := capturedByCallee[sym]
		if len(nestedFields) == 0 {
			continue
		}
		for _, baseSym := range cfg.SortedSymbolIDs(nestedFields) {
			fields := nestedFields[baseSym]
			if !parentSymbols[baseSym] {
				continue
			}
			if result[baseSym] == nil {
				result[baseSym] = make(map[string]typ.Type)
			}
			for _, fieldName := range cfg.SortedFieldNames(fields) {
				fieldType := fields[fieldName]
				if existing := result[baseSym][fieldName]; existing != nil {
					result[baseSym][fieldName] = typ.JoinPreferNonSoft(existing, fieldType)
				} else {
					result[baseSym][fieldName] = fieldType
				}
			}
		}
	}

	return result
}

// CalledNestedMutatorAssignments is the flow replay payload for captured
// mutations made by called nested functions.
type CalledNestedMutatorAssignments struct {
	Table     []flow.TableMutatorAssignment
	Container []flow.ContainerMutatorAssignment
}

// CollectNestedMutatorAssignments collects captured mutations recorded for
// parent-visible nested functions and replays them through the matching flow
// operator. Direct invocation is driven by transfer call evidence.
//
// This supports cases where a nested function mutates a captured table
// (table.insert) or generic container (channel.send) and the nested function is:
//   - invoked directly,
//   - passed as a callback to a function with a callback spec, or
//   - stored in a field/global position that can be called outside the parent
//     graph before another exported function reads the captured state.
func CollectNestedMutatorAssignments(
	parent *cfg.Graph,
	bindings *bind.BindingTable,
	calls []api.CallEvidence,
	escapes []api.FunctionEscapeEvidence,
	capturedByCallee api.CapturedContainerMutations,
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
) CalledNestedMutatorAssignments {
	if parent == nil || len(capturedByCallee) == 0 {
		return CalledNestedMutatorAssignments{}
	}

	parentSymbols := parent.AllSymbolIDs()
	trackedCallees := make(map[cfg.SymbolID]bool, len(capturedByCallee))
	for calleeSym := range capturedByCallee {
		trackedCallees[calleeSym] = true
	}
	assignments := CalledNestedMutatorAssignments{}
	emitForCallee := func(p cfg.Point, sym cfg.SymbolID) {
		nestedMutations := capturedByCallee[sym]
		if len(nestedMutations) == 0 {
			return
		}
		for _, targetSym := range cfg.SortedSymbolIDs(nestedMutations) {
			mutations := nestedMutations[targetSym]
			if !parentSymbols[targetSym] {
				continue
			}
			root := resolve.RootNameFromGraphAndBindings(parent, bindings, targetSym, "")
			appendNestedMutatorAssignments(&assignments, p, root, targetSym, mutations)
		}
	}

	for _, call := range calls {
		if call.Info == nil {
			continue
		}

		calledSyms := calledSymbolsFromCall(call.Info, call.Point, parent, bindings, resolveCalleeType, func(sym cfg.SymbolID) bool {
			return trackedCallees[sym]
		})
		if len(calledSyms) == 0 {
			continue
		}

		for _, sym := range cfg.SortedSymbolIDs(calledSyms) {
			emitForCallee(call.Point, sym)
		}
	}

	for _, escape := range escapes {
		if escape.Symbol == 0 || !trackedCallees[escape.Symbol] {
			continue
		}
		emitForCallee(escape.Point, escape.Symbol)
	}

	return assignments
}

func appendNestedMutatorAssignments(
	assignments *CalledNestedMutatorAssignments,
	p cfg.Point,
	root string,
	targetSym cfg.SymbolID,
	mutations []api.ContainerMutation,
) {
	if assignments == nil || targetSym == 0 || len(mutations) == 0 {
		return
	}
	for _, mutation := range mutations {
		segs := make([]constraint.Segment, len(mutation.Segments))
		copy(segs, mutation.Segments)
		target := constraint.Path{
			Root:     root,
			Symbol:   targetSym,
			Segments: segs,
		}
		switch mutation.Kind {
		case api.ContainerMutationTableElement:
			assignments.Table = append(assignments.Table, flow.TableMutatorAssignment{
				Point:     p,
				Target:    target,
				ValueType: mutation.ValueType,
			})
		default:
			assignments.Container = append(assignments.Container, flow.ContainerMutatorAssignment{
				Point:     p,
				Target:    target,
				ValueType: mutation.ValueType,
			})
		}
	}
}

func calledSymbolsFromCall(
	info *cfg.CallInfo,
	p cfg.Point,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
	prefer func(cfg.SymbolID) bool,
) map[cfg.SymbolID]bool {
	calledSyms := make(map[cfg.SymbolID]bool)
	if info == nil {
		return calledSyms
	}

	selected := checkcallsite.SelectPreferredSymbol(
		checkcallsite.CallableCalleeSymbolCandidates(info, graph, bindings, bindings),
		prefer,
	)
	if selected != 0 {
		calledSyms[selected] = true
	}

	if resolveCalleeType != nil {
		if fnType := resolveCalleeType(info, p); fnType != nil {
			if spec := contract.ExtractSpec(fnType); spec != nil && len(spec.Callbacks) > 0 {
				paramIndexes := make([]int, 0, len(spec.Callbacks))
				for paramIdx := range spec.Callbacks {
					paramIndexes = append(paramIndexes, paramIdx)
				}
				sort.Ints(paramIndexes)
				for _, paramIdx := range paramIndexes {
					arg := checkcallsite.RuntimeArgAt(info, paramIdx)
					if sym := checkcallsite.CanonicalSymbolFromExprWithAliases(arg, 0, graph, bindings, bindings, prefer); sym != 0 {
						calledSyms[sym] = true
					}
				}
			}
		}
	}

	return calledSyms
}
