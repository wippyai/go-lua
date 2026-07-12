package semanticplan

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// BranchPathRelationOp is the first executable semantic-guard operation. It
// owns structural paths and delegates only the concrete lattice action to the
// shared production kernel.
type BranchPathRelationOp struct {
	Kind  factflow.BranchPathRelationKind
	Left  pathdom.Path
	Right pathdom.Path
}

func CompileBranchPathRelation(relation factflow.BranchPathRelation) BranchPathRelationOp {
	return BranchPathRelationOp{Kind: relation.Kind(), Left: relation.LeftPath(), Right: relation.RightPath()}
}

// Execute applies the relation to Output, which is deliberately the state
// after preceding edge operations. This preserves ordered guard correlation.
func (op BranchPathRelationOp) Execute(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	project factapply.PathTypeProjector,
	types *typevalue.Cache,
	output state.State,
) state.State {
	return factapply.ApplyConcreteBranchPathRelation(factapply.ConcreteBranchPathRelationRequest{
		Context: ctx, Resolver: resolver, ProjectPath: project, TypeValues: types,
		Output: output, Kind: op.Kind, LeftPath: op.Left, RightPath: op.Right,
	})
}
