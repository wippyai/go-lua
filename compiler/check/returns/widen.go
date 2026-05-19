package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/infer/paramevidence"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WidenFacts merges two interproc fact bundles.
func WidenFacts(prev, next api.Facts) api.Facts {
	out := api.Facts{
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
		writeNormalizedFunctionFactToFacts(&out, sym, widenFunctionFactForConvergence(prevFact, nextFact))
	}
	if len(out.FunctionFacts) == 0 {
		out.FunctionFacts = nil
	}
	return out
}

// JoinFacts performs a precise same-iteration merge of interproc facts.
// Unlike WidenFacts, this may keep directional refinements that are useful
// inside one analysis round. Recursive fixpoint boundaries must use WidenFacts.
func JoinFacts(prev, next api.Facts) api.Facts {
	out := api.Facts{
		LiteralSigs:        JoinLiteralSigs(prev.LiteralSigs, next.LiteralSigs),
		CapturedTypes:      JoinCapturedTypes(prev.CapturedTypes, next.CapturedTypes),
		CapturedFields:     JoinCapturedFieldAssigns(prev.CapturedFields, next.CapturedFields),
		CapturedContainers: JoinCapturedContainerMutations(prev.CapturedContainers, next.CapturedContainers),
		ConstructorFields:  JoinConstructorFields(prev.ConstructorFields, next.ConstructorFields),
	}

	symbols := collectCanonicalFunctionFactSymbols(prev.FunctionFacts, next.FunctionFacts)
	if len(symbols) > 0 {
		out.FunctionFacts = make(api.FunctionFacts, len(symbols))
	}
	for _, sym := range symbols {
		prevFact := readFunctionFactFromFacts(&prev, sym)
		nextFact := readFunctionFactFromFacts(&next, sym)
		writeNormalizedFunctionFactToFacts(&out, sym, JoinFunctionFact(prevFact, nextFact))
	}
	return out
}

