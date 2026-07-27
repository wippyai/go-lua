package state

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// ValueRefinementCoordinateKind is the closed storage role of one exact
// descendant fact retained while a root is narrowed. It is deliberately not
// an axis tag: the unique registered path family classifies its own opaque
// coordinates while sealing ValueRefinementPlan.
type ValueRefinementCoordinateKind uint8

const (
	ValueRefinementCoordinateInvalid ValueRefinementCoordinateKind = iota
	ValueRefinementCoordinatePath
	ValueRefinementCoordinateStaticMember
)

// ValueRefinementCoordinate is one ProductDomain-owned descendant value
// coordinate. Consumers may route its value through the matching carrier
// operation, but cannot manufacture or reinterpret its opaque slot.
type ValueRefinementCoordinate struct {
	slot CoordinateSlot
	path keyspace.Key
	kind ValueRefinementCoordinateKind
}

// ValueRefinementDescendantFact is one exact pre-invalidation candidate. The
// coordinate stays opaque; factapply decides only whether the product value is
// compatible with the narrowed root and returns accepted facts for atomic
// restoration through ProductDomain.
type ValueRefinementDescendantFact struct {
	coordinate ValueRefinementCoordinate
	value      product.Value
}

func (f ValueRefinementDescendantFact) Coordinate() ValueRefinementCoordinate {
	return f.coordinate
}
func (f ValueRefinementDescendantFact) Value() product.Value { return f.value }

// WithValueRefinementDescendantValue replaces only the scalar carried by a
// plan-owned fact. The opaque coordinate cannot be forged or reinterpreted by
// the semantic kernel.
func (d ProductDomain) WithValueRefinementDescendantValue(
	plan ValueRefinementPlan,
	fact ValueRefinementDescendantFact,
	value product.Value,
) (ValueRefinementDescendantFact, error) {
	if !plan.ValidFor(d) || !product.BelongsToRegistry(d.reg, value) {
		return ValueRefinementDescendantFact{}, fmt.Errorf("%w: invalid value-refinement descendant value", ErrInvalidLaneFactor)
	}
	for _, candidate := range plan.descendants {
		equal, err := d.CoordinateSlotEqual(candidate.slot, fact.coordinate.slot)
		if err == nil && equal && candidate.path == fact.coordinate.path && candidate.kind == fact.coordinate.kind {
			return ValueRefinementDescendantFact{coordinate: candidate, value: value}, nil
		}
	}
	return ValueRefinementDescendantFact{}, fmt.Errorf("%w: foreign value-refinement descendant coordinate", ErrInvalidLaneFactor)
}

func (c ValueRefinementCoordinate) Slot() CoordinateSlot { return c.slot }
func (c ValueRefinementCoordinate) Path() keyspace.Key   { return c.path }
func (c ValueRefinementCoordinate) Kind() ValueRefinementCoordinateKind {
	return c.kind
}

// ValueRefinementPlan is the frozen physical topology of one general value
// refinement. It owns the exact Values root, path coordinates eligible for
// compatible preservation, and heap-root coordinate lookup. Runtime Apply is
// therefore width-proportional to this certified cone and never scans an axis
// inventory or decodes a coordinate kind.
type ValueRefinementPlan struct {
	seal                *productDomainSeal
	keys                *keyspace.KeySpace
	target              keyspace.Key
	root                keyspace.Key
	rootValue           statekey.ValueDependency
	anchors             []keyspace.Key
	coordinates         []CoordinateSlot
	reads               []CoordinateSlot
	writes              []CoordinateSlot
	factorReads         []CoordinateSlot
	readInventory       CoordinateFactorInventory
	writeInventory      CoordinateFactorInventory
	factorReadInventory CoordinateFactorInventory
	regions             []CoordinateDependencyLocation
	descendants         []ValueRefinementCoordinate
	heapRoots           map[identity.Term]valueRefinementHeapRootBinding
	inventory           CoordinateFactorInventory
}

