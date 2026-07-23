package callpayload

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
)

// CallOutcomeAlternativeSet is the exact disjunction of outcomes produced by
// reachable guarded valuations. It deliberately does not collapse distinct
// outcomes field-by-field: result, presence, and refinement correlations stay
// inside their originating alternative. The zero value is the empty set and
// therefore means that the producer did not execute on any reachable valuation.
type CallOutcomeAlternativeSet struct {
	outcomes []CallOutcome
}

// NewCallOutcomeAlternativeSet detaches, normalizes, and deduplicates outcomes.
// An executed empty CallOutcome is an ordinary member and is distinct from the
// empty set.
func NewCallOutcomeAlternativeSet(reg *axis.Registry, outcomes ...CallOutcome) CallOutcomeAlternativeSet {
	if reg == nil || len(outcomes) == 0 {
		return CallOutcomeAlternativeSet{}
	}
	out := CallOutcomeAlternativeSet{outcomes: make([]CallOutcome, 0, len(outcomes))}
	for _, candidate := range outcomes {
		candidate = NormalizeCallOutcome(reg, candidate)
		duplicate := false
		for _, retained := range out.outcomes {
			if EqualCallOutcome(reg, retained, candidate) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out.outcomes = append(out.outcomes, candidate)
		}
	}
	return out
}

// Empty reports whether no guarded valuation produced an outcome.
func (s CallOutcomeAlternativeSet) Empty() bool { return len(s.outcomes) == 0 }

// Outcomes returns detached immutable-publication storage.
func (s CallOutcomeAlternativeSet) Outcomes() []CallOutcome {
	out := make([]CallOutcome, len(s.outcomes))
	for i := range s.outcomes {
		out[i] = s.outcomes[i].Clone()
	}
	return out
}

// Normalize returns the canonical semantic set represented by s.
func (s CallOutcomeAlternativeSet) Normalize(reg *axis.Registry) CallOutcomeAlternativeSet {
	return NewCallOutcomeAlternativeSet(reg, s.outcomes...)
}

