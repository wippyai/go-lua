package heap

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// RawPresence is a finite may-mask for one partition cell. A present tuple is
// retained only when RawPresent is possible; Lua nil is therefore represented
// by RawAbsent, never by a fabricated present payload.
type RawPresence uint8

const (
	RawInvalid RawPresence = 0
	RawAbsent  RawPresence = 1 << iota
	RawPresent
)

const rawAll = RawAbsent | RawPresent

func (raw RawPresence) valid() bool               { return raw != RawInvalid && raw&^rawAll == 0 }
func (raw RawPresence) has(part RawPresence) bool { return raw&part != 0 }

// Shape and Frozen are finite may-masks. Constructors issue singleton masks;
// only canonical widening joins alternatives into a mask.
type Shape uint8

const (
	ShapeInvalid  Shape = 0
	ShapeEligible Shape = 1 << iota
	ShapeIneligible
)

const shapeAll = ShapeEligible | ShapeIneligible

func (shape Shape) valid() bool { return shape != ShapeInvalid && shape&^shapeAll == 0 }

// singleton reports whether shape denotes exactly one concrete header state.
// Generalized masks are valid abstract observations, but they arise only from
// canonical merge/Widen; they are never issued by a seed constructor.
func (shape Shape) singleton() bool {
	return shape == ShapeEligible || shape == ShapeIneligible
}

type Frozen uint8

const (
	FrozenInvalid Frozen = 0
	FrozenMutable Frozen = 1 << iota
	FrozenFrozen
)

const frozenAll = FrozenMutable | FrozenFrozen

func (frozen Frozen) valid() bool { return frozen != FrozenInvalid && frozen&^frozenAll == 0 }

// singleton reports whether frozen denotes exactly one concrete header state.
// As with Shape, joined masks remain valid inside Object but cannot enter the
// carrier through Schema.Object.
func (frozen Frozen) singleton() bool {
	return frozen == FrozenMutable || frozen == FrozenFrozen
}

// Present is one complete raw-present tuple. The source slot remains private
// provenance for admission; it is not a key identity. Key equality is carried
// only by a Link key, an exact Heap reference, or a kind selector.
type Present struct {
	owner            *schema
	slotID           uint32
	payloadID        uint32
	valueContainment Containment
	keyContainment   Containment
}

func (present Present) valid() bool {
	return present.owner != nil && present.slotID != 0 && int(present.slotID) <= len(present.owner.slots) &&
		present.payloadID != 0 && int(present.payloadID) <= len(present.owner.payloads) &&
		present.valueContainment.valid() && present.valueContainment.owner == present.owner &&
		present.keyContainment.valid() && present.keyContainment.owner == present.owner
}

// Slot returns the sealed structural storage source that supplied this tuple.
func (present Present) Slot() (Slot, bool) {
	if !present.valid() {
		return Slot{}, false
	}
	return Slot{owner: present.owner, id: present.slotID}, true
}

func (present Present) Payload() (Payload, bool) {
	if !present.valid() {
		return Payload{}, false
	}
	return Payload{owner: present.owner, id: present.payloadID}, true
}

func (present Present) Containment() (Containment, Containment, bool) {
	if !present.valid() {
		return Containment{}, Containment{}, false
	}
	return present.valueContainment, present.keyContainment, true
}

// CellState is the complete state at one semantic key-partition coordinate.
// It deliberately carries no selector or cover: a stored Partition owns the
// complete disjoint coordinate system. RawAbsent has no payload; every
// payload and containment edge lives inside a complete RawPresent tuple.
type CellState struct {
	owner    *schema
	raw      RawPresence
	presents []Present
}

// valid is the constant-time residue of the construction proof: owner
// fencing and the raw/presents representation correlation. Present validity
// and strict ascending order are proved once by canonicalCellState over a
// presents image that is never mutated in place after it becomes a
// CellState, so no algebra entry re-derives them.
func (state CellState) valid() bool {
	return state.owner != nil && state.raw.valid() && state.raw.has(RawPresent) == (len(state.presents) != 0)
}

// samePresents recognizes one shared immutable presents image. A CellState's
// presents slice is written only before construction and never mutated in
// place afterward, so a shared backing array with the same length already is
// the same normalized set.
func samePresents(left, right []Present) bool {
	return len(left) == len(right) && (len(left) == 0 || &left[0] == &right[0])
}

// canonicalCellState is the single construction gate for a fresh CellState
// presents image. It is the one place ownership, individual Present
// validity, and strict Present ordering are proved.
func canonicalCellState(owner *schema, raw RawPresence, presents []Present) (CellState, bool) {
	if owner == nil || !raw.valid() || raw.has(RawPresent) != (len(presents) != 0) {
		return CellState{}, false
	}
	for index, present := range presents {
		if !present.valid() || present.owner != owner || index != 0 && comparePresent(presents[index-1], present) >= 0 {
			return CellState{}, false
		}
	}
	return CellState{owner: owner, raw: raw, presents: presents}, true
}

func (state CellState) Valid() bool { return state.valid() }

func (state CellState) Raw() (RawPresence, bool) {
	if !state.valid() {
		return RawInvalid, false
	}
	return state.raw, true
}

func (state CellState) PresentCount() int {
	if !state.valid() {
		return 0
	}
	return len(state.presents)
}

func (state CellState) PresentAt(index int) (Present, bool) {
	if !state.valid() || index < 0 || index >= len(state.presents) {
		return Present{}, false
	}
	return state.presents[index], true
}

// CellPresent creates a singleton raw-present state. The operation that uses
// it separately supplies a KeySelector; it cannot smuggle selection into
// stored state.
func (schema Schema) CellPresent(slot Slot, payload Payload, valueChild, keyChild Containment) (CellState, bool) {
	if !schema.valid() || !slot.valid() || slot.owner != schema.owner ||
		!payload.valid() || payload.owner != schema.owner || !valueChild.valid() || valueChild.owner != schema.owner ||
		!keyChild.valid() || keyChild.owner != schema.owner {
		return CellState{}, false
	}
	present := Present{owner: schema.owner, slotID: slot.id, payloadID: payload.id, valueContainment: valueChild, keyContainment: keyChild}
	return canonicalCellState(schema.owner, RawPresent, []Present{present})
}

// CellAbsent creates the one raw-absence state. It intentionally has no
// source payload, containment fields, or selector.
func (schema Schema) CellAbsent() (CellState, bool) {
	if !schema.valid() {
		return CellState{}, false
	}
	return canonicalCellState(schema.owner, RawAbsent, nil)
}

// CellUnion is the exact pointwise least upper bound of two complete cell
// states, issued by the same authority that issues the states themselves. It
// is the join the canonical partition merge already performs, exposed so a
// construction owner can represent one cell whose stored alternatives are a
// disjunction instead of forking one whole World per alternative. It adds no
// containment kind and no ordering: presents remain a normalized set and raw
// remains a may-mask.
func (schema Schema) CellUnion(left, right CellState) (CellState, bool) {
	if !schema.valid() || !left.valid() || left.owner != schema.owner || !right.valid() || right.owner != schema.owner {
		return CellState{}, false
	}
	return mergeCellStatesAdmitted(left, right)
}

