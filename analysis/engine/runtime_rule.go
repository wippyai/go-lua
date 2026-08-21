// runtime_rule.go declares the bound Rule and runs its execution and derivation.

package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

type boundRule[V, O any] struct {
	proof          *ruleRuntimeProof
	admission      RuleAdmission[V, O]
	anchor         identity.SemanticKey
	operandContent [32]byte
	coordinates    ActivationCoordinates
	transfer       func(Access[V, O]) bool
	operand        O
	reads          []readRuntime
	output         outputAccess[V]
	// routeScope retains only the owning Factor authority. Its O(R) target
	// vector and transformed-carry closure remain Factor/Binding-owned; this
	// member retains only authored carry surfaces plus the route bit.
	routeScope     runtimeFactor
	routeTransform bool
	carrySemantic  identity.SemanticKey
	carryTargets   []carrier.Target
	carryApply     func(V) (V, bool)
	carryOnly      bool
	nextEpoch      generationSequence
}

// ruleOperand is the private typed bridge used only while attaching a
// SchemaBinding selected read. It exposes the canonical bound operand to the
// locator without putting an operand into SelectorContext or retaining a
// mutable snapshot.
func (bound *boundRule[V, O]) ruleOperand() O {
	if bound == nil {
		var zero O
		return zero
	}
	return bound.operand
}

func (bound *boundRule[V, O]) transformedCarry() (identity.SemanticKey, []carrier.Target, func(V) (V, bool), bool) {
	if bound == nil || !bound.carrySemantic.Available() || bound.carryApply == nil {
		return identity.SemanticKey{}, nil, nil, false
	}
	return bound.carrySemantic, bound.carryTargets, bound.carryApply, true
}

func (bound *boundRule[V, O]) transformedCarryRoute() bool {
	return bound != nil && bound.routeTransform
}

func (bound *boundRule[V, O]) validCarryDisposition(disposition RuleDisposition[V]) bool {
	semantic, transformed := disposition.CarryTransform()
	if disposition.Kind() == RuleDispositionNoCandidate {
		return !transformed && !disposition.TransformOnly()
	}
	if disposition.Kind() != RuleDispositionStaged {
		return false
	}
	if bound.carrySemantic.Available() {
		return transformed && semantic == bound.carrySemantic
	}
	return !transformed && !disposition.TransformOnly()
}

func (bound *boundRule[V, O]) requiresDerivation() bool {
	return bound != nil && bound.admission.kind == ruleAdmissionDerivation
}

func (bound *boundRule[V, O]) initialReads() []demand.Observation {
	if bound == nil {
		return nil
	}
	result := make([]demand.Observation, 0, len(bound.reads))
	for _, read := range bound.reads {
		result = append(result, read.observations()...)
	}
	return result
}

func (bound *boundRule[V, O]) dynamicReads() []demand.DynamicRead {
	if bound == nil {
		return nil
	}
	result := make([]demand.DynamicRead, 0, len(bound.reads))
	for _, read := range bound.reads {
		if read != nil {
			result = append(result, read.dynamicReads()...)
		}
	}
	return result
}

type ruleExecution struct {
	owner   anyRule
	work    *carrier.Work
	base    carrier.RuleContributionBase
	inputs  []carrier.State
	epoch   identity.Generation
	active  generationCell
	failed  atomic.Bool
	product *productSession
	output  outputSession
}

// anyRule contains no typed Fact payload. It exists only to ensure an Access
// cannot be replayed against another canonical Rule-instance row.
type anyRule interface {
	requiresDerivation() bool
	runtimeRuleProof() *ruleRuntimeProof
}

func (bound *boundRule[V, O]) runtimeRuleProof() *ruleRuntimeProof {
	if bound == nil {
		return nil
	}
	return bound.proof
}

// readBinding is the one private E-side sink for a cold Rule's ordered typed
// read projection. Factor and structural support Rules use this exact path;
// only Factor Rules additionally install an output Patch session.
type readBinding interface {
	appendReadRuntime(readRuntime) bool
	runtimeRuleProof() *ruleRuntimeProof
}

