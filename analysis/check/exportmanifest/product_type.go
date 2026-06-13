package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func typeList(reg *axis.Registry, values []product.Value) (typ.Type, bool) {
	if len(values) == 0 {
		return nil, false
	}
	types := make([]typ.Type, 0, len(values))
	for _, value := range values {
		t, ok := valueType(reg, value)
		if !ok {
			return nil, false
		}
		types = append(types, t)
	}
	if len(types) == 1 {
		return types[0], true
	}
	return typ.NewTuple(types...), true
}

func unionType(types []typ.Type) (typ.Type, bool) {
	switch len(types) {
	case 0:
		return nil, false
	case 1:
		if types[0] == nil {
			return nil, false
		}
		return types[0], true
	default:
		return normalize.UnionForEvidence(types...), true
	}
}

func valueType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil || product.Equal(reg, value, product.Bottom(reg)) {
		return nil, false
	}
	p := product.PresenceOf(value)
	if presence.Equal(p, presence.Absent()) {
		return typ.Nil, true
	}
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			return typeWithPresence(t, p), true
		}
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
			return typeWithPresence(t, p), true
		}
	}
	return scalarRuntimeKindType(reg, value)
}

func scalarRuntimeKindType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	members := make([]typ.Type, 0, len(kinds.Tags()))
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil:
			members = append(members, typ.Nil)
		case runtimekind.Boolean:
			members = append(members, typ.Boolean)
		case runtimekind.Number:
			members = append(members, typ.Number)
		case runtimekind.String:
			members = append(members, typ.String)
		default:
			return nil, false
		}
	}
	t, ok := unionType(members)
	if !ok {
		return nil, false
	}
	return typeWithPresence(t, product.PresenceOf(value)), true
}

func typeWithPresence(t typ.Type, p presence.Value) typ.Type {
	switch {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil
	case presence.Equal(p, presence.Maybe()):
		if !typeIncludesNil(t) {
			return typ.NewOptional(t)
		}
	case presence.Equal(p, presence.Present()):
		if withoutNil := typetable.PresentReadonlyEntryValue(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func typeIncludesNil(t typ.Type) bool {
	if t == nil {
		return false
	}
	normalized := unwrap.NormalizeNil(t)
	return (normalized != nil && normalized.Kind() == kind.Nil) || projectionHasNil(t, 0)
}

func projectionHasNil(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	t = unwrap.NormalizeNil(unwrap.Annotated(t))
	if t == nil {
		return false
	}
	if t.Kind() == kind.Nil {
		return true
	}
	switch tt := t.(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range tt.Members {
			if projectionHasNil(member, depth+1) {
				return true
			}
		}
	}
	return false
}