func widenFunctionFactForConvergence(prev, next api.FunctionFact) api.FunctionFact {
	out := api.FunctionFact{
		Params:  joinParameterEvidenceVectors(prev.Params, next.Params),
		Summary: widenReturnSummaryForConvergence(prev.Summary, next.Summary),
		Narrow:  widenReturnSummaryForConvergence(prev.Narrow, next.Narrow),
		Type:    widenFunctionFactTypeForConvergence(prev.Type, next.Type),
	}

	// Narrow summaries can refine optional/non-nil returns, but a nil-only
	// narrow observation must not erase an already-informative summary.
	if len(out.Narrow) > 0 && !ReturnTypesAllNil(out.Narrow) {
		if len(out.Summary) == 0 {
			out.Summary = canonicalReturnVector(out.Narrow)
		} else {
			out.Summary = widenReturnSummaryForConvergence(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Type); fn != nil {
		if len(out.Summary) > 0 {
			if aligned, changed := AlignFunctionTypeWithSummary(fn, out.Summary); changed {
				out.Type = widenFunctionFactTypeForConvergence(fn, aligned)
			}
		} else if len(fn.Returns) > 0 {
			out.Summary = widenReturnSummaryForConvergence(nil, fn.Returns)
		}
	}

	return out
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

func widenReturnSummaryForConvergence(prev, next []typ.Type) []typ.Type {
	prev = normalizeAndPruneReturnVector(prev)
	next = normalizeAndPruneReturnVector(next)
	if len(prev) == 0 {
		return widenReturnVectorForConvergence(next)
	}
	if len(next) == 0 {
		return widenReturnVectorForConvergence(prev)
	}

	merged := MergeReturnSummary(prev, next)
	if returnVectorUnsafePrecisionDrop(prev, merged) {
		merged = prev
	}
	return widenReturnVectorForConvergence(normalizeAndPruneReturnVector(merged))
}

func returnVectorUnsafePrecisionDrop(prev, merged []typ.Type) bool {
	if len(prev) == 0 || len(merged) == 0 || len(prev) != len(merged) {
		return false
	}
	for i := range prev {
		if typeUnsafePrecisionDrop(prev[i], merged[i]) {
			return true
		}
	}
	return false
}

func typeUnsafePrecisionDrop(prev, merged typ.Type) bool {
	if prev == nil || merged == nil || typ.TypeEquals(prev, merged) {
		return false
	}
	if typeElidesOptional(merged, prev) {
		return false
	}
	if refines, _ := typeRefinesFalsyMapKey(merged, prev); refines {
		return false
	}
	if typ.IsAny(prev) || typ.IsUnknown(prev) {
		return true
	}

	switch p := unwrapStructuralShape(prev).(type) {
	case *typ.Union:
		if unionStrictMemberSubset(merged, p) {
			return true
		}
		if subtype.IsSubtype(merged, p) && !subtype.IsSubtype(p, merged) {
			return true
		}
	case *typ.Record:
		m, ok := unwrapStructuralShape(merged).(*typ.Record)
		if !ok {
			break
		}
		for _, pf := range p.Fields {
			mf := m.GetField(pf.Name)
			if mf != nil && typeUnsafePrecisionDrop(pf.Type, mf.Type) {
				return true
			}
		}
		if p.HasMapComponent() && m.HasMapComponent() && typeUnsafePrecisionDrop(p.MapValue, m.MapValue) {
			return true
		}
	case *typ.Array:
		if m, ok := unwrapStructuralShape(merged).(*typ.Array); ok {
			return typeUnsafePrecisionDrop(p.Element, m.Element)
		}
	case *typ.Map:
		if m, ok := unwrapStructuralShape(merged).(*typ.Map); ok {
			return typeUnsafePrecisionDrop(p.Key, m.Key) || typeUnsafePrecisionDrop(p.Value, m.Value)
		}
	case *typ.Tuple:
		m, ok := unwrapStructuralShape(merged).(*typ.Tuple)
		if !ok || len(p.Elements) != len(m.Elements) {
			break
		}
		for i := range p.Elements {
			if typeUnsafePrecisionDrop(p.Elements[i], m.Elements[i]) {
				return true
			}
		}
	case *typ.Function:
		m, ok := unwrapStructuralShape(merged).(*typ.Function)
		if !ok {
			break
		}
		for i := 0; i < len(p.Params) && i < len(m.Params); i++ {
			if typeUnsafePrecisionDrop(p.Params[i].Type, m.Params[i].Type) {
				return true
			}
		}
		for i := 0; i < len(p.Returns) && i < len(m.Returns); i++ {
			if typeUnsafePrecisionDrop(p.Returns[i], m.Returns[i]) {
				return true
			}
		}
	}

	if subtype.IsSubtype(merged, prev) && !subtype.IsSubtype(prev, merged) && !TypeExtendsRecord(merged, prev) {
		return true
	}
	return false
}

func unionStrictMemberSubset(candidate typ.Type, baseline *typ.Union) bool {
	if baseline == nil {
		return false
	}
	candidateMembers := unionMembers(candidate)
	if len(candidateMembers) == 0 {
		candidateMembers = []typ.Type{candidate}
	}
	if len(candidateMembers) >= len(baseline.Members) {
		return false
	}
	for _, member := range candidateMembers {
		found := false
		for _, baseMember := range baseline.Members {
			if typ.TypeEquals(member, baseMember) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func unionMembers(t typ.Type) []typ.Type {
	switch v := unwrapStructuralShape(t).(type) {
	case *typ.Union:
		return v.Members
	case *typ.Optional:
		return append([]typ.Type{typ.Nil}, unionMembers(v.Inner)...)
	default:
		return nil
	}
}

// WidenParameterEvidence merges two parameter evidence maps using the same
// vector law used by canonical FunctionFacts.
func WidenParameterEvidence(prev, next map[cfg.SymbolID][]typ.Type) map[cfg.SymbolID][]typ.Type {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return filterEmptyParameterEvidence(next)
	}
	if next == nil {
		return filterEmptyParameterEvidence(prev)
	}
	merged := make(map[cfg.SymbolID][]typ.Type, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		evidence := normalizeParameterEvidenceVector(prev[sym])
		if hasNonNilEvidence(evidence) {
			merged[sym] = evidence
		}
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		evidence := normalizeParameterEvidenceVector(next[sym])
		if !hasNonNilEvidence(evidence) {
			continue
		}
		if existing := merged[sym]; existing != nil {
			merged[sym] = joinParameterEvidenceVectors(existing, evidence)
		} else {
			merged[sym] = evidence
		}
	}
	return merged
}

func filterEmptyParameterEvidence(evidence map[cfg.SymbolID][]typ.Type) map[cfg.SymbolID][]typ.Type {
	if evidence == nil {
		return nil
	}
	out := make(map[cfg.SymbolID][]typ.Type, len(evidence))
	for _, sym := range cfg.SortedSymbolIDs(evidence) {
		v := filterEmptyParameterEvidenceVector(evidence[sym])
		if hasNonNilEvidence(v) {
			out[sym] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func filterEmptyParameterEvidenceVector(evidence []typ.Type) []typ.Type {
	v := normalizeParameterEvidenceVector(evidence)
	if !hasNonNilEvidence(v) {
		return nil
	}
	return v
}

func normalizeParameterEvidenceVector(evidence []typ.Type) []typ.Type {
	var out []typ.Type
	for i, observed := range evidence {
		normalized := paramevidence.NormalizeType(observed)
		if out != nil {
			out[i] = normalized
			continue
		}
		if !typ.TypeEquals(observed, normalized) {
			out = make([]typ.Type, len(evidence))
			copy(out, evidence[:i])
			out[i] = normalized
		}
	}
	if out != nil {
		return out
	}
	return evidence
}

func hasNonNilEvidence(evidence []typ.Type) bool {
	for _, observed := range evidence {
		if observed != nil {
			return true
		}
	}
	return false
}

// joinParameterEvidenceVectors joins two parameter evidence vectors element-wise.
func joinParameterEvidenceVectors(a, b []typ.Type) []typ.Type {
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
		result[i] = joinParameterEvidence(ai, bi)
	}
	return result
}

func joinParameterEvidence(a, b typ.Type) typ.Type {
	a = paramevidence.NormalizeType(a)
	b = paramevidence.NormalizeType(b)
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
	if joined, ok := joinNilableParameterEvidence(a, b); ok {
		return joined
	}
	return joinNonNilParameterEvidence(a, b)
}

func joinNilableParameterEvidence(a, b typ.Type) (typ.Type, bool) {
	ai, anil := splitNilableParameterEvidence(a)
	bi, bnil := splitNilableParameterEvidence(b)
	if !anil && !bnil {
		return nil, false
	}
	if ai == nil && bi == nil {
		return typ.Nil, true
	}
	if ai == nil {
		return typ.NewOptional(bi), true
	}
	if bi == nil {
		return typ.NewOptional(ai), true
	}
	return typ.NewOptional(joinNonNilParameterEvidence(ai, bi)), true
}

func splitNilableParameterEvidence(t typ.Type) (typ.Type, bool) {
	t = unwrap.Alias(t)
	switch v := t.(type) {
	case nil:
		return nil, false
	case *typ.Optional:
		return v.Inner, true
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		nilable := false
		for _, member := range v.Members {
			member = unwrap.Alias(member)
			if unwrap.IsNilType(member) {
				nilable = true
				continue
			}
			members = append(members, member)
		}
		if !nilable {
			return t, false
		}
		return typ.NewUnion(members...), true
	default:
		if unwrap.IsNilType(t) {
			return nil, true
		}
		return t, false
	}
}

func joinNonNilParameterEvidence(a, b typ.Type) typ.Type {
	if upper, ok := selectParameterEvidenceTableUpperBound(a, b); ok {
		return upper
	}
	if preferred, ok := preferConcreteOverSoftType(a, b); ok {
		return preferred
	}
	if typeCanSelfEmbed(a) && typeContainsEquivalent(b, a) && !typ.IsAbsentOrUnknown(a) {
		if typeContainsUnion(a) {
			return a
		}
		return typ.JoinPreferNonSoft(a, b)
	}
	if typeCanSelfEmbed(b) && typeContainsEquivalent(a, b) && !typ.IsAbsentOrUnknown(b) {
		if typeContainsUnion(b) {
			return b
		}
		return typ.JoinPreferNonSoft(a, b)
	}
	if typeIsTruthyRefinement(a, b) {
		return a
	}
	if typeIsTruthyRefinement(b, a) {
		return b
	}
	if joined, ok := typ.JoinCompatibleRecords(a, b); ok {
		return joined
	}
	if joined, ok := joinParameterEvidenceMapRecord(a, b); ok {
		return joined
	}
	if TypeExtendsRecord(a, b) {
		return a
	}
	if TypeExtendsRecord(b, a) {
		return b
	}
	if !typ.IsAbsentOrUnknown(a) && !typ.IsAbsentOrUnknown(b) {
		if subtype.IsSubtype(a, b) {
			return b
		}
		if subtype.IsSubtype(b, a) {
			return a
		}
	}
	return paramevidence.NormalizeType(typ.JoinPreferNonSoft(a, b))
}

func preferConcreteOverSoftType(a, b typ.Type) (typ.Type, bool) {
	aSoft := typ.IsSoft(a, typ.SoftPlaceholderPolicy)
	bSoft := typ.IsSoft(b, typ.SoftPlaceholderPolicy)
	switch {
	case aSoft && !bSoft && !unwrap.IsNilType(b):
		return b, true
	case bSoft && !aSoft && !unwrap.IsNilType(a):
		return a, true
	}
	if preferred, ok := preferConcreteOverNilableSoftType(a, b); ok {
		return preferred, true
	}
	return nil, false
}

func preferConcreteOverNilableSoftType(a, b typ.Type) (typ.Type, bool) {
	if preferred, ok := preferConcreteOverNilableSoftTypeDirected(a, b); ok {
		return preferred, true
	}
	return preferConcreteOverNilableSoftTypeDirected(b, a)
}

func preferConcreteOverNilableSoftTypeDirected(softMaybeNil, concrete typ.Type) (typ.Type, bool) {
	inner, nilable := splitNilableParameterEvidence(softMaybeNil)
	if !nilable || inner == nil || !typ.IsSoft(inner, typ.SoftPlaceholderPolicy) {
		return nil, false
	}
	if concrete == nil || unwrap.IsNilType(concrete) {
		return nil, false
	}
	concreteInner, concreteNilable := splitNilableParameterEvidence(concrete)
	if concreteInner == nil {
		return nil, false
	}
	if typ.IsSoft(concreteInner, typ.SoftPlaceholderPolicy) {
		return nil, false
	}
	if concreteNilable {
		return concrete, true
	}
	return typ.NewOptional(concrete), true
}

func joinParameterEvidenceMapRecord(a, b typ.Type) (typ.Type, bool) {
	if joined, ok := joinParameterEvidenceMapRecordDirected(a, b); ok {
		return joined, true
	}
	return joinParameterEvidenceMapRecordDirected(b, a)
}

func joinParameterEvidenceMapRecordDirected(mapType, recordType typ.Type) (typ.Type, bool) {
	m, ok := unwrap.Alias(mapType).(*typ.Map)
	if !ok || m == nil {
		return nil, false
	}
	r, ok := unwrap.Alias(recordType).(*typ.Record)
	if !ok || r == nil || !r.HasMapComponent() {
		return nil, false
	}

	key := joinNonNilParameterEvidence(m.Key, r.MapKey)
	value := joinNonNilParameterEvidence(m.Value, r.MapValue)
	if len(r.Fields) == 0 && r.Metatable == nil {
		return typ.NewMap(key, value), true
	}
	builder := typ.NewRecord()
	if r.Open {
		builder.SetOpen(true)
	}
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	builder.MapComponent(key, value)
	for _, field := range r.Fields {
		fieldType := field.Type
		optional := true
		if subtype.IsSubtype(typ.LiteralString(field.Name), key) {
			fieldType = joinNonNilParameterEvidence(field.Type, value)
		} else {
			optional = field.Optional
		}
		switch {
		case optional && field.Readonly:
			builder.OptReadonlyField(field.Name, fieldType)
		case optional:
			builder.OptField(field.Name, fieldType)
		case field.Readonly:
			builder.ReadonlyField(field.Name, fieldType)
		default:
			builder.Field(field.Name, fieldType)
		}
	}
	return builder.Build(), true
}

func selectParameterEvidenceTableUpperBound(a, b typ.Type) (typ.Type, bool) {
	if parameterEvidenceIsOnlyTableTop(a) && typ.IsAny(b) {
		return a, true
	}
	if parameterEvidenceIsOnlyTableTop(b) && typ.IsAny(a) {
		return b, true
	}
	if parameterEvidenceContainsTableTop(a) && parameterEvidenceCoveredByTableTop(b) && subtype.IsSubtype(b, a) {
		return a, true
	}
	if parameterEvidenceContainsTableTop(b) && parameterEvidenceCoveredByTableTop(a) && subtype.IsSubtype(a, b) {
		return b, true
	}
	return nil, false
}

func parameterEvidenceContainsTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return parameterEvidenceContainsTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return parameterEvidenceContainsTableTop(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if parameterEvidenceContainsTableTop(member) {
				return true
			}
		}
	}
	return false
}

func parameterEvidenceIsOnlyTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return parameterEvidenceIsOnlyTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return parameterEvidenceIsOnlyTableTop(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		hasTableTop := false
		for _, member := range v.Members {
			if unwrap.IsNilType(member) {
				continue
			}
			if !parameterEvidenceIsOnlyTableTop(member) {
				return false
			}
			hasTableTop = true
		}
		return hasTableTop
	default:
		return false
	}
}

func parameterEvidenceCoveredByTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) {
		return true
	}
	if unwrap.IsNilType(t) || unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return parameterEvidenceCoveredByTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return parameterEvidenceCoveredByTableTop(v.Inner)
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && parameterEvidenceCoveredByTableTop(v.Body)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !parameterEvidenceCoveredByTableTop(member) {
				return false
			}
		}
		return true
	case *typ.Record, *typ.Map, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Intersection:
		return true
	default:
		return false
	}
}

