package relation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/internal/hash"
	luatable "github.com/wippyai/go-lua/analysis/lua/table"
	"github.com/wippyai/go-lua/analysis/type/coalesce"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/gradual"
	"github.com/wippyai/go-lua/analysis/type/kind"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

const recursiveRecordFamilyName = "FlowJoin"

// SlotJoinFunc joins two nested product slots while relation owns the
// surrounding return/product orchestration.
type SlotJoinFunc = coalesce.SlotJoinFunc

// JoinPreferNonSoft joins two types while preferring non-soft placeholders.
// This centralizes the "soft placeholder" policy used across inference and flow.
func JoinPreferNonSoft(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = gradual.PruneSoftUnionMembers(a)
	b = gradual.PruneSoftUnionMembers(b)
	if gradual.IsSoft(a, gradual.SoftPlaceholderPolicy) && !gradual.IsSoft(b, gradual.SoftPlaceholderPolicy) {
		return b
	}
	if gradual.IsSoft(b, gradual.SoftPlaceholderPolicy) && !gradual.IsSoft(a, gradual.SoftPlaceholderPolicy) {
		return a
	}
	// Inline join.Two to avoid dependency cycles inside typ.
	if IsAbsentOrUnknown(a) {
		return b
	}
	if IsAbsentOrUnknown(b) {
		return a
	}
	if SameNodeOrAcyclicEqual(a, b) {
		return a
	}
	return gradual.PruneSoftUnionMembers(NormalizeUnionForJoin(a, b))
}

// JoinReturnSlot merges return slot types while preserving uncertainty.
//
// Unknown in return inference means unresolved runtime behavior. When one branch
// is unknown and another is explicit nil, keep unknown so summaries do not
// collapse to nil-only.
func JoinReturnSlot(a, b Type) Type {
	return newReturnJoinState().joinReturnSlot(a, b)
}

type returnJoinKey struct {
	aHash uint64
	bHash uint64
	aKind kind.Kind
	bKind kind.Kind
}

type recordJoinResult struct {
	t  Type
	ok bool
}

type recursiveRewriteKey struct {
	bodyKind kind.Kind
	bodyPtr  uintptr
	bodyHash uint64
	fromID   uint64
	toID     uint64
}

type returnJoinState struct {
	returnSlots         map[returnJoinKey]Type
	records             map[returnJoinKey]recordJoinResult
	recursiveRewrites   map[recursiveRewriteKey]Type
	discriminants       *discriminant.Detector
	precision           *precisionSeen
	recursiveFamilyFold bool
}

func newReturnJoinState() *returnJoinState {
	return &returnJoinState{}
}

func (s *returnJoinState) discriminantDetector() *discriminant.Detector {
	if s == nil {
		return discriminant.NewDetector()
	}
	if s.discriminants == nil {
		s.discriminants = discriminant.NewDetector()
	}
	return s.discriminants
}

func (s *returnJoinState) slotJoinOrDefault(slotJoin SlotJoinFunc) SlotJoinFunc {
	if slotJoin != nil {
		return slotJoin
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	return state.joinReturnSlot
}

func (s *returnJoinState) recordPolicy(slotJoin SlotJoinFunc) coalesce.RecordPolicy {
	return coalesce.RecordPolicy{
		SlotJoin:      s.slotJoinOrDefault(slotJoin),
		KeyJoin:       JoinPreferNonSoft,
		Discriminants: s.discriminantDetector(),
	}
}

func (s *returnJoinState) joinKey(a, b Type) returnJoinKey {
	if a == nil || b == nil {
		return returnJoinKey{}
	}
	ah, bh := returnJoinHash(a), returnJoinHash(b)
	ak, bk := a.Kind(), b.Kind()
	if ah > bh || (ah == bh && ak > bk) {
		ah, bh = bh, ah
		ak, bk = bk, ak
	}
	return returnJoinKey{aHash: ah, bHash: bh, aKind: ak, bKind: bk}
}

func returnJoinHash(t Type) uint64 {
	if t == nil {
		return 0
	}
	if ContainsRecursive(t) {
		if ptr := TypePointer(t); ptr != 0 {
			return hash.HashCombine(uint64(t.Kind()), uint64(ptr))
		}
		return productFamilyHash(t)
	}
	return EqualityHash(t)
}

func (s *returnJoinState) sameReturnJoinInput(a, b Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		if s != nil && s.recursiveFamilyFold {
			return false
		}
		return sameProductFamily(a, b)
	}
	return EqualityHash(a) == EqualityHash(b) && TypeEquals(a, b)
}

