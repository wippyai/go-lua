package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WidenFacts merges two interproc fact bundles.
func WidenFacts(prev, next api.Facts) api.Facts {
	NormalizeFunctionFactChannels(&prev)
	NormalizeFunctionFactChannels(&next)

	out := api.Facts{
		ParamHints:         WidenParamHints(prev.ParamHints, next.ParamHints),
		LiteralSigs:        WidenLiteralSigs(prev.LiteralSigs, next.LiteralSigs),
		CapturedTypes:      WidenCapturedTypes(prev.CapturedTypes, next.CapturedTypes),
		CapturedFields:     WidenCapturedFieldAssigns(prev.CapturedFields, next.CapturedFields),
		CapturedContainers: WidenCapturedContainerMutations(prev.CapturedContainers, next.CapturedContainers),
		ConstructorFields:  WidenConstructorFields(prev.ConstructorFields, next.ConstructorFields),
	}

	symbols := collectCanonicalFunctionFactSymbols(prev.FunctionFacts, next.FunctionFacts)
	if len(symbols) == 0 {
		return out
	}

	out.FunctionFacts = make(api.FunctionFacts, len(symbols))
	for _, sym := range symbols {
		prevFact := readFunctionFactFromFacts(&prev, sym)
		nextFact := readFunctionFactFromFacts(&next, sym)
		reconciled := ReconcileFunctionFact(ReconcileFunctionFactInput{
			ExistingSummary:  prevFact.Summary,
			ExistingNarrow:   prevFact.Narrow,
			ExistingFunc:     prevFact.Func,
			CandidateSummary: nextFact.Summary,
			CandidateNarrow:  nextFact.Narrow,
			CandidateFunc:    nextFact.Func,
		})
		writeFunctionFactToFacts(&out, sym, api.FunctionFact{
			Summary: widenReturnVectorForConvergence(reconciled.Summary),
			Narrow:  widenReturnVectorForConvergence(reconciled.Narrow),
			Func:    maybeWidenTypeForConvergence(reconciled.Func),
		})
	}
	if len(out.FunctionFacts) == 0 {
		out.FunctionFacts = nil
	}
	return out
}

// WidenReturnSummaries merges return summaries through the canonical
// return-summary merge policy shared by all iterative channels.
func WidenReturnSummaries(prev, next api.ReturnSummaries) api.ReturnSummaries {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.ReturnSummaries, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = widenReturnVectorForConvergence(NormalizeReturnVector(prev[sym]))
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		rets := next[sym]
		if existing := merged[sym]; existing != nil {
			merged[sym] = widenReturnVectorForConvergence(MergeReturnSummary(existing, rets))
		} else {
			merged[sym] = widenReturnVectorForConvergence(NormalizeReturnVector(rets))
		}
	}
	return merged
}

func shouldUseMonotoneReturnJoin(a, b []typ.Type) bool {
	for _, t := range a {
		if hasHigherOrderGrowthRisk(t) {
			return true
		}
	}
	for _, t := range b {
		if hasHigherOrderGrowthRisk(t) {
			return true
		}
	}
	return false
}

func hasHigherOrderGrowthRisk(t typ.Type) bool {
	if t == nil {
		return false
	}
	return scanType(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		switch n := node.(type) {
		case *typ.Function:
			for _, ret := range n.Returns {
				if typeContainsFunction(ret) {
					return true, false
				}
			}
		case *typ.Record:
			if recordHasSelfRecursiveMethod(n) {
				return true, false
			}
		}
		return false, true
	})
}

func typeContainsFunction(t typ.Type) bool {
	if t == nil {
		return false
	}
	return scanType(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		// Interface method signatures are behavioral contracts, not first-class
		// returned function values. Ignore them for higher-order growth risk.
		if _, ok := node.(*typ.Interface); ok {
			return false, false
		}
		if _, ok := node.(*typ.Function); ok {
			return true, false
		}
		return false, true
	})
}

func recordHasSelfRecursiveMethod(r *typ.Record) bool {
	if r == nil {
		return false
	}
	for _, f := range r.Fields {
		if methodTypeHasSelfRecursiveReturn(f.Type, r) {
			return true
		}
	}
	if r.HasMapComponent() && methodTypeHasSelfRecursiveReturn(r.MapValue, r) {
		return true
	}
	return false
}

