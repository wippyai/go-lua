package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// CallTargetRow is the exact closure-allocation-to-callable-body proof.  It
// is artifact data, not a domain coordinate: all fields are Program-issued
// IDs captured while the allocation and Body proofs were live.
type CallTargetRow struct {
	row cold.CallTarget
}

func (row CallTargetRow) Available() bool { return row.row.Available() }
func (row CallTargetRow) AllocationID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.row.Allocation
}
func (row CallTargetRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.row.Body
}
func (row CallTargetRow) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.row.Context
}
func (row CallTargetRow) FunctionContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.row.Function
}
func (row CallTargetRow) CallFormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.row.Formal
}

// CallTargetCount is the exact closure-allocation denominator captured while
// the Program proof was live. It is separate from BodyCount because only
// executable function bodies are Call targets.
func (artifact *Artifact) CallTargetCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := cold.CallTargetFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !published {
		return 0
	}
	return count
}

// CallTargetAt returns one immutable allocation-to-body target proof. It
// exposes IDs only; no Program, transformer coordinate, or allocation handle
// can escape the artifact boundary.
func (artifact *Artifact) CallTargetAt(index int) (CallTargetRow, bool) {
	if !artifact.Available() {
		return CallTargetRow{}, false
	}
	row, held := cold.CallTargetFamily().At(&artifact.frozen, artifact.coldCatalog, index)
	if !held {
		return CallTargetRow{}, false
	}
	return CallTargetRow{row: row}, true
}

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
