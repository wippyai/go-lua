package core

import (
	"sort"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// Index resolves an index lookup (t[key]) on a container without using an engine.
//
// This is the pure, non-memoized version of index resolution. It handles:
//   - Array: numeric key returns element type
//   - Map: key subtype check, returns Optional(value type)
//   - Tuple: literal integer returns specific element, dynamic returns union
//   - Record: string key for field access, map component for others
//   - Union: index must succeed on all members
//   - Intersection: index succeeds if any member supports it
//
// Results are typically Optional because Lua tables return nil for missing keys.
func Index(t typ.Type, keyType typ.Type) (typ.Type, bool) {
	return indexDepth(t, keyType, 0)
}

// RuntimeIndex resolves Lua runtime indexed-read semantics.
//
// Index is the strict structural query: it returns false when the type cannot
// prove a present indexed value. RuntimeIndex answers the different question
// used by transfer and diagnostics: can Lua read t[key]? For table-like values,
// a missing key is a defined read of nil, not an indexing error.
func RuntimeIndex(t typ.Type, keyType typ.Type) (typ.Type, bool) {
	return runtimeIndexDepth(t, keyType, 0)
}

type indexResult struct {
	t  typ.Type
	ok bool
}

const runtimeIndexMaxDepth = 32

func runtimeIndexDepth(t, keyType typ.Type, depth int) (typ.Type, bool) {
	if depth > runtimeIndexMaxDepth || t == nil {
		return nil, false
	}
	if strict, ok := indexDepth(t, keyType, depth+1); ok {
		return strict, true
	}
	switch v := t.(type) {
	case *typ.Union:
		return runtimeUnionIndexType(v.Members, func(member typ.Type) (typ.Type, bool) {
			return runtimeIndexDepth(member, keyType, depth+1)
		})
	case *typ.Optional:
		return runtimeOptionalIndexType(v.Inner, func(inner typ.Type) (typ.Type, bool) {
			return runtimeIndexDepth(inner, keyType, depth+1)
		})
	case *typ.Alias:
		return runtimeIndexDepth(v.Target, keyType, depth+1)
	case *typ.Generic:
		return runtimeIndexDepth(v.Body, keyType, depth+1)
	case *typ.TypeParam:
		return runtimeIndexDepth(v.Constraint, keyType, depth+1)
	case *typ.Recursive:
		if v.Body == v {
			return nil, false
		}
		return runtimeIndexDepth(v.Body, keyType, depth+1)
	case *typ.Instantiated:
		resolved, err := ResolveInstantiated(v)
		if err != nil {
			return nil, false
		}
		return runtimeIndexDepth(resolved, keyType, depth+1)
	}
	if MissingFieldReadsNil(t) {
		return typ.Nil, true
	}
	return nil, false
}

func runtimeUnionIndexType(members []typ.Type, read func(typ.Type) (typ.Type, bool)) (typ.Type, bool) {
	if len(members) == 0 || read == nil {
		return nil, false
	}
	out := make([]typ.Type, 0, len(members))
	for _, member := range typ.CoalesceProductUnionMembers(members) {
		t, ok := read(member)
		if !ok || t == nil {
			return nil, false
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, false
	}
	return typ.CoalesceProductUnion(typ.NewUnion(out...)), true
}

func runtimeOptionalIndexType(inner typ.Type, read func(typ.Type) (typ.Type, bool)) (typ.Type, bool) {
	t, ok := read(inner)
	if !ok || t == nil || runtimeIndexContainsNil(t) {
		return t, ok
	}
	return typ.NewOptional(t), true
}

func runtimeIndexContainsNil(t typ.Type) bool {
	if t == nil {
		return false
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(*typ.Optional) bool {
			return true
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if runtimeIndexContainsNil(member) {
					return true
				}
			}
			return false
		},
		Default: func(t typ.Type) bool {
			return t.Kind() == kind.Nil
		},
	})
}

