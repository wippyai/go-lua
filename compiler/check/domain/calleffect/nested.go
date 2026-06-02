package calleffect

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
	interprocfields "github.com/wippyai/go-lua/compiler/check/domain/interproc"
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

// CalledNestedAssignments is the remaining legacy flow replay payload for
// captured field writes made by nested functions that are proven to run from the
// parent graph.
type CalledNestedAssignments struct {
	Fields []flow.UnifiedAssignment
}

// CollectNestedAssignments collects captured field writes recorded for
// parent-visible nested functions and replays them through the legacy flow
// assignment operator. Direct invocation is driven by transfer call evidence.
//
// Captured container mutations are intentionally not replayed here. The
// canonical single-fixpoint path owns those through CaptureEffects on Summary and
// transfer.applyCallCellEffects; fabricating flow.Inputs mutator rows here would
// preserve the old parallel precision lane.
//
// This supports cases where a nested function writes fields and the nested
// function is:
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
	resolveCalleeType func(*cfg.CallInfo, cfg.Point) typ.Type,
) CalledNestedAssignments {
	if parent == nil || len(capturedFields) == 0 {
		return CalledNestedAssignments{}
	}

	parentSymbols := parent.AllSymbolIDs()
	trackedCallees := make(map[cfg.SymbolID]bool, len(capturedFields))
	for calleeSym := range capturedFields {
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
	fields api.FieldValues,
) {
	if assignments == nil || targetSym == 0 || len(fields) == 0 {
		return
	}
	for _, fieldKey := range interprocfields.SortedFieldKeys(fields) {
		fieldType := projectCarrierField(fields[fieldKey])
		if fieldType == nil {
			fieldType = typ.Unknown
		}
		segment := fieldKey
		assignments.Fields = append(assignments.Fields, flow.UnifiedAssignment{
			Point: p,
			TargetPath: constraint.Path{
				Root:     root,
				Symbol:   targetSym,
				Segments: []constraint.Segment{segment},
			},
			Type: fieldType,
		})
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