func typeIsTruthyRefinement(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	refined := narrow.ToTruthy(baseline)
	if refined == nil || refined.Kind().IsNever() || typ.TypeEquals(refined, baseline) {
		return false
	}
	return typ.TypeEquals(candidate, refined) || subtype.IsSubtype(candidate, refined)
}

func typeCanSelfEmbed(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Annotated:
		return typeCanSelfEmbed(v.Inner)
	case *typ.Alias:
		return typeCanSelfEmbed(v.Target)
	case *typ.Optional:
		return typeCanSelfEmbed(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanSelfEmbed(member) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range v.Members {
			if typeCanSelfEmbed(member) {
				return true
			}
		}
		return false
	case *typ.Array, *typ.Map, *typ.Tuple, *typ.Record, *typ.Function:
		return true
	default:
		return false
	}
}

func typeContainsEquivalent(haystack, needle typ.Type) bool {
	if haystack == nil || needle == nil {
		return false
	}
	return scanType(haystack, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if typ.TypeEquals(node, needle) {
			return true, false
		}
		return false, true
	})
}

func typeContainsUnion(t typ.Type) bool {
	if t == nil {
		return false
	}
	return scanType(t, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if _, ok := node.(*typ.Union); ok {
			return true, false
		}
		return false, true
	})
}

