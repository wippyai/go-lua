package target

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// These helpers are the remaining Target-owned relation plane. They expose
// only ranges needed by continuation, subedge, and identity construction; the
// published operation declaration/value/effect queries live on operation.Core.
func (c *Contract) boundOperationCount() int {
	if c == nil {
		return 0
	}
	return c.Core.BoundCount()
}

func (c *Contract) boundOperationAt(index int) (vocabulary.Operation, bool) {
	if c == nil || index < 0 || index >= c.boundOperationCount() {
		return 0, false
	}
	return c.Core.OperationAt(index)
}

func (c *Contract) rowFormalCount(op vocabulary.Operation) int {
	return c.Core.RowFormalCount(op)
}

func (c *Contract) opaqueOperation(op vocabulary.Operation) bool {
	opaque, ok := c.Core.Opaque()
	return ok && op == opaque
}

func (c *Contract) operationIndex(op vocabulary.Operation) (int, bool) {
	if c == nil || op == 0 || uint64(op) > uint64(len(c.operations)) {
		return 0, false
	}
	if _, ok := c.Core.OperationAt(int(op) - 1); !ok {
		return 0, false
	}
	return int(op) - 1, true
}

func (c *Contract) operation(op vocabulary.Operation) (operationRow, bool) {
	index, ok := c.operationIndex(op)
	if !ok {
		return operationRow{}, false
	}
	return c.operations[index], true
}

func (c *Contract) operationOutcomeRange(op vocabulary.Operation) (indexRange, bool) {
	row, ok := c.operation(op)
	if !ok {
		return indexRange{}, false
	}
	return row.outcomes, true
}

func (c *Contract) transferCount(op vocabulary.Operation) int {
	return c.Core.TransferCount(op)
}

func (c *Contract) transferIDAt(op vocabulary.Operation, index int) (vocabulary.TransferID, bool) {
	return c.Core.TransferIDAt(op, index)
}

func (c *Contract) transferOwner(id vocabulary.TransferID) (vocabulary.Operation, bool) {
	return c.Core.TransferOwner(id)
}

func (c *Contract) transferDeclaration(id vocabulary.TransferID) (endpoint vocabulary.TransferEndpoint, payload vocabulary.InputSource, alias vocabulary.InputSource, identity vocabulary.TransferIdentity, capabilities vocabulary.TransferCapabilities, ok bool) {
	return c.Core.TransferDeclaration(id)
}

func (c *Contract) transferDeclarationOutcomeAt(id vocabulary.TransferID, index int) (uint32, vocabulary.TransferPossibility, bool) {
	return c.Core.TransferDeclarationOutcomeAt(id, index)
}

func (c *Contract) transferEndpointAt(op vocabulary.Operation, index int) (vocabulary.TransferEndpoint, bool) {
	return c.Core.TransferEndpointAt(op, index)
}

func (c *Contract) transferPayloadAt(op vocabulary.Operation, index int) (vocabulary.InputSource, bool) {
	return c.Core.TransferPayloadAt(op, index)
}

func (c *Contract) transferAliasAt(op vocabulary.Operation, index int) (vocabulary.InputSource, bool) {
	return c.Core.TransferAliasAt(op, index)
}

func (c *Contract) transferIdentityAt(op vocabulary.Operation, index int) (vocabulary.TransferIdentity, bool) {
	return c.Core.TransferIdentityAt(op, index)
}

func (c *Contract) transferCapabilitiesAt(op vocabulary.Operation, index int) (vocabulary.TransferCapabilities, bool) {
	return c.Core.TransferCapabilitiesAt(op, index)
}

func (c *Contract) transferOutcomeCount(op vocabulary.Operation, index int) int {
	return c.Core.TransferOutcomeCount(op, index)
}

func (c *Contract) transferOutcomeAt(op vocabulary.Operation, transfer, index int) (uint32, vocabulary.TransferPossibility, bool) {
	return c.Core.TransferOutcomeAt(op, transfer, index)
}

func (c *Contract) effect(op vocabulary.Operation, index int) (effectRow, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() {
		return effectRow{}, false
	}
	effects := row.effects
	return c.effects[effects.start+uint32(index)], true
}

func (c *Contract) bindingOwnerKeyAt(op vocabulary.Operation, binding, index int) (vocabulary.ExactKey, bool) {
	return c.Core.BindingOwnerKeyAt(op, binding, index)
}

func (c *Contract) bindingMemberKeyAt(op vocabulary.Operation, binding, index int) (vocabulary.ExactKey, bool) {
	return c.Core.BindingMemberKeyAt(op, binding, index)
}

func (c *Contract) bindingOwnerAt(op vocabulary.Operation, binding, index int) (string, bool) {
	return c.Core.BindingOwnerAt(op, binding, index)
}

func (c *Contract) effectRowArgumentAt(op vocabulary.Operation, index, argument int) (vocabulary.RowVar, bool) {
	return c.Core.EffectRowArgumentAt(op, index, argument)
}

func (c *Contract) validPublicationEffectRow(effect effectRow) bool {
	if c == nil || !effect.hasPublication || !effect.publication.validConsequences() {
		return false
	}
	target, ok := c.Input(effect.target)
	if !ok || uint64(effect.publication.subject) >= uint64(c.ValuesCount(target)) {
		return false
	}
	return effect.publication.destination != vocabulary.PublicationDestinationValueFormal ||
		uint64(effect.publication.context) < uint64(c.ValuesCount(target))
}

func (c *Contract) PublicationEffectDescriptor(op vocabulary.Operation, index int) (PublicationEffectDescriptor, bool) {
	row, ok := c.effect(op, index)
	if !ok || !c.sealed || !c.validPublicationEffectRow(row) {
		return PublicationEffectDescriptor{}, false
	}
	return row.publication, true
}
