package typ

import (
	"sort"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

const recursiveRecordFamilyName = "FlowJoin"

// JoinPreferNonSoft joins two types while preferring non-soft placeholders.
// This centralizes the "soft placeholder" policy used across inference and flow.
func JoinPreferNonSoft(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = PruneSoftUnionMembers(a)
	b = PruneSoftUnionMembers(b)
	if IsSoft(a, SoftPlaceholderPolicy) && !IsSoft(b, SoftPlaceholderPolicy) {
		return b
	}
	if IsSoft(b, SoftPlaceholderPolicy) && !IsSoft(a, SoftPlaceholderPolicy) {
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
	return PruneSoftUnionMembers(NewUnion(a, b))
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
	discriminants       map[Type]map[string]uint64
	activeDiscriminants map[Type]bool
	precision           *precisionSeen
	recursiveFamilyFold bool
}

func newReturnJoinState() *returnJoinState {
	return &returnJoinState{}
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
		if ptr := typePointer(t); ptr != 0 {
			return internal.HashCombine(uint64(t.Kind()), uint64(ptr))
		}
		return ProductFamilyHash(t)
	}
	return typeEqualityHash(t)
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
		return s.sameProductFamily(a, b)
	}
	return typeEqualityHash(a) == typeEqualityHash(b) && TypeEquals(a, b)
}

