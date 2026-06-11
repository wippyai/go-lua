// Package unwrap provides shared type unwrapping and predicate operations.
//
// These are pure operations on types that do not depend on subtype checking.
package unwrap

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Annotated unwraps a single Annotated layer.
func Annotated(t typ.Type) typ.Type {
	if a, ok := t.(*typ.Annotated); ok {
		if a.Inner == nil {
			return typ.Unknown
		}
		return a.Inner
	}
	return t
}

// Annotations strips all Annotated wrappers, returning the innermost
// non-Annotated type.
func Annotations(t typ.Type) typ.Type {
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
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

// RecordAliasPolicy selects how record unwrapping follows Alias nodes.
type RecordAliasPolicy int

const (
	// RecordAliasTarget follows the current Alias.Target chain.
	//
	// Use this when callers intentionally observe alias target mutation.
	RecordAliasTarget RecordAliasPolicy = iota

	// RecordAliasUnaliasedTarget follows Alias.UnaliasedTarget snapshots.
	//
	// Use this when callers need the flattened target captured at alias creation.
	RecordAliasUnaliasedTarget
)

// RecordWithAliasPolicy follows Alias nodes according to policy and returns
// the final record. It intentionally does not unwrap annotations.
func RecordWithAliasPolicy(t typ.Type, policy RecordAliasPolicy) *typ.Record {
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		switch policy {
		case RecordAliasUnaliasedTarget:
			t = a.UnaliasedTarget()
		default:
			t = a.Target
		}
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
