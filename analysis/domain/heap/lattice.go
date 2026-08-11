package heap

import (
	"sort"

	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// Domain exposes Heap's homogeneous complete-world algebra. Coordinate
// admission remains Schema's responsibility: this algebra never learns a
// root, slot, or source occurrence. Meet is intentionally absent. The
// complete-world carrier has a forward may semantics, but it has not declared
// a greatest-lower-bound representation for partitioned objects; publishing a
// plausible intersection would be a second, unsound semantic authority.
func (schema Schema) Domain() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom:   schema.Bottom,
		Top:      schema.Top,
		Equal:    Equal,
		Same:     Same,
		LessOrEq: LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := Join(left, right)
			if !ok {
				return Value{}
			}
			return value
		},
		Widen: func(previous, next Value) Value {
			value, ok := Widen(previous, next)
			if !ok {
				return Value{}
			}
			return value
		},
	}
}

// Same is an O(1) immutable-representation prefilter. A positive answer is
// always Equal; union and widening retain an existing operand whenever they
// can, so this avoids an otherwise hot structural walk.
func Same(left, right Value) bool {
	return left.owner != nil && right.owner != nil && left.owner == right.owner && left.top == right.top && len(left.worlds) == len(right.worlds) &&
		(len(left.worlds) == 0 || &left.worlds[0] == &right.worlds[0])
}

func Equal(left, right Value) bool {
	if Same(left, right) {
		return true
	}
	if !left.valid() || !right.valid() || left.owner != right.owner || left.top != right.top || len(left.worlds) != len(right.worlds) {
		return false
	}
	return equalValueAdmitted(left, right)
}

// equalValueAdmitted compares already admitted Values.  Value's nested
// slices are immutable outside this package and every successful constructor
// validates the complete representation once; callers must therefore pass
// through the public boundary above before using this helper.
func equalValueAdmitted(left, right Value) bool {
	if left.owner != right.owner || left.top != right.top || len(left.worlds) != len(right.worlds) {
		return false
	}
	for index := range left.worlds {
		if compareWorld(left.worlds[index], right.worlds[index]) != 0 {
			return false
		}
	}
	return true
}

// LessOrEq is inclusion of complete control worlds. It never compares
// marginal raw/slot/role fragments: a world is included only by a world of
// the same cardinality family whose complete object state includes it.
func LessOrEq(left, right Value) bool {
	if Same(left, right) {
		return true
	}
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return false
	}
	return valueLessOrEqAdmitted(left, right)
}