func (s *returnJoinState) joinReturnSlot(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = PruneSoftUnionMembers(a)
	b = PruneSoftUnionMembers(b)
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
	if preferred, ok := preferArrayOverEmptyRecord(a, b); ok {
		result = preferred
	} else if merged, ok := s.joinCompatibleRecords(a, b); ok {
		result = merged
	} else if (IsAny(a) && b.Kind() == kind.Nil) || (IsAny(b) && a.Kind() == kind.Nil) {
		result = Any
	} else if concrete, ok := concreteScalarOverUnknownReturnSlot(a, b); ok {
		result = concrete
	} else if IsUnknown(a) || IsUnknown(b) {
		result = Unknown
	} else {
		result = s.joinCoalescedUnion(a, b)
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

func preferArrayOverEmptyRecord(a, b Type) (Type, bool) {
	if isEmptyRecordNoMap(a) && isArrayLike(b) {
		return b, true
	}
	if isEmptyRecordNoMap(b) && isArrayLike(a) {
		return a, true
	}
	return nil, false
}

func isEmptyRecordNoMap(t Type) bool {
	switch v := t.(type) {
	case *Alias:
		return isEmptyRecordNoMap(v.Target)
	case *Record:
		return len(v.Fields) == 0 && !v.HasMapComponent()
	default:
		return false
	}
}

func isArrayLike(t Type) bool {
	switch v := t.(type) {
	case *Alias:
		return isArrayLike(v.Target)
	case *Array:
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
	return newReturnJoinState().joinCompatibleRecords(a, b)
}

// JoinClosedCompatibleRecordSet joins a compatible set of closed, non-map
// records in one pass. It is the bulk form of JoinCompatibleRecords for large
// record unions where repeated pair joins would rebuild the same optional-field
// product many times.
func JoinClosedCompatibleRecordSet(records []*Record) (Type, bool) {
	return newReturnJoinState().joinClosedCompatibleRecordSet(records)
}

func (s *returnJoinState) joinClosedCompatibleRecordSet(records []*Record) (Type, bool) {
	if len(records) == 0 {
		return nil, false
	}
	if len(records) == 1 {
		return records[0], true
	}
	for _, rec := range records {
		if rec == nil || rec.Open || rec.HasMapComponent() || rec.Metatable != nil || ContainsRecursive(rec) {
			return nil, false
		}
	}
	if s.closedRecordSetHasConflictingRequiredLiteralField(records) {
		return nil, false
	}

	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	type fieldAcc struct {
		fieldType Type
		count     int
		optional  bool
		readonly  bool
	}
	fields := make(map[string]*fieldAcc)
	for _, rec := range records {
		for _, field := range rec.Fields {
			acc := fields[field.Name]
			if acc == nil {
				fields[field.Name] = &fieldAcc{
					fieldType: field.Type,
					count:     1,
					optional:  field.Optional,
					readonly:  field.Readonly,
				}
				continue
			}
			acc.fieldType = state.joinRecordFieldSlot(field.Name, acc.fieldType, field.Type)
			acc.count++
			acc.optional = acc.optional || field.Optional
			acc.readonly = acc.readonly && field.Readonly
		}
	}

	mergedFields := make([]Field, 0, len(fields))
	for name, acc := range fields {
		mergedFields = append(mergedFields, Field{
			Name:     name,
			Type:     acc.fieldType,
			Optional: acc.optional || acc.count < len(records),
			Readonly: acc.readonly,
		})
	}
	return buildRecordType(mergedFields, nil, nil, nil, false, false, false), true
}

// CoalesceCompatibleRecords merges compatible record alternatives before a
// union is constructed. It is the canonical batch form for return-slot and flow
// joins that would otherwise repeatedly build large transient unions.
func CoalesceCompatibleRecords(types []Type) []Type {
	return newReturnJoinState().coalesceCompatibleRecordTypes(types)
}

// CoalesceCompatibleRecordAlternatives canonicalizes compatible record members
// inside one union or optional union without changing non-record alternatives.
func CoalesceCompatibleRecordAlternatives(t Type) Type {
	return newReturnJoinState().coalesceCompatibleRecordAlternatives(t)
}

// CoalesceProductUnionMembers applies the canonical product-level union
// compaction used before projection, field lookup, precision comparison, and
// fact storage. It keeps variant/discriminant alternatives intact while
// collapsing recursive record-family construction histories and compatible
// record observations.
func CoalesceProductUnionMembers(types []Type) []Type {
	return newReturnJoinState().coalesceProductUnionMembers(types)
}

func (s *returnJoinState) coalesceProductUnionMembers(types []Type) []Type {
	if len(types) < 2 {
		return types
	}
	out := types
	if s == nil || !s.recursiveFamilyFold {
		out = s.coalesceRecursiveRecordFamilies(out)
	} else {
		out = s.coalesceFoldedProductFamilyMembers(out)
	}
	out = s.coalesceCompatibleRecordTypes(out)
	return out
}

func coalesceFoldedProductFamilyMembers(types []Type) []Type {
	return newReturnJoinState().coalesceFoldedProductFamilyMembers(types)
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
		if !ContainsRecursive(candidate) {
			out = append(out, candidate)
			continue
		}
		if ptr := typePointer(candidate); ptr != 0 && seenNodes[ptr] {
			changed = true
			continue
		}
		// Distinct recursive handles that denote the same product family (a fresh
		// *Recursive is minted each fixpoint iteration) cannot be detected by
		// pointer identity, so dedup by the coinductive family hash refined with
		// the structural same-family probe.
		h := ProductFamilyHash(candidate)
		duplicate := false
		for _, r := range recReps {
			if r.hash == h && SameProductFamily(r.rep, candidate) {
				duplicate = true
				break
			}
		}
		if duplicate {
			changed = true
			continue
		}
		recReps = append(recReps, familyRep{hash: h, rep: candidate})
		if ptr := typePointer(candidate); ptr != 0 {
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
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !ContainsRecursive(a) && !ContainsRecursive(b) {
		return false
	}
	if s.productFamilyHash(a) != s.productFamilyHash(b) {
		return false
	}
	seen := s.precisionState()
	aStrict, aComparable := comparePrecision(a, b, 0, seen)
	if !aComparable || aStrict {
		return false
	}
	bStrict, bComparable := comparePrecision(b, a, 0, seen)
	return bComparable && !bStrict
}

func (s *returnJoinState) productFamilyHash(t Type) uint64 {
	if s == nil {
		return ProductFamilyHash(t)
	}
	return precisionFamilyHash(t, s.precisionState())
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
	return newReturnJoinState().coalesceProductUnion(t)
}

func (s *returnJoinState) coalesceProductUnion(t Type) Type {
	switch v := UnwrapAnnotated(t).(type) {
	case *Optional:
		inner := s.coalesceProductUnion(v.Inner)
		if SameNodeOrAcyclicEqual(inner, v.Inner) {
			return t
		}
		return NewOptional(inner)
	case *Union:
		if v == nil || len(v.Members) < 2 {
			return t
		}
		members := s.coalesceProductUnionMembers(v.Members)
		if sameTypeSlice(v.Members, members) {
			return t
		}
		return NewUnion(members...)
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
	return newReturnJoinState().coalesceRecursiveRecordFamilies(types)
}

func (s *returnJoinState) coalesceRecursiveRecordFamilies(types []Type) []Type {
	if len(types) < 2 {
		return types
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
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
			joinedBody, ok := state.joinRecursiveFamilyBodies(body, nextBody)
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
		return ProductFamilyHash(rec)
	}
	hash := internal.HashCombine(uint64(kind.Recursive), uint64(kind.Record))
	if body.HasMapComponent() {
		hash = internal.HashCombine(hash, 1)
	} else {
		hash = internal.HashCombine(hash, 0)
	}
	tags := s.requiredDiscriminantTags(body)
	if len(tags) == 0 {
		return hash
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		hash = internal.HashCombine(hash, internal.FnvString(key))
		hash = internal.HashCombine(hash, tags[key])
	}
	return hash
}

func sameRecursiveFamilyBody(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if !ContainsRecursive(a) && !ContainsRecursive(b) {
		return false
	}
	return SameProductFamily(a, b)
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
		return SameProductFamily(a, b)
	}
	return TypeEquals(a, b)
}

func (s *returnJoinState) joinRecursiveFamilyBodies(a, b Type) (Type, bool) {
	if s == nil {
		return newReturnJoinState().joinRecursiveFamilyBodies(a, b)
	}
	previous := s.recursiveFamilyFold
	s.recursiveFamilyFold = true
	defer func() {
		s.recursiveFamilyFold = previous
	}()
	return s.joinCompatibleRecords(a, b)
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

func rewriteRecursiveFamilySelf(t Type, from, to *Recursive) Type {
	return newReturnJoinState().rewriteRecursiveFamilySelf(t, from, to)
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
	ptr := typePointer(t)
	key := recursiveRewriteKey{
		bodyKind: t.Kind(),
		bodyPtr:  ptr,
		fromID:   from.ID,
		toID:     to.ID,
	}
	if ptr == 0 {
		key.bodyHash = typeEqualityHash(t)
	}
	return key, true
}

func (s *returnJoinState) coalesceCompatibleRecordAlternatives(t Type) Type {
	switch v := UnwrapAnnotated(t).(type) {
	case *Optional:
		inner := s.coalesceCompatibleRecordAlternatives(v.Inner)
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
		coalesced := s.coalesceCompatibleRecordTypes(members)
		if sameTypeSlice(members, coalesced) {
			return t
		}
		return NewUnion(coalesced...)
	default:
		return t
	}
}

func (s *returnJoinState) joinCoalescedUnion(a, b Type) Type {
	members := make([]Type, 0, 4)
	members = appendUnionMembers(members, a)
	members = appendUnionMembers(members, b)
	members = s.coalesceProductUnionMembers(members)
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return PruneSoftUnionMembers(members[0])
	}
	return PruneSoftUnionMembers(NewUnion(members...))
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
	if len(types) < 2 {
		return types
	}
	if fast, ok, complete := s.coalesceClosedCompatibleRecords(types); ok {
		if complete {
			return fast
		}
		types = fast
	}
	if fast, ok := s.coalesceCompatibleRecordGroups(types); ok {
		return fast
	}
	return s.coalesceCompatibleRecordsPairwise(types)
}

type compatibleRecordGroup struct {
	indices []int
	records []*Record
	tags    map[string]uint64
	hasMap  bool
}

func (s *returnJoinState) coalesceCompatibleRecordGroups(types []Type) ([]Type, bool) {
	groups := make([]*compatibleRecordGroup, 0, len(types))
	for i, t := range types {
		rec := unaliasRecord(t)
		if rec == nil {
			continue
		}
		tags := s.requiredDiscriminantTags(rec)
		var group *compatibleRecordGroup
		for _, candidate := range groups {
			if candidate.hasMap == rec.HasMapComponent() && closedRecordTagsCompatible(candidate.tags, tags) {
				group = candidate
				break
			}
		}
		if group == nil {
			group = &compatibleRecordGroup{tags: copyDiscriminantTags(tags), hasMap: rec.HasMapComponent()}
			groups = append(groups, group)
		} else {
			group.tags = mergeClosedRecordTags(group.tags, tags)
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
		merged, ok := s.joinCompatibleRecordSet(group.records)
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
	if len(records) == 0 {
		return nil, false
	}
	current := Type(records[0])
	for _, rec := range records[1:] {
		merged, ok := s.joinCompatibleRecords(current, rec)
		if !ok {
			return nil, false
		}
		current = merged
	}
	return current, true
}

func (s *returnJoinState) coalesceCompatibleRecordsPairwise(types []Type) []Type {
	if len(types) < 2 {
		return types
	}
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
			merged, ok := s.joinCompatibleRecords(currentRecord, candidateRecord)
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

type closedRecordGroup struct {
	indices []int
	records []*Record
	tags    map[string]uint64
}

func (s *returnJoinState) coalesceClosedCompatibleRecords(types []Type) ([]Type, bool, bool) {
	groups := make([]*closedRecordGroup, 0, len(types))
	changed := false
	ineligible := false
	eligibleCount := 0
	for i, t := range types {
		rec := unaliasRecord(t)
		if rec == nil {
			ineligible = true
			continue
		}
		if rec.Open || rec.HasMapComponent() || rec.Metatable != nil || ContainsRecursive(rec) {
			ineligible = true
			continue
		}
		eligibleCount++
		tags := s.requiredDiscriminantTags(rec)
		var group *closedRecordGroup
		for _, candidate := range groups {
			if closedRecordTagsCompatible(candidate.tags, tags) {
				group = candidate
				break
			}
		}
		if group == nil {
			group = &closedRecordGroup{tags: copyDiscriminantTags(tags)}
			groups = append(groups, group)
		} else {
			changed = true
			group.tags = mergeClosedRecordTags(group.tags, tags)
		}
		group.indices = append(group.indices, i)
		group.records = append(group.records, rec)
	}
	if eligibleCount == 0 {
		return nil, false, false
	}
	if !changed {
		if !ineligible {
			return types, true, true
		}
		return nil, false, false
	}

	mergedAt := make(map[int]Type)
	skip := make(map[int]bool)
	for _, group := range groups {
		if len(group.records) == 1 {
			continue
		}
		merged, ok := s.joinClosedCompatibleRecordSet(group.records)
		if !ok {
			return nil, false, false
		}
		mergedAt[group.indices[0]] = merged
		for _, idx := range group.indices[1:] {
			skip[idx] = true
		}
	}
	out := make([]Type, 0, len(types))
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
	return out, true, !ineligible
}

func closedRecordTagsCompatible(a, b map[string]uint64) bool {
	for name, left := range a {
		if right, ok := b[name]; ok && left != right {
			return false
		}
	}
	return true
}

func mergeClosedRecordTags(dst, src map[string]uint64) map[string]uint64 {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]uint64, len(src))
	}
	for name, hash := range src {
		dst[name] = hash
	}
	return dst
}

func (s *returnJoinState) closedRecordSetHasConflictingRequiredLiteralField(records []*Record) bool {
	hasTags := false
	for _, rec := range records {
		if s.hasRequiredDiscriminantTag(rec) {
			hasTags = true
			break
		}
	}
	if !hasTags {
		return false
	}

	seen := make(map[string]uint64)
	for _, rec := range records {
		for path, hash := range s.requiredDiscriminantTags(rec) {
			if existing, ok := seen[path]; ok && existing != hash {
				return true
			}
			seen[path] = hash
		}
	}
	return false
}

func closedRecordSetHasConflictingRequiredLiteralField(records []*Record) bool {
	return newReturnJoinState().closedRecordSetHasConflictingRequiredLiteralField(records)
}

func (s *returnJoinState) joinCompatibleRecords(a, b Type) (Type, bool) {
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar == nil || br == nil {
		return nil, false
	}
	if s.sameReturnJoinInput(ar, br) {
		return ar, true
	}
	key := s.joinKey(ar, br)
	if s != nil && s.records != nil {
		if cached, ok := s.records[key]; ok {
			return cached.t, cached.ok
		}
	}

	// Keep discriminated unions intact when required literal tags conflict.
	if s.hasConflictingRequiredLiteralField(ar, br) {
		if s != nil {
			s.cacheRecordJoin(key, nil, false)
		}
		return nil, false
	}

	// Mixing map and non-map record slots can be semantically distinct.
	if ar.HasMapComponent() != br.HasMapComponent() {
		if s != nil {
			s.cacheRecordJoin(key, nil, false)
		}
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
			fields = append(fields, s.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, Field{}, false, ar, br))
			i++
		case i >= len(ar.Fields):
			fields = append(fields, s.mergeRecordField(br.Fields[j].Name, Field{}, false, br.Fields[j], true, ar, br))
			j++
		case ar.Fields[i].Name == br.Fields[j].Name:
			fields = append(fields, s.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, br.Fields[j], true, ar, br))
			i++
			j++
		case ar.Fields[i].Name < br.Fields[j].Name:
			fields = append(fields, s.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, Field{}, false, ar, br))
			i++
		default:
			fields = append(fields, s.mergeRecordField(br.Fields[j].Name, Field{}, false, br.Fields[j], true, ar, br))
			j++
		}
	}

	merged := buildRecordType(fields, metatable, mapKey, mapValue, open, true, false)
	if s != nil {
		s.cacheRecordJoin(key, merged, true)
	}
	return merged, true
}