type valueRefinementHeapRootBinding struct {
	slot HeapObjectRootSlot
}

func (p ValueRefinementPlan) ValidFor(d ProductDomain) bool {
	return d.Valid() && p.seal == d.seal && p.keys != nil && p.keys.Valid() &&
		p.target.Kind != keyspace.KindInvalid && p.root.Kind != keyspace.KindInvalid && p.rootValue.Valid() &&
		p.inventory.ValidFor(d, p.keys)
}

func (p ValueRefinementPlan) KeySpace() *keyspace.KeySpace { return p.keys }
func (p ValueRefinementPlan) Target() keyspace.Key         { return p.target }
func (p ValueRefinementPlan) Root() keyspace.Key           { return p.root }
func (p ValueRefinementPlan) RootValue() statekey.ValueDependency {
	return p.rootValue
}
func (p ValueRefinementPlan) Anchors() []keyspace.Key {
	return append([]keyspace.Key(nil), p.anchors...)
}
func (p ValueRefinementPlan) Descendants() []ValueRefinementCoordinate {
	return append([]ValueRefinementCoordinate(nil), p.descendants...)
}
func (p ValueRefinementPlan) Coordinates() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.coordinates...)
}
func (p ValueRefinementPlan) CoordinateReads() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.reads...)
}
func (p ValueRefinementPlan) CoordinateWrites() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.writes...)
}
func (p ValueRefinementPlan) FactorCoordinateReads() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.factorReads...)
}
func (p ValueRefinementPlan) MutationRegions() []CoordinateDependencyLocation {
	return append([]CoordinateDependencyLocation(nil), p.regions...)
}
func (p ValueRefinementPlan) CoordinateInventory() CoordinateFactorInventory { return p.inventory }
func (p ValueRefinementPlan) CoordinateReadInventory() CoordinateFactorInventory {
	return p.readInventory
}
func (p ValueRefinementPlan) CoordinateWriteInventory() CoordinateFactorInventory {
	return p.writeInventory
}
func (p ValueRefinementPlan) FactorCoordinateReadInventory() CoordinateFactorInventory {
	return p.factorReadInventory
}

// HeapRootSlot resolves an exact runtime/relational heap identity through the
// freeze-time coordinate table and retains identity.Term rather than
// collapsing formal identities.
func (p ValueRefinementPlan) HeapRootSlot(id identity.Term) (HeapObjectRootSlot, bool) {
	binding, ok := p.heapRoots[id]
	return binding.slot, ok
}