func (s *returnJoinState) joinReturnSlot(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = gradual.PruneSoftUnionMembers(a)
	b = gradual.PruneSoftUnionMembers(b)
	if s.sameReturnJoinInput(a, b) {
		return a
	}
	key := s.joinKey(a, b)
	if s != nil && s.returnSlots != nil {
		if cached, ok := s.returnSlots[key]; ok {
			return cached
		}
	}

	var result Type
	if preferred, ok := coalesce.PreferArrayOverEmptyRecord(a, b); ok {
		result = preferred
	} else if merged, ok := s.joinCompatibleRecordsWithSlotJoin(a, b, s.joinReturnSlot); ok {
		result = merged
	} else if (IsAny(a) && b.Kind() == kind.Nil) || (IsAny(b) && a.Kind() == kind.Nil) {
		result = Any
	} else if concrete, ok := concreteScalarOverUnknownReturnSlot(a, b); ok {
		result = concrete
	} else if IsUnknown(a) || IsUnknown(b) {
		result = Unknown
	} else {
		result = s.joinCoalescedUnionWithSlotJoin(a, b, s.joinReturnSlot)
	}
	if s != nil {
		if s.returnSlots == nil {
			s.returnSlots = make(map[returnJoinKey]Type)
		}
		s.returnSlots[key] = result
	}
	return result
}

// concreteScalarOverUnknownReturnSlot prefers concrete scalar evidence over a
// bare unknown peer. A bare unknown is unresolved evidence ("no value yet"), not
// the dynamic top: in the convergence evidence lattice it is below a solved
// scalar, so the least upper bound of a scalar and an unknown peer is the scalar,
// not unknown. Widening the scalar back to unknown drops precision and, because
// whether two observations reach this join depends on map-iteration order, lets a
// record field (e.g. full_path: string) flip to unknown across runs. The rule is
// symmetric and limited to scalar primitives, matching the documented
// return-summary policy that bare unknown yields to concrete scalar evidence
// while structural unknown stays load-bearing for gradual member access.
func concreteScalarOverUnknownReturnSlot(a, b Type) (Type, bool) {
	aUnknown := IsUnknown(UnwrapAnnotated(a))
	bUnknown := IsUnknown(UnwrapAnnotated(b))
	if aUnknown == bUnknown {
		return nil, false
	}
	concrete := a
	if aUnknown {
		concrete = b
	}
	if !isScalarReturnSlotEvidence(concrete) {
		return nil, false
	}
	return concrete, true
}

func isScalarReturnSlotEvidence(t Type) bool {
	base := UnwrapAnnotated(t)
	if base == nil {
		return false
	}
	k := base.Kind()
	if k == kind.Literal {
		if lit, ok := base.(*Literal); ok {
			k = lit.Base
		}
	}
	switch k {
	case kind.Number, kind.Integer, kind.String, kind.Boolean:
		return true
	default:
		return false
	}
}

// JoinCompatibleRecords joins two record types into a single record when they
// are structurally compatible for safe optional-field widening.
//
// This preserves discriminated unions by refusing joins when required literal
// fields conflict across the two records.
func JoinCompatibleRecords(a, b Type) (Type, bool) {
	return JoinCompatibleRecordsWithSlotJoin(a, b, nil)
}

// JoinCompatibleRecordsWithSlotJoin joins two record types using slotJoin for
// nested field/static/container slots. A nil slotJoin preserves JoinReturnSlot
// behavior.
func JoinCompatibleRecordsWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) (Type, bool) {
	state := newReturnJoinState()
	return state.joinCompatibleRecordsWithSlotJoin(a, b, state.slotJoinOrDefault(slotJoin))
}

// JoinClosedCompatibleRecordSet joins a compatible set of closed, non-map
// records in one pass. It is the bulk form of JoinCompatibleRecords for large
// record unions where repeated pair joins would rebuild the same optional-field
// product many times.
func JoinClosedCompatibleRecordSet(records []*Record) (Type, bool) {
	return JoinClosedCompatibleRecordSetWithSlotJoin(records, nil)
}

// JoinClosedCompatibleRecordSetWithSlotJoin joins a compatible set of closed,
// non-map records using slotJoin for repeated field/static member slots. A nil
// slotJoin preserves JoinReturnSlot behavior.
func JoinClosedCompatibleRecordSetWithSlotJoin(records []*Record, slotJoin SlotJoinFunc) (Type, bool) {
	state := newReturnJoinState()
	return state.joinClosedCompatibleRecordSetWithSlotJoin(records, state.slotJoinOrDefault(slotJoin))
}

func (s *returnJoinState) joinClosedCompatibleRecordSet(records []*Record) (Type, bool) {
	return s.joinClosedCompatibleRecordSetWithSlotJoin(records, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) joinClosedCompatibleRecordSetWithSlotJoin(records []*Record, slotJoin SlotJoinFunc) (Type, bool) {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	return coalesce.JoinClosedCompatibleRecordSet(records, state.recordPolicy(slotJoin))
}

// CoalesceCompatibleRecords merges compatible record alternatives before a
// union is constructed. It is the canonical batch form for return-slot and flow
// joins that would otherwise repeatedly build large transient unions.
func CoalesceCompatibleRecords(types []Type) []Type {
	return CoalesceCompatibleRecordsWithSlotJoin(types, nil)
}

// CoalesceCompatibleRecordsWithSlotJoin merges compatible record alternatives
// using slotJoin for nested field/static/container slots. A nil slotJoin
// preserves JoinReturnSlot behavior.
func CoalesceCompatibleRecordsWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	state := newReturnJoinState()
	return state.coalesceCompatibleRecordTypesWithSlotJoin(types, state.slotJoinOrDefault(slotJoin))
}

