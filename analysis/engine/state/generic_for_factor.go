package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// GenericForFactorConfig is the fully resolved, syntax-free binding geometry
// for one loop variable. Every path belongs to Keys. Invalid optional paths
// mean that the corresponding iterator theorem is unavailable; clearing the
// previous target evidence and writing the scalar result remain exact.
type GenericForFactorConfig struct {
	Keys            *keyspace.KeySpace
	Iterator        iteration.IteratorKind
	HasIterator     bool
	VariableIndex   int
	Target          keyspace.Key
	FirstTarget     keyspace.Key
	SourceContainer keyspace.Key
	SourceTable     keyspace.Key
	TypeValues      *typevalue.Cache
	registry        *axis.Registry
}

func (c GenericForFactorConfig) valid() bool {
	return c.registry != nil && c.Keys != nil && c.Keys.Valid() && c.VariableIndex >= 0 &&
		c.Target.Kind != keyspace.KindInvalid && c.Keys.FormatReadOnly(c.Target) != ""
}

func (c GenericForFactorConfig) indexedValue() bool {
	return c.HasIterator && c.Iterator == iteration.IterateIndexed && c.VariableIndex == 1
}

type genericForStaticCopy struct {
	target keyspace.Key
	value  product.Value
}

type genericForFactorEvidence struct {
	staticCopies []genericForStaticCopy
	dynamicSites []dynamicindex.Site
}

type genericForFactorObserve func(laneFactorPayload, GenericForFactorConfig, *genericForFactorEvidence) bool
type genericForFactorApply func(laneFactorPayload, GenericForFactorConfig, genericForFactorEvidence) (laneFactorPayload, bool)

type genericForFactorLane struct {
	lane         ProductLane
	sourceIndex  int
	currentIndex int
	writeIndex   int
	observe      genericForFactorObserve
	apply        genericForFactorApply
}

// GenericForFactorTransaction is the sole sparse product transaction for a
// generic-for binding. Its lane order and executable laws are frozen from the
// ProductDomain catalog; execution never scans the catalog or switches on a
// LaneID.
type GenericForFactorTransaction struct {
	domain  ProductDomain
	config  GenericForFactorConfig
	lanes   []genericForFactorLane
	source  []ProductLane
	current []ProductLane
	writes  []ProductLane
	sealed  bool
}

func (t GenericForFactorTransaction) Valid() bool {
	return t.sealed && t.domain.Valid() && t.config.valid()
}

func cloneProductLanes(in []ProductLane) []ProductLane { return append([]ProductLane(nil), in...) }

func (t GenericForFactorTransaction) SourceLanes() []ProductLane { return cloneProductLanes(t.source) }
func (t GenericForFactorTransaction) CurrentLanes() []ProductLane {
	return cloneProductLanes(t.current)
}
func (t GenericForFactorTransaction) WriteLanes() []ProductLane { return cloneProductLanes(t.writes) }

// RekeyFormal binds the already-sealed transaction geometry to the formal
// tuple keyspace through the same root substitution used by every factor. The
// executable lane program is rebuilt from the ProductDomain catalog, so
// concrete and formal execution cannot retain different role inventories.
func (t GenericForFactorTransaction) RekeyFormal(plan CoordinateFormalRootRekey) (GenericForFactorTransaction, error) {
	if !t.Valid() || !plan.validFor(t.domain) || t.config.Keys != plan.from {
		return GenericForFactorTransaction{}, fmt.Errorf("%w: generic-for formal rekey plan", ErrInvalidLaneFactor)
	}
	config := t.config
	config.Keys = plan.to
	rekey := func(source keyspace.Key) (keyspace.Key, error) {
		if source.Kind == keyspace.KindInvalid {
			return keyspace.Key{}, nil
		}
		return t.domain.RekeyStructuralKeyFormal(plan, source)
	}
	var err error
	if config.Target, err = rekey(t.config.Target); err != nil {
		return GenericForFactorTransaction{}, err
	}
	if config.FirstTarget, err = rekey(t.config.FirstTarget); err != nil {
		return GenericForFactorTransaction{}, err
	}
	if config.SourceContainer, err = rekey(t.config.SourceContainer); err != nil {
		return GenericForFactorTransaction{}, err
	}
	if config.SourceTable, err = rekey(t.config.SourceTable); err != nil {
		return GenericForFactorTransaction{}, err
	}
	return t.domain.PrepareGenericForFactorTransaction(config)
}