// Every runtime family but nil is a legal Lua table key. That partition is the
// vocabulary's own, so the count, the enumeration, and the admission test are
// projections of it rather than an ordinal range restated here.
var legalKeyKindCount = runtimekind.NonNil.Members()

func legalKeyKindAt(index int) (runtimekind.Kind, bool) {
	return runtimekind.NonNil.MemberAt(index)
}

func legalKeyKind(kind runtimekind.Kind) bool {
	return runtimekind.NonNil.Contains(kind)
}

// Partition is a complete, canonical key-space partition. `rest[k]` covers
// every legal key of kind k except exactly the stored atomic exceptions whose
// owner-derived possible-kind mask includes k. Exceptions are sorted and
// retained only when their state differs from their derived residual baseline.
// There is no stored Unknown/Finite cover and no caller-supplied exclusion.
type Partition struct {
	owner      *schema
	rest       [runtimekind.Count]CellState
	exceptions []partitionException
}

type partitionException struct {
	atom  keyAtom
	state CellState
}

func emptyPartition(owner *schema) Partition {
	partition := Partition{owner: owner}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		partition.rest[kind] = CellState{owner: owner, raw: RawAbsent}
	}
	return partition
}

func (partition Partition) valid() bool {
	dbgHeap.PartitionValidations++
	if partition.owner == nil {
		return false
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !partition.rest[kind].valid() || partition.rest[kind].owner != partition.owner {
			return false
		}
	}
	for _, kind := range []runtimekind.Kind{runtimekind.Invalid, runtimekind.Nil} {
		if partition.rest[kind].owner != nil {
			return false
		}
	}
	for index, exception := range partition.exceptions {
		if !validExactKeyAtom(partition.owner, exception.atom) || !exception.state.valid() || exception.state.owner != partition.owner ||
			index != 0 && compareKeyAtom(partition.exceptions[index-1].atom, exception.atom) >= 0 {
			return false
		}
		baseline, ok := partition.defaultFor(exception.atom)
		if !ok || equalCellState(exception.state, baseline) {
			return false
		}
	}
	return true
}

func (partition Partition) defaultFor(atom keyAtom) (CellState, bool) {
	if partition.owner == nil || !validExactKeyAtom(partition.owner, atom) {
		return CellState{}, false
	}
	return partition.defaultForAdmitted(atom)
}

func (partition Partition) defaultForKinds(kinds runtimekind.Set) (CellState, bool) {
	if partition.owner == nil || kinds == 0 || kinds&^runtimekind.NonNil != 0 {
		return CellState{}, false
	}
	return partition.defaultForKindsAdmitted(kinds)
}

// defaultForAdmitted is the validation-free path for a canonical Partition.
// Its caller has already proved the atom and the complete partition; it only
// performs the semantic residual join.
func (partition Partition) defaultForAdmitted(atom keyAtom) (CellState, bool) {
	return partition.defaultForKindsAdmitted(keyAtomRuntimeKinds(partition.owner, atom))
}

func (partition Partition) defaultForKindsAdmitted(kinds runtimekind.Set) (CellState, bool) {
	dbgHeap.DefaultDerivations++
	if partition.owner == nil || kinds == 0 || kinds&^runtimekind.NonNil != 0 {
		return CellState{}, false
	}
	var result CellState
	found := false
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !kinds.Contains(kind) {
			continue
		}
		if !found {
			result, found = partition.rest[kind], true
			continue
		}
		var ok bool
		result, ok = mergeCellStatesAdmitted(result, partition.rest[kind])
		if !ok {
			return CellState{}, false
		}
	}
	return result, found
}

func (partition Partition) lookup(atom keyAtom) (CellState, bool) {
	if !partition.validShallow() || !validExactKeyAtom(partition.owner, atom) {
		return CellState{}, false
	}
	return partition.lookupAdmitted(atom)
}

func (partition Partition) lookupAdmitted(atom keyAtom) (CellState, bool) {
	index, found := partition.exceptionIndex(atom)
	if found {
		return partition.exceptions[index].state, true
	}
	return partition.defaultForAdmitted(atom)
}

// validShallow avoids recursive canonicality checks while lookup/defaultFor
// validates a prospective Partition during its own normalization.
func (partition Partition) validShallow() bool {
	if partition.owner == nil {
		return false
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !partition.rest[kind].valid() || partition.rest[kind].owner != partition.owner {
			return false
		}
	}
	for index, exception := range partition.exceptions {
		if !validExactKeyAtom(partition.owner, exception.atom) || !exception.state.valid() || exception.state.owner != partition.owner ||
			index != 0 && compareKeyAtom(partition.exceptions[index-1].atom, exception.atom) >= 0 {
			return false
		}
	}
	return true
}

func (partition Partition) exceptionIndex(atom keyAtom) (int, bool) {
	index := sort.Search(len(partition.exceptions), func(index int) bool {
		return compareKeyAtom(partition.exceptions[index].atom, atom) >= 0
	})
	return index, index < len(partition.exceptions) && compareKeyAtom(partition.exceptions[index].atom, atom) == 0
}

// Object is one full role-local object state. It keeps header observations,
// metatable alternatives, and one complete partition together; none can be
// published as an independent Heap fact.
type Object struct {
	owner       *schema
	shape       Shape
	frozen      Frozen
	noMeta      bool
	unknownMeta bool
	metatables  []Reference
	partition   Partition
}

func (object Object) valid() bool {
	if object.owner == nil || !object.shape.valid() || !object.frozen.valid() || !object.noMeta && !object.unknownMeta && len(object.metatables) == 0 {
		return false
	}
	for index, reference := range object.metatables {
		if !reference.valid() || reference.owner != object.owner || index != 0 && compareReference(object.metatables[index-1], reference) >= 0 {
			return false
		}
	}
	return object.partition.owner == object.owner && object.partition.valid()
}

func (object Object) Valid() bool { return object.valid() }

func (object Object) Header() (Shape, Frozen, bool) {
	if !object.valid() {
		return ShapeInvalid, FrozenInvalid, false
	}
	return object.shape, object.frozen, true
}

func (object Object) MayHaveNoMetatable() bool { return object.valid() && object.noMeta }

// MayHaveUnknownMetatable reports whether an opaque, untracked table may be
// the object's metatable. It is distinct from the no-metatable alternative.
func (object Object) MayHaveUnknownMetatable() bool { return object.valid() && object.unknownMeta }

func (object Object) MetatableCount() int {
	if !object.valid() {
		return 0
	}
	return len(object.metatables)
}

func (object Object) MetatableAt(index int) (Reference, bool) {
	if !object.valid() || index < 0 || index >= len(object.metatables) {
		return Reference{}, false
	}
	return object.metatables[index], true
}

// Object creates a singleton complete object state. The owner-issued
// containment fact explicitly selects no metatable, one exact tracked
// metatable, or an opaque untracked metatable. Its complete absent residual
// partition is created eagerly; writes add canonical exceptions through a
// KeySelector.
func (schema Schema) Object(shape Shape, frozen Frozen, metatable Containment) (Object, bool) {
	if !schema.valid() || !shape.singleton() || !frozen.singleton() || !metatable.valid() || metatable.owner != schema.owner {
		return Object{}, false
	}
	object := Object{owner: schema.owner, shape: shape, frozen: frozen, partition: emptyPartition(schema.owner)}
	switch metatable.kind {
	case ContainmentNone:
		object.noMeta = true
	case ContainmentUnknown:
		object.unknownMeta = true
	case ContainmentExact:
		object.metatables = []Reference{{owner: schema.owner, root: metatable.root, role: metatable.role}}
	default:
		return Object{}, false
	}
	if !object.valid() {
		return Object{}, false
	}
	return object, true
}