func canonicalInterprocValueType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return maybeWidenTypeForConvergence(fn)
	}
	return maybeWidenTypeForConvergence(t)
}

func mergeInterprocValueType(existing, candidate typ.Type) typ.Type {
	existing = canonicalInterprocValueType(existing)
	candidate = canonicalInterprocValueType(candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	if unwrap.Function(existing) != nil || unwrap.Function(candidate) != nil {
		return maybeWidenTypeForConvergence(widenFunctionFactTypeForConvergence(existing, candidate))
	}
	return maybeWidenTypeForConvergence(widenValueTypeForConvergence(existing, candidate))
}

func normalizeInterprocValueType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return fn
	}
	return typ.PruneSoftUnionMembers(t)
}

func joinInterprocValueType(existing, candidate typ.Type) typ.Type {
	existing = normalizeInterprocValueType(existing)
	candidate = normalizeInterprocValueType(candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	if unwrap.Function(existing) != nil || unwrap.Function(candidate) != nil {
		return MergeFunctionFactType(existing, candidate)
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

func widenValueTypeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = normalizeInterprocValueType(existing)
	candidate = normalizeInterprocValueType(candidate)
	if existing == nil {
		return maybeWidenTypeForConvergence(candidate)
	}
	if candidate == nil {
		return maybeWidenTypeForConvergence(existing)
	}
	existing = maybeWidenTypeForConvergence(existing)
	candidate = maybeWidenTypeForConvergence(candidate)
	if typ.TypeEquals(existing, candidate) {
		return existing
	}
	if unwrap.IsNilType(existing) && !unwrap.IsNilType(candidate) {
		return candidate
	}
	if unwrap.IsNilType(candidate) && !unwrap.IsNilType(existing) {
		return existing
	}
	if typ.IsAny(existing) || typ.IsUnknown(existing) {
		return existing
	}
	if typ.IsAny(candidate) || typ.IsUnknown(candidate) {
		return candidate
	}
	if typeElidesOptional(candidate, existing) {
		return candidate
	}
	if TypeExtendsRecord(candidate, existing) && !typeContainsNestedStructuralShape(candidate, existing) {
		return candidate
	}
	if refines, _ := typeRefinesFalsyMapKey(candidate, existing); refines {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) && !subtype.IsSubtype(existing, candidate) {
		return existing
	}
	if subtype.IsSubtype(existing, candidate) && !subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	return typ.JoinPreferNonSoft(existing, candidate)
}

func widenFunctionFactTypeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = normalizeInterprocValueType(existing)
	candidate = normalizeInterprocValueType(candidate)
	if existing == nil {
		return maybeWidenTypeForConvergence(candidate)
	}
	if candidate == nil {
		return maybeWidenTypeForConvergence(existing)
	}
	existingFn := unwrap.Function(existing)
	candidateFn := unwrap.Function(candidate)
	if existingFn != nil && candidateFn != nil && sameFunctionShapeForFactMerge(existingFn, candidateFn) {
		return maybeWidenTypeForConvergence(widenFunctionFactsByShape(existingFn, candidateFn))
	}
	return widenValueTypeForConvergence(existing, candidate)
}

func widenFunctionFactsByShape(existing, candidate *typ.Function) typ.Type {
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
		paramType := widenFunctionParamFactTypeForConvergence(p.Type, candidate.Params[i].Type)
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
		builder = builder.Variadic(widenFunctionParamFactTypeForConvergence(existing.Variadic, candidate.Variadic))
	}
	if returns := widenReturnSummaryForConvergence(existing.Returns, candidate.Returns); len(returns) > 0 {
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

func widenFunctionParamFactTypeForConvergence(existing, candidate typ.Type) typ.Type {
	existing = normalizeInterprocValueType(existing)
	candidate = normalizeInterprocValueType(candidate)
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
	if preferred, ok := preferConcreteOverSoftType(existing, candidate); ok {
		return preferred
	}
	if candidateRefinesFunctionParam(candidate, existing) {
		return candidate
	}
	if candidateRefinesFunctionParam(existing, candidate) {
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

// WidenLiteralSigs merges two literal signature maps.
func WidenLiteralSigs(prev, next api.LiteralSigs) api.LiteralSigs {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeLiteralSigs(next)
	}
	if next == nil {
		return normalizeLiteralSigs(prev)
	}
	merged := make(api.LiteralSigs, len(prev)+len(next))
	for fn, sig := range prev {
		merged[fn] = maybeWidenFunctionForConvergence(sig)
	}
	for fn, sig := range next {
		if existing := merged[fn]; existing != nil {
			merged[fn] = maybeWidenFunctionForConvergence(mergeLiteralSigForConvergence(existing, sig))
		} else {
			merged[fn] = maybeWidenFunctionForConvergence(sig)
		}
	}
	return merged
}

func normalizeLiteralSigs(sigs api.LiteralSigs) api.LiteralSigs {
	if sigs == nil {
		return nil
	}
	out := make(api.LiteralSigs, len(sigs))
	for fn, sig := range sigs {
		out[fn] = maybeWidenFunctionForConvergence(sig)
	}
	return out
}

// JoinLiteralSigs merges literal signatures precisely inside one iteration.
func JoinLiteralSigs(prev, next api.LiteralSigs) api.LiteralSigs {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeLiteralSigs(next)
	}
	if next == nil {
		return normalizeLiteralSigs(prev)
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

func mergeLiteralSigForConvergence(prev, next *typ.Function) *typ.Function {
	merged := widenFunctionFactTypeForConvergence(prev, next)
	if fn := unwrap.Function(merged); fn != nil {
		return fn
	}
	return mergeLiteralSig(prev, next)
}

// WidenCapturedTypes merges two captured type maps using monotone join.
func WidenCapturedTypes(prev, next api.CapturedTypes) api.CapturedTypes {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeCapturedTypes(next)
	}
	if next == nil {
		return normalizeCapturedTypes(prev)
	}
	merged := make(api.CapturedTypes, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = canonicalInterprocValueType(prev[sym])
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		t := next[sym]
		if existing := merged[sym]; existing != nil {
			merged[sym] = mergeInterprocValueType(existing, t)
		} else {
			merged[sym] = canonicalInterprocValueType(t)
		}
	}
	return merged
}

// JoinCapturedTypes merges captured types precisely inside one iteration.
func JoinCapturedTypes(prev, next api.CapturedTypes) api.CapturedTypes {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeCapturedTypesForJoin(next)
	}
	if next == nil {
		return normalizeCapturedTypesForJoin(prev)
	}
	merged := make(api.CapturedTypes, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = normalizeInterprocValueType(prev[sym])
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		t := next[sym]
		if existing := merged[sym]; existing != nil {
			merged[sym] = joinInterprocValueType(existing, t)
		} else {
			merged[sym] = normalizeInterprocValueType(t)
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
		return normalizeCapturedFieldAssigns(next)
	}
	if next == nil {
		return normalizeCapturedFieldAssigns(prev)
	}
	merged := make(api.CapturedFieldAssigns, len(prev)+len(next))
	for _, callee := range cfg.SortedSymbolIDs(prev) {
		merged[callee] = normalizeCapturedFieldSymbolMap(prev[callee])
	}
	for _, callee := range cfg.SortedSymbolIDs(next) {
		captured := next[callee]
		existing := merged[callee]
		if existing == nil {
			merged[callee] = normalizeCapturedFieldSymbolMap(captured)
			continue
		}
		merged[callee] = MergeCapturedFieldSymbolMaps(existing, captured, func(prev typ.Type, next typ.Type) typ.Type {
			if prev != nil {
				return mergeInterprocValueType(prev, next)
			}
			return canonicalInterprocValueType(next)
		})
	}
	return merged
}

func normalizeCapturedTypes(types api.CapturedTypes) api.CapturedTypes {
	if types == nil {
		return nil
	}
	out := make(api.CapturedTypes, len(types))
	for _, sym := range cfg.SortedSymbolIDs(types) {
		out[sym] = canonicalInterprocValueType(types[sym])
	}
	return out
}

func normalizeCapturedTypesForJoin(types api.CapturedTypes) api.CapturedTypes {
	if types == nil {
		return nil
	}
	out := make(api.CapturedTypes, len(types))
	for _, sym := range cfg.SortedSymbolIDs(types) {
		out[sym] = normalizeInterprocValueType(types[sym])
	}
	return out
}

func normalizeCapturedFieldAssigns(fields api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	if fields == nil {
		return nil
	}
	out := make(api.CapturedFieldAssigns, len(fields))
	for _, callee := range cfg.SortedSymbolIDs(fields) {
		out[callee] = normalizeCapturedFieldSymbolMap(fields[callee])
	}
	return out
}

func normalizeCapturedFieldSymbolMap(fieldsBySym map[cfg.SymbolID]map[string]typ.Type) map[cfg.SymbolID]map[string]typ.Type {
	if fieldsBySym == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]map[string]typ.Type, len(fieldsBySym))
	for _, sym := range cfg.SortedSymbolIDs(fieldsBySym) {
		fields := fieldsBySym[sym]
		fieldOut := make(map[string]typ.Type, len(fields))
		for _, name := range cfg.SortedFieldNames(fields) {
			fieldOut[name] = canonicalInterprocValueType(fields[name])
		}
		out[sym] = fieldOut
	}
	return out
}

// JoinCapturedFieldAssigns merges captured field assignments inside one iteration.
func JoinCapturedFieldAssigns(prev, next api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeCapturedFieldAssignsForJoin(next)
	}
	if next == nil {
		return normalizeCapturedFieldAssignsForJoin(prev)
	}
	merged := make(api.CapturedFieldAssigns, len(prev)+len(next))
	for _, callee := range cfg.SortedSymbolIDs(prev) {
		merged[callee] = normalizeCapturedFieldSymbolMapForJoin(prev[callee])
	}
	for _, callee := range cfg.SortedSymbolIDs(next) {
		captured := next[callee]
		existing := merged[callee]
		if existing == nil {
			merged[callee] = normalizeCapturedFieldSymbolMapForJoin(captured)
			continue
		}
		merged[callee] = MergeCapturedFieldSymbolMaps(existing, captured, func(prev typ.Type, next typ.Type) typ.Type {
			if prev != nil {
				return joinInterprocValueType(prev, next)
			}
			return normalizeInterprocValueType(next)
		})
	}
	return merged
}

