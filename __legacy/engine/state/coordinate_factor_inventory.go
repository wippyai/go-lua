package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// ProjectCoordinateFactorInventoryFormalBoundary removes coordinate scalars
// whose registered Values dependencies name an unwired formal Input or
// Output of the source body. Each wire is an already-sealed root rekey; a
// dependency is bound when any wire maps its exact root. Middle dependencies
// remain invocation-local and may enter the boundary existential namespace.
// No family name or coordinate representation is inspected here.
func (d ProductDomain) ProjectCoordinateFactorInventoryFormalBoundary(
	inventory CoordinateFactorInventory,
	wires ...CoordinateFormalRootRekey,
) (CoordinateFactorInventory, error) {
	if !inventory.ValidFor(d, inventory.KeySpace()) || len(wires) == 0 {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: formal boundary coordinate selector is unowned", ErrInvalidLaneFactor)
	}
	for _, wire := range wires {
		if !wire.validFor(d) || !wire.formalSource || wire.from != inventory.KeySpace() {
			return CoordinateFactorInventory{}, fmt.Errorf("%w: formal boundary coordinate wire is foreign", ErrInvalidLaneFactor)
		}
	}
	selected := make([]CoordinateSlot, 0, inventory.Len())
	for _, slot := range inventory.Slots() {
		keep := true
		var visitErr error
		if err := d.VisitCoordinateValueDependencies(slot, func(dependency statekey.ValueDependency) {
			if visitErr != nil || !dependency.Valid() {
				visitErr = fmt.Errorf("%w: formal boundary coordinate dependency is invalid", ErrInvalidLaneFactor)
				return
			}
			root, formalDependency := dependency.Formal()
			if !formalDependency {
				visitErr = fmt.Errorf("%w: formal boundary coordinate dependency is concrete", ErrInvalidLaneFactor)
				return
			}
			if root.Owner() != wires[0].sourceOwner {
				visitErr = fmt.Errorf("%w: formal boundary coordinate dependency has foreign owner", ErrInvalidLaneFactor)
				return
			}
			if root.Vocabulary() == formal.Middle {
				return
			}
			if root.Vocabulary() != formal.Input && root.Vocabulary() != formal.Output {
				visitErr = fmt.Errorf("%w: formal boundary coordinate dependency has invalid vocabulary", ErrInvalidLaneFactor)
				return
			}
			path, exact := inventory.KeySpace().InternFormalRoot(root)
			if !exact {
				visitErr = fmt.Errorf("%w: formal boundary coordinate dependency is outside source keyspace", ErrInvalidLaneFactor)
				return
			}
			bound := false
			for _, wire := range wires {
				if _, mapped := wire.rekey(path); mapped {
					bound = true
					break
				}
			}
			if !bound {
				keep = false
			}
		}); err != nil {
			return CoordinateFactorInventory{}, err
		}
		if visitErr != nil {
			return CoordinateFactorInventory{}, visitErr
		}
		if keep {
			selected = append(selected, slot)
		}
	}
	return d.SealCoordinateFactorInventory(inventory.KeySpace(), selected)
}

// CoordinateFactorInventory is the sealed scalar-coordinate universe of one
// ProductDomain and keyspace. Slots are stored in registered family/key order;
// consumers can select a family without inspecting lane or key representations.
type CoordinateFactorInventory struct {
	seal *productDomainSeal
	keys *keyspace.KeySpace
	set  *coordinateFactorInventorySet
	// completed is the exact subset of set whose registered unary completion
	// consequences have already been expanded.  Equality with set is the
	// immutable closure certificate; nil denotes the empty completed subset.
	completed *coordinateFactorInventorySet
}

// coordinateFactorInventorySet is an immutable canonical coordinate set.
// Its pointer is the retained set identity used by fixed-point clients.
// Buckets and slots are never exposed mutably.
type coordinateFactorInventorySet struct {
	families      []coordinateFactorFamilyInventory
	count         int
	hasCompletion bool
}

type coordinateFactorFamilyInventory struct {
	family CoordinateFamily
	slots  []CoordinateSlot
}

