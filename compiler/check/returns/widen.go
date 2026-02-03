package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// WidenFacts merges two interproc fact bundles.
func WidenFacts(prev, next api.Facts) api.Facts {
	mergedReturns := WidenReturnSummaries(prev.ReturnSummaries, next.ReturnSummaries)
	mergedReturns = refineReturnSummariesWithNarrow(mergedReturns, prev.NarrowReturns)
	mergedReturns = refineReturnSummariesWithNarrow(mergedReturns, next.NarrowReturns)
	return api.Facts{
		ReturnSummaries:   mergedReturns,
		NarrowReturns:     WidenReturnSummaries(prev.NarrowReturns, next.NarrowReturns),
		ParamHints:        WidenParamHints(prev.ParamHints, next.ParamHints),
		FuncTypes:         WidenFuncTypes(prev.FuncTypes, next.FuncTypes),
		LiteralSigs:       WidenLiteralSigs(prev.LiteralSigs, next.LiteralSigs),
		CapturedTypes:     WidenCapturedTypes(prev.CapturedTypes, next.CapturedTypes),
		CapturedFields:    WidenCapturedFieldAssigns(prev.CapturedFields, next.CapturedFields),
		CapturedContainers: WidenCapturedContainerMutations(prev.CapturedContainers, next.CapturedContainers),
		ConstructorFields: WidenConstructorFields(prev.ConstructorFields, next.ConstructorFields),
	}
}

func refineReturnSummariesWithNarrow(summaries, narrow api.ReturnSummaries) api.ReturnSummaries {
	if len(summaries) == 0 || len(narrow) == 0 {
		return summaries
	}
	for _, sym := range cfg.SortedSymbolIDs(narrow) {
		rets := narrow[sym]
		if len(rets) == 0 {
			continue
		}
		existing := summaries[sym]
		if len(existing) == 0 {
			continue
		}
		if ReturnTypesElideOptional(rets, existing) || ReturnTypesExtendRecord(rets, existing) {
			summaries[sym] = rets
		}
	}
	return summaries
}

// WidenReturnSummaries merges two return summary maps using monotone union.
// Types can only grow (become more general), never shrink.
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
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		rets := next[sym]
		if existing := merged[sym]; existing != nil {
			if ReturnTypesRefine(rets, existing) ||
				ReturnTypesExtendRecord(rets, existing) ||
				ReturnTypesElideOptional(rets, existing) {
				merged[sym] = rets
			} else if ReturnTypesRefine(existing, rets) ||
				ReturnTypesExtendRecord(existing, rets) ||
				ReturnTypesElideOptional(existing, rets) {
				merged[sym] = existing
			} else {
				merged[sym] = JoinReturnVectorsPreferNonSoft(existing, rets)
			}
		} else {
			merged[sym] = rets
		}
	}
	return merged
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

// WidenFuncTypes merges two function-type maps using monotone union.
// Prefers refined return types when possible.
func WidenFuncTypes(prev, next api.FuncTypes) api.FuncTypes {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	merged := make(api.FuncTypes, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = prev[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		t := next[sym]
		if existing := merged[sym]; existing != nil {
			merged[sym] = mergeFuncTypes(existing, t)
		} else {
			merged[sym] = t
		}
	}
	return merged
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
		result[i] = joinParamHint(ai, bi)
	}
	return result
}

func joinParamHint(a, b typ.Type) typ.Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if isNilType(a) && !isNilType(b) {
		return b
	}
	if isNilType(b) && !isNilType(a) {
		return a
	}
	if TypeExtendsRecord(a, b) {
		return a
	}
	if TypeExtendsRecord(b, a) {
		return b
	}
	return typ.JoinPreferNonSoft(a, b)
}

func isNilType(t typ.Type) bool {
	return t != nil && t.Kind() == kind.Nil
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
		merged[fn] = sig
	}
	for fn, sig := range next {
		if existing := merged[fn]; existing != nil {
			if sig != nil {
				merged[fn] = sig
			}
		} else {
			merged[fn] = sig
		}
	}
	return merged
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
			merged[sym] = typ.JoinPreferNonSoft(existing, t)
		} else {
			merged[sym] = t
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
		out := make(map[cfg.SymbolID]map[string]typ.Type, len(existing)+len(captured))
		for _, sym := range cfg.SortedSymbolIDs(existing) {
			out[sym] = existing[sym]
		}
		for _, sym := range cfg.SortedSymbolIDs(captured) {
			fields := captured[sym]
			existingFields := out[sym]
			if existingFields == nil {
				out[sym] = fields
				continue
			}
			mergedFields := make(map[string]typ.Type, len(existingFields)+len(fields))
			for _, name := range cfg.SortedFieldNames(existingFields) {
				mergedFields[name] = existingFields[name]
			}
			for _, name := range cfg.SortedFieldNames(fields) {
				t := fields[name]
				if prevType := mergedFields[name]; prevType != nil {
					mergedFields[name] = typ.JoinPreferNonSoft(prevType, t)
				} else {
					mergedFields[name] = t
				}
			}
			out[sym] = mergedFields
		}
		merged[callee] = out
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
		merged[sym] = mergeCapturedContainerMutations(existing, muts)
	}
	return merged
}

func mergeCapturedContainerMutations(
	existing map[cfg.SymbolID][]api.ContainerMutation,
	next map[cfg.SymbolID][]api.ContainerMutation,
) map[cfg.SymbolID][]api.ContainerMutation {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	merged := make(map[cfg.SymbolID][]api.ContainerMutation, len(existing)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(existing) {
		merged[sym] = existing[sym]
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		merged[sym] = mergeContainerMutationSlice(merged[sym], next[sym])
	}
	return merged
}

func mergeContainerMutationSlice(
	existing []api.ContainerMutation,
	next []api.ContainerMutation,
) []api.ContainerMutation {
	if len(existing) == 0 {
		return next
	}
	if len(next) == 0 {
		return existing
	}
	byKey := make(map[string]api.ContainerMutation, len(existing)+len(next))
	for _, m := range existing {
		key := mutationKey(m)
		byKey[key] = m
	}
	for _, m := range next {
		key := mutationKey(m)
		if prev, ok := byKey[key]; ok {
			m.ValueType = join.Two(prev.ValueType, m.ValueType)
		}
		byKey[key] = m
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]api.ContainerMutation, 0, len(byKey))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func mutationKey(m api.ContainerMutation) string {
	return constraint.FormatSegments(m.Segments)
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
				out[name] = join.Two(prevType, t)
			} else {
				out[name] = t
			}
		}
		merged[sym] = out
	}
	return merged
}

func mergeFuncTypes(prev, next typ.Type) typ.Type {
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	prevFn, okPrev := prev.(*typ.Function)
	nextFn, okNext := next.(*typ.Function)
	if okPrev && okNext {
		if len(prevFn.Returns) > 0 && len(nextFn.Returns) > 0 {
			if ReturnTypesRefine(prevFn.Returns, nextFn.Returns) {
				return prev
			}
			if ReturnTypesRefine(nextFn.Returns, prevFn.Returns) {
				return next
			}
		}
		if len(prevFn.Returns) > 0 && len(nextFn.Returns) == 0 {
			return prev
		}
		if len(nextFn.Returns) > 0 && len(prevFn.Returns) == 0 {
			return next
		}
	}
	return join.Two(prev, next)
}
