package address

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// StateKey is the opaque carrier for state lanes that operate on root-or-visible
// keys. It is distinct from LocalKey and StableKey:
// root symbol facts may use an unversioned resolver root (symN), while member
// facts use the visible point-local resolver key (symN@V.suffix).
// Length-prefixed named, placeholder, and return-slot roots are also valid.
//
// Hot state maps should still store the interned keyspace.Key form; StateKey is
// the boundary vocabulary that prevents callers from passing an arbitrary
// syntax PathKey where the state-key grammar is required.
type StateKey struct {
	key pathdom.PathKey
}

// PathKey returns the shared canonical carrier.
func (k StateKey) PathKey() pathdom.PathKey { return k.key }

func (k StateKey) String() string { return string(k.key) }

// StateKeyFromPathKey validates and narrows a canonical key to state semantics.
func StateKeyFromPathKey(key pathdom.PathKey) (StateKey, bool) {
	path, ok := pathdom.ParseKey(key)
	if !ok || path.Symbol == 0 && path.Root == "" {
		return StateKey{}, false
	}
	return StateKey{key: key}, true
}
