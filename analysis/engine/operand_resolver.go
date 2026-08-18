package engine

import "github.com/wippyai/go-lua/analysis/identity"

// OperandCoords is the sealed neutral member coordinate the engine already
// holds. The owner-supplied resolver on the rule cell turns these into the
// typed operand; the engine never threads that operand through attach.
type OperandCoords struct {
	Member     identity.ContentID
	Mount      identity.ContentID
	Point      identity.ContentID
	Occurrence identity.ContentID
}

// InstallOperandResolver publishes the one owner-supplied operand resolver
// on this rule's sealed cell. A second install is rejected: one rule has
// exactly one resolver.
func (implementation *RuleImplementation[K, V, O]) InstallOperandResolver(resolve func(OperandCoords) (O, bool)) bool {
	if implementation == nil || !implementation.receipt.valid() || implementation.receipt.state == nil || resolve == nil {
		return false
	}
	state := implementation.receipt.state
	state.mu.Lock()
	defer state.mu.Unlock()
	hot := implementation.receipt.cell.impl
	if hot == nil || hot.operandResolver != nil {
		return false
	}
	hot.operandResolver = resolve
	return true
}

// HasOperandResolver reports whether this rule's sealed cell already holds
// its owner-supplied resolver.
func (implementation *RuleImplementation[K, V, O]) HasOperandResolver() bool {
	if implementation == nil || !implementation.receipt.valid() || implementation.receipt.cell == nil || implementation.receipt.cell.impl == nil {
		return false
	}
	return implementation.receipt.cell.impl.operandResolver != nil
}

// RuleProgramAttach is the erased construction join for one sealed rule cell.
// The engine enumerates neutral member coordinates and this handle binds the
// executable row; the typed operand never leaves the cell.
type RuleProgramAttach interface {
	AdmitMounted(assembly *ReceiptAssembly, role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool
	AdmitLink(assembly *ReceiptAssembly, role RuleSlotCapability, occurrence identity.ContentID) bool
	AttachMountedMember(construction *ProgramConstruction, role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool
	AttachLinkMember(construction *ProgramConstruction, role RuleSlotCapability, occurrence identity.ContentID) bool
}

// RuleProgramSource is the hot-cell surface that publishes the one program
// attach for a mounted or Link rule. The composition recovers it from the
// opaque cell; the engine never switches on a domain type.
type RuleProgramSource interface {
	ProgramAttach() (RuleProgramAttach, bool)
}

func (implementation *RuleImplementation[K, V, O]) AttachMountedMember(construction *ProgramConstruction, role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	return AttachMountedRuleMember(construction, implementation, role, mount, point, occurrence)
}

func (implementation *RuleImplementation[K, V, O]) AttachLinkMember(construction *ProgramConstruction, role RuleSlotCapability, occurrence identity.ContentID) bool {
	return AttachLinkRuleMember(construction, implementation, role, occurrence)
}

func (implementation *RuleImplementation[K, V, O]) resolveOperand(coords OperandCoords) (O, bool) {
	var absent O
	if implementation == nil || !implementation.receipt.valid() || implementation.receipt.cell == nil || implementation.receipt.cell.impl == nil || implementation.receipt.cell.impl.operandResolver == nil {
		return absent, false
	}
	return implementation.receipt.cell.impl.operandResolver(coords)
}
