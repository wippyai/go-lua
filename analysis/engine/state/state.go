package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// State carries point-local abstract values. Absence in either lane denotes
// product.Bottom for the registry used by the caller.
type State struct {
	values map[key.Value]product.Value
	paths  map[pathdom.PathKey]product.Value

	valuesTop bool
	pathsTop  bool
}

// Clone returns an independent copy of the finite lanes in s.
func (s State) Clone() State {
	return State{
		values:    cloneValueMap(s.values),
		paths:     clonePathMap(s.paths),
		valuesTop: s.valuesTop,
		pathsTop:  s.pathsTop,
	}
}

// ReadValue reads a value slot. Missing slots read as product.Bottom(reg).
func (s State) ReadValue(reg *axis.Registry, slot key.Value) product.Value {
	if slot == "" {
		return product.Bottom(reg)
	}
	if s.valuesTop {
		return product.Top()
	}
	if v, ok := s.values[slot]; ok {
		return v
	}
	return product.Bottom(reg)
}

// WriteValue returns a state with slot updated. Writing product.Bottom(reg)
// removes the finite entry so absence remains the canonical bottom spelling.
func (s State) WriteValue(reg *axis.Registry, slot key.Value, value product.Value) State {
	if slot == "" {
		return s
	}
	if s.valuesTop {
		panic("state: cannot finite-write value slot into top value lane")
	}
	valueDomain := product.Domain(reg)
	if valueDomain.Equal(value, valueDomain.Bottom()) {
		values, changed := deleteValueEntry(s.values, slot)
		if !changed {
			return s
		}
		out := s
		out.values = values
		return out
	}
	values := cloneValueMap(s.values)
	if values == nil {
		values = make(map[key.Value]product.Value, 1)
	}
	values[slot] = value
	out := s
	out.values = values
	return out
}

// UpdateValue reads slot, applies fn, and writes the transformed value.
// Transforming a finite entry to product.Bottom(reg) removes it.
func (s State) UpdateValue(reg *axis.Registry, slot key.Value, fn func(product.Value) product.Value) State {
	if slot == "" {
		return s
	}
	return s.WriteValue(reg, slot, fn(s.ReadValue(reg, slot)))
}

// ReadReturnSlot reads a non-symbol return value slot.
func (s State) ReadReturnSlot(reg *axis.Registry, index int) product.Value {
	return s.ReadValue(reg, key.ReturnSlot(index))
}

// WriteReturnSlot writes a non-symbol return value slot.
func (s State) WriteReturnSlot(reg *axis.Registry, index int, value product.Value) State {
	return s.WriteValue(reg, key.ReturnSlot(index), value)
}

// UpdateReturnSlot reads a non-symbol return value slot, applies fn, and writes
// the transformed value.
func (s State) UpdateReturnSlot(reg *axis.Registry, index int, fn func(product.Value) product.Value) State {
	return s.UpdateValue(reg, key.ReturnSlot(index), fn)
}

// ReadPathKey reads a point-local path refinement key. Missing keys read as
// product.Bottom(reg).
func (s State) ReadPathKey(reg *axis.Registry, pathKey pathdom.PathKey) product.Value {
	if pathKey == "" {
		return product.Bottom(reg)
	}
	if s.pathsTop {
		return product.Top()
	}
	if v, ok := s.paths[pathKey]; ok {
		return v
	}
	return product.Bottom(reg)
}

// WritePathKey returns a state with pathKey updated. Writing
// product.Bottom(reg) removes the finite entry.
func (s State) WritePathKey(reg *axis.Registry, pathKey pathdom.PathKey, value product.Value) State {
	if pathKey == "" {
		return s
	}
	if s.pathsTop {
		panic("state: cannot finite-write path key into top path lane")
	}
	valueDomain := product.Domain(reg)
	if valueDomain.Equal(value, valueDomain.Bottom()) {
		paths, changed := deletePathEntry(s.paths, pathKey)
		if !changed {
			return s
		}
		out := s
		out.paths = paths
		return out
	}
	paths := clonePathMap(s.paths)
	if paths == nil {
		paths = make(map[pathdom.PathKey]product.Value, 1)
	}
	paths[pathKey] = value
	out := s
	out.paths = paths
	return out
}

