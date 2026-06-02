// Package globalenv owns source-global names and typed overlays at the
// analysis-domain boundary.
//
// Raw string-keyed maps remain external contract/API vocabulary. Internal
// analysis packages should use Name and TypeOverlay until canonical lowering
// maps a source global to a graph-local symbol.
package globalenv

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// Name identifies an external source-global name before canonical symbol
// lowering. It is not a solver identity.
type Name string

func (n Name) String() string {
	return string(n)
}

// TypeBinding is one source-global name and its type.
type TypeBinding struct {
	Name Name
	Type typ.Type
}

// TypeOverlay is a deterministic, normalized set of source-global type
// bindings. It is sorted by Name and contains no empty names or nil types.
type TypeOverlay []TypeBinding

// ValueBinding is one source-global name and its abstract value.
type ValueBinding struct {
	Name  Name
	Value product.AbstractValue
}

// ValueOverlay is a deterministic, normalized set of source-global abstract
// value bindings. It is sorted by Name and contains no empty names or zero
// values. Duplicate names converge with product.CarryForward.
type ValueOverlay []ValueBinding

// Names returns the deterministic source-global names in this overlay.
func (o TypeOverlay) Names() []string {
	if len(o) == 0 {
		return nil
	}
	names := make([]string, 0, len(o))
	for _, binding := range o {
		if binding.Name == "" || binding.Type == nil {
			continue
		}
		names = append(names, binding.Name.String())
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

// TypeOverlayFromMap normalizes an external string-keyed map into the domain
// carrier. Raw maps should be admitted only at API/contract boundaries.
func TypeOverlayFromMap(env map[string]typ.Type) TypeOverlay {
	if len(env) == 0 {
		return nil
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make(TypeOverlay, 0, len(names))
	for _, name := range names {
		t := env[name]
		if name == "" || t == nil {
			continue
		}
		out = append(out, TypeBinding{Name: Name(name), Type: t})
	}
	return out
}

// ToMap projects the domain carrier back to an external string-keyed map.
func (o TypeOverlay) ToMap() map[string]typ.Type {
	if len(o) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(o))
	for _, binding := range o {
		if binding.Name == "" || binding.Type == nil {
			continue
		}
		out[binding.Name.String()] = binding.Type
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ToContractMap projects the domain carrier to the external contract map
// vocabulary. It is an alias for ToMap kept at contract boundaries.
func (o TypeOverlay) ToContractMap() map[string]typ.Type {
	return o.ToMap()
}

// Type returns the binding type for name.
func (o TypeOverlay) Type(name string) (typ.Type, bool) {
	if name == "" || len(o) == 0 {
		return nil, false
	}
	idx, ok := slices.BinarySearchFunc(o, Name(name), func(binding TypeBinding, target Name) int {
		return cmp.Compare(binding.Name, target)
	})
	if !ok {
		return nil, false
	}
	return o[idx].Type, true
}

// NormalizeTypeOverlay returns the canonical form of a source-global type overlay.
func NormalizeTypeOverlay(overlay TypeOverlay) TypeOverlay {
	if len(overlay) == 0 {
		return nil
	}
	byName := make(map[Name]typ.Type, len(overlay))
	for _, binding := range overlay {
		if binding.Name == "" || binding.Type == nil {
			continue
		}
		if existing := byName[binding.Name]; existing != nil {
			byName[binding.Name] = value.JoinPrecise(existing, binding.Type)
		} else {
			byName[binding.Name] = binding.Type
		}
	}
	if len(byName) == 0 {
		return nil
	}
	names := make([]Name, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make(TypeOverlay, 0, len(names))
	for _, name := range names {
		out = append(out, TypeBinding{Name: name, Type: byName[name]})
	}
	return out
}

// Clone returns a normalized copy of the overlay.
func (o TypeOverlay) Clone() TypeOverlay {
	return NormalizeTypeOverlay(o)
}

// MergeTypeOverlay joins two normalized overlays by source-global name.
func MergeTypeOverlay(base, overlay TypeOverlay) TypeOverlay {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	byName := make(map[Name]typ.Type, len(base)+len(overlay))
	for _, binding := range base {
		if binding.Name != "" && binding.Type != nil {
			byName[binding.Name] = binding.Type
		}
	}
	for _, binding := range overlay {
		if binding.Name == "" || binding.Type == nil {
			continue
		}
		if existing := byName[binding.Name]; existing != nil {
			byName[binding.Name] = value.JoinPrecise(existing, binding.Type)
		} else {
			byName[binding.Name] = binding.Type
		}
	}
	if len(byName) == 0 {
		return nil
	}
	names := make([]Name, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make(TypeOverlay, 0, len(names))
	for _, name := range names {
		out = append(out, TypeBinding{Name: name, Type: byName[name]})
	}
	return out
}

// OverrideTypeOverlay applies overlay over base by source-global name. Unlike
// MergeTypeOverlay, duplicate names are replaced, not joined; this models a
// context-local global rebinding at an API/env boundary.
func OverrideTypeOverlay(base, overlay TypeOverlay) TypeOverlay {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	byName := make(map[Name]typ.Type, len(base)+len(overlay))
	for _, binding := range base {
		if binding.Name != "" && binding.Type != nil {
			byName[binding.Name] = binding.Type
		}
	}
	for _, binding := range overlay {
		if binding.Name != "" && binding.Type != nil {
			byName[binding.Name] = binding.Type
		}
	}
	if len(byName) == 0 {
		return nil
	}
	names := make([]Name, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make(TypeOverlay, 0, len(names))
	for _, name := range names {
		out = append(out, TypeBinding{Name: name, Type: byName[name]})
	}
	return out
}

// ValueOverlayFromTypeMap admits an external string-keyed type map into the
// analysis-context global value carrier.
func ValueOverlayFromTypeMap(env map[string]typ.Type) ValueOverlay {
	if len(env) == 0 {
		return nil
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make(ValueOverlay, 0, len(names))
	for _, name := range names {
		t := env[name]
		if name == "" || t == nil {
			continue
		}
		av := product.FromType(t)
		if av.IsZero() {
			continue
		}
		out = append(out, ValueBinding{Name: Name(name), Value: av})
	}
	return out
}

// ValueOverlayFromValueMap admits a source-global abstract-value map into the
// deterministic value carrier.
func ValueOverlayFromValueMap(env map[Name]product.AbstractValue) ValueOverlay {
	if len(env) == 0 {
		return nil
	}
	names := make([]Name, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make(ValueOverlay, 0, len(names))
	for _, name := range names {
		av := env[name]
		if name == "" || av.IsZero() {
			continue
		}
		out = append(out, ValueBinding{Name: name, Value: av})
	}
	return out
}

// NormalizeValueOverlay returns the canonical form of a value overlay.
func NormalizeValueOverlay(overlay ValueOverlay) ValueOverlay {
	if len(overlay) == 0 {
		return nil
	}
	byName := make(map[Name]product.AbstractValue, len(overlay))
	for _, binding := range overlay {
		if binding.Name == "" || binding.Value.IsZero() {
			continue
		}
		if existing := byName[binding.Name]; !existing.IsZero() {
			byName[binding.Name] = product.CarryForward(existing, binding.Value)
		} else {
			byName[binding.Name] = binding.Value
		}
	}
	if len(byName) == 0 {
		return nil
	}
	names := make([]Name, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make(ValueOverlay, 0, len(names))
	for _, name := range names {
		out = append(out, ValueBinding{Name: name, Value: byName[name]})
	}
	return out
}

// Empty reports whether the overlay carries no usable bindings.
func (o ValueOverlay) Empty() bool {
	return len(NormalizeValueOverlay(o)) == 0
}

// Clone returns a normalized copy of the overlay.
func (o ValueOverlay) Clone() ValueOverlay {
	return NormalizeValueOverlay(o)
}

// Value returns the abstract value binding for name.
func (o ValueOverlay) Value(name string) (product.AbstractValue, bool) {
	if name == "" || len(o) == 0 {
		return product.AbstractValue{}, false
	}
	normalized := NormalizeValueOverlay(o)
	idx, ok := slices.BinarySearchFunc(normalized, Name(name), func(binding ValueBinding, target Name) int {
		return cmp.Compare(binding.Name, target)
	})
	if !ok || normalized[idx].Value.IsZero() {
		return product.AbstractValue{}, false
	}
	return normalized[idx].Value, true
}

// Type returns the projected type binding for name.
func (o ValueOverlay) Type(name string) (typ.Type, bool) {
	av, ok := o.Value(name)
	if !ok {
		return nil, false
	}
	t := av.ProjectValue()
	return t, t != nil
}

// ToTypeOverlay projects value bindings to source-global type bindings.
func (o ValueOverlay) ToTypeOverlay() TypeOverlay {
	if len(o) == 0 {
		return nil
	}
	out := make(TypeOverlay, 0, len(o))
	for _, binding := range NormalizeValueOverlay(o) {
		if binding.Name == "" || binding.Value.IsZero() {
			continue
		}
		t := binding.Value.ProjectValue()
		if t == nil {
			continue
		}
		out = append(out, TypeBinding{Name: binding.Name, Type: t})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ToTypeMap projects value bindings to the external string-keyed type map.
func (o ValueOverlay) ToTypeMap() map[string]typ.Type {
	return o.ToTypeOverlay().ToMap()
}

// MergeValueOverlay converges two analysis-context overlays by source-global
// name. This preserves the existing callback-context semantics: duplicate
// global evidence uses product.CarryForward, not source-environment override.
func MergeValueOverlay(base, overlay ValueOverlay) ValueOverlay {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	byName := make(map[Name]product.AbstractValue, len(base)+len(overlay))
	for _, binding := range NormalizeValueOverlay(base) {
		if binding.Name != "" && !binding.Value.IsZero() {
			byName[binding.Name] = binding.Value
		}
	}
	for _, binding := range NormalizeValueOverlay(overlay) {
		if binding.Name == "" || binding.Value.IsZero() {
			continue
		}
		if existing := byName[binding.Name]; !existing.IsZero() {
			byName[binding.Name] = product.CarryForward(existing, binding.Value)
		} else {
			byName[binding.Name] = binding.Value
		}
	}
	return ValueOverlayFromValueMap(byName)
}

// EqualValueOverlay reports semantic equality of two normalized value overlays.
func EqualValueOverlay(a, b ValueOverlay) bool {
	a = NormalizeValueOverlay(a)
	b = NormalizeValueOverlay(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || !product.Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}
