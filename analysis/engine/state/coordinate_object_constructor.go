package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// ObjectConstructorShape is the value-independent syntax of one table
// constructor. Values remain separate guarded coordinates; the shape is
// normalized once and is shared by concrete and guarded execution.
type ObjectConstructorShape struct {
	Identity       identity.Term
	MemberSuffixes [][]segment.Segment
	StableShape    bool
}

type ObjectConstructorValues struct {
	Root    product.Value
	Members []product.Value
}

type ObjectConstructorValueRef struct {
	object int
	member int // -1 denotes the object root
}

func (r ObjectConstructorValueRef) ObjectIndex() int         { return r.object }
func (r ObjectConstructorValueRef) MemberIndex() (int, bool) { return r.member, r.member >= 0 }

type objectConstructorMember struct {
	key    keyspace.Key
	source ObjectConstructorValueRef
}

type objectConstructorObject struct {
	id          identity.Term
	source      int
	members     []objectConstructorMember
	stableShape bool
}

// ObjectConstructorPlan is sealed to one ProductDomain and KeySpace.
type ObjectConstructorPlan struct {
	seal       *productDomainSeal
	keys       *keyspace.KeySpace
	shapeCount int
	objects    []objectConstructorObject
}

func (p ObjectConstructorPlan) Valid() bool {
	return p.seal != nil && p.keys != nil && len(p.objects) != 0
}

// PrepareObjectConstructorPlan canonicalizes repeated identities (last write
// wins), field mirrors, and member overwrites without reading a prior heap.
func (d ProductDomain) PrepareObjectConstructorPlan(keys *keyspace.KeySpace, shapes []ObjectConstructorShape) (ObjectConstructorPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || len(shapes) == 0 {
		return ObjectConstructorPlan{}, fmt.Errorf("%w: invalid object constructor", ErrInvalidLaneFactor)
	}
	seen := make(map[identity.Term]struct{}, len(shapes))
	reverse := make([]objectConstructorObject, 0, len(shapes))
	for objectIndex := len(shapes) - 1; objectIndex >= 0; objectIndex-- {
		shape := shapes[objectIndex]
		if !shape.Identity.Valid() {
			return ObjectConstructorPlan{}, fmt.Errorf("%w: constructor has empty identity", ErrInvalidLaneFactor)
		}
		if _, duplicate := seen[shape.Identity]; duplicate {
			continue
		}
		seen[shape.Identity] = struct{}{}
		members := make(map[keyspace.Key]ObjectConstructorValueRef, len(shape.MemberSuffixes)*2)
		for memberIndex, suffix := range shape.MemberSuffixes {
			key, ok := heapidentity.StaticMemberSuffixKey(keys, suffix)
			if !ok {
				return ObjectConstructorPlan{}, fmt.Errorf("%w: constructor member %d.%d has invalid suffix", ErrInvalidLaneFactor, objectIndex, memberIndex)
			}
			source := ObjectConstructorValueRef{object: objectIndex, member: memberIndex}
			members[key] = source
			if canonical, mirrored := heapidentity.FieldCanonicalStaticMemberSuffixKey(keys, suffix); mirrored {
				members[canonical] = source
			}
		}
		ordered := make([]objectConstructorMember, 0, len(members))
		for key, source := range members {
			ordered = append(ordered, objectConstructorMember{key: key, source: source})
		}
		sort.Slice(ordered, func(i, j int) bool { return keys.Less(ordered[i].key, ordered[j].key) })
		reverse = append(reverse, objectConstructorObject{id: shape.Identity, source: objectIndex, members: ordered, stableShape: shape.StableShape})
	}
	objects := make([]objectConstructorObject, len(reverse))
	for i := range reverse {
		objects[len(reverse)-1-i] = reverse[i]
	}
	return ObjectConstructorPlan{seal: d.seal, keys: keys, shapeCount: len(shapes), objects: objects}, nil
}

// ObjectGraphMutation is one complete object/placement contribution. The
// object and placement share Identity and are validated together before the
// mutation can enter either concrete or formal execution. Placement Bottom
// means this mutation does not publish placement.
type ObjectGraphMutation struct {
	Identity  identity.Term
	Object    heapidentity.TableObject
	Placement placement.Value
}