func methodTypeHasSelfRecursiveReturn(t typ.Type, owner *typ.Record) bool {
	if t == nil || owner == nil {
		return false
	}
	return scanType(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		// Interface method signatures are behavioral contracts, not concrete
		// record method bodies. Treating them as self-recursive growth risk
		// over-applies monotone widening and blocks valid summary refinement.
		if _, ok := node.(*typ.Interface); ok {
			return false, false
		}
		fn, ok := node.(*typ.Function)
		if !ok {
			return false, true
		}
		for _, ret := range fn.Returns {
			if ret == nil {
				continue
			}
			if subtype.IsSubtype(ret, owner) || subtype.IsSubtype(owner, ret) ||
				TypeExtendsRecord(ret, owner) || TypeExtendsRecord(owner, ret) {
				return true, false
			}
		}
		return false, true
	})
}

func scanType(
	t typ.Type,
	guard internal.RecursionGuard,
	visit func(node typ.Type) (stop bool, descend bool),
) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	node := t
	for {
		ann, ok := node.(*typ.Annotated)
		if !ok || ann.Inner == nil || ann.Inner == node {
			break
		}
		node = ann.Inner
	}

	if stop, descend := visit(node); stop {
		return true
	} else if !descend {
		return false
	}

	switch n := node.(type) {
	case *typ.Optional:
		return scanType(n.Inner, next, visit)
	case *typ.Union:
		for _, m := range n.Members {
			if scanType(m, next, visit) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, m := range n.Members {
			if scanType(m, next, visit) {
				return true
			}
		}
		return false
	case *typ.Array:
		return scanType(n.Element, next, visit)
	case *typ.Map:
		return scanType(n.Key, next, visit) || scanType(n.Value, next, visit)
	case *typ.Tuple:
		for _, e := range n.Elements {
			if scanType(e, next, visit) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, p := range n.Params {
			if scanType(p.Type, next, visit) {
				return true
			}
		}
		for _, r := range n.Returns {
			if scanType(r, next, visit) {
				return true
			}
		}
		return n.Variadic != nil && scanType(n.Variadic, next, visit)
	case *typ.Record:
		for _, f := range n.Fields {
			if scanType(f.Type, next, visit) {
				return true
			}
		}
		if n.Metatable != nil && scanType(n.Metatable, next, visit) {
			return true
		}
		if n.HasMapComponent() {
			return scanType(n.MapKey, next, visit) || scanType(n.MapValue, next, visit)
		}
		return false
	case *typ.Alias:
		return scanType(n.Target, next, visit)
	case *typ.Instantiated:
		for _, a := range n.TypeArgs {
			if scanType(a, next, visit) {
				return true
			}
		}
		return false
	case *typ.Interface:
		for _, m := range n.Methods {
			if m.Type != nil && scanType(m.Type, next, visit) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func joinReturnVectorsMonotone(a, b []typ.Type) []typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi typ.Type
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		out[i] = joinReturnTypeMonotone(ai, bi)
	}
	return out
}

func joinReturnTypeMonotone(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) {
		return a
	}
	// Keep widening monotone: if one side is already an upper bound, keep it.
	if subtype.IsSubtype(a, b) || TypeExtendsRecord(a, b) || typeElidesOptional(a, b) {
		return b
	}
	if subtype.IsSubtype(b, a) || TypeExtendsRecord(b, a) || typeElidesOptional(b, a) {
		return a
	}
	return typ.JoinPreferNonSoft(a, b)
}

// WidenParamHints merges two param hint maps using monotone union.
func WidenParamHints(prev, next api.ParamHints) api.ParamHints {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return filterEmptyParamHints(next)
	}
	if next == nil {
		return filterEmptyParamHints(prev)
	}
	merged := make(api.ParamHints, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		hints := prev[sym]
		if hasNonNilHint(hints) {
			merged[sym] = hints
		}
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		hints := next[sym]
		if !hasNonNilHint(hints) {
			continue
		}
		if existing := merged[sym]; existing != nil {
			merged[sym] = joinParamHintVectors(existing, hints)
		} else {
			merged[sym] = hints
		}
	}
	return merged
}

