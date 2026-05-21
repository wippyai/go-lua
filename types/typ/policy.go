package typ

import "github.com/wippyai/go-lua/types/kind"

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
	if TypeEquals(a, b) {
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

type returnJoinState struct {
	returnSlots map[returnJoinKey]Type
	records     map[returnJoinKey]recordJoinResult
}

func newReturnJoinState() *returnJoinState {
	return &returnJoinState{}
}

func joinKey(a, b Type) returnJoinKey {
	if a == nil || b == nil {
		return returnJoinKey{}
	}
	ah, bh := a.Hash(), b.Hash()
	ak, bk := a.Kind(), b.Kind()
	if ah > bh || (ah == bh && ak > bk) {
		ah, bh = bh, ah
		ak, bk = bk, ak
	}
	return returnJoinKey{aHash: ah, bHash: bh, aKind: ak, bKind: bk}
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
	if a == b || (a.Hash() == b.Hash() && TypeEquals(a, b)) {
		return a
	}
	key := joinKey(a, b)
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
	} else if IsUnknown(a) || IsUnknown(b) {
		result = Unknown
	} else {
		result = s.coalesceCompatibleRecordMembers(JoinPreferNonSoft(a, b))
	}
	if s != nil {
		if s.returnSlots == nil {
			s.returnSlots = make(map[returnJoinKey]Type)
		}
		s.returnSlots[key] = result
	}
	return result
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

func (s *returnJoinState) joinCompatibleRecords(a, b Type) (Type, bool) {
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar == nil || br == nil {
		return nil, false
	}
	if ar == br || (ar.Hash() == br.Hash() && TypeEquals(ar, br)) {
		return ar, true
	}
	key := joinKey(ar, br)
	if s != nil && s.records != nil {
		if cached, ok := s.records[key]; ok {
			return cached.t, cached.ok
		}
	}

	// Keep discriminated unions intact when required literal tags conflict.
	if hasConflictingRequiredLiteralField(ar, br) {
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
	if ar.Metatable != nil && br.Metatable != nil && TypeEquals(ar.Metatable, br.Metatable) {
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

	merged := buildRecordType(fields, metatable, mapKey, mapValue, open, true)
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
		fieldType = s.joinReturnSlot(fa.Type, fb.Type)
		optional = fa.Optional || fb.Optional
		readonly = fa.Readonly && fb.Readonly
	case oka:
		fieldType = fa.Type
		optional = true
		readonly = fa.Readonly
		if tail, ok := recordTailFieldType(br, name); ok {
			fieldType, optional = normalizeMergedRecordField(s.joinReturnSlot(fa.Type, tail))
			readonly = false
		}
	case okb:
		fieldType = fb.Type
		optional = true
		readonly = fb.Readonly
		if tail, ok := recordTailFieldType(ar, name); ok {
			fieldType, optional = normalizeMergedRecordField(s.joinReturnSlot(tail, fb.Type))
			readonly = false
		}
	}
	return Field{Name: name, Type: fieldType, Optional: optional, Readonly: readonly}
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
	changed := false

	for i := 0; i < len(members); i++ {
		left := unaliasRecord(members[i])
		if left == nil {
			continue
		}
		for j := i + 1; j < len(members); j++ {
			right := unaliasRecord(members[j])
			if right == nil {
				continue
			}
			merged, ok := s.joinCompatibleRecords(left, right)
			if !ok {
				continue
			}
			members[i] = merged
			left = unaliasRecord(merged)
			members = append(members[:j], members[j+1:]...)
			j--
			changed = true
		}
	}

	if !changed {
		return t
	}
	return NewUnion(members...)
}

func hasConflictingRequiredLiteralField(a, b *Record) bool {
	i, j := 0, 0
	for i < len(a.Fields) && j < len(b.Fields) {
		fa := a.Fields[i]
		fb := b.Fields[j]
		switch {
		case fa.Name < fb.Name:
			i++
			continue
		case fa.Name > fb.Name:
			j++
			continue
		}
		if fa.Optional || fb.Optional {
			i++
			j++
			continue
		}
		la, oka := literalType(fa.Type)
		lb, okb := literalType(fb.Type)
		if !oka || !okb {
			i++
			j++
			continue
		}
		if !isDiscriminantLiteralField(fa.Name) {
			i++
			j++
			continue
		}
		if !TypeEquals(la, lb) {
			return true
		}
		i++
		j++
	}
	return false
}

func isDiscriminantLiteralField(name string) bool {
	switch name {
	case "type", "kind", "tag", "role", "variant", "success", "ok":
		return true
	default:
		return false
	}
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
	if TypeEquals(a, b) {
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
	return IsSoft(t, SoftAnnotationPolicy)
}