type objectGraphMutationMode uint8

const (
	objectGraphReplace objectGraphMutationMode = iota + 1
	objectGraphJoin
)

type objectGraphMutation struct {
	id        identity.Term
	object    heapidentity.TableObject
	placement placement.Value
}

// ObjectGraphMutationPlan is the sealed, deterministic object-graph law used
// by constructors and allocation templates. It owns no State and carries no
// callbacks; every participating coordinate family interprets the same plan.
type ObjectGraphMutationPlan struct {
	seal    *productDomainSeal
	keys    *keyspace.KeySpace
	mode    objectGraphMutationMode
	objects []objectGraphMutation
}

func (p ObjectGraphMutationPlan) Valid() bool {
	return p.seal != nil && p.keys != nil && p.keys.Valid() && len(p.objects) != 0 &&
		(p.mode == objectGraphReplace || p.mode == objectGraphJoin)
}

// PrepareObjectGraphJoinPlan seals a recursive allocation contribution. Heap
// objects and placements are joined pointwise with the input graph.
func (d ProductDomain) PrepareObjectGraphJoinPlan(keys *keyspace.KeySpace, mutations []ObjectGraphMutation) (ObjectGraphMutationPlan, error) {
	return d.prepareObjectGraphMutationPlan(keys, objectGraphJoin, mutations)
}

// PrepareObjectGraphReplacePlan seals an exact object replacement. Individual
// rows may omit placement by carrying placement.Bottom.
func (d ProductDomain) PrepareObjectGraphReplacePlan(keys *keyspace.KeySpace, mutations []ObjectGraphMutation) (ObjectGraphMutationPlan, error) {
	return d.prepareObjectGraphMutationPlan(keys, objectGraphReplace, mutations)
}

func (d ProductDomain) prepareObjectGraphMutationPlan(keys *keyspace.KeySpace, mode objectGraphMutationMode, mutations []ObjectGraphMutation) (ObjectGraphMutationPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || len(mutations) == 0 ||
		(mode != objectGraphReplace && mode != objectGraphJoin) {
		return ObjectGraphMutationPlan{}, fmt.Errorf("%w: invalid object graph mutation", ErrInvalidLaneFactor)
	}
	objects := make([]objectGraphMutation, len(mutations))
	seen := make(map[identity.Term]struct{}, len(mutations))
	for index, mutation := range mutations {
		if !mutation.Identity.Valid() || mutation.Object.IsBottom() || mutation.Placement > placement.Unknown {
			return ObjectGraphMutationPlan{}, fmt.Errorf("%w: invalid object graph member %d", ErrInvalidLaneFactor, index)
		}
		if _, duplicate := seen[mutation.Identity]; duplicate {
			return ObjectGraphMutationPlan{}, fmt.Errorf("%w: duplicate object graph identity", ErrInvalidLaneFactor)
		}
		seen[mutation.Identity] = struct{}{}
		rootID, exact := identityvalue.ExactTerm(d.reg, mutation.Object.Root())
		if !exact || rootID != mutation.Identity || !product.BelongsToRegistry(d.reg, mutation.Object.Root()) {
			return ObjectGraphMutationPlan{}, fmt.Errorf("%w: object graph root mismatch", ErrInvalidLaneFactor)
		}
		valid := true
		mutation.Object.VisitStaticMembers(func(key keyspace.Key, value product.Value) bool {
			_, owned := keys.SuffixSegmentsView(key)
			valid = owned && product.BelongsToRegistry(d.reg, value)
			return valid
		})
		mutation.Object.VisitDynamicIndexFacts(func(key dynamicindex.Key, fact dynamicindex.Fact) bool {
			// Dynamic-index facts name their container in the active keyspace;
			// unlike StaticMembers they are not rootless member suffixes.
			_, owned := keys.SegmentsView(key.Table)
			valid = owned && product.BelongsToRegistry(d.reg, fact.KeyValue) && product.BelongsToRegistry(d.reg, fact.Value)
			return valid
		})
		if !valid {
			return ObjectGraphMutationPlan{}, fmt.Errorf("%w: foreign object graph coordinate", ErrInvalidLaneFactor)
		}
		objects[index] = objectGraphMutation{
			id: mutation.Identity, object: heapidentity.CloneObject(mutation.Object), placement: mutation.Placement,
		}
	}
	sort.Slice(objects, func(i, j int) bool { return identityTermLess(objects[i].id, objects[j].id) })
	return ObjectGraphMutationPlan{seal: d.seal, keys: keys, mode: mode, objects: objects}, nil
}