// Equal reports exact set equality, independent of insertion order.
func (s CallOutcomeAlternativeSet) Equal(reg *axis.Registry, other CallOutcomeAlternativeSet) bool {
	left, right := s.Normalize(reg), other.Normalize(reg)
	if len(left.outcomes) != len(right.outcomes) {
		return false
	}
	for _, candidate := range left.outcomes {
		found := false
		for _, target := range right.outcomes {
			if EqualCallOutcome(reg, candidate, target) {
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

// Join is exact disjunction: canonical deduplicated set union.
func (s CallOutcomeAlternativeSet) Join(reg *axis.Registry, other CallOutcomeAlternativeSet) CallOutcomeAlternativeSet {
	joined := make([]CallOutcome, 0, len(s.outcomes)+len(other.outcomes))
	joined = append(joined, s.outcomes...)
	joined = append(joined, other.outcomes...)
	return NewCallOutcomeAlternativeSet(reg, joined...)
}

// Fingerprint is an order-independent accelerator consistent with Equal.
// Structural equality remains authoritative inside hash buckets.
func (s CallOutcomeAlternativeSet) Fingerprint(reg *axis.Registry) uint64 {
	normalized := s.Normalize(reg)
	var xor, sum uint64
	for _, outcome := range normalized.outcomes {
		h := FingerprintCallOutcome(reg, outcome)
		xor ^= internalhash.MixHash(h, 0x9e3779b97f4a7c15)
		sum += internalhash.MixHash(h, 0xc2b2ae3d27d4eb4f)
	}
	return internalhash.MixHash(xor, internalhash.MixHash(sum, uint64(len(normalized.outcomes))))
}

// NormalizeCallOutcome returns detached storage with every algebra-owned lane
// normalized. Descriptor-total binding below makes a newly registered field a
// mandatory compile-time-visible ownership decision.
func NormalizeCallOutcome(reg *axis.Registry, outcome CallOutcome) CallOutcome {
	out := outcome.Clone()
	for _, binding := range callOutcomeAlgebra {
		if binding.normalize != nil {
			binding.normalize(reg, &out)
		}
	}
	return out
}

// EqualCallOutcome reports semantic equality across every registered field.
func EqualCallOutcome(reg *axis.Registry, left, right CallOutcome) bool {
	if reg == nil {
		return false
	}
	left, right = NormalizeCallOutcome(reg, left), NormalizeCallOutcome(reg, right)
	for _, binding := range callOutcomeAlgebra {
		if !binding.equal(reg, left, right) {
			return false
		}
	}
	return true
}

// JoinCallOutcome returns the least upper bound of two reachable peer
// alternatives. Reachability belongs to the surrounding guarded tuple: a zero
// CallOutcome here is an executed empty payload, not bottom.
func JoinCallOutcome(reg *axis.Registry, left, right CallOutcome) CallOutcome {
	if reg == nil {
		return CallOutcome{}
	}
	left, right = NormalizeCallOutcome(reg, left), NormalizeCallOutcome(reg, right)
	var out CallOutcome
	for _, binding := range callOutcomeAlgebra {
		binding.join(reg, left, right, &out)
	}
	return NormalizeCallOutcome(reg, out)
}

// Collapse projects an exact guarded alternative set into the public
// CallOutcome lattice after stabilization. It is intentionally not used by
// guarded solving, where alternatives remain separate to retain correlation.
func (s CallOutcomeAlternativeSet) Collapse(reg *axis.Registry) CallOutcome {
	if reg == nil || len(s.outcomes) == 0 {
		return CallOutcome{}
	}
	out := NormalizeCallOutcome(reg, s.outcomes[0])
	for _, alternative := range s.outcomes[1:] {
		out = JoinCallOutcome(reg, out, alternative)
	}
	return out
}

// FingerprintCallOutcome is a deterministic accelerator consistent with
// EqualCallOutcome. It intentionally permits collisions; equality is the
// authority. Field presence is emitted in descriptor order and high-volume
// product result values contribute their canonical product hash.
func FingerprintCallOutcome(reg *axis.Registry, outcome CallOutcome) uint64 {
	outcome = NormalizeCallOutcome(reg, outcome)
	w := internalhash.NewWriter()
	_, _ = w.WriteString("callpayload.CallOutcome/v1")
	for _, lane := range callOutcomeLanes {
		_, _ = w.WriteString(lane.fieldName)
		w.WriteBool(lane.has(outcome))
	}
	for _, result := range outcome.Results {
		w.WriteIntDecimal(int64(result.Index))
		w.WriteUintHex(product.Hash(reg, result.Value))
	}
	w.WriteUintHex(DiagnosticOutputFromCallOutcome(reg, outcome).Fingerprint(reg))
	return w.Sum64()
}

type callOutcomeFieldAlgebra struct {
	field     string
	normalize func(*axis.Registry, *CallOutcome)
	equal     func(*axis.Registry, CallOutcome, CallOutcome) bool
	join      func(*axis.Registry, CallOutcome, CallOutcome, *CallOutcome)
}

var callOutcomeAlgebra = bindCallOutcomeAlgebra()

func bindCallOutcomeAlgebra() []callOutcomeFieldAlgebra {
	handlers := map[string]callOutcomeFieldAlgebra{
		"Results": {field: "Results", normalize: normalizeOutcomeResults,
			equal: func(reg *axis.Registry, a, b CallOutcome) bool {
				return equalSlice(a.Results, b.Results, func(x, y CallResult) bool { return x.Index == y.Index && product.Equal(reg, x.Value, y.Value) })
			},
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.Results = joinOutcomeResults(reg, a.Results, b.Results)
			}},
		"PostReturnAuthority": {field: "PostReturnAuthority",
			equal: func(_ *axis.Registry, a, b CallOutcome) bool { return a.PostReturnAuthority == b.PostReturnAuthority },
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.PostReturnAuthority = a.PostReturnAuthority && b.PostReturnAuthority
			}},
		"SuspensionKnown": {field: "SuspensionKnown", normalize: normalizeOutcomeDiagnostics,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool { return a.SuspensionKnown == b.SuspensionKnown },
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.SuspensionKnown = joinedOutcomeDiagnostics(reg, a, b).SuspensionKnown
			}},
		"MaySuspend": {field: "MaySuspend",
			equal: func(_ *axis.Registry, a, b CallOutcome) bool { return a.MaySuspend == b.MaySuspend },
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.MaySuspend = joinedOutcomeDiagnostics(reg, a, b).MaySuspend
			}},
		"NormalReturnFacts": {field: "NormalReturnFacts",
			normalize: func(reg *axis.Registry, out *CallOutcome) {
				out.NormalReturnFacts = callboundary.NormalizeNormalReturnFacts(reg, out.NormalReturnFacts)
			},
			equal: func(reg *axis.Registry, a, b CallOutcome) bool {
				return callboundary.NormalReturnFactsEqualNormalized(reg, a.NormalReturnFacts, b.NormalReturnFacts)
			},
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.NormalReturnFacts = callboundary.JoinNormalReturnFacts(reg, a.NormalReturnFacts, b.NormalReturnFacts)
			}},
		"ProtectedCallTypestate": {field: "ProtectedCallTypestate",
			equal: func(_ *axis.Registry, a, b CallOutcome) bool {
				return a.ProtectedCallTypestate.Equal(b.ProtectedCallTypestate)
			},
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ProtectedCallTypestate = callboundary.JoinProtectedCallTypestate(a.ProtectedCallTypestate, b.ProtectedCallTypestate)
			}},
		"HeapTableObjects": {field: "HeapTableObjects",
			equal: func(reg *axis.Registry, a, b CallOutcome) bool {
				return heapidentity.MapDomain(reg).Equal(a.HeapTableObjects, b.HeapTableObjects)
			},
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.HeapTableObjects = heapidentity.MapDomain(reg).Join(a.HeapTableObjects, b.HeapTableObjects)
			}},
		"Placements": {field: "Placements", normalize: normalizeOutcomePlacements,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool { return equalPlacementMaps(a.Placements, b.Placements) },
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.Placements = joinPlacementMaps(a.Placements, b.Placements)
			}},
		"ParamObligations": {field: "ParamObligations", equal: func(reg *axis.Registry, a, b CallOutcome) bool {
			return equalSlice(a.ParamObligations, b.ParamObligations, func(x, y CallParamObligation) bool {
				return x.ParamIndex == y.ParamIndex && x.Origin == y.Origin && x.SignatureSurface == y.SignatureSurface && product.Equal(reg, x.Value, y.Value)
			})
		}, join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
			out.ParamObligations = joinedOutcomeDiagnostics(reg, a, b).ParamObligations
		}},
		"PathObligations": {field: "PathObligations", equal: func(reg *axis.Registry, a, b CallOutcome) bool {
			return equalSlice(a.PathObligations, b.PathObligations, func(x, y CallPathObligation) bool {
				return x.Path.Equal(y.Path) && product.Equal(reg, x.Value, y.Value)
			})
		}, join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
			out.PathObligations = joinedOutcomeDiagnostics(reg, a, b).PathObligations
		}},
		"TypestateRequirements": {field: "TypestateRequirements", normalize: normalizeOutcomeTypestateRequirements,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool {
				return callTypestateRequirementSet.Equal(a.TypestateRequirements, b.TypestateRequirements)
			},
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.TypestateRequirements = callTypestateRequirementSet.Join(a.TypestateRequirements, b.TypestateRequirements)
			}},
		"ParamPathRefinements": {field: "ParamPathRefinements", normalize: normalizeOutcomeParamPathRefinements,
			equal: func(reg *axis.Registry, a, b CallOutcome) bool {
				return callParamPathRefinementMap(reg).Equal(a.ParamPathRefinements, b.ParamPathRefinements)
			},
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ParamPathRefinements = callParamPathRefinementMap(reg).Join(a.ParamPathRefinements, b.ParamPathRefinements)
			}},
		"ParamPathWrites": {field: "ParamPathWrites", normalize: normalizeOutcomeParamPathWrites,
			equal: func(reg *axis.Registry, a, b CallOutcome) bool {
				return callParamPathWriteMap(reg).Equal(a.ParamPathWrites, b.ParamPathWrites)
			},
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ParamPathWrites = callParamPathWriteMap(reg).Join(a.ParamPathWrites, b.ParamPathWrites)
			}},
		"ParamLengthFloors": {field: "ParamLengthFloors", normalize: normalizeOutcomeParamLengthFloors,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool {
				return equalParamLengthFloors(a.ParamLengthFloors, b.ParamLengthFloors)
			},
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ParamLengthFloors = joinParamLengthFloors(a.ParamLengthFloors, b.ParamLengthFloors)
			}},
		"ParamPathInvalidations": {field: "ParamPathInvalidations", normalize: normalizeOutcomeParamPathInvalidations,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool {
				return callParamPathInvalidationSet.Equal(a.ParamPathInvalidations, b.ParamPathInvalidations)
			},
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ParamPathInvalidations = callParamPathInvalidationSet.Join(a.ParamPathInvalidations, b.ParamPathInvalidations)
			}},
		"ParamConditions": {field: "ParamConditions", normalize: normalizeOutcomeParamConditions,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool {
				return callParamConditionSet.Equal(a.ParamConditions, b.ParamConditions)
			},
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ParamConditions = callParamConditionSet.Join(a.ParamConditions, b.ParamConditions)
			}},
		"ParamPathRelations": {field: "ParamPathRelations", normalize: normalizeOutcomeParamPathRelations,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool {
				return callParamPathRelationSet.Equal(a.ParamPathRelations, b.ParamPathRelations)
			},
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ParamPathRelations = callParamPathRelationSet.Join(a.ParamPathRelations, b.ParamPathRelations)
			}},
		"ReturnConditionRefinements": {field: "ReturnConditionRefinements", normalize: normalizeOutcomeReturnConditionRefinements,
			equal: func(reg *axis.Registry, a, b CallOutcome) bool {
				return callReturnConditionRefinementMap(reg).Equal(a.ReturnConditionRefinements, b.ReturnConditionRefinements)
			},
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ReturnConditionRefinements = callReturnConditionRefinementMap(reg).Join(a.ReturnConditionRefinements, b.ReturnConditionRefinements)
			}},
		"ReturnConditionSlots": {field: "ReturnConditionSlots", normalize: normalizeOutcomeReturnConditionSlots,
			equal: func(reg *axis.Registry, a, b CallOutcome) bool {
				return callReturnConditionSlotMap(reg).Equal(a.ReturnConditionSlots, b.ReturnConditionSlots)
			},
			join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ReturnConditionSlots = callReturnConditionSlotMap(reg).Join(a.ReturnConditionSlots, b.ReturnConditionSlots)
			}},
		"ReturnPresenceRelations": {field: "ReturnPresenceRelations", normalize: normalizeOutcomeReturnPresenceRelations,
			equal: func(_ *axis.Registry, a, b CallOutcome) bool {
				return callReturnPresenceRelationSet.Equal(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
			},
			join: func(_ *axis.Registry, a, b CallOutcome, out *CallOutcome) {
				out.ReturnPresenceRelations = callReturnPresenceRelationSet.Join(a.ReturnPresenceRelations, b.ReturnPresenceRelations)
			}},
		"ParamExposures": {field: "ParamExposures", equal: func(reg *axis.Registry, a, b CallOutcome) bool {
			return equalSlice(a.ParamExposures, b.ParamExposures, func(x, y CallParamExposure) bool {
				return x.Kind == y.Kind && x.Source.Equal(y.Source) && product.Equal(reg, x.Contract, y.Contract)
			})
		}, join: func(reg *axis.Registry, a, b CallOutcome, out *CallOutcome) {
			out.ParamExposures = joinedOutcomeDiagnostics(reg, a, b).ParamExposures
		}},
	}
	descriptors := CallOutcomeDescriptors()
	out := make([]callOutcomeFieldAlgebra, 0, len(descriptors))
	seen := make(map[string]struct{}, len(descriptors))
	for _, descriptor := range descriptors {
		name := string(descriptor.Kind)
		handler, ok := handlers[name]
		if !ok || handler.field != name || handler.equal == nil || handler.join == nil {
			panic("callpayload: CallOutcome algebra has no handler for " + name)
		}
		if _, duplicate := seen[name]; duplicate {
			panic("callpayload: duplicate CallOutcome algebra handler for " + name)
		}
		seen[name] = struct{}{}
		out = append(out, handler)
	}
	if len(seen) != len(handlers) {
		panic("callpayload: CallOutcome algebra has orphan handlers")
	}
	return out
}

