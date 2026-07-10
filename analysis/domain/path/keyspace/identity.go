package keyspace

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// PathIdentity is the comparable cache identity for a source path at an
// analysis boundary. Prefer Key when a KeySpace is available; Legacy preserves
// the old string carrier for tests and compatibility-only call paths that have
// no resolver-backed keyspace.
type PathIdentity struct {
	Key    Key
	Legacy pathdom.PathKey
}

// PathIdentityFromPath returns the canonical comparable identity for p under
// ks. A nil KeySpace deliberately falls back to p.Key() so older compatibility
// boundaries keep exactly the same map-key behavior.
func PathIdentityFromPath(ks *KeySpace, p pathdom.Path) (PathIdentity, bool) {
	if p.IsEmpty() {
		return PathIdentity{}, false
	}
	if ks != nil {
		return PathIdentity{Key: ks.FromPath(p)}, true
	}
	return PathIdentity{Legacy: p.Key()}, true
}
