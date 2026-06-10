package address

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

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

// Path returns a defensive copy of the underlying path.
func (a Local) Path() pathdom.Path {
	return clonePath(a.path)
}

// Key returns the version-sensitive point-local map key.
func (a Local) Key() pathdom.PathKey {
	if a.path.IsEmpty() {
		return ""
	}
	return a.path.Key()
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