// CoordinateFactorInventoryFromPreparedState freezes the explicit scalar
// coordinates of one statically prepared producer State. It is an admission
// edge for immutable producer inputs such as InitialStatePlan seeds, not a
// runtime topology-discovery operation. Every enabled lane and registered
// family is traversed in ProductDomain order without semantic-name dispatch.
func (d ProductDomain) CoordinateFactorInventoryFromPreparedState(keys *keyspace.KeySpace, prepared State) (CoordinateFactorInventory, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: prepared coordinate factor inventory authority", ErrInvalidLaneFactor)
	}
	lanes := d.LaneInventory()
	factors, err := d.Decompose(prepared)
	if err != nil || len(factors) != len(lanes) {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: prepared coordinate factor inventory decomposition", ErrInvalidLaneFactor)
	}
	slots := make([]CoordinateSlot, 0)
	for laneIndex, lane := range lanes {
		families, familyErr := d.CoordinateFamilies(lane)
		if familyErr != nil {
			return CoordinateFactorInventory{}, familyErr
		}
		for _, family := range families {
			_, scalars, decomposeErr := d.DecomposeCoordinateFamily(factors[laneIndex], family, keys)
			if decomposeErr != nil {
				return CoordinateFactorInventory{}, decomposeErr
			}
			for _, scalar := range scalars {
				slots = append(slots, scalar.Slot())
			}
		}
	}
	return d.SealCoordinateFactorInventory(keys, slots)
}

// SealCoordinateFactorInventory validates, sorts and deduplicates an opaque
// coordinate slot set. Validation is paid exactly once at this admission
// edge. The returned authority is immutable and all public views detach, so
// downstream ownership checks need only compare its seals.
func (d ProductDomain) SealCoordinateFactorInventory(keys *keyspace.KeySpace, slots []CoordinateSlot) (CoordinateFactorInventory, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate factor inventory authority", ErrInvalidLaneFactor)
	}
	type familyBuilder struct {
		coordinate *coordinateFamilyRuntime
		slots      []CoordinateSlot
	}
	builders := make(map[CoordinateFamily]*familyBuilder)
	families := make([]CoordinateFamily, 0)
	for index, slot := range slots {
		builder := builders[slot.family]
		if builder == nil {
			coordinate, err := d.validateCoordinateFamily(slot.family)
			if err != nil {
				return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate factor inventory slot %d", ErrInvalidLaneFactor, index)
			}
			builder = &familyBuilder{coordinate: coordinate}
			builders[slot.family] = builder
			families = append(families, slot.family)
		}
		if d.validateCoordinateSlotFor(builder.coordinate, slot, keys) != nil {
			return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate factor inventory slot %d", ErrInvalidLaneFactor, index)
		}
		builder.slots = append(builder.slots, slot)
	}
	sort.Slice(families, func(i, j int) bool {
		return coordinateFamilyLess(families[i], families[j])
	})
	set := &coordinateFactorInventorySet{}
	for _, family := range families {
		builder := builders[family]
		canonical := builder.slots
		sort.SliceStable(canonical, func(i, j int) bool {
			return builder.coordinate.ops.keyLess(canonical[i].key, canonical[j].key, keys)
		})
		unique := canonical[:0]
		for _, slot := range canonical {
			if len(unique) != 0 && builder.coordinate.ops.keyEqual(unique[len(unique)-1].key, slot.key) {
				continue
			}
			unique = append(unique, slot)
		}
		if len(unique) != 0 {
			set.families = append(set.families, coordinateFactorFamilyInventory{family: family, slots: unique})
			set.count += len(unique)
			set.hasCompletion = set.hasCompletion ||
				builder.coordinate.ops.inventoryCompletion.kind == coordinateInventoryCompletionConsequences ||
				builder.coordinate.ops.returnIdentity.roles != 0
		}
	}
	out := CoordinateFactorInventory{seal: d.seal, keys: keys, set: set}
	if !set.hasCompletion {
		out.completed = set
	}
	return out, nil
}

func coordinateFamilyLess(left, right CoordinateFamily) bool {
	if left.lane.ordinal != right.lane.ordinal {
		return left.lane.ordinal < right.lane.ordinal
	}
	return left.ordinal < right.ordinal
}

