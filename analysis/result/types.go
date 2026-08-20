package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// Mount is one compile-time ingress snapshot and its canonical self-describing
// cold Program row. Result projection reads structural rows from the snapshot
// and Program-owned summaries from the program; the module key is carried by
// the program.
type Mount struct {
	snapshot *ingress.Snapshot
	program  programmount.Program
}

// NewMount admits one sealed ingress snapshot together with the canonical
// mount row already issued for it. The program is retained exactly as issued;
// Result does not rederive a mount row from neutral ingress and a module key.
func NewMount(snapshot *ingress.Snapshot, program programmount.Program) (Mount, bool) {
	mount := Mount{snapshot: snapshot, program: program}
	return mount, mount.Valid()
}

// Valid reports a sealed ingress snapshot and matching canonical cold Program.
func (mount Mount) Valid() bool {
	return mount.snapshot != nil && mount.snapshot.Available() && mount.program.Available() &&
		mount.snapshot.ArtifactID() == mount.program.ArtifactID && mount.snapshot.ProgramID() == mount.program.ProgramID && mount.snapshot.SchemaID() == mount.program.SchemaID
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
