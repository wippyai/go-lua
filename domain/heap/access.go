package heap

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// RawRouteTag is the schema-local numeric root/role identity of one staged
// Heap selection. The Heap Ref itself is not retained by a later staged
// locator, so the tag keeps its materialization role explicit. It is
// transport only, not Heap state, a cache key, or a serialized identity; its
// owning Schema validates and decodes it at every use.
type RawRouteTag uint64

// RawPayloadTag is the schema-local ID of one sealed Payload row. Equal
// Payloads deliberately share their Value/Pack staged read across roots and
// worlds; each RawAccess retains its own containment and mutation policy.
// Its owning Schema resolves it at every use.
type RawPayloadTag uint64

// RawAccess is one complete Heap control/world alternative together with one
// owner-selected key fragment and one raw alternative.  Its unexported
// fragment retains policy exclusions: callers cannot recreate a writable
// selector that includes InitialFrozen boot slots.
type RawAccess struct {
	owner         *schema
	key           Key
	role          materialization.Role
	world         World
	object        Object
	cell          CellState
	fragment      rawAccessFragment
	initialFrozen bool
	top           bool
}

type rawAccessFragment struct {
	selector   KeySelector
	residual   bool
	exclusions []keyAtom
}

func (fragment rawAccessFragment) valid(owner *schema) bool {
	if owner == nil || !fragment.selector.valid() || fragment.selector.owner != owner {
		return false
	}
	if !fragment.residual {
		return len(fragment.exclusions) == 0 && fragment.selector.exactSelection()
	}
	if fragment.selector.kinds == 0 || fragment.selector.kinds&(fragment.selector.kinds-1) != 0 {
		return false
	}
	for index, atom := range fragment.exclusions {
		if !validExactKeyAtom(owner, atom) || keyAtomRuntimeKinds(owner, atom)&fragment.selector.kinds == 0 ||
			index != 0 && compareKeyAtom(fragment.exclusions[index-1], atom) >= 0 {
			return false
		}
	}
	return true
}

func (access RawAccess) valid() bool {
	if access.owner == nil || !access.key.valid() || access.key.owner != access.owner || !access.role.Valid() {
		return false
	}
	if access.top {
		return !access.initialFrozen && !access.world.valid() && !access.object.valid() && !access.cell.valid() && !access.fragment.selector.valid()
	}
	if !access.world.valid() || access.world.owner != access.owner || !access.object.valid() || access.object.owner != access.owner ||
		!access.cell.valid() || access.cell.owner != access.owner || !access.fragment.valid(access.owner) {
		return false
	}
	selected, ok := rawWorldObject(access.world, access.role)
	return ok && compareObject(selected, access.object) == 0
}

// Valid reports whether this is an owner-issued transient raw-access route.
func (access RawAccess) Valid() bool { return access.valid() }

func (access RawAccess) IsTop() bool { return access.valid() && access.top }

// Object returns the complete selected role object.  It is unavailable for
// Top, which deliberately has no fabricated header, metatable, or cell.
func (access RawAccess) Object() (Object, bool) {
	if !access.valid() || access.top {
		return Object{}, false
	}
	return access.object, true
}

// Cell returns exactly one raw alternative: RawAbsent or one RawPresent
// tuple.  A public caller therefore cannot accidentally pair a payload from
// one stored tuple with containment from another.
func (access RawAccess) Cell() (CellState, bool) {
	if !access.valid() || access.top {
		return CellState{}, false
	}
	return access.cell, true
}

// InitialFrozen reports Target's immutable exact boot-slot policy for this
// route.  It is independent of Object.Header's table.freeze predicate.
func (access RawAccess) InitialFrozen() bool {
	return access.valid() && !access.top && access.initialFrozen
}

// RouteTag issues the compact staged route projection for key and role. The
// returned tag has no meaning until this exact Schema admits it again.
func (schema Schema) RouteTag(key Key, role materialization.Role) (RawRouteTag, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || !role.Valid() || !schema.admitsReferenceRole(key.slot, role) {
		return 0, false
	}
	return rawRouteTag(key, role)
}

// PayloadTag returns the sealed source-row identity for the sole Present
// tuple in this RawAccess. A foreign or nonselected Present fails closed.
func (access RawAccess) PayloadTag(present Present) (RawPayloadTag, bool) {
	if !access.valid() || access.top || !present.valid() || present.owner != access.owner || access.cell.PresentCount() != 1 {
		return 0, false
	}
	selected, ok := access.cell.PresentAt(0)
	if !ok || comparePresent(selected, present) != 0 {
		return 0, false
	}
	return RawPayloadTag(selected.payloadID), selected.payloadID != 0
}