func valueLessOrEqAdmitted(left, right Value) bool {
	if right.top || !left.top && len(left.worlds) == 0 {
		return true
	}
	if left.top || !right.top && len(right.worlds) == 0 {
		return false
	}
	for _, wanted := range left.worlds {
		found := false
		for _, candidate := range right.worlds {
			if worldLessOrEqAdmitted(wanted, candidate) {
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

func worldLessOrEq(left, right World) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner || left.kind != right.kind {
		return false
	}
	return worldLessOrEqAdmitted(left, right)
}

func worldLessOrEqAdmitted(left, right World) bool {
	if left.owner != right.owner || left.kind != right.kind {
		return false
	}
	switch left.kind {
	case WorldZero:
		return true
	case WorldExact:
		return objectLessOrEqAdmitted(left.exact, right.exact)
	case WorldOne:
		return objectLessOrEqAdmitted(left.recent, right.recent)
	case WorldMany:
		return objectLessOrEqAdmitted(left.recent, right.recent) && objectLessOrEqAdmitted(left.summary, right.summary)
	default:
		return false
	}
}

func objectLessOrEq(left, right Object) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return false
	}
	return objectLessOrEqAdmitted(left, right)
}

func objectLessOrEqAdmitted(left, right Object) bool {
	return left.owner == right.owner && left.shape&^right.shape == 0 && left.frozen&^right.frozen == 0 && (!left.noMeta || right.noMeta) && (!left.unknownMeta || right.unknownMeta) &&
		referencesSubset(left.metatables, right.metatables) && partitionLessOrEqAdmitted(left.partition, right.partition)
}

func referencesSubset(left, right []Reference) bool {
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left); {
		if rightIndex == len(right) {
			return false
		}
		compared := compareReference(left[leftIndex], right[rightIndex])
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

// Join is the canonical union of complete worlds. Different worlds stay
// separate: collapsing them would lose branch correlations before Mu. A
// dominated complete world is removed only when another complete world proves
// that it includes every one of its fields.
func Join(left, right Value) (Value, bool) {
	if Same(left, right) {
		return left, true
	}
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return Value{}, false
	}
	if left.top || right.top {
		return left.owner.top, true
	}
	if len(left.worlds) == 0 {
		return right, true
	}
	if len(right.worlds) == 0 {
		return left, true
	}
	if valueLessOrEqAdmitted(left, right) {
		return right, true
	}
	if valueLessOrEqAdmitted(right, left) {
		return left, true
	}
	worlds := make([]World, 0, len(left.worlds)+len(right.worlds))
	for _, world := range left.worlds {
		worlds = appendCompleteWorld(worlds, world)
	}
	for _, world := range right.worlds {
		worlds = appendCompleteWorld(worlds, world)
	}
	sort.Slice(worlds, func(leftIndex, rightIndex int) bool {
		return compareWorld(worlds[leftIndex], worlds[rightIndex]) < 0
	})
	value := Value{owner: left.owner, worlds: worlds}
	if !value.valid() {
		return Value{}, false
	}
	return value, true
}

func appendCompleteWorld(worlds []World, candidate World) []World {
	for index := 0; index < len(worlds); index++ {
		current := worlds[index]
		if worldLessOrEqAdmitted(candidate, current) {
			return worlds
		}
		if worldLessOrEqAdmitted(current, candidate) {
			worlds[index] = candidate
			// The new world can subsume more than the element it replaced.
			for other := 0; other < len(worlds); {
				if other != index && worldLessOrEqAdmitted(worlds[other], candidate) {
					worlds = append(worlds[:other], worlds[other+1:]...)
					if other < index {
						index--
					}
					continue
				}
				other++
			}
			return worlds
		}
	}
	return append(worlds, candidate)
}

// Widen first covers exact Join, then applies the fixed complete-world
// generalization: alternatives of the same control family merge their whole
// Object state. mergeObjects transports raw, present tuples, containment,
// metatable, and the full fixed-coordinate Partition together; it is never a
// union of marginal observations. WidenRank below proves this single fixed
// generalization descends without a threshold or a Top-on-count path.
func Widen(previous, next Value) (Value, bool) {
	joined, ok := Join(previous, next)
	if !ok || equalValueAdmitted(previous, joined) {
		return joined, ok
	}
	if joined.top || len(joined.worlds) == 0 {
		return joined, true
	}
	worlds, ok := generalizeWorldFamiliesAdmitted(joined.worlds)
	if !ok {
		return Value{}, false
	}
	value := Value{owner: joined.owner, worlds: worlds}
	if !value.valid() || !valueLessOrEqAdmitted(previous, value) || !valueLessOrEqAdmitted(next, value) {
		return Value{}, false
	}
	return value, true
}

func generalizeWorldFamilies(source []World) ([]World, bool) {
	for _, candidate := range source {
		if !candidate.valid() {
			return nil, false
		}
	}
	return generalizeWorldFamiliesAdmitted(source)
}

func generalizeWorldFamiliesAdmitted(source []World) ([]World, bool) {
	if len(source) == 0 {
		return nil, true
	}
	var found [WorldMany + 1]bool
	var worlds [WorldMany + 1]World
	for _, candidate := range source {
		index := int(candidate.kind)
		if index < int(WorldZero) || index > int(WorldMany) || candidate.owner == nil {
			return nil, false
		}
		if !found[index] {
			worlds[index] = candidate
			found[index] = true
			continue
		}
		merged, ok := mergeWorldAdmitted(worlds[index], candidate)
		if !ok {
			return nil, false
		}
		worlds[index] = merged
	}
	result := make([]World, 0, len(source))
	for kind := WorldZero; kind <= WorldMany; kind++ {
		if found[kind] {
			result = append(result, worlds[kind])
		}
	}
	return result, true
}

func mergeWorld(left, right World) (World, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner || left.kind != right.kind {
		return World{}, false
	}
	return mergeWorldAdmitted(left, right)
}

func mergeWorldAdmitted(left, right World) (World, bool) {
	if left.owner == nil || left.owner != right.owner || left.kind != right.kind {
		return World{}, false
	}
	merged := World{owner: left.owner, kind: left.kind}
	var ok bool
	switch left.kind {
	case WorldZero:
		return left, true
	case WorldExact:
		merged.exact, ok = mergeObjectsAdmitted(left.exact, right.exact)
	case WorldOne:
		merged.recent, ok = mergeObjectsAdmitted(left.recent, right.recent)
	case WorldMany:
		merged.recent, ok = mergeObjectsAdmitted(left.recent, right.recent)
		if ok {
			merged.summary, ok = mergeObjectsAdmitted(left.summary, right.summary)
		}
	default:
		return World{}, false
	}
	if !ok || merged.owner == nil {
		return World{}, false
	}
	return merged, true
}

// Fingerprint returns the deterministic hot identity of one complete Heap
// relation. It includes every semantic field; the schema identity fences the
// family, while equality remains the collision authority.
func (schema Schema) Fingerprint(value Value) (uint64, bool) {
	if !schema.owns(value) {
		return 0, false
	}
	hash := uint64(0x4845_4150)
	for _, word := range schema.owner.id {
		hash = internal.MixHash(hash, uint64(word))
	}
	if value.top {
		return internal.MixHash(hash, 1), true
	}
	hash = internal.MixHash(hash, 0)
	for _, world := range value.worlds {
		hash = hashWorld(hash, world)
	}
	return hash, true
}

func hashWorld(hash uint64, world World) uint64 {
	hash = internal.MixHash(hash, uint64(world.kind))
	if world.exact.owner != nil {
		hash = hashObject(hash, world.exact)
	}
	if world.recent.owner != nil {
		hash = hashObject(hash, world.recent)
	}
	if world.summary.owner != nil {
		hash = hashObject(hash, world.summary)
	}
	return hash
}

func hashObject(hash uint64, object Object) uint64 {
	hash = internal.MixHash(hash, uint64(object.shape))
	hash = internal.MixHash(hash, uint64(object.frozen))
	if object.noMeta {
		hash = internal.MixHash(hash, 1)
	} else {
		hash = internal.MixHash(hash, 0)
	}
	if object.unknownMeta {
		hash = internal.MixHash(hash, 1)
	} else {
		hash = internal.MixHash(hash, 0)
	}
	for _, reference := range object.metatables {
		hash = hashReference(hash, reference)
	}
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		hash = internal.MixHash(hash, uint64(kind))
		hash = hashCellState(hash, object.partition.rest[kind])
	}
	for _, exception := range object.partition.exceptions {
		hash = hashKeyAtom(hash, exception.atom)
		hash = hashCellState(hash, exception.state)
	}
	return hash
}