// UpdatePathKey reads pathKey, applies fn, and writes the transformed value.
// Transforming a finite entry to product.Bottom(reg) removes it.
func (s State) UpdatePathKey(reg *axis.Registry, pathKey pathdom.PathKey, fn func(product.Value) product.Value) State {
	if pathKey == "" {
		return s
	}
	return s.WritePathKey(reg, pathKey, fn(s.ReadPathKey(reg, pathKey)))
}

// ReadPathAt resolves path at point and reads the matching point-local key.
// It returns false when the resolver cannot produce a key, such as a
// symbol-rooted path with no visible SSA version.
func (s State) ReadPathAt(
	reg *axis.Registry,
	resolver *key.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (product.Value, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return product.Bottom(reg), false
	}
	return s.ReadPathKey(reg, pathKey), true
}

// WritePathAt resolves path at point and writes the matching point-local key.
// It returns false and leaves s unchanged when no key can be resolved.
func (s State) WritePathAt(
	reg *axis.Registry,
	resolver *key.Resolver,
	point cfg.Point,
	path pathdom.Path,
	value product.Value,
) (State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	return s.WritePathKey(reg, pathKey, value), true
}

// UpdatePathAt resolves path at point, reads the matching point-local key,
// applies fn, and writes the transformed value. It returns false and leaves s
// unchanged when no key can be resolved.
func (s State) UpdatePathAt(
	reg *axis.Registry,
	resolver *key.Resolver,
	point cfg.Point,
	path pathdom.Path,
	fn func(product.Value) product.Value,
) (State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	return s.UpdatePathKey(reg, pathKey, fn), true
}

// InvalidatePathKeySubtree removes finite path refinements at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (s State) InvalidatePathKeySubtree(pathKey pathdom.PathKey) (State, bool) {
	prefix, ok := parseStatePathKey(pathKey)
	if !ok {
		return s, false
	}
	if s.pathsTop {
		panic("state: cannot invalidate path subtree in top path lane")
	}
	paths, changed := deletePathSubtree(s.paths, prefix)
	if !changed {
		return s, true
	}
	out := s
	out.paths = paths
	return out, true
}

// InvalidatePathSubtreeAt resolves path at point and removes finite path
// refinements at the resolved key and any descendant key. It returns false and
// leaves s unchanged when no key can be resolved.
func (s State) InvalidatePathSubtreeAt(
	resolver *key.Resolver,
	point cfg.Point,
	path pathdom.Path,
) (State, bool) {
	pathKey := resolver.KeyAt(point, path)
	if pathKey == "" {
		return s, false
	}
	return s.InvalidatePathKeySubtree(pathKey)
}

// Domain builds the State lattice as the product of two pointwise map lattices
// over product.Value.
func Domain(reg *axis.Registry) lattice.Lattice[State] {
	valueDomain := product.Domain(reg)
	ops := domainOps{
		values: lift.Map[key.Value, product.Value](valueDomain),
		paths:  lift.Map[pathdom.PathKey, product.Value](valueDomain),
	}
	return lattice.Lattice[State]{
		Bottom: func() State {
			return State{}
		},
		Top: func() State {
			return State{valuesTop: true, pathsTop: true}
		},
		Equal: func(a, b State) bool {
			return ops.values.Equal(ops.valueLane(a), ops.valueLane(b)) &&
				ops.paths.Equal(ops.pathLane(a), ops.pathLane(b))
		},
		LessOrEq: func(a, b State) bool {
			return ops.values.LessOrEq(ops.valueLane(a), ops.valueLane(b)) &&
				ops.paths.LessOrEq(ops.pathLane(a), ops.pathLane(b))
		},
		Join: func(a, b State) State {
			return ops.fromLanes(
				ops.values.Join(ops.valueLane(a), ops.valueLane(b)),
				ops.paths.Join(ops.pathLane(a), ops.pathLane(b)),
			)
		},
		Widen: func(prev, next State) State {
			return ops.fromLanes(
				ops.values.Widen(ops.valueLane(prev), ops.valueLane(next)),
				ops.paths.Widen(ops.pathLane(prev), ops.pathLane(next)),
			)
		},
	}
}