// InitialPayload projects the selected immutable boot payload without
// exposing RawAccess's private root/key fragment. It succeeds only for the
// one selected Present tuple and returns the exact Target initial source
// together with its owning boot root. Program Values payloads deliberately
// fail here and continue through PayloadTag/PayloadForRawTag.
func (access RawAccess) InitialPayload(present Present) (identity.ContentID, vocabulary.InitialValue, bool) {
	if !access.valid() || access.top || !present.valid() || present.owner != access.owner || access.cell.PresentCount() != 1 {
		return identity.ContentID{}, 0, false
	}
	selected, selectedOK := access.cell.PresentAt(0)
	if !selectedOK || comparePresent(selected, present) != 0 || access.key.Kind() != RootBoot {
		return identity.ContentID{}, 0, false
	}
	payload, payloadOK := present.Payload()
	initial, initialOK := payload.InitialValue()
	root, rootOK := access.key.BootID()
	if !payloadOK || !initialOK || !rootOK {
		return identity.ContentID{}, 0, false
	}
	return root, initial, true
}

// PayloadForRawTag resolves a staged payload projection to its exact sealed
// Payload. Numeric tags are schema-local: a caller must present them to the
// same Schema that issued the corresponding RawAccess.
func (schema Schema) PayloadForRawTag(tag RawPayloadTag) (Payload, bool) {
	if !schema.valid() || tag == 0 || uint64(tag) > uint64(len(schema.owner.payloads)) {
		return Payload{}, false
	}
	return schema.payload(uint32(tag))
}

// VisitRawPayloadTags walks the complete sealed RawPayloadTag universe in
// canonical ascending numeric order. It is a cold declaration-time bridge for
// Rules that must precompute tag-indexed descriptors; it exposes neither a
// Heap root/cell/object nor an alternate payload registry. Returning false
// from visit stops the walk successfully.
func (schema Schema) VisitRawPayloadTags(visit func(RawPayloadTag, Payload) bool) bool {
	if !schema.valid() || visit == nil {
		return false
	}
	for index := range schema.owner.payloads {
		tag := RawPayloadTag(index + 1)
		payload, ok := schema.PayloadForRawTag(tag)
		if !ok {
			return false
		}
		if !visit(tag, payload) {
			return true
		}
	}
	return true
}

// RawWriteBranches is the exact outcome split of one raw mutation.  Normal
// retains a one-world Heap successor; a frozen branch has no successor and is
// consumed by the caller's existing error outcome.  It is deliberately not a
// second Heap Fact.
type RawWriteBranches struct {
	owner  *schema
	normal Value
	has    bool
	frozen bool
}

func (branches RawWriteBranches) valid() bool {
	if branches.owner == nil || !branches.has && !branches.frozen {
		return false
	}
	return !branches.has || branches.normal.valid() && branches.normal.owner == branches.owner
}

func (branches RawWriteBranches) Normal() (Value, bool) {
	if !branches.valid() || !branches.has {
		return Value{}, false
	}
	return branches.normal, true
}

func (branches RawWriteBranches) FrozenError() bool { return branches.valid() && branches.frozen }

