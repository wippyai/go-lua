package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WidenMap merges two parameter evidence maps with the same vector law used by
// canonical FunctionFacts.
func WidenMap(prev, next map[cfg.SymbolID][]typ.Type) map[cfg.SymbolID][]typ.Type {
	if prev == nil && next == nil {
		return nil
	}
	if prev == nil {
		return FilterEmptyMap(next)
	}
	if next == nil {
		return FilterEmptyMap(prev)
	}
	merged := make(map[cfg.SymbolID][]typ.Type, len(prev)+len(next))
	for _, sym := range cfg.SortedSymbolIDs(prev) {
		evidence := NormalizeVector(prev[sym])
		if hasNonNilEvidence(evidence) {
			merged[sym] = evidence
		}
	}
	for _, sym := range cfg.SortedSymbolIDs(next) {
		evidence := NormalizeVector(next[sym])
		if !hasNonNilEvidence(evidence) {
			continue
		}
		if existing := merged[sym]; existing != nil {
			merged[sym] = JoinVectors(existing, evidence)
		} else {
			merged[sym] = evidence
		}
	}
	return merged
}

// FilterEmptyMap normalizes evidence and drops entries with no informative
// slots.
func FilterEmptyMap(evidence map[cfg.SymbolID][]typ.Type) map[cfg.SymbolID][]typ.Type {
	if evidence == nil {
		return nil
	}
	out := make(map[cfg.SymbolID][]typ.Type, len(evidence))
	for _, sym := range cfg.SortedSymbolIDs(evidence) {
		v := FilterEmptyVector(evidence[sym])
		if hasNonNilEvidence(v) {
			out[sym] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FilterEmptyVector normalizes one evidence vector and returns nil when all
// slots are empty.
func FilterEmptyVector(evidence []typ.Type) []typ.Type {
	v := NormalizeVector(evidence)
	if !hasNonNilEvidence(v) {
		return nil
	}
	return v
}

// NormalizeVector canonicalizes all occupied evidence slots.
func NormalizeVector(evidence []typ.Type) []typ.Type {
	var out []typ.Type
	for i, observed := range evidence {
		normalized := NormalizeType(observed)
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

// EqualVectors reports whether two normalized evidence vectors are structurally
// equal.
func EqualVectors(a, b []typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !typ.TypeEquals(a[i], b[i]) {
			return false
		}
	}
	return true
}

func hasNonNilEvidence(evidence []typ.Type) bool {
	for _, observed := range evidence {
		if observed != nil {
			return true
		}
	}
	return false
}

// JoinVectors joins two parameter evidence vectors element-wise.
func JoinVectors(a, b []typ.Type) []typ.Type {
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
		result[i] = Join(ai, bi)
	}
	return result
}

// Join merges two parameter evidence observations.
func Join(a, b typ.Type) typ.Type {
	a = NormalizeType(a)
	b = NormalizeType(b)
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
	if joined, ok := joinNilable(a, b); ok {
		return joined
	}
	return joinNonNil(a, b)
}

func joinNilable(a, b typ.Type) (typ.Type, bool) {
	ai, anil := value.SplitNilable(a)
	bi, bnil := value.SplitNilable(b)
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
	return typ.NewOptional(joinNonNil(ai, bi)), true
}

func joinNonNil(a, b typ.Type) typ.Type {
	if upper, ok := selectTableUpperBound(a, b); ok {
		return upper
	}
	if preferred, ok := value.PreferConcreteOverSoft(a, b); ok {
		return preferred
	}
	if value.CanSelfEmbed(a) && value.ContainsEquivalent(b, a) && !typ.IsAbsentOrUnknown(a) {
		if value.ContainsUnion(a) {
			return a
		}
		return typ.JoinPreferNonSoft(a, b)
	}
	if value.CanSelfEmbed(b) && value.ContainsEquivalent(a, b) && !typ.IsAbsentOrUnknown(b) {
		if value.ContainsUnion(b) {
			return b
		}
		return typ.JoinPreferNonSoft(a, b)
	}
	if value.IsTruthyRefinement(a, b) {
		return a
	}
	if value.IsTruthyRefinement(b, a) {
		return b
	}
	if joined, ok := typ.JoinCompatibleRecords(a, b); ok {
		return joined
	}
	if joined, ok := joinMapRecord(a, b); ok {
		return joined
	}
	if value.ExtendsRecord(a, b) {
		return a
	}
	if value.ExtendsRecord(b, a) {
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
	return NormalizeType(typ.JoinPreferNonSoft(a, b))
}

func joinMapRecord(a, b typ.Type) (typ.Type, bool) {
	if joined, ok := joinMapRecordDirected(a, b); ok {
		return joined, true
	}
	return joinMapRecordDirected(b, a)
}

func joinMapRecordDirected(mapType, recordType typ.Type) (typ.Type, bool) {
	m, ok := unwrap.Alias(mapType).(*typ.Map)
	if !ok || m == nil {
		return nil, false
	}
	r, ok := unwrap.Alias(recordType).(*typ.Record)
	if !ok || r == nil || !r.HasMapComponent() {
		return nil, false
	}

	key := joinNonNil(m.Key, r.MapKey)
	value := joinNonNil(m.Value, r.MapValue)
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
			fieldType = joinNonNil(field.Type, value)
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

func selectTableUpperBound(a, b typ.Type) (typ.Type, bool) {
	if isOnlyTableTop(a) && typ.IsAny(b) {
		return a, true
	}
	if isOnlyTableTop(b) && typ.IsAny(a) {
		return b, true
	}
	if containsTableTop(a) && coveredByTableTop(b) && subtype.IsSubtype(b, a) {
		return a, true
	}
	if containsTableTop(b) && coveredByTableTop(a) && subtype.IsSubtype(a, b) {
		return b, true
	}
	return nil, false
}

func containsTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return containsTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return containsTableTop(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if containsTableTop(member) {
				return true
			}
		}
	}
	return false
}

func isOnlyTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	if unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t)) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return isOnlyTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return isOnlyTableTop(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		hasTableTop := false
		for _, member := range v.Members {
			if unwrap.IsNilType(member) {
				continue
			}
			if !isOnlyTableTop(member) {
				return false
			}
			hasTableTop = true
		}
		return hasTableTop
	default:
		return false
	}
}

