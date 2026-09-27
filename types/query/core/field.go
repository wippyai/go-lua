package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

type namedMemberLookup[T any] func(typ.Type, int) (T, bool)

type namedMemberResolver[T any] struct {
	special      func(typ.Type) (T, bool)
	optional     func(*typ.Optional, int, namedMemberLookup[T]) (T, bool)
	union        func(*typ.Union, int, namedMemberLookup[T]) (T, bool)
	intersection func(*typ.Intersection, int, namedMemberLookup[T]) (T, bool)
	visit        func(typ.Type, int, namedMemberLookup[T]) (T, bool)
}

func (resolver namedMemberResolver[T]) lookup(t typ.Type, depth int) (T, bool) {
	var zero T
	if stopDepth(t, depth) {
		return zero, false
	}
	if resolver.special != nil {
		if result, ok := resolver.special(t); ok {
			return result, true
		}
	}

	next := resolver.lookup
	switch current := t.(type) {
	case *typ.TypeParam:
		if current.Constraint == nil {
			return zero, false
		}
		return next(current.Constraint, depth+1)
	case *typ.Recursive:
		if current.Body == nil || current.Body == current {
			return zero, false
		}
		return next(current.Body, depth+1)
	case *typ.Alias:
		return next(current.Target, depth+1)
	case *typ.Instantiated:
		resolved, err := ResolveInstantiated(current)
		if err != nil {
			return zero, false
		}
		return next(resolved, depth+1)
	case *typ.Optional:
		if resolver.optional != nil {
			return resolver.optional(current, depth, next)
		}
	case *typ.Union:
		if resolver.union != nil {
			return resolver.union(current, depth, next)
		}
	case *typ.Intersection:
		if resolver.intersection != nil {
			return resolver.intersection(current, depth, next)
		}
	}
	return resolver.visit(t, depth, next)
}

// Field resolves a field type on a container without using an engine.
//
// This is the pure, non-memoized version of field lookup. It traverses the
// type structure to find the named field, handling:
//   - Record fields (direct lookup)
//   - Map key-value pairs (if key accepts string literal)
//   - Interface methods
//   - Union types (field must exist in all members)
//   - Intersection types (field from any member)
//   - __index metamethod fallback
//
// Returns (nil, false) if the field does not exist.
func Field(t typ.Type, name string) (typ.Type, bool) {
	return fieldDepth(t, name, 0)
}

// HasField returns true if the type has the given field.
//
// This is a convenience wrapper around Field that discards the type result.
func HasField(t typ.Type, name string) bool {
	_, ok := fieldDepth(t, name, 0)
	return ok
}