type coordinateObjectMutationPublication struct {
	key       coordinateKeyPayload
	value     product.Value
	placement placement.Value
	mode      objectGraphMutationMode
}

type coordinateObjectMutationOps struct {
	participant   bool
	active        func(ObjectGraphMutationPlan) bool
	applySkeleton func(coordinateSkeletonPayload, ObjectGraphMutationPlan) (coordinateSkeletonPayload, []coordinateObjectMutationPublication, bool, error)
	affectsKey    func(ObjectGraphMutationPlan, coordinateKeyPayload) bool
	applyScalar   func(coordinateObjectMutationPublication, coordinateScalarPayload) (coordinateScalarPayload, error)
}

func coordinateObjectMutationOpsComplete(ops coordinateObjectMutationOps) bool {
	return ops.active != nil && ops.applySkeleton != nil && ops.affectsKey != nil && ops.applyScalar != nil
}

func noCoordinateObjectMutation() coordinateObjectMutationOps {
	return coordinateObjectMutationOps{
		participant: false,
		active:      func(ObjectGraphMutationPlan) bool { return false },
		applySkeleton: func(source coordinateSkeletonPayload, _ ObjectGraphMutationPlan) (coordinateSkeletonPayload, []coordinateObjectMutationPublication, bool, error) {
			return source, nil, false, nil
		},
		affectsKey: func(ObjectGraphMutationPlan, coordinateKeyPayload) bool { return false },
		applyScalar: func(coordinateObjectMutationPublication, coordinateScalarPayload) (coordinateScalarPayload, error) {
			return nil, ErrInvalidLaneFactor
		},
	}
}

// ObjectConstructorLanes returns the exact registered dependent-product
// groups touched by this constructor.  Participation is a property of the
// coordinate-family registration, not an executor-side axis inventory.
func (d ProductDomain) ObjectConstructorLanes(plan ObjectConstructorPlan) ([]ProductLane, error) {
	if !d.Valid() || !plan.Valid() || plan.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign object constructor", ErrInvalidLaneFactor)
	}
	return d.ObjectMutationParticipantLanes(), nil
}

// ObjectMutationParticipantLanes returns the complete registration-owned
// dependent-product groups capable of interpreting an object graph. The
// inventory is shape-independent, so formal Effect freezing can bind groups
// before a guarded leaf supplies its identity terms and values.
func (d ProductDomain) ObjectMutationParticipantLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 2)
	for laneIndex := range d.factorLanes {
		for _, coordinate := range d.factorLanes[laneIndex].coordinates {
			if coordinate.ops.objectMutation.participant {
				out = append(out, d.factorLanes[laneIndex].lane)
				break
			}
		}
	}
	return out
}

// ObjectConstructorCoordinateWrites returns the exact static coordinate slots
// introduced by a constructor shape. Scalar values are deliberately absent:
// the formal fiber universe depends on topology, identities, and member keys,
// never on guarded leaf values.
func (d ProductDomain) ObjectConstructorCoordinateWrites(plan ObjectConstructorPlan) ([]CoordinateSlot, error) {
	if !d.Valid() || !plan.Valid() || plan.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign object constructor", ErrInvalidLaneFactor)
	}
	objects := make([]objectGraphMutation, len(plan.objects))
	for index, object := range plan.objects {
		members := make(map[keyspace.Key]product.Value, len(object.members))
		for _, member := range object.members {
			members[member.key] = product.Top()
		}
		objects[index] = objectGraphMutation{
			id: object.id,
			object: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: identityvalue.PresentTerm(d.reg, object.id), StaticMembers: members,
				StableShape: object.stableShape, PrefixStableShape: true,
			}),
			placement: placement.Stack,
		}
	}
	graph := ObjectGraphMutationPlan{seal: d.seal, keys: plan.keys, mode: objectGraphReplace, objects: objects}
	return d.ObjectGraphMutationCoordinateWrites(graph)
}