// ValidFor reports exact ProductDomain and keyspace ownership. Canonical
// ordering and slot validity were certified at the sole seal boundary; the
// inventory has no mutating API and all exported slice views are detached.
// Revalidating every retained coordinate here would discard that authority and
// turn every fixed-point observation into an O(width) scan.
func (i CoordinateFactorInventory) ValidFor(d ProductDomain, keys *keyspace.KeySpace) bool {
	return d.Valid() && i.seal != nil && i.seal == d.seal && i.keys != nil && i.keys == keys && i.keys.Valid() &&
		i.set != nil && i.set.count >= 0
}

func (i CoordinateFactorInventory) KeySpace() *keyspace.KeySpace { return i.keys }
func (i CoordinateFactorInventory) Len() int {
	if i.set == nil {
		return 0
	}
	return i.set.count
}

// CoordinateFactorInventoryCursor is a borrowed read-only traversal over one
// immutable canonical set. It allocates no detached slot slice.
type CoordinateFactorInventoryCursor struct {
	set                 *coordinateFactorInventorySet
	familyIndex, offset int
}

// BorrowSlots returns a zero-allocation cursor over canonical family/key order.
func (i CoordinateFactorInventory) BorrowSlots() CoordinateFactorInventoryCursor {
	return CoordinateFactorInventoryCursor{set: i.set}
}

// Next advances one slot. The returned slot is an immutable opaque value; no
// backing inventory storage is exposed.
func (c *CoordinateFactorInventoryCursor) Next() (CoordinateSlot, bool) {
	for c != nil && c.set != nil && c.familyIndex < len(c.set.families) {
		bucket := c.set.families[c.familyIndex]
		if c.offset < len(bucket.slots) {
			slot := bucket.slots[c.offset]
			c.offset++
			return slot, true
		}
		c.familyIndex++
		c.offset = 0
	}
	return CoordinateSlot{}, false
}

// Slots returns the complete detached canonical inventory.
func (i CoordinateFactorInventory) Slots() []CoordinateSlot {
	out := make([]CoordinateSlot, 0, i.Len())
	cursor := i.BorrowSlots()
	for slot, ok := cursor.Next(); ok; slot, ok = cursor.Next() {
		out = append(out, slot)
	}
	return out
}

// CoordinateFactorInventoryIdentityTerms returns the exact identity syntax
// support named by a sealed selector. Each family exposes its terms through
// the mandatory return-identity registration; the query never inspects a
// runtime factor or dispatches on a lane or family name. The detached result
// is structurally sorted and deduplicated across families.
func (d ProductDomain) CoordinateFactorInventoryIdentityTerms(inventory CoordinateFactorInventory) ([]identity.Term, error) {
	if !inventory.ValidFor(d, inventory.KeySpace()) {
		return nil, fmt.Errorf("%w: coordinate factor identity-term inventory", ErrInvalidLaneFactor)
	}
	seen := make(map[identity.Term]struct{})
	terms := make([]identity.Term, 0)
	for _, bucket := range inventory.set.families {
		coordinate, err := d.validateCoordinateFamily(bucket.family)
		if err != nil {
			return nil, fmt.Errorf("%w: coordinate factor identity-term family", ErrInvalidLaneFactor)
		}
		for _, slot := range bucket.slots {
			if !coordinate.ops.returnIdentity.visitInventoryTerms(slot.key, func(term identity.Term) bool {
				if !term.Valid() {
					return false
				}
				if _, present := seen[term]; !present {
					seen[term] = struct{}{}
					terms = append(terms, term)
				}
				return true
			}) {
				return nil, fmt.Errorf("%w: coordinate factor identity-term visitor", ErrInvalidLaneFactor)
			}
		}
	}
	sort.Slice(terms, func(left, right int) bool { return identityTermLess(terms[left], terms[right]) })
	return terms, nil
}