func hashReference(hash uint64, reference Reference) uint64 {
	hash = internal.MixHash(hash, uint64(reference.root))
	return internal.MixHash(hash, uint64(reference.role))
}

func hashCellState(hash uint64, state CellState) uint64 {
	hash = internal.MixHash(hash, uint64(state.raw))
	for _, present := range state.presents {
		hash = internal.MixHash(hash, uint64(present.slotID))
		hash = internal.MixHash(hash, uint64(present.payloadID))
		hash = hashContainment(hash, present.valueContainment)
		hash = hashContainment(hash, present.keyContainment)
	}
	return hash
}

func hashContainment(hash uint64, containment Containment) uint64 {
	hash = internal.MixHash(hash, uint64(containment.kind))
	hash = internal.MixHash(hash, uint64(containment.root))
	return internal.MixHash(hash, uint64(containment.role))
}

func hashKeyAtom(hash uint64, atom keyAtom) uint64 {
	hash = internal.MixHash(hash, uint64(atom.kind))
	hash = internal.MixHash(hash, uint64(atom.root))
	return internal.MixHash(hash, uint64(atom.role))
}

// WidenRank is Heap's finite, key-aware termination witness. It has three
// lexicographic components:
//
//   - control phase/family capacity;
//   - surplus same-family complete worlds that Widen will merge;
//   - fixed-coordinate object precision, only once every family is singular.
//
// The third component deliberately scores the semantic partition coordinates
// owned by Object, not sparse exception storage. No component is a work,
// depth, cardinality, or iteration budget.
type WidenRank struct {
	schema       Schema
	maxObjectSum uint64
}