// CoalesceCompatibleRecordAlternatives canonicalizes compatible record members
// inside one union or optional union without changing non-record alternatives.
func CoalesceCompatibleRecordAlternatives(t Type) Type {
	return CoalesceCompatibleRecordAlternativesWithSlotJoin(t, nil)
}

// CoalesceCompatibleRecordAlternativesWithSlotJoin canonicalizes compatible
// record members inside one union or optional union using slotJoin for nested
// slots. A nil slotJoin preserves JoinReturnSlot behavior.
func CoalesceCompatibleRecordAlternativesWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := newReturnJoinState()
	return state.coalesceCompatibleRecordAlternativesWithSlotJoin(t, state.slotJoinOrDefault(slotJoin))
}

// CoalesceProductUnionMembers applies the canonical product-level union
// compaction used before projection, field lookup, precision comparison, and
// fact storage. It keeps variant/discriminant alternatives intact while
// collapsing recursive record-family construction histories and compatible
// record observations.
func CoalesceProductUnionMembers(types []Type) []Type {
	return CoalesceProductUnionMembersWithSlotJoin(types, nil)
}

// CoalesceProductUnionMembersWithSlotJoin applies product-level union
// compaction using slotJoin for nested record/product slots. A nil slotJoin
// preserves JoinReturnSlot behavior.
func CoalesceProductUnionMembersWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	state := newReturnJoinState()
	return state.coalesceProductUnionMembersWithSlotJoin(types, state.slotJoinOrDefault(slotJoin))
}

func (s *returnJoinState) coalesceProductUnionMembers(types []Type) []Type {
	return s.coalesceProductUnionMembersWithSlotJoin(types, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceProductUnionMembersWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	out := types
	if !state.recursiveFamilyFold {
		out = state.coalesceRecursiveRecordFamiliesWithSlotJoin(out, slotJoin)
	} else {
		out = state.coalesceFoldedProductFamilyMembers(out)
	}
	out = state.coalesceCompatibleRecordTypesWithSlotJoin(out, slotJoin)
	return out
}

func (s *returnJoinState) coalesceFoldedProductFamilyMembers(types []Type) []Type {
	if len(types) < 2 {
		return types
	}
	out := make([]Type, 0, len(types))
	type familyRep struct {
		hash uint64
		rep  Type
	}
	var recReps []familyRep
	seenNodes := make(map[uintptr]bool)
	changed := false
	for _, candidate := range types {
		if candidate == nil {
			changed = true
			continue
		}
		rec := unaliasRecursive(candidate)
		if rec == nil {
			out = append(out, candidate)
			continue
		}
		if ptr := TypePointer(rec); ptr != 0 && seenNodes[ptr] {
			changed = true
			continue
		}
		// Distinct recursive handles that denote the same product family (a fresh
		// *Recursive is minted each fixpoint iteration) cannot be detected by
		// pointer identity, so dedup by the coinductive family hash refined with
		// the structural same-family probe.
		h := productFamilyHash(rec)
		duplicate := false
		for _, r := range recReps {
			if r.hash == h && sameProductFamily(r.rep, rec) {
				duplicate = true
				break
			}
		}
		if duplicate {
			changed = true
			continue
		}
		recReps = append(recReps, familyRep{hash: h, rep: rec})
		if ptr := TypePointer(rec); ptr != 0 {
			seenNodes[ptr] = true
		}
		out = append(out, candidate)
	}
	if !changed {
		return types
	}
	return out
}

func (s *returnJoinState) sameProductFamily(a, b Type) bool {
	seen := s.precisionState()
	return identity.SameProductFamilyWithPrecisionAndCache(a, b, func(candidate, baseline Type) (bool, bool) {
		return comparePrecision(candidate, baseline, 0, seen)
	}, productFamilyHashCache(seen))
}

func (s *returnJoinState) productFamilyHash(t Type) uint64 {
	if s == nil {
		return identity.ProductFamilyHash(t)
	}
	return identity.ProductFamilyHashWithCache(t, productFamilyHashCache(s.precisionState()))
}

func (s *returnJoinState) precisionState() *precisionSeen {
	if s == nil {
		return &precisionSeen{}
	}
	if s.precision == nil {
		s.precision = &precisionSeen{}
	}
	return s.precision
}

// CoalesceProductUnion canonicalizes union-bearing values with the
// product-level member compaction law. Non-union values are returned unchanged.
func CoalesceProductUnion(t Type) Type {
	return CoalesceProductUnionWithSlotJoin(t, nil)
}

// CoalesceProductUnionWithSlotJoin canonicalizes union-bearing values using
// slotJoin for nested record/product slots. A nil slotJoin preserves
// JoinReturnSlot behavior.
func CoalesceProductUnionWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := newReturnJoinState()
	return state.coalesceProductUnionWithSlotJoin(t, state.slotJoinOrDefault(slotJoin))
}

