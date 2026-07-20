package state

import (
	"fmt"
	"sort"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type rootAssignmentDynamicSourceInputRole uint8

const (
	rootAssignmentDynamicSourceInputIndependent rootAssignmentDynamicSourceInputRole = iota
	rootAssignmentDynamicSourceInputFacts
	rootAssignmentDynamicSourceInputMemberships
)

// RootAssignmentDynamicSourceDependencies is the ProductDomain-sealed input
// topology for resolving a dynamic-index root-assignment sidecar. Lanes remain
// in catalog order while semantic roles remain registration-owned.
type RootAssignmentDynamicSourceDependencies struct {
	seal             *productDomainSeal
	lanes            []ProductLane
	factsIndex       int
	membershipsIndex int
}

// SealRootAssignmentDynamicSourceDependencies freezes the exact registered
// factor inputs. A selected product missing either role fails closed.
func (d ProductDomain) SealRootAssignmentDynamicSourceDependencies() (RootAssignmentDynamicSourceDependencies, error) {
	if !d.Valid() {
		return RootAssignmentDynamicSourceDependencies{}, fmt.Errorf("%w: invalid dynamic-source dependencies", ErrInvalidLaneFactor)
	}
	dependencies := RootAssignmentDynamicSourceDependencies{seal: d.seal, factsIndex: -1, membershipsIndex: -1}
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		role := runtime.rootAssignment.dynamicSourceInput
		if role == rootAssignmentDynamicSourceInputIndependent {
			continue
		}
		ordinal := len(dependencies.lanes)
		dependencies.lanes = append(dependencies.lanes, runtime.lane)
		switch role {
		case rootAssignmentDynamicSourceInputFacts:
			if dependencies.factsIndex >= 0 {
				return RootAssignmentDynamicSourceDependencies{}, fmt.Errorf("%w: duplicate dynamic-source facts input", ErrInvalidLaneFactor)
			}
			dependencies.factsIndex = ordinal
		case rootAssignmentDynamicSourceInputMemberships:
			if dependencies.membershipsIndex >= 0 {
				return RootAssignmentDynamicSourceDependencies{}, fmt.Errorf("%w: duplicate dynamic-source memberships input", ErrInvalidLaneFactor)
			}
			dependencies.membershipsIndex = ordinal
		default:
			return RootAssignmentDynamicSourceDependencies{}, fmt.Errorf("%w: unknown dynamic-source input role", ErrInvalidLaneFactor)
		}
	}
	if dependencies.factsIndex < 0 || dependencies.membershipsIndex < 0 {
		return RootAssignmentDynamicSourceDependencies{}, fmt.Errorf("%w: incomplete dynamic-source input roles", ErrIncompleteLaneFactors)
	}
	return dependencies, nil
}

// InputLanes returns the dependency factors in exact product order.
func (d RootAssignmentDynamicSourceDependencies) InputLanes() []ProductLane {
	return cloneRootAssignmentProductLanes(d.lanes)
}

// OwnsRootAssignmentDynamicSourceDependencies reports whether dependencies
// were sealed by this exact product instance.
func (d ProductDomain) OwnsRootAssignmentDynamicSourceDependencies(dependencies RootAssignmentDynamicSourceDependencies) bool {
	return d.Valid() && dependencies.seal != nil && dependencies.seal == d.seal &&
		dependencies.factsIndex >= 0 && dependencies.factsIndex < len(dependencies.lanes) &&
		dependencies.membershipsIndex >= 0 && dependencies.membershipsIndex < len(dependencies.lanes)
}

// RootAssignmentDynamicSourceFactorInputs is one validated factor row bound
// through a sealed dependency descriptor.
type RootAssignmentDynamicSourceFactorInputs struct {
	seal           *productDomainSeal
	dynamicFacts   LaneFactor
	keyMemberships LaneFactor
}