// WorldKind is a complete allocation cardinality/control alternative. Many
// is deliberately one World carrying Recent and Summary simultaneously; it
// is never encoded as two sibling alternatives.
type WorldKind uint8

const (
	WorldInvalid WorldKind = iota
	WorldZero
	WorldExact
	WorldOne
	WorldMany
)

func (kind WorldKind) valid() bool { return kind >= WorldZero && kind <= WorldMany }

// World is one complete control disjunct for a selected root.
type World struct {
	owner   *schema
	kind    WorldKind
	exact   Object
	recent  Object
	summary Object
}

func (world World) valid() bool {
	if world.owner == nil || !world.kind.valid() {
		return false
	}
	switch world.kind {
	case WorldZero:
		return world.exact.owner == nil && world.recent.owner == nil && world.summary.owner == nil
	case WorldExact:
		return world.exact.valid() && world.exact.owner == world.owner && world.recent.owner == nil && world.summary.owner == nil
	case WorldOne:
		return world.recent.valid() && world.recent.owner == world.owner && world.exact.owner == nil && world.summary.owner == nil
	case WorldMany:
		return world.recent.valid() && world.summary.valid() && world.recent.owner == world.owner && world.summary.owner == world.owner && world.exact.owner == nil
	default:
		return false
	}
}

func (world World) Valid() bool { return world.valid() }
func (world World) Kind() WorldKind {
	if !world.valid() {
		return WorldInvalid
	}
	return world.kind
}
func (world World) Exact() (Object, bool) {
	if !world.valid() || world.kind != WorldExact {
		return Object{}, false
	}
	return world.exact, true
}
func (world World) Recent() (Object, bool) {
	if !world.valid() || world.kind != WorldOne && world.kind != WorldMany {
		return Object{}, false
	}
	return world.recent, true
}
func (world World) Summary() (Object, bool) {
	if !world.valid() || world.kind != WorldMany {
		return Object{}, false
	}
	return world.summary, true
}

func (schema Schema) Zero(key Key) (World, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootAllocation {
		return World{}, false
	}
	return World{owner: schema.owner, kind: WorldZero}, true
}

func (schema Schema) Exact(key Key, object Object) (World, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootBoot || !object.valid() || object.owner != schema.owner {
		return World{}, false
	}
	return World{owner: schema.owner, kind: WorldExact, exact: object}, true
}

func (schema Schema) One(key Key, recent Object) (World, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootAllocation || !recent.valid() || recent.owner != schema.owner {
		return World{}, false
	}
	return World{owner: schema.owner, kind: WorldOne, recent: recent}, true
}

func (schema Schema) Many(key Key, recent, summary Object) (World, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootAllocation || !recent.valid() || !summary.valid() || recent.owner != schema.owner || summary.owner != schema.owner {
		return World{}, false
	}
	return World{owner: schema.owner, kind: WorldMany, recent: recent, summary: summary}, true
}

// Value is Bottom, Top, or a finite sorted disjunction of complete Worlds.
// Its outer alternatives are control worlds only; a Many world's two roles
// coexist in the same element and cannot be recombined as independent state.
type Value struct {
	owner  *schema
	top    bool
	worlds []World
}

func (value Value) valid() bool {
	if value.owner == nil {
		return false
	}
	if value.top {
		return len(value.worlds) == 0
	}
	for index, world := range value.worlds {
		if !world.valid() || world.owner != value.owner || index != 0 && compareWorld(value.worlds[index-1], world) >= 0 {
			return false
		}
	}
	return true
}

func (schema Schema) Bottom() Value {
	if !schema.valid() {
		return Value{}
	}
	return schema.owner.bottom
}
func (schema Schema) Default() Value { return schema.Bottom() }
func (schema Schema) Top() Value {
	if !schema.valid() {
		return Value{}
	}
	return schema.owner.top
}
func (value Value) Valid() bool    { return value.valid() }
func (value Value) IsBottom() bool { return value.valid() && !value.top && len(value.worlds) == 0 }
func (value Value) IsTop() bool    { return value.valid() && value.top }
func (value Value) WorldCount() int {
	if !value.valid() || value.top {
		return 0
	}
	return len(value.worlds)
}
func (value Value) WorldAt(index int) (World, bool) {
	if !value.valid() || value.top || index < 0 || index >= len(value.worlds) {
		return World{}, false
	}
	return value.worlds[index], true
}

// ExactSelector creates the singleton already-normalized exact-key selection.
// The literal is matched against Heap's sealed quotient, so caller input
// cannot mint an unseen key or carry a Project coordinate.
func (schema Schema) ExactSelector(literal keyspace.LiteralValue) (KeySelector, bool) {
	if !schema.valid() || literal.Kind == 0 {
		return KeySelector{}, false
	}
	ordinal := schema.owner.exactIndex[literal]
	if ordinal == 0 || int(ordinal) > len(schema.owner.exactKeys) {
		return KeySelector{}, false
	}
	return KeySelector{owner: schema.owner, atoms: []keyAtom{{kind: keyAtomExact, exactOrdinal: ordinal}}}, true
}

// ReferenceSelector creates a singleton exact rooted-key selection. Summary
// is whole-role information, not one exact object, and is therefore rejected.
func (schema Schema) ReferenceSelector(reference Reference) (KeySelector, bool) {
	if !schema.valid() || !reference.valid() || reference.owner != schema.owner || reference.role == materialization.Summary {
		return KeySelector{}, false
	}
	selector := KeySelector{owner: schema.owner, atoms: []keyAtom{{kind: keyAtomReference, root: reference.root, role: reference.role}}}
	return selector, selector.valid()
}

func (schema Schema) KindSelector() (KeySelector, bool) {
	return schema.KindsSelector(runtimekind.NonNil)
}

// KindsSelector is the explicit typed loss of key identity. Source
// occurrences, strings, decoded syntax, and caller-supplied exclusions never
// enter this representation.
func (schema Schema) KindsSelector(kinds runtimekind.Set) (KeySelector, bool) {
	if !schema.valid() || !kinds.Valid() || kinds == 0 || kinds&^runtimekind.NonNil != 0 {
		return KeySelector{}, false
	}
	return KeySelector{owner: schema.owner, kinds: kinds}, true
}

// SelectorForSlot keeps structural source incidence cold while refusing to use
// a dynamic source Value as runtime equality. Dynamic and open-tail sources
// enter only the kind-masked selector until Numeric/equality installs a
// separate typed descriptor.
func (schema Schema) SelectorForSlot(slot Slot) (KeySelector, bool) {
	if !schema.valid() || !slot.valid() || slot.owner != schema.owner {
		return KeySelector{}, false
	}
	kind, exact, _, ok := slot.Origin()
	if !ok {
		return KeySelector{}, false
	}
	if kind == SlotExact {
		literal, literalOK := exact.Literal()
		if !literalOK {
			return KeySelector{}, false
		}
		return schema.ExactSelector(literal)
	}
	return schema.KindSelector()
}

