package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WidenMap merges two parameter evidence maps with the same vector law used by
// FunctionFacts projection.
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

// JoinEntryConvergeVectors joins observed call-entry parameter states across a
// recursive fixpoint boundary. A pure-nil prior slot is a seed produced before
// the argument's defining call was inferred; once a later iteration proves a
// definite non-nil entry it must replace that seed instead of widening it to an
// optional. Genuine nilable observations already carry their nilability into the
// incoming slot within a single iteration, so they remain optional here.
func JoinEntryConvergeVectors(a, b []typ.Type) []typ.Type {
	return joinVectorsWith(a, b, JoinEntryConverge)
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
// precondition, not another passive observation. Structurally-incompatible
// evidence is refined member-wise against the contract so the result stays
// bounded across fixpoint iterations.
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
	return refineEntryByContract(existing, contract)
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
	return refineEntryByContract(entry, contract)
}

// EntryContradictsBodyContract reports whether observed call-entry evidence and
// a body-proven contract are disjoint. In that case the contradiction is local to
// the analyzed entry state: the body will diagnose the impossible operation from
// its seeded parameter value, and projecting the same contract as a public caller
// obligation would duplicate the error at the call boundary.
//
// Top-like entry evidence is not a contradiction; it carries too little runtime
// information, so the body contract remains the exported obligation.
func EntryContradictsBodyContract(entry, contract typ.Type) bool {
	entry = NormalizeBodyType(entry)
	contract = NormalizeBodyType(contract)
	if entry == nil || contract == nil {
		return false
	}
	if typ.IsAny(entry) || typ.IsUnknown(entry) || typ.IsAny(contract) || typ.IsUnknown(contract) {
		return false
	}
	if value.Covers(contract, entry) || value.Covers(entry, contract) {
		return false
	}
	intersection := subtype.NormalizeIntersection(entry, contract)
	return intersection == nil || typ.IsNever(intersection)
}

// refineEntryByContract intersects an observed entry state with a body contract
// member-wise so the result stays bounded across fixpoint iterations. A whole
// union-of-unions intersection cross-distributes into a quadratically growing
// set of intersection members; refining each entry member independently and
// keeping members that already satisfy the contract is idempotent once an entry
// has been refined, so the entry channel converges. A non-nilable contract is a
// hard precondition that the body only satisfies for non-nil values, so it
// refines away a seed nil in the entry; a nilable contract leaves entry
// nilability intact because nil remains a valid runtime entry state.
func refineEntryByContract(entry, contract typ.Type) typ.Type {
	entryNonNil, entryNilable := value.SplitNilable(entry)
	if entryNonNil == nil {
		return entry
	}
	_, contractNilable := value.SplitNilable(contract)
	resultNilable := entryNilable && contractNilable
	members := unionMembers(entryNonNil)
	refined := make([]typ.Type, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		if value.Covers(contract, member) {
			refined = append(refined, member)
			continue
		}
		intersected := subtype.NormalizeIntersection(member, contract)
		if intersected == nil || typ.IsNever(intersected) {
			continue
		}
		refined = append(refined, intersected)
	}
	if len(refined) == 0 {
		if resultNilable {
			return typ.Nil
		}
		return entry
	}
	result := typ.NewUnion(refined...)
	if resultNilable {
		result = typ.NewOptional(result)
	}
	return result
}

