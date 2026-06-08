package summary

import (
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	returndomain "github.com/wippyai/go-lua/compiler/check/domain/returns"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// CallSummaryTarget identifies one candidate callee summary at a call site.
type CallSummaryTarget struct {
	Ref             FuncRef
	Summary         Summary
	EntryValues     EntryValues
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

// ReturnValues joins caller-visible returns across selected targets slotwise.
// Closed declared-return targets use their signature-projected tuple: a source
// contract such as `(): number` owns the public return surface even when the body
// happens to return a literal. Open generic declarations (`(): T`, `(): {T}`)
// must not be marked DeclaredReturns here; they are binder relations, not closed
// runtime facts. Those targets keep the exact-context Summary.Returns so calls
// like `apply<T,U>(x, fn): U` can return the solved callback result instead of a
// broad signature fallback.
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
		if refined, ok := RefineReturnValuesWithTypes(target.Summary.Returns, target.SignatureReturns); ok {
			return refined, true
		}
		return target.Summary.Returns, true
	}
	if len(target.SignatureReturns) == 0 {
		return nil, false
	}
	return declaredReturnValuesWithSummary(target.Summary.Returns, target.SignatureReturns), true
}

func declaredReturnValuesWithSummary(summary []product.AbstractValue, signature []typ.Type) []product.AbstractValue {
	// Declared signature returns are folded immediately through returnsDomain.
	// That makes this a lattice boundary, not a transfer-storage seam: unknown
	// declared slots must be real product values so slotwise Join cannot see a
	// zero handle.
	out := product.FromTypesTotal(signature)
	if len(summary) == 0 || len(summary) != len(signature) {
		return out
	}
	for i, fallbackType := range signature {
		refined, ok := returndomain.RefineDeclaredReturnType(fallbackType, product.ProjectValueOrUnknown(summary[i]))
		if !ok || typ.TypeEquals(refined, fallbackType) {
			continue
		}
		out[i] = product.FromType(refined)
	}
	return out
}

// RefineReturnValuesWithTypes repairs product return slots with a closed
// same-expression type fallback. The summary keeps precise evidence it already
// owns; the fallback closes top-like or free-symbol leaves such as an open `T`
// that should not cross the call boundary. This is a precision merge, not a
// join and not whole-slot replacement.
func RefineReturnValuesWithTypes(values []product.AbstractValue, types []typ.Type) ([]product.AbstractValue, bool) {
	if len(values) == 0 || len(values) != len(types) {
		return nil, false
	}
	out := make([]product.AbstractValue, len(values))
	copy(out, values)
	changed := false
	for i, fallbackType := range types {
		if fallbackType == nil || typ.IsUnknown(fallbackType) {
			continue
		}
		fallbackType = subst.ExpandInstantiated(fallbackType)
		summaryType := product.ProjectValueOrUnknown(values[i])
		if typ.IsClosedUnionAnnotation(fallbackType) && !subtype.IsSubtype(summaryType, fallbackType) {
			out[i] = product.FromType(fallbackType)
			changed = true
			continue
		}
		refinedType, refined := typ.RefineWithFallback(summaryType, fallbackType)
		if !refined {
			continue
		}
		if typ.TypeEquals(refinedType, summaryType) {
			continue
		}
		out[i] = product.FromType(refinedType)
		changed = true
	}
	if !changed {
		return nil, false
	}
	return out, true
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

// ReturnRefs joins caller-visible returned callable identities across targets
// slotwise.
func (p CallSummaryProjection) ReturnRefs() flow.ReturnRefs {
	var out flow.ReturnRefs
	for _, target := range p.Targets {
		out = flow.ReturnRefsDomain.Join(out, target.Summary.ReturnRefs)
	}
	return out
}

// ReturnStaticMembers joins caller-visible returned child-path facts across
// selected targets. The slot lattice drops facts not proven by every possible
// callee, so transfer may replay the result as definite PointState facts.
func (p CallSummaryProjection) ReturnStaticMembers() []flow.StaticMemberFacts {
	var out []flow.StaticMemberFacts
	for _, target := range p.Targets {
		out = returnStaticMembersDomain.Join(out, target.Summary.ReturnStaticMembers)
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

// BoundaryFacts folds caller-visible boundary facts across candidate callees.
// Facts are must-postconditions, so possible targets are joined by intersection.
func (p CallSummaryProjection) BoundaryFacts() flow.BoundaryFacts {
	out := flow.BoundaryFactsDomain.Bottom()
	for _, target := range p.Targets {
		out = flow.BoundaryFactsDomain.Join(out, target.Summary.BoundaryFacts)
	}
	if flow.BoundaryFactsDomain.Equal(out, flow.BoundaryFactsDomain.Bottom()) {
		return flow.BoundaryFactsDomain.Top()
	}
	return out
}

// Postconditions folds normal-return must-proofs across candidate callees.
func (p CallSummaryProjection) Postconditions() paramevidence.ReturnPostconditions {
	out := paramevidence.ReturnPostconditionsDomain.Bottom()
	for _, target := range p.Targets {
		post := target.Summary.Postconditions
		if !post.HasConstraints() {
			post = paramevidence.ReturnPostconditionsFromParamNarrows(target.Summary.ParamNarrows)
		}
		out = paramevidence.ReturnPostconditionsDomain.Join(out, post)
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
