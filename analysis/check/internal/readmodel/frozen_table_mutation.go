package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ForEachFrozenTableMutation visits writes and mutating calls that target a table
// identity proved frozen at the mutation point.
func (r Reader) ForEachFrozenTableMutation(visit func(FrozenTableMutation) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.result.Graph().RPO() {
		if !r.result.PointNormallyReachable(point) {
			continue
		}
		if write, ok := r.result.LoweredAssignmentWrite(point); ok {
			if mutation, ok := r.frozenAssignmentMutation(point, write); ok {
				visited = true
				if !visit(mutation) {
					return true
				}
			}
		}
		if mutation, ok := r.frozenCallMutation(point); ok {
			visited = true
			if !visit(mutation) {
				return true
			}
		}
	}
	return visited
}

func (r Reader) frozenAssignmentMutation(point cfg.Point, write body.LoweredAssignmentWrite) (FrozenTableMutation, bool) {
	if !write.HasContainer || write.Container.IsEmpty() {
		return FrozenTableMutation{}, false
	}
	tableID, ok := r.frozenMutationContainerIdentity(point, write.Container)
	if !ok {
		return FrozenTableMutation{}, false
	}
	in, ok := r.result.StateAt(point)
	if !ok || !in.IsTableFrozen(tableID) {
		return FrozenTableMutation{}, false
	}
	frozenSpan, hasFrozenSpan := r.frozenProofSpan(point, write.Container)
	return FrozenTableMutation{
		Point:              point,
		Kind:               readapi.FrozenTableMutationAssignment,
		ContainerLabel:     r.displayPath(write.Container),
		ContainerKey:       string(write.Container.Key()),
		MutationSpan:       sourceSpanFromBody(write.Span),
		FreezeProofSpan:    frozenSpan,
		HasFreezeProofSpan: hasFrozenSpan,
	}, true
}

func (r Reader) frozenCallMutation(point cfg.Point) (FrozenTableMutation, bool) {
	outcome, ok := r.result.CallOutcomeAt(point)
	if !ok {
		return FrozenTableMutation{}, false
	}
	site, ok := r.result.CallSiteView(point)
	if !ok {
		return FrozenTableMutation{}, false
	}
	in, ok := r.result.StateAt(point)
	if !ok {
		return FrozenTableMutation{}, false
	}
	for _, target := range r.frozenCallInvalidationTargets(site, outcome) {
		tableID, ok := r.frozenMutationContainerIdentity(point, target)
		if !ok || !in.IsTableFrozen(tableID) {
			continue
		}
		frozenSpan, hasFrozenSpan := r.frozenProofSpan(point, target)
		return FrozenTableMutation{
			Point:              point,
			Kind:               readapi.FrozenTableMutationCall,
			ContainerLabel:     r.displayPath(target),
			ContainerKey:       string(target.Key()),
			MutationSpan:       sourceSpanFromFactflow(site.CallSpan()),
			FreezeProofSpan:    frozenSpan,
			HasFreezeProofSpan: hasFrozenSpan,
		}, true
	}
	return FrozenTableMutation{}, false
}

func (r Reader) frozenCallInvalidationTargets(site factflow.CallSiteView, outcome callpayload.CallOutcome) []path.Path {
	var out []path.Path
	appendSubstituted := func(bindings []path.Path, target path.Path) {
		substituted, ok := target.Substitute(bindings)
		if !ok || substituted.IsEmpty() {
			return
		}
		for _, existing := range out {
			if existing.Equal(substituted) {
				return
			}
		}
		out = append(out, substituted)
	}
	argBindings := r.callArgumentBindings(site)
	callBindings := r.callBindings(site)
	for _, invalidation := range outcome.ParamPathInvalidations {
		appendSubstituted(argBindings, invalidation.Path)
	}
	for _, write := range outcome.ParamPathWrites {
		appendSubstituted(argBindings, write.Path)
	}
	for _, invalidation := range outcome.NormalReturnFacts.PathInvalidations {
		appendSubstituted(callBindings, invalidation.Path)
	}
	return out
}

func (r Reader) frozenMutationContainerIdentity(point cfg.Point, container path.Path) (identity.ID, bool) {
	reg := r.result.Registry()
	if reg == nil {
		return identity.ID{}, false
	}
	value, ok := r.result.PathValueBeforeBoundary(point, container)
	if !ok {
		return identity.ID{}, false
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	return id, ok && id != (identity.ID{})
}

func (r Reader) frozenProofSpan(stop cfg.Point, container path.Path) (SourceSpan, bool) {
	graph := r.result.Graph()
	if graph == nil || container.IsEmpty() {
		return SourceSpan{}, false
	}
	for _, point := range graph.RPO() {
		if point == stop {
			break
		}
		outcome, ok := r.result.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.FrozenTables) == 0 {
			continue
		}
		site, ok := r.result.CallSiteView(point)
		if !ok {
			continue
		}
		bindings := r.callBindings(site)
		for _, fact := range outcome.NormalReturnFacts.FrozenTables {
			target, ok := fact.Target.Substitute(bindings)
			if !ok || !target.Equal(container) {
				continue
			}
			return sourceSpanFromFactflow(site.CallSpan()), true
		}
	}
	return SourceSpan{}, false
}
