package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// Mount is one compile-time ingress snapshot and its canonical self-describing
// cold Program row. Result projection reads structural rows from Snapshot and
// Program-owned summaries from Program; the module key is carried by Program.
type Mount struct {
	Snapshot *ingress.Snapshot
	Program  programmount.Program
}

// NewMount admits one sealed ingress snapshot at a module key.
func NewMount(snapshot *ingress.Snapshot, moduleKey identity.ContentID) (Mount, bool) {
	if snapshot == nil || !snapshot.Available() || !moduleKey.Available() {
		return Mount{}, false
	}
	program, programOK := programmount.ProgramFromSnapshot(snapshot, moduleKey)
	if !programOK {
		return Mount{}, false
	}
	return Mount{Snapshot: snapshot, Program: program}, true
}

// Valid reports a sealed ingress snapshot and matching canonical cold Program.
func (mount Mount) Valid() bool {
	return mount.Snapshot != nil && mount.Snapshot.Available() && mount.Program.Available() &&
		mount.Snapshot.ArtifactID() == mount.Program.ArtifactID && mount.Snapshot.ProgramID() == mount.Program.ProgramID && mount.Snapshot.SchemaID() == mount.Program.SchemaID
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
