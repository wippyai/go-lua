package address

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// LocalKey carries a canonical key admitted by point-local semantics.
type LocalKey struct {
	key pathdom.PathKey
}

// PathKey returns the PathKey carrier for existing map APIs.
func (k LocalKey) PathKey() pathdom.PathKey { return k.key }

// Local is a point-local path identity that preserves path versions.
type Local struct {
	path pathdom.Path
}

// LocalOfPath normalizes a path into point-local address mode.
func LocalOfPath(path pathdom.Path) (Local, bool) {
	if path.IsEmpty() || pathdom.FormatKey(path) == "" {
		return Local{}, false
	}
	return Local{path: clonePath(path)}, true
}

// Key returns the version-sensitive point-local map key.
func (a Local) Key() pathdom.PathKey {
	return a.LocalKey().PathKey()
}

// LocalKey returns the version-sensitive point-local key.
func (a Local) LocalKey() LocalKey {
	if a.path.IsEmpty() {
		return LocalKey{}
	}
	return LocalKey{key: a.path.Key()}
}

// Stable returns the version-insensitive address for the local address.
func (a Local) Stable() (Stable, bool) {
	return StableOfPath(a.path)
}

// SameVersion reports exact point-local identity.
func (a Local) SameVersion(b Local) bool {
	return a.path.Equal(b.path)
}

func clonePath(path pathdom.Path) pathdom.Path {
	path.Segments = cloneSegments(path.Segments)
	return path
}