// bindRuntimeRuleReads consumes a Rule's sealed positional read binders in
// Graph member order. The compiler supplies only the factor catalog; every
// typed normalization, equality, and selector callback remains schema-owned.
func (bound *boundRule[V, O]) appendReadRuntime(read readRuntime) bool {
	if bound == nil || read == nil || bound.proof == nil || !bound.proof.valid() || len(bound.reads) >= int(bound.proof.reads) {
		return false
	}
	bound.reads = append(bound.reads, read)
	return true
}

func (bound *boundRule[V, O]) execute(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) (carrier.Patch, []demand.Observation, bool, bool, solveBoundary) {
	if bound == nil || bound.transfer == nil || work == nil || !work.OwnsRuleContributionStates(base, inputs) {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	epoch, issued := bound.nextEpoch.issue()
	if !issued {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	execution := &ruleExecution{owner: bound, work: work, base: base, inputs: append([]carrier.State(nil), inputs...), epoch: epoch}
	execution.active.open(epoch)
	defer func() {
		if execution.output != nil {
			execution.output.discard()
		}
		if execution.product != nil {
			execution.product.close()
		}
		execution.active.revoke(epoch)
	}()
	product, ok := newProductSession(execution, bound.reads, work, inputs, within)
	if !ok {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	execution.product = product
	if !bound.carryOnly {
		execution.output = bound.output.begin(execution)
		if execution.output == nil {
			return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
		}
	}
	access := Access[V, O]{execution: execution, owner: bound, epoch: epoch, output: bound.output}
	transferred := bound.transfer(access)
	if !product.checkpoint() {
		return carrier.Patch{}, nil, false, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	if !transferred || execution.failed.Load() {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "transfer")
	}
	reads := product.observations()
	if !product.requireCheckpoint() {
		return carrier.Patch{}, nil, false, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	if !product.started.Load() || !bound.carryOnly && !execution.output.complete() {
		return carrier.Patch{}, nil, false, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	// An empty support intersection has no Product row and therefore no
	// semantic derivation or conclusion to admit.  It is a successful
	// structural no-op: retain the already-resolved read dependencies, but
	// mint no evidence, invoke no domain checker, allocate no Patch, and emit
	// no row/disposition coverage.  Nonempty Products continue through the
	// ordinary derivation/admission cut below.
	if product.rows.Count() == 0 {
		return carrier.Patch{}, reads, false, true, boundaryNone
	}
	derivation, ticket, derivationOK := bound.derivation(execution, reads)
	if !derivationOK {
		if !product.checkpoint() {
			return carrier.Patch{}, nil, false, false, stalled(SolveFailureFamilyExecution, "checkpoint")
		}
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "derivation")
	}
	defer ticket.invalidate()
	evidence, admitted := bound.admission.admit(derivation, bound.proof)
	if !execution.product.requireCheckpoint() {
		return carrier.Patch{}, nil, false, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	if !admitted {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "admission")
	}
	if bound.carryOnly {
		if !evidence.consume() {
			return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "publication")
		}
		return carrier.Patch{}, reads, false, true, boundaryNone
	}
	if !execution.output.hasStaged() {
		// An explicit all-omitted Product is a valid empty successor, not a
		// sparse Default write. It therefore consumes its admission evidence
		// but publishes no Factor patch and cannot prune structural support.
		if !evidence.consume() {
			return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "publication")
		}
		execution.output.discard()
		execution.output = nil
		return carrier.Patch{}, reads, false, true, boundaryNone
	}
	patch, ok := execution.output.accept(&evidence)
	execution.output = nil
	if !ok {
		return patch, reads, true, false, refused(SolveFailureFamilyExecution, "publication")
	}
	return patch, reads, true, true, boundaryNone
}

func (bound *boundRule[V, O]) derivation(execution *ruleExecution, reads []demand.Observation) (RuleDerivation[V, O], *ruleAdmissionTicket, bool) {
	if bound == nil || bound.proof == nil || !bound.proof.valid() || !bound.admission.same(bound.proof.admission) || execution == nil || execution.owner != bound || !bound.carryOnly && execution.output == nil || execution.product == nil || !execution.product.requireCheckpoint() || !execution.active.holds(execution.epoch) || !bound.anchor.Available() {
		return RuleDerivation[V, O]{}, nil, false
	}
	compositionID := bound.proof.compositionID()
	if !compositionID.Available() {
		return RuleDerivation[V, O]{}, nil, false
	}
	ticket := &ruleAdmissionTicket{proof: bound.proof, composition: compositionID, identity: bound.admission.identity, epoch: execution.epoch, anchor: bound.anchor, execution: execution, product: execution.product, live: true}
	// Trusted theorem admission has no checker-visible operands. Preserve all
	// runtime authority in its one live ticket without copying input, read, or
	// staged-result proof payloads.
	if !bound.requiresDerivation() {
		return RuleDerivation[V, O]{ticket: ticket}, ticket, true
	}
	inputs := make([]RuleInput, len(execution.inputs))
	for index, input := range execution.inputs {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[V, O]{}, nil, false
		}
		if !input.Valid() && !input.Same(execution.base.State()) {
			return RuleDerivation[V, O]{}, nil, false
		}
		inputs[index] = RuleInput{state: input}
	}
	proofReads := make([]RuleRead, len(reads))
	for index, read := range reads {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[V, O]{}, nil, false
		}
		proofReads[index] = RuleRead{input: read.Input, unit: read.Unit}
	}
	var dispositions []RuleDisposition[V]
	if bound.requiresDerivation() {
		if bound.carryOnly {
			dispositions = []RuleDisposition[V]{}
		} else {
			if bound.output.derivation == nil {
				return RuleDerivation[V, O]{}, nil, false
			}
			var okay bool
			dispositions, okay = bound.output.derivation(execution.output)
			if !okay {
				return RuleDerivation[V, O]{}, nil, false
			}
			if !validRuleDispositionCoverage(dispositions, len(execution.product.values)) {
				return RuleDerivation[V, O]{}, nil, false
			}
		}
	}
	for index := range dispositions {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[V, O]{}, nil, false
		}
		if dispositions[index].row.index != index || dispositions[index].row.index < 0 || dispositions[index].row.index >= len(execution.product.values) || !bound.validCarryDisposition(dispositions[index]) {
			return RuleDerivation[V, O]{}, nil, false
		}
		dispositions[index].row.ticket = ticket
		dispositions[index].ordinal = index
		for outputIndex := range dispositions[index].outputs {
			output := &dispositions[index].outputs[outputIndex]
			if output.ordinal != outputIndex || output.witness.ticket != nil {
				return RuleDerivation[V, O]{}, nil, false
			}
			output.witness = ruleRouteOutputWitness{ticket: ticket, row: index, ordinal: outputIndex}
		}
	}
	if !execution.product.requireCheckpoint() {
		return RuleDerivation[V, O]{}, nil, false
	}
	return RuleDerivation[V, O]{proof: bound.proof, composition: compositionID, identity: bound.admission.identity, epoch: execution.epoch, anchor: bound.anchor, operandContent: bound.operandContent, coordinates: bound.coordinates, inputs: inputs, reads: proofReads, dispositions: dispositions, product: execution.product, ticket: ticket, operand: bound.operand}, ticket, true
}

// validRuleDispositionCoverage makes a checker-visible derivation total over
// the Product that executed it: every row is represented once as either a
// staged result or an explicit no-candidate omission. A checker never has to
// infer whether a missing result was accidental.
func validRuleDispositionCoverage[V any](dispositions []RuleDisposition[V], rows int) bool {
	if rows < 0 || len(dispositions) != rows {
		return false
	}
	for index, disposition := range dispositions {
		if disposition.row.index != index || disposition.ordinal != index || (disposition.kind != RuleDispositionStaged && disposition.kind != RuleDispositionNoCandidate) {
			return false
		}
	}
	return true
}