func coveredByTableTop(t typ.Type) bool {
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
		return coveredByTableTop(v.UnaliasedTarget())
	case *typ.Optional:
		return coveredByTableTop(v.Inner)
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && coveredByTableTop(v.Body)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !coveredByTableTop(member) {
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

// RefinesFunctionParam reports whether candidate is a valid directional
// refinement of baseline for parameter-slot facts.
func RefinesFunctionParam(candidate, baseline typ.Type) bool {
	return value.ElidesOptional(candidate, baseline) ||
		value.IsTruthyRefinement(candidate, baseline) ||
		refinesTableKeyByTruthiness(candidate, baseline)
}

func refinesTableKeyByTruthiness(candidate, baseline typ.Type) bool {
	if candidate == nil || baseline == nil || typ.TypeEquals(candidate, baseline) {
		return false
	}
	candidateInner, _ := value.SplitNilable(candidate)
	baselineInner, _ := value.SplitNilable(baseline)
	if candidateInner == nil || baselineInner == nil {
		return false
	}
	return nonNilRefinesTableKeyByTruthiness(candidateInner, baselineInner)
}

func nonNilRefinesTableKeyByTruthiness(candidate, baseline typ.Type) bool {
	candidate = unwrap.Alias(candidate)
	baseline = unwrap.Alias(baseline)
	switch b := baseline.(type) {
	case *typ.Record:
		c, ok := candidate.(*typ.Record)
		if !ok {
			return false
		}
		return recordRefinesTableKeyByTruthiness(c, b)
	case *typ.Map:
		c, ok := candidate.(*typ.Map)
		if !ok {
			return false
		}
		return value.IsTruthyRefinement(c.Key, b.Key) && value.Equivalent(c.Value, b.Value)
	default:
		return false
	}
}

func recordRefinesTableKeyByTruthiness(candidate, baseline *typ.Record) bool {
	if candidate == nil || baseline == nil || !candidate.HasMapComponent() || !baseline.HasMapComponent() {
		return false
	}
	if candidate.Open != baseline.Open || len(candidate.Fields) != len(baseline.Fields) {
		return false
	}
	if (candidate.Metatable == nil) != (baseline.Metatable == nil) {
		return false
	}
	if candidate.Metatable != nil && !typ.TypeEquals(candidate.Metatable, baseline.Metatable) {
		return false
	}
	for i, field := range candidate.Fields {
		other := baseline.Fields[i]
		if field.Name != other.Name || field.Optional != other.Optional || field.Readonly != other.Readonly {
			return false
		}
		if !value.Equivalent(field.Type, other.Type) {
			return false
		}
	}
	return value.IsTruthyRefinement(candidate.MapKey, baseline.MapKey) &&
		value.Equivalent(candidate.MapValue, baseline.MapValue)
}