// fieldDepth recursively resolves field lookup with depth limiting.
// The depth parameter prevents infinite recursion on recursive types.
func fieldDepth(t typ.Type, name string, depth int) (typ.Type, bool) {
	resolver := namedMemberResolver[fieldResult]{
		special: func(t typ.Type) (fieldResult, bool) {
			ft, ok := specialAccessType(t)
			return fieldResult{t: ft, ok: ok}, ok
		},
		optional: func(o *typ.Optional, depth int, lookup namedMemberLookup[fieldResult]) (fieldResult, bool) {
			if o == nil {
				return fieldResult{}, false
			}
			ft, ok := lookup(o.Inner, depth+1)
			if !ok {
				return fieldResult{}, false
			}
			return fieldResult{t: typ.NewOptional(ft.t), ok: true}, true
		},
		union: func(u *typ.Union, depth int, lookup namedMemberLookup[fieldResult]) (fieldResult, bool) {
			ft, ok := fieldInUnion(u, name, depth, func(t typ.Type, depth int) (typ.Type, bool) {
				result, ok := lookup(t, depth)
				return result.t, ok
			})
			return fieldResult{t: ft, ok: ok}, ok
		},
		intersection: func(i *typ.Intersection, depth int, lookup namedMemberLookup[fieldResult]) (fieldResult, bool) {
			ft, ok := fieldInIntersection(i, name, depth, func(t typ.Type, depth int) (typ.Type, bool) {
				result, ok := lookup(t, depth)
				return result.t, ok
			})
			return fieldResult{t: ft, ok: ok}, ok
		},
		visit: func(t typ.Type, depth int, lookup namedMemberLookup[fieldResult]) (fieldResult, bool) {
			result := typ.Visit(t, typ.Visitor[fieldResult]{
				Record: func(r *typ.Record) fieldResult {
					ft, ok := fieldInRecord(r, name)
					return fieldResult{t: ft, ok: ok}
				},
				Map: func(m *typ.Map) fieldResult {
					key := typ.LiteralString(name)
					if subtype.IsSubtype(key, m.Key) {
						if m.Value == nil {
							return fieldResult{t: typ.Nil, ok: true}
						}
						if m.InferredPresence && !m.ExplicitNilWrite {
							return fieldResult{t: m.Value, ok: true}
						}
						// Map field access behaves like index with string key (missing keys return nil).
						return fieldResult{t: typ.NewOptional(m.Value), ok: true}
					}
					return fieldResult{}
				},
				Interface: func(i *typ.Interface) fieldResult {
					ft, ok := fieldInInterface(i, name)
					return fieldResult{t: ft, ok: ok}
				},
				Generic: func(g *typ.Generic) fieldResult {
					result, ok := lookup(g.Body, depth+1)
					return fieldResult{t: result.t, ok: ok}
				},
				Default: func(t typ.Type) fieldResult {
					ft, ok := fieldOnSpecial(t, name)
					return fieldResult{t: ft, ok: ok}
				},
			})
			return result, result.ok
		},
	}
	result, ok := resolver.lookup(t, depth)
	return result.t, ok
}

// fieldInRecord looks up a field in a record type.
// Entry point for record-specific lookup with depth tracking.
func fieldInRecord(r *typ.Record, name string) (typ.Type, bool) {
	return fieldInRecordDepth(r, name, 0)
}

// fieldInRecordDepth resolves a field in a record with multiple fallback strategies.
//
// Resolution order:
//  1. Direct field lookup in the record's Fields slice
//  2. Map component fallback: if record has MapKey/MapValue and the string
//     literal key is a subtype of MapKey, returns Optional(MapValue)
//  3. Metatable __index lookup: if record has a metatable with __index,
//     recursively searches there (table) or returns return type (function)
//  4. Open record fallback: nil for a complete record, unknown for any other
//     open record
//
// Returns (nil, false) for closed records without the field.
func fieldInRecordDepth(r *typ.Record, name string, depth int) (typ.Type, bool) {
	if stopDepth(r, depth) {
		return nil, false
	}

	// Direct field lookup
	if f := r.GetField(name); f != nil {
		if f.Optional && !f.InferredPresence {
			return typ.NewOptional(f.Type), true
		}
		return f.Type, true
	}

	// Map component fallback: if record has map component and the literal string key
	// is a subtype of MapKey, return Optional(MapValue)
	if r.HasMapComponent() {
		key := typ.LiteralString(name)
		if subtype.IsSubtype(key, r.MapKey) {
			if r.MapInferredPresence && !r.MapExplicitNilWrite {
				return r.MapValue, true
			}
			return typ.NewOptional(r.MapValue), true
		}
	}

	// Check __index metamethod fallback
	if r.Metatable == nil {
		if r.Open {
			return openRecordAbsentField(r), true
		}
		return nil, false
	}

	indexMeta, ok := fieldDepth(r.Metatable, "__index", depth+1)
	if !ok {
		if r.Open {
			return openRecordAbsentField(r), true
		}
		return nil, false
	}

	// __index can be a table or a function
	res := typ.Visit(indexMeta, typ.Visitor[fieldResult]{
		Record: func(r *typ.Record) fieldResult {
			// __index is a table - look up field there recursively
			ft, ok := fieldInRecordDepth(r, name, depth+1)
			return fieldResult{t: ft, ok: ok}
		},
		Function: func(fn *typ.Function) fieldResult {
			// __index is a function - returns any (we can't know what it returns)
			if len(fn.Returns) > 0 {
				return fieldResult{t: fn.Returns[0], ok: true}
			}

			return fieldResult{t: typ.Unknown, ok: true}
		},
		Default: func(t typ.Type) fieldResult {
			// __index could be any type with fields
			ft, ok := fieldDepth(t, name, depth+1)
			return fieldResult{t: ft, ok: ok}
		},
	})
	return res.t, res.ok
}