// PrepareGenericForFactorTransaction compiles every registered lane law once.
// A write declaration without an executable factor law is rejected here,
// before either concrete or formal execution can publish a partial result.
func (d ProductDomain) PrepareGenericForFactorTransaction(config GenericForFactorConfig) (GenericForFactorTransaction, error) {
	config.registry = d.reg
	if !d.Valid() || !config.valid() {
		return GenericForFactorTransaction{}, fmt.Errorf("%w: invalid generic-for factor transaction", ErrInvalidLaneFactor)
	}
	request := genericForBindingRequest{indexedValue: config.indexedValue()}
	out := GenericForFactorTransaction{domain: d, config: config}
	for i := range d.factorLanes {
		runtime := &d.factorLanes[i]
		law, ok := findLaneSemanticLaw(runtime.semanticLaws, request.semanticCapabilityID())
		if !ok || law.genericForBinding == nil {
			return GenericForFactorTransaction{}, fmt.Errorf("state: lane %q has no complete generic-for binding law", runtime.lane.ID())
		}
		binding := law.genericForBinding(request)
		if !binding.sourceRead && !binding.currentRead && !binding.write {
			continue
		}
		lane := genericForFactorLane{lane: runtime.lane, sourceIndex: -1, currentIndex: -1, writeIndex: -1, observe: binding.observe, apply: binding.apply}
		if binding.sourceRead {
			lane.sourceIndex = len(out.source)
			out.source = append(out.source, runtime.lane)
		}
		if binding.currentRead {
			lane.currentIndex = len(out.current)
			out.current = append(out.current, runtime.lane)
		}
		if binding.write {
			if binding.currentRead == false || binding.apply == nil {
				return GenericForFactorTransaction{}, fmt.Errorf("state: lane %q has an incomplete generic-for factor write law", runtime.lane.ID())
			}
			lane.writeIndex = len(out.writes)
			out.writes = append(out.writes, runtime.lane)
		}
		out.lanes = append(out.lanes, lane)
	}
	out.sealed = true
	return out, nil
}

// Apply executes against only the declared factor rows. Outputs are returned
// in WriteLanes order. All results are staged before return, so cancellation or
// validation failure leaves both caller rows untouched.
func (t GenericForFactorTransaction) Apply(source, current []LaneFactor) ([]LaneFactor, error) {
	if !t.Valid() || len(source) != len(t.source) || len(current) != len(t.current) {
		return nil, fmt.Errorf("%w: malformed generic-for factor operands", ErrInvalidLaneFactor)
	}
	for i, factor := range source {
		if _, err := t.domain.validateFactorFor(&t.domain.factorLanes[t.source[i].ordinal], factor); err != nil {
			return nil, err
		}
	}
	for i, factor := range current {
		if _, err := t.domain.validateFactorFor(&t.domain.factorLanes[t.current[i].ordinal], factor); err != nil {
			return nil, err
		}
	}
	evidence := genericForFactorEvidence{}
	for _, lane := range t.lanes {
		if lane.sourceIndex < 0 || lane.observe == nil {
			continue
		}
		if !lane.observe(source[lane.sourceIndex].payload, t.config, &evidence) {
			return nil, fmt.Errorf("state: lane %q rejected generic-for source factor", lane.lane.ID())
		}
	}
	staged := make([]LaneFactor, len(t.writes))
	for _, lane := range t.lanes {
		if lane.writeIndex < 0 {
			continue
		}
		payload, ok := lane.apply(current[lane.currentIndex].payload, t.config, evidence)
		if !ok {
			return nil, fmt.Errorf("state: lane %q rejected generic-for factor transaction", lane.lane.ID())
		}
		staged[lane.writeIndex] = LaneFactor{lane: lane.lane, payload: payload}
	}
	return staged, nil
}

