package core

import (
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

type indexResult struct {
	t  typ.Type
	ok bool
}

// indexDepth recursively resolves index operations with depth limiting.
// Handles various container types and propagates through wrappers.
func indexDepth(t, keyType typ.Type, depth int) (typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}

	if t.Kind() == kind.Any {
		return typ.Any, true
	}

	if t.Kind() == kind.Unknown {
		return typ.Unknown, true
	}

	res := typ.Visit(t, typ.Visitor[indexResult]{
		Array: func(a *typ.Array) indexResult {
			if keyType != nil && isNumeric(keyType) {
				if a.Element == nil {
					return indexResult{t: typ.Nil, ok: true}
				}
				return indexResult{t: a.Element, ok: true}
			}
			return indexResult{}
		},
		Map: func(m *typ.Map) indexResult {
			if keyType == nil {
				return indexResult{}
			}

			if keyType.Kind() == kind.Any || keyType.Kind() == kind.Unknown {
				if m.Value == nil {
					return indexResult{}
				}
				return indexResult{t: typ.NewOptional(m.Value), ok: true}
			}

			if subtype.IsSubtype(keyType, m.Key) {
				if m.Value == nil {
					return indexResult{}
				}

				// Map index returns optional because missing keys return nil in Lua
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

			return indexResult{}
		},
		Record: func(r *typ.Record) indexResult {
			// Map component takes priority for index access
			if r.HasMapComponent() && keyType != nil {
				if keyType.Kind() == kind.Any || keyType.Kind() == kind.Unknown || subtype.IsSubtype(keyType, r.MapKey) {
					return indexResult{t: typ.NewOptional(r.MapValue), ok: true}
				}
			}

			if len(r.Fields) == 0 && !r.HasMapComponent() {
				return indexResult{t: typ.Nil, ok: true}
			}
			// String key for record field access
			if lit, ok := keyType.(*typ.Literal); ok && lit.Base == kind.String {
				fieldType, found := fieldInRecord(r, lit.Value.(string))
				return indexResult{t: fieldType, ok: found}
			}
			// Union of string literals: look up each field and union the results
			if union, ok := keyType.(*typ.Union); ok {
				var resultTypes []typ.Type
				allLiterals := true
				for _, m := range union.Members {
					lit, isLit := m.(*typ.Literal)
					if !isLit || lit.Base != kind.String {
						allLiterals = false
						break
					}
					fieldType, found := fieldInRecord(r, lit.Value.(string))
					if found && fieldType != nil {
						resultTypes = append(resultTypes, fieldType)
					}
				}
				if allLiterals && len(resultTypes) > 0 {
					return indexResult{t: typ.NewUnion(resultTypes...), ok: true}
				}
			}
			// Unknown string returns optional union of all field types
			if keyType.Kind() == kind.String {
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
			et, ok := indexOnSpecial(t, keyType)
			return indexResult{t: et, ok: ok}
		},
	})
	return res.t, res.ok
}

// indexOnSpecial handles index access on special types (any, unknown, never).
// These types have defined behavior regardless of key type.
func indexOnSpecial(t, _ typ.Type) (typ.Type, bool) {
	switch t.Kind() {
	case kind.Any:
		return typ.Any, true
	case kind.Unknown:
		return typ.Unknown, true
	case kind.Never:
		return typ.Never, true
	default:
		return nil, false
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