// openRecordAbsentField is the value of a field an open record lacks. A
// complete record lists every field its table holds, so the field is nil; any
// other open record describes its value partially, so the field is unknown.
func openRecordAbsentField(r *typ.Record) typ.Type {
	if r.Complete {
		return typ.Nil
	}
	return typ.Unknown
}

// fieldInInterface looks up a method name in an interface type.
// Interfaces expose methods as fields for consistency with record syntax.
func fieldInInterface(i *typ.Interface, name string) (typ.Type, bool) {
	for _, m := range i.Methods {
		if m.Name == name {
			return m.Type, true
		}
	}

	return nil, false
}

// fieldInUnion resolves a field across all union members.
// For a union A | B, field access t.name behaves as:
//   - if all members expose the field, result is union of member field types
//   - if some table-like members miss the field, result is optional(union(...))
//   - if the union contains nil, missing members contribute nil (gradual access)
//   - otherwise, if a non-table-like member misses the field, lookup fails
//
// Nil-bearing unions retain gradual field projection; other unions only allow
// missing fields on table-like members.
func fieldInUnion(u *typ.Union, name string, depth int, lookup func(typ.Type, int) (typ.Type, bool)) (typ.Type, bool) {
	var types []typ.Type
	missingFromSome := false
	containsNil := false
	for _, m := range u.Members {
		if m != nil && m.Kind() == kind.Nil {
			containsNil = true
			break
		}
	}

	for _, m := range u.Members {
		ft, ok := lookup(m, depth+1)
		if !ok {
			if containsNil || allowsMissingFieldAsNil(m, depth+1) {
				missingFromSome = true
				continue
			}
			return nil, false
		}

		types = append(types, ft)
	}

	if len(types) == 0 {
		return nil, false
	}

	out := joinProjections(types...)
	if missingFromSome {
		out = typ.NewOptional(out)
	}
	return out, true
}

// fieldInIntersection resolves a field from any intersection member.
// For an intersection A & B, field access t.name succeeds if ANY member has
// the field. The result is the intersection of field types from all members
// that have the field. Returns (nil, false) if no member has the field.
func fieldInIntersection(i *typ.Intersection, name string, depth int, lookup func(typ.Type, int) (typ.Type, bool)) (typ.Type, bool) {
	// Field from ANY member
	var types []typ.Type

	for _, m := range i.Members {
		if ft, ok := lookup(m, depth+1); ok {
			types = append(types, ft)
		}
	}

	if len(types) == 0 {
		return nil, false
	}

	if len(types) == 1 {
		return types[0], true
	}

	return typ.NewIntersection(types...), true
}

// fieldOnSpecial handles special fields on primitive types.
// Currently handles string.len as a builtin property.
func fieldOnSpecial(t typ.Type, name string) (typ.Type, bool) {
	switch t.Kind() {
	case kind.String:
		if name == "len" {
			return typ.Integer, true
		}
	}

	return nil, false
}

// allowsMissingFieldAsNil reports whether missing fields are safely interpreted
// as nil for this member when resolving a field on a union.
func allowsMissingFieldAsNil(t typ.Type, depth int) bool {
	if stopDepth(t, depth) || t == nil {
		return false
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Alias: func(a *typ.Alias) bool {
			return allowsMissingFieldAsNil(a.Target, depth+1)
		},
		Instantiated: func(inst *typ.Instantiated) bool {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return false
			}
			return allowsMissingFieldAsNil(resolved, depth+1)
		},
		Generic: func(g *typ.Generic) bool {
			return allowsMissingFieldAsNil(g.Body, depth+1)
		},
		Record: func(*typ.Record) bool {
			return true
		},
		Map: func(*typ.Map) bool {
			return true
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}
