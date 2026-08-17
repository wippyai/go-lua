package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// CallTargetRow is the exact closure-allocation-to-callable-body proof.  It
// is artifact data, not a domain coordinate: all fields are Program-issued
// IDs captured while the allocation and Body proofs were live.
type CallTargetRow struct {
	allocation identity.ContentID
	body       identity.ContentID
	context    identity.ContentID
	function   identity.ContentID
	formal     identity.ContentID
	sealed     bool
}

func (row CallTargetRow) Available() bool {
	return row.sealed && row.allocation.Available() && row.body.Available() && row.context.Available() && row.function.Available() && row.formal.Available()
}
func (row CallTargetRow) AllocationID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.allocation
}
func (row CallTargetRow) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}
func (row CallTargetRow) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.context
}
func (row CallTargetRow) FunctionContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.function
}
func (row CallTargetRow) CallFormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.formal
}

// CallTargetCount is the exact closure-allocation denominator captured while
// the Program proof was live. It is separate from BodyCount because only
// executable function bodies are Call targets.
func (artifact *Artifact) CallTargetCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.callTargets)
}

// CallTargetAt returns one immutable allocation-to-body target proof. It
// exposes IDs only; no Program, transformer coordinate, or allocation handle
// can escape the artifact boundary.
func (artifact *Artifact) CallTargetAt(index int) (CallTargetRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.callTargets) {
		return CallTargetRow{}, false
	}
	return artifact.callTargets[index], true
}
