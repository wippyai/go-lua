package value

import (
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// AdmitArrayElementMutation returns the value-domain result of observing an
// array-like table mutation that appends or stores elementType.
func AdmitArrayElementMutation(arrayType, elementType typ.Type, joinFn func(a, b typ.Type) typ.Type) typ.Type {
	if elementType == nil {
		return arrayType
	}
	if joinFn == nil {
		joinFn = typ.JoinPreferNonSoft
	}
	elementType = AdmitObservation(elementType)

	if arrayType == nil {
		return typ.NewArray(elementType)
	}
	if typ.IsAny(arrayType) || typ.IsUnknown(arrayType) {
		return typ.NewArray(elementType)
	}

	return typ.Visit(arrayType, typ.Visitor[typ.Type]{
		Alias: func(a *typ.Alias) typ.Type {
			widened := AdmitArrayElementMutation(a.Target, elementType, joinFn)
			if widened == nil || typ.TypeEquals(widened, a.Target) {
				return arrayType
			}
			return typ.NewAlias(a.Name, widened)
		},
		Array: func(arr *typ.Array) typ.Type {
			joined := joinFn(arr.Element, elementType)
			if arr.Element != nil && arr.Element.Kind().IsPlaceholder() &&
				elementType != nil && !elementType.Kind().IsPlaceholder() {
				joined = elementType
			}
			if joined == nil || typ.TypeEquals(joined, arr.Element) {
				return arrayType
			}
			return typ.NewArray(joined)
		},
		Record: func(rec *typ.Record) typ.Type {
			if len(rec.Fields) == 0 {
				return typ.NewArray(elementType)
			}
			return arrayType
		},
		Union: func(u *typ.Union) typ.Type {
			if widened, ok := admitSoftArrayPlaceholderUnion(u, elementType, joinFn); ok {
				return widened
			}
			var updated []typ.Type
			found := false
			changed := false
			for _, member := range u.Members {
				if arr, ok := member.(*typ.Array); ok && !found {
					joined := joinFn(arr.Element, elementType)
					if joined == nil || typ.TypeEquals(joined, arr.Element) {
						updated = append(updated, member)
					} else {
						updated = append(updated, typ.NewArray(joined))
						changed = true
					}
					found = true
				} else {
					updated = append(updated, member)
				}
			}
			if found && changed {
				return typ.NewUnion(updated...)
			}
			return arrayType
		},
		Default: func(t typ.Type) typ.Type {
			if arrayType.Kind().IsPlaceholder() {
				return typ.NewArray(elementType)
			}
			return arrayType
		},
	})
}

func admitSoftArrayPlaceholderUnion(u *typ.Union, elementType typ.Type, joinFn func(typ.Type, typ.Type) typ.Type) (typ.Type, bool) {
	if u == nil || elementType == nil || elementType.Kind().IsPlaceholder() {
		return nil, false
	}
	arrayIdx := -1
	hasEmptyRecord := false
	for i, member := range u.Members {
		if arr, ok := member.(*typ.Array); ok && arr.Element != nil && arr.Element.Kind().IsPlaceholder() && arrayIdx < 0 {
			arrayIdx = i
			continue
		}
		if unwrap.IsEmptyRecord(member) {
			hasEmptyRecord = true
		}
	}
	if arrayIdx < 0 || !hasEmptyRecord {
		return nil, false
	}
	out := make([]typ.Type, 0, len(u.Members))
	for i, member := range u.Members {
		switch {
		case i == arrayIdx:
			arr := member.(*typ.Array)
			elem := joinFn(arr.Element, elementType)
			if elem == nil || elem.Kind().IsPlaceholder() {
				elem = elementType
			}
			out = append(out, typ.NewArray(elem))
		case unwrap.IsEmptyRecord(member):
			continue
		default:
			out = append(out, member)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return typ.NewUnion(out...), true
}

// AdmitMapArrayElementMutation returns the value-domain result of observing a
// table mutation through a dynamic key whose value slot is array-like.
func AdmitMapArrayElementMutation(mapType typ.Type, keyType, elementType typ.Type) typ.Type {
	keyType = subtype.Widen(keyType)
	elementType = AdmitObservation(elementType)
	if mapType == nil {
		return typ.NewMap(keyType, typ.NewArray(elementType))
	}

	return typ.Visit(mapType, typ.Visitor[typ.Type]{
		Alias: func(a *typ.Alias) typ.Type {
			widened := AdmitMapArrayElementMutation(a.Target, keyType, elementType)
			if widened == nil || typ.TypeEquals(widened, a.Target) {
				return mapType
			}
			return typ.NewAlias(a.Name, widened)
		},
		Map: func(m *typ.Map) typ.Type {
			newKey := mergeMapKeyDomain(m.Key, keyType)
			newVal := AdmitArrayElementMutation(m.Value, elementType, joinContainerValueTypes)
			if newVal == nil {
				return mapType
			}
			if typ.TypeEquals(m.Key, newKey) && typ.TypeEquals(m.Value, newVal) {
				return mapType
			}
			return typ.NewMap(newKey, newVal)
		},
		Record: func(r *typ.Record) typ.Type {
			if len(r.Fields) == 0 {
				return typ.NewMap(keyType, typ.NewArray(elementType))
			}
			return mapType
		},
		Union: func(u *typ.Union) typ.Type {
			var updated []typ.Type
			found := false
			changed := false
			for _, member := range u.Members {
				if mp, ok := member.(*typ.Map); ok && !found {
					newKey := mergeMapKeyDomain(mp.Key, keyType)
					newVal := AdmitArrayElementMutation(mp.Value, elementType, joinContainerValueTypes)
					if newVal == nil {
						updated = append(updated, member)
					} else if typ.TypeEquals(mp.Key, newKey) && typ.TypeEquals(mp.Value, newVal) {
						updated = append(updated, member)
					} else {
						updated = append(updated, typ.NewMap(newKey, newVal))
						changed = true
					}
					found = true
				} else {
					updated = append(updated, member)
				}
			}
			if found && changed {
				return typ.NewUnion(updated...)
			}
			return mapType
		},
		Default: func(t typ.Type) typ.Type {
			if mapType.Kind().IsPlaceholder() {
				return typ.NewMap(keyType, typ.NewArray(elementType))
			}
			return mapType
		},
	})
}

func mergeMapKeyDomain(existing, incoming typ.Type) typ.Type {
	if existing == nil {
		return incoming
	}
	if incoming == nil {
		return existing
	}
	if incoming.Kind().IsPlaceholder() && !existing.Kind().IsPlaceholder() {
		return existing
	}
	if existing.Kind().IsPlaceholder() && !incoming.Kind().IsPlaceholder() {
		return incoming
	}
	return typ.JoinPreferNonSoft(existing, incoming)
}

func joinContainerValueTypes(existing, incoming typ.Type) typ.Type {
	joined := typ.JoinPreferNonSoft(existing, incoming)
	if union, ok := joined.(*typ.Union); ok {
		return typejoin.Types(union.Members...)
	}
	return joined
}

// AdmitIndexedWrite returns the value-domain result of observing t[k] = v.
//
// Nil values are skipped because Lua table assignment of nil deletes a key
// rather than storing a value. Map reads already model missing keys as optional.
func AdmitIndexedWrite(t typ.Type, keyType, valType typ.Type) typ.Type {
	if valType != nil && valType.Kind() == kind.Nil {
		return t
	}
	if t == nil {
		return typ.NewMap(keyType, valType)
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Alias: func(a *typ.Alias) typ.Type {
			widened := AdmitIndexedWrite(a.Target, keyType, valType)
			if widened == nil || typ.TypeEquals(widened, a.Target) {
				return t
			}
			return typ.NewAlias(a.Name, widened)
		},
		Tuple: func(tp *typ.Tuple) typ.Type {
			elemType := valType
			for _, elem := range tp.Elements {
				elemType = typ.JoinPreferNonSoft(elemType, elem)
			}
			if subtype.IsSubtype(keyType, typ.Integer) || subtype.IsSubtype(keyType, typ.Number) {
				return typ.NewArray(elemType)
			}
			return typ.NewMap(keyType, elemType)
		},
		Record: func(r *typ.Record) typ.Type {
			if len(r.Fields) == 0 && !r.HasMapComponent() {
				return typ.NewMap(keyType, valType)
			}
			if r.HasMapComponent() {
				newKey := mergeMapKeyDomain(r.MapKey, keyType)
				newVal := joinContainerValueTypes(r.MapValue, valType)
				if typ.TypeEquals(r.MapKey, newKey) && typ.TypeEquals(r.MapValue, newVal) {
					return t
				}
				return rebuildRecordWithMapComponent(r, newKey, newVal)
			}
			return rebuildRecordWithMapComponent(r, keyType, valType)
		},
		Map: func(m *typ.Map) typ.Type {
			newKey := mergeMapKeyDomain(m.Key, keyType)
			newVal := joinContainerValueTypes(m.Value, valType)
			if typ.TypeEquals(m.Key, newKey) && typ.TypeEquals(m.Value, newVal) {
				return t
			}
			return typ.NewMap(newKey, newVal)
		},
		Default: func(t typ.Type) typ.Type {
			if t.Kind().IsPlaceholder() {
				return typ.NewMap(keyType, valType)
			}
			return t
		},
	})
}

// AdmitForeignIndexedWrite returns the value-domain result of observing t[k] = v
// when v is a FOREIGN value — one not provably drawn from t at the key being
// written. For a closed record with declared fields it differs from
// AdmitIndexedWrite: a foreign value stored through a dynamic key that could match
// a declared field's name WEAKENS that field's type (joining v into it), because at
// runtime the write can land on that field and replace its value. Merely adding a
// `[K]: v` map component (as AdmitIndexedWrite does) would leave the declared field
// type intact and let a later `r.field` read return the original type when the
// runtime value is v — unsound.
//
// A field is weakened only when keyType is consistent with the literal field name
// (a write under keyType could land on that key): a string-literal key weakens only
// the named field, a `string`-domain key weakens every string-named field, an
// integer/other key weakens none. For a non-record container, or a write whose key
// cannot reach any declared field, the result equals AdmitIndexedWrite (the map/array
// widening already over-approximates soundly).
func AdmitForeignIndexedWrite(t typ.Type, keyType, valType typ.Type) typ.Type {
	if valType != nil && valType.Kind() == kind.Nil {
		return t
	}
	if t == nil || valType == nil {
		return AdmitIndexedWrite(t, keyType, valType)
	}
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || len(rec.Fields) == 0 {
		return AdmitIndexedWrite(t, keyType, valType)
	}
	weakened := weakenRecordFieldsForForeignWrite(rec, keyType, valType)
	// The map component still records the open key/value domain the write admits, so a
	// later dynamic-key read sees v. Reuse the AdmitIndexedWrite record path over the
	// field-weakened record so the map component merge stays identical.
	return AdmitIndexedWrite(weakened, keyType, valType)
}

// weakenRecordFieldsForForeignWrite rebuilds rec with every declared field the
// dynamic key could match joined with valType. A field name the key domain cannot
// reach is left intact, so a literal-keyed write weakens only its target field. The
// record's open flag, metatable, and existing map component are preserved; the caller
// adds the map component for the dynamic key.
func weakenRecordFieldsForForeignWrite(rec *typ.Record, keyType, valType typ.Type) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range rec.Fields {
		ft := f.Type
		if keyCanMatchFieldName(keyType, f.Name) {
			ft = joinContainerValueTypes(f.Type, valType)
		}
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, ft)
		case f.Optional:
			builder.OptField(f.Name, ft)
		case f.Readonly:
			builder.ReadonlyField(f.Name, ft)
		default:
			builder.Field(f.Name, ft)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.Build()
}

// keyCanMatchFieldName reports whether a dynamic key of type keyType could equal the
// string field name at runtime. A nil/placeholder key domain (an untyped dynamic key)
// could be any string, so it matches; a concrete key matches when the literal field
// name is within its domain (subtype of keyType). An integer/number-only key never
// names a string field.
func keyCanMatchFieldName(keyType typ.Type, name string) bool {
	if keyType == nil || keyType.Kind().IsPlaceholder() {
		return true
	}
	return subtype.IsSubtype(typ.LiteralString(name), keyType)
}

// IndexedWriteAdmits reports whether the value domain can soundly admit an
// indexed write on t. It is the predicate counterpart to AdmitIndexedWrite:
// transfer uses AdmitIndexedWrite to compute the next value, while proof
// consumers use this predicate to avoid reimplementing write-side table laws.
func IndexedWriteAdmits(t typ.Type, keyType, valType typ.Type) bool {
	if valType == nil {
		return false
	}
	if valType.Kind() == kind.Nil {
		return true
	}
	if t == nil {
		return true
	}
	if t.Kind().IsPlaceholder() {
		return true
	}
	// A map's value slot is covariant: an indexed write widens it (and the key
	// domain) to the union of all writes, matching AdmitIndexedWrite, so an
	// inferred map admits any value. The invariant obligation below applies to a
	// record reached through a dynamic key, where a write that does not satisfy
	// every field obligation would corrupt a typed field.
	if _, ok := unwrap.Alias(t).(*typ.Map); ok {
		return true
	}
	if obligation, ok := querycore.IndexWriteObligation(t, keyType); ok {
		return subtype.IsSubtype(valType, obligation)
	}
	return TableTopCovers(t)
}

// IndexedValueMutationAdmits reports whether the value domain can soundly
// admit a structural mutation inside t[key], such as t[k].field = v. Unlike a
// replacement write, the incoming value is a patch for the element slot, so the
// obligation is that the selected slot is table-like or gradual.
func IndexedValueMutationAdmits(t typ.Type, keyType, valType typ.Type) bool {
	if valType == nil {
		return false
	}
	if t == nil {
		return true
	}
	if t.Kind().IsPlaceholder() {
		return true
	}
	if slot, ok := querycore.IndexWriteObligation(t, keyType); ok {
		return slot.Kind().IsPlaceholder() || TableTopCovers(slot)
	}
	return TableTopCovers(t)
}

// AdmitIndexedValueMutation returns the value-domain result of observing a
// mutation to the table stored at t[k], such as t[k].field = v. Unlike a full
// indexed write, the incoming value is joined with the existing map value as a
// structural update instead of replacing the possible slot value.
func AdmitIndexedValueMutation(t typ.Type, keyType, valType typ.Type) typ.Type {
	if valType != nil && valType.Kind() == kind.Nil {
		return t
	}
	if t == nil {
		return typ.NewMap(keyType, valType)
	}

	mergeValue := func(existing typ.Type) typ.Type {
		if existing == nil {
			return valType
		}
		return MergeForConvergence(existing, valType)
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Alias: func(a *typ.Alias) typ.Type {
			widened := AdmitIndexedValueMutation(a.Target, keyType, valType)
			if widened == nil || typ.TypeEquals(widened, a.Target) {
				return t
			}
			return typ.NewAlias(a.Name, widened)
		},
		Record: func(r *typ.Record) typ.Type {
			if len(r.Fields) == 0 && !r.HasMapComponent() {
				return typ.NewMap(keyType, valType)
			}
			if r.HasMapComponent() {
				newKey := mergeMapKeyDomain(r.MapKey, keyType)
				newVal := mergeValue(r.MapValue)
				if typ.TypeEquals(r.MapKey, newKey) && typ.TypeEquals(r.MapValue, newVal) {
					return t
				}
				return rebuildRecordWithMapComponent(r, newKey, newVal)
			}
			return rebuildRecordWithMapComponent(r, keyType, valType)
		},
		Map: func(m *typ.Map) typ.Type {
			newKey := mergeMapKeyDomain(m.Key, keyType)
			newVal := mergeValue(m.Value)
			if typ.TypeEquals(m.Key, newKey) && typ.TypeEquals(m.Value, newVal) {
				return t
			}
			return typ.NewMap(newKey, newVal)
		},
		Default: func(t typ.Type) typ.Type {
			if t.Kind().IsPlaceholder() {
				return typ.NewMap(keyType, valType)
			}
			return t
		},
	})
}

func rebuildRecordWithMapComponent(rec *typ.Record, mapKey, mapVal typ.Type) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range rec.Fields {
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, f.Type)
		case f.Optional:
			builder.OptField(f.Name, f.Type)
		case f.Readonly:
			builder.ReadonlyField(f.Name, f.Type)
		default:
			builder.Field(f.Name, f.Type)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	builder.MapComponent(mapKey, mapVal)
	return builder.Build()
}