func (s *returnJoinState) cacheRecordJoin(key returnJoinKey, t Type, ok bool) {
	if s.records == nil {
		s.records = make(map[returnJoinKey]recordJoinResult)
	}
	s.records[key] = recordJoinResult{t: t, ok: ok}
}

func (s *returnJoinState) mergeRecordField(name string, fa Field, oka bool, fb Field, okb bool, ar, br *Record) Field {
	fieldType := Type(nil)
	optional := true
	readonly := false
	switch {
	case oka && okb:
		// Record coalescing is used from JoinReturnSlot; keep field-level merge
		// on the same return-slot policy so empty-collection paths and nil/unknown
		// interactions are handled consistently in nested return records.
		fieldType = s.joinRecordFieldSlot(name, fa.Type, fb.Type)
		optional = fa.Optional || fb.Optional
		readonly = fa.Readonly && fb.Readonly
	case oka:
		fieldType = fa.Type
		optional = true
		readonly = fa.Readonly
		if tail, ok := recordTailFieldType(br, name); ok {
			fieldType, optional = normalizeMergedRecordField(s.joinReturnSlot(fa.Type, tail))
			if recordMapTailMayContain(br, name) {
				optional = true
			}
			readonly = false
		}
	case okb:
		fieldType = fb.Type
		optional = true
		readonly = fb.Readonly
		if tail, ok := recordTailFieldType(ar, name); ok {
			fieldType, optional = normalizeMergedRecordField(s.joinReturnSlot(tail, fb.Type))
			if recordMapTailMayContain(ar, name) {
				optional = true
			}
			readonly = false
		}
	}
	return Field{Name: name, Type: fieldType, Optional: optional, Readonly: readonly}
}