// SelectCoordinateFactorInventory returns the exact registered coordinate
// families owned by lanes. Selection is paid once while a transfer is frozen;
// execution receives the resulting dense inventory and never scans the body
// universe. Disabled or unselected lanes contribute no coordinates.
func (d ProductDomain) SelectCoordinateFactorInventory(
	keys *keyspace.KeySpace,
	inventory CoordinateFactorInventory,
	lanes LaneSet,
) (CoordinateFactorInventory, error) {
	if !inventory.ValidFor(d, keys) || !transferLanesRegistered(d.Lanes(), lanes) {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate factor inventory selection", ErrInvalidLaneFactor)
	}
	slots := make([]CoordinateSlot, 0, inventory.Len())
	for _, bucket := range inventory.set.families {
		if lanes.Has(bucket.family.lane.id) {
			slots = append(slots, bucket.slots...)
		}
	}
	return d.SealCoordinateFactorInventory(keys, slots)
}

// FamilySlots returns one registered family's detached canonical bucket.
func (i CoordinateFactorInventory) FamilySlots(family CoordinateFamily) ([]CoordinateSlot, error) {
	slots, err := i.familySlots(family)
	return append([]CoordinateSlot(nil), slots...), err
}

// familySlots is the package-private immutable view used by sealed operators.
// Public callers receive a detached copy through FamilySlots.
func (i CoordinateFactorInventory) familySlots(family CoordinateFamily) ([]CoordinateSlot, error) {
	if i.seal == nil || family.seal != i.seal || family.id == "" {
		return nil, fmt.Errorf("%w: coordinate inventory family", ErrInvalidProductLane)
	}
	for _, bucket := range i.set.families {
		if bucket.family == family {
			return bucket.slots, nil
		}
	}
	return nil, nil
}

// Contains reports exact semantic slot membership.
func (i CoordinateFactorInventory) Contains(d ProductDomain, slot CoordinateSlot) (bool, error) {
	if !i.ValidFor(d, i.keys) {
		return false, fmt.Errorf("%w: coordinate factor inventory", ErrInvalidLaneFactor)
	}
	slots, err := i.familySlots(slot.family)
	if err != nil {
		return false, err
	}
	for _, candidate := range slots {
		equal, equalErr := d.CoordinateSlotEqual(candidate, slot)
		if equalErr != nil {
			return false, equalErr
		}
		if equal {
			return true, nil
		}
	}
	return false, nil
}

// CoordinateFactorInventoriesEqual compares two sealed canonical sets. The
// retained immutable-set identity makes the fixed-point common case O(1);
// independently admitted equal sets use one allocation-free cursor pass.
func (d ProductDomain) CoordinateFactorInventoriesEqual(left, right CoordinateFactorInventory) (bool, error) {
	if !left.ValidFor(d, left.keys) || !right.ValidFor(d, right.keys) || left.keys != right.keys {
		return false, fmt.Errorf("%w: coordinate factor inventory equality", ErrInvalidLaneFactor)
	}
	if left.set == right.set {
		return true, nil
	}
	if left.Len() != right.Len() {
		return false, nil
	}
	leftCursor, rightCursor := left.BorrowSlots(), right.BorrowSlots()
	for {
		leftSlot, leftOK := leftCursor.Next()
		rightSlot, rightOK := rightCursor.Next()
		if leftOK != rightOK {
			return false, nil
		}
		if !leftOK {
			return true, nil
		}
		equal, err := d.CoordinateSlotEqual(leftSlot, rightSlot)
		if err != nil || !equal {
			return false, err
		}
	}
}

// UnionCoordinateFactorInventories forms the canonical set union without
// exposing or interpreting any family key representation.
func (d ProductDomain) UnionCoordinateFactorInventories(keys *keyspace.KeySpace, inventories ...CoordinateFactorInventory) (CoordinateFactorInventory, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate factor inventory union authority", ErrInvalidLaneFactor)
	}
	for index, inventory := range inventories {
		if !inventory.ValidFor(d, keys) {
			return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate factor inventory union input %d", ErrInvalidLaneFactor, index)
		}
	}
	switch len(inventories) {
	case 0:
		empty := &coordinateFactorInventorySet{}
		return CoordinateFactorInventory{seal: d.seal, keys: keys, set: empty, completed: empty}, nil
	case 1:
		return inventories[0], nil
	case 2:
		return d.unionCoordinateFactorInventories(keys, inventories[0], inventories[1]), nil
	}
	work := append([]CoordinateFactorInventory(nil), inventories...)
	for len(work) > 1 {
		next := make([]CoordinateFactorInventory, 0, (len(work)+1)/2)
		for index := 0; index < len(work); index += 2 {
			if index+1 == len(work) {
				next = append(next, work[index])
				continue
			}
			next = append(next, d.unionCoordinateFactorInventories(keys, work[index], work[index+1]))
		}
		work = next
	}
	return work[0], nil
}

