package call

import (
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
	NeverReturns    bool
}

// BoundaryEvidenceInput supplies call-site facts that are orthogonal to the
// selected outcome. Return relations and boundary facts are already inside
// CallOutcome and are not recomputed here.
type BoundaryEvidenceInput struct {
	CellEffects    summary.CellEffectAggregation
	ArgDemands     []callobligation.Obligation
	Postconditions paramevidence.ReturnPostconditions
	HasNoReturn    func(summary.FuncRef) bool
}

// BoundaryEvidence projects every non-return-value call-boundary axis from the
// same selected outcome and fallback policy.
func (o CallOutcome) BoundaryEvidence(in BoundaryEvidenceInput) BoundaryEvidence {
	return BoundaryEvidence{
		ReturnRefs:      o.ReturnRefs(),
		ReturnRelations: o.ReturnRelations(),
		CellEffects:     o.CellEffects(in.CellEffects),
		ReceiverEffects: o.ReceiverEffects(),
		BoundaryFacts:   o.BoundaryFacts(),
		ArgDemands:      cloneArgDemands(in.ArgDemands),
		Postconditions:  paramevidence.CloneReturnPostconditions(in.Postconditions),
		NeverReturns:    o.NeverReturns(in.HasNoReturn),
	}
}

func cloneArgDemands(in []callobligation.Obligation) []callobligation.Obligation {
	if len(in) == 0 {
		return nil
	}
	out := make([]callobligation.Obligation, len(in))
	copy(out, in)
	return out
}