// VisitRawAccess projects a Heap Value only through complete world
// alternatives.  It never merges Objects, CellStates, or role-local worlds
// before publishing them.  Exact and finite selectors enumerate their exact
// atoms.  A kind selector enumerates present exceptions and a residual per
// kind; frozen boot atoms are split out and retained privately as residual
// exclusions, so a caller never manufactures an exclusion selector.
func (schema Schema) VisitRawAccess(key Key, fact Value, role materialization.Role, selector KeySelector, visit func(RawAccess) bool) bool {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || !schema.owns(fact) || !schema.Admits(key, fact) ||
		!role.Valid() || !schema.admitsReferenceRole(key.slot, role) || !selector.valid() || selector.owner != schema.owner || visit == nil {
		return false
	}
	emitCell := func(world World, object Object, fragment rawAccessFragment, initialFrozen bool, cell CellState) bool {
		raw, ok := cell.Raw()
		if !ok {
			return false
		}
		emit := func(selected CellState) bool {
			return visit(RawAccess{owner: schema.owner, key: key, role: role, world: world, object: object, cell: selected, fragment: fragment, initialFrozen: initialFrozen})
		}
		if raw.has(RawAbsent) {
			absent, absentOK := schema.CellAbsent()
			if !absentOK || !emit(absent) {
				return false
			}
		}
		// CellPresent is the overwhelmingly common exact route shape. When
		// there is exactly one present alternative and no simultaneous
		// absence, the authenticated CellState is already the required
		// singleton. Reusing it avoids allocating a one-element Present
		// slice for every warm route walk. Mixed/multiple cells retain the
		// explicit split below so RawAccess never publishes a union cell.
		if raw == RawPresent && len(cell.presents) == 1 {
			return emit(cell)
		}
		for index := 0; index < cell.PresentCount(); index++ {
			present, presentOK := cell.PresentAt(index)
			if !presentOK {
				return false
			}
			single, singleOK := canonicalCellState(schema.owner, RawPresent, []Present{present})
			if !singleOK || !emit(single) {
				return false
			}
		}
		return true
	}
	if fact.top {
		return visit(RawAccess{owner: schema.owner, key: key, role: role, top: true})
	}
	for _, world := range fact.worlds {
		object, selected := rawWorldObject(world, role)
		if !selected {
			continue
		}
		if selector.kinds == 0 {
			for _, atom := range selector.atoms {
				cell, found := object.partition.lookup(atom)
				// A singleton exact selector already carries the complete
				// authenticated atom slice. Reuse it on the hot route so a
				// warm exact RawAccess walk does not allocate a second selector
				// or fragment. Finite selectors still receive their own
				// singleton selector below, since publishing the caller's union
				// would make one RawAccess represent multiple keys.
				fragmentSelector := selector
				if len(selector.atoms) != 1 {
					fragmentSelector = KeySelector{owner: schema.owner, atoms: []keyAtom{atom}}
				}
				fragment := rawAccessFragment{selector: fragmentSelector}
				if !found || !fragment.valid(schema.owner) || !emitCell(world, object, fragment, schema.initialFrozenBootSlot(key, atom), cell) {
					return false
				}
			}
			continue
		}
		atoms := schema.selectedPolicyAtoms(key, object.partition, selector.kinds)
		for _, atom := range atoms {
			cell, found := object.partition.lookup(atom)
			fragment := rawAccessFragment{selector: KeySelector{owner: schema.owner, atoms: []keyAtom{atom}}}
			if !found || !fragment.valid(schema.owner) || !emitCell(world, object, fragment, schema.initialFrozenBootSlot(key, atom), cell) {
				return false
			}
		}
		for index := 0; index < legalKeyKindCount; index++ {
			kind, _ := legalKeyKindAt(index)
			if !selector.kinds.Contains(kind) {
				continue
			}
			kindSelector := KeySelector{owner: schema.owner, kinds: runtimekind.Bit(kind)}
			excluded := selectedAtomsForKind(schema.owner, atoms, kind)
			fragment := rawAccessFragment{selector: kindSelector, residual: true, exclusions: excluded}
			if !fragment.valid(schema.owner) || !emitCell(world, object, fragment, false, object.partition.rest[kind]) {
				return false
			}
		}
	}
	return true
}

// VisitRawAccessRoute resolves a previously issued route tag inside Heap and
// then applies the ordinary complete-world raw projection. Callers therefore
// cannot decode or reconstruct a Key or materialization.Role outside Heap.
func (schema Schema) VisitRawAccessRoute(route RawRouteTag, fact Value, selector KeySelector, visit func(RawAccess) bool) bool {
	key, role, ok := schema.rawRoute(route)
	if !ok {
		return false
	}
	return schema.VisitRawAccess(key, fact, role, selector, visit)
}

// rawRouteTag packs Schema-local root and the closed materialization role.
// Key slots are uint32 and Role is uint8, so this is injective without a map,
// hash, string, allocation, or cardinality policy.
func rawRouteTag(key Key, role materialization.Role) (RawRouteTag, bool) {
	if !key.valid() || !role.Valid() {
		return 0, false
	}
	tag := RawRouteTag(uint64(key.slot)<<8 | uint64(role))
	return tag, tag != 0
}

// RouteForTag admits one owner-issued route tag back as the coordinate and
// role it was issued for. It is the inverse of RouteTag, and it exists because
// a routed fold is handed the tag its cells were paired by rather than the
// coordinate: without an inverse the tag is a presence bit, and a judgment
// that must read or publish at the route would have no coordinate to name.
//
// It fences the tag to this Schema and re-encodes it canonically, so a tag
// this Schema did not issue, or one whose numeric form is not the one it would
// have written, is refused rather than decoded into a neighbouring slot.
func (schema Schema) RouteForTag(route RawRouteTag) (Key, materialization.Role, bool) {
	return schema.rawRoute(route)
}

