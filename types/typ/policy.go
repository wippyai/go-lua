package typ

import (
	"sort"

	"github.com/wippyai/go-lua/types/kind"
)

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
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = PruneSoftUnionMembers(a)
	b = PruneSoftUnionMembers(b)
	if preferred, ok := preferArrayOverEmptyRecord(a, b); ok {
		return preferred
	}
	if merged, ok := JoinCompatibleRecords(a, b); ok {
		return merged
	}
	if (IsAny(a) && b.Kind() == kind.Nil) || (IsAny(b) && a.Kind() == kind.Nil) {
		return Any
	}
	if IsUnknown(a) || IsUnknown(b) {
		return Unknown
	}
	return coalesceCompatibleRecordMembers(JoinPreferNonSoft(a, b))
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
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar == nil || br == nil {
		return nil, false
	}

	// Keep discriminated unions intact when required literal tags conflict.
	if hasConflictingRequiredLiteralField(ar, br) {
		return nil, false
	}

	// Mixing map and non-map record slots can be semantically distinct.
	if ar.HasMapComponent() != br.HasMapComponent() {
		return nil, false
	}

	builder := NewRecord()
	if ar.Open || br.Open {
		builder.SetOpen(true)
	}
	if ar.Metatable != nil && br.Metatable != nil && TypeEquals(ar.Metatable, br.Metatable) {
		builder.Metatable(ar.Metatable)
	}
	if ar.HasMapComponent() && br.HasMapComponent() {
		builder.MapComponent(
			JoinPreferNonSoft(ar.MapKey, br.MapKey),
			JoinPreferNonSoft(ar.MapValue, br.MapValue),
		)
	}

	fieldsA := recordFieldsByName(ar)
	fieldsB := recordFieldsByName(br)
	names := make(map[string]struct{}, len(fieldsA)+len(fieldsB))
	for name := range fieldsA {
		names[name] = struct{}{}
	}
	for name := range fieldsB {
		names[name] = struct{}{}
	}
	sortedNames := make([]string, 0, len(names))
	for name := range names {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	for _, name := range sortedNames {
		fa, oka := fieldsA[name]
		fb, okb := fieldsB[name]

		fieldType := Type(nil)
		optional := true
		readonly := false
		switch {
		case oka && okb:
			// Record coalescing is used from JoinReturnSlot; keep field-level merge
			// on the same return-slot policy so empty-collection paths and nil/unknown
			// interactions are handled consistently in nested return records.
			fieldType = JoinReturnSlot(fa.Type, fb.Type)
			optional = fa.Optional || fb.Optional
			readonly = fa.Readonly && fb.Readonly
		case oka:
			fieldType = fa.Type
			optional = true
			readonly = fa.Readonly
			if tail, ok := recordTailFieldType(br, name); ok {
				fieldType, optional = normalizeMergedRecordField(JoinReturnSlot(fa.Type, tail))
				readonly = false
			}
		case okb:
			fieldType = fb.Type
			optional = true
			readonly = fb.Readonly
			if tail, ok := recordTailFieldType(ar, name); ok {
				fieldType, optional = normalizeMergedRecordField(JoinReturnSlot(tail, fb.Type))
				readonly = false
			}
		}

		switch {
		case optional && readonly:
			builder.OptReadonlyField(name, fieldType)
		case optional:
			builder.OptField(name, fieldType)
		case readonly:
			builder.ReadonlyField(name, fieldType)
		default:
			builder.Field(name, fieldType)
		}
	}

	return builder.Build(), true
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

func coalesceCompatibleRecordMembers(t Type) Type {
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
			merged, ok := JoinCompatibleRecords(left, right)
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

func recordFieldsByName(r *Record) map[string]Field {
	fields := make(map[string]Field, len(r.Fields))
	for _, field := range r.Fields {
		fields[field.Name] = field
	}
	return fields
}

func hasConflictingRequiredLiteralField(a, b *Record) bool {
	fieldsA := recordFieldsByName(a)
	fieldsB := recordFieldsByName(b)
	for name, fa := range fieldsA {
		fb, ok := fieldsB[name]
		if !ok {
			continue
		}
		if fa.Optional || fb.Optional {
			continue
		}
		la, oka := literalType(fa.Type)
		lb, okb := literalType(fb.Type)
		if !oka || !okb {
			continue
		}
		if !isDiscriminantLiteralField(name) {
			continue
		}
		if !TypeEquals(la, lb) {
			return true
		}
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
