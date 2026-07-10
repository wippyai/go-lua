package visibility

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Address is the point-scoped engine address view of a source path. It is the
// canonical owner for choosing visible vs. root-or-visible state spelling and
// for crossing from path keys into the analysis keyspace.
type Address struct {
	resolver PathKeyResolver
	point    cfg.Point
	path     pathdom.Path
}

// StateKeyForm names one concrete state-key spelling an address can produce.
// Callers choose the semantic forms they need; Address owns ordering,
// deduplication, and conversion.
type StateKeyForm int

const (
	StateKeyVisible StateKeyForm = iota
	StateKeyRootOrVisible
	StateKeyStructural
)

type keyspaceAddressResolver interface {
	VisibleKeyspaceKeyAt(cfg.Point, pathdom.Path) (keyspace.Key, bool)
	VisibleLocalKeyspaceKeyAt(cfg.Point, pathdom.Path) (keyspace.Key, bool)
	RootOrVisibleKeyspaceKeyAt(cfg.Point, pathdom.Path) (keyspace.Key, bool)
}

// AddressAt returns the address view for path at point.
func AddressAt(resolver PathKeyResolver, point cfg.Point, path pathdom.Path) Address {
	return Address{resolver: resolver, point: point, path: path}
}

// VisiblePathKey returns the point-visible key for lanes keyed by local source
// values.
func (a Address) VisiblePathKey() (pathdom.PathKey, bool) {
	if a.resolver == nil || a.path.IsEmpty() {
		return "", false
	}
	key := a.resolver.KeyAt(a.point, a.path)
	if key == "" {
		return "", false
	}
	return key, true
}

// VisibleStateKey returns the typed point-visible key for lanes keyed by local
// source values.
func (a Address) VisibleStateKey() (pathaddr.StateKey, bool) {
	key, ok := a.VisiblePathKey()
	if !ok {
		return "", false
	}
	return pathaddr.StateKeyFromPathKey(key)
}

// VisibleKeyspaceKey returns the interned keyspace key for VisibleStateKey.
func (a Address) VisibleKeyspaceKey() (keyspace.Key, bool) {
	if direct, ok := a.resolver.(keyspaceAddressResolver); ok {
		return direct.VisibleKeyspaceKeyAt(a.point, a.path)
	}
	stateKey, ok := a.VisibleStateKey()
	if !ok {
		return keyspace.Key{}, false
	}
	return KeyspaceKeyFromStateKey(a.resolver, stateKey)
}

// VisibleLocalKeyspaceKey returns the interned key for point-local value lanes.
// Unlike VisibleKeyspaceKey, it accepts only resolver-versioned local paths.
func (a Address) VisibleLocalKeyspaceKey() (keyspace.Key, bool) {
	if direct, ok := a.resolver.(keyspaceAddressResolver); ok {
		return direct.VisibleLocalKeyspaceKeyAt(a.point, a.path)
	}
	key, ok := a.VisiblePathKey()
	if !ok || a.resolver == nil {
		return keyspace.Key{}, false
	}
	ks := a.resolver.KeySpace()
	if ks == nil {
		return keyspace.Key{}, false
	}
	return ks.FromPathKey(key)
}

// RootOrVisiblePathKey returns the key for facts that store root-symbol paths
// structurally but member paths under the point-visible version.
func (a Address) RootOrVisiblePathKey() (pathdom.PathKey, bool) {
	key, ok := a.RootOrVisibleStateKey()
	if !ok {
		return "", false
	}
	return key.PathKey(), true
}

// RootOrVisibleStateKey returns the typed key for facts that store root-symbol
// paths structurally but member paths under the point-visible version.
func (a Address) RootOrVisibleStateKey() (pathaddr.StateKey, bool) {
	if a.path.IsEmpty() || a.path.Symbol == 0 {
		return "", false
	}
	if len(a.path.Segments) == 0 {
		return pathaddr.StateKeyFromPathKey(a.path.Key())
	}
	if a.resolver == nil {
		return "", false
	}
	return pathaddr.StateKeyFromPathKey(a.resolver.KeyAt(a.point, a.path))
}

// RootOrVisibleKeyspaceKey returns the interned keyspace key for
// RootOrVisibleStateKey.
func (a Address) RootOrVisibleKeyspaceKey() (keyspace.Key, bool) {
	if direct, ok := a.resolver.(keyspaceAddressResolver); ok {
		return direct.RootOrVisibleKeyspaceKeyAt(a.point, a.path)
	}
	stateKey, ok := a.RootOrVisibleStateKey()
	if !ok {
		return keyspace.Key{}, false
	}
	return KeyspaceKeyFromStateKey(a.resolver, stateKey)
}