func genericForPathEvidenceBinding() laneSemanticLaw {
	return genericForBindingWithFactors(true, func(request genericForBindingRequest) genericForLaneBinding {
		if !request.indexedValue {
			return genericForLaneBinding{}
		}
		return genericForLaneBinding{
			sourceRead: true, currentRead: true, write: true,
			observe: observeGenericForPathEvidence,
			apply:   applyGenericForPathEvidence,
		}
	})
}

func observeGenericForPathEvidence(payload laneFactorPayload, config GenericForFactorConfig, evidence *genericForFactorEvidence) bool {
	if evidence == nil || !config.indexedValue() || config.SourceContainer.Kind == keyspace.KindInvalid || config.Target.Kind == keyspace.KindInvalid {
		return evidence != nil
	}
	lane := typedLaneFactorValue[pathevidence.Lane](payload)
	elements := make(map[segment.Segment]struct{})
	type candidate struct {
		value product.Value
		seen  map[segment.Segment]struct{}
	}
	copies := make(map[keyspace.Key]candidate)
	lane.ForEachPathStaticMember(func(member keyspace.Key, value product.Value) bool {
		remainder, ok := config.Keys.ExactRemainderAfterPrefix(member, config.SourceContainer)
		if !ok || len(remainder) == 0 || remainder[0].Kind != segment.SegmentIndexInt {
			return true
		}
		element := remainder[0]
		elements[element] = struct{}{}
		if len(remainder) == 1 || presence.Equal(product.PresenceOf(value), presence.Absent()) {
			return true
		}
		suffix, ok := config.Keys.FromRootlessSuffix(remainder[1:])
		if !ok {
			return true
		}
		copy := copies[suffix]
		if copy.seen == nil {
			copy.value, copy.seen = value, make(map[segment.Segment]struct{}, 1)
		} else if _, duplicate := copy.seen[element]; !duplicate {
			copy.value = product.Join(config.registry, copy.value, value)
		}
		copy.seen[element] = struct{}{}
		copies[suffix] = copy
		return true
	})
	for suffix, copy := range copies {
		if len(copy.seen) != len(elements) || len(elements) == 0 {
			continue
		}
		target, ok := appendGenericForSegments(config.Keys, config.Target, config.Keys.Segments(suffix))
		if ok {
			evidence.staticCopies = append(evidence.staticCopies, genericForStaticCopy{target: target, value: copy.value})
		}
	}
	sort.Slice(evidence.staticCopies, func(i, j int) bool {
		return config.Keys.Less(evidence.staticCopies[i].target, evidence.staticCopies[j].target)
	})
	return true
}

func appendGenericForSegments(keys *keyspace.KeySpace, root keyspace.Key, segments []segment.Segment) (keyspace.Key, bool) {
	current := root
	for _, item := range segments {
		var ok bool
		current, ok = keys.AppendSegment(current, item)
		if !ok {
			return keyspace.Key{}, false
		}
	}
	return current, true
}

func applyGenericForPathEvidence(payload laneFactorPayload, _ GenericForFactorConfig, evidence genericForFactorEvidence) (laneFactorPayload, bool) {
	lane := typedLaneFactorValue[pathevidence.Lane](payload)
	for _, copy := range evidence.staticCopies {
		var reachable bool
		lane, reachable = lane.WritePathStaticMember(copy.target, copy.value)
		if !reachable {
			return payload, false
		}
	}
	return typedLaneFactorPayload[pathevidence.Lane]{value: lane}, true
}

func genericForDynamicIndexBinding() laneSemanticLaw {
	return genericForBindingWithFactors(true, func(request genericForBindingRequest) genericForLaneBinding {
		if !request.indexedValue {
			return genericForLaneBinding{}
		}
		return genericForLaneBinding{sourceRead: true, observe: observeGenericForDynamicIndex}
	})
}

