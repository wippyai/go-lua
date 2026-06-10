package address

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ContainerRef is the canonical identity for a symbol-rooted container.
type ContainerRef struct {
	root symbol.ID
	key  pathdom.PathKey
}

// ContainerOfPath lowers a resolved static path to a shared container identity.
func ContainerOfPath(path pathdom.Path) (ContainerRef, bool) {
	key, ok := SymbolPathKeyOf(path)
	if !ok {
		return ContainerRef{}, false
	}
	return ContainerRef{root: path.Symbol, key: key}, true
}

// ContainerOfSymbol builds a container identity for a bare symbol slot.
func ContainerOfSymbol(sym symbol.ID) (ContainerRef, bool) {
	if sym == 0 {
		return ContainerRef{}, false
	}
	return ContainerRef{root: sym, key: SymbolPathKey(sym, nil)}, true
}

// ContainerFromKey parses a compact symbol-path key into a container identity.
func ContainerFromKey(key pathdom.PathKey) (ContainerRef, bool) {
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
func (r ContainerRef) Root() symbol.ID {
	return r.root
}

// Key returns the compact symbol-path key for map/set use.
func (r ContainerRef) Key() pathdom.PathKey {
	return r.key
}

// Stable returns the normalized address view for cross-domain container facts.
func (r ContainerRef) Stable() (Stable, bool) {
	if !r.IsValid() {
		return Stable{}, false
	}
	return StableFromKey(r.key)
}
