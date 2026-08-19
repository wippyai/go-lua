package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// ColdPublication is this artifact's sealed cold publication together with
// the catalog identity it is addressed under. It is what a Link places in its
// mount directory: one value, shared by reference with every mount of this
// artifact, that admits no derivation and therefore cannot be advanced by any
// holder of it.
func (artifact *Artifact) ColdPublication() (snapshot.Frozen, identity.ContentID, bool) {
	if !artifact.Available() || !artifact.frozen.Published() || !artifact.coldCatalog.Available() {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	return artifact.frozen, artifact.coldCatalog, true
}