// BindRootAssignmentDynamicSourceInputs validates exact factor order and maps
// registered roles without inspecting lane names.
func (d ProductDomain) BindRootAssignmentDynamicSourceInputs(
	dependencies RootAssignmentDynamicSourceDependencies,
	factors []LaneFactor,
) (RootAssignmentDynamicSourceFactorInputs, error) {
	if !d.OwnsRootAssignmentDynamicSourceDependencies(dependencies) || len(factors) != len(dependencies.lanes) {
		return RootAssignmentDynamicSourceFactorInputs{}, fmt.Errorf("%w: incomplete dynamic-source input factors", ErrIncompleteLaneFactors)
	}
	for index := range factors {
		runtime, err := d.validateFactor(factors[index])
		if err != nil || runtime.lane != dependencies.lanes[index] {
			return RootAssignmentDynamicSourceFactorInputs{}, fmt.Errorf("%w: invalid dynamic-source input factor order", ErrInvalidLaneFactor)
		}
	}
	return RootAssignmentDynamicSourceFactorInputs{
		seal: d.seal, dynamicFacts: factors[dependencies.factsIndex], keyMemberships: factors[dependencies.membershipsIndex],
	}, nil
}

// DynamicIndexFactor returns the registered dynamic-fact operand.
func (i RootAssignmentDynamicSourceFactorInputs) DynamicIndexFactor() (LaneFactor, bool) {
	return i.dynamicFacts, i.seal != nil && i.dynamicFacts.payload != nil
}

// KeyMembershipFactor returns the registered membership operand.
func (i RootAssignmentDynamicSourceFactorInputs) KeyMembershipFactor() (LaneFactor, bool) {
	return i.keyMemberships, i.seal != nil && i.keyMemberships.payload != nil
}

// RootAssignmentDynamicSourceConfig is the finite must-evidence published by
// assigning from a dynamic index. ReadOrigins and KeyMemberships are kept in
// one transaction because concrete N4 publishes both even when its root value
// is not yet productive.
type RootAssignmentDynamicSourceConfig struct {
	ReadOrigins    []DynamicIndexReadOrigin
	KeyMemberships []KeyMembership
}

// RootAssignmentDynamicSourceTransaction is an immutable ProductDomain-owned
// key-membership delta. It contains no State and is safe to retain in guarded
// execution.
type RootAssignmentDynamicSourceTransaction struct {
	seal           *productDomainSeal
	readOrigins    []DynamicIndexReadOrigin
	keyMemberships []KeyMembership
}

// SealRootAssignmentDynamicSource validates, deduplicates, and binds one
// dynamic-source delta to this exact registered product.
func (d ProductDomain) SealRootAssignmentDynamicSource(config RootAssignmentDynamicSourceConfig) (RootAssignmentDynamicSourceTransaction, error) {
	if !d.Valid() {
		return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("state: invalid root-assignment dynamic-source domain")
	}
	origins := make([]DynamicIndexReadOrigin, 0, len(config.ReadOrigins))
	seenOrigins := make(map[DynamicIndexReadOrigin]struct{}, len(config.ReadOrigins))
	for index, origin := range config.ReadOrigins {
		if !origin.valid() {
			return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("state: invalid root-assignment dynamic read origin %d", index)
		}
		if _, duplicate := seenOrigins[origin]; duplicate {
			continue
		}
		seenOrigins[origin] = struct{}{}
		origins = append(origins, origin)
	}
	memberships := make([]KeyMembership, 0, len(config.KeyMemberships))
	seenMemberships := make(map[KeyMembership]struct{}, len(config.KeyMemberships))
	for index, membership := range config.KeyMemberships {
		if !membership.valid() {
			return RootAssignmentDynamicSourceTransaction{}, fmt.Errorf("state: invalid root-assignment dynamic key membership %d", index)
		}
		if _, duplicate := seenMemberships[membership]; duplicate {
			continue
		}
		seenMemberships[membership] = struct{}{}
		memberships = append(memberships, membership)
	}
	return RootAssignmentDynamicSourceTransaction{
		seal: d.seal, readOrigins: origins, keyMemberships: memberships,
	}, nil
}