func normalizeCapturedFieldAssignsForJoin(fields api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	if fields == nil {
		return nil
	}
	out := make(api.CapturedFieldAssigns, len(fields))
	for _, callee := range cfg.SortedSymbolIDs(fields) {
		out[callee] = normalizeCapturedFieldSymbolMapForJoin(fields[callee])
	}
	return out
}

func normalizeCapturedFieldSymbolMapForJoin(fieldsBySym map[cfg.SymbolID]map[string]typ.Type) map[cfg.SymbolID]map[string]typ.Type {
	if fieldsBySym == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]map[string]typ.Type, len(fieldsBySym))
	for _, sym := range cfg.SortedSymbolIDs(fieldsBySym) {
		fields := fieldsBySym[sym]
		fieldOut := make(map[string]typ.Type, len(fields))
		for _, name := range cfg.SortedFieldNames(fields) {
			fieldOut[name] = normalizeInterprocValueType(fields[name])
		}
		out[sym] = fieldOut
	}
	return out
}

// WidenCapturedContainerMutations merges captured container mutation maps using monotone union.
func WidenCapturedContainerMutations(prev, next api.CapturedContainerMutations) api.CapturedContainerMutations {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeCapturedContainerMutations(next)
	}
	if next == nil {
		return normalizeCapturedContainerMutations(prev)
	}
	merged := make(api.CapturedContainerMutations, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = normalizeCapturedContainerMutationMap(prev[sym])
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		muts := next[sym]
		existing := merged[sym]
		merged[sym] = MergeCapturedContainerMutationMaps(existing, muts, func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation {
			if prev != nil {
				next.ValueType = mergeInterprocValueType(prev.ValueType, next.ValueType)
			} else {
				next.ValueType = maybeWidenTypeForConvergence(next.ValueType)
			}
			return next
		})
	}
	return merged
}

