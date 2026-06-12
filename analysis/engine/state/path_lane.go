package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statepathkey "github.com/wippyai/go-lua/analysis/domain/state/pathkey"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

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
		out := s.reachable()
		out.paths = paths
		return out
	}
	paths := clonePathMap(s.paths)
	if paths == nil {
		paths = make(map[pathdom.PathKey]product.Value, 1)
	}
	paths[pathKey] = value
	out := s.reachable()
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

// InvalidatePathKeySubtree removes finite path refinements at pathKey and any
// descendant key. It returns false when pathKey is not a recognized structural
// path-key spelling.
func (s State) InvalidatePathKeySubtree(pathKey pathdom.PathKey) (State, bool) {
	if s.pathsTop {
		panic("state: cannot invalidate path subtree in top path lane")
	}
	paths, changed, ok := statepathkey.DeleteSubtree(s.paths, pathKey)
	if !ok {
		return s, false
	}
	if !changed {
		return s, true
	}
	out := s
	out.paths = paths
	return out, true
}

// InvalidatePathKeyDescendants removes finite path refinements below pathKey
// while preserving the exact pathKey refinement. It returns false when pathKey
// is not a recognized structural path-key spelling.
func (s State) InvalidatePathKeyDescendants(pathKey pathdom.PathKey) (State, bool) {
	if s.pathsTop {
		panic("state: cannot invalidate path descendants in top path lane")
	}
	paths, changed, ok := statepathkey.DeleteDescendants(s.paths, pathKey)
	if !ok {
		return s, false
	}
	if !changed {
		return s, true
	}
	out := s
	out.paths = paths
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
