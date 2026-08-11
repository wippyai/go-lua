package transfer

import (
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	"github.com/wippyai/go-lua/program/target"
)

type Classification struct {
	source       *link.Link
	arm          Arm
	endpoint     target.TransferEndpoint
	identity     target.TransferIdentity
	capabilities target.TransferCapabilities
}

func Classify(algebra Algebra, arm Arm) (Classification, bool) {
	if _, ok := algebra.armIndex(arm); !ok {
		return Classification{}, false
	}
	contract, ok := algebra.owner.source.Boundary().Target()
	endpoint, _, _, identity, capabilities, ok := contract.TransferDeclaration(arm.transfer)
	if !ok {
		return Classification{}, false
	}
	return Classification{algebra.owner.source, arm, endpoint, identity, capabilities}, true
}
func (c Classification) valid() bool { return c.source != nil && c.arm.validFor(c.source) }
func (c Classification) Valid() bool { return c.valid() }
func (c Classification) LinkContentID() (keyspace.ContentID, bool) {
	if !c.valid() {
		return keyspace.ContentID{}, false
	}
	return c.source.ContentID(), true
}
func (c Classification) Arm() (Arm, bool) {
	if !c.valid() {
		return Arm{}, false
	}
	return c.arm, true
}
func (c Classification) Transfer() (target.TransferID, bool) {
	if !c.valid() {
		return 0, false
	}
	return c.arm.transfer, true
}
func (c Classification) Endpoint() (target.TransferEndpoint, bool) {
	if !c.valid() {
		return target.TransferEndpoint{}, false
	}
	return c.endpoint, true
}
func (c Classification) Disposition() (target.TransferPossibility, bool) {
	if !c.valid() {
		return 0, false
	}
	return c.arm.disposition, true
}
func (c Classification) Delivers() (bool, bool) {
	d, ok := c.Disposition()
	return d == target.TransferMayDeliver, ok
}
func (c Classification) Rejects() (bool, bool) {
	d, ok := c.Disposition()
	return d == target.TransferMayReject, ok
}
func (c Classification) Identity() (target.TransferIdentity, bool) {
	if !c.valid() {
		return target.TransferIdentityInvalid, false
	}
	return c.identity, true
}
func (c Classification) Capabilities() (target.TransferCapabilities, bool) {
	if !c.valid() {
		return target.TransferCapabilitiesInvalid, false
	}
	return c.capabilities, true
}
func (c Classification) Rebind(source *link.Link) (Classification, bool) {
	if !c.valid() || source == nil || source.ContentID() != c.source.ContentID() {
		return Classification{}, false
	}
	a, ok := NewAlgebra(source)
	if !ok {
		return Classification{}, false
	}
	for i := 0; i < a.ArmCount(); i++ {
		arm, ok := a.ArmAt(i)
		if ok && arm == c.arm {
			return Classify(a, arm)
		}
	}
	return Classification{}, false
}