// SealValueRefinementPlan classifies the complete coordinate universe once
// through the registered path family. Only strict descendants of target's structural
// root can participate in preservation. Heap roots are likewise validated and
// indexed once; duplicate identities are rejected rather than made order
// dependent.
func (d ProductDomain) SealValueRefinementPlan(
	keys *keyspace.KeySpace,
	target keyspace.Key,
	inventory CoordinateFactorInventory,
) (ValueRefinementPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || target.Kind == keyspace.KindInvalid ||
		!inventory.ValidFor(d, keys) {
		return ValueRefinementPlan{}, fmt.Errorf("%w: invalid value-refinement topology", ErrInvalidLaneFactor)
	}
	root, ok := keys.StructuralRoot(target)
	if !ok {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement target has no structural root", ErrInvalidLaneFactor)
	}
	rootValue, ok := pathevidence.PathValueDependency(keys, root)
	if !ok {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement root has no Values dependency", ErrInvalidLaneFactor)
	}
	owner, ok := d.PathEvidenceCoordinateFamily()
	if !ok {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement path family is absent", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	if err != nil {
		return ValueRefinementPlan{}, err
	}
	closedInventory, err := d.CloseCoordinateFactorInventory(keys, inventory)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement coordinate universe closure", err)
	}
	union, err := closedInventory.FamilySlots(owner)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement path coordinate universe", err)
	}
	seed := CoordinateDependencySeed{
		ID:                      1,
		ResolvePaths:            []keyspace.Key{target},
		DescendantMutationRoots: []keyspace.Key{root},
	}
	if target != root {
		segments, exact := keys.SegmentsView(target)
		if !exact || len(segments) == 0 {
			return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement target has no structural suffix", ErrInvalidLaneFactor)
		}
		anchor := root
		for _, member := range segments {
			anchor, exact = keys.AppendSegment(anchor, member)
			if !exact || anchor.Kind == keyspace.KindInvalid {
				return ValueRefinementPlan{}, fmt.Errorf("%w: invalid value-refinement anchor", ErrInvalidLaneFactor)
			}
			targetSlot, slotErr := d.PathRefinementCoordinateSlot(keys, anchor)
			if slotErr != nil {
				return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement anchor coordinate", slotErr)
			}
			seed.WritePaths = append(seed.WritePaths, anchor)
			seed.AddCoordinates = append(seed.AddCoordinates, targetSlot)
		}
	}
	dependencies, dependencyErr := d.PlanPathCoordinateDependencies(keys, union, []CoordinateDependencySeed{seed})
	if dependencyErr != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement dependency closure", dependencyErr)
	}
	dependency, present := dependencies.Dependency(seed.ID)
	if !present {
		return ValueRefinementPlan{}, fmt.Errorf("%w: missing value-refinement dependency certificate", ErrInvalidLaneFactor)
	}
	out := ValueRefinementPlan{
		seal: d.seal, keys: keys, target: target, root: root, rootValue: rootValue,
		anchors:     append([]keyspace.Key(nil), seed.WritePaths...),
		coordinates: dependencies.Coordinates(), reads: dependency.CoordinateReads(), writes: dependency.CoordinateWrites(),
		regions: dependency.MutationRegions(),
	}
	out.readInventory, err = d.SealCoordinateFactorInventory(keys, out.reads)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement read inventory", err)
	}
	out.writeInventory, err = d.SealCoordinateFactorInventory(keys, out.writes)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement write inventory", err)
	}
	dependencyInventory, err := d.SealCoordinateFactorInventory(keys, out.coordinates)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement dependency inventory", err)
	}
	out.inventory, err = d.UnionCoordinateFactorInventories(keys, closedInventory, dependencyInventory)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement coordinate universe", err)
	}
	out.inventory, err = d.CloseCoordinateFactorInventory(keys, out.inventory)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement completed coordinate universe", err)
	}
	// The factor program observes the path owner plus every coordinate-backed
	// participant in the registered descendant-mutation tuple directly. Carry
	// their exact sealed slots in the read footprint so adapters cannot replace
	// a real scalar with its authorized default.
	topology, err := d.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return ValueRefinementPlan{}, err
	}
	directReads := make([]CoordinateSlot, 0)
	for _, family := range topology.Families() {
		slots, slotsErr := out.inventory.FamilySlots(family)
		if slotsErr != nil {
			return ValueRefinementPlan{}, slotsErr
		}
		directReads = append(directReads, slots...)
	}
	readInventory, err := d.SealCoordinateFactorInventory(keys, directReads)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement direct read footprint", err)
	}
	out.factorReads = readInventory.Slots()
	out.factorReadInventory = readInventory
	heapRoots, err := d.HeapObjectRootSlotsFromCoordinateInventory(out.inventory)
	if err != nil {
		return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement heap-root inventory", err)
	}
	out.heapRoots = make(map[identity.Term]valueRefinementHeapRootBinding, len(heapRoots))
	for _, slot := range out.coordinates {
		if slot.family != owner || slot.keys != keys {
			continue
		}
		if err := d.validateCoordinateSlotFor(coordinate, slot, keys); err != nil {
			return ValueRefinementPlan{}, fmt.Errorf("%w: value-refinement descendant coordinate", err)
		}
		descriptor, valid := pathevidence.DescribeCoordinate(pathEvidenceCoordinateKey(slot.key))
		if !valid || descriptor.Path.Kind == keyspace.KindInvalid || !keys.HasStrictPrefix(descriptor.Path, root) {
			continue
		}
		kind := ValueRefinementCoordinateInvalid
		switch descriptor.Kind {
		case pathevidence.CoordinateDescriptorRefinement:
			kind = ValueRefinementCoordinatePath
		case pathevidence.CoordinateDescriptorStaticMember:
			kind = ValueRefinementCoordinateStaticMember
		default:
			continue
		}
		out.descendants = append(out.descendants, ValueRefinementCoordinate{slot: slot, path: descriptor.Path, kind: kind})
	}
	for _, slot := range heapRoots {
		if _, duplicate := out.heapRoots[slot.IdentityTerm()]; duplicate {
			return ValueRefinementPlan{}, fmt.Errorf("%w: duplicate value-refinement heap root", ErrInvalidLaneFactor)
		}
		out.heapRoots[slot.IdentityTerm()] = valueRefinementHeapRootBinding{slot: slot}
	}
	return out, nil
}

