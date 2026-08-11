package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

const instanceOperandEntityVersion uint64 = 1

// OperandEntity is the opaque equation-owned identity of one typed Rule
// operand. Domains may compare canonical content only; the source encoding
// version remains private to the engine/equation boundary.
type OperandEntity struct{ key composition.Key }

func (entity OperandEntity) MatchesContentDigest(digest [32]byte) bool {
	return digest != [32]byte{} && entity.key.Available() && entity.key.Version == instanceOperandEntityVersion &&
		[32]byte(entity.key.ID) == digest
}

func operandEntityForContent(digest [32]byte) (composition.Key, bool) {
	entity := composition.Key{ID: composition.ID(digest), Version: instanceOperandEntityVersion}
	return entity, digest != [32]byte{} && entity.Available()
}

func canonicalInstanceOperand[V, O any](instance *RuleInstance[V, O]) (O, [32]byte, bool) {
	var zero O
	if instance == nil || instance.rule == nil || instance.rule.operandContent == nil {
		return zero, [32]byte{}, false
	}
	frozen, digest, ok := instance.rule.operandContent(instance.operand)
	return frozen, digest, ok && digest != [32]byte{} && digest == instance.content
}

// admitInstanceOperandCore is the ordinary Rule-operand admission path used
// by SourceAssembly.PrepareInstance. The equation entity is derived from the
// Rule-owned typed operand content; a caller cannot pair an independently
// named identity with O.
func admitInstanceOperandCore[V, O any](admission sourceAdmissionCore, occurrence equation.Occurrence, instance *RuleInstance[V, O]) (equation.Operand, bool) {
	if instance == nil || instance.state == nil {
		return equation.Operand{}, false
	}
	state := instance.state
	state.admitMu.Lock()
	if state.admitClosed {
		state.admitMu.Unlock()
		return equation.Operand{}, false
	}
	if state.admitBusy {
		state.admitOverlap = true
		state.admitMu.Unlock()
		admission.reject()
		return equation.Operand{}, false
	}
	state.admitBusy = true
	state.admitMu.Unlock()

	frozen, digest, ok := canonicalInstanceOperand(instance)
	if !ok || instance.kind != ruleInstanceDirect && instance.kind != ruleInstanceActivation || admission.batch == nil {
		instance.closeAdmission(nil)
		admission.reject()
		return equation.Operand{}, false
	}
	entity, ok := operandEntityForContent(digest)
	if !ok {
		instance.closeAdmission(nil)
		admission.reject()
		return equation.Operand{}, false
	}
	// Occurrence.Available is a sealed-phase predicate. AdmitOperand is the
	// sole open-phase authority: it proves that occurrence belongs to this
	// exact live Batch before admitting the derived entity.
	operand, ok := admission.admitOperand(occurrence, entity)
	if !ok {
		instance.closeAdmission(nil)
		return equation.Operand{}, false
	}
	record := &ruleInstanceAdmission{batch: admission.batch, occurrence: occurrence, operand: operand}
	state.admitMu.Lock()
	accepted := state.admitBusy && !state.admitClosed && !state.admitOverlap
	state.admitBusy = false
	state.admitClosed = true
	if accepted {
		instance.operand = frozen
		state.admit = record
	}
	state.admitMu.Unlock()
	if !accepted {
		admission.reject()
		return equation.Operand{}, false
	}
	return operand, true
}

// admitInstanceOperand remains private engine-law plumbing. Production source
// construction passes through admitInstanceOperandCore and its shared source
// admission core; this wrapper exists only for same-package equation laws.
func admitInstanceOperand[V, O any](batch *equation.Batch, occurrence equation.Occurrence, instance *RuleInstance[V, O]) (equation.Operand, bool) {
	return admitInstanceOperandCore(newSourceAdmissionCore(batch), occurrence, instance)
}

func (instance *RuleInstance[V, O]) closeAdmission(record *ruleInstanceAdmission) {
	if instance == nil || instance.state == nil {
		return
	}
	state := instance.state
	state.admitMu.Lock()
	state.admitBusy = false
	state.admitClosed = true
	if record != nil && !state.admitOverlap {
		state.admit = record
	}
	state.admitMu.Unlock()
}

