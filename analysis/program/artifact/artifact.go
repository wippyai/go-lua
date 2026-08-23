package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Artifact is immutable after Compile succeeds. It owns one sealed canonical
// Program publication; consumers read that publication through Program.
type Artifact struct {
	key CompileKey
	id  identity.ContentID
	// entryBodyID is the scalar root-activation relation issued by the
	// compiler's Flow Body owner. It is immutable Artifact state, not a second
	// Body index or a module-local reconstruction.
	entryBodyID identity.ContentID
	// frozen is this program's cold publication: the families that have moved
	// onto the shared publication substrate, sealed once here and shared by
	// reference with every Link that mounts this artifact. It is not a second
	// copy of anything -- a family published here is not also retained as a
	// slice above, because two authorities for one family is exactly what the
	// frozen publication exists to remove.
	frozen snapshot.Frozen
	// coldCatalog is the identity the frozen publication is sealed under. It
	// is derived from the declaration catalog this artifact was compiled
	// against, so a cold column cannot be addressed by an axis of another
	// catalog and cannot be addressed against a runtime snapshot at all.
	coldCatalog identity.ContentID
	sealed      identity.ContentID
	counts      denominator.CountRows
}

func (artifact *Artifact) Available() bool {
	return artifact != nil && artifact.key.Available() && artifact.id.Available() && artifact.counts.Available() && artifact.sealed == artifact.id
}

func (artifact *Artifact) CompileKey() CompileKey {
	if !artifact.Available() {
		return CompileKey{}
	}
	return artifact.key
}

func (artifact *Artifact) ID() identity.ContentID {
	if !artifact.Available() {
		return identity.ContentID{}
	}
	return artifact.id
}

// Program returns the canonical immutable Program publication owned by this
// artifact. Consumers use its dense families directly; Artifact retains no
// occurrence or rule-placement slices of its own.
func (artifact *Artifact) Program() programschema.Program {
	if artifact == nil || !artifact.frozen.Published() || !artifact.id.Available() || !artifact.key.ExecutionSchemaID().Available() || !artifact.entryBodyID.Available() {
		return programschema.Program{}
	}
	row, ok := programschema.New(artifact.frozen, artifact.id, artifact.key.ProgramID(), artifact.key.ExecutionSchemaID().ContentID(), artifact.entryBodyID)
	if !ok {
		return programschema.Program{}
	}
	return row
}