// unionMembers returns the alias-stripped top-level union members of t, or t
// itself when it is not a union.
func unionMembers(t typ.Type) []typ.Type {
	if u, ok := unwrap.Alias(t).(*typ.Union); ok {
		return u.Members
	}
	return []typ.Type{t}
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

// JoinConvergeCallVectors joins public parameter contracts across a recursive
// fixpoint boundary. It mirrors JoinCallVectors for non-boundary merges but
// replaces the intersecting HardContractJoin with the convergence-bounded
// ConvergeContractJoin so structurally-incompatible call evidence widens to a
// finite union instead of forming an ever-growing intersection.
func JoinConvergeCallVectors(a, b []typ.Type) []typ.Type {
	return joinVectorsWith(a, b, ConvergeContractJoin)
}

// ConvergeContractJoin merges one public parameter contract at a recursive
// fixpoint boundary. A never-seed contract carries no evidence yet, so it yields
// to concrete call evidence. When neither side covers the other the join widens
// to the union rather than intersecting, keeping the contract monotone and
// bounded across iterations.
func ConvergeContractJoin(prev, next typ.Type) typ.Type {
	if prev == nil || typ.IsAny(prev) || typ.IsUnknown(prev) {
		return next
	}
	if next == nil || typ.IsAny(next) || typ.IsUnknown(next) {
		return prev
	}
	if isNeverSeed(prev) && !isNeverSeed(next) {
		return next
	}
	if isNeverSeed(next) && !isNeverSeed(prev) {
		return prev
	}
	if precise, ok := preciseTableContract(prev, next); ok {
		return precise
	}
	if finite, ok := finiteRecursiveContract(prev, next); ok {
		return finite
	}
	// Convergence widens upward: when one side already covers the other, keep the
	// broader contract so the obligation only ever weakens across iterations.
	if value.Covers(next, prev) {
		return next
	}
	if value.Covers(prev, next) {
		return prev
	}
	return typ.JoinPreferNonSoft(prev, next)
}

// isNeverSeed reports whether a parameter contract is the empty "no evidence
// yet" seed: the bottom type, or a container/optional whose every value-carrying
// position bottoms out at never. Such a seed admits no concrete value and must
// yield to real call evidence at the fixpoint boundary. A type that carries any
// concrete content is not a seed.
func isNeverSeed(t typ.Type) bool {
	if t == nil {
		return false
	}
	if typ.IsNever(t) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Optional:
		return isNeverSeed(v.Inner)
	case *typ.Array:
		return isNeverSeed(v.Element)
	case *typ.Map:
		return isNeverSeed(v.Value)
	case *typ.ReadonlyMap:
		return isNeverSeed(v.Value)
	default:
		return false
	}
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

// JoinEntryConverge merges observed call-entry states across a recursive
// fixpoint boundary. It behaves like JoinEntry except that a pure-nil prior
// observation is treated as an uninferred seed: a definite non-nil incoming
// observation replaces it rather than widening to an optional. An incoming
// observation that is itself nilable keeps its nilability, so genuine optional
// parameters are preserved.
func JoinEntryConverge(prev, next typ.Type) typ.Type {
	prev = NormalizeBodyType(prev)
	next = NormalizeBodyType(next)
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	if unwrap.IsNilType(prev) {
		ni, nilable := value.SplitNilable(next)
		if !nilable && ni != nil {
			return next
		}
	}
	return JoinEntry(prev, next)
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
	return joinNonNilWith(a, b, NormalizeType, Join, joinNonNil, nil)
}

func joinNonNilBody(a, b typ.Type) typ.Type {
	return joinNonNilWith(a, b, NormalizeBodyType, JoinBody, joinNonNilBody, joinBodyFallback)
}

func joinNonNilWith(
	a, b typ.Type,
	normalize normalizer,
	recursiveJoin JoinFn,
	recursiveNonNil func(typ.Type, typ.Type) typ.Type,
	fallback func(typ.Type, typ.Type) typ.Type,
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
	if fallback != nil {
		if union, ok := bodyVariantUnion(a, b); ok {
			return union
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
	if fallback != nil {
		return fallback(a, b)
	}
	return typ.JoinPreferNonSoft(a, b)
}

func joinBodyFallback(a, b typ.Type) typ.Type {
	if union, ok := bodyVariantUnion(a, b); ok {
		return union
	}
	return typ.JoinPreferNonSoft(a, b)
}

func bodyVariantUnion(a, b typ.Type) (typ.Type, bool) {
	members := append([]typ.Type{}, unionMembers(a)...)
	members = append(members, unionMembers(b)...)
	if len(members) < 2 {
		return nil, false
	}
	for _, member := range members {
		if _, ok := unwrap.Alias(member).(*typ.Record); !ok {
			return nil, false
		}
	}
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			if _, ok := value.JoinRecordShape(members[i], members[j], JoinBody); !ok {
				return typ.NewUnion(members...), true
			}
		}
	}
	return nil, false
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