// rawRoute decodes a compact route tag only after fencing it to this Schema.
// The canonical re-encoding check rejects malformed numeric representations.
func (schema Schema) rawRoute(route RawRouteTag) (Key, materialization.Role, bool) {
	if !schema.valid() || route == 0 {
		return Key{}, materialization.Invalid, false
	}
	encoded := uint64(route)
	slot64 := encoded >> 8
	if slot64 == 0 || slot64 > uint64(^uint32(0)) {
		return Key{}, materialization.Invalid, false
	}
	key := Key{owner: schema.owner, slot: uint32(slot64)}
	role := materialization.Role(encoded & 0xff)
	if !key.valid() || !role.Valid() || !schema.admitsReferenceRole(key.slot, role) {
		return Key{}, materialization.Invalid, false
	}
	canonical, ok := rawRouteTag(key, role)
	if !ok || route != canonical {
		return Key{}, materialization.Invalid, false
	}
	return key, role, true
}

// RawStore applies a direct raw-present update only to RawAccess's selected
// world role and private key fragment.  Zero MutationLicence remains weak on
// that fragment; it cannot mutate a WorldMany companion role.
func (schema Schema) RawStore(access RawAccess, replacement CellState, licence MutationLicence) (RawWriteBranches, bool) {
	if !schema.valid() || !access.valid() || access.owner != schema.owner || !replacement.valid() || replacement.owner != schema.owner || !replacement.raw.has(RawPresent) {
		return RawWriteBranches{}, false
	}
	return schema.rawWrite(access, replacement, licence)
}

// RawDelete is RawStore's raw-absence counterpart.
func (schema Schema) RawDelete(access RawAccess, licence MutationLicence) (RawWriteBranches, bool) {
	replacement, ok := schema.CellAbsent()
	if !ok || !access.valid() || access.owner != schema.owner {
		return RawWriteBranches{}, false
	}
	return schema.rawWrite(access, replacement, licence)
}

func (schema Schema) rawWrite(access RawAccess, replacement CellState, licence MutationLicence) (RawWriteBranches, bool) {
	branches := RawWriteBranches{owner: schema.owner}
	if access.top {
		branches.normal, branches.has, branches.frozen = schema.Top(), true, true
		return branches, branches.valid()
	}
	if access.initialFrozen {
		branches.frozen = true
		return branches, branches.valid()
	}
	_, frozen, headerOK := access.object.Header()
	if !headerOK {
		return RawWriteBranches{}, false
	}
	branches.frozen = frozen&FrozenFrozen != 0
	if frozen&FrozenMutable == 0 {
		return branches, branches.valid()
	}
	object := access.object
	object.frozen = FrozenMutable
	strong := access.key.Kind() == RootAllocation && access.role == materialization.Recent && access.fragment.selector.exactSelection() &&
		licence.validForCell(schema.owner, access.key.slot, access.fragment.selector)
	var updated Object
	var updatedOK bool
	if strong {
		updated, updatedOK = overwriteObjectCell(object, access.fragment.selector, replacement)
	} else {
		updated, updatedOK = weakObjectFragment(object, access.fragment, replacement)
	}
	if !updatedOK {
		return RawWriteBranches{}, false
	}
	world, worldOK := rawWorldWithObject(access.world, access.role, updated)
	if !worldOK {
		return RawWriteBranches{}, false
	}
	normal, normalOK := schema.Relation(access.key, world)
	if !normalOK {
		return RawWriteBranches{}, false
	}
	branches.normal, branches.has = normal, true
	return branches, branches.valid()
}

func rawWorldObject(world World, role materialization.Role) (Object, bool) {
	if !world.valid() {
		return Object{}, false
	}
	switch role {
	case materialization.Exact:
		return world.Exact()
	case materialization.Recent:
		return world.Recent()
	case materialization.Summary:
		return world.Summary()
	default:
		return Object{}, false
	}
}

func rawWorldWithObject(world World, role materialization.Role, object Object) (World, bool) {
	if !world.valid() || !object.valid() || object.owner != world.owner {
		return World{}, false
	}
	next := world
	switch role {
	case materialization.Exact:
		if world.kind != WorldExact {
			return World{}, false
		}
		next.exact = object
	case materialization.Recent:
		if world.kind != WorldOne && world.kind != WorldMany {
			return World{}, false
		}
		next.recent = object
	case materialization.Summary:
		if world.kind != WorldMany {
			return World{}, false
		}
		next.summary = object
	default:
		return World{}, false
	}
	return next, next.valid()
}