// unionCoordinateFactorInventories linearly merges two already-canonical,
// immutable family bucket sequences. Unchanged buckets are safely shared;
// callers can only obtain detached views.
func (d ProductDomain) unionCoordinateFactorInventories(keys *keyspace.KeySpace, left, right CoordinateFactorInventory) CoordinateFactorInventory {
	if left.set.count == 0 {
		return right
	}
	if right.set.count == 0 {
		return left
	}
	set := d.unionCoordinateFactorInventorySets(keys, left.set, right.set)
	var completed *coordinateFactorInventorySet
	if left.completed == left.set && right.completed == right.set {
		completed = set
	} else {
		completed = d.unionCoordinateFactorInventorySets(keys, left.completed, right.completed)
	}
	if set == left.set && completed == left.completed {
		return left
	}
	if set == right.set && completed == right.completed {
		return right
	}
	return CoordinateFactorInventory{seal: d.seal, keys: keys, set: set, completed: completed}
}

// unionCoordinateFactorInventorySets returns an exact operand when it already
// contains the other set. The common unchanged fixed-point edge is therefore
// allocation-free and retains canonical set identity.
func (d ProductDomain) unionCoordinateFactorInventorySets(keys *keyspace.KeySpace, left, right *coordinateFactorInventorySet) *coordinateFactorInventorySet {
	if left == nil || left.count == 0 {
		return right
	}
	if right == nil || right.count == 0 {
		return left
	}
	if d.coordinateFactorInventorySetContains(keys, left, right) {
		return left
	}
	if d.coordinateFactorInventorySetContains(keys, right, left) {
		return right
	}
	set := &coordinateFactorInventorySet{
		families:      make([]coordinateFactorFamilyInventory, 0, len(left.families)+len(right.families)),
		hasCompletion: left.hasCompletion || right.hasCompletion,
	}
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left.families) || rightIndex < len(right.families); {
		switch {
		case leftIndex == len(left.families):
			bucket := right.families[rightIndex]
			set.families = append(set.families, bucket)
			set.count += len(bucket.slots)
			rightIndex++
		case rightIndex == len(right.families):
			bucket := left.families[leftIndex]
			set.families = append(set.families, bucket)
			set.count += len(bucket.slots)
			leftIndex++
		default:
			leftBucket, rightBucket := left.families[leftIndex], right.families[rightIndex]
			switch {
			case coordinateFamilyLess(leftBucket.family, rightBucket.family):
				set.families = append(set.families, leftBucket)
				set.count += len(leftBucket.slots)
				leftIndex++
			case coordinateFamilyLess(rightBucket.family, leftBucket.family):
				set.families = append(set.families, rightBucket)
				set.count += len(rightBucket.slots)
				rightIndex++
			default:
				coordinate, _ := d.validateCoordinateFamily(leftBucket.family)
				merged := mergeCoordinateFactorFamilySlots(coordinate, keys, leftBucket.slots, rightBucket.slots)
				set.families = append(set.families, coordinateFactorFamilyInventory{family: leftBucket.family, slots: merged})
				set.count += len(merged)
				leftIndex++
				rightIndex++
			}
		}
	}
	return set
}

func (d ProductDomain) coordinateFactorInventorySetContains(keys *keyspace.KeySpace, whole, subset *coordinateFactorInventorySet) bool {
	if subset == nil || subset.count == 0 {
		return true
	}
	if whole == nil || whole.count < subset.count {
		return false
	}
	wholeIndex := 0
	for _, want := range subset.families {
		for wholeIndex < len(whole.families) && coordinateFamilyLess(whole.families[wholeIndex].family, want.family) {
			wholeIndex++
		}
		if wholeIndex >= len(whole.families) || whole.families[wholeIndex].family != want.family {
			return false
		}
		coordinate, _ := d.validateCoordinateFamily(want.family)
		have := whole.families[wholeIndex].slots
		for haveIndex, wantIndex := 0, 0; wantIndex < len(want.slots); {
			for haveIndex < len(have) && coordinate.ops.keyLess(have[haveIndex].key, want.slots[wantIndex].key, keys) {
				haveIndex++
			}
			if haveIndex >= len(have) || !coordinate.ops.keyEqual(have[haveIndex].key, want.slots[wantIndex].key) {
				return false
			}
			wantIndex++
		}
	}
	return true
}

