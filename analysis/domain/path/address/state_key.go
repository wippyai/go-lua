package address

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// StateKey is the typed string carrier for state lanes that operate on the
// root-or-visible state-key grammar. It is distinct from LocalKey and StableKey:
// root symbol facts may use an unversioned resolver root (symN), while member
// facts use the visible point-local resolver key (symN@V.suffix). Plain named,
// placeholder, and return-slot roots are also valid state keys.
//
// Hot state maps should still store the interned keyspace.Key form; StateKey is
// the boundary vocabulary that prevents callers from passing an arbitrary
// syntax PathKey where the state-key grammar is required.
type StateKey pathdom.PathKey

// PathKey returns the legacy string carrier for parsers and compatibility APIs.
func (k StateKey) PathKey() pathdom.PathKey { return pathdom.PathKey(k) }

func (k StateKey) String() string { return string(k) }

// StateKeyFromPathKey validates and narrows key to the state-key grammar used
// by keyspace.FromStateKey.
func StateKeyFromPathKey(key pathdom.PathKey) (StateKey, bool) {
	if key == "" {
		return "", false
	}
	if _, _, _, ok := ParseResolverPath(key); ok {
		return StateKey(key), true
	}
	if _, ok := parsePlainNamedRootSuffix(key); ok {
		return StateKey(key), true
	}
	return "", false
}
