package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

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
