package mounted

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
)

// Mount is one placed compiled Program: the sealed ingress snapshot together
// with the Link-local module identity that places it.
type Mount struct {
	ModuleKey identity.ContentID
	Snapshot  *ingress.Snapshot
}

func (mount Mount) Available() bool {
	return mount.ModuleKey.Available() && mount.Snapshot != nil && mount.Snapshot.Available()
}

// mountsAvailable states the admission every seal shares: at least one mount,
// each one complete, and no module identity placed twice. A repeated module
// identity would collapse two mounts into one key space, so the whole
// population fails closed rather than publishing a merged census.
func mountsAvailable(mounts []Mount) bool {
	if len(mounts) == 0 {
		return false
	}
	seen := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if !mount.Available() {
			return false
		}
		if _, duplicate := seen[mount.ModuleKey]; duplicate {
			return false
		}
		seen[mount.ModuleKey] = struct{}{}
	}
	return true
}