func (s *returnJoinState) coalesceProductUnion(t Type) Type {
	return s.coalesceProductUnionWithSlotJoin(t, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceProductUnionWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	switch v := UnwrapAnnotated(t).(type) {
	case *Optional:
		inner := state.coalesceProductUnionWithSlotJoin(v.Inner, slotJoin)
		if SameNodeOrAcyclicEqual(inner, v.Inner) {
			return t
		}
		return NewOptional(inner)
	case *Union:
		if v == nil || len(v.Members) < 2 {
			return t
		}
		members := state.coalesceProductUnionMembersWithSlotJoin(v.Members, slotJoin)
		if sameTypeSlice(v.Members, members) {
			return t
		}
		return NormalizeUnionForJoin(members...)
	default:
		return t
	}
}

// CoalesceRecursiveRecordFamilies merges recursive record observations that
// describe the same inferred table family. The recursive wrapper is the
// finite-height representation of one abstract table family; compatible
// observations join into one recursive product with optional/merged body fields
// instead of a growing union of construction histories.
func CoalesceRecursiveRecordFamilies(types []Type) []Type {
	return CoalesceRecursiveRecordFamiliesWithSlotJoin(types, nil)
}

// CoalesceRecursiveRecordFamiliesWithSlotJoin merges recursive record
// observations using slotJoin for nested body slots. A nil slotJoin preserves
// JoinReturnSlot behavior.
func CoalesceRecursiveRecordFamiliesWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	state := newReturnJoinState()
	return state.coalesceRecursiveRecordFamiliesWithSlotJoin(types, state.slotJoinOrDefault(slotJoin))
}

func (s *returnJoinState) coalesceRecursiveRecordFamilies(types []Type) []Type {
	return s.coalesceRecursiveRecordFamiliesWithSlotJoin(types, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceRecursiveRecordFamiliesWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	types = state.coalesceFoldedProductFamilyMembers(types)
	if len(types) < 2 {
		return types
	}

	recs := make([]*Recursive, len(types))
	keys := make([]uint64, len(types))
	buckets := make(map[uint64][]int)
	for i, t := range types {
		rec := unaliasRecursive(t)
		if rec == nil {
			continue
		}
		key := state.recursiveRecordCoalesceKey(rec)
		recs[i] = rec
		keys[i] = key
		buckets[key] = append(buckets[key], i)
	}

	used := make([]bool, len(types))
	out := make([]Type, 0, len(types))
	for i, t := range types {
		if used[i] {
			continue
		}
		rec := recs[i]
		if rec == nil {
			out = append(out, t)
			continue
		}

		merged := NewRecursivePlaceholder(recursiveRecordFamilyName)
		body := state.rewriteRecursiveFamilySelf(rec.Body, rec, merged)
		mergedAny := false
		bodyChanged := false
		for _, j := range buckets[keys[i]] {
			if j <= i {
				continue
			}
			if used[j] {
				continue
			}
			next := recs[j]
			if next == nil {
				continue
			}
			nextBody := state.rewriteRecursiveFamilySelf(next.Body, next, merged)
			if !recursiveFamilyBodiesShareAnchor(body, nextBody) {
				continue
			}
			joinedBody, ok := state.joinRecursiveFamilyBodiesWithSlotJoin(body, nextBody, slotJoin)
			if !ok {
				continue
			}
			if !sameRecursiveFamilyBody(body, joinedBody) {
				bodyChanged = true
			}
			body = joinedBody
			mergedAny = true
			used[j] = true
		}
		if !mergedAny || !bodyChanged {
			out = append(out, t)
			continue
		}
		merged.SetBody(body)
		out = append(out, merged)
	}
	return out
}

func (s *returnJoinState) recursiveRecordCoalesceKey(rec *Recursive) uint64 {
	if rec == nil {
		return 0
	}
	body, _ := UnwrapAnnotated(rec.Body).(*Record)
	if body == nil {
		return productFamilyHash(rec)
	}
	h := hash.HashCombine(uint64(kind.Recursive), uint64(kind.Record))
	if body.HasMapComponent() {
		h = hash.HashCombine(h, 1)
	} else {
		h = hash.HashCombine(h, 0)
	}
	tags := s.discriminantDetector().RequiredTags(body)
	if len(tags) == 0 {
		return h
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		h = hash.HashCombine(h, hash.FnvString(key))
		h = hash.HashCombine(h, tags[key])
	}
	return h
}

func sameRecursiveFamilyBody(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if !ContainsRecursive(a) && !ContainsRecursive(b) {
		return false
	}
	return sameProductFamily(a, b)
}

func recursiveFamilyBodiesShareAnchor(a, b Type) bool {
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar == nil || br == nil {
		return false
	}
	i, j := 0, 0
	for i < len(ar.Fields) && j < len(br.Fields) {
		left := ar.Fields[i]
		right := br.Fields[j]
		switch {
		case left.Name == right.Name:
			if (!left.Optional || !right.Optional) && recursiveFamilyAnchorTypesCompatible(left.Type, right.Type) {
				return true
			}
			i++
			j++
		case left.Name < right.Name:
			i++
		default:
			j++
		}
	}
	return false
}

func recursiveFamilyAnchorTypesCompatible(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		return sameProductFamily(a, b)
	}
	return TypeEquals(a, b)
}

