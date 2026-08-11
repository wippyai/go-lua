package transfer

import (
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

type Isolation uint8

const (
	IsolationInvalid Isolation = iota
	IsolationMutable
	IsolationSealed
	IsolationDeepImmutable
	IsolationUnknown
)

func (i Isolation) valid() bool { return i >= IsolationMutable && i <= IsolationUnknown }

// Arm is Transfer's private Application-selected Target transfer-outcome port.
type Arm struct {
	application linkproject.Application
	operation   target.Operation
	transfer    target.TransferID
	outcome     uint32
	disposition target.TransferPossibility
	isolation   Isolation
}

func (arm Arm) validFor(source *link.Link) bool {
	if source == nil || !arm.isolation.valid() {
		return false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil || !source.Boundary().ApplicationOperationAvailable(contract, arm.application, arm.operation) {
		return false
	}
	owner, ownerOK := contract.TransferOwner(arm.transfer)
	if !ok || !ownerOK || owner != arm.operation {
		return false
	}
	_, disposition, portOK := contract.TransferOutcomeAt(arm.operation, int(arm.transfer)-1, int(arm.outcome))
	return portOK && disposition == arm.disposition
}
func lessArm(left, right Arm) bool {
	if left.operation != right.operation {
		return left.operation < right.operation
	}
	if left.transfer != right.transfer {
		return left.transfer < right.transfer
	}
	if left.outcome != right.outcome {
		return left.outcome < right.outcome
	}
	return left.isolation < right.isolation
}
