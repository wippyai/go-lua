package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// IndexWrite resolves the value type accepted by a write t[key] = value.
//
// This is the write-side counterpart to Index. Reads return optional values
// because missing Lua table keys produce nil. Writes target the slot itself:
// a value must be compatible with every possible slot the key may denote.
func IndexWrite(t typ.Type, keyType typ.Type) (typ.Type, bool) {
	return indexWriteDepth(t, keyType, 0)
}

// IndexDelete reports whether t[key] = nil is a valid table deletion.
//
// Deleting a map entry is not the same operation as writing nil into the
// element domain: a {[string]: T} table may have absent keys even when present
// values must be T. Required record fields still reject deletion because it
// would break the record invariant.
func IndexDelete(t typ.Type, keyType typ.Type) bool {
	return indexDeleteDepth(t, keyType, 0)
}

func indexWriteDepth(t, keyType typ.Type, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	if keyType == nil {
		keyType = typ.Unknown
	}
	if top, ok := specialAccessType(t); ok {
		return top, true
	}

	res := typ.Visit(t, typ.Visitor[indexResult]{
		Array: func(a *typ.Array) indexResult {
			if keyType.Kind().IsPlaceholder() || isNumeric(keyType) {
				return indexResult{t: a.Element, ok: a.Element != nil}
			}
			return indexResult{}
		},
		Map: func(m *typ.Map) indexResult {
			if keyMatchesIndex(keyType, m.Key) {
				return indexResult{t: m.Value, ok: m.Value != nil}
			}
			return indexResult{}
		},
		Tuple: func(tup *typ.Tuple) indexResult {
			if lit, ok := unwrap.Alias(keyType).(*typ.Literal); ok && lit.Base == kind.Integer {
				idx := lit.Value.(int64)
				if idx >= 1 && int(idx) <= len(tup.Elements) {
					return indexResult{t: tup.Elements[idx-1], ok: tup.Elements[idx-1] != nil}
				}
				return indexResult{}
			}
			if keyType.Kind().IsPlaceholder() || isNumeric(keyType) {
				return meetWritableTypes(tup.Elements)
			}
			return indexResult{}
		},
		Record: func(r *typ.Record) indexResult {
			return recordIndexWriteType(r, keyType, depth+1)
		},
		Union: func(u *typ.Union) indexResult {
			var slots []typ.Type
			for _, member := range u.Members {
				slot, ok := indexWriteDepth(member, keyType, depth+1)
				if !ok {
					return indexResult{}
				}
				slots = append(slots, slot)
			}
			return meetWritableTypes(slots)
		},
		Intersection: func(in *typ.Intersection) indexResult {
			var slots []typ.Type
			for _, member := range in.Members {
				if slot, ok := indexWriteDepth(member, keyType, depth+1); ok {
					slots = append(slots, slot)
				}
			}
			return meetWritableTypes(slots)
		},
		Optional: func(o *typ.Optional) indexResult {
			slot, ok := indexWriteDepth(o.Inner, keyType, depth+1)
			return indexResult{t: slot, ok: ok}
		},
		Recursive: func(rec *typ.Recursive) indexResult {
			if rec.Body == nil || rec.Body == rec {
				return indexResult{}
			}
			slot, ok := indexWriteDepth(rec.Body, keyType, depth+1)
			return indexResult{t: slot, ok: ok}
		},
		Alias: func(a *typ.Alias) indexResult {
			slot, ok := indexWriteDepth(a.Target, keyType, depth+1)
			return indexResult{t: slot, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) indexResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return indexResult{}
			}
			slot, ok := indexWriteDepth(resolved, keyType, depth+1)
			return indexResult{t: slot, ok: ok}
		},
		TypeParam: func(tp *typ.TypeParam) indexResult {
			if tp.Constraint == nil {
				return indexResult{}
			}
			slot, ok := indexWriteDepth(tp.Constraint, keyType, depth+1)
			return indexResult{t: slot, ok: ok}
		},
		Default: func(t typ.Type) indexResult {
			return indexResult{}
		},
	})
	return res.t, res.ok
}

func indexDeleteDepth(t, keyType typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	if keyType == nil {
		keyType = typ.Unknown
	}
	if _, ok := specialAccessType(t); ok {
		return false
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Map: func(m *typ.Map) bool {
			return keyMatchesIndex(keyType, m.Key)
		},
		Record: func(r *typ.Record) bool {
			return recordIndexDeleteAllowed(r, keyType, depth+1)
		},
		Union: func(u *typ.Union) bool {
			if len(u.Members) == 0 {
				return false
			}
			for _, member := range u.Members {
				if !indexDeleteDepth(member, keyType, depth+1) {
					return false
				}
			}
			return true
		},
		Intersection: func(in *typ.Intersection) bool {
			if len(in.Members) == 0 {
				return false
			}
			for _, member := range in.Members {
				if !indexDeleteDepth(member, keyType, depth+1) {
					return false
				}
			}
			return true
		},
		Optional: func(o *typ.Optional) bool {
			return indexDeleteDepth(o.Inner, keyType, depth+1)
		},
		Recursive: func(rec *typ.Recursive) bool {
			return rec.Body != nil && rec.Body != rec && indexDeleteDepth(rec.Body, keyType, depth+1)
		},
		Alias: func(a *typ.Alias) bool {
			return indexDeleteDepth(a.Target, keyType, depth+1)
		},
		Instantiated: func(inst *typ.Instantiated) bool {
			resolved, err := ResolveInstantiated(inst)
			return err == nil && indexDeleteDepth(resolved, keyType, depth+1)
		},
		TypeParam: func(tp *typ.TypeParam) bool {
			return tp.Constraint != nil && indexDeleteDepth(tp.Constraint, keyType, depth+1)
		},
		Default: func(t typ.Type) bool {
			return false
		},
	})
}

