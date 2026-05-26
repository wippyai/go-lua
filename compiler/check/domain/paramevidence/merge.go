package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value"
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
			merged[sym] = WidenVectors(existing, evidence)
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

// FilterEmptyBodyVector normalizes body-effective evidence and drops empty slots.
func FilterEmptyBodyVector(evidence []typ.Type) []typ.Type {
	v := NormalizeBodyVector(evidence)
	if !hasNonNilEvidence(v) {
		return nil
	}
	return v
}

// NormalizeVector canonicalizes all occupied evidence slots.
func NormalizeVector(evidence []typ.Type) []typ.Type {
	return normalizeVectorWith(evidence, NormalizeType)
}

// NormalizeBodyVector canonicalizes body-effective evidence slots.
func NormalizeBodyVector(evidence []typ.Type) []typ.Type {
	return normalizeVectorWith(evidence, NormalizeBodyType)
}

func normalizeVectorWith(evidence []typ.Type, normalize func(typ.Type) typ.Type) []typ.Type {
	var out []typ.Type
	for i, observed := range evidence {
		normalized := normalize(observed)
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
	return joinVectorsWith(a, b, Join)
}

// JoinBodyVectors joins two body-effective parameter evidence vectors.
func JoinBodyVectors(a, b []typ.Type) []typ.Type {
	return joinVectorsWith(a, b, JoinBody)
}

// JoinEntryVectors joins observed call-entry parameter states. Unlike body
// contracts, explicit nil is a runtime state and must remain visible to the
// abstract interpreter.
func JoinEntryVectors(a, b []typ.Type) []typ.Type {
	return joinVectorsWith(a, b, JoinEntry)
}

// MergeArgumentObservation combines a current call-argument observation with a
// re-synthesized/contextual observation. Top-like candidates add no evidence;
// otherwise the more precise comparable type wins before falling back to the
// public non-soft join policy.
func MergeArgumentObservation(current, candidate typ.Type) typ.Type {
	if candidate == nil {
		return current
	}
	if current == nil {
		return candidate
	}
	if typ.IsAny(candidate) || typ.IsUnknown(candidate) {
		return current
	}
	if !typ.IsAny(current) && !typ.IsUnknown(current) {
		if subtype.IsSubtype(candidate, current) {
			return candidate
		}
		if subtype.IsSubtype(current, candidate) {
			return current
		}
	}
	return typ.JoinPreferNonSoft(current, candidate)
}

// ApplyBodyContractsToEntries combines observed call-entry states with body
// contracts for callee-body interpretation. Entry states keep their structural
// discriminants when they already satisfy the contract; incompatible shapes are
// intersected instead of widened.
func ApplyBodyContractsToEntries(contracts, entries []typ.Type) []typ.Type {
	return joinVectorsWith(entries, contracts, BodyEntryContractJoin)
}

// WidenVectors merges evidence at an interprocedural convergence boundary.
// If the next observation only embeds the previous parameter evidence under a
// container, the previous slot is already the finite parameter contract.
func WidenVectors(a, b []typ.Type) []typ.Type {
	return joinVectorsWith(a, b, WidenJoin)
}

func joinVectorsWith(a, b []typ.Type, join func(typ.Type, typ.Type) typ.Type) []typ.Type {
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
		result[i] = join(ai, bi)
	}
	return result
}

// JoinCallVectors joins public parameter contracts. Observed call arguments do
// not enter this channel; callers that need dynamic observation joins use the
// body-effective vector instead.
func JoinCallVectors(a, b []typ.Type) []typ.Type {
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
		result[i] = PublicContractJoin(ai, bi)
	}
	return result
}

// PublicSignatureVector returns the explicit public parameter contract.
func PublicSignatureVector(public []typ.Type) []typ.Type {
	return FilterEmptyVector(public)
}

// HardPublicEvidence reports whether body evidence is a caller obligation rather
// than a passive local refinement.
func HardPublicEvidence(t typ.Type) bool {
	return IsInformative(t) && !PassiveOptionalRecordEvidence(t)
}

// BodyEvidenceDominatesCallArg reports whether an existing body-effective
// parameter contract already admits an observed call argument. In that case the
// call shape adds no information and must not narrow or inflate the callee body
// state.
func BodyEvidenceDominatesCallArg(existing, observed typ.Type) bool {
	existing = NormalizeBodyType(existing)
	observed = NormalizeBodyType(observed)
	if existing == nil || observed == nil {
		return false
	}
	if value.SameConvergedFact(existing, observed) {
		return true
	}
	if !HardPublicEvidence(existing) {
		return false
	}
	return value.Covers(existing, observed)
}