func (s *returnJoinState) joinRecordFieldSlot(name string, a, b Type) Type {
	if joined, ok := s.joinFieldContainerSlot(a, b); ok {
		return joined
	}
	if !IsDiscriminantLiteralField(name) {
		if widened, ok := joinNonDiscriminantLiteralField(a, b); ok {
			return widened
		}
	}
	return s.joinReturnSlot(a, b)
}

func (s *returnJoinState) joinFieldContainerSlot(a, b Type) (Type, bool) {
	a = UnwrapAnnotated(a)
	b = UnwrapAnnotated(b)
	switch av := a.(type) {
	case *Array:
		bv, ok := b.(*Array)
		if !ok {
			return nil, false
		}
		return NewArray(s.joinReturnSlot(av.Element, bv.Element)), true
	case *Map:
		bv, ok := b.(*Map)
		if !ok {
			return nil, false
		}
		return NewMap(JoinPreferNonSoft(av.Key, bv.Key), s.joinReturnSlot(av.Value, bv.Value)), true
	case *Tuple:
		bv, ok := b.(*Tuple)
		if !ok || len(av.Elements) != len(bv.Elements) {
			return nil, false
		}
		elements := make([]Type, len(av.Elements))
		for i := range av.Elements {
			elements[i] = s.joinReturnSlot(av.Elements[i], bv.Elements[i])
		}
		return NewTuple(elements...), true
	default:
		return nil, false
	}
}

