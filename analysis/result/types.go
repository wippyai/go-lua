package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
)

// Mount is one compile-time ingress snapshot placed at a Link module key.
// Result projection reads Snapshot and ModuleKey only.
type Mount struct {
	Snapshot  *ingress.Snapshot
	ModuleKey identity.ContentID
}

// NewMount admits one sealed ingress snapshot at a module key.
func NewMount(snapshot *ingress.Snapshot, moduleKey identity.ContentID) (Mount, bool) {
	if snapshot == nil || !snapshot.Available() || !moduleKey.Available() {
		return Mount{}, false
	}
	return Mount{Snapshot: snapshot, ModuleKey: moduleKey}, true
}

// Valid reports a sealed ingress snapshot at an available module key.
func (mount Mount) Valid() bool {
	return mount.Snapshot != nil && mount.Snapshot.Available() && mount.ModuleKey.Available() &&
		mount.Snapshot.ArtifactID().Available()
}

// ValueCoordinate is the Link substitution for one Value factor coordinate.
type ValueCoordinate struct {
	id    identity.ContentID
	mount identity.ContentID
}

// NewValueCoordinate admits one value identity at a mount.
func NewValueCoordinate(id, mount identity.ContentID) (ValueCoordinate, bool) {
	if !id.Available() || !mount.Available() {
		return ValueCoordinate{}, false
	}
	return ValueCoordinate{id: id, mount: mount}, true
}
