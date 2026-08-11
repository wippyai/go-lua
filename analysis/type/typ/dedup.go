package typ

import "sort"

type hashedType struct {
	typ  Type
	hash uint64
}

func deduplicateTypesWithHashes(types []Type) ([]Type, []uint64) {
	if len(types) == 0 {
		return nil, nil
	}

	seen := make(map[uint64][]Type)
	result := make([]Type, 0, len(types))
	hashes := make([]uint64, 0, len(types))

	for _, t := range types {
		h := unionMemberHash(t)
		bucket := seen[h]
		duplicate := false

		for _, existing := range bucket {
			if sameUnionMember(existing, t) {
				duplicate = true
				break
			}
		}

		if !duplicate {
			seen[h] = append(bucket, t)
			result = append(result, t)
			hashes = append(hashes, h)
		}
	}

	return result, hashes
}

func sortHashedTypes(types []Type, hashes []uint64) {
	if len(types) != len(hashes) || len(types) < 2 {
		return
	}
	slots := make([]hashedType, len(types))
	for i, t := range types {
		slots[i] = hashedType{typ: t, hash: hashes[i]}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].hash != slots[j].hash {
			return slots[i].hash < slots[j].hash
		}
		if slots[i].typ.String() != slots[j].typ.String() {
			return slots[i].typ.String() < slots[j].typ.String()
		}
		return typePointer(slots[i].typ) < typePointer(slots[j].typ)
	})
	for i, slot := range slots {
		types[i] = slot.typ
		hashes[i] = slot.hash
	}
}

func unionMemberHash(t Type) uint64 {
	if t == nil {
		return 0
	}
	return EqualityHash(t)
}

func sameUnionMember(a, b Type) bool {
	if sameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if knownContainsRecursive(a) || knownContainsRecursive(b) {
		if !sameRecursiveIdentityGraph(a, b) {
			return false
		}
	}
	return typeEquals(a, b)
}

func sameRecursiveIdentityGraph(a, b Type) bool {
	var left, right recursiveIdentitySet
	var leftSeen, rightSeen recursivePointerSet
	collectRecursiveIdentities(a, &left, &leftSeen)
	collectRecursiveIdentities(b, &right, &rightSeen)
	if left.len() != right.len() {
		return false
	}
	return left.all(func(id uint64) bool {
		if !right.has(id) {
			return false
		}
		return true
	})
}

// RecursiveIdentitySignature is a compact, order-independent signature of the
// recursive identities reachable from a type. It is a filter, not structural
// equality: callers must still compare the type bodies before accepting two
// types as equal.
type RecursiveIdentitySignature struct {
	Small    [8]uint64
	SmallLen int
	Overflow bool
}

// RecursiveIdentitySignatureOf returns the recursive identity signature for t.
// The ok result is false when t contains more recursive identities than the
// inline representation can hold; callers can then fall back to graph traversal.
func RecursiveIdentitySignatureOf(t Type) (RecursiveIdentitySignature, bool) {
	var ids recursiveIdentitySet
	var seen recursivePointerSet
	collectRecursiveIdentities(t, &ids, &seen)
	if ids.large != nil {
		return RecursiveIdentitySignature{Overflow: true}, false
	}
	out := RecursiveIdentitySignature{SmallLen: ids.smallLen}
	copy(out.Small[:], ids.small[:])
	sort.Slice(out.Small[:out.SmallLen], func(i, j int) bool {
		return out.Small[i] < out.Small[j]
	})
	return out, true
}

// Equal reports exact equality for two inline recursive identity signatures.
func (s RecursiveIdentitySignature) Equal(other RecursiveIdentitySignature) bool {
	if s.Overflow || other.Overflow || s.SmallLen != other.SmallLen {
		return false
	}
	for i := 0; i < s.SmallLen; i++ {
		if s.Small[i] != other.Small[i] {
			return false
		}
	}
	return true
}

type recursiveIdentitySet struct {
	small    [8]uint64
	smallLen int
	large    map[uint64]struct{}
}

func (s *recursiveIdentitySet) add(id uint64) {
	if s == nil || id == 0 || s.has(id) {
		return
	}
	if s.large != nil {
		s.large[id] = struct{}{}
		return
	}
	if s.smallLen < len(s.small) {
		s.small[s.smallLen] = id
		s.smallLen++
		return
	}
	s.large = make(map[uint64]struct{}, len(s.small)+1)
	for i := 0; i < s.smallLen; i++ {
		s.large[s.small[i]] = struct{}{}
		s.small[i] = 0
	}
	s.smallLen = 0
	s.large[id] = struct{}{}
}

func (s *recursiveIdentitySet) has(id uint64) bool {
	if s == nil || id == 0 {
		return false
	}
	if s.large != nil {
		_, ok := s.large[id]
		return ok
	}
	for i := 0; i < s.smallLen; i++ {
		if s.small[i] == id {
			return true
		}
	}
	return false
}

func (s *recursiveIdentitySet) len() int {
	if s == nil {
		return 0
	}
	if s.large != nil {
		return len(s.large)
	}
	return s.smallLen
}

func (s *recursiveIdentitySet) all(fn func(uint64) bool) bool {
	if s == nil || fn == nil {
		return true
	}
	if s.large != nil {
		for id := range s.large {
			if !fn(id) {
				return false
			}
		}
		return true
	}
	for i := 0; i < s.smallLen; i++ {
		if !fn(s.small[i]) {
			return false
		}
	}
	return true
}

type recursivePointerSet struct {
	small    [128]uintptr
	smallLen int
	large    map[uintptr]struct{}
}

func (s *recursivePointerSet) enter(ptr uintptr) bool {
	if s == nil || ptr == 0 {
		return true
	}
	if s.large != nil {
		if _, ok := s.large[ptr]; ok {
			return false
		}
		s.large[ptr] = struct{}{}
		return true
	}
	for i := 0; i < s.smallLen; i++ {
		if s.small[i] == ptr {
			return false
		}
	}
	if s.smallLen < len(s.small) {
		s.small[s.smallLen] = ptr
		s.smallLen++
		return true
	}
	s.large = make(map[uintptr]struct{}, len(s.small)+1)
	for i := 0; i < s.smallLen; i++ {
		s.large[s.small[i]] = struct{}{}
		s.small[i] = 0
	}
	s.smallLen = 0
	s.large[ptr] = struct{}{}
	return true
}

func collectRecursiveIdentities(t Type, ids *recursiveIdentitySet, seen *recursivePointerSet) {
	if t == nil {
		return
	}
	if seen == nil {
		seen = &recursivePointerSet{}
	}

	// Identity collection is a graph property, not a call-stack property. Keep
	// the traversal explicit so deep product chains and recursive back-edges
	// cannot grow the Go stack. `seen` remains the declaration/node memo: once a
	// pointer has been entered, its descendants have either been scheduled or
	// were already scheduled by the first encounter.
	work := []Type{t}
	for len(work) != 0 {
		last := len(work) - 1
		current := unwrapAnnotatedOrNil(work[last])
		work = work[:last]
		if current == nil {
			continue
		}
		if !seen.enter(typePointer(current)) {
			continue
		}
		if rec, ok := current.(*Recursive); ok {
			ids.add(rec.ID)
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
	}
}