// JoinRecordFieldSlot merges observations for one named record field. Ordinary
// literal fields widen to their scalar base, while discriminant fields preserve
// literal alternatives for path-sensitive narrowing.
func JoinRecordFieldSlot(name string, a, b Type) Type {
	return newReturnJoinState().joinRecordFieldSlot(name, a, b)
}

func joinNonDiscriminantLiteralField(a, b Type) (Type, bool) {
	al, aOK := literalType(a)
	bl, bOK := literalType(b)
	if aOK && bOK && al.Base == bl.Base {
		if LiteralEquals(al, bl) {
			return a, true
		}
		return literalBase(al), true
	}
	return commonLiteralFamilyBase(a, b)
}

func commonLiteralFamilyBase(a, b Type) (Type, bool) {
	left, ok := literalFamilyBase(a)
	if !ok {
		return nil, false
	}
	right, ok := literalFamilyBase(b)
	if !ok {
		return nil, false
	}
	return mergeLiteralBases(left, right)
}

func literalFamilyBase(t Type) (Type, bool) {
	t = UnwrapAnnotated(t)
	if t == nil {
		return nil, false
	}
	switch v := t.(type) {
	case *Alias:
		return literalFamilyBase(v.Target)
	case *Literal:
		base := literalBase(v)
		return base, base != nil
	case *Union:
		var base Type
		for _, member := range v.Members {
			memberBase, ok := literalFamilyBase(member)
			if !ok {
				return nil, false
			}
			if base == nil {
				base = memberBase
				continue
			}
			merged, ok := mergeLiteralBases(base, memberBase)
			if !ok {
				return nil, false
			}
			base = merged
		}
		return base, base != nil
	default:
		switch t.Kind() {
		case kind.Boolean, kind.Integer, kind.Number, kind.String:
			return t, true
		default:
			return nil, false
		}
	}
}

