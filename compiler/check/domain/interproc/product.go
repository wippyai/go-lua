package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WidenFacts merges two interproc fact bundles.
func WidenFacts(prev, next api.Facts) api.Facts {
	if FactsEqual(prev, next) {
		return prev
	}
	widening := value.NewConvergenceWidening()
	out := api.Facts{
		LiteralSigs:       widenLiteralSigsWith(widening, prev.LiteralSigs, next.LiteralSigs),
		CapturedTypes:     widenCapturedTypesWith(widening, prev.CapturedTypes, next.CapturedTypes),
		CapturedFields:    widenCapturedFieldAssignsWith(widening, prev.CapturedFields, next.CapturedFields),
		ConstructorFields: widenConstructorFieldsWith(widening, prev.ConstructorFields, next.ConstructorFields),
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
	widening := value.NewConvergenceWidening()
	out := api.Facts{
		LiteralSigs:       joinLiteralSigsWith(widening, prev.LiteralSigs, next.LiteralSigs),
		CapturedTypes:     JoinCapturedTypes(prev.CapturedTypes, next.CapturedTypes),
		CapturedFields:    JoinCapturedFieldAssigns(prev.CapturedFields, next.CapturedFields),
		ConstructorFields: JoinConstructorFields(prev.ConstructorFields, next.ConstructorFields),
	}

	symbols := collectCanonicalFunctionFactSymbols(prev.FunctionFacts, next.FunctionFacts)
	if len(symbols) > 0 {
		out.FunctionFacts = make(api.FunctionFacts, len(symbols))
	}
	for _, sym := range symbols {
		prevFact := readFunctionFactFromFacts(&prev, sym)
		nextFact := readFunctionFactFromFacts(&next, sym)
		writeNormalizedFunctionFactToFacts(&out, sym, functionfact.JoinCanonical(prevFact, nextFact))
	}
	return out
}

func canonicalInterprocValueType(t typ.Type) typ.Type {
	return canonicalInterprocValueTypeWith(value.NewConvergenceWidening(), t)
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

func mergeInterprocValueType(existing, candidate typ.Type) typ.Type {
	return mergeInterprocValueTypeWith(value.NewConvergenceWidening(), existing, candidate)
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

// WidenLiteralSigs merges two literal signature maps.
func WidenLiteralSigs(prev, next api.LiteralSigs) api.LiteralSigs {
	return widenLiteralSigsWith(value.NewConvergenceWidening(), prev, next)
}

func widenLiteralSigsWith(widening *value.ConvergenceWidening, prev, next api.LiteralSigs) api.LiteralSigs {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeLiteralSigsWith(widening, next)
	}
	if next == nil {
		return normalizeLiteralSigsWith(widening, prev)
	}
	merged := make(api.LiteralSigs, len(prev)+len(next))
	for fn, sig := range prev {
		merged[fn] = sig
	}
	for fn, sig := range next {
		if existing := merged[fn]; existing != nil {
			mergedSig := mergeLiteralSigForConvergence(existing, sig)
			if typ.TypeEquals(existing, mergedSig) {
				merged[fn] = existing
			} else {
				merged[fn] = widening.Function(mergedSig)
			}
		} else {
			merged[fn] = widening.Function(sig)
		}
	}
	return merged
}

func normalizeLiteralSigs(sigs api.LiteralSigs) api.LiteralSigs {
	return normalizeLiteralSigsWith(value.NewConvergenceWidening(), sigs)
}

func normalizeLiteralSigsWith(widening *value.ConvergenceWidening, sigs api.LiteralSigs) api.LiteralSigs {
	if sigs == nil {
		return nil
	}
	out := make(api.LiteralSigs, len(sigs))
	for fn, sig := range sigs {
		out[fn] = widening.Function(sig)
	}
	return out
}

// JoinLiteralSigs merges literal signatures precisely inside one iteration.
func JoinLiteralSigs(prev, next api.LiteralSigs) api.LiteralSigs {
	return joinLiteralSigsWith(value.NewConvergenceWidening(), prev, next)
}

func joinLiteralSigsWith(widening *value.ConvergenceWidening, prev, next api.LiteralSigs) api.LiteralSigs {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return normalizeLiteralSigsWith(widening, next)
	}
	if next == nil {
		return normalizeLiteralSigsWith(widening, prev)
	}
	merged := make(api.LiteralSigs, len(prev)+len(next))
	for fn, sig := range prev {
		merged[fn] = sig
	}
	for fn, sig := range next {
		if existing := merged[fn]; existing != nil {
			mergedSig := mergeLiteralSig(existing, sig)
			if typ.TypeEquals(existing, mergedSig) {
				merged[fn] = existing
			} else {
				merged[fn] = widening.Function(mergedSig)
			}
		} else {
			merged[fn] = widening.Function(sig)
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

func mergeCapturedType(existing, candidate typ.Type) typ.Type {
	return mergeCapturedTypeWith(value.NewConvergenceWidening(), existing, candidate)
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

func normalizeCapturedTypes(types api.CapturedTypes) api.CapturedTypes {
	return normalizeCapturedTypesWith(value.NewConvergenceWidening(), types)
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

func normalizeCapturedFieldAssigns(fields api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	return normalizeCapturedFieldAssignsWith(value.NewConvergenceWidening(), fields)
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

func normalizeCapturedFieldSymbolMap(fieldsBySym map[cfg.SymbolID]FieldValues) map[cfg.SymbolID]FieldValues {
	return normalizeCapturedFieldSymbolMapWith(value.NewConvergenceWidening(), fieldsBySym)
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

func normalizeConstructorFields(fields api.ConstructorFields) api.ConstructorFields {
	return normalizeConstructorFieldsWith(value.NewConvergenceWidening(), fields)
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

func normalizeConstructorFieldMap(fields FieldValues) FieldValues {
	return normalizeConstructorFieldMapWith(value.NewConvergenceWidening(), fields)
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
