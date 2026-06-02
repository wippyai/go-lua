// Package unwrap provides type unwrapping, extraction, and predicate operations.
//
// These are pure operations on types that do not depend on subtype checking.
// For operations requiring subtype checking, see types/query/core.
package unwrap

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// Underlying returns the underlying type by unwrapping transparent wrappers.
// Unwraps: Alias, Optional (to inner type).
// Does NOT unwrap: Instantiated (requires type substitution), Union, Ref.
func Underlying(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = transparent(t)
		switch tt := t.(type) {
		case nil:
			return nil
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Optional:
			next := tt.Inner
			if next == nil || next == t {
				return next
			}
			t = next
		default:
			return t
		}
	}
	return nil
}

// Alias unwraps only Alias wrappers, preserving Optional.
func Alias(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = transparent(t)
		alias, ok := t.(*typ.Alias)
		if !ok {
			return t
		}
		next := alias.UnaliasedTarget()
		if next == nil || next == t {
			return next
		}
		t = next
	}
	return nil
}

// Optional unwraps Optional to get the inner non-nil type.
// Also unwraps Alias. Returns nil if type is nil or Nil.
func Optional(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = transparent(t)
		switch tt := t.(type) {
		case nil:
			return nil
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Optional:
			next := tt.Inner
			if next == nil || next == t {
				return next
			}
			t = next
		default:
			return t
		}
	}
	return nil
}

// IsOptionalLike returns true if the type is Optional or contains nil.
func IsOptionalLike(t typ.Type) bool {
	if t == nil {
		return true
	}

	u := Alias(t)
	if u == nil {
		return true
	}
	return typ.Visit(u, typ.Visitor[bool]{
		Optional: func(*typ.Optional) bool {
			return true
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if IsOptionalLike(m) {
					return true
				}
			}

			return false
		},
		Default: func(t typ.Type) bool {
			k := t.Kind()
			return k == kind.Nil || k.IsPlaceholder()
		},
	})
}

// IsSingleton returns true if the type represents exactly one value (nil or a literal).
func IsSingleton(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = Alias(t)
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Literal:
		return true
	}
	return false
}

// IsEmptyRecord returns true if t is a record with no fields and no map component.
func IsEmptyRecord(t typ.Type) bool {
	if t == nil {
		return false
	}
	rec, ok := t.(*typ.Record)
	return ok && len(rec.Fields) == 0 && len(rec.StaticMembers) == 0 && !rec.HasMapComponent()
}

// IsContainer returns true if t is an array, map, tuple, or record.
func IsContainer(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Array, kind.Map, kind.ReadonlyMap, kind.Tuple, kind.Record:
		return true
	}
	return false
}

// IsBuiltinTableTop reports whether t is the builtin "table" top marker type.
//
// The checker models `table` as an interface named "table" with no methods.
// This marker means "some Lua table shape", not a closed interface.
func IsBuiltinTableTop(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = Alias(t)
	if t == nil {
		return false
	}
	iface, ok := t.(*typ.Interface)
	return ok && iface.Name == "table" && len(iface.Methods) == 0
}

// Function extracts a Function type, unwrapping Alias and Optional.
func Function(t typ.Type) *typ.Function {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = transparent(t)
		switch tt := t.(type) {
		case nil:
			return nil
		case *typ.Function:
			return tt
		case *typ.Optional:
			next := tt.Inner
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Recursive:
			next := tt.Body
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return nil
			}
			t = next
		default:
			return nil
		}
	}
	return nil
}

// Record extracts a Record type, unwrapping Alias and Optional.
func Record(t typ.Type) *typ.Record {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = transparent(t)
		switch tt := t.(type) {
		case nil:
			return nil
		case *typ.Record:
			return tt
		case *typ.Recursive:
			next := tt.Body
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Optional:
			next := tt.Inner
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Instantiated:
			next := subst.ExpandInstantiated(tt)
			if next == nil || next == t {
				return nil
			}
			t = next
		default:
			return nil
		}
	}
	return nil
}

// Union extracts a Union type, unwrapping Alias and Optional.
func Union(t typ.Type) *typ.Union {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = transparent(t)
		switch tt := t.(type) {
		case nil:
			return nil
		case *typ.Union:
			return tt
		case *typ.Recursive:
			next := tt.Body
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Optional:
			next := tt.Inner
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Instantiated:
			next := subst.ExpandInstantiated(tt)
			if next == nil || next == t {
				return nil
			}
			t = next
		default:
			return nil
		}
	}
	return nil
}

// IsLiteralString returns true if the type is a string literal.
func IsLiteralString(t typ.Type) bool {
	unwrapped := Alias(t)
	if lit, ok := unwrapped.(*typ.Literal); ok {
		_, isStr := lit.Value.(string)
		return isStr
	}
	return false
}

// ToKind unwraps a type until the requested kind is found.
// Follows aliases to find the underlying type of the specified kind.
// Returns nil if the requested kind is not found or if aliases form a cycle.
func ToKind(t typ.Type, k kind.Kind) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		t = transparent(t)
		if t == nil {
			return nil
		}
		if t.Kind() == k {
			return t
		}
		switch tt := t.(type) {
		case *typ.Recursive:
			next := tt.Body
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return nil
			}
			t = next
		default:
			return nil
		}
	}
	return nil
}

// IsNilType returns true if the type is exactly nil.
func IsNilType(t typ.Type) bool {
	return t != nil && t.Kind() == kind.Nil
}

// Instantiated expands an Instantiated type to its structural form.
// Returns the expanded type if t is an Instantiated and expansion produces
// a different type. Returns t unchanged otherwise.
func Instantiated(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	inst, ok := t.(*typ.Instantiated)
	if !ok {
		return t
	}
	expanded := subst.ExpandInstantiated(inst)
	if expanded == nil || expanded == t {
		return t
	}
	return expanded
}

func transparent(t typ.Type) typ.Type {
	for {
		annotated, ok := t.(*typ.Annotated)
		if !ok {
			return t
		}
		if annotated.Inner == nil || annotated.Inner == t {
			return t
		}
		t = annotated.Inner
	}
}