func NewWidenRank(schema Schema) (WidenRank, bool) {
	if !schema.valid() || schema.owner == nil || schema.owner.fixedObjectRankBound == 0 || schema.owner.maxObjectRankSum == 0 {
		return WidenRank{}, false
	}
	// Schema.finish already checked the fixed-coordinate Object bound and the
	// at-most-three-object sum with overflow arithmetic. A sealed Schema is
	// consequently sufficient to construct this witness: runtime analysis
	// cannot discover a new cardinality or fail rank construction.
	return WidenRank{schema: schema, maxObjectSum: schema.owner.maxObjectRankSum}, true
}

func (rank WidenRank) Width() int {
	if !rank.schema.valid() {
		return 0
	}
	return 3
}

func (rank WidenRank) At(key Key, value Value, component int) uint64 {
	if component < 0 || component >= rank.Width() || !key.valid() || key.owner != rank.schema.owner || !rank.schema.Admits(key, value) {
		return 0
	}
	if value.top {
		return 0
	}
	families, active, ok := activeWorldFamilies(key, value)
	if !ok || active > families {
		return 0
	}
	switch component {
	case 0:
		// Top is the terminal 0. Every non-Top Value, including Bottom,
		// has a positive phase rank, so a finite-to-Top widening descends.
		return 1 + families - active
	case 1:
		return uint64(len(value.worlds)) - active
	case 2:
		if len(value.worlds) != int(active) {
			return 0
		}
		score, ok := valueObjectWidenScore(value)
		if !ok || score > rank.maxObjectSum {
			return 0
		}
		return score
	default:
		return 0
	}
}

func activeWorldFamilies(key Key, value Value) (families, active uint64, ok bool) {
	if !key.valid() || !value.valid() || key.owner != value.owner {
		return 0, 0, false
	}
	var seen [WorldMany + 1]bool
	switch key.Kind() {
	case RootAllocation:
		families = 3
	case RootBoot:
		families = 1
	default:
		return 0, 0, false
	}
	for _, world := range value.worlds {
		if !world.valid() {
			return 0, 0, false
		}
		allowed := key.Kind() == RootAllocation && (world.kind == WorldZero || world.kind == WorldOne || world.kind == WorldMany) ||
			key.Kind() == RootBoot && world.kind == WorldExact
		if !allowed {
			return 0, 0, false
		}
		if !seen[world.kind] {
			seen[world.kind] = true
			active++
		}
	}
	return families, active, true
}

func valueObjectWidenScore(value Value) (uint64, bool) {
	if !value.valid() || value.top {
		return 0, false
	}
	var score uint64
	add := func(object Object) bool {
		part, ok := objectWidenScore(object)
		if !ok {
			return false
		}
		next, ok := safeAdd(score, part)
		if !ok {
			return false
		}
		score = next
		return true
	}
	for _, world := range value.worlds {
		switch world.kind {
		case WorldZero:
		case WorldExact:
			if !add(world.exact) {
				return 0, false
			}
		case WorldOne:
			if !add(world.recent) {
				return 0, false
			}
		case WorldMany:
			if !add(world.recent) || !add(world.summary) {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	return score, true
}