// Relation admits complete worlds only. It has no legacy bridge and cannot add
// a standalone slot, root, metatable, or role edge.
func (schema Schema) Relation(key Key, worlds ...World) (Value, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner {
		return Value{}, false
	}
	if len(worlds) == 0 {
		return schema.Bottom(), true
	}
	normalized := normalizeWorlds(worlds)
	value := Value{owner: schema.owner, worlds: normalized}
	if !value.valid() || !schema.Admits(key, value) {
		return Value{}, false
	}
	return value, true
}

// EmptyObject is the complete zero-object state of an allocation root.
func (schema Schema) EmptyObject(key Key) (Value, bool) {
	world, ok := schema.Zero(key)
	if !ok {
		return Value{}, false
	}
	return schema.Relation(key, world)
}

// IngressFact is Heap's zero-input allocation-ingress reducer. It rederives
// the WorldZero image sealed for one owner-issued allocation Key.
func IngressFact(key Key) (Value, structure.ReductionOutcome) {
	if !key.valid() {
		return Value{}, structure.Refuse
	}
	fact, ok := Schema{owner: key.owner}.EmptyObject(key)
	if !ok {
		return Value{}, structure.Refuse
	}
	return fact, structure.Concrete
}

// Ingress rederives the allocation Key coordinate and its WorldZero fact.
func (key Key) Ingress() (Key, Value, bool) {
	fact, outcome := IngressFact(key)
	if outcome != structure.Concrete {
		return Key{}, Value{}, false
	}
	return key, fact, true
}

// Age maps all nested Recent references to allocationKey into Summary.  It
// is total for every owned Value and every allocation key of this Schema:
// Top and Bottom are preserved, and no coordinate/host admission is needed.
//
// The implementation is one structural homomorphism followed by the same
// canonical normalizers used by Join.  RecentToSummary is idempotent, while
// every affected finite set is normalized through union/pointwise join; thus
// Age is monotone, idempotent, default-preserving, and join-homomorphic by
// construction.  Roots other than allocationKey are unchanged.
func (schema Schema) Age(value Value, allocationKey Key) (Value, bool) {
	if !schema.valid() || !schema.owns(value) || !allocationKey.valid() || allocationKey.owner != schema.owner || allocationKey.Kind() != RootAllocation {
		return Value{}, false
	}
	return schema.age(value, allocationKey.slot)
}

// Age is the allocation carry transition published on the coordinate that
// carries its own owner. It is the transform every allocation-form rule
// carries with, stated once here rather than once per constructor
// descriptor: a Key already fences its schema, so the transition needs no
// source-side descriptor and no allocation topology query.
func (key Key) Age(prior Value) (Value, bool) {
	if !key.valid() {
		return Value{}, false
	}
	return Schema{owner: key.owner}.Age(prior, key)
}

func (schema Schema) age(value Value, selected uint32) (Value, bool) {
	root, rootOK := schema.owner.rootAt(selected)
	if !schema.owns(value) || !rootOK || root.kind != RootAllocation {
		return Value{}, false
	}
	if value.top || value.IsBottom() {
		return value, true
	}
	// Do not materialize an image until the first selected Recent edge is
	// actually found.  Age is hot on unrelated Heap coordinates, and Value is
	// immutable: retaining its slice is both the correct no-op result and the
	// allocation-free fast path.
	var worlds []World
	for index, world := range value.worlds {
		next, changed, ok := ageWorld(world, selected)
		if !ok {
			return Value{}, false
		}
		if !changed {
			continue
		}
		if worlds == nil {
			worlds = make([]World, len(value.worlds))
			copy(worlds, value.worlds)
		}
		worlds[index] = next
	}
	if worlds == nil {
		return value, true
	}
	aged := Value{owner: schema.owner, worlds: normalizeWorldsOwned(worlds)}
	return aged, aged.valid()
}

// Create is the only principal exact constructor. It is atomic: Zero becomes
// One(fresh), One(old) becomes Many(fresh, old), and Many(old,summary) becomes
// Many(fresh, join(summary, old)).  Its predecessor is aged as one whole
// Heap value before the fresh object is inserted, so no old Recent reference
// can leak into the successor and fresh itself is never aged.
func (schema Schema) Create(predecessor Value, allocationKey Key, fresh Object) (Value, bool) {
	if !schema.valid() || !allocationKey.valid() || allocationKey.owner != schema.owner || allocationKey.Kind() != RootAllocation || !fresh.valid() || fresh.owner != schema.owner {
		return Value{}, false
	}
	key := allocationKey
	if !schema.owns(predecessor) || !schema.Admits(key, predecessor) {
		return Value{}, false
	}
	aged, agedOK := schema.age(predecessor, key.slot)
	if !agedOK {
		return Value{}, false
	}
	if aged.top || aged.IsBottom() {
		return aged, true
	}
	worlds := make([]World, 0, len(aged.worlds))
	for _, world := range aged.worlds {
		switch world.kind {
		case WorldZero:
			worlds = append(worlds, World{owner: schema.owner, kind: WorldOne, recent: fresh})
		case WorldOne:
			worlds = append(worlds, World{owner: schema.owner, kind: WorldMany, recent: fresh, summary: world.recent})
		case WorldMany:
			summary, ok := mergeObjects(world.summary, world.recent)
			if !ok {
				return Value{}, false
			}
			worlds = append(worlds, World{owner: schema.owner, kind: WorldMany, recent: fresh, summary: summary})
		default:
			return Value{}, false
		}
	}
	return schema.Relation(key, worlds...)
}

func validExactKeyAtoms(owner *schema, atoms []keyAtom) bool {
	if owner == nil || len(atoms) == 0 {
		return false
	}
	for index, atom := range atoms {
		if !validExactKeyAtom(owner, atom) || index != 0 && compareKeyAtom(atoms[index-1], atom) >= 0 {
			return false
		}
	}
	return true
}

func validExactKeyAtom(owner *schema, atom keyAtom) bool {
	if owner == nil {
		return false
	}
	switch atom.kind {
	case keyAtomExact:
		return atom.exactOrdinal != 0 && int(atom.exactOrdinal) <= len(owner.exactKeys) && keyAtomRuntimeKinds(owner, atom) != 0
	case keyAtomReference:
		return atom.role != materialization.Summary && owner.admitsReferenceRole(atom.root, atom.role) && keyAtomRuntimeKinds(owner, atom) != 0
	default:
		return false
	}
}

func normalizeKeyAtoms(atoms []keyAtom) []keyAtom {
	if len(atoms) == 0 {
		return nil
	}
	items := append([]keyAtom(nil), atoms...)
	sort.Slice(items, func(left, right int) bool { return compareKeyAtom(items[left], items[right]) < 0 })
	end := 1
	for index := 1; index < len(items); index++ {
		if compareKeyAtom(items[end-1], items[index]) != 0 {
			items[end] = items[index]
			end++
		}
	}
	return items[:end]
}