func filterEmptyParamHints(hints api.ParamHints) api.ParamHints {
	if hints == nil {
		return nil
	}
	out := make(api.ParamHints, len(hints))
	for _, sym := range cfg.SortedSymbolIDs(hints) {
		v := hints[sym]
		if hasNonNilHint(v) {
			out[sym] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hasNonNilHint(hints []typ.Type) bool {
	for _, h := range hints {
		if h != nil {
			return true
		}
	}
	return false
}

// joinParamHintVectors joins two parameter hint vectors element-wise.
func joinParamHintVectors(a, b []typ.Type) []typ.Type {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	result := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi typ.Type
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		result[i] = joinIterationFact(ai, bi)
	}
	return result
}

// joinIterationFact joins a fact from the previous fixpoint iteration with the
// same fact from the current one: a parameter hint, a captured variable type,
// a captured field or container write, a constructor field. Both describe the
// same program values, the current one under better-resolved facts, so the
// join follows the information order of inference:
//   - an unresolved fact (nil, unknown or a soft placeholder) yields to a
//     resolved one, and soft placeholder members drop out of unions;
//   - records join field by field, keeping fields that only one iteration
//     has discovered; a mutable field is invariant, so differing resolved
//     field types resolve to the current one;
//   - resolved types otherwise join to their upper bound, so a fact that
//     widened between iterations admits the values of both.
func joinIterationFact(a, b typ.Type) typ.Type {
	return joinIterationFactAt(a, b, false)
}

// joinIterationFactAt joins two iteration facts at a covariant position, or
// at an invariant one such as a mutable record field or a map key or value.
// At an invariant position an upper bound admits only equivalent types, so
// resolved facts that are not equivalent resolve to the current one, the
// fact that describes the values at the position now.
func joinIterationFactAt(a, b typ.Type, invariant bool) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if unwrap.IsNilType(a) && !unwrap.IsNilType(b) {
		return b
	}
	if unwrap.IsNilType(b) && !unwrap.IsNilType(a) {
		return a
	}
	if typ.IsUnknown(a) {
		return b
	}
	if typ.IsUnknown(b) {
		return a
	}
	a = typ.PruneSoftUnionMembers(a)
	b = typ.PruneSoftUnionMembers(b)
	softA := typ.IsSoft(a, typ.SoftPlaceholderPolicy)
	softB := typ.IsSoft(b, typ.SoftPlaceholderPolicy)
	if softA && !softB {
		return b
	}
	if softB && !softA {
		return a
	}
	if joined, ok := joinIterationRecords(a, b); ok {
		return joined
	}
	// The previous fact stays while it admits the current one, so equivalent
	// facts with different spellings do not alternate between iterations.
	previousAdmitsCurrent := subtype.IsSubtype(b, a)
	if invariant {
		if previousAdmitsCurrent && subtype.IsSubtype(a, b) {
			return a
		}
		return b
	}
	if previousAdmitsCurrent {
		return a
	}
	if subtype.IsSubtype(a, b) {
		return b
	}
	return typ.JoinPreferNonSoft(a, b)
}

// joinIterationRecords joins two record hints field by field with
// joinIterationFact. Records with different map-component shapes or conflicting
// metatables are not joined here.
func joinIterationRecords(a, b typ.Type) (typ.Type, bool) {
	ar, ok := a.(*typ.Record)
	if !ok {
		return nil, false
	}
	br, ok := b.(*typ.Record)
	if !ok {
		return nil, false
	}
	if ar.HasMapComponent() != br.HasMapComponent() {
		return nil, false
	}
	metatable := ar.Metatable
	switch {
	case metatable == nil:
		metatable = br.Metatable
	case br.Metatable != nil && !typ.TypeEquals(metatable, br.Metatable):
		return nil, false
	}

	// The join reuses an input record it equals, so unchanged hints keep their
	// identity across iterations.
	sameAsA := ar.Metatable == metatable && ar.Open == (ar.Open || br.Open)
	sameAsB := br.Metatable == metatable && br.Open == (ar.Open || br.Open)

	builder := typ.NewRecord().SetOpen(ar.Open || br.Open)
	if metatable != nil {
		builder.Metatable(metatable)
	}
	if ar.HasMapComponent() {
		key := joinIterationFactAt(ar.MapKey, br.MapKey, true)
		value := joinIterationFactAt(ar.MapValue, br.MapValue, true)
		sameAsA = sameAsA && key == ar.MapKey && value == ar.MapValue
		sameAsB = sameAsB && key == br.MapKey && value == br.MapValue
		builder.MapComponent(key, value)
	}
	for _, fa := range ar.Fields {
		field := fa
		fb := br.GetField(fa.Name)
		if fb != nil {
			field.Optional = fa.Optional || fb.Optional
			field.Readonly = fa.Readonly && fb.Readonly
			field.Type = joinIterationFactAt(fa.Type, fb.Type, !field.Readonly)
		}
		sameAsA = sameAsA && field == fa
		sameAsB = sameAsB && fb != nil && field == *fb
		addIterationField(builder, field)
	}
	for _, fb := range br.Fields {
		if ar.GetField(fb.Name) == nil {
			sameAsA = false
			addIterationField(builder, fb)
		}
	}
	switch {
	case sameAsA:
		return a, true
	case sameAsB:
		return b, true
	}
	return builder.Build(), true
}

func addIterationField(builder *typ.RecordBuilder, f typ.Field) {
	switch {
	case f.Optional && f.Readonly:
		builder.OptReadonlyField(f.Name, f.Type)
	case f.Optional:
		builder.OptField(f.Name, f.Type)
	case f.Readonly:
		builder.ReadonlyField(f.Name, f.Type)
	default:
		builder.Field(f.Name, f.Type)
	}
}

// WidenLiteralSigs merges two literal signature maps.
func WidenLiteralSigs(prev, next api.LiteralSigs) api.LiteralSigs {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.LiteralSigs, len(prev)+len(next))
	for fn, sig := range prev {
		merged[fn] = maybeWidenFunctionForConvergence(sig)
	}
	for fn, sig := range next {
		if existing := merged[fn]; existing != nil {
			merged[fn] = maybeWidenFunctionForConvergence(mergeLiteralSig(existing, sig))
		} else {
			merged[fn] = maybeWidenFunctionForConvergence(sig)
		}
	}
	return merged
}