func (d ProductDomain) ownsRootAssignmentDynamicSource(transaction RootAssignmentDynamicSourceTransaction) bool {
	return d.Valid() && transaction.seal != nil && transaction.seal == d.seal
}

func applyRootAssignmentDynamicSourceLane(lane keyMembershipLane, transaction RootAssignmentDynamicSourceTransaction) (keyMembershipLane, bool) {
	changed := false
	for _, origin := range transaction.readOrigins {
		var added bool
		lane, added = lane.addReadOrigin(origin)
		changed = changed || added
	}
	for _, membership := range transaction.keyMemberships {
		var added bool
		lane, added = lane.add(membership)
		changed = changed || added
	}
	return lane, changed
}

func withRootAssignmentDynamicSourceLaw(policy rootAssignmentLanePolicy) rootAssignmentLanePolicy {
	policy.dynamicSource = true
	policy.dynamicSourceInput = rootAssignmentDynamicSourceInputMemberships
	policy.applyDynamicSourceState = func(out *State, transaction RootAssignmentDynamicSourceTransaction) bool {
		next, changed := applyRootAssignmentDynamicSourceLane(out.keyMemberships, transaction)
		if changed {
			out.keyMemberships = next
		}
		return changed
	}
	policy.applyDynamicSourceFactor = func(current laneFactorPayload, transaction RootAssignmentDynamicSourceTransaction) (laneFactorPayload, bool) {
		next, changed := applyRootAssignmentDynamicSourceLane(typedLaneFactorValue[keyMembershipLane](current), transaction)
		if !changed {
			return current, false
		}
		return typedLaneFactorPayload[keyMembershipLane]{value: next}, true
	}
	return policy
}

func withRootAssignmentDynamicSourceFactsInput(policy rootAssignmentLanePolicy) rootAssignmentLanePolicy {
	policy.dynamicSourceInput = rootAssignmentDynamicSourceInputFacts
	return policy
}

// RootAssignmentDynamicSourceLanes returns the registered factor owner of the
// dynamic-source delta. The inventory is registration-derived.
func (d ProductDomain) RootAssignmentDynamicSourceLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 1)
	for index := range d.factorLanes {
		if d.factorLanes[index].rootAssignment.dynamicSource {
			out = append(out, d.factorLanes[index].lane)
		}
	}
	return out
}

// ApplyRootAssignmentDynamicSource applies the same registered per-lane law
// used by factored execution to a concrete product value.
func (d ProductDomain) ApplyRootAssignmentDynamicSource(transaction RootAssignmentDynamicSourceTransaction, current State) (State, error) {
	if !d.ownsRootAssignmentDynamicSource(transaction) {
		return State{}, fmt.Errorf("state: foreign root-assignment dynamic-source transaction")
	}
	out := d.Normalize(current)
	for index := range d.factorLanes {
		apply := d.factorLanes[index].rootAssignment.applyDynamicSourceState
		if apply == nil {
			return State{}, fmt.Errorf("state: lane %q has no root-assignment dynamic-source law", d.factorLanes[index].lane.id)
		}
		apply(&out, transaction)
	}
	return out, nil
}