// indexDepth recursively resolves index operations with depth limiting.
// Handles various container types and propagates through wrappers.
func indexDepth(t, keyType typ.Type, depth int) (typ.Type, bool) {
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
			if isNumeric(keyType) {
				if a.Element == nil {
					return indexResult{t: typ.Nil, ok: true}
				}
				// A sequence has unknown runtime length, so an unproven numeric
				// index may be out of range and read nil. The result is optional;
				// a length lower-bound proof narrows it back to the element type.
				return indexResult{t: typ.NewOptional(a.Element), ok: true}
			}
			if keyType.Kind().IsPlaceholder() && a.Element != nil {
				return indexResult{t: typ.NewOptional(a.Element), ok: true}
			}
			return indexResult{}
		},
		Map: func(m *typ.Map) indexResult {
			if keyType.Kind().IsPlaceholder() {
				if m.Value == nil {
					return indexResult{}
				}
				return indexResult{t: typ.NewOptional(m.Value), ok: true}
			}

			// A nil-bearing key reads soundly in Lua (t[nil] yields nil) and the
			// result is already optional, so match against the non-nil key domain.
			if subtype.IsSubtype(nonNilKey(keyType), m.Key) {
				if m.Value == nil {
					return indexResult{}
				}

				// Map index returns optional because missing keys return nil in Lua
				return indexResult{t: typ.NewOptional(m.Value), ok: true}
			}

			return indexResult{}
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) indexResult {
			if keyType.Kind().IsPlaceholder() {
				if m.Value == nil {
					return indexResult{}
				}
				return indexResult{t: typ.NewOptional(m.Value), ok: true}
			}
			if subtype.IsSubtype(nonNilKey(keyType), m.Key) {
				if m.Value == nil {
					return indexResult{}
				}
				return indexResult{t: typ.NewOptional(m.Value), ok: true}
			}
			return indexResult{}
		},
		Tuple: func(tup *typ.Tuple) indexResult {
			// Integer literal index
			if lit, ok := keyType.(*typ.Literal); ok && lit.Base == kind.Integer {
				idx := lit.Value.(int64)
				if idx >= 1 && int(idx) <= len(tup.Elements) {
					return indexResult{t: tup.Elements[idx-1], ok: true}
				}
				// Out of bounds literal returns nil
				return indexResult{t: typ.Nil, ok: true}
			}
			// Unknown integer index returns optional union of all elements
			if isNumeric(keyType) && len(tup.Elements) > 0 {
				return indexResult{t: typ.NewOptional(typ.NewUnion(tup.Elements...)), ok: true}
			}
			if keyType != nil && keyType.Kind().IsPlaceholder() && len(tup.Elements) > 0 {
				return indexResult{t: typ.NewOptional(typ.NewUnion(tup.Elements...)), ok: true}
			}

			return indexResult{}
		},
		Record: func(r *typ.Record) indexResult {
			if keySet, ok := exactStringKeyDomain(keyType, depth+1); ok {
				return indexRecordByExactStringKeyDomain(r, keySet, depth+1)
			}
			if keySet, ok := exactIntKeyDomain(keyType, depth+1); ok {
				return indexRecordByExactIntKeyDomain(r, keySet)
			}
			if len(r.Fields) == 0 && !r.HasMapComponent() {
				if r.Open {
					return indexResult{t: typ.Unknown, ok: true}
				}
				return indexResult{t: typ.Nil, ok: true}
			}
			// Unknown string returns optional union of all field types. A
			// nil-bearing key reads soundly in Lua, so match the non-nil domain.
			if nnKey := nonNilKey(keyType); nnKey.Kind() == kind.String {
				return indexRecordByGenericStringKey(r, nnKey)
			}
			// Placeholder/unknown keys may still resolve to string fields at runtime.
			// Keep this sound by returning an optional union of field types.
			if keyType.Kind().IsPlaceholder() {
				var types []typ.Type
				for _, f := range r.Fields {
					types = append(types, f.Type)
				}
				if r.HasMapComponent() {
					types = append(types, r.MapValue)
				}
				if len(types) == 0 {
					return indexResult{t: typ.Nil, ok: true}
				}
				return indexResult{t: typ.NewOptional(typ.NewUnion(types...)), ok: true}
			}

			// Map component fallback for non-string-literal keys.
			if r.HasMapComponent() && keyType != nil {
				if keyType.Kind().IsPlaceholder() || subtype.IsSubtype(nonNilKey(keyType), r.MapKey) {
					return indexResult{t: typ.NewOptional(r.MapValue), ok: true}
				}
			}

			return indexResult{}
		},
		Union: func(u *typ.Union) indexResult {
			var types []typ.Type

			for _, m := range u.Members {
				et, ok := indexDepth(m, keyType, depth+1)
				if !ok {
					return indexResult{}
				}

				types = append(types, et)
			}

			if len(types) == 0 {
				return indexResult{}
			}

			return indexResult{t: typ.NewUnion(types...), ok: true}
		},
		Intersection: func(in *typ.Intersection) indexResult {
			var types []typ.Type

			for _, m := range in.Members {
				if et, ok := indexDepth(m, keyType, depth+1); ok {
					types = append(types, et)
				}
			}

			if len(types) == 0 {
				return indexResult{}
			}

			if len(types) == 1 {
				return indexResult{t: types[0], ok: true}
			}

			return indexResult{t: typ.NewIntersection(types...), ok: true}
		},
		Optional: func(o *typ.Optional) indexResult {
			et, ok := indexDepth(o.Inner, keyType, depth+1)
			if !ok {
				return indexResult{}
			}

			if containsNilOrOptional(et) {
				return indexResult{t: et, ok: true}
			}

			return indexResult{t: typ.NewOptional(et), ok: true}
		},
		Recursive: func(rec *typ.Recursive) indexResult {
			if rec.Body == nil || rec.Body == rec {
				return indexResult{}
			}
			et, ok := indexDepth(rec.Body, keyType, depth+1)
			return indexResult{t: et, ok: ok}
		},
		Alias: func(a *typ.Alias) indexResult {
			et, ok := indexDepth(a.Target, keyType, depth+1)
			return indexResult{t: et, ok: ok}
		},
		Instantiated: func(inst *typ.Instantiated) indexResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return indexResult{}
			}
			et, ok := indexDepth(resolved, keyType, depth+1)
			return indexResult{t: et, ok: ok}
		},
		Default: func(t typ.Type) indexResult {
			return indexResult{}
		},
	})
	return res.t, res.ok
}