func recordIndexWriteType(r *typ.Record, keyType typ.Type, depth int) indexResult {
	if r == nil || keyType == nil {
		return indexResult{}
	}
	if keySet, ok := exactStringKeyDomain(keyType, depth+1); ok {
		var slots []typ.Type
		source := writeSlotUnknown
		for _, name := range keySet {
			if slot, slotSource, ok := recordStringKeyWriteType(r, name); ok {
				slots = append(slots, slot)
				source = source.merge(slotSource)
				continue
			}
			return indexResult{}
		}
		if source == writeSlotMixed {
			return uniformWritableType(slots)
		}
		return meetWritableTypes(slots)
	}
	if keyType.Kind().IsPlaceholder() || subtype.IsSubtype(keyType, typ.String) {
		var slots []typ.Type
		for _, field := range r.Fields {
			slots = append(slots, writableFieldType(field))
		}
		if r.HasMapComponent() && keyMatchesIndex(keyType, r.MapKey) {
			slots = append(slots, r.MapValue)
		}
		return uniformWritableType(slots)
	}
	if r.HasMapComponent() && keyMatchesIndex(keyType, r.MapKey) {
		return indexResult{t: r.MapValue, ok: r.MapValue != nil}
	}
	return indexResult{}
}

func recordIndexDeleteAllowed(r *typ.Record, keyType typ.Type, depth int) bool {
	if r == nil || keyType == nil {
		return false
	}
	if keySet, ok := exactStringKeyDomain(keyType, depth+1); ok {
		for _, name := range keySet {
			if !recordStringKeyDeleteAllowed(r, name) {
				return false
			}
		}
		return len(keySet) > 0
	}
	if keyType.Kind().IsPlaceholder() || subtype.IsSubtype(keyType, typ.String) {
		for _, field := range r.Fields {
			if !field.Optional {
				return false
			}
		}
		return r.HasMapComponent() && keyMatchesIndex(keyType, r.MapKey)
	}
	return r.HasMapComponent() && keyMatchesIndex(keyType, r.MapKey)
}

type writeSlotSource uint8

const (
	writeSlotUnknown writeSlotSource = iota
	writeSlotDirect
	writeSlotTail
	writeSlotMixed
)

func (s writeSlotSource) merge(other writeSlotSource) writeSlotSource {
	if s == writeSlotUnknown {
		return other
	}
	if other == writeSlotUnknown || s == other {
		return s
	}
	return writeSlotMixed
}

func recordStringKeyWriteType(r *typ.Record, name string) (typ.Type, writeSlotSource, bool) {
	if field := r.GetField(name); field != nil {
		return writableFieldType(*field), writeSlotDirect, true
	}
	key := typ.LiteralString(name)
	if r.HasMapComponent() && subtype.IsSubtype(key, r.MapKey) {
		return r.MapValue, writeSlotTail, r.MapValue != nil
	}
	return nil, writeSlotUnknown, false
}

func recordStringKeyDeleteAllowed(r *typ.Record, name string) bool {
	if field := r.GetField(name); field != nil {
		return field.Optional
	}
	key := typ.LiteralString(name)
	return r.HasMapComponent() && subtype.IsSubtype(key, r.MapKey)
}

func writableFieldType(field typ.Field) typ.Type {
	slot := widenWritableFieldDefault(field.Type)
	if field.Optional {
		return typ.NewOptional(slot)
	}
	return slot
}

func widenWritableFieldDefault(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if isClosedLiteralDomain(t) {
		return t
	}
	if opt, ok := unwrap.Alias(t).(*typ.Optional); ok && opt != nil {
		inner := widenWritableFieldDefault(opt.Inner)
		if inner == nil || typ.TypeEquals(inner, opt.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	}
	return subtype.WidenForInference(t)
}

func isClosedLiteralDomain(t typ.Type) bool {
	if t == nil {
		return false
	}
	if opt, ok := unwrap.Alias(t).(*typ.Optional); ok && opt != nil {
		return isClosedLiteralDomain(opt.Inner)
	}
	u, ok := unwrap.Alias(t).(*typ.Union)
	if !ok || len(u.Members) == 0 {
		return false
	}
	for _, member := range u.Members {
		member = unwrap.Alias(member)
		if member == nil || (member.Kind() != kind.Literal && member.Kind() != kind.Nil) {
			return false
		}
	}
	return true
}

func keyMatchesIndex(keyType, indexType typ.Type) bool {
	if keyType == nil || indexType == nil {
		return false
	}
	if keyType.Kind().IsPlaceholder() {
		return true
	}
	return subtype.IsSubtype(keyType, indexType)
}

func meetWritableTypes(slots []typ.Type) indexResult {
	if len(slots) == 0 {
		return indexResult{}
	}
	if len(slots) == 1 {
		return indexResult{t: slots[0], ok: slots[0] != nil}
	}
	return indexResult{t: typ.NewIntersection(slots...), ok: true}
}

func uniformWritableType(slots []typ.Type) indexResult {
	if len(slots) == 0 {
		return indexResult{}
	}
	first := slots[0]
	if first == nil {
		return indexResult{}
	}
	for _, slot := range slots[1:] {
		if slot == nil || !typ.TypeEquals(slot, first) {
			return indexResult{}
		}
	}
	return indexResult{t: first, ok: true}
}
