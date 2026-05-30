package core

import (
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

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
	return fieldLookupEnv{}.lookup(t, name, depth).materialize()
}

func (e *Engine) fieldDepth(ctx *db.QueryContext, t typ.Type, name string, depth int) (typ.Type, bool) {
	return fieldLookupEnv{engine: e, ctx: ctx}.lookup(t, name, depth).materialize()
}

type fieldLookupResult struct {
	t       typ.Type
	ok      bool
	nilable bool
}

func (r fieldLookupResult) materialize() (typ.Type, bool) {
	if !r.ok {
		return nil, false
	}
	if r.nilable {
		return typ.NewOptional(r.t), true
	}
	return r.t, true
}

type fieldLookupEnv struct {
	engine *Engine
	ctx    *db.QueryContext
}

func (env fieldLookupEnv) lookupChild(t typ.Type, name string, depth int) fieldLookupResult {
	if t == nil {
		return fieldLookupResult{}
	}
	if env.engine != nil && env.engine.fieldQ != nil && env.ctx != nil && env.ctx.DB() != nil {
		res := env.engine.fieldQ.Get(env.ctx, fieldKey{t: internTypeRef(env.ctx, t), name: name})
		return fieldLookupResult{t: res.t, ok: res.ok}
	}
	return env.lookup(t, name, depth)
}

func (env fieldLookupEnv) depth(t typ.Type, name string, depth int) (typ.Type, bool) {
	return env.lookup(t, name, depth).materialize()
}

func (env fieldLookupEnv) lookup(t typ.Type, name string, depth int) fieldLookupResult {
	if stopDepth(t, depth) {
		return fieldLookupResult{}
	}
	if top, ok := specialAccessType(t); ok {
		return fieldLookupResult{t: top, ok: true}
	}

	return typ.Visit(t, typ.Visitor[fieldLookupResult]{
		TypeParam: func(tp *typ.TypeParam) fieldLookupResult {
			if tp.Constraint != nil {
				return env.lookupChild(tp.Constraint, name, depth+1)
			}
			return fieldLookupResult{}
		},
		Record: func(r *typ.Record) fieldLookupResult {
			return env.fieldInRecordLookup(r, name, depth)
		},
		Map: func(m *typ.Map) fieldLookupResult {
			key := typ.LiteralString(name)
			if subtype.IsSubtype(key, m.Key) {
				if m.Value == nil {
					return fieldLookupResult{t: typ.Nil, ok: true}
				}
				// Map field access behaves like index with string key (missing keys return nil).
				return fieldLookupResult{t: m.Value, ok: true, nilable: true}
			}
			return fieldLookupResult{}
		},
		Interface: func(i *typ.Interface) fieldLookupResult {
			ft, ok := fieldInInterface(i, name)
			return fieldLookupResult{t: ft, ok: ok}
		},
		Union: func(u *typ.Union) fieldLookupResult {
			return env.fieldInUnionLookup(u, name, depth)
		},
		Intersection: func(i *typ.Intersection) fieldLookupResult {
			ft, ok := env.fieldInIntersection(i, name, depth)
			return fieldLookupResult{t: ft, ok: ok}
		},
		Optional: func(o *typ.Optional) fieldLookupResult {
			if o == nil {
				return fieldLookupResult{}
			}
			res := env.lookupChild(o.Inner, name, depth+1)
			if res.ok {
				res.nilable = true
			}
			return res
		},
		Recursive: func(rec *typ.Recursive) fieldLookupResult {
			if rec.Body == nil || rec.Body == rec {
				return fieldLookupResult{}
			}
			return env.lookupChild(rec.Body, name, depth+1)
		},
		Alias: func(a *typ.Alias) fieldLookupResult {
			return env.lookupChild(a.Target, name, depth+1)
		},
		Instantiated: func(inst *typ.Instantiated) fieldLookupResult {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return fieldLookupResult{}
			}

			return env.lookupChild(resolved, name, depth+1)
		},
		Generic: func(g *typ.Generic) fieldLookupResult {
			return env.lookupChild(g.Body, name, depth+1)
		},
		Default: func(t typ.Type) fieldLookupResult {
			ft, ok := fieldOnSpecial(t, name)
			return fieldLookupResult{t: ft, ok: ok}
		},
	})
}

// fieldInRecordDepth resolves a field in a record with multiple fallback strategies.
//
// Resolution order:
//  1. Direct field lookup in the record's Fields slice
//  2. Map component fallback: if record has MapKey/MapValue and the string
//     literal key is a subtype of MapKey, returns Optional(MapValue)
//  3. Metatable __index lookup: if record has a metatable with __index,
//     recursively searches there (table) or returns return type (function)
//  4. Open record fallback: returns Unknown for open records (allows any field)
//
// Returns (nil, false) for closed records without the field.
func fieldInRecordDepth(r *typ.Record, name string, depth int) (typ.Type, bool) {
	return fieldLookupEnv{}.fieldInRecordLookup(r, name, depth).materialize()
}

