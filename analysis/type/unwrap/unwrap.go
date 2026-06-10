// Package unwrap provides shared type unwrapping and predicate operations.
//
// These are pure operations on types that do not depend on subtype checking.
package unwrap

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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

// RecordAliasOnly follows raw Alias.Target links and returns the final record.
//
// This intentionally does not unwrap annotations or use Alias.UnaliasedTarget.
func RecordAliasOnly(t typ.Type) *typ.Record {
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	rec, _ := t.(*typ.Record)
	return rec
}

// RecordUnaliased follows Alias.UnaliasedTarget links and returns the final record.
//
// This intentionally does not unwrap annotations.
func RecordUnaliased(t typ.Type) *typ.Record {
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		t = a.UnaliasedTarget()
	}
	rec, _ := t.(*typ.Record)
	return rec
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

// IsBuiltinTableTop reports whether t is the builtin "table" top marker type.
//
// The checker models `table` as an interface named "table" with no methods.
// This marker means "some table-like shape", not a closed interface.
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