func (s *returnJoinState) joinRecursiveFamilyBodies(a, b Type) (Type, bool) {
	return s.joinRecursiveFamilyBodiesWithSlotJoin(a, b, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) joinRecursiveFamilyBodiesWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) (Type, bool) {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	previous := state.recursiveFamilyFold
	state.recursiveFamilyFold = true
	defer func() {
		state.recursiveFamilyFold = previous
	}()
	return state.joinCompatibleRecordsWithSlotJoin(a, b, slotJoin)
}

func unaliasRecursive(t Type) *Recursive {
	for {
		a, ok := t.(*Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	rec, _ := t.(*Recursive)
	return rec
}

func (s *returnJoinState) rewriteRecursiveFamilySelf(t Type, from, to *Recursive) Type {
	if from == nil || to == nil {
		return t
	}
	if from == to {
		return t
	}
	if s == nil {
		s = newReturnJoinState()
	}
	key, ok := recursiveRewriteCacheKey(t, from, to)
	if ok && s.recursiveRewrites != nil {
		if cached, found := s.recursiveRewrites[key]; found {
			return cached
		}
	}
	out := Rewrite(t, func(node Type) (Type, bool) {
		if IsRecursiveRef(node, from) {
			return to, true
		}
		return nil, false
	})
	if ok {
		if s.recursiveRewrites == nil {
			s.recursiveRewrites = make(map[recursiveRewriteKey]Type)
		}
		s.recursiveRewrites[key] = out
	}
	return out
}

func recursiveRewriteCacheKey(t Type, from, to *Recursive) (recursiveRewriteKey, bool) {
	if t == nil || from == nil || to == nil {
		return recursiveRewriteKey{}, false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return recursiveRewriteKey{}, false
	}
	ptr := TypePointer(t)
	key := recursiveRewriteKey{
		bodyKind: t.Kind(),
		bodyPtr:  ptr,
		fromID:   from.ID,
		toID:     to.ID,
	}
	if ptr == 0 {
		key.bodyHash = EqualityHash(t)
	}
	return key, true
}

func (s *returnJoinState) coalesceCompatibleRecordAlternatives(t Type) Type {
	return s.coalesceCompatibleRecordAlternativesWithSlotJoin(t, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceCompatibleRecordAlternativesWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	switch v := UnwrapAnnotated(t).(type) {
	case *Optional:
		inner := state.coalesceCompatibleRecordAlternativesWithSlotJoin(v.Inner, slotJoin)
		if SameNodeOrAcyclicEqual(inner, v.Inner) {
			return t
		}
		return NewOptional(inner)
	case *Union:
		if len(v.Members) < 2 {
			return t
		}
		members := make([]Type, len(v.Members))
		copy(members, v.Members)
		coalesced := state.coalesceCompatibleRecordTypesWithSlotJoin(members, slotJoin)
		if sameTypeSlice(members, coalesced) {
			return t
		}
		return NormalizeUnionForJoin(coalesced...)
	default:
		return t
	}
}

func (s *returnJoinState) joinCoalescedUnion(a, b Type) Type {
	return s.joinCoalescedUnionWithSlotJoin(a, b, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) joinCoalescedUnionWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) Type {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	members := make([]Type, 0, 4)
	members = appendUnionMembers(members, a)
	members = appendUnionMembers(members, b)
	members = state.coalesceProductUnionMembersWithSlotJoin(members, slotJoin)
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return gradual.PruneSoftUnionMembers(members[0])
	}
	return gradual.PruneSoftUnionMembers(NormalizeUnionForJoin(members...))
}

func appendUnionMembers(out []Type, t Type) []Type {
	if t == nil {
		return out
	}
	if u, ok := UnwrapAnnotated(t).(*Union); ok {
		for _, member := range u.Members {
			out = appendUnionMembers(out, member)
		}
		return out
	}
	if opt, ok := UnwrapAnnotated(t).(*Optional); ok {
		out = append(out, Nil)
		return appendUnionMembers(out, opt.Inner)
	}
	return append(out, t)
}

func (s *returnJoinState) coalesceCompatibleRecordTypes(types []Type) []Type {
	return s.coalesceCompatibleRecordTypesWithSlotJoin(types, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceCompatibleRecordTypesWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	if fast, ok, complete := state.coalesceClosedCompatibleRecordsWithSlotJoin(types, slotJoin); ok {
		if complete {
			return fast
		}
		types = fast
	}
	if fast, ok := state.coalesceCompatibleRecordGroupsWithSlotJoin(types, slotJoin); ok {
		return fast
	}
	return state.coalesceCompatibleRecordsPairwiseWithSlotJoin(types, slotJoin)
}

type compatibleRecordGroup struct {
	indices []int
	records []*Record
	hasMap  bool
}

func (s *returnJoinState) coalesceCompatibleRecordGroups(types []Type) ([]Type, bool) {
	return s.coalesceCompatibleRecordGroupsWithSlotJoin(types, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceCompatibleRecordGroupsWithSlotJoin(types []Type, slotJoin SlotJoinFunc) ([]Type, bool) {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	groups := make([]*compatibleRecordGroup, 0, len(types))
	for i, t := range types {
		rec := unaliasRecord(t)
		if rec == nil {
			continue
		}
		var group *compatibleRecordGroup
		for _, candidate := range groups {
			if candidate.hasMap == rec.HasMapComponent() && coalesce.RecordMergesIntoGroup(rec, candidate.records, state.discriminantDetector()) {
				group = candidate
				break
			}
		}
		if group == nil {
			group = &compatibleRecordGroup{hasMap: rec.HasMapComponent()}
			groups = append(groups, group)
		}
		group.indices = append(group.indices, i)
		group.records = append(group.records, rec)
	}

	changed := false
	mergedAt := make(map[int]Type)
	skip := make(map[int]bool)
	for _, group := range groups {
		if len(group.records) < 2 {
			continue
		}
		merged, ok := state.joinCompatibleRecordSetWithSlotJoin(group.records, slotJoin)
		if !ok {
			return nil, false
		}
		changed = true
		mergedAt[group.indices[0]] = merged
		for _, idx := range group.indices[1:] {
			skip[idx] = true
		}
	}
	if !changed {
		return nil, false
	}

	out := make([]Type, 0, len(types)-len(skip))
	for i, t := range types {
		if merged, ok := mergedAt[i]; ok {
			out = append(out, merged)
			continue
		}
		if skip[i] {
			continue
		}
		out = append(out, t)
	}
	return out, true
}

func (s *returnJoinState) joinCompatibleRecordSet(records []*Record) (Type, bool) {
	return s.joinCompatibleRecordSetWithSlotJoin(records, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) joinCompatibleRecordSetWithSlotJoin(records []*Record, slotJoin SlotJoinFunc) (Type, bool) {
	if len(records) == 0 {
		return nil, false
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	current := Type(records[0])
	for _, rec := range records[1:] {
		merged, ok := state.joinCompatibleRecordsWithSlotJoin(current, rec, slotJoin)
		if !ok {
			return nil, false
		}
		current = merged
	}
	return current, true
}

func (s *returnJoinState) coalesceCompatibleRecordsPairwise(types []Type) []Type {
	return s.coalesceCompatibleRecordsPairwiseWithSlotJoin(types, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceCompatibleRecordsPairwiseWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	used := make([]bool, len(types))
	out := make([]Type, 0, len(types))
	for i := 0; i < len(types); i++ {
		if used[i] {
			continue
		}
		current := types[i]
		currentRecord := unaliasRecord(current)
		if currentRecord == nil {
			out = append(out, current)
			continue
		}
		for j := i + 1; j < len(types); j++ {
			if used[j] {
				continue
			}
			candidateRecord := unaliasRecord(types[j])
			if candidateRecord == nil {
				continue
			}
			merged, ok := state.joinCompatibleRecordsWithSlotJoin(currentRecord, candidateRecord, slotJoin)
			if !ok {
				continue
			}
			current = merged
			currentRecord = unaliasRecord(merged)
			if currentRecord == nil {
				break
			}
			used[j] = true
		}
		out = append(out, current)
	}
	return out
}

func (s *returnJoinState) coalesceClosedCompatibleRecords(types []Type) ([]Type, bool, bool) {
	return s.coalesceClosedCompatibleRecordsWithSlotJoin(types, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) coalesceClosedCompatibleRecordsWithSlotJoin(types []Type, slotJoin SlotJoinFunc) ([]Type, bool, bool) {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	return coalesce.CoalesceClosedCompatibleRecords(types, state.recordPolicy(slotJoin))
}

func (s *returnJoinState) joinCompatibleRecords(a, b Type) (Type, bool) {
	return s.joinCompatibleRecordsWithSlotJoin(a, b, s.slotJoinOrDefault(nil))
}

func (s *returnJoinState) joinCompatibleRecordsWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) (Type, bool) {
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar == nil || br == nil {
		return nil, false
	}
	if state.sameReturnJoinInput(ar, br) {
		return ar, true
	}
	key := state.joinKey(ar, br)
	if state.records != nil {
		if cached, ok := state.records[key]; ok {
			return cached.t, cached.ok
		}
	}

	// Keep discriminated unions intact when required literal tags conflict.
	if state.discriminantDetector().RecordsConflict(ar, br) {
		state.cacheRecordJoin(key, nil, false)
		return nil, false
	}

	// Mixing map and non-map record slots can be semantically distinct.
	if ar.HasMapComponent() != br.HasMapComponent() {
		state.cacheRecordJoin(key, nil, false)
		return nil, false
	}
	if !coalesce.CompatibleRecordMetatables(ar, br) {
		state.cacheRecordJoin(key, nil, false)
		return nil, false
	}

	open := ar.Open || br.Open
	metatable := Type(nil)
	if ar.Metatable != nil && br.Metatable != nil && SameNodeOrAcyclicEqual(ar.Metatable, br.Metatable) {
		metatable = ar.Metatable
	}
	mapKey := Type(nil)
	mapValue := Type(nil)
	if ar.HasMapComponent() && br.HasMapComponent() {
		mapKey = JoinPreferNonSoft(ar.MapKey, br.MapKey)
		mapValue = JoinPreferNonSoft(ar.MapValue, br.MapValue)
	}

	fields := make([]Field, 0, len(ar.Fields)+len(br.Fields))
	i, j := 0, 0
	for i < len(ar.Fields) || j < len(br.Fields) {
		switch {
		case j >= len(br.Fields):
			fields = append(fields, state.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, Field{}, false, ar, br, slotJoin))
			i++
		case i >= len(ar.Fields):
			fields = append(fields, state.mergeRecordField(br.Fields[j].Name, Field{}, false, br.Fields[j], true, ar, br, slotJoin))
			j++
		case ar.Fields[i].Name == br.Fields[j].Name:
			fields = append(fields, state.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, br.Fields[j], true, ar, br, slotJoin))
			i++
			j++
		case ar.Fields[i].Name < br.Fields[j].Name:
			fields = append(fields, state.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, Field{}, false, ar, br, slotJoin))
			i++
		default:
			fields = append(fields, state.mergeRecordField(br.Fields[j].Name, Field{}, false, br.Fields[j], true, ar, br, slotJoin))
			j++
		}
	}

	staticMembers := state.mergeRecordStaticMembers(ar, br, slotJoin)
	merged := luatable.RebuildRecord(RecordParts{
		Fields:        fields,
		StaticMembers: staticMembers,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          open,
		AssumeSorted:  true,
	})
	state.cacheRecordJoin(key, merged, true)
	return merged, true
}

func (s *returnJoinState) cacheRecordJoin(key returnJoinKey, t Type, ok bool) {
	if s.records == nil {
		s.records = make(map[returnJoinKey]recordJoinResult)
	}
	s.records[key] = recordJoinResult{t: t, ok: ok}
}

func (s *returnJoinState) mergeRecordField(name string, fa Field, oka bool, fb Field, okb bool, ar, br *Record, slotJoin SlotJoinFunc) Field {
	slotJoin = s.slotJoinOrDefault(slotJoin)
	fieldType := Type(nil)
	optional := true
	readonly := false
	switch {
	case oka && okb:
		// Keep field-level merge on the caller's slot policy so
		// empty-collection paths and nil/unknown interactions stay consistent
		// with the surrounding join.
		fieldType = coalesce.JoinRecordFieldSlot(fa.Type, fb.Type, s.recordPolicy(slotJoin))
		optional = fa.Optional || fb.Optional
		readonly = fa.Readonly && fb.Readonly
	case oka:
		fieldType = fa.Type
		optional = true
		readonly = fa.Readonly
		if tail, ok := luatable.RecordTailFieldType(br, name); ok {
			fieldType, optional = normalizeMergedRecordField(slotJoin(fa.Type, tail))
			if luatable.RecordMapTailMayContainFieldName(br, name) {
				optional = true
			}
			readonly = false
		}
	case okb:
		fieldType = fb.Type
		optional = true
		readonly = fb.Readonly
		if tail, ok := luatable.RecordTailFieldType(ar, name); ok {
			fieldType, optional = normalizeMergedRecordField(slotJoin(tail, fb.Type))
			if luatable.RecordMapTailMayContainFieldName(ar, name) {
				optional = true
			}
			readonly = false
		}
	}
	return Field{Name: name, Type: fieldType, Optional: optional, Readonly: readonly}
}

func (s *returnJoinState) mergeRecordStaticMembers(ar, br *Record, slotJoin SlotJoinFunc) []StaticMember {
	slotJoin = s.slotJoinOrDefault(slotJoin)
	staticMembers := make([]StaticMember, 0, len(ar.StaticMembers)+len(br.StaticMembers))
	i, j := 0, 0
	for i < len(ar.StaticMembers) || j < len(br.StaticMembers) {
		switch {
		case j >= len(br.StaticMembers):
			staticMembers = append(staticMembers, s.mergeRecordStaticMember(ar.StaticMembers[i], true, StaticMember{}, false, ar, br, slotJoin))
			i++
		case i >= len(ar.StaticMembers):
			staticMembers = append(staticMembers, s.mergeRecordStaticMember(StaticMember{}, false, br.StaticMembers[j], true, ar, br, slotJoin))
			j++
		default:
			cmp := CompareStaticMembers(ar.StaticMembers[i], br.StaticMembers[j])
			switch {
			case cmp == 0:
				staticMembers = append(staticMembers, s.mergeRecordStaticMember(ar.StaticMembers[i], true, br.StaticMembers[j], true, ar, br, slotJoin))
				i++
				j++
			case cmp < 0:
				staticMembers = append(staticMembers, s.mergeRecordStaticMember(ar.StaticMembers[i], true, StaticMember{}, false, ar, br, slotJoin))
				i++
			default:
				staticMembers = append(staticMembers, s.mergeRecordStaticMember(StaticMember{}, false, br.StaticMembers[j], true, ar, br, slotJoin))
				j++
			}
		}
	}
	return staticMembers
}

func (s *returnJoinState) mergeRecordStaticMember(ma StaticMember, oka bool, mb StaticMember, okb bool, ar, br *Record, slotJoin SlotJoinFunc) StaticMember {
	slotJoin = s.slotJoinOrDefault(slotJoin)
	member := ma
	memberType := Type(nil)
	optional := true
	readonly := false
	switch {
	case oka && okb:
		memberType = coalesce.JoinRecordFieldSlot(ma.Type, mb.Type, s.recordPolicy(slotJoin))
		optional = ma.Optional || mb.Optional
		readonly = ma.Readonly && mb.Readonly
	case oka:
		memberType = ma.Type
		optional = true
		readonly = ma.Readonly
		if tail, ok := luatable.RecordTailStaticMemberType(br, ma); ok {
			memberType, optional = normalizeMergedRecordField(slotJoin(ma.Type, tail))
			if luatable.RecordMapTailMayContainStaticMember(br, ma) {
				optional = true
			}
			readonly = false
		}
	case okb:
		member = mb
		memberType = mb.Type
		optional = true
		readonly = mb.Readonly
		if tail, ok := luatable.RecordTailStaticMemberType(ar, mb); ok {
			memberType, optional = normalizeMergedRecordField(slotJoin(tail, mb.Type))
			if luatable.RecordMapTailMayContainStaticMember(ar, mb) {
				optional = true
			}
			readonly = false
		}
	}
	member.Type = memberType
	member.Optional = optional
	member.Readonly = readonly
	return member
}

// JoinUnionFieldSlot merges the per-member results of reading one field across a
// union. When preserveLiteral is set the field is the union's structural
// discriminant, so distinct literal alternatives are kept for path-sensitive
// narrowing; otherwise ordinary literal data fields widen to their scalar base so
// many-member unions do not explode into giant literal unions on read. The caller
// owns the discriminant decision because it alone holds the union context.
func JoinUnionFieldSlot(a, b Type, preserveLiteral bool) Type {
	s := newReturnJoinState()
	slotJoin := s.slotJoinOrDefault(nil)
	if preserveLiteral {
		if joined, ok := coalesce.JoinFieldContainerSlot(a, b, s.recordPolicy(slotJoin)); ok {
			return joined
		}
		return slotJoin(a, b)
	}
	return coalesce.JoinRecordFieldSlot(a, b, s.recordPolicy(slotJoin))
}

func normalizeMergedRecordField(t Type) (Type, bool) {
	if inner, optional := luatable.SplitNilableField(t); optional {
		return inner, true
	}
	return t, false
}

func unaliasRecord(t Type) *Record {
	return coalesce.UnaliasRecord(t)
}

func sameTypeSlice(a, b []Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !SameNodeOrAcyclicEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// RecordsConflictOnLiteralDiscriminant reports whether two records are discriminated
// variants kept distinct by a shared required literal field rather than coalesced.
// It is the structural, name-free discriminant test shared by the return-slot join
// and the value-domain shape join.
func RecordsConflictOnLiteralDiscriminant(a, b *Record) bool {
	return discriminant.RecordsConflict(a, b)
}

// JoinBranchOutcome merges mutually-exclusive expression outcomes (for example,
// `a and b` / `a or b`) while preserving every runtime possibility.
//
// Unlike inference joins, expression outcomes are value-level alternatives:
// a soft placeholder returned by one branch is still a real possible runtime
// value and must not be pruned just because the other branch is concrete.
func JoinBranchOutcome(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if SameNodeOrAcyclicEqual(a, b) {
		return a
	}
	if IsAny(a) || IsAny(b) {
		return Any
	}
	if IsUnknown(a) && b.Kind() != kind.Nil {
		return Unknown
	}
	if IsUnknown(b) && a.Kind() != kind.Nil {
		return Unknown
	}
	return NormalizeUnionForJoin(a, b)
}