// ApplyRootAssignmentDynamicSourceFactor applies one registered lane law. A
// non-participating or already-satisfied factor is returned verbatim.
func (d ProductDomain) ApplyRootAssignmentDynamicSourceFactor(transaction RootAssignmentDynamicSourceTransaction, current LaneFactor) (LaneFactor, error) {
	if !d.ownsRootAssignmentDynamicSource(transaction) {
		return LaneFactor{}, fmt.Errorf("%w: foreign root-assignment dynamic-source transaction", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	apply := runtime.rootAssignment.applyDynamicSourceFactor
	if apply == nil {
		return LaneFactor{}, fmt.Errorf("%w: lane %q has no root-assignment dynamic-source law", ErrInvalidLaneFactor, runtime.lane.id)
	}
	next, changed := apply(current.payload, transaction)
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}

// RootAssignmentDynamicSourceCommonTables derives the must tables shared by
// every admitted present dynamic value source of container. Both operands are
// explicit registered factors; the query never materializes State or clones a
// lane snapshot.
func (d ProductDomain) RootAssignmentDynamicSourceCommonTables(
	container keyspace.Key,
	dynamicFacts LaneFactor,
	keyMemberships LaneFactor,
) ([]pathaddr.StateKey, error) {
	tables, _, err := d.rootAssignmentDynamicSourceCommonTables(container, dynamicFacts, keyMemberships)
	return tables, err
}

func (d ProductDomain) rootAssignmentDynamicSourceCommonTables(
	container keyspace.Key,
	dynamicFacts LaneFactor,
	keyMemberships LaneFactor,
) ([]pathaddr.StateKey, int, error) {
	if container.Kind == keyspace.KindInvalid {
		return nil, 0, fmt.Errorf("%w: invalid dynamic-source container", ErrInvalidLaneFactor)
	}
	dynamicRuntime, err := d.validateFactor(dynamicFacts)
	if err != nil || dynamicRuntime.rootAssignment.dynamicSourceInput != rootAssignmentDynamicSourceInputFacts {
		return nil, 0, fmt.Errorf("%w: dynamic-source facts operand", ErrInvalidLaneFactor)
	}
	membershipRuntime, err := d.validateFactor(keyMemberships)
	if err != nil || membershipRuntime.rootAssignment.dynamicSourceInput != rootAssignmentDynamicSourceInputMemberships {
		return nil, 0, fmt.Errorf("%w: dynamic-source memberships operand", ErrInvalidLaneFactor)
	}
	dynamicLane := typedLaneFactorValue[dynamicIndexLane](dynamicFacts.payload)
	membershipLane := typedLaneFactorValue[keyMembershipLane](keyMemberships.payload)
	visits := 0

	allValues := make([]pathaddr.StateKey, 0)
	for membership := range membershipLane.dynamicAll {
		visits++
		if membership.Container == container {
			allValues = append(allValues, membership.Table)
		}
	}
	if len(allValues) != 0 {
		sort.Slice(allValues, func(i, j int) bool { return allValues[i] < allValues[j] })
		return allValues, visits, nil
	}
	if membershipLane.bottom || membershipLane.dynamicTop || dynamicLane.top || len(dynamicLane.values) == 0 {
		return nil, visits, nil
	}

	bySite := make(map[dynamicindex.Site][]pathaddr.StateKey)
	for membership := range membershipLane.dynamic {
		visits++
		if membership.Container == container {
			bySite[membership.Site] = append(bySite[membership.Site], membership.Table)
		}
	}
	domain := product.Domain(d.reg)
	common := make(map[pathaddr.StateKey]struct{})
	found := false
	for dynamicKey, fact := range dynamicLane.values {
		visits++
		if dynamicKey.Table != container || fact.Admission == dynamicindex.AdmissionRejected ||
			domain.Equal(fact.Value, domain.Bottom()) || presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			continue
		}
		tables := bySite[dynamicKey.Site]
		if len(tables) == 0 {
			return nil, visits, nil
		}
		if !found {
			for _, table := range tables {
				visits++
				common[table] = struct{}{}
			}
			found = true
			continue
		}
		next := make(map[pathaddr.StateKey]struct{}, len(common))
		for _, table := range tables {
			visits++
			if _, ok := common[table]; ok {
				next[table] = struct{}{}
			}
		}
		common = next
		if len(common) == 0 {
			return nil, visits, nil
		}
	}
	if !found {
		return nil, visits, nil
	}
	out := make([]pathaddr.StateKey, 0, len(common))
	for table := range common {
		out = append(out, table)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, visits, nil
}