func mergeLiteralBases(a, b Type) (Type, bool) {
	if a == nil || b == nil {
		return nil, false
	}
	if SameNodeOrAcyclicEqual(a, b) {
		return a, true
	}
	if (a.Kind() == kind.Integer && b.Kind() == kind.Number) ||
		(a.Kind() == kind.Number && b.Kind() == kind.Integer) {
		return Number, true
	}
	return nil, false
}

func literalBase(lit *Literal) Type {
	if lit == nil {
		return nil
	}
	switch lit.Base {
	case kind.Boolean:
		return Boolean
	case kind.Integer:
		return Integer
	case kind.Number:
		return Number
	case kind.String:
		return String
	default:
		return nil
	}
}

func normalizeMergedRecordField(t Type) (Type, bool) {
	if inner, optional := SplitNilableFieldType(t); optional {
		return inner, true
	}
	return t, false
}

func recordTailFieldType(r *Record, name string) (Type, bool) {
	if r == nil {
		return nil, false
	}
	if r.HasMapComponent() && mapComponentMayContainStringKey(r.MapKey, name) {
		return NewOptional(r.MapValue), true
	}
	if r.Open {
		return Unknown, true
	}
	return nil, false
}

func recordMapTailMayContain(r *Record, name string) bool {
	return r != nil && r.HasMapComponent() && mapComponentMayContainStringKey(r.MapKey, name)
}