func (schema Schema) initialFrozenBootSlot(key Key, atom keyAtom) bool {
	if !schema.valid() || key.Kind() != RootBoot || atom.kind != keyAtomExact {
		return false
	}
	slot := schema.owner.exactSlots[atom.exactOrdinal]
	if slot == 0 {
		return false
	}
	entry, found := schema.owner.bootEntries[rootSlot{root: key.slot, slot: slot}]
	return found && entry.mutability == vocabulary.InitialFrozen
}

func (schema Schema) selectedPolicyAtoms(key Key, partition Partition, kinds runtimekind.Set) []keyAtom {
	atoms := make([]keyAtom, 0, len(partition.exceptions))
	for _, exception := range partition.exceptions {
		if keyAtomRuntimeKinds(schema.owner, exception.atom)&kinds != 0 {
			atoms = append(atoms, exception.atom)
		}
	}
	if key.Kind() == RootBoot {
		for _, pair := range schema.owner.bootEntryOrder {
			if pair.root != key.slot {
				continue
			}
			entry := schema.owner.bootEntries[pair]
			if entry.mutability != vocabulary.InitialFrozen || int(pair.slot) > len(schema.owner.slots) {
				continue
			}
			slot := schema.owner.slots[pair.slot-1]
			if slot.kind != SlotExact {
				continue
			}
			atom := keyAtom{kind: keyAtomExact, exactOrdinal: slot.exact}
			if keyAtomRuntimeKinds(schema.owner, atom)&kinds != 0 {
				atoms = append(atoms, atom)
			}
		}
	}
	return normalizeKeyAtoms(atoms)
}

func selectedAtomsForKind(owner *schema, atoms []keyAtom, kind runtimekind.Kind) []keyAtom {
	selected := make([]keyAtom, 0, len(atoms))
	for _, atom := range atoms {
		if keyAtomRuntimeKinds(owner, atom).Contains(kind) {
			selected = append(selected, atom)
		}
	}
	return selected
}

func weakObjectFragment(object Object, fragment rawAccessFragment, replacement CellState) (Object, bool) {
	if !object.valid() || !fragment.valid(object.owner) || !replacement.valid() || replacement.owner != object.owner {
		return Object{}, false
	}
	if !fragment.residual {
		return weakObjectCell(object, fragment.selector, replacement)
	}
	partition := object.partition
	rest := partition.rest
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !fragment.selector.kinds.Contains(kind) {
			continue
		}
		next, ok := mergeCellStates(rest[kind], replacement)
		if !ok {
			return Object{}, false
		}
		rest[kind] = next
	}
	for _, exception := range partition.exceptions {
		if keyAtomRuntimeKinds(object.owner, exception.atom)&fragment.selector.kinds == 0 || containsKeyAtom(fragment.exclusions, exception.atom) {
			continue
		}
		// A residual fragment must exclude every explicit exception.  Each
		// exception is instead emitted as its own RawAccess alternative.
		return Object{}, false
	}
	exceptions := append([]partitionException(nil), partition.exceptions...)
	for _, atom := range fragment.exclusions {
		current, ok := partition.lookup(atom)
		if !ok || !setPartitionException(&exceptions, atom, current) {
			return Object{}, false
		}
	}
	next, ok := partitionWith(object.owner, rest, exceptions)
	if !ok {
		return Object{}, false
	}
	object.partition = next
	return object, object.valid()
}

func containsKeyAtom(atoms []keyAtom, wanted keyAtom) bool {
	index := sort.Search(len(atoms), func(index int) bool { return compareKeyAtom(atoms[index], wanted) >= 0 })
	return index < len(atoms) && compareKeyAtom(atoms[index], wanted) == 0
}

func setPartitionException(exceptions *[]partitionException, atom keyAtom, state CellState) bool {
	if exceptions == nil || !state.valid() {
		return false
	}
	items := *exceptions
	index := sort.Search(len(items), func(index int) bool { return compareKeyAtom(items[index].atom, atom) >= 0 })
	if index < len(items) && compareKeyAtom(items[index].atom, atom) == 0 {
		items[index].state = state
		*exceptions = items
		return true
	}
	items = append(items, partitionException{})
	copy(items[index+1:], items[index:])
	items[index] = partitionException{atom: atom, state: state}
	*exceptions = items
	return true
}
