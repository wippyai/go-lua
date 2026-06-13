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

// ContainerRefOfPath lowers a resolved symbol-rooted path to a container ref.
func ContainerRefOfPath(path pathdom.Path) (ContainerRef, bool) {
	stable, ok := StableOfPath(path)
	if !ok {
		return ContainerRef{}, false
	}
	return ContainerRefOfStable(stable)
}

// ContainerRefOfStable lowers a symbol-rooted stable address to a container ref.
func ContainerRefOfStable(stable Stable) (ContainerRef, bool) {
	root, ok := stable.Symbol()
	if !ok {
		return ContainerRef{}, false
	}
	return ContainerRefFromKey(root, stable.Key())
}

// ContainerRefFromKey builds a container ref from a symbol root and stable key.
func ContainerRefFromKey(root symbol.ID, key pathdom.PathKey) (ContainerRef, bool) {
	ref := ContainerRef{root: root, key: key}
	if !ref.IsValid() {
		return ContainerRef{}, false
	}
	return ref, true
}

// IsValid reports whether ref names a real symbol-rooted container.
func (r ContainerRef) IsValid() bool {
	if r.root == 0 || r.key == "" {
		return false
	}
	root, _, ok := ParseSymbolPathKey(r.key)
	return ok && root == r.root
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
