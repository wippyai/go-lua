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
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// NormalizeNil converts typed-nil Type implementations to nil.
func NormalizeNil(t typ.Type) typ.Type {
	return typ.NormalizeNil(t)
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

// AnnotatedOrNil unwraps a single Annotated layer but returns nil for nil input.
func AnnotatedOrNil(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	return Annotated(t)
}

// Annotations strips every Annotated wrapper and returns the first non-annotated type.
func Annotations(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		ann, ok := t.(*typ.Annotated)
		if !ok {
			return t
		}
		if ann.Inner == nil || ann.Inner == t {
			return t
		}
		t = ann.Inner
	}
	return t
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
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		var next typ.Type
		switch policy {
		case RecordAliasUnaliasedTarget:
			next = a.UnaliasedTarget()
		default:
			next = a.Target
		}
		if next == nil || next == t {
			return nil
		}
		t = next
	}
	if _, ok := t.(*typ.Alias); ok {
		return nil
	}
	rec, _ := t.(*typ.Record)
	return rec
}

// IsOptionalLike returns true when t is nil-like, optional, or a union that
// contains a nil-like member. Aliases are resolved before the check.
func IsOptionalLike(t typ.Type) bool {
	return isOptionalLike(t, 0, nil)
}

func isOptionalLike(t typ.Type, depth int, active map[*typ.Union]bool) bool {
	if depth > typ.DefaultRecursionDepth {
		return false
	}
	if t == nil {
		return true
	}

	switch tt := t.(type) {
	case *typ.Annotated:
		if tt.Inner == nil {
			return true
		}
		if tt.Inner == t {
			return false
		}
		return isOptionalLike(tt.Inner, depth+1, active)
	case *typ.Alias:
		next := tt.UnaliasedTarget()
		if next == nil {
			return true
		}
		if next == t {
			return false
		}
		return isOptionalLike(next, depth+1, active)
	case *typ.Optional:
		return true
	case *typ.Union:
		if active == nil {
			active = make(map[*typ.Union]bool)
		}
		if active[tt] {
			return false
		}
		active[tt] = true
		defer delete(active, tt)
		for _, m := range tt.Members {
			if isOptionalLike(m, depth+1, active) {
				return true
			}
		}
		return false
	default:
		k := t.Kind()
		return k == kind.Nil || k.IsPlaceholder()
	}
}

func transparent(t typ.Type) typ.Type {
	return Annotations(t)
}