func matchesInstanceAdmission[V, O any](instance *RuleInstance[V, O], batch *equation.Batch, occurrence equation.Occurrence, operand equation.Operand) bool {
	if instance == nil || instance.state == nil || batch == nil {
		return false
	}
	state := instance.state
	state.admitMu.Lock()
	defer state.admitMu.Unlock()
	if state.admit == nil || state.admit.batch != batch {
		return false
	}
	if batch.Sealed() {
		return state.admit.occurrence.Same(occurrence) && state.admit.operand.Same(operand)
	}
	return state.admit.occurrence.SameOpen(occurrence) && state.admit.operand.SameOpen(operand)
}

// admitStructuralOperand is the sole source-facade operand admission for an
// output-free Support rule. Its entity is source-authored topology identity,
// not a Factor coordinate; the retained admission binds the exact Batch,
// Occurrence, Operand, and StructuralInstance for later Assembly consumption.
func admitStructuralOperand(admission sourceAdmissionCore, occurrence equation.Occurrence, entity composition.Key, instance *StructuralInstance) (equation.Operand, bool) {
	if instance == nil || instance.state == nil {
		return equation.Operand{}, false
	}
	state := instance.state
	state.admitMu.Lock()
	if state.admitClosed {
		state.admitMu.Unlock()
		return equation.Operand{}, false
	}
	if state.admitBusy {
		state.admitOverlap = true
		state.admitMu.Unlock()
		admission.reject()
		return equation.Operand{}, false
	}
	state.admitBusy = true
	state.admitMu.Unlock()

	if admission.batch == nil || !entity.Available() || instance.rule == nil || instance.variant != nil || instance.rule.outputKind != ruleStructuralOutput ||
		instance.rule.output != nil || instance.rule.support == nil || instance.rule.activation != nil {
		closeStructuralAdmission(instance, nil)
		admission.reject()
		return equation.Operand{}, false
	}
	operand, ok := admission.admitOperand(occurrence, entity)
	if !ok {
		closeStructuralAdmission(instance, nil)
		return equation.Operand{}, false
	}
	record := &structuralInstanceAdmission{batch: admission.batch, occurrence: occurrence, operand: operand}
	state.admitMu.Lock()
	accepted := state.admitBusy && !state.admitClosed && !state.admitOverlap
	state.admitBusy = false
	state.admitClosed = true
	if accepted {
		state.admit = record
	}
	state.admitMu.Unlock()
	if !accepted {
		admission.reject()
		return equation.Operand{}, false
	}
	return operand, true
}

func closeStructuralAdmission(instance *StructuralInstance, record *structuralInstanceAdmission) {
	if instance == nil || instance.state == nil {
		return
	}
	state := instance.state
	state.admitMu.Lock()
	state.admitBusy = false
	state.admitClosed = true
	if record != nil && !state.admitOverlap {
		state.admit = record
	}
	state.admitMu.Unlock()
}

func matchesStructuralInstanceAdmission(instance *StructuralInstance, batch *equation.Batch, occurrence equation.Occurrence, operand equation.Operand) bool {
	if instance == nil || instance.state == nil || batch == nil {
		return false
	}
	state := instance.state
	state.admitMu.Lock()
	defer state.admitMu.Unlock()
	if state.admit == nil || state.admit.batch != batch {
		return false
	}
	if batch.Sealed() {
		return state.admit.occurrence.Same(occurrence) && state.admit.operand.Same(operand)
	}
	return state.admit.occurrence.SameOpen(occurrence) && state.admit.operand.SameOpen(operand)
}

// matchesInstancePrototypeRow is the sealed-template counterpart of
// matchesInstanceAdmission.  PrototypeRow keeps its Batch private, so this
// compares the two issued opaque handles directly; Same includes their exact
// source batch authority.  A matching content digest alone is never enough to
// attach a typed payload.
func matchesInstancePrototypeRow[V, O any](instance *RuleInstance[V, O], row equation.PrototypeRow) bool {
	if instance == nil || instance.state == nil || !row.Available() {
		return false
	}
	state := instance.state
	state.admitMu.Lock()
	defer state.admitMu.Unlock()
	return state.admit != nil && state.admit.occurrence.Same(row.Occurrence()) && state.admit.operand.Same(row.Operand())
}
