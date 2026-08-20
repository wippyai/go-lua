package artifact

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// coldStores issues the runtime store identity for each independently sealed
// immutable publication. Store identity is deliberately not content-derived.
var coldStores atomic.Uint64

// Publish seals one canonical Program publication as an immutable Artifact.
// Publication is the only row store crossing the compiler boundary: Publish
// neither calls back into the compiler nor accepts drafts or alternate planes.
func Publish(key CompileKey, publication programschema.Publication, counts denominator.CountRows) (*Artifact, bool) {
	if !key.Available() || !counts.Available() {
		return nil, false
	}
	catalog, catalogOK := programschema.CatalogID(key.SchemaDigest())
	if !catalogOK {
		return nil, false
	}
	frozen, sealed := publication.Seal(catalog, identity.StoreID(coldStores.Add(1)))
	if !sealed {
		return nil, false
	}
	artifact := &Artifact{key: key, frozen: frozen, coldCatalog: catalog, counts: counts}
	artifact.id = artifactID(artifact)
	if !artifact.validUnsealed() {
		return nil, false
	}
	artifact.sealed = artifact.id
	return artifact, artifact.Available()
}
