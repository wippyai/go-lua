package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ConcreteValueRefinementRequest is the complete concrete input to one value
// refinement. Input is the immutable transfer-entry snapshot. Output is the
// evolving point-local snapshot and is the state against which the refinement
// is resolved and applied. Keeping both snapshots explicit makes the kernel's
// read boundary match the other concrete operation kernels without allowing a
// future symbolic caller to accidentally narrow stale input.
//
// ApplyConcreteValueRefinement and ApplyConcreteGuardRefinement deliberately
// do not activate path-presence implications. Activation is an ordered
// post-step owned by the transfer barriers that already schedule it.
type ConcreteValueRefinementRequest struct {
	Registry    *axis.Registry
	TypeValues  *typevalue.Cache
	Resolver    *visibility.Resolver
	ProjectPath PathTypeProjector
	Point       cfg.Point
	Input       state.State
	Output      state.State
	TargetPath  pathdom.Path
	Refinement  factflow.ValueRefinement
}

// ConcreteGuardRefinementRequest shares the value-refinement boundary. A
// guard adds only the contradiction check that turns an impossible edge into
// Bottom; its actual narrowing has exactly the value-refinement semantics.
type ConcreteGuardRefinementRequest = ConcreteValueRefinementRequest

// ApplyConcreteValueRefinement narrows one value in the evolving output state.
// It leaves implication activation to the caller's existing ordered barrier.
func ApplyConcreteValueRefinement(request ConcreteValueRefinementRequest) state.State {
	return applyValueRefinementAtWithoutImplicationsCached(
		request.TypeValues,
		request.Registry,
		request.Resolver,
		request.ProjectPath,
		request.Point,
		request.Output,
		request.TargetPath,
		request.Refinement,
	)
}

// ApplyConcreteGuardRefinement applies the contradiction check and narrowing
// for one guard against the evolving output state. The second result is false
// exactly when the guard contradicted the current value. It lets the existing
// edge barrier preserve its rule that contradiction returns Bottom immediately,
// without running the implication post-step. The kernel itself does not
// activate implications.
func ApplyConcreteGuardRefinement(request ConcreteGuardRefinementRequest) (state.State, bool) {
	if branchRefinementContradictsCurrentValue(
		request.TypeValues,
		request.Registry,
		request.Resolver,
		request.ProjectPath,
		request.Point,
		request.Output,
		request.TargetPath,
		request.Refinement,
	) {
		return unreachableState(request.Registry), false
	}
	return ApplyConcreteValueRefinement(request), true
}

// ConcretePathAssignmentRequest is the complete concrete input to one path
// assignment. Input is the immutable node-entry snapshot used to resolve the
// assignment source and propagate source-owned lanes. Output is the evolving
// point-local snapshot mutated by the assignment. They must remain distinct.
type ConcretePathAssignmentRequest struct {
	Context    transfer.NodeContext
	Resolver   *visibility.Resolver
	Facts      factflow.Facts
	Sources    sourcevalue.SourceValues
	Read       func(cfg.Point) state.State
	Input      state.State
	Output     state.State
	Assignment factflow.PathAssignment
}

// ApplyConcretePathAssignment applies one path assignment transactionally. A
// failed resolution or write returns Output unchanged.
func ApplyConcretePathAssignment(request ConcretePathAssignmentRequest) (state.State, bool) {
	return applyPathAssignment(
		request.Context,
		request.Resolver,
		request.Facts,
		request.Sources,
		request.Read,
		request.Input,
		request.Output,
		request.Assignment,
	)
}

// ConcreteBranchPathRelationRequest is the complete concrete input to one
// branch path relation. Output is the state after all preceding edge facts and
// relations at this edge.
type ConcreteBranchPathRelationRequest struct {
	Context     transfer.EdgeContext
	Resolver    *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
	Output      state.State
	Kind        factflow.BranchPathRelationKind
	LeftPath    pathdom.Path
	RightPath   pathdom.Path
}

// ApplyConcreteBranchPathRelation applies one relation to the evolving edge
// state.
func ApplyConcreteBranchPathRelation(request ConcreteBranchPathRelationRequest) state.State {
	return applyConcreteBranchPathRelation(
		request.TypeValues,
		request.Context,
		request.Resolver,
		request.ProjectPath,
		request.Output,
		request.Kind,
		request.LeftPath,
		request.RightPath,
	)
}
