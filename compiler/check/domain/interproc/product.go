package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WidenFunctionFacts merges projected function facts at an iteration boundary.
func WidenFunctionFacts(prev, next api.FunctionFacts) api.FunctionFacts {
	if FunctionFactsEqual(prev, next) {
		return prev
	}
	symbols := collectCanonicalFunctionFactSymbols(prev, next)
	if len(symbols) == 0 {
		return nil
	}

	out := make(api.FunctionFacts, len(symbols))
	for _, sym := range symbols {
		prevFact := readFunctionFact(prev, sym)
		nextFact := readFunctionFact(next, sym)
		writeNormalizedFunctionFact(out, sym, functionfact.WidenForConvergence(prevFact, nextFact))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// JoinFunctionFacts performs a precise same-iteration merge of projected
// function facts. Recursive fixpoint boundaries must use WidenFunctionFacts.
func JoinFunctionFacts(prev, next api.FunctionFacts) api.FunctionFacts {
	symbols := collectCanonicalFunctionFactSymbols(prev, next)
	if len(symbols) == 0 {
		return nil
	}
	out := make(api.FunctionFacts, len(symbols))
	for _, sym := range symbols {
		prevFact := readFunctionFact(prev, sym)
		nextFact := readFunctionFact(next, sym)
		writeNormalizedFunctionFact(out, sym, functionfact.JoinCanonical(prevFact, nextFact))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func canonicalInterprocValueTypeWith(widening *value.ConvergenceWidening, t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return widening.Function(fn)
	}
	return widening.Type(t)
}

func mergeInterprocValueTypeWith(widening *value.ConvergenceWidening, existing, candidate typ.Type) typ.Type {
	existing = canonicalInterprocValueTypeWith(widening, existing)
	candidate = canonicalInterprocValueTypeWith(widening, candidate)
	if existing == nil {
		return candidate
	}
	if candidate == nil {
		return existing
	}
	if unwrap.Function(existing) != nil || unwrap.Function(candidate) != nil {
		return widening.Type(functionfact.WidenTypeForConvergence(existing, candidate))
	}
	return widening.Merge(existing, candidate)
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

// WidenCapturedTypes merges two captured type maps using monotone join.
func WidenCapturedTypes(prev, next api.CapturedTypes) api.CapturedTypes {
	return widenCapturedTypesWith(value.NewConvergenceWidening(), prev, next)
}

func widenCapturedTypesWith(widening *value.ConvergenceWidening, prev, next api.CapturedTypes) api.CapturedTypes {
	if prev == nil && next == nil {
		return nil
	}
	if symbolTypeMapEqual(prev, next) {
		return prev
	}
	if prev == nil {
		return normalizeCapturedTypesWith(widening, next)
	}
	if next == nil {
		return normalizeCapturedTypesWith(widening, prev)
	}
	merged := make(api.CapturedTypes, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = liftCarrier(canonicalInterprocValueTypeWith(widening, projectCarrier(prev[sym])))
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		t := projectCarrier(next[sym])
		if existing, ok := merged[sym]; ok && !existing.IsZero() {
			merged[sym] = liftCarrier(mergeCapturedTypeWith(widening, existing.ProjectValue(), t))
		} else {
			merged[sym] = liftCarrier(canonicalInterprocValueTypeWith(widening, t))
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
		merged[sym] = liftCarrier(normalizeInterprocValueType(projectCarrier(prev[sym])))
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		t := projectCarrier(next[sym])
		if existing, ok := merged[sym]; ok && !existing.IsZero() {
			merged[sym] = liftCarrier(joinCapturedType(existing.ProjectValue(), t))
		} else {
			merged[sym] = liftCarrier(normalizeInterprocValueType(t))
		}
	}
	return merged
}

func mergeCapturedTypeWith(widening *value.ConvergenceWidening, existing, candidate typ.Type) typ.Type {
	existing = canonicalInterprocValueTypeWith(widening, existing)
	candidate = canonicalInterprocValueTypeWith(widening, candidate)
	if replacement, ok := capturedPrecisionReplacement(existing, candidate); ok {
		return widening.Type(replacement)
	}
	return mergeInterprocValueTypeWith(widening, existing, candidate)
}

func joinCapturedType(existing, candidate typ.Type) typ.Type {
	existing = normalizeInterprocValueType(existing)
	candidate = normalizeInterprocValueType(candidate)
	if replacement, ok := capturedPrecisionReplacement(existing, candidate); ok {
		return normalizeInterprocValueType(replacement)
	}
	return joinInterprocValueType(existing, candidate)
}

func capturedPrecisionReplacement(existing, candidate typ.Type) (typ.Type, bool) {
	switch {
	case capturedTopSeed(existing) && capturedConcreteSnapshot(candidate):
		return candidate, true
	case capturedTopSeed(candidate) && capturedConcreteSnapshot(existing):
		return existing, true
	default:
		return nil, false
	}
}

func capturedTopSeed(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) || t.Kind().IsPlaceholder() {
		return true
	}
	inner := unwrap.Optional(t)
	return inner != t && (typ.IsAny(inner) || typ.IsUnknown(inner) || inner.Kind().IsPlaceholder())
}

func capturedConcreteSnapshot(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) || t.Kind().IsPlaceholder() {
		return false
	}
	inner := unwrap.Optional(t)
	return inner == t || (!typ.IsAny(inner) && !typ.IsUnknown(inner) && !inner.Kind().IsPlaceholder())
}

// WidenCapturedFieldAssigns merges captured field assignment maps using monotone union.
func WidenCapturedFieldAssigns(prev, next api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	return widenCapturedFieldAssignsWith(value.NewConvergenceWidening(), prev, next)
}

func widenCapturedFieldAssignsWith(widening *value.ConvergenceWidening, prev, next api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeCapturedFieldAssignsWith(widening, next)
	}
	if next == nil {
		return normalizeCapturedFieldAssignsWith(widening, prev)
	}
	merged := make(api.CapturedFieldAssigns, len(prev)+len(next))
	for _, callee := range cfg.SortedSymbolIDs(prev) {
		merged[callee] = normalizeCapturedFieldSymbolMapWith(widening, prev[callee])
	}
	for _, callee := range cfg.SortedSymbolIDs(next) {
		captured := next[callee]
		existing := merged[callee]
		if existing == nil {
			merged[callee] = normalizeCapturedFieldSymbolMapWith(widening, captured)
			continue
		}
		merged[callee] = MergeCapturedFieldSymbolMaps(existing, captured, func(prev typ.Type, next typ.Type) typ.Type {
			if prev != nil {
				return mergeInterprocValueTypeWith(widening, prev, next)
			}
			return canonicalInterprocValueTypeWith(widening, next)
		})
	}
	return merged
}

func normalizeCapturedTypesWith(widening *value.ConvergenceWidening, types api.CapturedTypes) api.CapturedTypes {
	if types == nil {
		return nil
	}
	out := make(api.CapturedTypes, len(types))
	for _, sym := range cfg.SortedSymbolIDs(types) {
		out[sym] = liftCarrier(canonicalInterprocValueTypeWith(widening, projectCarrier(types[sym])))
	}
	return out
}

func normalizeCapturedTypesForJoin(types api.CapturedTypes) api.CapturedTypes {
	if types == nil {
		return nil
	}
	out := make(api.CapturedTypes, len(types))
	for _, sym := range cfg.SortedSymbolIDs(types) {
		out[sym] = liftCarrier(normalizeInterprocValueType(projectCarrier(types[sym])))
	}
	return out
}

func normalizeCapturedFieldAssignsWith(widening *value.ConvergenceWidening, fields api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	if fields == nil {
		return nil
	}
	out := make(api.CapturedFieldAssigns, len(fields))
	for _, callee := range cfg.SortedSymbolIDs(fields) {
		out[callee] = normalizeCapturedFieldSymbolMapWith(widening, fields[callee])
	}
	return out
}

func normalizeCapturedFieldSymbolMapWith(widening *value.ConvergenceWidening, fieldsBySym map[cfg.SymbolID]FieldValues) map[cfg.SymbolID]FieldValues {
	if fieldsBySym == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]FieldValues, len(fieldsBySym))
	for _, sym := range cfg.SortedSymbolIDs(fieldsBySym) {
		fields := fieldsBySym[sym]
		fieldOut := make(FieldValues, len(fields))
		for _, key := range SortedFieldKeys(fields) {
			fieldOut[key] = liftCarrier(canonicalInterprocValueTypeWith(widening, projectCarrier(fields[key])))
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

func normalizeCapturedFieldSymbolMapForJoin(fieldsBySym map[cfg.SymbolID]FieldValues) map[cfg.SymbolID]FieldValues {
	if fieldsBySym == nil {
		return nil
	}
	out := make(map[cfg.SymbolID]FieldValues, len(fieldsBySym))
	for _, sym := range cfg.SortedSymbolIDs(fieldsBySym) {
		fields := fieldsBySym[sym]
		fieldOut := make(FieldValues, len(fields))
		for _, key := range SortedFieldKeys(fields) {
			fieldOut[key] = liftCarrier(normalizeInterprocValueType(projectCarrier(fields[key])))
		}
		out[sym] = fieldOut
	}
	return out
}

// WidenConstructorFields merges constructor field maps using monotone join.
func WidenConstructorFields(prev, next api.ConstructorFields) api.ConstructorFields {
	return widenConstructorFieldsWith(value.NewConvergenceWidening(), prev, next)
}

func widenConstructorFieldsWith(widening *value.ConvergenceWidening, prev, next api.ConstructorFields) api.ConstructorFields {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeConstructorFieldsWith(widening, next)
	}
	if next == nil {
		return normalizeConstructorFieldsWith(widening, prev)
	}
	merged := make(api.ConstructorFields, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		merged[sym] = normalizeConstructorFieldMapWith(widening, prev[sym])
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		fields := next[sym]
		existing := merged[sym]
		if existing == nil {
			merged[sym] = normalizeConstructorFieldMapWith(widening, fields)
			continue
		}
		out := make(FieldValues, len(existing)+len(fields))
		for _, key := range SortedFieldKeys(existing) {
			out[key] = existing[key]
		}
		for _, key := range SortedFieldKeys(fields) {
			t := projectCarrier(fields[key])
			if prevType := projectCarrier(out[key]); prevType != nil {
				out[key] = liftCarrier(mergeInterprocValueTypeWith(widening, prevType, t))
			} else {
				out[key] = liftCarrier(widening.Type(t))
			}
		}
		merged[sym] = out
	}
	return merged
}

func normalizeConstructorFieldsWith(widening *value.ConvergenceWidening, fields api.ConstructorFields) api.ConstructorFields {
	if fields == nil {
		return nil
	}
	out := make(api.ConstructorFields, len(fields))
	for _, sym := range cfg.SortedSymbolIDs(fields) {
		out[sym] = normalizeConstructorFieldMapWith(widening, fields[sym])
	}
	return out
}

func normalizeConstructorFieldMapWith(widening *value.ConvergenceWidening, fields FieldValues) FieldValues {
	if fields == nil {
		return nil
	}
	out := make(FieldValues, len(fields))
	for _, key := range SortedFieldKeys(fields) {
		out[key] = liftCarrier(canonicalInterprocValueTypeWith(widening, projectCarrier(fields[key])))
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
		out := make(FieldValues, len(existing)+len(fields))
		for _, key := range SortedFieldKeys(existing) {
			out[key] = existing[key]
		}
		for _, key := range SortedFieldKeys(fields) {
			t := projectCarrier(fields[key])
			if prevType := projectCarrier(out[key]); prevType != nil {
				out[key] = liftCarrier(joinInterprocValueType(prevType, t))
			} else {
				out[key] = liftCarrier(normalizeInterprocValueType(t))
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

func normalizeConstructorFieldMapForJoin(fields FieldValues) FieldValues {
	if fields == nil {
		return nil
	}
	out := make(FieldValues, len(fields))
	for _, key := range SortedFieldKeys(fields) {
		out[key] = liftCarrier(normalizeInterprocValueType(projectCarrier(fields[key])))
	}
	return out
}