// SnapshotValueRefinementDescendants reads only the plan-certified finite
// preservation cone. No path-family inventory is inspected at execution.
func (d ProductDomain) SnapshotValueRefinementDescendants(
	plan ValueRefinementPlan,
	carrier *CoordinatePathEvidenceCarrier[statekey.Value],
) ([]ValueRefinementDescendantFact, error) {
	return SnapshotValueRefinementDescendants(d, plan, carrier)
}

func SnapshotValueRefinementDescendants[K comparable](
	d ProductDomain,
	plan ValueRefinementPlan,
	carrier *CoordinatePathEvidenceCarrier[K],
) ([]ValueRefinementDescendantFact, error) {
	if !plan.ValidFor(d) || carrier == nil || !carrier.Valid() || carrier.keys != plan.keys || carrier.domain.seal != d.seal {
		return nil, fmt.Errorf("%w: invalid value-refinement descendant snapshot", ErrInvalidLaneFactor)
	}
	out := make([]ValueRefinementDescendantFact, 0, len(plan.descendants))
	for _, coordinate := range plan.descendants {
		var value product.Value
		var present bool
		switch coordinate.kind {
		case ValueRefinementCoordinatePath:
			value, present = carrier.ReadPath(coordinate.path)
		case ValueRefinementCoordinateStaticMember:
			value, present = carrier.ReadStaticMember(coordinate.path)
		default:
			return nil, fmt.Errorf("%w: invalid value-refinement descendant role", ErrInvalidLaneFactor)
		}
		if present && !product.Equal(d.reg, value, product.Bottom(d.reg)) {
			out = append(out, ValueRefinementDescendantFact{coordinate: coordinate, value: value})
		}
	}
	return out, nil
}

// ResolveValueRefinementTarget executes the registered structural read law
// over the plan's target and explicit factors. It is the same canonical
// DynamicRead binder used by path replacement; no resolver or State is
// reconstructed by the caller.
func (d ProductDomain) ResolveValueRefinementTarget(
	plan ValueRefinementPlan,
	values PathReplacementValueReader,
	factors []LaneFactor,
) (product.Value, bool) {
	if !plan.ValidFor(d) || values == nil {
		return product.Value{}, false
	}
	return d.resolvePathReplacementValue(plan.keys, plan.target, values, factors)
}

// ResolveValueRefinementAnchor applies the same registered DynamicRead law to
// one plan-owned nested-union anchor. Arbitrary runtime paths are rejected.
func (d ProductDomain) ResolveValueRefinementAnchor(
	plan ValueRefinementPlan,
	target keyspace.Key,
	values PathReplacementValueReader,
	factors []LaneFactor,
) (product.Value, bool) {
	if !plan.ValidFor(d) || values == nil {
		return product.Value{}, false
	}
	owned := target == plan.target
	for _, anchor := range plan.anchors {
		owned = owned || target == anchor
	}
	if !owned {
		return product.Value{}, false
	}
	return d.resolvePathReplacementValue(plan.keys, target, values, factors)
}