func keyAtomRuntimeKinds(owner *schema, atom keyAtom) runtimekind.Set {
	if owner == nil {
		return 0
	}
	switch atom.kind {
	case keyAtomExact:
		if atom.exactOrdinal == 0 || int(atom.exactOrdinal) > len(owner.exactKeys) {
			return 0
		}
		literal := owner.exactKeys[atom.exactOrdinal-1].literal
		switch literal.Kind {
		case keyspace.LiteralBool:
			return runtimekind.Bit(runtimekind.Boolean)
		case keyspace.LiteralInteger, keyspace.LiteralFloat:
			return runtimekind.Bit(runtimekind.Number)
		case keyspace.LiteralString:
			return runtimekind.Bit(runtimekind.String)
		}
	case keyAtomReference:
		kinds, ok := owner.rootRuntimeKinds(atom.root)
		if !ok {
			return 0
		}
		return kinds
	}
	return 0
}

func compareKeyAtom(left, right keyAtom) int {
	if left.kind != right.kind {
		if left.kind < right.kind {
			return -1
		}
		return 1
	}
	switch left.kind {
	case keyAtomExact:
		if left.exactOrdinal < right.exactOrdinal {
			return -1
		}
		if left.exactOrdinal > right.exactOrdinal {
			return 1
		}
	case keyAtomReference:
		if left.root < right.root {
			return -1
		}
		if left.root > right.root {
			return 1
		}
		if left.role < right.role {
			return -1
		}
		if left.role > right.role {
			return 1
		}
	}
	return 0
}

func equalKeySelector(left, right KeySelector) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner || left.kinds != right.kinds || len(left.atoms) != len(right.atoms) {
		return false
	}
	for index := range left.atoms {
		if compareKeyAtom(left.atoms[index], right.atoms[index]) != 0 {
			return false
		}
	}
	return true
}

func (selector KeySelector) exactSelection() bool {
	return selector.valid() && selector.kinds == 0 && len(selector.atoms) == 1
}

func comparePresent(left, right Present) int {
	for _, pair := range [][2]uint32{{left.slotID, right.slotID}, {left.payloadID, right.payloadID}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if compared := compareContainment(left.valueContainment, right.valueContainment); compared != 0 {
		return compared
	}
	return compareContainment(left.keyContainment, right.keyContainment)
}

func compareContainment(left, right Containment) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if left.root < right.root {
		return -1
	}
	if left.root > right.root {
		return 1
	}
	if left.role < right.role {
		return -1
	}
	if left.role > right.role {
		return 1
	}
	return 0
}

// normalizePresentsOwned canonicalizes a caller-owned slice in place.  It is
// used by copy-on-write transforms so one changed CellState needs exactly one
// slice image, never a defensive copy of its fresh image.
func normalizePresentsOwned(items []Present) []Present {
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(left, right int) bool { return comparePresent(items[left], items[right]) < 0 })
	end := 1
	for index := 1; index < len(items); index++ {
		if comparePresent(items[end-1], items[index]) != 0 {
			items[end] = items[index]
			end++
		}
	}
	return items[:end]
}

func mergeCellStates(left, right CellState) (CellState, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return CellState{}, false
	}
	return mergeCellStatesAdmitted(left, right)
}

// mergeCellStatesAdmitted is the exact pointwise LUB of two owned cell
// states. left and right each already carry a proven sorted, deduped
// presents image, so their OR'd raw mask automatically carries RawPresent
// whenever the merged presents are non-empty and neither operand needs a
// second proof pass when one side contributes nothing new.
func mergeCellStatesAdmitted(left, right CellState) (CellState, bool) {
	if left.owner == nil || left.owner != right.owner {
		return CellState{}, false
	}
	if len(left.presents) == 0 {
		return CellState{owner: left.owner, raw: left.raw | right.raw, presents: right.presents}, true
	}
	if len(right.presents) == 0 {
		return CellState{owner: left.owner, raw: left.raw | right.raw, presents: left.presents}, true
	}
	if samePresents(left.presents, right.presents) {
		return CellState{owner: left.owner, raw: left.raw | right.raw, presents: left.presents}, true
	}
	// One operand already contains the other's raw mask and complete present
	// set exactly when it is the pointwise LUB, so reuse it unchanged rather
	// than rebuilding an image cellStateLessOrEqAdmitted already proves is
	// redundant.
	if cellStateLessOrEqAdmitted(left, right) {
		return right, true
	}
	if cellStateLessOrEqAdmitted(right, left) {
		return left, true
	}
	return canonicalCellState(left.owner, left.raw|right.raw, mergePresentsSorted(left.presents, right.presents))
}

// mergePresentsSorted is the linear pointwise union of two already sorted,
// deduped Present sequences. Each input is itself a proven CellState image,
// so one ascending merge pass reaches the union in final sorted form without
// a discarded concatenation or a generic sort.
func mergePresentsSorted(left, right []Present) []Present {
	result := make([]Present, 0, len(left)+len(right))
	leftAt, rightAt := 0, 0
	for leftAt < len(left) && rightAt < len(right) {
		switch compared := comparePresent(left[leftAt], right[rightAt]); {
		case compared < 0:
			result = append(result, left[leftAt])
			leftAt++
		case compared > 0:
			result = append(result, right[rightAt])
			rightAt++
		default:
			result = append(result, left[leftAt])
			leftAt++
			rightAt++
		}
	}
	result = append(result, left[leftAt:]...)
	result = append(result, right[rightAt:]...)
	return result
}

func equalCellState(left, right CellState) bool {
	return left.valid() && right.valid() && left.owner == right.owner && compareCellState(left, right) == 0
}

func cellStateLessOrEq(left, right CellState) bool {
	return left.valid() && right.valid() && cellStateLessOrEqAdmitted(left, right)
}

func cellStateLessOrEqAdmitted(left, right CellState) bool {
	if left.owner != right.owner || left.raw&^right.raw != 0 {
		return false
	}
	return samePresents(left.presents, right.presents) || presentSetSubset(left.presents, right.presents)
}

func compareCellState(left, right CellState) int {
	if left.raw < right.raw {
		return -1
	}
	if left.raw > right.raw {
		return 1
	}
	if samePresents(left.presents, right.presents) {
		return 0
	}
	for index := 0; index < len(left.presents) && index < len(right.presents); index++ {
		if compared := comparePresent(left.presents[index], right.presents[index]); compared != 0 {
			return compared
		}
	}
	if len(left.presents) < len(right.presents) {
		return -1
	}
	if len(left.presents) > len(right.presents) {
		return 1
	}
	return 0
}

func presentSetSubset(left, right []Present) bool {
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left); {
		if rightIndex == len(right) {
			return false
		}
		compared := comparePresent(left[leftIndex], right[rightIndex])
		switch {
		case compared == 0:
			leftIndex++
			rightIndex++
		case compared < 0:
			return false
		default:
			rightIndex++
		}
	}
	return true
}

