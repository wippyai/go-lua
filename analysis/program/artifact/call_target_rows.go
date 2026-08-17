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