// ObjectGraphMutationCoordinateWrites returns the exact static coordinate
// topology introduced by a sealed object graph, independent of guarded
// execution. AllocationTemplate and object constructors share this inventory.
func (d ProductDomain) ObjectGraphMutationCoordinateWrites(graph ObjectGraphMutationPlan) ([]CoordinateSlot, error) {
	if !d.Valid() || !graph.Valid() || graph.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign object graph mutation", ErrInvalidLaneFactor)
	}
	var out []CoordinateSlot
	for laneIndex := range d.factorLanes {
		for _, coordinate := range d.factorLanes[laneIndex].coordinates {
			if !coordinate.ops.objectMutation.active(graph) {
				continue
			}
			skeleton, err := d.CoordinateSkeletonBottom(coordinate.family, graph.keys)
			if err != nil {
				return nil, err
			}
			_, publications, participant, err := d.applyCoordinateObjectMutationSkeleton(coordinate.family, skeleton, graph)
			if err != nil {
				return nil, err
			}
			if participant {
				for _, publication := range publications {
					out = append(out, publication.slot)
				}
			}
		}
	}
	return out, nil
}

type coordinateObjectMutationPublicationView struct {
	family    CoordinateFamily
	slot      CoordinateSlot
	value     product.Value
	placement placement.Value
	mode      objectGraphMutationMode
}

func (d ProductDomain) applyCoordinateObjectMutationSkeleton(family CoordinateFamily, skeleton CoordinateFamilySkeleton, plan ObjectGraphMutationPlan) (CoordinateFamilySkeleton, []coordinateObjectMutationPublicationView, bool, error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || plan.seal != d.seal || plan.keys != skeleton.keys || skeleton.family != family {
		return CoordinateFamilySkeleton{}, nil, false, fmt.Errorf("%w: foreign object graph mutation", ErrInvalidLaneFactor)
	}
	payload, pubs, participant, err := coordinate.ops.objectMutation.applySkeleton(skeleton.payload, plan)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, false, err
	}
	out := make([]coordinateObjectMutationPublicationView, len(pubs))
	for i, pub := range pubs {
		slot := CoordinateSlot{family: family, keys: plan.keys, key: pub.key}
		if !coordinate.ops.keyValid(pub.key, plan.keys) {
			return CoordinateFamilySkeleton{}, nil, false, ErrInvalidLaneFactor
		}
		out[i] = coordinateObjectMutationPublicationView{family: family, slot: slot, value: pub.value, placement: pub.placement, mode: pub.mode}
	}
	return CoordinateFamilySkeleton{family: family, keys: plan.keys, payload: payload}, out, participant, nil
}

func (d ProductDomain) coordinateObjectMutationAffects(plan ObjectGraphMutationPlan, slot CoordinateSlot) (bool, error) {
	coordinate, err := d.validateCoordinateFamily(slot.family)
	if err != nil || plan.seal != d.seal || slot.keys != plan.keys || !coordinate.ops.keyValid(slot.key, slot.keys) {
		return false, ErrInvalidLaneFactor
	}
	return coordinate.ops.objectMutation.affectsKey(plan, slot.key), nil
}

