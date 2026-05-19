package factproduct

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/value"
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
		writeNormalizedFunctionFactToFacts(&out, sym, functionfact.WidenForConvergence(prevFact, nextFact))
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
		writeNormalizedFunctionFactToFacts(&out, sym, functionfact.Join(prevFact, nextFact))
	}
	return out
}

func canonicalInterprocValueType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return value.WidenForConvergence(fn)
	}
	return value.WidenForConvergence(t)
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
		return value.WidenForConvergence(functionfact.WidenTypeForConvergence(existing, candidate))
	}
	return value.WidenForConvergence(value.MergeForConvergence(existing, candidate))
}

func normalizeInterprocValueType(t typ.Type) typ.Type {
	return value.NormalizeFactType(t)
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
		return functionfact.MergeType(existing, candidate)
	}
	return value.JoinPrecise(existing, candidate)
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
		merged[fn] = value.WidenFunctionForConvergence(sig)
	}
	for fn, sig := range next {
		if existing := merged[fn]; existing != nil {
			merged[fn] = value.WidenFunctionForConvergence(mergeLiteralSigForConvergence(existing, sig))
		} else {
			merged[fn] = value.WidenFunctionForConvergence(sig)
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
		out[fn] = value.WidenFunctionForConvergence(sig)
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
		merged[fn] = value.WidenFunctionForConvergence(sig)
	}
	for fn, sig := range next {
		if existing := merged[fn]; existing != nil {
			merged[fn] = value.WidenFunctionForConvergence(mergeLiteralSig(existing, sig))
		} else {
			merged[fn] = value.WidenFunctionForConvergence(sig)
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
	if merged, ok := functionfact.MergeReturnsForSameSignature(prev, next); ok {
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
	merged := functionfact.WidenTypeForConvergence(prev, next)
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
				next.ValueType = widenContainerMutationValueType(prev.ValueType, next.ValueType)
			} else {
				next.ValueType = value.WidenForConvergence(next.ValueType)
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
		normalized := MergeContainerMutationSlices(nil, entries, func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation {
			if prev != nil {
				next.ValueType = widenContainerMutationValueType(prev.ValueType, next.ValueType)
			} else {
				next.ValueType = value.WidenForConvergence(next.ValueType)
			}
			return next
		})
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
				next.ValueType = joinContainerMutationValueType(prev.ValueType, next.ValueType)
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
		normalized := MergeContainerMutationSlices(nil, entries, func(prev *api.ContainerMutation, next api.ContainerMutation) api.ContainerMutation {
			if prev != nil {
				next.ValueType = joinContainerMutationValueType(prev.ValueType, next.ValueType)
			} else {
				next.ValueType = normalizeInterprocValueType(next.ValueType)
			}
			return next
		})
		out[sym] = normalized
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func widenContainerMutationValueType(prev, next typ.Type) typ.Type {
	prev = canonicalInterprocValueType(prev)
	next = canonicalInterprocValueType(next)
	if prev == nil {
		return value.WidenForConvergence(next)
	}
	if next == nil {
		return value.WidenForConvergence(prev)
	}
	if typ.TypeEquals(prev, next) {
		return prev
	}
	return value.WidenForConvergence(typ.JoinReturnSlot(prev, next))
}

func joinContainerMutationValueType(prev, next typ.Type) typ.Type {
	prev = normalizeInterprocValueType(prev)
	next = normalizeInterprocValueType(next)
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	if typ.TypeEquals(prev, next) {
		return prev
	}
	return normalizeInterprocValueType(typ.JoinReturnSlot(prev, next))
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
				out[name] = value.WidenForConvergence(t)
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