func normalizeCapturedContainerMutations(muts api.CapturedContainerMutations) api.CapturedContainerMutations {
	if muts == nil {
		return nil
	}
	out := make(api.CapturedContainerMutations, len(muts))
	for _, sym := range cfg.SortedSymbolIDs(muts) {
		out[sym] = normalizeCapturedContainerMutationMap(muts[sym])
	}
	return out
}

func normalizeCapturedContainerMutationMap(muts map[cfg.SymbolID][]api.ContainerMutation) map[cfg.SymbolID][]api.ContainerMutation {
	if muts == nil {
		return nil
	}
	out := make(map[cfg.SymbolID][]api.ContainerMutation, len(muts))
	for _, sym := range cfg.SortedSymbolIDs(muts) {
		entries := muts[sym]
		if len(entries) == 0 {
			continue
		}
		normalized := make([]api.ContainerMutation, len(entries))
		for i, mut := range entries {
			normalized[i] = mut
			normalized[i].ValueType = canonicalInterprocValueType(mut.ValueType)
		}
		out[sym] = normalized
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// JoinCapturedContainerMutations merges captured container mutations inside one iteration.
func JoinCapturedContainerMutations(prev, next api.CapturedContainerMutations) api.CapturedContainerMutations {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeCapturedContainerMutationsForJoin(next)
	}
	if next == nil {
		return normalizeCapturedContainerMutationsForJoin(prev)
	}
	merged := make(api.CapturedContainerMutations, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = normalizeCapturedContainerMutationMapForJoin(prev[sym])
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		muts := next[sym]
		existing := merged[sym]
		merged[sym] = MergeCapturedContainerMutationMaps(existing, muts, func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation {
			if prev != nil {
				next.ValueType = joinInterprocValueType(prev.ValueType, next.ValueType)
			} else {
				next.ValueType = normalizeInterprocValueType(next.ValueType)
			}
			return next
		})
	}
	return merged
}

func normalizeCapturedContainerMutationsForJoin(muts api.CapturedContainerMutations) api.CapturedContainerMutations {
	if muts == nil {
		return nil
	}
	out := make(api.CapturedContainerMutations, len(muts))
	for _, sym := range cfg.SortedSymbolIDs(muts) {
		out[sym] = normalizeCapturedContainerMutationMapForJoin(muts[sym])
	}
	return out
}

func normalizeCapturedContainerMutationMapForJoin(muts map[cfg.SymbolID][]api.ContainerMutation) map[cfg.SymbolID][]api.ContainerMutation {
	if muts == nil {
		return nil
	}
	out := make(map[cfg.SymbolID][]api.ContainerMutation, len(muts))
	for _, sym := range cfg.SortedSymbolIDs(muts) {
		entries := muts[sym]
		if len(entries) == 0 {
			continue
		}
		normalized := make([]api.ContainerMutation, len(entries))
		for i, mut := range entries {
			normalized[i] = mut
			normalized[i].ValueType = normalizeInterprocValueType(mut.ValueType)
		}
		out[sym] = normalized
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// WidenConstructorFields merges constructor field maps using monotone join.
func WidenConstructorFields(prev, next api.ConstructorFields) api.ConstructorFields {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeConstructorFields(next)
	}
	if next == nil {
		return normalizeConstructorFields(prev)
	}
	merged := make(api.ConstructorFields, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = normalizeConstructorFieldMap(prev[sym])
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
				out[name] = mergeInterprocValueType(prevType, t)
			} else {
				out[name] = maybeWidenTypeForConvergence(t)
			}
		}
		merged[sym] = out
	}
	return merged
}