func observeGenericForDynamicIndex(payload laneFactorPayload, config GenericForFactorConfig, evidence *genericForFactorEvidence) bool {
	if evidence == nil || !config.indexedValue() || config.SourceContainer.Kind == keyspace.KindInvalid {
		return evidence != nil
	}
	lane := typedLaneFactorValue[dynamicIndexLane](payload)
	if lane.top {
		return true
	}
	seen := make(map[dynamicindex.Site]struct{})
	for key, fact := range lane.values {
		if key.Table != config.SourceContainer || fact.Admission == dynamicindex.AdmissionRejected || presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			continue
		}
		if config.TypeValues == nil {
			continue
		}
		keyType, ok := config.TypeValues.TypeOf(config.registry, fact.KeyValue)
		if !ok || !typ.IsIntegerIndexType(keyType) {
			continue
		}
		if _, duplicate := seen[key.Site]; duplicate {
			continue
		}
		seen[key.Site] = struct{}{}
		evidence.dynamicSites = append(evidence.dynamicSites, key.Site)
	}
	sort.Slice(evidence.dynamicSites, func(i, j int) bool { return evidence.dynamicSites[i] < evidence.dynamicSites[j] })
	return true
}

func genericForKeyMembershipBinding() laneSemanticLaw {
	return genericForBindingWithFactors(true, func(genericForBindingRequest) genericForLaneBinding {
		return genericForLaneBinding{currentRead: true, write: true, apply: applyGenericForKeyMemberships}
	})
}

func applyGenericForKeyMemberships(payload laneFactorPayload, config GenericForFactorConfig, evidence genericForFactorEvidence) (laneFactorPayload, bool) {
	lane := typedLaneFactorValue[keyMembershipLane](payload)
	target, targetOK := pathaddr.StateKeyFromPathKey(config.Keys.FormatReadOnly(config.Target))
	if !targetOK {
		return payload, false
	}
	var changed bool
	lane, _ = lane.clearMatching(func(m KeyMembership) bool { return m.Key == target || m.Table == target })
	if !config.HasIterator {
		return typedLaneFactorPayload[keyMembershipLane]{value: lane}, true
	}
	switch config.Iterator {
	case iteration.IterateKeyed:
		if config.VariableIndex == 0 && config.SourceTable.Kind != keyspace.KindInvalid {
			table, tableOK := pathaddr.StateKeyFromPathKey(config.Keys.FormatReadOnly(config.SourceTable))
			if tableOK {
				lane, changed = lane.add(PathKeyMembership(target, table))
				_ = changed
			}
		} else if config.VariableIndex == 1 && config.SourceContainer.Kind != keyspace.KindInvalid && config.FirstTarget.Kind != keyspace.KindInvalid {
			first, firstOK := pathaddr.StateKeyFromPathKey(config.Keys.FormatReadOnly(config.FirstTarget))
			if firstOK {
				lane, _ = lane.addReadOrigin(DynamicIndexReadOrigin{Value: target, Container: config.SourceContainer, Key: first})
			}
		}
	case iteration.IterateIndexed:
		if config.VariableIndex == 1 && config.SourceContainer.Kind != keyspace.KindInvalid {
			for _, site := range evidence.dynamicSites {
				origin := DynamicIndexValueOrigin{Value: target, Container: config.SourceContainer, Site: site}
				if _, exists := lane.valueOrigins[origin]; exists {
					continue
				}
				lane = lane.reachable()
				lane.valueOrigins = mapCloneGenericFor(lane.valueOrigins)
				lane.valueOrigins[origin] = struct{}{}
			}
		}
	}
	return typedLaneFactorPayload[keyMembershipLane]{value: lane}, true
}

func mapCloneGenericFor[K comparable](in map[K]struct{}) map[K]struct{} {
	out := make(map[K]struct{}, len(in)+1)
	for value := range in {
		out[value] = struct{}{}
	}
	return out
}