// BodyContractJoin merges a body-proven parameter contract into existing
// body-effective evidence. When the existing evidence is merely a compatible
// call shape, the contract wins because it is the callee's semantic
// precondition, not another passive observation.
func BodyContractJoin(existing, contract typ.Type) typ.Type {
	existing = NormalizeBodyType(existing)
	contract = NormalizeBodyType(contract)
	if existing == nil || typ.IsAny(existing) || typ.IsUnknown(existing) {
		return contract
	}
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) {
		return existing
	}
	if value.Covers(contract, existing) || value.Covers(existing, contract) {
		return contract
	}
	return subtype.NormalizeIntersection(existing, contract)
}

// BodyEntryContractJoin applies a body contract to an observed entry state.
// Unlike BodyContractJoin, a precise entry state that satisfies a broad contract
// remains precise because the abstract interpreter needs its discriminants.
func BodyEntryContractJoin(entry, contract typ.Type) typ.Type {
	entry = NormalizeBodyType(entry)
	contract = NormalizeBodyType(contract)
	if entry == nil || typ.IsAny(entry) || typ.IsUnknown(entry) {
		return contract
	}
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) {
		return entry
	}
	if value.Covers(contract, entry) {
		return entry
	}
	if value.Covers(entry, contract) {
		return contract
	}
	return subtype.NormalizeIntersection(entry, contract)
}

// HardContractJoin intersects hard parameter obligations, with concrete
// evidence dominating top-like dynamic observations.
func HardContractJoin(prev, next typ.Type) typ.Type {
	if prev == nil || typ.IsAny(prev) || typ.IsUnknown(prev) {
		return next
	}
	if next == nil || typ.IsAny(next) || typ.IsUnknown(next) {
		return prev
	}
	if precise, ok := preciseTableContract(prev, next); ok {
		return precise
	}
	if finite, ok := finiteRecursiveContract(prev, next); ok {
		return finite
	}
	if value.Covers(next, prev) {
		return prev
	}
	if value.Covers(prev, next) {
		return next
	}
	return subtype.NormalizeIntersection(prev, next)
}

// PublicContractJoin intersects public obligations, with hard evidence
// dominating top-like dynamic observations.
func PublicContractJoin(prev, next typ.Type) typ.Type {
	return HardContractJoin(prev, next)
}

func finiteRecursiveContract(prev, next typ.Type) (typ.Type, bool) {
	if upper, ok := value.SelfEmbeddingUpperBound(prev, next); ok {
		return upper, true
	}
	if upper, ok := value.SelfEmbeddingUpperBound(next, prev); ok {
		return upper, true
	}
	return nil, false
}

func preciseTableContract(a, b typ.Type) (typ.Type, bool) {
	if tableTopContract(a) && value.IsStructuredTableShape(unwrap.Optional(b)) {
		return b, true
	}
	if tableTopContract(b) && value.IsStructuredTableShape(unwrap.Optional(a)) {
		return a, true
	}
	return nil, false
}

func tableTopContract(t typ.Type) bool {
	inner := unwrap.Optional(t)
	return value.IsOpenTopRecord(inner) || unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(inner))
}

// Join merges two parameter evidence observations.
func Join(a, b typ.Type) typ.Type {
	return joinWith(a, b, NormalizeType, joinNonNil)
}

// WidenJoin is the public parameter-evidence convergence join.
func WidenJoin(prev, next typ.Type) typ.Type {
	prev = NormalizeType(prev)
	next = NormalizeType(next)
	if evidenceGrowthEmbedsStable(prev, next) {
		return prev
	}
	return Join(prev, next)
}

// JoinBody merges two body-effective parameter evidence observations while
// preserving structural literals needed for path-sensitive interpretation.
func JoinBody(a, b typ.Type) typ.Type {
	return joinWith(a, b, NormalizeBodyType, joinNonNilBody)
}

// JoinEntry merges observed call-entry states. It preserves explicit nilability
// so closed-world body interpretation can prove dead branches such as
// `if arg then ...` when every observed call passed nil.
func JoinEntry(a, b typ.Type) typ.Type {
	a = NormalizeBodyType(a)
	b = NormalizeBodyType(b)
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) {
		return a
	}
	if joined, ok := joinNilable(a, b, joinNonNilBody); ok {
		return joined
	}
	return joinNonNilBody(a, b)
}

