package functionfact

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/compiler/check/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Normalize canonicalizes one stored function fact.
func Normalize(ff api.FunctionFact) api.FunctionFact {
	return api.FunctionFact{
		Params:  paramevidence.FilterEmptyVector(ff.Params),
		Summary: returnsummary.Canonical(ff.Summary),
		Narrow:  returnsummary.Canonical(ff.Narrow),
		Type:    value.NormalizeFactType(ff.Type),
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

	summaryBeforeNarrow := out.Summary
	if len(out.Narrow) > 0 {
		if len(out.Summary) == 0 {
			out.Summary = returnsummary.Canonical(out.Narrow)
		} else {
			out.Summary = returnsummary.Merge(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Type); fn != nil {
		alignedReturns := out.Summary
		usingNarrow := len(out.Narrow) > 0 && !returnsummary.AllNil(out.Narrow)
		if usingNarrow {
			repairBase := summaryBeforeNarrow
			if len(repairBase) == 0 {
				repairBase = out.Summary
			}
			alignedReturns = repairSummaryWithNarrow(repairBase, out.Narrow)
		}
		if len(alignedReturns) > 0 {
			if usingNarrow {
				if aligned := typjoin.WithReturns(fn, alignedReturns); aligned != nil {
					out.Type = aligned
					fn = aligned
				}
			} else {
				if aligned, changed := returnsummary.AlignFunction(fn, alignedReturns); changed {
					out.Type = aligned
					fn = aligned
				}
			}
		}
		if len(out.Summary) == 0 && fn != nil && len(fn.Returns) > 0 {
			out.Summary = returnsummary.Canonical(fn.Returns)
		}
	}

	return out
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

// WidenForConvergence merges one function fact at a recursive fixpoint
// boundary.
func WidenForConvergence(prev, next api.FunctionFact) api.FunctionFact {
	out := api.FunctionFact{
		Params:  paramevidence.JoinVectors(prev.Params, next.Params),
		Summary: returnsummary.WidenForConvergence(prev.Summary, next.Summary),
		Narrow:  returnsummary.WidenForConvergence(prev.Narrow, next.Narrow),
		Type:    WidenTypeForConvergence(prev.Type, next.Type),
	}

	summaryBeforeNarrow := out.Summary
	// Narrow summaries can refine optional/non-nil returns, but a nil-only
	// narrow observation must not erase an already-informative summary.
	if len(out.Narrow) > 0 && !returnsummary.AllNil(out.Narrow) {
		if len(out.Summary) == 0 {
			out.Summary = returnsummary.Canonical(out.Narrow)
		} else {
			out.Summary = returnsummary.WidenForConvergence(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Type); fn != nil {
		alignedReturns := out.Summary
		usingNarrow := len(out.Narrow) > 0 && !returnsummary.AllNil(out.Narrow)
		if usingNarrow {
			repairBase := summaryBeforeNarrow
			if len(repairBase) == 0 {
				repairBase = out.Summary
			}
			alignedReturns = repairSummaryWithNarrow(repairBase, out.Narrow)
		}
		if len(alignedReturns) > 0 {
			if usingNarrow {
				if aligned := typjoin.WithReturns(fn, alignedReturns); aligned != nil {
					out.Type = value.WidenForConvergence(aligned)
				}
			} else {
				if aligned, changed := returnsummary.AlignFunction(fn, alignedReturns); changed {
					out.Type = WidenTypeForConvergence(fn, aligned)
				}
			}
		} else if len(fn.Returns) > 0 {
			out.Summary = returnsummary.WidenForConvergence(nil, fn.Returns)
		}
	}

	return out
}

func repairSummaryWithNarrow(summary, narrow []typ.Type) []typ.Type {
	if len(narrow) == 0 {
		return summary
	}
	if len(summary) != len(narrow) || len(summary) == 0 {
		return narrow
	}
	out := make([]typ.Type, len(summary))
	for i := range summary {
		out[i] = repairTypeWithNarrow(summary[i], narrow[i], 0)
	}
	return out
}

func repairTypeWithNarrow(summary, narrow typ.Type, depth int) typ.Type {
	if summary == nil || narrow == nil || depth > typ.DefaultRecursionDepth {
		return narrow
	}
	if typ.IsAny(summary) && !typ.IsAny(narrow) {
		return narrow
	}
	summary = unwrap.Alias(summary)
	narrow = unwrap.Alias(narrow)
	switch s := summary.(type) {
	case *typ.Union:
		n, ok := narrow.(*typ.Union)
		if !ok {
			members := make([]typ.Type, len(s.Members))
			for i, member := range s.Members {
				members[i] = repairTypeWithNarrow(member, narrow, depth+1)
			}
			return typ.NewUnion(members...)
		}
		if len(s.Members) != len(n.Members) {
			return summary
		}
		members := make([]typ.Type, len(s.Members))
		for i, member := range s.Members {
			members[i] = repairTypeWithNarrow(member, bestNarrowUnionMember(member, n.Members), depth+1)
		}
		return typ.NewUnion(members...)
	case *typ.Record:
		n, ok := narrow.(*typ.Record)
		if !ok {
			return narrow
		}
		builder := typ.NewRecord().SetOpen(s.Open)
		if s.HasMapComponent() {
			mapValue := s.MapValue
			if n.HasMapComponent() {
				mapValue = repairTypeWithNarrow(s.MapValue, n.MapValue, depth+1)
			}
			builder.MapComponent(s.MapKey, mapValue)
		}
		if s.Metatable != nil {
			builder.Metatable(s.Metatable)
		}
		for _, field := range s.Fields {
			fieldType := field.Type
			if nf := n.GetField(field.Name); nf != nil {
				fieldType = repairTypeWithNarrow(field.Type, nf.Type, depth+1)
			}
			switch {
			case field.Optional && field.Readonly:
				builder.OptReadonlyField(field.Name, fieldType)
			case field.Optional:
				builder.OptField(field.Name, fieldType)
			case field.Readonly:
				builder.ReadonlyField(field.Name, fieldType)
			default:
				builder.Field(field.Name, fieldType)
			}
		}
		return builder.Build()
	default:
		return narrow
	}
}

func bestNarrowUnionMember(summary typ.Type, members []typ.Type) typ.Type {
	for _, member := range members {
		if subtype.IsSubtype(member, summary) || subtype.IsSubtype(summary, member) {
			return member
		}
	}
	if len(members) > 0 {
		return members[0]
	}
	return summary
}

// WidenTypeForConvergence merges function-type facts at a recursive fixpoint
// boundary.
func WidenTypeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = value.NormalizeFactType(existing)
	candidate = value.NormalizeFactType(candidate)
	if existing == nil {
		return value.WidenForConvergence(candidate)
	}
	if candidate == nil {
		return value.WidenForConvergence(existing)
	}
	existingFn := unwrap.Function(existing)
	candidateFn := unwrap.Function(candidate)
	if existingFn != nil && candidateFn != nil && SameShape(existingFn, candidateFn) {
		return value.WidenForConvergence(widenByShapeForConvergence(existingFn, candidateFn))
	}
	return value.MergeForConvergence(existing, candidate)
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

func widenByShapeForConvergence(existing, candidate *typ.Function) typ.Type {
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
		paramType := widenParamTypeForConvergence(p.Type, candidate.Params[i].Type)
		name := p.Name
		if name == "" {
			name = candidate.Params[i].Name
		}
		if p.Optional || candidate.Params[i].Optional {
			builder = builder.OptParam(name, paramType)
		} else {
			builder = builder.Param(name, paramType)
		}
	}
	if existing.Variadic != nil || candidate.Variadic != nil {
		builder = builder.Variadic(widenParamTypeForConvergence(existing.Variadic, candidate.Variadic))
	}
	if returns := returnsummary.WidenForConvergence(existing.Returns, candidate.Returns); len(returns) > 0 {
		builder = builder.Returns(returns...)
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

func widenParamTypeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = value.NormalizeFactType(existing)
	candidate = value.NormalizeFactType(candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	if typ.TypeEquals(existing, candidate) {
		return existing
	}
	if typ.IsAny(existing) || typ.IsUnknown(existing) {
		return existing
	}
	if typ.IsAny(candidate) || typ.IsUnknown(candidate) {
		return candidate
	}
	if preferred, ok := value.PreferConcreteOverSoft(existing, candidate); ok {
		return preferred
	}
	if paramevidence.RefinesFunctionParam(candidate, existing) {
		return candidate
	}
	if paramevidence.RefinesFunctionParam(existing, candidate) {
		return existing
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		return existing
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
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

// MergeReturnsForSameSignature merges return slots for function signatures that
// already have identical call shapes.
func MergeReturnsForSameSignature(prevFn, nextFn *typ.Function) (typ.Type, bool) {
	if prevFn == nil || nextFn == nil {
		return nil, false
	}
	if len(prevFn.TypeParams) != len(nextFn.TypeParams) {
		return nil, false
	}
	if !typeParamsEqual(prevFn.TypeParams, nextFn.TypeParams) {
		return nil, false
	}
	if len(prevFn.Params) != len(nextFn.Params) {
		return nil, false
	}
	if (prevFn.Variadic == nil) != (nextFn.Variadic == nil) {
		return nil, false
	}
	if prevFn.Variadic != nil && !typ.TypeEquals(prevFn.Variadic, nextFn.Variadic) {
		return nil, false
	}
	for i := range prevFn.Params {
		if prevFn.Params[i].Optional != nextFn.Params[i].Optional {
			return nil, false
		}
		if !typ.TypeEquals(prevFn.Params[i].Type, nextFn.Params[i].Type) {
			return nil, false
		}
	}
	if len(prevFn.Returns) == 0 && len(nextFn.Returns) == 0 {
		return prevFn, true
	}
	if len(prevFn.Returns) != len(nextFn.Returns) || len(prevFn.Returns) == 0 {
		return nil, false
	}

	allowedTypeParams := make(map[string]bool, len(prevFn.TypeParams))
	for _, tp := range prevFn.TypeParams {
		if tp != nil && tp.Name != "" {
			allowedTypeParams[tp.Name] = true
		}
	}
	normalizeReturn := func(t typ.Type) (typ.Type, bool) {
		if t == nil {
			return nil, false
		}
		leaked := false
		return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
			tp, ok := node.(*typ.TypeParam)
			if !ok {
				return node, false
			}
			if allowedTypeParams[tp.Name] {
				return node, false
			}
			// Free type params in non-generic function returns are unstable placeholders.
			leaked = true
			return typ.Unknown, true
		}), leaked
	}
	normalizedPrev := make([]typ.Type, len(prevFn.Returns))
	normalizedNext := make([]typ.Type, len(nextFn.Returns))
	leakedPrev := make([]bool, len(prevFn.Returns))
	leakedNext := make([]bool, len(nextFn.Returns))
	for i := range prevFn.Returns {
		normalizedPrev[i], leakedPrev[i] = normalizeReturn(prevFn.Returns[i])
		normalizedNext[i], leakedNext[i] = normalizeReturn(nextFn.Returns[i])
	}

	mergedReturns := make([]typ.Type, len(normalizedPrev))
	for i := range mergedReturns {
		switch {
		case leakedPrev[i] && !leakedNext[i]:
			mergedReturns[i] = normalizedNext[i]
		case leakedNext[i] && !leakedPrev[i]:
			mergedReturns[i] = normalizedPrev[i]
		default:
			mergedReturns[i] = typ.JoinReturnSlot(normalizedPrev[i], normalizedNext[i])
		}
	}
	if returnsummary.Equal(prevFn.Returns, mergedReturns) {
		return prevFn, true
	}
	if returnsummary.Equal(nextFn.Returns, mergedReturns) {
		return nextFn, true
	}

	effects := prevFn.Effects
	if effects == nil {
		effects = nextFn.Effects
	}
	spec := prevFn.Spec
	if spec == nil {
		spec = nextFn.Spec
	}
	refinement := prevFn.Refinement
	if refinement == nil {
		refinement = nextFn.Refinement
	}

	builder := typ.Func().
		Effects(effects).
		Spec(spec).
		WithRefinement(refinement)
	for _, tp := range prevFn.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range prevFn.Params {
		if p.Optional {
			builder = builder.OptParam(p.Name, p.Type)
		} else {
			builder = builder.Param(p.Name, p.Type)
		}
	}
	if prevFn.Variadic != nil {
		builder = builder.Variadic(prevFn.Variadic)
	}
	builder = builder.Returns(mergedReturns...)
	return builder.Build(), true
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