type domainOps struct {
	values lattice.Lattice[map[key.Value]product.Value]
	paths  lattice.Lattice[map[pathdom.PathKey]product.Value]
}

func (o domainOps) valueLane(s State) map[key.Value]product.Value {
	if s.valuesTop {
		return o.values.Top()
	}
	return s.values
}

func (o domainOps) pathLane(s State) map[pathdom.PathKey]product.Value {
	if s.pathsTop {
		return o.paths.Top()
	}
	return s.paths
}

func (o domainOps) fromLanes(
	values map[key.Value]product.Value,
	paths map[pathdom.PathKey]product.Value,
) State {
	out := State{}
	if o.values.Equal(values, o.values.Top()) {
		out.valuesTop = true
	} else {
		out.values = values
	}
	if o.paths.Equal(paths, o.paths.Top()) {
		out.pathsTop = true
	} else {
		out.paths = paths
	}
	return out
}

func cloneValueMap(in map[key.Value]product.Value) map[key.Value]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[key.Value]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func clonePathMap(in map[pathdom.PathKey]product.Value) map[pathdom.PathKey]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func deleteValueEntry(
	in map[key.Value]product.Value,
	slot key.Value,
) (map[key.Value]product.Value, bool) {
	if _, ok := in[slot]; !ok {
		return in, false
	}
	out := make(map[key.Value]product.Value, len(in)-1)
	for k, v := range in {
		if k != slot {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func deletePathEntry(
	in map[pathdom.PathKey]product.Value,
	pathKey pathdom.PathKey,
) (map[pathdom.PathKey]product.Value, bool) {
	if _, ok := in[pathKey]; !ok {
		return in, false
	}
	out := make(map[pathdom.PathKey]product.Value, len(in)-1)
	for k, v := range in {
		if k != pathKey {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

type statePathKey struct {
	versioned bool
	sym       symbol.ID
	version   int
	segments  []segment.Segment
	stable    pathaddr.Stable
}

func parseStatePathKey(pathKey pathdom.PathKey) (statePathKey, bool) {
	sym, version, suffix, ok := key.ParsePathKey(pathKey)
	if ok && version > 0 {
		segments, ok := segment.InternFormattedSegments(suffix)
		if !ok {
			return statePathKey{}, false
		}
		return statePathKey{
			versioned: true,
			sym:       sym,
			version:   version,
			segments:  segments,
		}, true
	}
	stable, ok := pathaddr.StableFromKey(pathKey)
	if !ok {
		return statePathKey{}, false
	}
	return statePathKey{stable: stable}, true
}

func deletePathSubtree(
	in map[pathdom.PathKey]product.Value,
	prefix statePathKey,
) (map[pathdom.PathKey]product.Value, bool) {
	if len(in) == 0 {
		return in, false
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	changed := false
	for pathKey, value := range in {
		keyAddr, ok := parseStatePathKey(pathKey)
		remove := ok && keyAddr.hasPrefix(prefix)
		if remove {
			changed = true
			continue
		}
		out[pathKey] = value
	}
	if !changed {
		return in, false
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func (k statePathKey) hasPrefix(prefix statePathKey) bool {
	if k.versioned || prefix.versioned {
		return k.versioned &&
			prefix.versioned &&
			k.sym == prefix.sym &&
			k.version == prefix.version &&
			segmentsHavePrefix(k.segments, prefix.segments)
	}
	return k.stable.HasPrefix(prefix.stable)
}

func segmentsHavePrefix(segments, prefix []segment.Segment) bool {
	if len(prefix) > len(segments) {
		return false
	}
	for i := range prefix {
		if segments[i] != prefix[i] {
			return false
		}
	}
	return true
}