func normalizeOutcomeResults(reg *axis.Registry, outcome *CallOutcome) {
	if outcome == nil || len(outcome.Results) == 0 {
		return
	}
	sort.SliceStable(outcome.Results, func(i, j int) bool { return outcome.Results[i].Index < outcome.Results[j].Index })
	out := outcome.Results[:0]
	for _, result := range outcome.Results {
		if len(out) != 0 && out[len(out)-1].Index == result.Index {
			out[len(out)-1].Value = product.Join(reg, out[len(out)-1].Value, result.Value)
			continue
		}
		out = append(out, result)
	}
	outcome.Results = out
}

func normalizeOutcomeDiagnostics(reg *axis.Registry, outcome *CallOutcome) {
	if outcome == nil {
		return
	}
	DiagnosticOutputFromCallOutcome(reg, *outcome).ApplyTo(reg, outcome)
}

func joinedOutcomeDiagnostics(reg *axis.Registry, left, right CallOutcome) DiagnosticOutput {
	return DiagnosticOutputFromCallOutcome(reg, left).Join(reg, DiagnosticOutputFromCallOutcome(reg, right))
}

func joinOutcomeResults(reg *axis.Registry, left, right []CallResult) []CallResult {
	out := CallOutcome{Results: make([]CallResult, 0, len(left)+len(right))}
	out.Results = append(out.Results, left...)
	out.Results = append(out.Results, right...)
	normalizeOutcomeResults(reg, &out)
	return out.Results
}