// JoinCall merges two call-boundary parameter observations.
func JoinCall(a, b typ.Type) typ.Type {
	a = NormalizeType(a)
	b = NormalizeType(b)
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.IsAny(a) || typ.IsAny(b) {
		return typ.Any
	}
	if typ.IsUnknown(a) {
		return b
	}
	if typ.IsUnknown(b) {
		return a
	}
	return Join(a, b)
}

func joinWith(a, b typ.Type, normalize normalizer, nonNil func(typ.Type, typ.Type) typ.Type) typ.Type {
	if normalize == nil {
		normalize = NormalizeType
	}
	if nonNil == nil {
		nonNil = joinNonNil
	}
	a = normalize(a)
	b = normalize(b)
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if typ.TypeEquals(a, b) {
		return a
	}
	if unwrap.IsNilType(a) && !unwrap.IsNilType(b) {
		return b
	}
	if unwrap.IsNilType(b) && !unwrap.IsNilType(a) {
		return a
	}
	if joined, ok := joinNilable(a, b, nonNil); ok {
		return joined
	}
	return nonNil(a, b)
}

func joinNilable(a, b typ.Type, nonNil func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
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
	return typ.NewOptional(nonNil(ai, bi)), true
}

func joinNonNil(a, b typ.Type) typ.Type {
	return joinNonNilWith(a, b, NormalizeType, Join, joinNonNil)
}

func joinNonNilBody(a, b typ.Type) typ.Type {
	return joinNonNilWith(a, b, NormalizeBodyType, JoinBody, joinNonNilBody)
}

func joinNonNilWith(
	a, b typ.Type,
	normalize normalizer,
	recursiveJoin JoinFn,
	recursiveNonNil func(typ.Type, typ.Type) typ.Type,
) typ.Type {
	if upper, ok := value.SelectTableUpperBound(a, b); ok {
		return upper
	}
	if seq, ok := value.JoinSequenceShape(a, b, recursiveJoin); ok {
		if joinAdmitsObservedPair(a, b, seq) {
			return seq
		}
	}
	if joined, ok := value.JoinRecordShape(a, b, recursiveJoin); ok {
		if joinAdmitsObservedPair(a, b, joined) {
			return joined
		}
	}
	if preferred, ok := value.PreferConcreteOverSoft(a, b); ok {
		return preferred
	}
	if upper, ok := value.SelfEmbeddingUpperBound(a, b); ok {
		return upper
	}
	if value.IsTruthyRefinement(a, b) {
		return a
	}
	if value.IsTruthyRefinement(b, a) {
		return b
	}
	if joined, ok := value.JoinMapRecordShape(a, b, recursiveNonNil); ok {
		if joinAdmitsObservedPair(a, b, joined) {
			return joined
		}
	}
	if joined, ok := value.JoinStructuralUnionShape(a, b, recursiveJoin); ok {
		if joinAdmitsObservedPair(a, b, joined) {
			return joined
		}
	}
	if !typ.IsAbsentOrUnknown(a) && !typ.IsAbsentOrUnknown(b) {
		if value.Covers(b, a) {
			return b
		}
		if value.Covers(a, b) {
			return a
		}
	}
	return typ.JoinPreferNonSoft(a, b)
}

func joinAdmitsObservedPair(a, b, joined typ.Type) bool {
	return joined != nil && joinAdmitsObservation(a, joined) && joinAdmitsObservation(b, joined)
}

func joinAdmitsObservation(observed, joined typ.Type) bool {
	return value.Covers(joined, observed) || typ.MorePrecise(joined, observed)
}

func evidenceGrowthEmbedsStable(stable, growing typ.Type) bool {
	if stable == nil || growing == nil || typ.TypeEquals(stable, growing) {
		return false
	}
	return value.CanSelfEmbed(stable) && value.ContainsNestedStructuralShape(growing, stable)
}

// RefinesFunctionParam reports whether candidate is a valid directional
// refinement of baseline for parameter-slot facts.
func RefinesFunctionParam(candidate, baseline typ.Type) bool {
	return value.ElidesOptional(candidate, baseline) ||
		value.IsTruthyRefinement(candidate, baseline) ||
		value.RefinesTableKeyByTruthiness(candidate, baseline)
}