// StructuralStateKey returns the path's own structural state key without
// point-local visibility rewriting.
func (a Address) StructuralStateKey() (pathaddr.StateKey, bool) {
	return pathaddr.StateKeyFromPathKey(a.path.Key())
}

// ForEachStateKey visits requested state-key forms in request order, omitting
// failed and duplicate spellings. Returning false from fn stops iteration.
func (a Address) ForEachStateKey(fn func(pathaddr.StateKey) bool, forms ...StateKeyForm) bool {
	if fn == nil {
		return true
	}
	var seen [3]pathaddr.StateKey
	seenCount := 0
	for _, form := range forms {
		key, ok := a.stateKeyForForm(form)
		if !ok || key == "" {
			continue
		}
		duplicate := false
		for i := 0; i < seenCount; i++ {
			if seen[i] == key {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if seenCount < len(seen) {
			seen[seenCount] = key
			seenCount++
		}
		if !fn(key) {
			return false
		}
	}
	return true
}

// StateKeys returns the requested state-key forms in request order, omitting
// failed and duplicate spellings. It is the canonical way to ask for an
// address's equivalent read/write candidate keys without reimplementing
// visible/root/structural set construction at each call site.
func (a Address) StateKeys(forms ...StateKeyForm) []pathaddr.StateKey {
	if len(forms) == 0 {
		return nil
	}
	out := make([]pathaddr.StateKey, 0, len(forms))
	a.ForEachStateKey(func(key pathaddr.StateKey) bool {
		out = append(out, key)
		return true
	}, forms...)
	return out
}

func (a Address) stateKeyForForm(form StateKeyForm) (pathaddr.StateKey, bool) {
	switch form {
	case StateKeyVisible:
		return a.VisibleStateKey()
	case StateKeyRootOrVisible:
		return a.RootOrVisibleStateKey()
	case StateKeyStructural:
		return a.StructuralStateKey()
	default:
		return "", false
	}
}

// ForEachKeyspaceKey interns and visits requested state-key forms in the
// resolver keyspace, preserving ForEachStateKey ordering and deduplication.
func (a Address) ForEachKeyspaceKey(fn func(keyspace.Key) bool, forms ...StateKeyForm) bool {
	if fn == nil {
		return true
	}
	var seen [3]keyspace.Key
	seenCount := 0
	for _, form := range forms {
		key, ok := a.keyspaceKeyForForm(form)
		if !ok || key.Kind == keyspace.KindInvalid {
			continue
		}
		duplicate := false
		for i := 0; i < seenCount; i++ {
			if seen[i] == key {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if seenCount < len(seen) {
			seen[seenCount] = key
			seenCount++
		}
		if !fn(key) {
			return false
		}
	}
	return true
}

// KeyspaceKeys interns the requested state-key forms in the resolver keyspace,
// preserving StateKeys ordering and deduplication.
func (a Address) KeyspaceKeys(forms ...StateKeyForm) []keyspace.Key {
	if len(forms) == 0 {
		return nil
	}
	out := make([]keyspace.Key, 0, len(forms))
	a.ForEachKeyspaceKey(func(key keyspace.Key) bool {
		out = append(out, key)
		return true
	}, forms...)
	return out
}

func (a Address) keyspaceKeyForForm(form StateKeyForm) (keyspace.Key, bool) {
	switch form {
	case StateKeyVisible:
		return a.VisibleKeyspaceKey()
	case StateKeyRootOrVisible:
		return a.RootOrVisibleKeyspaceKey()
	case StateKeyStructural:
		stateKey, ok := a.StructuralStateKey()
		if !ok {
			return keyspace.Key{}, false
		}
		return KeyspaceKeyFromStateKey(a.resolver, stateKey)
	default:
		return keyspace.Key{}, false
	}
}

// KeyspaceKeyFromStateKey interns an already typed state key in the resolver's
// keyspace.
func KeyspaceKeyFromStateKey(resolver PathKeyResolver, stateKey pathaddr.StateKey) (keyspace.Key, bool) {
	if resolver == nil || stateKey == "" {
		return keyspace.Key{}, false
	}
	ks := resolver.KeySpace()
	if ks == nil {
		return keyspace.Key{}, false
	}
	return ks.InternStateKey(stateKey)
}