func joinPlacementMaps(left, right map[identity.ID]placement.Value) map[identity.ID]placement.Value {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	out := make(map[identity.ID]placement.Value, len(left)+len(right))
	for id, value := range left {
		if !value.IsBottom() {
			out[id] = value
		}
	}
	for id, value := range right {
		if value.IsBottom() {
			continue
		}
		out[id] = placement.Join(out[id], value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOutcomePlacements(_ *axis.Registry, out *CallOutcome) {
	out.Placements = joinPlacementMaps(nil, out.Placements)
}

type callTypestateRequirementKey struct {
	target   pathdom.PathKey
	protocol string
	state    string
}

var callTypestateRequirementSet = factset.Set[callTypestateRequirementKey, CallTypestateRequirement]{
	Key: func(f CallTypestateRequirement) callTypestateRequirementKey {
		return callTypestateRequirementKey{target: f.Target.Key(), protocol: string(f.Protocol), state: string(f.State)}
	},
	EqualFact: func(a, b CallTypestateRequirement) bool {
		return a.Target.Equal(b.Target) && a.Protocol == b.Protocol && a.State == b.State
	},
	Less: func(a, b CallTypestateRequirement) bool {
		if !a.Target.Equal(b.Target) {
			return a.Target.Less(b.Target)
		}
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		return a.State < b.State
	},
	Valid: func(f CallTypestateRequirement) bool { return !f.Target.IsEmpty() && f.Protocol != "" && f.State != "" },
	CloneFact: func(f CallTypestateRequirement) CallTypestateRequirement {
		f.Target = f.Target.Clone()
		return f
	},
}

func normalizeOutcomeTypestateRequirements(_ *axis.Registry, out *CallOutcome) {
	out.TypestateRequirements = callTypestateRequirementSet.NormalizeOwned(out.TypestateRequirements)
}

type callParamPathKey pathdom.PathKey

func callParamPathRefinementMap(reg *axis.Registry) factmap.Map[callParamPathKey, CallParamPathRefinement, product.Value] {
	return factmap.Map[callParamPathKey, CallParamPathRefinement, product.Value]{
		Key:   func(f CallParamPathRefinement) callParamPathKey { return callParamPathKey(f.Path.Key()) },
		Value: func(f CallParamPathRefinement) product.Value { return f.Value },
		WithValue: func(f CallParamPathRefinement, value product.Value) CallParamPathRefinement {
			f.Value = value
			return f
		},
		Less:      func(a, b CallParamPathRefinement) bool { return a.Path.Less(b.Path) },
		Valid:     func(f CallParamPathRefinement) bool { return f.Path.IsPlaceholder() },
		CloneFact: func(f CallParamPathRefinement) CallParamPathRefinement { f.Path = f.Path.Clone(); return f },
		Domain:    product.Domain(reg),
	}
}

func normalizeOutcomeParamPathRefinements(reg *axis.Registry, out *CallOutcome) {
	out.ParamPathRefinements = callParamPathRefinementMap(reg).NormalizeOwned(out.ParamPathRefinements)
}

func callParamPathWriteMap(reg *axis.Registry) factmap.Map[callParamPathKey, CallParamPathWrite, product.Value] {
	return factmap.Map[callParamPathKey, CallParamPathWrite, product.Value]{
		Key:       func(f CallParamPathWrite) callParamPathKey { return callParamPathKey(f.Path.Key()) },
		Value:     func(f CallParamPathWrite) product.Value { return f.Value },
		WithValue: func(f CallParamPathWrite, value product.Value) CallParamPathWrite { f.Value = value; return f },
		Less:      func(a, b CallParamPathWrite) bool { return a.Path.Less(b.Path) },
		Valid:     func(f CallParamPathWrite) bool { return f.Path.IsPlaceholder() },
		CloneFact: func(f CallParamPathWrite) CallParamPathWrite { f.Path = f.Path.Clone(); return f },
		Domain:    product.Domain(reg),
		Intersect: true,
	}
}

func normalizeOutcomeParamPathWrites(reg *axis.Registry, out *CallOutcome) {
	out.ParamPathWrites = callParamPathWriteMap(reg).NormalizeOwned(out.ParamPathWrites)
}

type callParamLengthFloorKey pathdom.PathKey

func normalizeParamLengthFloors(in []CallParamLengthFloor) []CallParamLengthFloor {
	if len(in) == 0 {
		return nil
	}
	byPath := make(map[callParamLengthFloorKey]CallParamLengthFloor, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		key := callParamLengthFloorKey(fact.Path.Key())
		if kept, ok := byPath[key]; ok && kept.Floor >= fact.Floor {
			continue
		}
		byPath[key] = fact
	}
	out := make([]CallParamLengthFloor, 0, len(byPath))
	for _, fact := range byPath {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path.Less(out[j].Path) })
	return out
}

func normalizeOutcomeParamLengthFloors(_ *axis.Registry, out *CallOutcome) {
	out.ParamLengthFloors = normalizeParamLengthFloors(out.ParamLengthFloors)
}

func equalParamLengthFloors(left, right []CallParamLengthFloor) bool {
	left, right = normalizeParamLengthFloors(left), normalizeParamLengthFloors(right)
	return equalSlice(left, right, func(a, b CallParamLengthFloor) bool { return a.Floor == b.Floor && a.Path.Equal(b.Path) })
}

func joinParamLengthFloors(left, right []CallParamLengthFloor) []CallParamLengthFloor {
	left, right = normalizeParamLengthFloors(left), normalizeParamLengthFloors(right)
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightByPath := make(map[callParamLengthFloorKey]int64, len(right))
	for _, fact := range right {
		rightByPath[callParamLengthFloorKey(fact.Path.Key())] = fact.Floor
	}
	out := make([]CallParamLengthFloor, 0, len(left))
	for _, fact := range left {
		floor, ok := rightByPath[callParamLengthFloorKey(fact.Path.Key())]
		if !ok {
			continue
		}
		if floor < fact.Floor {
			fact.Floor = floor
		}
		out = append(out, fact)
	}
	return normalizeParamLengthFloors(out)
}

type callParamPathInvalidationKey pathdom.PathKey

var callParamPathInvalidationSet = factset.Set[callParamPathInvalidationKey, CallParamPathInvalidation]{
	Key: func(f CallParamPathInvalidation) callParamPathInvalidationKey {
		return callParamPathInvalidationKey(f.Path.Key())
	},
	EqualFact: func(a, b CallParamPathInvalidation) bool {
		return a.Path.Equal(b.Path) && a.PreserveStructuralWitness == b.PreserveStructuralWitness
	},
	Less:  func(a, b CallParamPathInvalidation) bool { return a.Path.Less(b.Path) },
	Valid: func(f CallParamPathInvalidation) bool { return f.Path.IsPlaceholder() },
	CloneFact: func(f CallParamPathInvalidation) CallParamPathInvalidation {
		f.Path = f.Path.Clone()
		return f
	},
	Prefer: func(kept, incoming CallParamPathInvalidation) bool {
		return kept.PreserveStructuralWitness && !incoming.PreserveStructuralWitness
	},
	Dominates: func(super, sub CallParamPathInvalidation) bool {
		return sub.Path.HasPrefix(super.Path) && (!super.PreserveStructuralWitness || sub.PreserveStructuralWitness)
	},
}

func normalizeOutcomeParamPathInvalidations(_ *axis.Registry, out *CallOutcome) {
	out.ParamPathInvalidations = callParamPathInvalidationSet.NormalizeOwned(out.ParamPathInvalidations)
}

var callParamConditionSet = factset.Set[CallParamCondition, CallParamCondition]{
	Key:       func(f CallParamCondition) CallParamCondition { return f },
	EqualFact: func(a, b CallParamCondition) bool { return a == b },
	Less: func(a, b CallParamCondition) bool {
		return a.ParamIndex < b.ParamIndex || a.ParamIndex == b.ParamIndex && !a.Value && b.Value
	},
	Valid:     func(f CallParamCondition) bool { return f.ParamIndex >= 0 },
	Intersect: true,
}

func normalizeOutcomeParamConditions(_ *axis.Registry, out *CallOutcome) {
	out.ParamConditions = callParamConditionSet.NormalizeOwned(out.ParamConditions)
}

type callParamPathRelationKey struct {
	kind        CallPathRelationKind
	left, right pathdom.PathKey
}

var callParamPathRelationSet = factset.Set[callParamPathRelationKey, CallParamPathRelation]{
	Key: func(f CallParamPathRelation) callParamPathRelationKey {
		return callParamPathRelationKey{kind: f.Kind, left: f.Left.Key(), right: f.Right.Key()}
	},
	EqualFact: func(a, b CallParamPathRelation) bool {
		return a.Kind == b.Kind && a.Left.Equal(b.Left) && a.Right.Equal(b.Right)
	},
	Less: func(a, b CallParamPathRelation) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if !a.Left.Equal(b.Left) {
			return a.Left.Less(b.Left)
		}
		return a.Right.Less(b.Right)
	},
	Admit: func(f CallParamPathRelation) (CallParamPathRelation, bool) {
		if f.Kind != CallPathRelationEqual || !f.Left.IsPlaceholder() || !f.Right.IsPlaceholder() || f.Left.Equal(f.Right) {
			return CallParamPathRelation{}, false
		}
		if f.Right.Less(f.Left) {
			f.Left, f.Right = f.Right, f.Left
		}
		return f, true
	},
	CloneFact: func(f CallParamPathRelation) CallParamPathRelation {
		f.Left, f.Right = f.Left.Clone(), f.Right.Clone()
		return f
	},
	Intersect: true,
}

func normalizeOutcomeParamPathRelations(_ *axis.Registry, out *CallOutcome) {
	out.ParamPathRelations = callParamPathRelationSet.NormalizeOwned(out.ParamPathRelations)
}

type callReturnConditionRefinementKey struct {
	returnIndex int
	returnValue bool
	target      pathdom.PathKey
}

func callReturnConditionRefinementMap(reg *axis.Registry) factmap.Map[callReturnConditionRefinementKey, CallReturnConditionRefinement, product.Value] {
	return factmap.Map[callReturnConditionRefinementKey, CallReturnConditionRefinement, product.Value]{
		Key: func(f CallReturnConditionRefinement) callReturnConditionRefinementKey {
			return callReturnConditionRefinementKey{returnIndex: f.ReturnIndex, returnValue: f.ReturnValue, target: f.Target.Key()}
		},
		Value: func(f CallReturnConditionRefinement) product.Value { return f.Value },
		WithValue: func(f CallReturnConditionRefinement, value product.Value) CallReturnConditionRefinement {
			f.Value = value
			return f
		},
		Less: func(a, b CallReturnConditionRefinement) bool {
			if a.ReturnIndex != b.ReturnIndex {
				return a.ReturnIndex < b.ReturnIndex
			}
			if a.ReturnValue != b.ReturnValue {
				return !a.ReturnValue && b.ReturnValue
			}
			return a.Target.Less(b.Target)
		},
		Valid: func(f CallReturnConditionRefinement) bool {
			return f.ReturnIndex >= 0 && f.Target.IsPlaceholder() && usefulOutcomeRefinement(reg, f.Value)
		},
		CloneFact: func(f CallReturnConditionRefinement) CallReturnConditionRefinement {
			f.Target = f.Target.Clone()
			return f
		},
		Domain:    product.Domain(reg),
		Collide:   func(a, b product.Value) product.Value { return product.Meet(reg, a, b) },
		Intersect: true,
	}
}

func normalizeOutcomeReturnConditionRefinements(reg *axis.Registry, out *CallOutcome) {
	out.ReturnConditionRefinements = callReturnConditionRefinementMap(reg).NormalizeOwned(out.ReturnConditionRefinements)
}

type callReturnConditionSlotKey struct {
	returnIndex int
	returnValue bool
	targetIndex int
}

func callReturnConditionSlotMap(reg *axis.Registry) factmap.Map[callReturnConditionSlotKey, CallReturnConditionSlotRefinement, product.Value] {
	return factmap.Map[callReturnConditionSlotKey, CallReturnConditionSlotRefinement, product.Value]{
		Key: func(f CallReturnConditionSlotRefinement) callReturnConditionSlotKey {
			return callReturnConditionSlotKey{returnIndex: f.ReturnIndex, returnValue: f.ReturnValue, targetIndex: f.TargetIndex}
		},
		Value: func(f CallReturnConditionSlotRefinement) product.Value { return f.Value },
		WithValue: func(f CallReturnConditionSlotRefinement, value product.Value) CallReturnConditionSlotRefinement {
			f.Value = value
			return f
		},
		Less: func(a, b CallReturnConditionSlotRefinement) bool {
			if a.ReturnIndex != b.ReturnIndex {
				return a.ReturnIndex < b.ReturnIndex
			}
			if a.ReturnValue != b.ReturnValue {
				return !a.ReturnValue && b.ReturnValue
			}
			return a.TargetIndex < b.TargetIndex
		},
		Valid: func(f CallReturnConditionSlotRefinement) bool {
			return f.ReturnIndex >= 0 && f.TargetIndex >= 0 && f.ReturnIndex != f.TargetIndex && usefulOutcomeRefinement(reg, f.Value)
		},
		Domain:    product.Domain(reg),
		Collide:   func(a, b product.Value) product.Value { return product.Meet(reg, a, b) },
		Intersect: true,
	}
}

func normalizeOutcomeReturnConditionSlots(reg *axis.Registry, out *CallOutcome) {
	out.ReturnConditionSlots = callReturnConditionSlotMap(reg).NormalizeOwned(out.ReturnConditionSlots)
}

func usefulOutcomeRefinement(reg *axis.Registry, value product.Value) bool {
	return !product.Equal(reg, value, product.Bottom(reg)) && !product.Equal(reg, value, product.Top())
}

type callReturnPresenceRelationKey struct {
	triggerIndex    int
	triggerPresence presence.Value
	targetIndex     int
	targetPresence  presence.Value
}

var callReturnPresenceRelationSet = factset.Set[callReturnPresenceRelationKey, CallReturnPresenceRelation]{
	Key: callReturnPresenceKey,
	EqualFact: func(a, b CallReturnPresenceRelation) bool {
		return a.TriggerIndex == b.TriggerIndex && a.TargetIndex == b.TargetIndex &&
			presence.Equal(a.TriggerPresence, b.TriggerPresence) && presence.Equal(a.TargetPresence, b.TargetPresence)
	},
	Less: func(a, b CallReturnPresenceRelation) bool {
		ak, bk := callReturnPresenceKey(a), callReturnPresenceKey(b)
		if ak.triggerIndex != bk.triggerIndex {
			return ak.triggerIndex < bk.triggerIndex
		}
		if ak.triggerPresence != bk.triggerPresence {
			return ak.triggerPresence < bk.triggerPresence
		}
		if ak.targetIndex != bk.targetIndex {
			return ak.targetIndex < bk.targetIndex
		}
		return ak.targetPresence < bk.targetPresence
	},
	Valid: func(f CallReturnPresenceRelation) bool {
		return f.TriggerIndex >= 0 && f.TargetIndex >= 0 && f.TriggerIndex != f.TargetIndex &&
			!f.TriggerPresence.IsBottom() && !f.TriggerPresence.IsTop() &&
			!f.TargetPresence.IsBottom() && !f.TargetPresence.IsTop()
	},
	Intersect: true,
}

func callReturnPresenceKey(f CallReturnPresenceRelation) callReturnPresenceRelationKey {
	return callReturnPresenceRelationKey{
		triggerIndex: f.TriggerIndex, triggerPresence: f.TriggerPresence,
		targetIndex: f.TargetIndex, targetPresence: f.TargetPresence,
	}
}

func normalizeOutcomeReturnPresenceRelations(_ *axis.Registry, out *CallOutcome) {
	out.ReturnPresenceRelations = callReturnPresenceRelationSet.NormalizeOwned(out.ReturnPresenceRelations)
}

func equalSlice[T any](a, b []T, equal func(T, T) bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalPlacementMaps(a, b map[identity.ID]placement.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for id, value := range a {
		other, ok := b[id]
		if !ok || !placement.Equal(value, other) {
			return false
		}
	}
	return true
}
