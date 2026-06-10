package typ

import (
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/internal/recursion"
	"github.com/wippyai/go-lua/analysis/type/kind"
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

type staticMemberJoinKey struct {
	kind  StaticMemberKind
	name  string
	index int64
}

type staticMemberAcc struct {
	memberType Type
	count      int
	optional   bool
	readonly   bool
	kind       StaticMemberKind
	name       string
	index      int64
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
			return hash.HashCombine(uint64(t.Kind()), uint64(ptr))
		}
		return productFamilyHash(t)
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
		return sameProductFamily(a, b)
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
		return len(v.Fields) == 0 && len(v.StaticMembers) == 0 && !v.HasMapComponent()
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
	staticMembers := make(map[staticMemberJoinKey]*staticMemberAcc)
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
			acc.fieldType = state.joinRecordFieldSlot(acc.fieldType, field.Type)
			acc.count++
			acc.optional = acc.optional || field.Optional
			acc.readonly = acc.readonly && field.Readonly
		}
		for _, member := range rec.StaticMembers {
			key := staticMemberKey(member)
			acc := staticMembers[key]
			if acc == nil {
				staticMembers[key] = &staticMemberAcc{
					memberType: member.Type,
					count:      1,
					optional:   member.Optional,
					readonly:   member.Readonly,
					kind:       member.Kind,
					name:       member.Name,
					index:      member.Index,
				}
				continue
			}
			acc.memberType = state.joinRecordFieldSlot(acc.memberType, member.Type)
			acc.count++
			acc.optional = acc.optional || member.Optional
			acc.readonly = acc.readonly && member.Readonly
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
	mergedStaticMembers := make([]StaticMember, 0, len(staticMembers))
	for _, acc := range staticMembers {
		mergedStaticMembers = append(mergedStaticMembers, StaticMember{
			Kind:     acc.kind,
			Name:     acc.name,
			Index:    acc.index,
			Type:     acc.memberType,
			Optional: acc.optional || acc.count < len(records),
			Readonly: acc.readonly,
		})
	}
	return RebuildRecord(RecordParts{
		Fields:        mergedFields,
		StaticMembers: mergedStaticMembers,
	}), true
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
		if ptr := typePointer(rec); ptr != 0 && seenNodes[ptr] {
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
		if ptr := typePointer(rec); ptr != 0 {
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
		return productFamilyHash(t)
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
		return productFamilyHash(rec)
	}
	h := hash.HashCombine(uint64(kind.Recursive), uint64(kind.Record))
	if body.HasMapComponent() {
		h = hash.HashCombine(h, 1)
	} else {
		h = hash.HashCombine(h, 0)
	}
	tags := s.requiredDiscriminantTags(body)
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
	hasMap  bool
}

func (s *returnJoinState) coalesceCompatibleRecordGroups(types []Type) ([]Type, bool) {
	groups := make([]*compatibleRecordGroup, 0, len(types))
	for i, t := range types {
		rec := unaliasRecord(t)
		if rec == nil {
			continue
		}
		var group *compatibleRecordGroup
		for _, candidate := range groups {
			if candidate.hasMap == rec.HasMapComponent() && s.recordMergesIntoGroup(rec, candidate.records) {
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
		var group *closedRecordGroup
		for _, candidate := range groups {
			if s.recordMergesIntoGroup(rec, candidate.records) {
				group = candidate
				break
			}
		}
		if group == nil {
			group = &closedRecordGroup{}
			groups = append(groups, group)
		} else {
			changed = true
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

	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			if s.hasConflictingRequiredLiteralField(records[i], records[j]) {
				return true
			}
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
	if !compatibleRecordMetatables(ar, br) {
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

	staticMembers := s.mergeRecordStaticMembers(ar, br)
	merged := RebuildRecord(RecordParts{
		Fields:        fields,
		StaticMembers: staticMembers,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          open,
		AssumeSorted:  true,
	})
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
		fieldType = s.joinRecordFieldSlot(fa.Type, fb.Type)
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

func (s *returnJoinState) mergeRecordStaticMembers(ar, br *Record) []StaticMember {
	staticMembers := make([]StaticMember, 0, len(ar.StaticMembers)+len(br.StaticMembers))
	i, j := 0, 0
	for i < len(ar.StaticMembers) || j < len(br.StaticMembers) {
		switch {
		case j >= len(br.StaticMembers):
			staticMembers = append(staticMembers, s.mergeRecordStaticMember(ar.StaticMembers[i], true, StaticMember{}, false, ar, br))
			i++
		case i >= len(ar.StaticMembers):
			staticMembers = append(staticMembers, s.mergeRecordStaticMember(StaticMember{}, false, br.StaticMembers[j], true, ar, br))
			j++
		default:
			cmp := CompareStaticMembers(ar.StaticMembers[i], br.StaticMembers[j])
			switch {
			case cmp == 0:
				staticMembers = append(staticMembers, s.mergeRecordStaticMember(ar.StaticMembers[i], true, br.StaticMembers[j], true, ar, br))
				i++
				j++
			case cmp < 0:
				staticMembers = append(staticMembers, s.mergeRecordStaticMember(ar.StaticMembers[i], true, StaticMember{}, false, ar, br))
				i++
			default:
				staticMembers = append(staticMembers, s.mergeRecordStaticMember(StaticMember{}, false, br.StaticMembers[j], true, ar, br))
				j++
			}
		}
	}
	return staticMembers
}

func (s *returnJoinState) mergeRecordStaticMember(ma StaticMember, oka bool, mb StaticMember, okb bool, ar, br *Record) StaticMember {
	member := ma
	memberType := Type(nil)
	optional := true
	readonly := false
	switch {
	case oka && okb:
		memberType = s.joinRecordFieldSlot(ma.Type, mb.Type)
		optional = ma.Optional || mb.Optional
		readonly = ma.Readonly && mb.Readonly
	case oka:
		memberType = ma.Type
		optional = true
		readonly = ma.Readonly
		if tail, ok := recordTailStaticMemberType(br, ma); ok {
			memberType, optional = normalizeMergedRecordField(s.joinReturnSlot(ma.Type, tail))
			if recordMapTailMayContainStaticMember(br, ma) {
				optional = true
			}
			readonly = false
		}
	case okb:
		member = mb
		memberType = mb.Type
		optional = true
		readonly = mb.Readonly
		if tail, ok := recordTailStaticMemberType(ar, mb); ok {
			memberType, optional = normalizeMergedRecordField(s.joinReturnSlot(tail, mb.Type))
			if recordMapTailMayContainStaticMember(ar, mb) {
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

func (s *returnJoinState) joinRecordFieldSlot(a, b Type) Type {
	if joined, ok := s.joinFieldContainerSlot(a, b); ok {
		return joined
	}
	// Records reach a field-slot merge only after the discriminant gate admits
	// their coalesce, so no surviving field is a partitioning tag: equal literals
	// stay precise and differing literals widen to their shared base.
	if widened, ok := joinNonDiscriminantLiteralField(a, b); ok {
		return widened
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
	case *ReadonlyMap:
		bv, ok := b.(*ReadonlyMap)
		if !ok {
			return nil, false
		}
		return NewReadonlyMap(JoinPreferNonSoft(av.Key, bv.Key), s.joinReturnSlot(av.Value, bv.Value)), true
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

// JoinUnionFieldSlot merges the per-member results of reading one field across a
// union. When preserveLiteral is set the field is the union's structural
// discriminant, so distinct literal alternatives are kept for path-sensitive
// narrowing; otherwise ordinary literal data fields widen to their scalar base so
// many-member unions do not explode into giant literal unions on read. The caller
// owns the discriminant decision because it alone holds the union context.
func JoinUnionFieldSlot(a, b Type, preserveLiteral bool) Type {
	s := newReturnJoinState()
	if preserveLiteral {
		if joined, ok := s.joinFieldContainerSlot(a, b); ok {
			return joined
		}
		return s.joinReturnSlot(a, b)
	}
	return s.joinRecordFieldSlot(a, b)
}

func joinNonDiscriminantLiteralField(a, b Type) (Type, bool) {
	// Join is idempotent: equal literal-bearing slots (single literals or whole
	// literal unions) keep their precise type rather than widening to the base.
	if SameNodeOrAcyclicEqual(a, b) {
		return a, true
	}
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

func recordTailStaticMemberType(r *Record, member StaticMember) (Type, bool) {
	if r == nil {
		return nil, false
	}
	if r.HasMapComponent() && mapComponentMayContainStaticMemberKey(r.MapKey, member) {
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

func recordMapTailMayContainStaticMember(r *Record, member StaticMember) bool {
	return r != nil && r.HasMapComponent() && mapComponentMayContainStaticMemberKey(r.MapKey, member)
}

func mapComponentMayContainStaticMemberKey(key Type, member StaticMember) bool {
	switch member.Kind {
	case StaticMemberStringIndex:
		return mapComponentMayContainStringKey(key, member.Name)
	case StaticMemberIntIndex:
		return mapComponentMayContainIntKey(key, member.Index)
	default:
		return false
	}
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

func mapComponentMayContainIntKey(key Type, index int64) bool {
	if key == nil {
		return false
	}
	if IsAny(key) || IsUnknown(key) {
		return true
	}
	switch k := key.(type) {
	case *Alias:
		return mapComponentMayContainIntKey(k.Target, index)
	case *Union:
		for _, member := range k.Members {
			if mapComponentMayContainIntKey(member, index) {
				return true
			}
		}
		return false
	case *Literal:
		switch k.Base {
		case kind.Integer:
			return k.Value == index
		case kind.Number:
			number, ok := k.Value.(float64)
			return ok && number == float64(index)
		default:
			return false
		}
	default:
		return k.Kind() == kind.Integer || k.Kind() == kind.Number
	}
}

func staticMemberKey(member StaticMember) staticMemberJoinKey {
	return staticMemberJoinKey{kind: member.Kind, name: member.Name, index: member.Index}
}

func staticMemberDiscriminantPath(member StaticMember) string {
	switch member.Kind {
	case StaticMemberStringIndex:
		return "[" + strconv.Quote(member.Name) + "]"
	case StaticMemberIntIndex:
		return "[" + strconv.FormatInt(member.Index, 10) + "]"
	default:
		return "[]"
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

// hasConflictingRequiredLiteralField reports whether two records are discriminated
// variants the join keeps distinct rather than coalescing.
//
// A required literal field shared by both records is either a variant tag or
// incidental literal data. The structural signal of a tag is that it is the single
// literal axis on which the records disagree: exactly one shared required literal
// field has differing values, and no shared required literal field has an equal
// value acting as a constant data key. When the records disagree on several literal
// fields, or share an equal-valued literal alongside the difference, the literals
// are incidental data and the records coalesce (the differing literals widen, the
// equal one stays). Records whose literal-erased residuals do not merge cleanly
// (conflicting shared non-literal field, or mutually disjoint required payloads)
// also stay distinct, since merging them would lose structure rather than widen a
// scalar.
func (s *returnJoinState) hasConflictingRequiredLiteralField(a, b *Record) bool {
	differing, equal := s.sharedRequiredLiteralAxes(a, b)
	if differing == 1 && equal == 0 {
		return true
	}
	if differing == 0 {
		return false
	}
	return !literalErasedResidualsCleanlyMergeable(a, b)
}

// RecordsConflictOnLiteralDiscriminant reports whether two records are discriminated
// variants kept distinct by a shared required literal field rather than coalesced.
// It is the structural, name-free discriminant test shared by the return-slot join
// and the value-domain shape join.
func RecordsConflictOnLiteralDiscriminant(a, b *Record) bool {
	return newReturnJoinState().hasConflictingRequiredLiteralField(a, b)
}

// sharedRequiredLiteralAxes counts the required literal fields both records require,
// split into those whose literal values differ and those whose values are equal.
func (s *returnJoinState) sharedRequiredLiteralAxes(a, b *Record) (differing, equal int) {
	left := s.requiredDiscriminantTags(a)
	right := s.requiredDiscriminantTags(b)
	for path, leftHash := range left {
		rightHash, ok := right[path]
		if !ok {
			continue
		}
		if leftHash == rightHash {
			equal++
		} else {
			differing++
		}
	}
	return differing, equal
}

func (s *returnJoinState) hasRequiredDiscriminantTag(t Type) bool {
	return len(s.requiredDiscriminantTags(t)) > 0
}

// recordMergesIntoGroup reports whether rec coalesces with every record already
// grouped together rather than forming a discriminated variant against any of
// them. A record joins a group only when no member of the group conflicts with it
// on a genuine literal discriminant.
func (s *returnJoinState) recordMergesIntoGroup(rec *Record, group []*Record) bool {
	for _, member := range group {
		if !compatibleRecordMetatables(rec, member) {
			return false
		}
		if s.hasConflictingRequiredLiteralField(rec, member) {
			return false
		}
	}
	return true
}

func compatibleRecordMetatables(a, b *Record) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Metatable == nil || b.Metatable == nil {
		return a.Metatable == nil && b.Metatable == nil
	}
	return SameNodeOrAcyclicEqual(a.Metatable, b.Metatable)
}

// literalErasedResidualsCleanlyMergeable reports whether two records merge into a
// single precise record once their literal-valued fields are erased. Bodies merge
// cleanly when every required non-literal field shared by both has compatible
// types (equal or ordered by subtyping, so the merge keeps one precise type) and
// neither carries a required non-literal field the other lacks (no disjoint
// payload). When merging would instead widen a shared required field to a union or
// optionalize a disjoint required payload, the residuals are not cleanly mergeable
// and the records form discriminated variants.
func literalErasedResidualsCleanlyMergeable(a, b *Record) bool {
	if a == nil || b == nil {
		return false
	}
	// Disjoint payload: each record requires a non-literal field the other lacks
	// entirely. A field missing from only one side is additive width that
	// optionalizes on merge; mutual exclusion is what partitions variants.
	if requiredNonLiteralPayloadMissingFrom(a, b) && requiredNonLiteralPayloadMissingFrom(b, a) {
		return false
	}
	for _, field := range a.Fields {
		if field.Optional {
			continue
		}
		if _, ok := literalType(field.Type); ok {
			continue
		}
		other := b.GetField(field.Name)
		if other == nil || other.Optional {
			continue
		}
		if _, ok := literalType(other.Type); ok {
			continue
		}
		if !mergeKeepsPreciseFieldType(field.Type, other.Type) {
			return false
		}
	}
	for _, member := range a.StaticMembers {
		if member.Optional {
			continue
		}
		if _, ok := literalType(member.Type); ok {
			continue
		}
		other := b.GetStaticMember(member.Kind, member.Name, member.Index)
		if other == nil || other.Optional {
			continue
		}
		if _, ok := literalType(other.Type); ok {
			continue
		}
		if !mergeKeepsPreciseFieldType(member.Type, other.Type) {
			return false
		}
	}
	return true
}

// requiredNonLiteralPayloadMissingFrom reports whether src carries a required
// non-literal field that dst lacks entirely. A field present (even optionally) in
// dst is additive width that optionalizes on merge, not a disjoint payload.
func requiredNonLiteralPayloadMissingFrom(src, dst *Record) bool {
	for _, field := range src.Fields {
		if field.Optional {
			continue
		}
		if _, ok := literalType(field.Type); ok {
			continue
		}
		if dst.GetField(field.Name) == nil {
			return true
		}
	}
	for _, member := range src.StaticMembers {
		if member.Optional {
			continue
		}
		if _, ok := literalType(member.Type); ok {
			continue
		}
		if dst.GetStaticMember(member.Kind, member.Name, member.Index) == nil {
			return true
		}
	}
	return false
}

// mergeKeepsPreciseFieldType reports whether joining two field types stays within a
// single type rather than widening to a cross-kind union. Equal types and types of
// the same outer kind merge precisely; differing kinds (number vs string, record vs
// scalar) widen, which marks the enclosing literal field as a real partition.
func mergeKeepsPreciseFieldType(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if fieldMergeKind(a) != fieldMergeKind(b) {
		return false
	}
	// Two record-valued fields merge precisely only when they are not themselves
	// discriminated variants, so a nested literal tag (channel.__tag) still
	// partitions the enclosing records.
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar != nil && br != nil {
		return literalErasedResidualsCleanlyMergeable(ar, br) && !nestedRequiredLiteralConflict(ar, br)
	}
	return true
}

// nestedRequiredLiteralConflict reports whether two nested record-valued fields
// are separated by a genuine literal discriminant. It uses the same structural
// signal as top-level record coalescing: a lone differing required literal axis
// partitions variants, while an equal literal axis alongside the difference marks
// incidental payload data that can widen under the field-slot join.
func nestedRequiredLiteralConflict(a, b *Record) bool {
	return newReturnJoinState().hasConflictingRequiredLiteralField(a, b)
}

// fieldMergeKind reduces a field type to the outer kind that governs whether two
// fields merge precisely, normalizing a literal to its scalar base so a literal and
// its base count as the same kind.
func fieldMergeKind(t Type) kind.Kind {
	t = UnwrapAnnotated(t)
	for {
		a, ok := t.(*Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	if lit, ok := t.(*Literal); ok {
		return lit.Base
	}
	if t == nil {
		return kind.Nil
	}
	return t.Kind()
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
			if lit, ok := literalType(field.Type); ok {
				tags[field.Name] = lit.Hash()
				continue
			}
			addPrefixedDiscriminantTags(tags, field.Name, s.requiredDiscriminantTags(field.Type))
		}
		for _, member := range v.StaticMembers {
			if member.Optional {
				continue
			}
			path := staticMemberDiscriminantPath(member)
			if lit, ok := literalType(member.Type); ok {
				tags[path] = lit.Hash()
				continue
			}
			addPrefixedDiscriminantTags(tags, path, s.requiredDiscriminantTags(member.Type))
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
		case *Instantiated:
			t = expandClosedUnionInstantiatedBody(v)
		default:
			return nil
		}
	}
}

func expandClosedUnionInstantiatedBody(inst *Instantiated) Type {
	if inst == nil || inst.Generic == nil || inst.Generic.Body == nil ||
		len(inst.Generic.TypeParams) != len(inst.TypeArgs) {
		return inst
	}
	params := inst.Generic.TypeParams
	args := inst.TypeArgs
	return Rewrite(inst.Generic.Body, func(node Type) (Type, bool) {
		tp, ok := node.(*TypeParam)
		if !ok {
			return nil, false
		}
		for i, param := range params {
			if param == nil || args[i] == nil {
				continue
			}
			if tp == param || tp.Equals(param) {
				return args[i], true
			}
		}
		return nil, false
	})
}

func isRefinableStructuralAnnotation(t Type, guard recursion.Guard) bool {
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
	case *ReadonlyMap:
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

func annotationSlotRefinable(t Type, guard recursion.Guard) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t.Kind().IsPlaceholder() {
		return true
	}
	return isRefinableStructuralAnnotation(t, guard)
}
