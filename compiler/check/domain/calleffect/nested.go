package calleffect

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// projectCarrierField projects a captured-effect carrier slot to its structural
// type at the flow-replay egress boundary. The zero AbstractValue (an absent
// slot) projects to nil so the flow assignment defaulting is unchanged.
func projectCarrierField(av product.AbstractValue) typ.Type {
	if av.IsZero() {
		return nil
	}
	return av.ProjectValue()
}

// CalledNestedAssignments is the flow replay payload for captured effects made
// by nested functions that are proven to run from the parent graph.
type CalledNestedAssignments struct {
	Fields    []flow.UnifiedAssignment
	Map       []flow.MapMutatorAssignment
	Table     []flow.TableMutatorAssignment
	Container []flow.ContainerMutatorAssignment
}

// CollectNestedAssignments collects captured field writes and mutator effects
// recorded for parent-visible nested functions and replays each effect through
// its matching flow operator. Direct invocation is driven by transfer call
// evidence.
//
// This supports cases where a nested function writes fields, mutates a captured
// table map (t[k] = v), mutates a table array (table.insert), or mutates a
// generic container (channel.send), and the nested function is:
//   - invoked directly,
//   - passed as a callback to a function with a callback spec, or
//   - stored in a field/global position that can be called outside the parent
//     graph before another exported function reads the captured state.
func CollectNestedAssignments(
	parent *cfg.Graph,
	bindings *bind.BindingTable,
	calls []api.CallEvidence,
	escapes []api.FunctionEscapeEvidence,
	capturedFields api.CapturedFieldAssigns,
	capturedContainers api.CapturedContainerMutations,
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
) CalledNestedAssignments {
	if parent == nil || (len(capturedFields) == 0 && len(capturedContainers) == 0) {
		return CalledNestedAssignments{}
	}

	parentSymbols := parent.AllSymbolIDs()
	trackedCallees := make(map[cfg.SymbolID]bool, len(capturedFields)+len(capturedContainers))
	for calleeSym := range capturedFields {
		trackedCallees[calleeSym] = true
	}
	for calleeSym := range capturedContainers {
		trackedCallees[calleeSym] = true
	}
	assignments := CalledNestedAssignments{}
	emitForCallee := func(p cfg.Point, sym cfg.SymbolID) {
		for _, targetSym := range cfg.SortedSymbolIDs(capturedFields[sym]) {
			fields := capturedFields[sym][targetSym]
			if !parentSymbols[targetSym] {
				continue
			}
			root := resolve.RootNameFromGraphAndBindings(parent, bindings, targetSym, "")
			appendNestedFieldAssignments(&assignments, p, root, targetSym, fields)
		}
		for _, targetSym := range cfg.SortedSymbolIDs(capturedContainers[sym]) {
			mutations := capturedContainers[sym][targetSym]
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

func appendNestedFieldAssignments(
	assignments *CalledNestedAssignments,
	p cfg.Point,
	root string,
	targetSym cfg.SymbolID,
	fields map[string]product.AbstractValue,
) {
	if assignments == nil || targetSym == 0 || len(fields) == 0 {
		return
	}
	for _, fieldName := range cfg.SortedFieldNames(fields) {
		fieldType := projectCarrierField(fields[fieldName])
		if fieldType == nil {
			fieldType = typ.Unknown
		}
		assignments.Fields = append(assignments.Fields, flow.UnifiedAssignment{
			Point: p,
			TargetPath: constraint.Path{
				Root:   root,
				Symbol: targetSym,
				Segments: []constraint.Segment{{
					Kind: constraint.SegmentField,
					Name: fieldName,
				}},
			},
			Type: fieldType,
		})
	}
}

func appendNestedMutatorAssignments(
	assignments *CalledNestedAssignments,
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
		case api.ContainerMutationMapElement:
			assignments.Map = append(assignments.Map, flow.MapMutatorAssignment{
				Point:     p,
				Target:    target,
				ValueMode: mutation.ValueMode,
				KeyType:   projectCarrierField(mutation.KeyType),
				ValueType: projectCarrierField(mutation.ValueType),
			})
		case api.ContainerMutationTableElement:
			assignments.Table = append(assignments.Table, flow.TableMutatorAssignment{
				Point:     p,
				Target:    target,
				KeyType:   projectCarrierField(mutation.KeyType),
				ValueType: projectCarrierField(mutation.ValueType),
			})
		default:
			assignments.Container = append(assignments.Container, flow.ContainerMutatorAssignment{
				Point:     p,
				Target:    target,
				ValueType: projectCarrierField(mutation.ValueType),
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
