// Package unwrap provides the public wrapper contract shared by type analyses.
//
// The helpers here do not change type behavior; they only define how callers
// peel the wrapper layers they care about:
//   - Annotated removes one annotation layer.
//   - Annotations removes all annotation layers.
//   - Alias removes aliases through transparent annotations but preserves Optional.
//   - Optional removes aliases and optionals.
//   - RecordWithAliasPolicy follows aliases and intentionally leaves annotations in place.
package unwrap

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// NormalizeNil converts typed-nil Type implementations to nil.
func NormalizeNil(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	v := reflect.ValueOf(t)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}
	return t
}

// Annotated unwraps a single Annotated layer and returns the inner type.
func Annotated(t typ.Type) typ.Type {
	if a, ok := t.(*typ.Annotated); ok {
		if a.Inner == nil {
			return typ.Unknown
		}
		return a.Inner
	}
	return t
}

// Annotations strips every Annotated wrapper and returns the first non-annotated type.
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

// Alias unwraps Alias wrappers, first through transparent annotations, and
// preserves Optional wrappers.
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

// Optional unwraps Alias and Optional wrappers to get the first non-optional type.
// It returns nil for nil inputs; typ.Nil remains typ.Nil as the nil-like sentinel.
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

// IsOptionalLike returns true when t is nil-like, optional, or a union that
// contains a nil-like member. Aliases are resolved before the check.
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