func normalizeConstructorFields(fields api.ConstructorFields) api.ConstructorFields {
	if fields == nil {
		return nil
	}
	out := make(api.ConstructorFields, len(fields))
	for _, sym := range cfg.SortedSymbolIDs(fields) {
		out[sym] = normalizeConstructorFieldMap(fields[sym])
	}
	return out
}

func normalizeConstructorFieldMap(fields map[string]typ.Type) map[string]typ.Type {
	if fields == nil {
		return nil
	}
	out := make(map[string]typ.Type, len(fields))
	for _, name := range cfg.SortedFieldNames(fields) {
		out[name] = canonicalInterprocValueType(fields[name])
	}
	return out
}

// JoinConstructorFields merges constructor field maps inside one iteration.
func JoinConstructorFields(prev, next api.ConstructorFields) api.ConstructorFields {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeConstructorFieldsForJoin(next)
	}
	if next == nil {
		return normalizeConstructorFieldsForJoin(prev)
	}
	merged := make(api.ConstructorFields, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = normalizeConstructorFieldMapForJoin(prev[sym])
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		fields := next[sym]
		existing := merged[sym]
		if existing == nil {
			merged[sym] = normalizeConstructorFieldMapForJoin(fields)
			continue
		}
		out := make(map[string]typ.Type, len(existing)+len(fields))
		for _, name := range cfg.SortedFieldNames(existing) {
			out[name] = existing[name]
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			t := fields[name]
			if prevType := out[name]; prevType != nil {
				out[name] = joinInterprocValueType(prevType, t)
			} else {
				out[name] = normalizeInterprocValueType(t)
			}
		}
		merged[sym] = out
	}
	return merged
}

func normalizeConstructorFieldsForJoin(fields api.ConstructorFields) api.ConstructorFields {
	if fields == nil {
		return nil
	}
	out := make(api.ConstructorFields, len(fields))
	for _, sym := range cfg.SortedSymbolIDs(fields) {
		out[sym] = normalizeConstructorFieldMapForJoin(fields[sym])
	}
	return out
}

func normalizeConstructorFieldMapForJoin(fields map[string]typ.Type) map[string]typ.Type {
	if fields == nil {
		return nil
	}
	out := make(map[string]typ.Type, len(fields))
	for _, name := range cfg.SortedFieldNames(fields) {
		out[name] = normalizeInterprocValueType(fields[name])
	}
	return out
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
	return subtype.WidenForInference(t)
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
