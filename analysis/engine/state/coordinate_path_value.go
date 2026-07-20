package state

import (
	"fmt"
	"sort"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// coordinatePathValueKind classifies ownership of State.ReadLocalPathKey.
// Every coordinate family declares none or unique ownership; the product
// rejects omissions and duplicate readers before guarded execution.
type coordinatePathValueKind uint8

const (
	coordinatePathValueInvalid coordinatePathValueKind = iota
	coordinatePathValueNone
	coordinatePathValueUnique
)

type coordinatePathValueOps struct {
	kind         coordinatePathValueKind
	closure      func([]coordinateKeyPayload, *keyspace.KeySpace, []keyspace.Key, []symbol.ID) ([]bool, bool)
	dependencies func(*keyspace.KeySpace, []coordinateKeyPayload, []coordinateDependencySeedPayload) (coordinateDependencyPlanPayload, bool)
	supportPaths func(coordinateKeyPayload) []keyspace.Key
}

type coordinateDependencySeedPayload struct {
	id                                                               uint64
	readPaths, resolvePaths, writePaths, mutationRoots, subtreeRoots []keyspace.Key
	stableRootMutations                                              []symbol.ID
	transientEqualities                                              []coordinateDependencyEqualityPayload
	readCoordinates, add                                             []coordinateKeyPayload
}

type coordinateDependencyEqualityPayload struct {
	left, right keyspace.Key
}
type coordinateDependencyLocationPayload struct {
	root statekey.ValueDependency
	path keyspace.Key
}
type coordinateDependencyPayload struct {
	id                                             uint64
	reads, writes                                  []coordinateKeyPayload
	locationReads, locationWrites, mutationRegions []coordinateDependencyLocationPayload
}
type coordinateDependencyPlanPayload struct {
	coordinates []coordinateKeyPayload
	order       []uint64
	byID        map[uint64]coordinateDependencyPayload
	depends     map[coordinateDependencyEdgePayload]struct{}
	feeds       map[coordinateDependencyEdgePayload]struct{}
}

type coordinateDependencyEdgePayload struct {
	writer uint64
	reader uint64
}

func noCoordinatePathValues() coordinatePathValueOps {
	return coordinatePathValueOps{kind: coordinatePathValueNone}
}

func uniqueCoordinatePathValues(
	closure func([]coordinateKeyPayload, *keyspace.KeySpace, []keyspace.Key, []symbol.ID) ([]bool, bool),
	dependencies func(*keyspace.KeySpace, []coordinateKeyPayload, []coordinateDependencySeedPayload) (coordinateDependencyPlanPayload, bool),
	supportPaths func(coordinateKeyPayload) []keyspace.Key,
) coordinatePathValueOps {
	return coordinatePathValueOps{kind: coordinatePathValueUnique, closure: closure, dependencies: dependencies, supportPaths: supportPaths}
}

func coordinatePathValueOpsComplete(ops coordinatePathValueOps) bool {
	return ops.kind == coordinatePathValueNone && ops.closure == nil && ops.dependencies == nil ||
		ops.kind == coordinatePathValueUnique && ops.closure != nil && ops.dependencies != nil && ops.supportPaths != nil
}

// PathCoordinateSupportPaths projects an exact sealed inventory to the
// structural paths named by its registered path-value coordinates. Operation
// code never interprets a coordinate kind to discover these dependencies.
func (d ProductDomain) PathCoordinateSupportPaths(slots []CoordinateSlot) ([]keyspace.Key, error) {
	owner, ok := d.PathValueFamily()
	if !ok {
		return nil, fmt.Errorf("%w: no path-value support family", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	if err != nil || coordinate.ops.pathValues.supportPaths == nil {
		return nil, fmt.Errorf("%w: invalid path-value support family", ErrInvalidLaneFactor)
	}
	var keys *keyspace.KeySpace
	seen := make(map[keyspace.Key]struct{})
	out := make([]keyspace.Key, 0)
	for _, slot := range slots {
		if slot.family != owner || slot.keys == nil || !slot.keys.Valid() {
			return nil, fmt.Errorf("%w: foreign path-value support slot", ErrInvalidLaneFactor)
		}
		if keys == nil {
			keys = slot.keys
		} else if keys != slot.keys {
			return nil, fmt.Errorf("%w: mixed path-value support keyspaces", ErrInvalidLaneFactor)
		}
		if err := d.validateCoordinateSlotFor(coordinate, slot, keys); err != nil {
			return nil, err
		}
		for _, path := range coordinate.ops.pathValues.supportPaths(slot.key) {
			if path.Kind == keyspace.KindInvalid {
				continue
			}
			if keys.FormatReadOnly(path) == "" {
				return nil, fmt.Errorf("%w: invalid path-value support path", ErrInvalidLaneFactor)
			}
			if _, duplicate := seen[path]; duplicate {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}
	if keys != nil {
		sort.Slice(out, func(i, j int) bool { return keys.Less(out[i], out[j]) })
	}
	return out, nil
}

type CoordinateDependencyID uint64

type CoordinateDependencySeed struct {
	ID                                                                                 CoordinateDependencyID
	ReadPaths, ResolvePaths, WritePaths, DescendantMutationRoots, SubtreeMutationRoots []keyspace.Key
	// StableRootMutations are lexical roots whose stable/unversioned evidence
	// is atomically rewritten. The registered path family turns this semantic
	// mutation region into exact coordinate reads/writes; operation executors
	// never enumerate coordinate kinds or axes.
	StableRootMutations []symbol.ID
	// TransientEqualities describe equality closure used by an operation without
	// publishing the equality itself. The path family certifies all proof and
	// refinement reads plus derived writes while keeping the equality absent
	// from the union.
	TransientEqualities []CoordinateDependencyEquality
	// ReadCoordinates are exact scalar observations which are not implied by
	// path resolution. They make representation-owned metadata membership an
	// explicit dependency instead of relying on a full-family carrier.
	ReadCoordinates []CoordinateSlot
	// AddCoordinates are operation-owned publications. The family adds them to
	// the union inventory and certifies each one as a CoordinateWrite in this
	// same dependency transaction.
	AddCoordinates []CoordinateSlot
}

type CoordinateDependencyEquality struct {
	Left, Right keyspace.Key
}

type CoordinateDependencyLocation struct {
	Root statekey.ValueDependency
	Path keyspace.Key
}

func (l CoordinateDependencyLocation) IsRoot() bool { return l.Root.Valid() }

type CoordinateDependency struct {
	id                                             CoordinateDependencyID
	coordinateReads, coordinateWrites              []CoordinateSlot
	locationReads, locationWrites, mutationRegions []CoordinateDependencyLocation
}

func (d CoordinateDependency) ID() CoordinateDependencyID { return d.id }
func (d CoordinateDependency) CoordinateReads() []CoordinateSlot {
	return append([]CoordinateSlot(nil), d.coordinateReads...)
}
func (d CoordinateDependency) CoordinateWrites() []CoordinateSlot {
	return append([]CoordinateSlot(nil), d.coordinateWrites...)
}
func (d CoordinateDependency) LocationReads() []CoordinateDependencyLocation {
	return append([]CoordinateDependencyLocation(nil), d.locationReads...)
}
func (d CoordinateDependency) LocationWrites() []CoordinateDependencyLocation {
	return append([]CoordinateDependencyLocation(nil), d.locationWrites...)
}
func (d CoordinateDependency) MutationRegions() []CoordinateDependencyLocation {
	return append([]CoordinateDependencyLocation(nil), d.mutationRegions...)
}

type CoordinateDependencyPlan struct {
	coordinates []CoordinateSlot
	order       []CoordinateDependencyID
	byID        map[CoordinateDependencyID]CoordinateDependency
	depends     map[coordinateDependencyEdge]struct{}
	feeds       map[coordinateDependencyEdge]struct{}
}

type coordinateDependencyEdge struct {
	writer CoordinateDependencyID
	reader CoordinateDependencyID
}

func (p CoordinateDependencyPlan) Coordinates() []CoordinateSlot {
	return append([]CoordinateSlot(nil), p.coordinates...)
}
func (p CoordinateDependencyPlan) IDs() []CoordinateDependencyID {
	return append([]CoordinateDependencyID(nil), p.order...)
}
func (p CoordinateDependencyPlan) Dependency(id CoordinateDependencyID) (CoordinateDependency, bool) {
	d, ok := p.byID[id]
	return d, ok
}

// Depends reports the family-certified directional dependency from writer to
// reader. The state layer transports this sealed result without interpreting
// path aliases, roots, descendants, or coordinate kinds.
func (p CoordinateDependencyPlan) Depends(writer, reader CoordinateDependencyID) bool {
	_, ok := p.depends[coordinateDependencyEdge{writer: writer, reader: reader}]
	return ok
}

// Feeds reports the registered family's directional output-to-observation RAW
// edge. WAW ownership and a reader's implicit target-accumulation read remain
// conflicts in Depends but are deliberately absent here.
func (p CoordinateDependencyPlan) Feeds(writer, reader CoordinateDependencyID) bool {
	_, ok := p.feeds[coordinateDependencyEdge{writer: writer, reader: reader}]
	return ok
}

func (d ProductDomain) PlanPathCoordinateDependencies(
	keys *keyspace.KeySpace,
	union []CoordinateSlot,
	seeds []CoordinateDependencySeed,
) (CoordinateDependencyPlan, error) {
	owner, ok := d.PathValueFamily()
	if !ok || keys == nil || !keys.Valid() {
		return CoordinateDependencyPlan{}, fmt.Errorf("%w: no path dependency family", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	if err != nil || coordinate.ops.pathValues.dependencies == nil {
		return CoordinateDependencyPlan{}, fmt.Errorf("%w: invalid path dependency family", ErrInvalidLaneFactor)
	}
	unionPayload := make([]coordinateKeyPayload, len(union))
	for index, slot := range union {
		if err := d.validateCoordinateSlotFor(coordinate, slot, keys); err != nil {
			return CoordinateDependencyPlan{}, err
		}
		unionPayload[index] = slot.key
	}
	seedPayload := make([]coordinateDependencySeedPayload, len(seeds))
	for index, seed := range seeds {
		readCoordinates := make([]coordinateKeyPayload, len(seed.ReadCoordinates))
		for readIndex, slot := range seed.ReadCoordinates {
			if err := d.validateCoordinateSlotFor(coordinate, slot, keys); err != nil {
				return CoordinateDependencyPlan{}, err
			}
			readCoordinates[readIndex] = slot.key
		}
		add := make([]coordinateKeyPayload, len(seed.AddCoordinates))
		for addIndex, slot := range seed.AddCoordinates {
			if err := d.validateCoordinateSlotFor(coordinate, slot, keys); err != nil {
				return CoordinateDependencyPlan{}, err
			}
			add[addIndex] = slot.key
		}
		equalities := make([]coordinateDependencyEqualityPayload, len(seed.TransientEqualities))
		for equalityIndex, equality := range seed.TransientEqualities {
			equalities[equalityIndex] = coordinateDependencyEqualityPayload{left: equality.Left, right: equality.Right}
		}
		seedPayload[index] = coordinateDependencySeedPayload{
			id: uint64(seed.ID), readPaths: append([]keyspace.Key(nil), seed.ReadPaths...), resolvePaths: append([]keyspace.Key(nil), seed.ResolvePaths...),
			writePaths: append([]keyspace.Key(nil), seed.WritePaths...), mutationRoots: append([]keyspace.Key(nil), seed.DescendantMutationRoots...),
			subtreeRoots:        append([]keyspace.Key(nil), seed.SubtreeMutationRoots...),
			stableRootMutations: append([]symbol.ID(nil), seed.StableRootMutations...),
			transientEqualities: equalities, readCoordinates: readCoordinates, add: add,
		}
	}
	payload, valid := coordinate.ops.pathValues.dependencies(keys, unionPayload, seedPayload)
	if !valid {
		return CoordinateDependencyPlan{}, fmt.Errorf("%w: path dependency planning failed", ErrInvalidLaneFactor)
	}
	wrapSlots := func(in []coordinateKeyPayload) []CoordinateSlot {
		out := make([]CoordinateSlot, len(in))
		for i, key := range in {
			out[i] = CoordinateSlot{family: owner, keys: keys, key: key}
		}
		return out
	}
	out := CoordinateDependencyPlan{
		coordinates: wrapSlots(payload.coordinates),
		order:       make([]CoordinateDependencyID, len(payload.order)),
		byID:        make(map[CoordinateDependencyID]CoordinateDependency, len(payload.byID)),
		depends:     make(map[coordinateDependencyEdge]struct{}, len(payload.depends)),
		feeds:       make(map[coordinateDependencyEdge]struct{}, len(payload.feeds)),
	}
	for i, id := range payload.order {
		out.order[i] = CoordinateDependencyID(id)
	}
	wrapLocations := func(in []coordinateDependencyLocationPayload) []CoordinateDependencyLocation {
		out := make([]CoordinateDependencyLocation, len(in))
		for i, v := range in {
			out[i] = CoordinateDependencyLocation{Root: v.root, Path: v.path}
		}
		return out
	}
	for id, dep := range payload.byID {
		out.byID[CoordinateDependencyID(id)] = CoordinateDependency{id: CoordinateDependencyID(dep.id), coordinateReads: wrapSlots(dep.reads), coordinateWrites: wrapSlots(dep.writes), locationReads: wrapLocations(dep.locationReads), locationWrites: wrapLocations(dep.locationWrites), mutationRegions: wrapLocations(dep.mutationRegions)}
	}
	for edge := range payload.depends {
		out.depends[coordinateDependencyEdge{writer: CoordinateDependencyID(edge.writer), reader: CoordinateDependencyID(edge.reader)}] = struct{}{}
	}
	for edge := range payload.feeds {
		out.feeds[coordinateDependencyEdge{writer: CoordinateDependencyID(edge.writer), reader: CoordinateDependencyID(edge.reader)}] = struct{}{}
	}
	return out, nil
}

// PathBranchProofCoordinateSlot seals an operation-owned branch-proof
// publication into the registered path-value family. The returned slot can be
// placed directly in CoordinateDependencySeed.AddCoordinates, so publication
// identity and write authority remain one frozen dependency certificate.
func (d ProductDomain) PathBranchProofCoordinateSlot(keys *keyspace.KeySpace, proof pathevidence.BranchProof) (CoordinateSlot, error) {
	owner, ok := d.PathValueFamily()
	if !ok || keys == nil || !keys.Valid() {
		return CoordinateSlot{}, fmt.Errorf("%w: invalid branch-proof coordinate publication", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	payload := typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: pathevidence.BranchProofCoordinate(proof)}
	if err != nil || !coordinate.ops.keyValid(payload, keys) {
		return CoordinateSlot{}, fmt.Errorf("%w: invalid branch-proof coordinate", ErrInvalidLaneFactor)
	}
	return CoordinateSlot{family: owner, keys: keys, key: payload}, nil
}

func (d ProductDomain) PathCoordinateMutationClosure(slots []CoordinateSlot, seeds []keyspace.Key, roots []symbol.ID) ([]int, error) {
	owner, ok := d.PathValueFamily()
	if !ok {
		return nil, fmt.Errorf("%w: no path-value family", ErrInvalidLaneFactor)
	}
	coordinate, err := d.validateCoordinateFamily(owner)
	if err != nil || coordinate.ops.pathValues.kind != coordinatePathValueUnique || coordinate.ops.pathValues.closure == nil {
		return nil, fmt.Errorf("%w: invalid path-value closure owner", ErrInvalidLaneFactor)
	}
	if len(slots) == 0 {
		return nil, nil
	}
	keys := make([]coordinateKeyPayload, len(slots))
	var keysOwner *keyspace.KeySpace
	for index, slot := range slots {
		if slot.family != owner {
			return nil, fmt.Errorf("%w: foreign path coordinate slot", ErrInvalidLaneFactor)
		}
		if err := d.validateCoordinateSlotFor(coordinate, slot, slot.keys); err != nil {
			return nil, err
		}
		if index == 0 {
			keysOwner = slot.keys
		} else if slot.keys != keysOwner {
			return nil, fmt.Errorf("%w: mixed path coordinate keyspaces", ErrInvalidLaneFactor)
		}
		keys[index] = slot.key
	}
	selected, ok := coordinate.ops.pathValues.closure(keys, keysOwner, seeds, roots)
	if !ok || len(selected) != len(keys) {
		return nil, fmt.Errorf("%w: path coordinate closure", ErrInvalidLaneFactor)
	}
	out := make([]int, 0, len(selected))
	for index, include := range selected {
		if include {
			out = append(out, index)
		}
	}
	return out, nil
}

// PathValueFamily returns the unique registered coordinate family observed by
// State.ReadLocalPathKey. Consumers select this opaque descriptor as an exact
// factor dependency and never dispatch on a lane or family name.
func (d ProductDomain) PathValueFamily() (CoordinateFamily, bool) {
	if !d.Valid() || !d.hasPathValueFamily {
		return CoordinateFamily{}, false
	}
	if _, err := d.validateCoordinateFamily(d.pathValueFamily); err != nil {
		return CoordinateFamily{}, false
	}
	return d.pathValueFamily, true
}

// ReadPathValueFactor reads one exact structural path through the unique
// registered path-value coordinate family. It is factor-native and dispatches
// through the family capability; callers never switch on a lane identifier.
func (d ProductDomain) ReadPathValueFactor(factor LaneFactor, keys *keyspace.KeySpace, path keyspace.Key) (product.Value, bool, error) {
	carrier, err := d.openPathValueFactor(factor, keys, path)
	if err != nil {
		return product.Value{}, false, err
	}
	value, present := carrier.ReadPath(path)
	if present && !product.BelongsToRegistry(d.reg, value) {
		return product.Value{}, false, fmt.Errorf("%w: path-value factor returned a foreign value", ErrInvalidLaneFactor)
	}
	return value, present, nil
}

func (d ProductDomain) openPathValueFactor(factor LaneFactor, keys *keyspace.KeySpace, path keyspace.Key) (coordinatePathEvidenceCarrier, error) {
	family, ok := d.PathValueFamily()
	if !ok || keys == nil || !keys.Valid() || keys.FormatReadOnly(path) == "" {
		return nil, fmt.Errorf("%w: path-value factor request is unowned", ErrInvalidLaneFactor)
	}
	_, coordinate, err := d.validateCoordinateFamilyFactor(factor, family)
	if err != nil || coordinate.ops.pathEvidence.kind != coordinatePathEvidenceUnique || coordinate.ops.pathEvidence.open == nil {
		return nil, fmt.Errorf("%w: path-value factor has no registered reader", ErrInvalidLaneFactor)
	}
	skeleton, scalars, err := d.DecomposeCoordinateFamily(factor, family, keys)
	if err != nil {
		return nil, err
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return nil, err
	}
	carrier, opened := coordinate.ops.pathEvidence.open(skeleton.payload, entries, keys)
	if !opened || carrier == nil {
		return nil, fmt.Errorf("%w: path-value factor open failed", ErrInvalidLaneFactor)
	}
	return carrier, nil
}

// EquivalentPathStateKeysFactor returns the finite already-tracked equality
// class of one path through the registered path-evidence carrier.
func (d ProductDomain) EquivalentPathStateKeysFactor(factor LaneFactor, keys *keyspace.KeySpace, path keyspace.Key) ([]pathaddr.StateKey, error) {
	carrier, err := d.openPathValueFactor(factor, keys, path)
	if err != nil {
		return nil, err
	}
	equivalent, ok := carrier.EquivalentKeys(path)
	if !ok {
		return nil, fmt.Errorf("%w: path equality observation rejected", ErrInvalidLaneFactor)
	}
	out := make([]pathaddr.StateKey, len(equivalent))
	for index, key := range equivalent {
		out[index] = pathaddr.StateKey(keys.FormatReadOnly(key))
	}
	return out, nil
}
