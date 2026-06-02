package summary

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// CallSummaryTarget identifies one candidate callee summary at a call site.
type CallSummaryTarget struct {
	Ref             FuncRef
	Summary         Summary
	DeclaredReturns bool
	// SignatureReturns is the selected-target declared-return fallback for
	// functions whose source signature owns the caller-visible return tuple.
	// It is computed at the call-outcome boundary so declared-return targets do
	// not leave the normalized selected-target path and re-resolve through a
	// less precise expression type.
	SignatureReturns []typ.Type
	// SignatureRelations, when non-empty, is a finite fallback used only when
	// Summary.Relations has no finite proof.
	SignatureRelations flow.ReturnRelations
}

// CallSummaryProjection folds multiple candidate callee summaries for one call site.
type CallSummaryProjection struct {
	Targets []CallSummaryTarget
}

// ReturnValues joins Summary.Returns across targets slotwise. Targets flagged as
// DeclaredReturns are intentionally skipped so the normal call pipeline handles
// declared return typing paths unchanged.
func (p CallSummaryProjection) ReturnValues() []product.AbstractValue {
	var out []product.AbstractValue
	for _, target := range p.Targets {
		returns, ok := targetReturnValues(target)
		if !ok {
			return nil
		}
		out = returnsDomain.Join(out, returns)
	}
	return out
}

// InferredReturnValues is the selected-target return fold. Declared-return
// targets contribute their signature-projected return tuple when the call
// outcome supplied one; otherwise the projection yields to the outer fallback.
func (p CallSummaryProjection) InferredReturnValues() []product.AbstractValue {
	return p.ReturnValues()
}

func targetReturnValues(target CallSummaryTarget) ([]product.AbstractValue, bool) {
	if !target.DeclaredReturns {
		return target.Summary.Returns, true
	}
	if len(target.SignatureReturns) == 0 {
		return nil, false
	}
	// Declared signature returns are folded immediately through returnsDomain.
	// That makes this a lattice boundary, not a transfer-storage seam: unknown
	// declared slots must be real product values so slotwise Join cannot see a
	// zero handle.
	return product.FromTypesTotal(target.SignatureReturns), true
}

// InferredReturnTypes projects InferredReturnValues to caller-visible types.
func (p CallSummaryProjection) InferredReturnTypes() []typ.Type {
	values := p.InferredReturnValues()
	if len(values) == 0 {
		return nil
	}
	out := make([]typ.Type, len(values))
	for i, av := range values {
		out[i] = product.ProjectValueOrUnknown(av)
	}
	return out
}

// ReturnFunctionRefs joins caller-visible returned function identities across targets
// slotwise.
func (p CallSummaryProjection) ReturnFunctionRefs() []flow.FunctionRefs {
	var out []flow.FunctionRefs
	for _, target := range p.Targets {
		out = JoinReturnFunctionRefs(out, target.Summary.ReturnFunctionRefs)
	}
	return out
}

// ReturnClosureRefs joins caller-visible returned closure identities across targets
// slotwise.
func (p CallSummaryProjection) ReturnClosureRefs() []flow.ClosureRefs {
	var out []flow.ClosureRefs
	for _, target := range p.Targets {
		out = JoinReturnClosureRefs(out, target.Summary.ReturnClosureRefs)
	}
	return out
}

// CellEffects folds caller-visible capture-cell effects across candidate callees.
// The summaries execute at the same fixed point as returns/relations, so unknown
// callee ordering is modeled with flow.CooccurringCaptureEffects.
func (p CallSummaryProjection) CellEffects() flow.CaptureEffects {
	out := flow.CaptureEffectsDomain.Bottom()
	for _, target := range p.Targets {
		out = flow.CooccurringCaptureEffects(out, target.Summary.CellEffects)
	}
	return out
}

// ReceiverEffects folds caller-visible runtime-argument effects across candidate
// callees. Unknown target order is modeled with the effect transformer algebra,
// matching CellEffects.
func (p CallSummaryProjection) ReceiverEffects() flow.ReceiverEffects {
	out := flow.ReceiverEffectsDomain.Bottom()
	for _, target := range p.Targets {
		out = flow.CooccurringReceiverEffects(out, target.Summary.ReceiverEffects)
	}
	return out
}

// ReturnRelations folds return-slot must-relations across possible callee targets.
// Each target contributes its proven summary relation when finite; otherwise it
// contributes a precomputed signature fallback when finite. If no finite proof is
// available for any target, the result is Top.
func (p CallSummaryProjection) ReturnRelations() flow.ReturnRelations {
	out := flow.ReturnRelationsDomain.Bottom()
	for _, target := range p.Targets {
		rels := target.Summary.Relations
		if !rels.HasProof() {
			if target.SignatureRelations.HasProof() {
				rels = target.SignatureRelations
			} else {
				rels = flow.ReturnRelationsDomain.Top()
			}
		}
		out = flow.ReturnRelationsDomain.Join(out, rels)
	}
	if flow.ReturnRelationsDomain.Equal(out, flow.ReturnRelationsDomain.Bottom()) {
		return flow.ReturnRelationsDomain.Top()
	}
	return out
}