func mergeCoordinateFactorFamilySlots(coordinate *coordinateFamilyRuntime, keys *keyspace.KeySpace, left, right []CoordinateSlot) []CoordinateSlot {
	out := make([]CoordinateSlot, 0, len(left)+len(right))
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left) || rightIndex < len(right); {
		switch {
		case leftIndex == len(left):
			out = append(out, right[rightIndex:]...)
			return out
		case rightIndex == len(right):
			out = append(out, left[leftIndex:]...)
			return out
		case coordinate.ops.keyEqual(left[leftIndex].key, right[rightIndex].key):
			out = append(out, left[leftIndex])
			leftIndex++
			rightIndex++
		case coordinate.ops.keyLess(left[leftIndex].key, right[rightIndex].key, keys):
			out = append(out, left[leftIndex])
			leftIndex++
		default:
			out = append(out, right[rightIndex])
			rightIndex++
		}
	}
	return out
}

// CloseCoordinateFactorInventory computes the least fixed point of the
// registered per-key completion relations over a producer-sealed inventory.
// Every admitted key is evaluated once; consequences are deduplicated by the
// owning family's structural key law before entering the uncapped worklist.
// It never discovers coordinates by scanning a runtime State.
func (d ProductDomain) CloseCoordinateFactorInventory(keys *keyspace.KeySpace, seed CoordinateFactorInventory) (CoordinateFactorInventory, error) {
	return d.CloseCoordinateFactorInventoryWithIdentityTerms(keys, seed, nil)
}

// CloseCoordinateFactorInventoryWithIdentityTerms computes the least fixed
// point over coordinate slots and the finite exact identity terms declared by
// the producer syntax. Terms are not discovered from runtime State: operator
// freezing supplies them from roots, constants, and allocation templates.
func (d ProductDomain) CloseCoordinateFactorInventoryWithIdentityTerms(keys *keyspace.KeySpace, seed CoordinateFactorInventory, terms []identity.Term) (CoordinateFactorInventory, error) {
	if !seed.ValidFor(d, keys) {
		return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate factor inventory closure seed", ErrInvalidLaneFactor)
	}
	if (!seed.set.hasCompletion || seed.completed == seed.set) && len(terms) == 0 {
		return seed, nil
	}
	return d.closeCoordinateFactorInventory(keys, seed, terms)
}