// ResolveValueRefinementAnchorFromFactorProjection resolves one plan-owned
// anchor through a frame projection whose registered coordinate topology was
// already decomposed exactly once.
func (d ProductDomain) ResolveValueRefinementAnchorFromFactorProjection(
	plan ValueRefinementPlan,
	target keyspace.Key,
	values PathReplacementValueReader,
	projection *DynamicReadFactorProjection,
) (product.Value, bool) {
	if !plan.ValidFor(d) || values == nil || projection == nil || !projection.plan.ValidFor(d, plan.keys) {
		return product.Value{}, false
	}
	owned := target == plan.target
	for _, anchor := range plan.anchors {
		owned = owned || target == anchor
	}
	if !owned {
		return product.Value{}, false
	}
	return d.resolvePathReplacementValueFromFactorProjection(plan.keys, target, values, projection)
}

// RestoreValueRefinementDescendants atomically restores the compatible subset
// chosen by the scalar refinement law. Every fact must belong to plan; a
// foreign or duplicate coordinate aborts without mutating carrier.
func (d ProductDomain) RestoreValueRefinementDescendants(
	plan ValueRefinementPlan,
	carrier *CoordinatePathEvidenceCarrier[statekey.Value],
	facts []ValueRefinementDescendantFact,
) error {
	return RestoreValueRefinementDescendants(d, plan, carrier, facts)
}

func RestoreValueRefinementDescendants[K comparable](
	d ProductDomain,
	plan ValueRefinementPlan,
	carrier *CoordinatePathEvidenceCarrier[K],
	facts []ValueRefinementDescendantFact,
) error {
	if !plan.ValidFor(d) || carrier == nil || !carrier.Valid() || carrier.keys != plan.keys || carrier.domain.seal != d.seal {
		return fmt.Errorf("%w: invalid value-refinement descendant restore", ErrInvalidLaneFactor)
	}
	staged := carrier.Clone()
	if staged == nil {
		return fmt.Errorf("%w: value-refinement descendant restore cannot fork", ErrInvalidLaneFactor)
	}
	seen := make([]CoordinateSlot, 0, len(facts))
	for _, fact := range facts {
		owned := false
		for _, candidate := range plan.descendants {
			equal, equalErr := d.CoordinateSlotEqual(candidate.slot, fact.coordinate.slot)
			if equalErr == nil && equal && candidate.path == fact.coordinate.path && candidate.kind == fact.coordinate.kind {
				owned = true
				break
			}
		}
		if !owned || !product.BelongsToRegistry(d.reg, fact.value) {
			return fmt.Errorf("%w: foreign value-refinement descendant fact", ErrInvalidLaneFactor)
		}
		for _, prior := range seen {
			equal, equalErr := d.CoordinateSlotEqual(prior, fact.coordinate.slot)
			if equalErr != nil || equal {
				return fmt.Errorf("%w: duplicate value-refinement descendant fact", ErrInvalidLaneFactor)
			}
		}
		seen = append(seen, fact.coordinate.slot)
		var valid bool
		switch fact.coordinate.kind {
		case ValueRefinementCoordinatePath:
			_, valid = staged.WritePath(fact.coordinate.path, fact.value)
		case ValueRefinementCoordinateStaticMember:
			_, valid = staged.WriteStaticMember(fact.coordinate.path, fact.value)
		}
		if !valid {
			return fmt.Errorf("%w: value-refinement descendant write rejected", ErrInvalidLaneFactor)
		}
	}
	if !carrier.Commit(staged) {
		return fmt.Errorf("%w: value-refinement descendant commit failed", ErrInvalidLaneFactor)
	}
	return nil
}
