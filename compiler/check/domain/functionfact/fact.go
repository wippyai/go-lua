package functionfact

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Normalize canonicalizes one stored function fact.
func Normalize(ff api.FunctionFact) api.FunctionFact {
	return api.FunctionFact{
		Params:  paramevidence.FilterEmptyVector(ff.Params),
		Summary: returnsummary.Canonical(ff.Summary),
		Narrow:  returnsummary.Canonical(ff.Narrow),
		Type:    normalizeType(ff.Type),
	}
}

// Empty reports whether a canonical function fact contains no information.
func Empty(ff api.FunctionFact) bool {
	return len(ff.Params) == 0 && len(ff.Summary) == 0 && len(ff.Narrow) == 0 && ff.Type == nil
}

// Join precisely merges two observations for one local function during a single
// analysis iteration.
func Join(existing, candidate api.FunctionFact) api.FunctionFact {
	existing = Normalize(existing)
	candidate = Normalize(candidate)
	out := existing

	if len(candidate.Params) > 0 {
		out.Params = paramevidence.JoinVectors(out.Params, candidate.Params)
	}
	if len(candidate.Summary) > 0 {
		out.Summary = returnsummary.Merge(out.Summary, candidate.Summary)
	}
	if len(candidate.Narrow) > 0 {
		out.Narrow = returnsummary.Merge(out.Narrow, candidate.Narrow)
	}
	if candidate.Type != nil {
		out.Type = MergeType(out.Type, candidate.Type)
	}

	if len(out.Narrow) > 0 {
		if len(out.Summary) == 0 {
			out.Summary = returnsummary.Canonical(out.Narrow)
		} else {
			out.Summary = returnsummary.Merge(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Type); fn != nil {
		alignedSummary := out.Summary
		if len(alignedSummary) > 0 {
			if aligned, changed := returnsummary.AlignFunction(fn, alignedSummary); changed {
				out.Type = aligned
				fn = aligned
			}
		}
		if len(out.Summary) == 0 && fn != nil && len(fn.Returns) > 0 {
			out.Summary = returnsummary.Canonical(fn.Returns)
		}
	}

	return out
}

func normalizeType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return fn
	}
	return typ.PruneSoftUnionMembers(t)
}

// MergeType merges function-type facts through the canonical per-function fact
// policy.
func MergeType(existing, candidate typ.Type) typ.Type {
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}

	existingFn := unwrap.Function(existing)
	candidateFn := unwrap.Function(candidate)
	if mergedFromVariants, ok := mergeVariants(existing, candidate); ok {
		return mergedFromVariants
	}
	if existingFn != nil && candidateFn != nil {
		if SameShape(existingFn, candidateFn) {
			return mergeByShape(existingFn, candidateFn)
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

type variants struct {
	funcs     []*typ.Function
	residuals []typ.Type
}

func mergeVariants(existing, candidate typ.Type) (typ.Type, bool) {
	existingVariants := splitVariants(existing)
	candidateVariants := splitVariants(candidate)
	if len(existingVariants.funcs) == 0 || len(candidateVariants.funcs) == 0 {
		return nil, false
	}

	all := make([]*typ.Function, 0, len(existingVariants.funcs)+len(candidateVariants.funcs))
	all = append(all, existingVariants.funcs...)
	all = append(all, candidateVariants.funcs...)
	for i := 1; i < len(all); i++ {
		if !SameShape(all[0], all[i]) {
			return nil, false
		}
	}

	merged := all[0]
	for i := 1; i < len(all); i++ {
		next, _ := mergeByShape(merged, all[i]).(*typ.Function)
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

func splitVariants(t typ.Type) variants {
	var out variants
	collectVariants(t, &out)
	return out
}

func collectVariants(t typ.Type, out *variants) {
	if t == nil || out == nil {
		return
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Union:
		for _, member := range v.Members {
			collectVariants(member, out)
		}
		return
	}
	if fn := unwrap.Function(t); fn != nil {
		out.funcs = append(out.funcs, fn)
		return
	}
	out.residuals = append(out.residuals, t)
}

// SameShape reports whether two function fact types can be merged slot-wise.
func SameShape(a, b *typ.Function) bool {
	if a == nil || b == nil {
		return false
	}
	if len(a.TypeParams) != len(b.TypeParams) {
		return false
	}
	if !typeParamsEqual(a.TypeParams, b.TypeParams) {
		return false
	}
	return len(a.Params) == len(b.Params)
}

func mergeByShape(existing, candidate *typ.Function) typ.Type {
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
		paramType := mergeParamType(p.Type, candidate.Params[i].Type)
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
		builder = builder.Variadic(mergeParamType(existing.Variadic, candidate.Variadic))
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

func mergeParamType(existing, candidate typ.Type) typ.Type {
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
	if preferred, ok := preferStructuredRecord(existing, candidate); ok {
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

func preferStructuredRecord(existing, candidate typ.Type) (typ.Type, bool) {
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

func typeParamsEqual(a, b []*typ.TypeParam) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] == nil || b[i] == nil {
			if a[i] != b[i] {
				return false
			}
			continue
		}
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}