func (env fieldLookupEnv) fieldInRecordLookup(r *typ.Record, name string, depth int) fieldLookupResult {
	if stopDepth(r, depth) {
		return fieldLookupResult{}
	}

	// Direct field lookup
	if f := r.GetField(name); f != nil {
		if f.Optional {
			return fieldLookupResult{t: f.Type, ok: true, nilable: true}
		}
		return fieldLookupResult{t: f.Type, ok: true}
	}

	// Map component fallback: if record has map component and the literal string key
	// is a subtype of MapKey, return Optional(MapValue)
	if r.HasMapComponent() {
		key := typ.LiteralString(name)
		if subtype.IsSubtype(key, r.MapKey) {
			return fieldLookupResult{t: r.MapValue, ok: true, nilable: true}
		}
	}

	// Check __index metamethod fallback
	if r.Metatable == nil || typ.IsMetatableUnconstrained(r.Metatable) {
		if r.Open {
			return fieldLookupResult{t: typ.Unknown, ok: true}
		}
		return fieldLookupResult{}
	}

	indexMeta, ok := env.depth(r.Metatable, "__index", depth+1)
	if !ok {
		if r.Open {
			return fieldLookupResult{t: typ.Unknown, ok: true}
		}
		return fieldLookupResult{}
	}

	// __index can be a table or a function
	return typ.Visit(indexMeta, typ.Visitor[fieldLookupResult]{
		Record: func(r *typ.Record) fieldLookupResult {
			// __index is a table - look up field there recursively
			return env.fieldInRecordLookup(r, name, depth+1)
		},
		Function: func(fn *typ.Function) fieldLookupResult {
			// __index is a function - returns any (we can't know what it returns)
			if len(fn.Returns) > 0 {
				return fieldLookupResult{t: fn.Returns[0], ok: true}
			}

			return fieldLookupResult{t: typ.Unknown, ok: true}
		},
		Default: func(t typ.Type) fieldLookupResult {
			// __index could be any type with fields
			return env.lookupChild(t, name, depth+1)
		},
	})
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

// fieldInUnionLookup resolves a field across all union members.
// For a union A | B, field access t.name behaves as:
//   - if all members expose the field, result is union of member field types
//   - if some table-like members miss the field, result is optional(union(...))
//   - if any non-table-like member misses the field, lookup fails
//
// This matches Lua table semantics for partial record unions while remaining
// sound for non-table members (where field access would be invalid).
func (env fieldLookupEnv) fieldInUnionLookup(u *typ.Union, name string, depth int) fieldLookupResult {
	members := typ.CoalesceProductUnionMembers(u.Members)
	var out typ.Type
	nilable := false

	for _, m := range members {
		res := env.lookupChild(m, name, depth+1)
		if !res.ok {
			if missingFieldReadsNilDepth(m, depth+1) {
				nilable = true
				continue
			}
			return fieldLookupResult{}
		}

		if res.nilable {
			nilable = true
		}
		out = typ.JoinRecordFieldSlot(name, out, res.t)
	}

	if out == nil {
		if nilable {
			return fieldLookupResult{t: typ.Nil, ok: true}
		}
		return fieldLookupResult{}
	}

	out = typ.CoalesceProductUnion(out)
	return fieldLookupResult{t: out, ok: true, nilable: nilable}
}

// fieldInIntersection resolves a field from any intersection member.
// For an intersection A & B, field access t.name succeeds if ANY member has
// the field. The result is the intersection of field types from all members
// that have the field. Returns (nil, false) if no member has the field.
func (env fieldLookupEnv) fieldInIntersection(i *typ.Intersection, name string, depth int) (typ.Type, bool) {
	// Field from ANY member
	var types []typ.Type

	for _, m := range i.Members {
		if ft, ok := env.depth(m, name, depth+1); ok {
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

// MissingFieldReadsNil reports whether a missing field read on t has defined
// Lua table semantics: it produces nil instead of raising an indexing error.
// Field still returns false for a missing field on precise table shapes so
// strict value reads can report likely typos; union field lookup and probe
// diagnostics use this to treat absent fields as nil for table variants.
func MissingFieldReadsNil(t typ.Type) bool {
	return missingFieldReadsNilDepth(t, 0)
}

func missingFieldReadsNilDepth(t typ.Type, depth int) bool {
	if stopDepth(t, depth) || t == nil {
		return false
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Alias: func(a *typ.Alias) bool {
			return missingFieldReadsNilDepth(a.Target, depth+1)
		},
		Instantiated: func(inst *typ.Instantiated) bool {
			resolved, err := ResolveInstantiated(inst)
			if err != nil {
				return false
			}
			return missingFieldReadsNilDepth(resolved, depth+1)
		},
		Generic: func(g *typ.Generic) bool {
			return missingFieldReadsNilDepth(g.Body, depth+1)
		},
		TypeParam: func(tp *typ.TypeParam) bool {
			return tp.Constraint != nil && missingFieldReadsNilDepth(tp.Constraint, depth+1)
		},
		Recursive: func(rec *typ.Recursive) bool {
			return rec.Body != nil && rec.Body != rec && missingFieldReadsNilDepth(rec.Body, depth+1)
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if !missingFieldReadsNilDepth(m, depth+1) {
					return false
				}
			}
			return len(u.Members) > 0
		},
		Intersection: func(i *typ.Intersection) bool {
			for _, m := range i.Members {
				if missingFieldReadsNilDepth(m, depth+1) {
					return true
				}
			}
			return false
		},
		Record: func(*typ.Record) bool {
			return true
		},
		Map: func(*typ.Map) bool {
			return true
		},
		Array: func(*typ.Array) bool {
			return true
		},
		Tuple: func(*typ.Tuple) bool {
			return true
		},
		Interface: func(*typ.Interface) bool {
			return true
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}