// partitionWith normalizes the only stored key partition representation. It
// never accepts a caller-provided residual exclusion: exceptions themselves
// are the exclusion set. Duplicate atoms are joined, then retained only when
// their exact pointwise state differs from the derived residual baseline.
func partitionWith(owner *schema, rest [runtimekind.Count]CellState, source []partitionException) (Partition, bool) {
	if owner == nil {
		return Partition{}, false
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !rest[kind].valid() || rest[kind].owner != owner {
			return Partition{}, false
		}
	}
	items := append([]partitionException(nil), source...)
	for _, exception := range items {
		if !validExactKeyAtom(owner, exception.atom) || !exception.state.valid() || exception.state.owner != owner {
			return Partition{}, false
		}
	}
	sort.Slice(items, func(left, right int) bool { return compareKeyAtom(items[left].atom, items[right].atom) < 0 })
	merged := items[:0]
	for _, exception := range items {
		if len(merged) == 0 || compareKeyAtom(merged[len(merged)-1].atom, exception.atom) != 0 {
			merged = append(merged, exception)
			continue
		}
		state, ok := mergeCellStates(merged[len(merged)-1].state, exception.state)
		if !ok {
			return Partition{}, false
		}
		merged[len(merged)-1].state = state
	}
	partition := Partition{owner: owner, rest: rest, exceptions: merged}
	canonical := partition.exceptions[:0]
	for _, exception := range partition.exceptions {
		baseline, ok := partition.defaultFor(exception.atom)
		if !ok {
			return Partition{}, false
		}
		if !equalCellState(exception.state, baseline) {
			canonical = append(canonical, exception)
		}
	}
	partition.exceptions = canonical
	return partition, partition.valid()
}

func (partition Partition) update(selector KeySelector, replacement CellState, strong bool) (Partition, bool) {
	if !partition.valid() || !selector.valid() || selector.owner != partition.owner || !replacement.valid() || replacement.owner != partition.owner ||
		strong && !selector.exactSelection() {
		return Partition{}, false
	}
	rest := partition.rest
	exceptions := append([]partitionException(nil), partition.exceptions...)
	applyAtom := func(atom keyAtom) bool {
		index := sort.Search(len(exceptions), func(index int) bool { return compareKeyAtom(exceptions[index].atom, atom) >= 0 })
		var current CellState
		if index < len(exceptions) && compareKeyAtom(exceptions[index].atom, atom) == 0 {
			current = exceptions[index].state
		} else {
			var ok bool
			current, ok = partition.defaultFor(atom)
			if !ok {
				return false
			}
		}
		next := replacement
		if !strong {
			var ok bool
			next, ok = mergeCellStates(current, replacement)
			if !ok {
				return false
			}
		}
		if index < len(exceptions) && compareKeyAtom(exceptions[index].atom, atom) == 0 {
			exceptions[index].state = next
			return true
		}
		exceptions = append(exceptions, partitionException{})
		copy(exceptions[index+1:], exceptions[index:])
		exceptions[index] = partitionException{atom: atom, state: next}
		return true
	}
	if selector.kinds == 0 {
		for _, atom := range selector.atoms {
			if !applyAtom(atom) {
				return Partition{}, false
			}
		}
		return partitionWith(partition.owner, rest, exceptions)
	}
	if strong {
		return Partition{}, false
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !selector.kinds.Contains(kind) {
			continue
		}
		next, ok := mergeCellStates(rest[kind], replacement)
		if !ok {
			return Partition{}, false
		}
		rest[kind] = next
	}
	for index := range exceptions {
		if keyAtomRuntimeKinds(partition.owner, exceptions[index].atom)&selector.kinds == 0 {
			continue
		}
		next, ok := mergeCellStates(exceptions[index].state, replacement)
		if !ok {
			return Partition{}, false
		}
		exceptions[index].state = next
	}
	return partitionWith(partition.owner, rest, exceptions)
}

// partitionLessOrEq compares the complete pointwise semantic coordinates:
// one residual state per runtime kind and every atom explicit in either side.
func partitionLessOrEq(left, right Partition) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return false
	}
	return partitionLessOrEqAdmitted(left, right)
}

func partitionLessOrEqAdmitted(left, right Partition) bool {
	if left.owner != right.owner {
		return false
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !cellStateLessOrEqAdmitted(left.rest[kind], right.rest[kind]) {
			return false
		}
	}
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left.exceptions) || rightIndex < len(right.exceptions); {
		var atom keyAtom
		switch {
		case rightIndex == len(right.exceptions) || leftIndex < len(left.exceptions) && compareKeyAtom(left.exceptions[leftIndex].atom, right.exceptions[rightIndex].atom) < 0:
			atom = left.exceptions[leftIndex].atom
			leftIndex++
		case leftIndex == len(left.exceptions) || compareKeyAtom(left.exceptions[leftIndex].atom, right.exceptions[rightIndex].atom) > 0:
			atom = right.exceptions[rightIndex].atom
			rightIndex++
		default:
			atom = left.exceptions[leftIndex].atom
			leftIndex++
			rightIndex++
		}
		leftState, leftOK := left.lookupAdmitted(atom)
		rightState, rightOK := right.lookupAdmitted(atom)
		if !leftOK || !rightOK || !cellStateLessOrEqAdmitted(leftState, rightState) {
			return false
		}
	}
	return true
}

// mergePartitions is the exact pointwise LUB. A one-sided exception remains
// when its joined coordinate still differs from the derived residual default;
// it is never blindly folded merely because it occurs on one side.
func mergePartitions(left, right Partition) (Partition, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return Partition{}, false
	}
	rest := left.rest
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		state, ok := mergeCellStates(left.rest[kind], right.rest[kind])
		if !ok {
			return Partition{}, false
		}
		rest[kind] = state
	}
	exceptions := make([]partitionException, 0, len(left.exceptions)+len(right.exceptions))
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left.exceptions) || rightIndex < len(right.exceptions); {
		var atom keyAtom
		switch {
		case rightIndex == len(right.exceptions) || leftIndex < len(left.exceptions) && compareKeyAtom(left.exceptions[leftIndex].atom, right.exceptions[rightIndex].atom) < 0:
			atom = left.exceptions[leftIndex].atom
			leftIndex++
		case leftIndex == len(left.exceptions) || compareKeyAtom(left.exceptions[leftIndex].atom, right.exceptions[rightIndex].atom) > 0:
			atom = right.exceptions[rightIndex].atom
			rightIndex++
		default:
			atom = left.exceptions[leftIndex].atom
			leftIndex++
			rightIndex++
		}
		leftState, leftOK := left.lookup(atom)
		rightState, rightOK := right.lookup(atom)
		state, merged := mergeCellStates(leftState, rightState)
		if !leftOK || !rightOK || !merged {
			return Partition{}, false
		}
		exceptions = append(exceptions, partitionException{atom: atom, state: state})
	}
	return partitionWith(left.owner, rest, exceptions)
}

func comparePartition(left, right Partition) int {
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if compared := compareCellState(left.rest[kind], right.rest[kind]); compared != 0 {
			return compared
		}
	}
	for index := 0; index < len(left.exceptions) && index < len(right.exceptions); index++ {
		if compared := compareKeyAtom(left.exceptions[index].atom, right.exceptions[index].atom); compared != 0 {
			return compared
		}
		if compared := compareCellState(left.exceptions[index].state, right.exceptions[index].state); compared != 0 {
			return compared
		}
	}
	if len(left.exceptions) < len(right.exceptions) {
		return -1
	}
	if len(left.exceptions) > len(right.exceptions) {
		return 1
	}
	return 0
}

func compareReference(left, right Reference) int {
	if left.root < right.root {
		return -1
	}
	if left.root > right.root {
		return 1
	}
	if left.role < right.role {
		return -1
	}
	if left.role > right.role {
		return 1
	}
	return 0
}

func mergeObjects(left, right Object) (Object, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return Object{}, false
	}
	return mergeObjectsAdmitted(left, right)
}

