package returns

import (
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// MergeFunctionFactType merges function-type facts through one canonical policy.
// This ensures all channels agree on when to preserve shape and how to merge
// returns, avoiding directional one-off behavior in individual phases.
func MergeFunctionFactType(existing, candidate typ.Type) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	existingFn := unwrap.Function(existing)
	candidateFn := unwrap.Function(candidate)
	if mergedFromVariants, ok := mergeFunctionFactVariants(existing, candidate); ok {
		return mergedFromVariants
	}
	if existingFn != nil && candidateFn != nil {
		if sameFunctionShapeForFactMerge(existingFn, candidateFn) {
			return mergeFunctionFactsByShape(existingFn, candidateFn)
		}
	}

	if subtype.IsSubtype(existing, candidate) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) {
		return existing
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

type functionFactVariants struct {
	funcs     []*typ.Function
	residuals []typ.Type
}

func mergeFunctionFactVariants(existing, candidate typ.Type) (typ.Type, bool) {
	existingVariants := splitFunctionFactVariants(existing)
	candidateVariants := splitFunctionFactVariants(candidate)
	if len(existingVariants.funcs) == 0 || len(candidateVariants.funcs) == 0 {
		return nil, false
	}

	all := make([]*typ.Function, 0, len(existingVariants.funcs)+len(candidateVariants.funcs))
	all = append(all, existingVariants.funcs...)
	all = append(all, candidateVariants.funcs...)
	for i := 1; i < len(all); i++ {
		if !sameFunctionShapeForFactMerge(all[0], all[i]) {
			return nil, false
		}
	}

	merged := all[0]
	for i := 1; i < len(all); i++ {
		next, _ := mergeFunctionFactsByShape(merged, all[i]).(*typ.Function)
		if next == nil {
			return nil, false
		}
		merged = next
	}

	residuals := make([]typ.Type, 0, len(existingVariants.residuals)+len(candidateVariants.residuals)+1)
	residuals = append(residuals, existingVariants.residuals...)
	residuals = append(residuals, candidateVariants.residuals...)
	if len(residuals) == 0 {
		return merged, true
	}
	residuals = append(residuals, merged)
	return typ.NewUnion(residuals...), true
}

func splitFunctionFactVariants(t typ.Type) functionFactVariants {
	var out functionFactVariants
	collectFunctionFactVariants(t, &out)
	return out
}

func collectFunctionFactVariants(t typ.Type, out *functionFactVariants) {
	if t == nil || out == nil {
		return
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Union:
		for _, member := range v.Members {
			collectFunctionFactVariants(member, out)
		}
		return
	}
	if fn := unwrap.Function(t); fn != nil {
		out.funcs = append(out.funcs, fn)
		return
	}
	out.residuals = append(out.residuals, t)
}

func sameFunctionShapeForFactMerge(a, b *typ.Function) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a.TypeParams) != len(b.TypeParams) {
		return false
	}
	if !typeParamsEqual(a.TypeParams, b.TypeParams) {
		return false
	}
	if len(a.Params) != len(b.Params) {
		return false
	}
	// Param type precision and optionality may differ across iterations.
	// Treat those as mergeable slots and reconcile in mergeFunctionFactsByShape.
	return true
}

func mergeFunctionFactsByShape(existing, candidate *typ.Function) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	builder := typ.Func()
	for _, tp := range existing.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}

	for i, p := range existing.Params {
		paramType := mergeFunctionParamFactType(p.Type, candidate.Params[i].Type)
		name := p.Name
		if name == "" {
			name = candidate.Params[i].Name
		}
		optional := p.Optional || candidate.Params[i].Optional
		if optional {
			builder = builder.OptParam(name, paramType)
		} else {
			builder = builder.Param(name, paramType)
		}
	}

	if existing.Variadic != nil || candidate.Variadic != nil {
		builder = builder.Variadic(mergeFunctionParamFactType(existing.Variadic, candidate.Variadic))
	}

	if mergedReturns := returnsummary.Merge(existing.Returns, candidate.Returns); len(mergedReturns) > 0 {
		builder = builder.Returns(mergedReturns...)
	}

	effects := existing.Effects
	if effects == nil {
		effects = candidate.Effects
	}
	if effects != nil {
		builder = builder.Effects(effects)
	}
	spec := existing.Spec
	if spec == nil {
		spec = candidate.Spec
	}
	if spec != nil {
		builder = builder.Spec(spec)
	}
	refinement := existing.Refinement
	if refinement == nil {
		refinement = candidate.Refinement
	}
	if refinement != nil {
		builder = builder.WithRefinement(refinement)
	}

	return builder.Build()
}

func mergeFunctionParamFactType(existing, candidate typ.Type) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	existing = typ.PruneSoftUnionMembers(existing)
	candidate = typ.PruneSoftUnionMembers(candidate)
	if unwrap.IsNilType(existing) && !unwrap.IsNilType(candidate) {
		return candidate
	}
	if unwrap.IsNilType(candidate) && !unwrap.IsNilType(existing) {
		return existing
	}
	if preferred, ok := preferStructuredRecordParam(existing, candidate); ok {
		return preferred
	}
	if preferred, ok := value.PreferConcreteOverSoft(existing, candidate); ok {
		return preferred
	}
	if typ.IsUnknown(existing) {
		return candidate
	}
	if typ.IsUnknown(candidate) {
		return existing
	}
	if typ.IsAny(existing) && typ.IsAny(candidate) {
		return typ.Any
	}
	if typ.IsAny(existing) {
		return candidate
	}
	if typ.IsAny(candidate) {
		return existing
	}
	if typ.TypeEquals(existing, candidate) {
		return existing
	}
	if paramevidence.RefinesFunctionParam(candidate, existing) {
		return candidate
	}
	if paramevidence.RefinesFunctionParam(existing, candidate) {
		return existing
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		return existing
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

func preferStructuredRecordParam(existing, candidate typ.Type) (typ.Type, bool) {
	existingRec, okExisting := unwrap.Alias(existing).(*typ.Record)
	candidateRec, okCandidate := unwrap.Alias(candidate).(*typ.Record)
	if !okExisting || !okCandidate {
		return nil, false
	}

	existingOpenTop := existingRec.Open && len(existingRec.Fields) == 0 && !existingRec.HasMapComponent()
	candidateOpenTop := candidateRec.Open && len(candidateRec.Fields) == 0 && !candidateRec.HasMapComponent()
	if existingOpenTop == candidateOpenTop {
		return nil, false
	}
	if existingOpenTop {
		if candidateRec.HasMapComponent() || len(candidateRec.Fields) > 0 {
			return candidate, true
		}
	}
	if candidateOpenTop {
		if existingRec.HasMapComponent() || len(existingRec.Fields) > 0 {
			return existing, true
		}
	}
	return nil, false
}