func indexRecordByGenericStringKey(r *typ.Record, keyType typ.Type) indexResult {
	if r == nil {
		return indexResult{}
	}
	var types []typ.Type
	for _, f := range r.Fields {
		types = append(types, f.Type)
	}
	if r.HasMapComponent() && subtype.IsSubtype(keyType, r.MapKey) {
		types = append(types, r.MapValue)
		if len(types) == 0 {
			return indexResult{t: typ.Nil, ok: true}
		}
		return indexResult{t: typ.NewOptional(typ.NewUnion(types...)), ok: true}
	}
	if r.Open {
		return indexResult{t: typ.Unknown, ok: true}
	}
	if r.HasMapComponent() {
		types = append(types, r.MapValue)
	}
	if len(types) == 0 {
		return indexResult{t: typ.Nil, ok: true}
	}
	return indexResult{t: typ.NewOptional(typ.NewUnion(types...)), ok: true}
}

// nonNilKey strips nil from an index key type. Reading a Lua table with a
// nil-bearing key is sound (t[nil] yields nil), and the optional read result
// already captures the absent case, so key matching uses the non-nil domain.
func nonNilKey(keyType typ.Type) typ.Type {
	if keyType == nil {
		return keyType
	}
	switch k := keyType.(type) {
	case *typ.Optional:
		return nonNilKey(k.Inner)
	case *typ.Union:
		if !containsNilOrOptional(keyType) {
			return keyType
		}
		members := make([]typ.Type, 0, len(k.Members))
		for _, m := range k.Members {
			if m.Kind() == kind.Nil {
				continue
			}
			members = append(members, nonNilKey(m))
		}
		if len(members) == 0 {
			return keyType
		}
		return typ.NewUnion(members...)
	default:
		return keyType
	}
}

// containsNilOrOptional returns true if the type already contains nil or Optional.
//
// This check prevents double-wrapping: if a type is already Optional or contains
// nil as a union member, wrapping it again in Optional would be redundant.
// Used when propagating optionality through index operations.
func containsNilOrOptional(t typ.Type) bool {
	if t == nil {
		return false
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return true
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if m.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if m.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Default: func(t typ.Type) bool {
			return t.Kind() == kind.Nil
		},
	})
}