func (d ProductDomain) applyCoordinateObjectMutationScalar(publication coordinateObjectMutationPublicationView, current CoordinateScalarFactor) (CoordinateScalarFactor, error) {
	coordinate, err := d.validateCoordinateFamily(publication.family)
	if err != nil || publication.slot.family != publication.family {
		return CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	var currentPayload coordinateScalarPayload
	if current.payload != nil {
		if err := d.validateCoordinateFactorFor(coordinate, current, publication.slot.keys); err != nil {
			return CoordinateScalarFactor{}, err
		}
		currentPayload = current.payload
	}
	payload, err := coordinate.ops.objectMutation.applyScalar(coordinateObjectMutationPublication{
		key: publication.slot.key, value: publication.value, placement: publication.placement, mode: publication.mode,
	}, currentPayload)
	if err != nil || payload == nil || !coordinate.ops.scalarValid(publication.slot.key, payload) {
		return CoordinateScalarFactor{}, ErrInvalidLaneFactor
	}
	return CoordinateScalarFactor{slot: publication.slot, payload: payload}, nil
}

func (d ProductDomain) objectConstructorMutationPlan(plan ObjectConstructorPlan, values []ObjectConstructorValues) (ObjectGraphMutationPlan, error) {
	if err := d.validateObjectConstructorValues(plan, values); err != nil {
		return ObjectGraphMutationPlan{}, err
	}
	mutations := make([]ObjectGraphMutation, len(plan.objects))
	for index, object := range plan.objects {
		members := make(map[keyspace.Key]product.Value, len(object.members))
		for _, member := range object.members {
			if member.source.object < 0 || member.source.object >= len(values) || member.source.member < 0 ||
				member.source.member >= len(values[member.source.object].Members) {
				return ObjectGraphMutationPlan{}, ErrInvalidLaneFactor
			}
			members[member.key] = values[member.source.object].Members[member.source.member]
		}
		mutations[index] = ObjectGraphMutation{
			Identity: object.id,
			Object: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: values[object.source].Root, StaticMembers: members,
				StableShape: object.stableShape, PrefixStableShape: true,
			}),
			Placement: placement.Stack,
		}
	}
	return d.prepareObjectGraphMutationPlan(plan.keys, objectGraphReplace, mutations)
}

func (d ProductDomain) validateObjectConstructorValues(plan ObjectConstructorPlan, values []ObjectConstructorValues) error {
	if plan.seal != d.seal || len(values) != plan.shapeCount {
		return ErrInvalidLaneFactor
	}
	for i := range values {
		if !product.BelongsToRegistry(d.reg, values[i].Root) {
			return ErrInvalidLaneFactor
		}
		for _, value := range values[i].Members {
			if !product.BelongsToRegistry(d.reg, value) {
				return ErrInvalidLaneFactor
			}
		}
	}
	for _, object := range plan.objects {
		id, exact := identityvalue.ExactTerm(d.reg, values[object.source].Root)
		if !exact || id != object.id {
			return fmt.Errorf("%w: constructor identity/root mismatch", ErrInvalidLaneFactor)
		}
	}
	return nil
}

// ApplyObjectConstructorFactor is the sole implementation of the registered
// object-constructor algebra. A coordinate lane is one dependent sum: all of
// its family skeletons and sparse scalar fibres enter and leave atomically.
// Concrete State execution and guarded execution both delegate here.
func (d ProductDomain) ApplyObjectConstructorFactor(plan ObjectConstructorPlan, values []ObjectConstructorValues, input LaneFactor) (LaneFactor, error) {
	mutation, err := d.objectConstructorMutationPlan(plan, values)
	if err != nil {
		return LaneFactor{}, err
	}
	return d.ApplyObjectGraphMutationFactor(mutation, input)
}

// ObjectGraphMutationLanes returns the exact registered dependent-product
// groups touched by a sealed graph mutation.
func (d ProductDomain) ObjectGraphMutationLanes(plan ObjectGraphMutationPlan) ([]ProductLane, error) {
	if !d.Valid() || !plan.Valid() || plan.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign object graph mutation", ErrInvalidLaneFactor)
	}
	out := make([]ProductLane, 0, 2)
	for laneIndex := range d.factorLanes {
		for _, coordinate := range d.factorLanes[laneIndex].coordinates {
			if coordinate.ops.objectMutation.active(plan) {
				out = append(out, d.factorLanes[laneIndex].lane)
				break
			}
		}
	}
	return out, nil
}

