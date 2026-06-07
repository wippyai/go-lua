package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// ContainerRef is the canonical identity for a symbol-rooted container whose
// size facts may live in multiple PointState axes. It keeps the deterministic
// map key private to flow while allowing transfer to talk about one semantic
// container rather than numeric/relation-specific key encodings.
type ContainerRef struct {
	root cfg.SymbolID
	key  constraint.PathKey
}

// ContainerRefOfPath lowers a resolved static path to the shared container
// identity used by numeric length and relation cardinality facts.
func ContainerRefOfPath(path constraint.Path) (ContainerRef, bool) {
	key, ok := SymbolPathKeyOf(path)
	if !ok {
		return ContainerRef{}, false
	}
	return ContainerRef{root: path.Symbol, key: key}, true
}

// ContainerRefOfSymbol builds a container identity for a bare symbol slot.
func ContainerRefOfSymbol(sym cfg.SymbolID) (ContainerRef, bool) {
	if sym == 0 {
		return ContainerRef{}, false
	}
	return ContainerRef{root: sym, key: SymbolPathKey(sym, nil)}, true
}

func containerRefOfKey(key constraint.PathKey) (ContainerRef, bool) {
	sym, _, ok := ParseSymbolPathKey(key)
	if !ok {
		return ContainerRef{}, false
	}
	return ContainerRef{root: sym, key: key}, true
}

// IsValid reports whether ref names a real symbol-rooted container.
func (r ContainerRef) IsValid() bool {
	return r.root != 0 && r.key != ""
}

// Equal reports semantic container identity equality.
func (r ContainerRef) Equal(other ContainerRef) bool {
	return r.root == other.root && r.key == other.key
}

// Root returns the container's root symbol.
func (r ContainerRef) Root() cfg.SymbolID {
	return r.root
}

func (r ContainerRef) pathKey() constraint.PathKey {
	return r.key
}
