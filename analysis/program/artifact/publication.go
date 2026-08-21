package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// Publish seals one canonical Program publication as an immutable Artifact.
// Publication is the only row store crossing the compiler boundary: Publish
// neither calls back into the compiler nor accepts drafts or alternate planes.
func Publish(key CompileKey, publication programpublication.Publication, counts denominator.CountRows) (*Artifact, bool) {
	if !key.Available() || !counts.Available() || !publication.EntryBodyID.Available() {
		return nil, false
	}
	catalog, catalogOK := programcatalog.CatalogID(key.ExecutionSchemaID().ContentID())
	if !catalogOK {
		return nil, false
	}
	store, storeOK := identity.IssueStore()
	if !storeOK {
		return nil, false
	}
	frozen, sealed := publication.Seal(catalog, store)
	if !sealed {
		return nil, false
	}
	artifact := &Artifact{key: key, frozen: frozen, entryBodyID: publication.EntryBodyID, coldCatalog: catalog, counts: counts}
	artifact.id = artifactID(artifact)
	if !artifact.key.Available() || !artifact.id.Available() || !artifact.counts.Available() || artifact.sealed.Available() || !programpublication.Validate(artifact.Program()) {
		return nil, false
	}
	artifact.sealed = artifact.id
	return artifact, artifact.Available()
}
