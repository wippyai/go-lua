// Package runtimekindof is the canonical mapping between static types and Lua
// runtime-kind evidence. It is a leaf shared by the typevalue projection and the
// typewitness axis reducer so the type-to-runtime-kind logic has exactly one
// implementation. It sits below typevalue (which delegates to it) and may be
// imported by axis packages; the runtimekind axis itself stays free of the
// static type system.
package runtimekindof

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func canonical(t typ.Type) typ.Type {
	return unwrap.Annotated(unwrap.NormalizeNil(t))
}

// RuntimeKindFromType returns concrete Lua runtime-kind evidence for t.
func RuntimeKindFromType(t typ.Type) (runtimekind.Value, bool) {
	t = canonical(t)
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return runtimekind.Value{}, false
	}
	if typ.IsBuiltinTableTopMarker(unwrap.Alias(t)) {
		return runtimekind.Singleton(runtimekind.Table), true
	}
	switch tt := t.(type) {
	case *typ.Literal:
		switch tt.Base() {
		case kind.Boolean:
			return runtimekind.Singleton(runtimekind.Boolean), true
		case kind.Integer, kind.Number:
			return runtimekind.Singleton(runtimekind.Number), true
		case kind.String:
			return runtimekind.Singleton(runtimekind.String), true
		default:
			return runtimekind.Value{}, false
		}
	case *typ.Optional:
		return RuntimeKindFromType(tt.Inner)
	case *typ.Union:
		var out runtimekind.Value
		seen := false
		seenNil := false
		for _, member := range tt.Members {
			member = canonical(member)
			if member != nil && member.Kind() == kind.Nil {
				seenNil = true
				continue
			}
			memberKind, ok := RuntimeKindFromType(member)
			if !ok {
				return runtimekind.Value{}, false
			}
			if seen {
				out = runtimekind.Join(out, memberKind)
			} else {
				out = memberKind
				seen = true
			}
		}
		if seen {
			return out, true
		}
		if seenNil {
			return runtimekind.Singleton(runtimekind.Nil), true
		}
		return runtimekind.Value{}, false
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return runtimekind.Value{}, false
		}
		return RuntimeKindFromType(expanded)
	case *typ.Alias:
		target := tt.UnaliasedTarget()
		if target == nil || target == t {
			return runtimekind.Value{}, false
		}
		return RuntimeKindFromType(target)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return runtimekind.Value{}, false
		}
		return RuntimeKindFromType(tt.Body)
	default:
		switch t.Kind() {
		case kind.Nil:
			return runtimekind.Singleton(runtimekind.Nil), true
		case kind.Boolean:
			return runtimekind.Singleton(runtimekind.Boolean), true
		case kind.Number, kind.Integer:
			return runtimekind.Singleton(runtimekind.Number), true
		case kind.String:
			return runtimekind.Singleton(runtimekind.String), true
		case kind.Function:
			return runtimekind.Singleton(runtimekind.Function), true
		case kind.Record, kind.Array, kind.Tuple, kind.Map, kind.ReadonlyMap:
			return runtimekind.Singleton(runtimekind.Table), true
		default:
			return runtimekind.Value{}, false
		}
	}
}

// RestrictTypeToRuntimeKind keeps the union members of t whose runtime kind is
// covered by allowed and drops members whose kind allowed excludes. Members
// whose runtime kind cannot be determined are kept, so the result is never an
// unsound under-approximation. The second return reports whether a strict
// narrowing occurred; an empty result is typ.Never (the value is unreachable).
func RestrictTypeToRuntimeKind(t typ.Type, allowed runtimekind.Value) (typ.Type, bool) {
	union, ok := unwrap.Alias(canonical(t)).(*typ.Union)
	if !ok {
		return t, false
	}
	out := make([]typ.Type, 0, len(union.Members))
	for _, member := range union.Members {
		if mk, ok := RuntimeKindFromType(member); ok && !allowed.Covers(mk) {
			continue
		}
		out = append(out, member)
	}
	if len(out) == len(union.Members) {
		return t, false
	}
	if len(out) == 0 {
		return typ.Never, true
	}
	return normalize.UnionForEvidence(out...), true
}
