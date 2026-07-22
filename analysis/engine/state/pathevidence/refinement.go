package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ReadPathKey reads a point-local path refinement key. Missing keys read as
// product.Bottom(reg).
func (l Lane) ReadPathKey(reg *axis.Registry, pathKey keyspace.Key) product.Value {
	if pathKey.Kind == keyspace.KindInvalid || l.refinementsBottom {
		return product.Bottom(reg)
	}
	if v, ok := l.refinements[pathKey]; ok {
		return v
	}
	return product.Bottom(reg)
}

// WritePathKey returns a lane with pathKey updated and whether this write made
// the surrounding state reachable. Writing product.Bottom(reg) removes the
// finite entry.
func (l Lane) WritePathKey(reg *axis.Registry, pathKey keyspace.Key, value product.Value) (Lane, bool) {
	if pathKey.Kind == keyspace.KindInvalid {
		return l, false
	}
	valueDomain := product.Domain(reg)
	if valueDomain.Equal(value, valueDomain.Bottom()) {
		refinements, changed := deletePathValueEntry(l.refinements, pathKey)
		if !changed && !l.refinementsBottom {
			return l, false
		}
		out := l.Reachable()
		out.refinements = refinements
		return out, true
	}
	if !l.refinementsBottom {
		if existing, ok := l.refinements[pathKey]; ok && valueDomain.Equal(existing, value) {
			return l, false
		}
	}
	refinements := cloneLocalValueMap(l.refinements)
	if refinements == nil {
		refinements = make(map[keyspace.Key]product.Value, 1)
	}
	refinements[pathKey] = value
	out := l.Reachable()
	out.refinements = refinements
	return out, true
}

// UpdatePathKey reads pathKey, applies fn, and writes the transformed value.
func (l Lane) UpdatePathKey(reg *axis.Registry, pathKey keyspace.Key, fn func(product.Value) product.Value) (Lane, bool) {
	if pathKey.Kind == keyspace.KindInvalid {
		return l, false
	}
	return l.WritePathKey(reg, pathKey, fn(l.ReadPathKey(reg, pathKey)))
}

func deletePathValueEntry(
	in map[keyspace.Key]product.Value,
	pathKey keyspace.Key,
) (map[keyspace.Key]product.Value, bool) {
	if _, ok := in[pathKey]; !ok {
		return in, false
	}
	out := make(map[keyspace.Key]product.Value, len(in)-1)
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