func mergeLiteralSig(prev, next *typ.Function) *typ.Function {
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	if merged, ok := mergeFunctionReturnsIfSameShape(prev, next); ok {
		if fn, ok := merged.(*typ.Function); ok {
			return fn
		}
	}
	if subtype.IsSubtype(prev, next) {
		return next
	}
	if subtype.IsSubtype(next, prev) {
		return prev
	}
	// Literal signatures are constrained to *typ.Function. For incomparable
	// function shapes, keep the prior stable signature instead of narrowing.
	return prev
}

// WidenCapturedTypes merges two captured type maps using monotone join.
func WidenCapturedTypes(prev, next api.CapturedTypes) api.CapturedTypes {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.CapturedTypes, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		t := next[sym]
		if existing := merged[sym]; existing != nil {
			merged[sym] = maybeWidenTypeForConvergence(joinIterationFact(existing, t))
		} else {
			merged[sym] = maybeWidenTypeForConvergence(t)
		}
	}
	return merged
}

// WidenCapturedFieldAssigns merges captured field assignment maps using monotone union.
func WidenCapturedFieldAssigns(prev, next api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.CapturedFieldAssigns, len(prev)+len(next))
	for _, callee := range cfg.SortedSymbolIDs(prev) {
		merged[callee] = prev[callee]
	}
	for _, callee := range cfg.SortedSymbolIDs(next) {
		captured := next[callee]
		existing := merged[callee]
		if existing == nil {
			merged[callee] = captured
			continue
		}
		merged[callee] = MergeCapturedFieldSymbolMaps(existing, captured, func(prev typ.Type, next typ.Type) typ.Type {
			if prev != nil {
				return maybeWidenTypeForConvergence(joinIterationFact(prev, next))
			}
			return maybeWidenTypeForConvergence(next)
		})
	}
	return merged
}

// WidenCapturedContainerMutations merges captured container mutation maps using monotone union.
func WidenCapturedContainerMutations(prev, next api.CapturedContainerMutations) api.CapturedContainerMutations {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.CapturedContainerMutations, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		muts := next[sym]
		existing := merged[sym]
		merged[sym] = MergeCapturedContainerMutationMaps(existing, muts, func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation {
			if prev != nil {
				next.ValueType = maybeWidenTypeForConvergence(joinIterationFact(prev.ValueType, next.ValueType))
			} else {
				next.ValueType = maybeWidenTypeForConvergence(next.ValueType)
			}
			return next
		})
	}
	return merged
}

// WidenConstructorFields merges constructor field maps using monotone join.
func WidenConstructorFields(prev, next api.ConstructorFields) api.ConstructorFields {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.ConstructorFields, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		fields := next[sym]
		existing := merged[sym]
		if existing == nil {
			merged[sym] = fields
			continue
		}
		out := make(map[string]typ.Type, len(existing)+len(fields))
		for _, name := range cfg.SortedFieldNames(existing) {
			out[name] = existing[name]
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			t := fields[name]
			if prevType := out[name]; prevType != nil {
				out[name] = maybeWidenTypeForConvergence(joinIterationFact(prevType, t))
			} else {
				out[name] = maybeWidenTypeForConvergence(t)
			}
		}
		merged[sym] = out
	}
	return merged
}