// mergeObjectsAdmitted is used after the enclosing Value/World has crossed
// the public validation boundary. mergePartitions still performs its own
// canonicalization check because it is the constructor of the exact stored
// partition; this avoids making a second unchecked partition authority.
func mergeObjectsAdmitted(left, right Object) (Object, bool) {
	if left.owner == nil || left.owner != right.owner {
		return Object{}, false
	}
	partition, ok := mergePartitions(left.partition, right.partition)
	if !ok {
		return Object{}, false
	}
	metas := normalizeReferences(append(append([]Reference(nil), left.metatables...), right.metatables...))
	object := Object{owner: left.owner, shape: left.shape | right.shape, frozen: left.frozen | right.frozen, noMeta: left.noMeta || right.noMeta, unknownMeta: left.unknownMeta || right.unknownMeta, metatables: metas, partition: partition}
	if !object.valid() {
		return Object{}, false
	}
	return object, true
}

func normalizeReferences(refs []Reference) []Reference {
	if len(refs) == 0 {
		return nil
	}
	items := append([]Reference(nil), refs...)
	return normalizeReferencesOwned(items)
}

// normalizeReferencesOwned is the copy-on-write counterpart of
// normalizeReferences.  Its input must not alias a published Object.
func normalizeReferencesOwned(items []Reference) []Reference {
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(left, right int) bool { return compareReference(items[left], items[right]) < 0 })
	end := 1
	for index := 1; index < len(items); index++ {
		if compareReference(items[end-1], items[index]) != 0 {
			items[end] = items[index]
			end++
		}
	}
	return items[:end]
}

func weakObjectCell(object Object, selector KeySelector, replacement CellState) (Object, bool) {
	if !object.valid() || !selector.valid() || selector.owner != object.owner || !replacement.valid() || replacement.owner != object.owner {
		return Object{}, false
	}
	partition, ok := object.partition.update(selector, replacement, false)
	if !ok {
		return Object{}, false
	}
	object.partition = partition
	return object, object.valid()
}

func overwriteObjectCell(object Object, selector KeySelector, replacement CellState) (Object, bool) {
	if !object.valid() || !selector.exactSelection() || selector.owner != object.owner || !replacement.valid() || object.owner != replacement.owner {
		return Object{}, false
	}
	partition, ok := object.partition.update(selector, replacement, true)
	if !ok {
		return Object{}, false
	}
	object.partition = partition
	return object, object.valid()
}

func ageWorld(world World, selected uint32) (World, bool, bool) {
	switch world.kind {
	case WorldExact:
		object, changed, ok := ageObject(world.exact, selected)
		if !ok {
			return World{}, false, false
		}
		if !changed {
			return world, false, true
		}
		world.exact = object
	case WorldOne:
		object, changed, ok := ageObject(world.recent, selected)
		if !ok {
			return World{}, false, false
		}
		if !changed {
			return world, false, true
		}
		world.recent = object
	case WorldMany:
		recent, recentChanged, recentOK := ageObject(world.recent, selected)
		summary, summaryChanged, summaryOK := ageObject(world.summary, selected)
		if !recentOK || !summaryOK {
			return World{}, false, false
		}
		if !recentChanged && !summaryChanged {
			return world, false, true
		}
		world.recent, world.summary = recent, summary
	case WorldZero:
		// Zero has no object-local references.
		return world, false, true
	default:
		return World{}, false, false
	}
	return world, true, world.valid()
}

func ageObject(object Object, selected uint32) (Object, bool, bool) {
	if !object.valid() {
		return Object{}, false, false
	}
	partition, partitionChanged, ok := agePartition(object.partition, selected)
	if !ok {
		return Object{}, false, false
	}
	metaChanged := false
	for _, reference := range object.metatables {
		if _, changed := ageReference(reference, selected); changed {
			metaChanged = true
			break
		}
	}
	if !metaChanged && !partitionChanged {
		return object, false, true
	}
	// Object is a value, but its metatable alternatives are slice-backed.  An
	// age successor must never rewrite a predecessor retained by solver/cache
	// evidence.  Clone that path only when it contains a selected edge.
	if metaChanged {
		metatables := append([]Reference(nil), object.metatables...)
		for index := range metatables {
			metatables[index], _ = ageReference(metatables[index], selected)
		}
		object.metatables = normalizeReferencesOwned(metatables)
	}
	object.partition = partition
	return object, true, object.valid()
}

func ageReference(reference Reference, selected uint32) (Reference, bool) {
	if reference.root == selected {
		if role, changed := materialization.RecentToSummary(reference.role); changed {
			reference.role = role
			return reference, true
		}
	}
	return reference, false
}

func ageCellState(state CellState, selected uint32) (CellState, bool) {
	if !state.valid() {
		return CellState{}, false
	}
	changed := false
	for _, present := range state.presents {
		if _, valueChanged := ageContainment(present.valueContainment, selected); valueChanged {
			changed = true
			break
		}
		if _, keyChanged := ageContainment(present.keyContainment, selected); keyChanged {
			changed = true
			break
		}
	}
	if !changed {
		return state, false
	}
	result := state
	result.presents = append([]Present(nil), state.presents...)
	for index := range result.presents {
		present := &result.presents[index]
		present.valueContainment, _ = ageContainment(present.valueContainment, selected)
		present.keyContainment, _ = ageContainment(present.keyContainment, selected)
	}
	result.presents = normalizePresentsOwned(result.presents)
	return result, true
}

func ageContainment(containment Containment, selected uint32) (Containment, bool) {
	if containment.kind == ContainmentExact && containment.root == selected {
		if role, changed := materialization.RecentToSummary(containment.role); changed {
			containment.role = role
			return containment, true
		}
	}
	return containment, false
}