func mapComponentMayContainStringKey(key Type, name string) bool {
	if key == nil {
		return false
	}
	if IsAny(key) || IsUnknown(key) {
		return true
	}
	switch k := key.(type) {
	case *Alias:
		return mapComponentMayContainStringKey(k.Target, name)
	case *Union:
		for _, member := range k.Members {
			if mapComponentMayContainStringKey(member, name) {
				return true
			}
		}
		return false
	case *Literal:
		return k.Base == kind.String && k.Value == name
	default:
		return k.Kind() == kind.String
	}
}

func unaliasRecord(t Type) *Record {
	for {
		a, ok := t.(*Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	rec, _ := t.(*Record)
	return rec
}

func unaliasUnion(t Type) *Union {
	for {
		a, ok := t.(*Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	u, _ := t.(*Union)
	return u
}

func (s *returnJoinState) coalesceCompatibleRecordMembers(t Type) Type {
	u := unaliasUnion(t)
	if u == nil || len(u.Members) < 2 {
		return t
	}

	members := make([]Type, len(u.Members))
	copy(members, u.Members)
	coalesced := s.coalesceCompatibleRecordTypes(members)
	if sameTypeSlice(members, coalesced) {
		return t
	}
	return NewUnion(coalesced...)
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

func (s *returnJoinState) hasConflictingRequiredLiteralField(a, b *Record) bool {
	left := s.requiredDiscriminantTags(a)
	right := s.requiredDiscriminantTags(b)
	for path, leftHash := range left {
		if rightHash, ok := right[path]; ok && leftHash != rightHash {
			return true
		}
	}
	return false
}

func (s *returnJoinState) hasRequiredDiscriminantTag(t Type) bool {
	return len(s.requiredDiscriminantTags(t)) > 0
}

// IsDiscriminantLiteralField reports whether a field name conventionally acts
// as a variant tag. Literal values for these fields are kept precise so
// path-sensitive narrowing can discriminate record unions.
func IsDiscriminantLiteralField(name string) bool {
	switch name {
	case "__tag", "type", "kind", "tag", "role", "variant", "success", "ok":
		return true
	default:
		return false
	}
}

func requiredDiscriminantTags(t Type) map[string]uint64 {
	return newReturnJoinState().requiredDiscriminantTags(t)
}

func (s *returnJoinState) requiredDiscriminantTags(t Type) map[string]uint64 {
	t = UnwrapAnnotated(t)
	if t == nil {
		return nil
	}
	if s == nil {
		s = newReturnJoinState()
	}
	if s.discriminants != nil {
		if cached, ok := s.discriminants[t]; ok {
			return copyDiscriminantTags(cached)
		}
	}
	if s.activeDiscriminants != nil && s.activeDiscriminants[t] {
		return nil
	}
	if s.activeDiscriminants == nil {
		s.activeDiscriminants = make(map[Type]bool)
	}
	s.activeDiscriminants[t] = true
	defer delete(s.activeDiscriminants, t)

	tags := s.collectRequiredDiscriminantTags(t)
	if s.discriminants == nil {
		s.discriminants = make(map[Type]map[string]uint64)
	}
	s.discriminants[t] = copyDiscriminantTags(tags)
	return tags
}

func (s *returnJoinState) collectRequiredDiscriminantTags(t Type) map[string]uint64 {
	t = UnwrapAnnotated(t)
	switch v := t.(type) {
	case *Alias:
		return s.requiredDiscriminantTags(v.Target)
	case *Recursive:
		return s.requiredDiscriminantTags(v.Body)
	case *Record:
		tags := make(map[string]uint64)
		for _, field := range v.Fields {
			if field.Optional {
				continue
			}
			if lit, ok := literalType(field.Type); ok && IsDiscriminantLiteralField(field.Name) {
				tags[field.Name] = lit.Hash()
				continue
			}
			addPrefixedDiscriminantTags(tags, field.Name, s.requiredDiscriminantTags(field.Type))
		}
		return tags
	case *Union:
		return s.commonUnionDiscriminantTags(v)
	}
	return nil
}

func (s *returnJoinState) commonUnionDiscriminantTags(u *Union) map[string]uint64 {
	if u == nil || len(u.Members) == 0 {
		return nil
	}
	var common map[string]uint64
	for i, member := range u.Members {
		memberTags := s.requiredDiscriminantTags(member)
		if i == 0 {
			common = copyDiscriminantTags(memberTags)
			continue
		}
		for path, hash := range common {
			if memberHash, ok := memberTags[path]; !ok || memberHash != hash {
				delete(common, path)
			}
		}
	}
	return common
}

func addPrefixedDiscriminantTags(dst map[string]uint64, prefix string, src map[string]uint64) {
	for path, hash := range src {
		dst[joinDiscriminantPath(prefix, path)] = hash
	}
}

func copyDiscriminantTags(src map[string]uint64) map[string]uint64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]uint64, len(src))
	for path, hash := range src {
		dst[path] = hash
	}
	return dst
}

func joinDiscriminantPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func literalType(t Type) (*Literal, bool) {
	for {
		a, ok := t.(*Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	lit, ok := t.(*Literal)
	return lit, ok
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
	return NewUnion(a, b)
}

// IsRefinableAnnotation reports whether an explicit annotation should be
// treated as a soft placeholder that call-site/contextual hints may refine.
//
// Canonical rule: explicit top types (`any`, `unknown`) are authoritative and
// must not be rewritten by hints. Structural soft placeholders like `{any}` or
// `any[]` remain refinable.
func IsRefinableAnnotation(t Type) bool {
	if t == nil {
		return false
	}
	if t.Kind().IsPlaceholder() {
		return false
	}
	return isRefinableStructuralAnnotation(t, NewGuard())
}

// IsClosedUnionAnnotation reports whether a declared annotation is a multi-member
// union whose members are all concrete (no placeholder/any/unknown at the top
// level of any member). Such an annotation carries a closed discriminant domain
// that flow narrowing must preserve at the variable's root even if the union
// has refinable slots deep inside member fields.
func IsClosedUnionAnnotation(t Type) bool {
	if t == nil {
		return false
	}
	u := closedUnionOf(t)
	if u == nil || len(u.Members) < 2 {
		return false
	}
	for _, member := range u.Members {
		if member == nil {
			return false
		}
		if IsAbsentOrUnknown(member) || member.Kind().IsPlaceholder() {
			return false
		}
	}
	return true
}

// closedUnionOf unwraps Annotated/Alias/Optional layers to expose an underlying
// Union, mirroring the unwrap.Union helper without the import.
func closedUnionOf(t Type) *Union {
	for {
		switch v := UnwrapAnnotated(t).(type) {
		case *Union:
			return v
		case *Alias:
			t = v.Target
		case *Optional:
			t = v.Inner
		default:
			return nil
		}
	}
}

func isRefinableStructuralAnnotation(t Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}
	switch v := t.(type) {
	case *Alias:
		return isRefinableStructuralAnnotation(v.UnaliasedTarget(), next)
	case *Optional:
		return annotationSlotRefinable(v.Inner, next)
	case *Array:
		return annotationSlotRefinable(v.Element, next)
	case *Map:
		return annotationSlotRefinable(v.Key, next) || annotationSlotRefinable(v.Value, next)
	case *Record:
		if v.Open && len(v.Fields) == 0 && !v.HasMapComponent() {
			return true
		}
		if v.HasMapComponent() &&
			(annotationSlotRefinable(v.MapKey, next) || annotationSlotRefinable(v.MapValue, next)) {
			return true
		}
		for _, field := range v.Fields {
			if annotationSlotRefinable(field.Type, next) {
				return true
			}
		}
		return false
	case *Tuple:
		for _, elem := range v.Elements {
			if annotationSlotRefinable(elem, next) {
				return true
			}
		}
		return false
	case *Union:
		for _, member := range v.Members {
			if annotationSlotRefinable(member, next) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func annotationSlotRefinable(t Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t.Kind().IsPlaceholder() {
		return true
	}
	return isRefinableStructuralAnnotation(t, guard)
}