func mergeFunctionReturnsIfSameShape(prevFn, nextFn *typ.Function) (typ.Type, bool) {
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
	normalizeReturn := func(t typ.Type) typ.Type {
		if t == nil {
			return nil
		}
		return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
			tp, ok := node.(*typ.TypeParam)
			if !ok {
				return node, false
			}
			if allowedTypeParams[tp.Name] {
				return node, false
			}
			// Free type params in non-generic function returns are unstable placeholders.
			return typ.Unknown, true
		})
	}
	normalizedPrev := make([]typ.Type, len(prevFn.Returns))
	normalizedNext := make([]typ.Type, len(nextFn.Returns))
	for i := range prevFn.Returns {
		normalizedPrev[i] = normalizeReturn(prevFn.Returns[i])
		normalizedNext[i] = normalizeReturn(nextFn.Returns[i])
	}

	mergedReturns := typjoin.ReturnVectors(normalizedPrev, normalizedNext)
	if ReturnTypesEqual(prevFn.Returns, mergedReturns) {
		return prevFn, true
	}
	if ReturnTypesEqual(nextFn.Returns, mergedReturns) {
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

func widenReturnVectorForConvergence(rets []typ.Type) []typ.Type {
	if len(rets) == 0 {
		return rets
	}
	out := make([]typ.Type, len(rets))
	changed := false
	for i, t := range rets {
		wt := maybeWidenTypeForConvergence(t)
		out[i] = wt
		if wt != t {
			changed = true
		}
	}
	if !changed {
		return rets
	}
	return out
}

func maybeWidenTypeForConvergence(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if !hasHigherOrderGrowthRisk(t) {
		return t
	}
	return foldSelfRecursiveRecords(subtype.WidenForInference(t))
}

// selfRecursiveRecordName names the recursive types produced by
// foldSelfRecursiveRecords.
const selfRecursiveRecordName = "self"

// foldSelfRecursiveRecords closes records whose methods reach an earlier
// approximation of the record itself.
//
// A table whose methods return the table is inferred one level at a time: each
// fixpoint iteration types the methods against the table type of the previous
// iteration, so the table type T_n nests T_(n-1) in its method signatures.
// The chain T_0 <: T_1 <: ... ascends forever. Replacing every nested
// approximation T' of T (same fields, T' <: T) by a reference to T itself
// yields mu X. T[T' := X], an upper bound of the whole chain and its limit.
func foldSelfRecursiveRecords(t typ.Type) typ.Type {
	folded := make(map[*typ.Record]typ.Type)
	return typ.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		rec, ok := node.(*typ.Record)
		if !ok {
			return nil, false
		}
		if out, ok := folded[rec]; ok {
			return out, true
		}
		out := foldSelfRecursiveRecord(rec)
		if out == typ.Type(rec) {
			return nil, false
		}
		folded[rec] = out
		return out, true
	})
}

// foldSelfRecursiveRecord returns mu X. owner[T' := X] for every approximation
// T' of owner nested in owner, or owner itself when none is nested.
func foldSelfRecursiveRecord(owner *typ.Record) typ.Type {
	return typ.FoldApproximations(selfRecursiveRecordName, owner, func(node typ.Type) bool {
		return isRecordApproximation(node, owner)
	})
}

// isRecordApproximation reports whether t is an approximation of owner: a
// record, or a recursive record, with exactly owner's fields that is a
// subtype of owner.
func isRecordApproximation(t typ.Type, owner *typ.Record) bool {
	shape := unwrap.Alias(t)
	if rr, ok := shape.(*typ.Recursive); ok {
		shape = rr.Body
	}
	rec, ok := shape.(*typ.Record)
	if !ok || !rec.HasSameFieldNames(owner) {
		return false
	}
	return subtype.IsSubtype(t, owner)
}

func maybeWidenFunctionForConvergence(fn *typ.Function) *typ.Function {
	if fn == nil {
		return nil
	}
	if widened, ok := maybeWidenTypeForConvergence(fn).(*typ.Function); ok {
		return widened
	}
	return fn
}
