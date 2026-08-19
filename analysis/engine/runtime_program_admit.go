package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

func (implementation *RuleImplementation[K, V, O]) AdmitsMounted(mount, point, occurrence identity.ContentID) bool {
	if implementation == nil {
		return false
	}
	_, resolved := implementation.resolveOperand(OperandCoords{Mount: mount, Point: point, Occurrence: occurrence})
	return resolved
}

func (implementation *RuleImplementation[K, V, O]) AdmitMounted(builder *BindingTopologyBuilder, role RuleSlotCapability, mount, point, occurrence identity.ContentID) bool {
	if implementation == nil || builder == nil || !role.Mounted() {
		return false
	}
	operand, resolved := implementation.resolveOperand(OperandCoords{Mount: mount, Point: point, Occurrence: occurrence})
	if !resolved {
		return false
	}
	row, rowOK := builder.AdmitMountedRuleOccurrence(role, mount, point, occurrence)
	if !rowOK {
		return false
	}
	return implementation.admitRule(builder, role, row, operand, builder.QueueMountedRuleFinalizer)
}

func (implementation *RuleImplementation[K, V, O]) AdmitLink(builder *BindingTopologyBuilder, role RuleSlotCapability, occurrence identity.ContentID) bool {
	if implementation == nil || builder == nil || !role.Link() {
		return false
	}
	operand, resolved := implementation.resolveOperand(OperandCoords{Occurrence: occurrence})
	if !resolved {
		return false
	}
	row, rowOK := builder.AdmitLinkRuleOccurrence(role, occurrence)
	if !rowOK {
		return false
	}
	return implementation.admitRule(builder, role, row, operand, builder.QueueLinkRuleFinalizer)
}

func (implementation *RuleImplementation[K, V, O]) admitRule(builder *BindingTopologyBuilder, role RuleSlotCapability, occurrence ruleOccurrence, operand O, queue func(RuleSlotCapability, func() bool) bool) bool {
	return admitRuleScope(builder, implementation, occurrence, operand,
		func(transaction *RuleSourceTransaction) bool {
			return implementation.placeSurfaces(transaction, operand)
		},
		func(source ruleSurfaceSource) bool {
			return implementation.issueDraft(builder, occurrence, source)
		},
		nil, queue, role)
}

func (implementation *RuleImplementation[K, V, O]) placeSurfaces(transaction *RuleSourceTransaction, operand O) bool {
	if implementation == nil || !implementation.binding.valid() || implementation.binding.cell == nil || implementation.binding.cell.impl == nil || transaction == nil {
		return false
	}
	hot := implementation.binding.cell.impl
	state := implementation.binding.state
	authority := implementation.binding.authority
	ordinal := implementation.binding.proof.ordinal
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
			receipt, receiptOK := implementation.selectedRead(index)
			surface, surfaceOK := transaction.AnchoredSelectedReadSurface(receipt, deps)
			if !receiptOK || !surfaceOK || !transaction.AddRead(surface) {
				return false
			}
			placed[index] = surface
		case composition.ReadSummary:
			receipt, receiptOK := implementation.summaryRead(index)
			provider, providerOK := hot.reads[index].(interface{ summarySurfaceAdmit() any })
			if !receiptOK || !providerOK {
				return false
			}
			project, projectOK := provider.summarySurfaceAdmit().(func(any) (any, bool))
			if !projectOK || project == nil {
				return false
			}
			refs, refsOK := project(operand)
			if !refsOK || !addSummaryReadRefs(transaction, receipt, refs) {
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
		receipt, receiptOK := implementation.routeWrite()
		return receiptOK && AddAnchoredRouteWrite(transaction, receipt)
	default:
		return false
	}
}

func (implementation *RuleImplementation[K, V, O]) issueDraft(builder *BindingTopologyBuilder, occurrence ruleOccurrence, source ruleSurfaceSource) bool {
	if implementation == nil || !implementation.binding.valid() || builder == nil {
		return false
	}
	shape, shapeOK := implementation.binding.state.schema.ruleShapeAt(implementation.binding.proof.ordinal)
	if !shapeOK {
		return false
	}
	draft, draftOK := implementation.beginReceiptRuleRow(source)
	if !draftOK {
		return false
	}
	for index := uint64(0); index < shape.ReadCount; index++ {
		part, partOK := implementation.receiptReadPart(source, index)
		if !partOK || !draft.AddRead(part) {
			return false
		}
	}
	if shape.CarryCount == 1 {
		part, partOK := implementation.receiptCarryPart(source, 0)
		if !partOK || !draft.AddCarry(part) {
			return false
		}
	}
	write, writeOK := implementation.receiptWritePart(source, 0)
	if !writeOK || !draft.AddWrite(write) {
		return false
	}
	_, added := builder.AddRuleFromDraft(occurrence, draft)
	return added
}

func applyMountedProgramAdmission(builder *BindingTopologyBuilder, admission MountedProgramAdmission) (receiptSealFailure, ProgramAdmissionStage, bool) {
	if builder == nil {
		return receiptSealFailure{}, ProgramAdmissionNone, false
	}
	for _, row := range admission.Link {
		if row.Attach == nil || !row.Attach.AdmitLink(builder, row.Capability, row.Occurrence) {
			return receiptSealFailure{}, ProgramAdmissionLink, false
		}
	}
	// Admissibility is decided once, by the declared issuance requirement the
	// artifact placed the row under. A placement therefore guarantees the
	// owner seals an operand for it, and an owner that cannot refuses the
	// whole assemble rather than being silently skipped here.
	for _, row := range admission.Mounted {
		if row.Attach == nil || !row.Attach.AdmitMounted(builder, row.Capability, row.Mount, row.Point, row.Occurrence) {
			return receiptSealFailure{}, ProgramAdmissionMounted, false
		}
	}
	for _, row := range admission.Activation {
		if !AdmitMountedActivationOccurrence(builder, row) {
			return receiptSealFailure{}, ProgramAdmissionMounted, false
		}
	}
	queryRejected := false
	if len(admission.Queries) == 0 || !builder.QueueMountedQueryBatch(func(batch *MountedQueryBatch) bool {
		for _, row := range admission.Queries {
			if row.admit == nil || !row.admit.admitMountedQuery(batch, row.ID, row.Mount, row.Point) {
				queryRejected = true
				return false
			}
		}
		return true
	}) {
		return receiptSealFailure{}, ProgramAdmissionQuery, false
	}
	if !builder.SealSources() {
		if queryRejected {
			return receiptSealFailure{}, ProgramAdmissionQuery, false
		}
		failed, _ := builder.SealFailure()
		return failed, ProgramAdmissionSeal, false
	}
	return receiptSealFailure{}, ProgramAdmissionNone, true
}
