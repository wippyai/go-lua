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
