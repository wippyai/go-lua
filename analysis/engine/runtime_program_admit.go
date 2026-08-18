package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

func (implementation *RuleImplementation[K, V, O]) AdmitMounted(assembly *ReceiptAssembly, role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	if implementation == nil || assembly == nil || !role.Mounted() {
		return false
	}
	operand, resolved := implementation.resolveOperand(OperandCoords{Mount: mount, Point: point, Occurrence: occurrence})
	if !resolved {
		return false
	}
	row, rowOK := assembly.AdmitMountedRuleOccurrence(role, mount, point, occurrence)
	if !rowOK {
		return false
	}
	return implementation.admitRule(assembly, role, row, operand, assembly.QueueMountedRuleFinalizer)
}

func (implementation *RuleImplementation[K, V, O]) AdmitLink(assembly *ReceiptAssembly, role RuleSlotCapability, occurrence identity.ContentID) bool {
	if implementation == nil || assembly == nil || !role.Link() {
		return false
	}
	operand, resolved := implementation.resolveOperand(OperandCoords{Occurrence: occurrence})
	if !resolved {
		return false
	}
	row, rowOK := assembly.AdmitLinkRuleOccurrence(role, occurrence)
	if !rowOK {
		return false
	}
	return implementation.admitRule(assembly, role, row, operand, assembly.QueueLinkRuleFinalizer)
}

func (implementation *RuleImplementation[K, V, O]) admitRule(assembly *ReceiptAssembly, role RuleSlotCapability, occurrence RuleOccurrenceReceipt, operand O, queue func(RuleSlotCapability, func() bool) bool) bool {
	return admitRuleScope(assembly, implementation, occurrence, operand,
		func(transaction *RuleSourceTransaction) bool {
			return implementation.placeSurfaces(transaction, operand)
		},
		func(source RuleSurfaceSourceReceipt) bool {
			return implementation.issueDraft(assembly, occurrence, source)
		},
		nil, queue, role)
}

func (implementation *RuleImplementation[K, V, O]) placeSurfaces(transaction *RuleSourceTransaction, operand O) bool {
	if implementation == nil || !implementation.receipt.valid() || implementation.receipt.cell == nil || implementation.receipt.cell.impl == nil || transaction == nil {
		return false
	}
	hot := implementation.receipt.cell.impl
	state := implementation.receipt.state
	authority := implementation.receipt.authority
	ordinal := implementation.receipt.proof.ordinal
	shape, shapeOK := state.schema.ruleShapeAt(ordinal)
	if !shapeOK || uint64(len(hot.reads)) != shape.ReadCount || shape.WriteCount != 1 {
		return false
	}
	placed := make([]RuleReadSurface, shape.ReadCount)
	for index := uint64(0); index < shape.ReadCount; index++ {
		readShape, readOK := state.schema.ruleReadShapeAt(ordinal, index)
		if !readOK {
			return false
		}
		switch readShape.Kind {
		case composition.ReadExact:
			local, projected := hot.reads[index].projectLocal(operand)
			factor := hot.reads[index].exactAdmitFactor()
			if !projected || factor == nil || !factor.schemaFactorAdmitExactRead(state, authority, transaction, local) {
				return false
			}
			placed[index] = transaction.reads[len(transaction.reads)-1]
		case composition.ReadSelect:
			deps := make([]RuleReadSurface, readShape.DependencyCount)
			for dep := uint64(0); dep < readShape.DependencyCount; dep++ {
				depIndex, depOK := state.schema.ruleReadDependencyAt(ordinal, index, dep)
				if !depOK || depIndex >= uint64(len(placed)) || !placed[depIndex].value.Available() {
					return false
				}
				deps[dep] = placed[depIndex]
			}
			receipt, receiptOK := implementation.SelectedReadReceipt(index)
			surface, surfaceOK := transaction.AnchoredSelectedReadSurface(receipt, deps)
			if !receiptOK || !surfaceOK || !transaction.AddRead(surface) {
				return false
			}
			placed[index] = surface
		case composition.ReadSummary:
			receipt, receiptOK := implementation.SummaryReadReceipt(index)
			provider, providerOK := hot.reads[index].(interface{ summarySurfaceAdmit() any })
			if !receiptOK || !providerOK {
				return false
			}
			admit, admitOK := provider.summarySurfaceAdmit().(func(*RuleSourceTransaction, SchemaSummaryReadReceipt, any) bool)
			if !admitOK || admit == nil || !admit(transaction, receipt, operand) {
				return false
			}
			placed[index] = transaction.reads[len(transaction.reads)-1]
		default:
			return false
		}
	}
	if shape.CarryCount == 1 {
		if !transaction.AddCarry() {
			return false
		}
	} else if shape.CarryCount != 0 {
		return false
	}
	writeShape, writeOK := state.schema.ruleWriteShapeAt(ordinal, 0)
	if !writeOK {
		return false
	}
	switch writeShape.Kind {
	case composition.WriteExact:
		local, projected := hot.projectWrite(operand)
		return projected && hot.output != nil && hot.output.schemaFactorAdmitExactWrite(state, authority, transaction, local)
	case composition.WriteRoute:
		receipt, receiptOK := implementation.RouteWriteReceipt()
		return receiptOK && AddAnchoredRouteWrite(transaction, receipt)
	default:
		return false
	}
}

func (implementation *RuleImplementation[K, V, O]) issueDraft(assembly *ReceiptAssembly, occurrence RuleOccurrenceReceipt, source RuleSurfaceSourceReceipt) bool {
	if implementation == nil || !implementation.receipt.valid() || assembly == nil {
		return false
	}
	shape, shapeOK := implementation.receipt.state.schema.ruleShapeAt(implementation.receipt.proof.ordinal)
	if !shapeOK {
		return false
	}
	draft, draftOK := implementation.BeginReceiptRuleRow(source)
	if !draftOK {
		return false
	}
	for index := uint64(0); index < shape.ReadCount; index++ {
		part, partOK := implementation.ReceiptReadPart(source, index)
		if !partOK || !draft.AddRead(part) {
			return false
		}
	}
	if shape.CarryCount == 1 {
		part, partOK := implementation.ReceiptCarryPart(source, 0)
		if !partOK || !draft.AddCarry(part) {
			return false
		}
	}
	write, writeOK := implementation.ReceiptWritePart(source, 0)
	if !writeOK || !draft.AddWrite(write) {
		return false
	}
	_, added := assembly.AddRuleFromDraft(occurrence, draft)
	return added
}
