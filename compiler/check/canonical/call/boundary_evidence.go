package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/flow"
)

// BoundaryEvidence is the caller-visible evidence selected for one call
// boundary. It keeps summary-owned postconditions, effects, obligations, and
// control facts together so transfer can instantiate one call boundary instead
// of rebuilding each axis through parallel routes.
type BoundaryEvidence struct {
	ReturnRefs      flow.ReturnRefs
	ReturnRelations flow.ReturnRelations
	CellEffects     flow.CaptureEffects
	ReceiverEffects flow.ReceiverEffects
	BoundaryFacts   flow.BoundaryFacts
	ArgDemands      []callobligation.Obligation
	Postconditions  paramevidence.ReturnPostconditions
	ParamNarrows    []paramevidence.ParamNarrow
	NeverReturns    bool
}

// BoundaryEvidenceInput supplies call-site fallback policy that is not owned by
// the selected summary projection itself.
type BoundaryEvidenceInput struct {
	Call                 *ast.FuncCallExpr
	Resolver             TypeResolver
	UseResolvedSignature bool
	CellEffects          summary.CellEffectAggregation
	ArgDemands           []callobligation.Obligation
	Postconditions       paramevidence.ReturnPostconditions
	ParamNarrows         []paramevidence.ParamNarrow
	HasNoReturn          func(summary.FuncRef) bool
}

// BoundaryEvidence projects every non-return-value call-boundary axis from the
// same selected outcome and fallback policy.
func (o CallOutcome) BoundaryEvidence(in BoundaryEvidenceInput) BoundaryEvidence {
	return BoundaryEvidence{
		ReturnRefs:      o.ReturnRefs(),
		ReturnRelations: o.ReturnRelations(in.Call, in.Resolver, in.UseResolvedSignature),
		CellEffects:     o.CellEffects(in.CellEffects),
		ReceiverEffects: o.ReceiverEffects(),
		BoundaryFacts:   o.BoundaryFacts(),
		ArgDemands:      cloneArgDemands(in.ArgDemands),
		Postconditions:  postconditionsOrCompatibility(in.Postconditions, in.ParamNarrows),
		ParamNarrows:    paramevidence.SortParamNarrows(in.ParamNarrows),
		NeverReturns:    o.NeverReturns(in.HasNoReturn),
	}
}

func postconditionsOrCompatibility(post paramevidence.ReturnPostconditions, narrows []paramevidence.ParamNarrow) paramevidence.ReturnPostconditions {
	if post.HasConstraints() {
		return paramevidence.CloneReturnPostconditions(post)
	}
	return paramevidence.ReturnPostconditionsFromParamNarrows(narrows)
}

func cloneArgDemands(in []callobligation.Obligation) []callobligation.Obligation {
	if len(in) == 0 {
		return nil
	}
	out := make([]callobligation.Obligation, len(in))
	copy(out, in)
	return out
}
