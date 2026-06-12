package address

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// LocalKey is the point-local path key spelling that preserves SSA versions.
type LocalKey pathdom.PathKey

// PathKey returns the PathKey carrier for existing map APIs.
func (k LocalKey) PathKey() pathdom.PathKey { return pathdom.PathKey(k) }

// Local is a point-local path identity that preserves path versions.
type Local struct {
	path pathdom.Path
}

// LocalOfPath normalizes a path into point-local address mode.
func LocalOfPath(path pathdom.Path) (Local, bool) {
	if path.IsEmpty() {
		return Local{}, false
	}
	return Local{path: clonePath(path)}, true
}

// LocalKeyOfPath returns the version-sensitive point-local key for path.
// It preserves path.Version and non-symbol root spelling. Symbol-rooted state
// application usually needs visibility.Resolver so the visible SSA version at
// a CFG point, rather than incidental path syntax, owns the key.
func LocalKeyOfPath(path pathdom.Path) (LocalKey, bool) {
	local, ok := LocalOfPath(path)
	if !ok {
		return "", false
	}
	return local.LocalKey(), true
}

// Path returns a defensive copy of the underlying path.
func (a Local) Path() pathdom.Path {
	return clonePath(a.path)
}

// Key returns the version-sensitive point-local map key.
func (a Local) Key() pathdom.PathKey {
	return a.LocalKey().PathKey()
}

// LocalKey returns the version-sensitive point-local key.
func (a Local) LocalKey() LocalKey {
	if a.path.IsEmpty() {
		return ""
	}
	return LocalKey(a.path.Key())
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