// ApplyObjectGraphMutationFactor is the sole object-graph implementation.
func (d ProductDomain) ApplyObjectGraphMutationFactor(plan ObjectGraphMutationPlan, input LaneFactor) (LaneFactor, error) {
	if !plan.Valid() || plan.seal != d.seal {
		return LaneFactor{}, ErrInvalidLaneFactor
	}
	runtime, err := d.validateFactor(input)
	if err != nil {
		return LaneFactor{}, err
	}
	if len(runtime.coordinates) == 0 {
		return input, nil
	}
	skeletons := make([]CoordinateFamilySkeleton, len(runtime.coordinates))
	scalars := make([][]CoordinateScalarFactor, len(runtime.coordinates))
	for familyIndex, coordinate := range runtime.coordinates {
		skeleton, old, decomposeErr := d.DecomposeCoordinateFamily(input, coordinate.family, plan.keys)
		if decomposeErr != nil {
			return LaneFactor{}, decomposeErr
		}
		next, pubs, participant, applyErr := d.applyCoordinateObjectMutationSkeleton(coordinate.family, skeleton, plan)
		if applyErr != nil {
			return LaneFactor{}, applyErr
		}
		if !participant {
			skeletons[familyIndex], scalars[familyIndex] = skeleton, old
			continue
		}
		out := make([]CoordinateScalarFactor, 0, len(old)+len(pubs))
		for _, scalar := range old {
			affected, affectedErr := d.coordinateObjectMutationAffects(plan, scalar.slot)
			if affectedErr != nil {
				return LaneFactor{}, affectedErr
			}
			if !affected {
				omitted, omitErr := d.CoordinateScalarIsOmitted(next, scalar)
				if omitErr != nil {
					return LaneFactor{}, omitErr
				}
				if !omitted {
					out = append(out, scalar)
				}
			}
		}
		for _, pub := range pubs {
			var current CoordinateScalarFactor
			for _, scalar := range old {
				if equal, _ := d.CoordinateSlotEqual(scalar.slot, pub.slot); equal {
					current = scalar
					break
				}
			}
			if current.payload == nil {
				// Scalar updates observe the pre-construction coordinate. The
				// topology transition and publications form one transaction; a
				// Join-style law must not read a default invented by next.
				current, err = d.CoordinateDefault(skeleton, pub.slot)
				if err != nil {
					current = CoordinateScalarFactor{}
				}
			}
			written, writeErr := d.applyCoordinateObjectMutationScalar(pub, current)
			if writeErr != nil {
				return LaneFactor{}, writeErr
			}
			omitted, omitErr := d.CoordinateScalarIsOmitted(next, written)
			if omitErr != nil {
				return LaneFactor{}, omitErr
			}
			if !omitted {
				out = append(out, written)
			}
		}
		sort.Slice(out, func(i, j int) bool { less, _ := d.CoordinateSlotLess(out[i].slot, out[j].slot); return less })
		skeletons[familyIndex], scalars[familyIndex] = next, out
	}
	return d.ComposeCoordinateFamilies(runtime.lane, plan.keys, skeletons, scalars)
}

// ApplyObjectConstructor executes the same complete-lane operation over every
// component of a concrete State. No second skeleton/scalar loop exists here.
func (d ProductDomain) ApplyObjectConstructor(plan ObjectConstructorPlan, values []ObjectConstructorValues, input State) (State, error) {
	mutation, err := d.objectConstructorMutationPlan(plan, values)
	if err != nil {
		return State{}, err
	}
	return d.ApplyObjectGraphMutation(mutation, input)
}

// ApplyObjectGraphMutation applies only the registered participant lanes and
// patches them back atomically; unrelated axes are neither decomposed nor
// scanned.
func (d ProductDomain) ApplyObjectGraphMutation(plan ObjectGraphMutationPlan, input State) (State, error) {
	lanes, err := d.ObjectGraphMutationLanes(plan)
	if err != nil {
		return State{}, err
	}
	factors, err := d.DecomposeLanes(input, lanes)
	if err != nil {
		return State{}, err
	}
	for laneIndex := range factors {
		factors[laneIndex], err = d.ApplyObjectGraphMutationFactor(plan, factors[laneIndex])
		if err != nil {
			return State{}, err
		}
	}
	return d.PatchLaneFactors(input, factors)
}
