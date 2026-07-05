package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyNormalReturnBranchProofs(ctx normalReturnApplyContext, out state.State) state.State {
	for _, proof := range ctx.normalFacts.BranchProofs {
		stateProof, ok := callBranchProofAt(ctx, proof)
		if !ok {
			continue
		}
		out = out.AddBranchProof(stateProof)
		out = applyNormalReturnPathRelationProof(ctx, out, proof)
	}
	return out
}

func callBranchProofAt(
	ctx normalReturnApplyContext,
	proof callboundary.BranchProof,
) (pathevidence.BranchProof, bool) {
	path, ok := ctx.keyspaceKey(proof.Path)
	if !ok {
		return pathevidence.BranchProof{}, false
	}
	switch proof.Kind {
	case pathevidence.BranchProofPathPresence:
		return pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     path,
			Presence: proof.Presence,
		}, true
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
		other, ok := ctx.keyspaceKey(proof.Other)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  proof.Kind,
			Path:  path,
			Other: other,
		}, true
	default:
		return pathevidence.BranchProof{}, false
	}
}

func applyNormalReturnPathRelationProof(ctx normalReturnApplyContext, out state.State, proof callboundary.BranchProof) state.State {
	switch proof.Kind {
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual:
	default:
		return out
	}
	leftPath, ok := ctx.substitute(proof.Path)
	if !ok {
		return out
	}
	rightPath, ok := ctx.substitute(proof.Other)
	if !ok {
		return out
	}
	edgeCtx := transfer.EdgeContext{
		Registry: ctx.node.Registry,
		Edge: cfg.Edge{
			From: ctx.point,
			Cond: true,
		},
	}
	relation, ok := normalReturnBranchPathRelation(proof.Kind, leftPath, rightPath)
	if !ok {
		return out
	}
	return applyBranchPathRelation(ctx.typeValues, edgeCtx, ctx.resolver, ctx.projectPath, out, relation)
}

func normalReturnBranchPathRelation(
	kind pathevidence.BranchProofKind,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) (factflow.BranchPathRelation, bool) {
	switch kind {
	case pathevidence.BranchProofPathEqual:
		return factflow.NewBranchPathEquality(leftPath, rightPath, true, false), true
	case pathevidence.BranchProofPathNotEqual:
		return factflow.NewBranchPathInequality(leftPath, rightPath, true, false), true
	default:
		return factflow.BranchPathRelation{}, false
	}
}