// exactStringKeyDomain returns the finite set of string keys represented by t.
//
// It only succeeds when the key type is exactly a finite union of string literals
// after transparently traversing wrappers such as aliases and instantiated types.
// This lets record projection reason about the full key domain instead of relying
// on raw AST/type shape.
func exactStringKeyDomain(t typ.Type, depth int) ([]string, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	keys := typ.Visit(t, typ.Visitor[[]string]{
		Literal: func(lit *typ.Literal) []string {
			if lit.Base != kind.String {
				return nil
			}
			str, ok := lit.Value.(string)
			if !ok {
				return nil
			}
			return []string{str}
		},
		Union: func(u *typ.Union) []string {
			if len(u.Members) == 0 {
				return nil
			}
			seen := make(map[string]struct{}, len(u.Members))
			var keys []string
			for _, member := range u.Members {
				memberKeys, ok := exactStringKeyDomain(member, depth+1)
				if !ok {
					return nil
				}
				for _, key := range memberKeys {
					if _, exists := seen[key]; exists {
						continue
					}
					seen[key] = struct{}{}
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			return keys
		},
		Alias: func(a *typ.Alias) []string {
			keys, ok := exactStringKeyDomain(a.Target, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		Optional: func(o *typ.Optional) []string {
			keys, ok := exactStringKeyDomain(o.Inner, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		Recursive: func(rec *typ.Recursive) []string {
			if rec.Body == nil || rec.Body == rec {
				return nil
			}
			keys, ok := exactStringKeyDomain(rec.Body, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		Instantiated: func(inst *typ.Instantiated) []string {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return nil
			}
			keys, ok := exactStringKeyDomain(resolved, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		TypeParam: func(tp *typ.TypeParam) []string {
			if tp.Constraint == nil {
				return nil
			}
			keys, ok := exactStringKeyDomain(tp.Constraint, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		Default: func(t typ.Type) []string {
			return nil
		},
	})
	if keys == nil {
		return nil, false
	}
	return keys, true
}

func exactIntKeyDomain(t typ.Type, depth int) ([]int64, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	keys := typ.Visit(t, typ.Visitor[[]int64]{
		Literal: func(lit *typ.Literal) []int64 {
			if lit.Base != kind.Integer {
				return nil
			}
			i, ok := lit.Value.(int64)
			if !ok {
				return nil
			}
			return []int64{i}
		},
		Union: func(u *typ.Union) []int64 {
			if len(u.Members) == 0 {
				return nil
			}
			seen := make(map[int64]struct{}, len(u.Members))
			var keys []int64
			for _, member := range u.Members {
				memberKeys, ok := exactIntKeyDomain(member, depth+1)
				if !ok {
					return nil
				}
				for _, key := range memberKeys {
					if _, exists := seen[key]; exists {
						continue
					}
					seen[key] = struct{}{}
					keys = append(keys, key)
				}
			}
			sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
			return keys
		},
		Alias: func(a *typ.Alias) []int64 {
			keys, ok := exactIntKeyDomain(a.Target, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		Optional: func(o *typ.Optional) []int64 {
			keys, ok := exactIntKeyDomain(o.Inner, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		TypeParam: func(tp *typ.TypeParam) []int64 {
			if tp.Constraint == nil {
				return nil
			}
			keys, ok := exactIntKeyDomain(tp.Constraint, depth+1)
			if !ok {
				return nil
			}
			return keys
		},
		Default: func(t typ.Type) []int64 {
			return nil
		},
	})
	if keys == nil {
		return nil, false
	}
	return keys, true
}

func indexRecordByExactStringKeyDomain(r *typ.Record, keys []string, depth int) indexResult {
	if len(keys) == 0 {
		return indexResult{}
	}

	var matched []typ.Type
	missing := false

	for _, key := range keys {
		var fieldType typ.Type
		found := false
		if member := r.GetStaticStringIndex(key); member != nil {
			fieldType = staticMemberReadType(member)
			found = true
		} else {
			fieldType, found = fieldInRecordDepth(r, key, depth+1)
		}
		if !found {
			missing = true
			continue
		}
		if fieldType != nil {
			matched = append(matched, fieldType)
		}
	}

	if len(matched) == 0 {
		return indexResult{}
	}

	out := typ.NewUnion(matched...)
	if missing && !containsNilOrOptional(out) {
		out = typ.NewOptional(out)
	}

	return indexResult{t: out, ok: true}
}

func staticMemberReadType(member *typ.StaticMember) typ.Type {
	if member == nil {
		return nil
	}
	t := member.Type
	if member.Optional && !containsNilOrOptional(t) {
		t = typ.NewOptional(t)
	}
	return t
}

func indexRecordByExactIntKeyDomain(r *typ.Record, keys []int64) indexResult {
	if len(keys) == 0 {
		return indexResult{}
	}

	var matched []typ.Type
	missing := false
	for _, key := range keys {
		member := r.GetStaticIntIndex(key)
		switch {
		case member != nil:
			if member.Type != nil {
				matched = append(matched, staticMemberReadType(member))
			}
		case r.HasMapComponent() && subtype.IsSubtype(typ.LiteralInt(key), r.MapKey):
			if r.MapValue != nil {
				matched = append(matched, typ.NewOptional(r.MapValue))
			}
		case r.Open:
			matched = append(matched, typ.Unknown)
		default:
			missing = true
		}
	}
	if len(matched) == 0 {
		return indexResult{}
	}
	out := typ.NewUnion(matched...)
	if missing && !containsNilOrOptional(out) {
		out = typ.NewOptional(out)
	}
	return indexResult{t: out, ok: true}
}