// agePartition transports contained references and folds a selected
// Recent reference-key exception into its residual kinds. After creation that
// reference denotes the whole Summary role, never an exact key selector.
func agePartition(partition Partition, selected uint32) (Partition, bool, bool) {
	if !partition.valid() || selected == 0 || int(selected) > len(partition.owner.roots) {
		return Partition{}, false, false
	}
	rest := partition.rest
	changed := false
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		state, stateChanged := ageCellState(rest[kind], selected)
		if !state.valid() {
			return Partition{}, false, false
		}
		if stateChanged {
			rest[kind] = state
			changed = true
		}
	}
	exceptions := partition.exceptions
	exceptionsOwned := false
	for index, exception := range partition.exceptions {
		state, stateChanged := ageCellState(exception.state, selected)
		if !state.valid() {
			return Partition{}, false, false
		}
		if exception.atom.kind == keyAtomReference && exception.atom.root == selected && exception.atom.role == materialization.Recent {
			if !exceptionsOwned {
				exceptions = make([]partitionException, index, len(partition.exceptions))
				copy(exceptions, partition.exceptions[:index])
				exceptionsOwned = true
			}
			kinds := keyAtomRuntimeKinds(partition.owner, exception.atom)
			for index := 0; index < legalKeyKindCount; index++ {
				kind, _ := legalKeyKindAt(index)
				if !kinds.Contains(kind) {
					continue
				}
				next, ok := mergeCellStates(rest[kind], state)
				if !ok {
					return Partition{}, false, false
				}
				rest[kind] = next
			}
			changed = true
			continue
		}
		if stateChanged {
			if !exceptionsOwned {
				exceptions = make([]partitionException, index, len(partition.exceptions))
				copy(exceptions, partition.exceptions[:index])
				exceptionsOwned = true
			}
			exceptions = append(exceptions, partitionException{atom: exception.atom, state: state})
			changed = true
			continue
		}
		if exceptionsOwned {
			exceptions = append(exceptions, exception)
		}
	}
	if !changed {
		return partition, false, true
	}
	result := Partition{owner: partition.owner, rest: rest, exceptions: exceptions}
	if !result.validShallow() {
		return Partition{}, false, false
	}
	// Existing exceptions are already sorted and unique, and Age never changes
	// their atoms.  Only a residual change can make one redundant; preserve a
	// shared exception slice when none becomes redundant.
	drop := false
	for _, exception := range result.exceptions {
		baseline, ok := result.defaultFor(exception.atom)
		if !ok {
			return Partition{}, false, false
		}
		if equalCellState(exception.state, baseline) {
			drop = true
			break
		}
	}
	if drop {
		if !exceptionsOwned {
			exceptions = append([]partitionException(nil), result.exceptions...)
			result.exceptions = exceptions
		}
		kept := result.exceptions[:0]
		for _, exception := range result.exceptions {
			baseline, ok := result.defaultFor(exception.atom)
			if !ok {
				return Partition{}, false, false
			}
			if !equalCellState(exception.state, baseline) {
				kept = append(kept, exception)
			}
		}
		result.exceptions = kept
	}
	return result, true, result.valid()
}

func compareObject(left, right Object) int {
	if left.shape < right.shape {
		return -1
	}
	if left.shape > right.shape {
		return 1
	}
	if left.frozen < right.frozen {
		return -1
	}
	if left.frozen > right.frozen {
		return 1
	}
	if left.noMeta != right.noMeta {
		if !left.noMeta {
			return -1
		}
		return 1
	}
	if left.unknownMeta != right.unknownMeta {
		if !left.unknownMeta {
			return -1
		}
		return 1
	}
	for index := 0; index < len(left.metatables) && index < len(right.metatables); index++ {
		if compared := compareReference(left.metatables[index], right.metatables[index]); compared != 0 {
			return compared
		}
	}
	if len(left.metatables) < len(right.metatables) {
		return -1
	}
	if len(left.metatables) > len(right.metatables) {
		return 1
	}
	return comparePartition(left.partition, right.partition)
}

func cellStateWidenScore(owner *schema, state CellState) (uint64, bool) {
	if owner == nil || !state.valid() || state.owner != owner || owner.presentPotential == ^uint64(0) {
		return 0, false
	}
	capacity, ok := safeAdd(owner.presentPotential, 1)
	if !ok {
		return 0, false
	}
	used := uint64(len(state.presents))
	if state.raw.has(RawAbsent) {
		used++
	}
	if used > capacity {
		return 0, false
	}
	return capacity - used, true
}

// objectWidenScore is the compact sum of a fixed semantic coordinate vector:
// seven residual cells, every sealed atom coordinate compressed by its exact
// possible-kind mask, and the finite header powersets. It never ranks sparse
// exception storage itself, which may grow under an exact pointwise join.
func objectWidenScore(object Object) (uint64, bool) {
	if !object.valid() || object.owner == nil {
		return 0, false
	}
	score := uint64(0)
	add := func(value uint64) bool {
		next, ok := safeAdd(score, value)
		if ok {
			score = next
		}
		return ok
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		value, ok := cellStateWidenScore(object.owner, object.partition.rest[kind])
		if !ok || !add(value) {
			return 0, false
		}
	}
	var active [1 << runtimekind.Count]uint64
	for _, exception := range object.partition.exceptions {
		mask := keyAtomRuntimeKinds(object.owner, exception.atom)
		index := int(mask)
		if index == 0 || index >= len(active) || active[index] == ^uint64(0) {
			return 0, false
		}
		active[index]++
		value, ok := cellStateWidenScore(object.owner, exception.state)
		if !ok || !add(value) {
			return 0, false
		}
	}
	for index, total := range object.owner.atomMaskCounts {
		if total == 0 {
			continue
		}
		if active[index] > total {
			return 0, false
		}
		remaining := total - active[index]
		if remaining == 0 {
			continue
		}
		baseline, ok := object.partition.defaultForKinds(runtimekind.Set(index))
		if !ok {
			return 0, false
		}
		value, ok := cellStateWidenScore(object.owner, baseline)
		if !ok {
			return 0, false
		}
		weighted, ok := safeMul(remaining, value)
		if !ok || !add(weighted) {
			return 0, false
		}
	}
	shapeCount := uint64(0)
	if object.shape&ShapeEligible != 0 {
		shapeCount++
	}
	if object.shape&ShapeIneligible != 0 {
		shapeCount++
	}
	frozenCount := uint64(0)
	if object.frozen&FrozenMutable != 0 {
		frozenCount++
	}
	if object.frozen&FrozenFrozen != 0 {
		frozenCount++
	}
	if !add(2-shapeCount) || !add(2-frozenCount) {
		return 0, false
	}
	metaCapacity, ok := safeAdd(object.owner.referenceCount, 2)
	if !ok {
		return 0, false
	}
	metaUsed := uint64(len(object.metatables))
	if object.noMeta {
		metaUsed++
	}
	if object.unknownMeta {
		metaUsed++
	}
	if metaUsed > metaCapacity || !add(metaCapacity-metaUsed) {
		return 0, false
	}
	return score, true
}

func compareWorld(left, right World) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	for _, pair := range [][2]Object{{left.exact, right.exact}, {left.recent, right.recent}, {left.summary, right.summary}} {
		if pair[0].owner == nil || pair[1].owner == nil {
			if pair[0].owner == pair[1].owner {
				continue
			}
			if pair[0].owner == nil {
				return -1
			}
			return 1
		}
		if compared := compareObject(pair[0], pair[1]); compared != 0 {
			return compared
		}
	}
	return 0
}

func normalizeWorlds(source []World) []World {
	if len(source) == 0 {
		return nil
	}
	// A Relation and Join expose the same carrier: a sorted antichain of
	// complete worlds.  Keeping a dominated world here would make direct
	// construction observably different from Join and would retain a stale
	// correlated branch until a later operation happened to remove it.
	worlds := make([]World, 0, len(source))
	for _, candidate := range source {
		worlds = appendCompleteWorld(worlds, candidate)
	}
	sort.Slice(worlds, func(left, right int) bool { return compareWorld(worlds[left], worlds[right]) < 0 })
	return worlds
}

// normalizeWorldsOwned retains Relation's sorted antichain invariant without
// a second slice image after a copy-on-write Age traversal.  `result` never
// grows ahead of the source cursor, so the in-place reduction cannot overwrite
// an unvisited world.
func normalizeWorldsOwned(worlds []World) []World {
	result := worlds[:0]
	for _, candidate := range worlds {
		result = appendCompleteWorld(result, candidate)
	}
	sort.Slice(result, func(left, right int) bool { return compareWorld(result[left], result[right]) < 0 })
	return result
}

func (schema Schema) owns(value Value) bool {
	return schema.valid() && value.valid() && value.owner == schema.owner
}