func (d ProductDomain) closeCoordinateFactorInventory(keys *keyspace.KeySpace, seed CoordinateFactorInventory, declaredTerms []identity.Term) (CoordinateFactorInventory, error) {
	type completionFamily struct {
		coordinate *coordinateFamilyRuntime
		seen       map[uint64][]coordinateKeyPayload
	}
	families := make(map[CoordinateFamily]*completionFamily)
	ordered := make([]*completionFamily, 0)
	for laneIndex := range d.factorLanes {
		for coordinateIndex := range d.factorLanes[laneIndex].coordinates {
			coordinate := &d.factorLanes[laneIndex].coordinates[coordinateIndex]
			family := &completionFamily{coordinate: coordinate, seen: make(map[uint64][]coordinateKeyPayload)}
			families[coordinate.family] = family
			ordered = append(ordered, family)
		}
	}
	worklist := d.coordinateFactorInventorySetDifferenceSlots(keys, seed.set, seed.completed)
	added := make([]CoordinateSlot, 0)
	termWorklist := make([]identity.Term, 0, len(declaredTerms))
	seenTerms := make(map[identity.Term]struct{}, len(declaredTerms))
	for _, bucket := range seed.set.families {
		family := families[bucket.family]
		if family == nil {
			return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate family inventory completion", ErrInvalidLaneFactor)
		}
		for _, slot := range bucket.slots {
			hash := family.coordinate.ops.keyHash(slot.key, keys)
			family.seen[hash] = append(family.seen[hash], slot.key)
		}
	}

	changed := false
	emitSlot := func(family *completionFamily, key coordinateKeyPayload) bool {
		if family == nil || key == nil {
			return false
		}
		slot := CoordinateSlot{family: family.coordinate.family, keys: keys, key: key}
		if d.validateCoordinateSlotFor(family.coordinate, slot, keys) != nil {
			return false
		}
		hash := family.coordinate.ops.keyHash(key, keys)
		for _, admitted := range family.seen[hash] {
			if family.coordinate.ops.keyEqual(admitted, key) {
				return true
			}
		}
		family.seen[hash] = append(family.seen[hash], key)
		worklist = append(worklist, slot)
		added = append(added, slot)
		changed = true
		return true
	}
	emitTerm := func(term identity.Term) bool {
		if !term.Valid() {
			return false
		}
		if _, exists := seenTerms[term]; exists {
			return true
		}
		seenTerms[term] = struct{}{}
		termWorklist = append(termWorklist, term)
		return true
	}
	for _, term := range declaredTerms {
		if !emitTerm(term) {
			return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate return-identity declared term", ErrInvalidLaneFactor)
		}
	}
	slotCursor, termCursor := 0, 0
	for slotCursor < len(worklist) || termCursor < len(termWorklist) {
		if slotCursor < len(worklist) {
			source := worklist[slotCursor]
			slotCursor++
			family := families[source.family]
			if family == nil {
				return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate family inventory completion", ErrInvalidLaneFactor)
			}
			emissionValid := true
			if !family.coordinate.ops.inventoryCompletion.emit(keys, source.key, func(key coordinateKeyPayload) bool {
				if !emitSlot(family, key) {
					emissionValid = false
					return false
				}
				return true
			}) || !emissionValid {
				return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate family inventory completion", ErrInvalidLaneFactor)
			}
			if !family.coordinate.ops.returnIdentity.visitInventoryTerms(source.key, emitTerm) {
				return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate return-identity inventory source", ErrInvalidLaneFactor)
			}
		}
		if termCursor < len(termWorklist) {
			term := termWorklist[termCursor]
			termCursor++
			for _, target := range ordered {
				if !target.coordinate.ops.returnIdentity.visitTermKeys(term, func(key coordinateKeyPayload) bool {
					return emitSlot(target, key)
				}) {
					return CoordinateFactorInventory{}, fmt.Errorf("%w: coordinate return-identity inventory target", ErrInvalidLaneFactor)
				}
			}
		}
	}
	if !changed {
		seed.completed = seed.set
		return seed, nil
	}
	addition, err := d.SealCoordinateFactorInventory(keys, added)
	if err != nil {
		return CoordinateFactorInventory{}, err
	}
	out := d.unionCoordinateFactorInventories(keys, seed, addition)
	out.completed = out.set
	return out, nil
}

// coordinateFactorInventorySetDifferenceSlots materializes only the pending
// completion frontier. completed is always an immutable subset of whole.
func (d ProductDomain) coordinateFactorInventorySetDifferenceSlots(keys *keyspace.KeySpace, whole, completed *coordinateFactorInventorySet) []CoordinateSlot {
	if whole == nil || whole == completed {
		return nil
	}
	out := make([]CoordinateSlot, 0, whole.count)
	completedFamily := 0
	for _, bucket := range whole.families {
		for completed != nil && completedFamily < len(completed.families) &&
			coordinateFamilyLess(completed.families[completedFamily].family, bucket.family) {
			completedFamily++
		}
		var prior []CoordinateSlot
		if completed != nil && completedFamily < len(completed.families) && completed.families[completedFamily].family == bucket.family {
			prior = completed.families[completedFamily].slots
		}
		coordinate, _ := d.validateCoordinateFamily(bucket.family)
		priorIndex := 0
		for _, slot := range bucket.slots {
			for priorIndex < len(prior) && coordinate.ops.keyLess(prior[priorIndex].key, slot.key, keys) {
				priorIndex++
			}
			if priorIndex >= len(prior) || !coordinate.ops.keyEqual(prior[priorIndex].key, slot.key) {
				out = append(out, slot)
			}
		}
	}
	return out
}
