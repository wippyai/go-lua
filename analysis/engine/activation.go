package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ActivationResult is the checker-visible result of one activation Product
// row. It exposes the exact immutable locator selections, but never an
// equation Member, activation axes, family catalog, or topology capability.
type ActivationResult struct{ selections []activationLocator }

type activationLocator struct {
	application identity.SemanticKey
	target      identity.SemanticKey
	endpoint    identity.SemanticKey
}

// Activation is the synchronous capability passed to an ActivationRule Run.
// It can resolve declared typed reads and submit raw semantic locator keys;
// it cannot observe activation axes, enumerate members, or retain activation
// authority after the callback returns.
type Activation struct {
	execution *activationExecution
	epoch     identity.Generation
	call      identity.Generation
	row       int
	region    support.Mask
}

func (value Activation) live() bool {
	return value.execution != nil && value.epoch.Available() && value.execution.epoch == value.epoch &&
		value.execution.active.holds(value.epoch) && value.execution.activeCall.holds(value.call) &&
		value.row >= 0 && value.execution.row == value.row && value.region.Valid() &&
		value.execution.frame != nil && value.execution.frame.active.holds(value.epoch) &&
		value.execution.frame.product != nil && value.row < len(value.execution.frame.product.values) &&
		value.execution.region.Valid() && value.region.Manager() == value.execution.region.Manager() &&
		value.region.Entails(value.execution.region) && value.execution.frame.product.requireCheckpoint()
}

// Activate submits one exact application,target,endpoint relation for the
// current Product row. The frame converts these engine-owned semantic keys to
// a private locator and asks the sealed topology for that trigger's one
// binding. A false return is fail-closed when local constituent membership
// does not admit the triple.
func Activate(value Activation, application, target, endpoint identity.SemanticKey) bool {
	if !value.live() || !application.Available() || !target.Available() || !endpoint.Available() || value.execution.owner == nil || value.execution.owner.topology == nil || !value.execution.owner.trigger.Available() {
		if value.execution != nil && value.execution.frame != nil {
			value.execution.frame.failed.Store(true)
		}
		return false
	}
	member, selected := value.execution.owner.topology.SelectActivationMember(value.execution.owner.trigger, equation.PairLocator{
		Application: compositionKeyOf(application),
		Target:      compositionKeyOf(target),
		Endpoint:    compositionKeyOf(endpoint),
	})
	if !selected {
		value.execution.frame.failed.Store(true)
		return false
	}
	if !member.Available() {
		value.execution.frame.failed.Store(true)
		return false
	}
	value.execution.selected = append(value.execution.selected, activationSelection{
		member: member, application: application, target: target, endpoint: endpoint, row: value.row, region: value.region,
	})
	// Checker-visible selection is the semantic call triple; accepted evidence
	// retains the sole corresponding Member.
	value.execution.locators = append(value.execution.locators, activationLocator{application: application, target: target, endpoint: endpoint})
	return true
}

// ActivationApplication returns the exact application sealed on the current
// trigger binding. It is a projection of existing topology authority, not a
// reconstruction from caller facts, target descriptors, or product rows.
func ActivationApplication(value Activation) (identity.SemanticKey, bool) {
	if !value.live() || value.execution.owner == nil || !value.execution.owner.application.Available() {
		if value.execution != nil && value.execution.frame != nil {
			value.execution.frame.failed.Store(true)
		}
		return identity.SemanticKey{}, false
	}
	return value.execution.owner.application, true
}

// ActivationReadValue resolves one declared typed Read on the current
// activation Product row. It has the same stale-frame and exact-provenance
// checks as SupportReadValue, without granting a factor write surface.
func ActivationReadValue[S any](value Activation, read Read[S]) (S, bool) {
	var zero S
	if !value.live() || value.execution.owner == nil || !read.matchesRuntimeOwner(value.execution.owner) || read.index < 0 || read.index >= len(value.execution.frame.product.reads) || read.resolve == nil {
		if value.execution != nil && value.execution.frame != nil {
			value.execution.frame.failed.Store(true)
		}
		return zero, false
	}
	id, found := value.execution.frame.product.readID(value.row, read.index)
	if !found {
		value.execution.frame.failed.Store(true)
		return zero, false
	}
	result, ok := read.resolve(value.execution.frame.product, read.index, id)
	if !ok || !value.execution.frame.product.requireCheckpoint() {
		value.execution.frame.failed.Store(true)
		return zero, false
	}
	return result, true
}

type activationExecution struct {
	owner      *compiledActivationRule
	frame      *ruleExecution
	epoch      identity.Generation
	active     generationCell
	activeCall generationCell
	nextCall   generationSequence
	row        int
	region     support.Mask
	selected   []activationSelection
	locators   []activationLocator
}

type activationSelection struct {
	member      equation.Member
	application identity.SemanticKey
	target      identity.SemanticKey
	endpoint    identity.SemanticKey
	row         int
	region      support.Mask
}

type compiledActivationRule struct {
	proof       *ruleRuntimeProof
	schema      *Schema
	receipt     *ActivationRuleImplementation
	admission   RuleAdmission[ActivationResult, ruleUnit]
	run         func(*activationExecution) bool
	topology    *equation.Topology
	trigger     composition.Key
	application identity.SemanticKey
	graph       *equation.Graph
	anchor      identity.SemanticKey
	reads       []readRuntime
	nextEpoch   generationSequence
}

func (compiled *compiledActivationRule) executableInstance() bool {
	if compiled == nil || compiled.proof == nil || !compiled.proof.valid() || !compiled.admission.valid() || compiled.run == nil || compiled.topology == nil || !compiled.trigger.Available() || !compiled.application.Available() || !compiled.anchor.Available() || compiled.graph == nil {
		return false
	}
	if compiled.receipt != nil {
		shape, ok := compiled.proof.schema.ruleShapeAt(compiled.proof.ordinal)
		return compiled.schema == compiled.proof.schema && compiled.receipt.binding.valid() && compiled.proof.state != nil && compiled.proof.outputKind == composition.StructuralOutput && compiled.proof.output == (composition.Key{}) && ok && shape.ActivationFamily.Available()
	}
	return false
}

func (compiled *compiledActivationRule) runtimeRuleProof() *ruleRuntimeProof {
	if compiled == nil {
		return nil
	}
	return compiled.proof
}

func (compiled *compiledActivationRule) declaredReadCount() uint64 {
	if compiled == nil {
		return 0
	}
	if compiled.receipt != nil {
		return compiled.proof.reads
	}
	return 0
}

func (compiled *compiledActivationRule) requiresDerivation() bool {
	return compiled != nil && compiled.admission.kind == ruleAdmissionDerivation
}

func (compiled *compiledActivationRule) appendReadRuntime(read readRuntime) bool {
	if compiled == nil || read == nil || uint64(len(compiled.reads)) >= compiled.declaredReadCount() {
		return false
	}
	compiled.reads = append(compiled.reads, read)
	return true
}

func (compiled *compiledActivationRule) initialReads() []demand.Observation {
	if compiled == nil {
		return nil
	}
	result := make([]demand.Observation, 0, len(compiled.reads))
	for _, read := range compiled.reads {
		if read != nil {
			result = append(result, read.observations()...)
		}
	}
	return result
}

// dynamicReads exposes only the sealed source-slot permissions for staged
// exact reads. The concrete Unit routes remain an epoch-local result of the
// Product; they are never compiled into this activation rule.
func (compiled *compiledActivationRule) dynamicReads() []demand.DynamicRead {
	if compiled == nil {
		return nil
	}
	result := make([]demand.DynamicRead, 0)
	for _, read := range compiled.reads {
		if read != nil {
			result = append(result, read.dynamicReads()...)
		}
	}
	return result
}

func (compiled *compiledActivationRule) execute(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) (selected []equation.AcceptedMember, reads []demand.Observation, accepted bool, boundary solveBoundary) {
	if !compiled.executableInstance() {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-instance")
	}
	if work == nil || !work.OwnsRuleContributionStates(base, inputs) {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-contribution")
	}
	if uint64(len(compiled.reads)) != compiled.declaredReadCount() {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-reads")
	}
	epoch, issued := compiled.nextEpoch.issue()
	if !issued {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-epoch")
	}
	frame := &ruleExecution{owner: compiled, work: work, base: base, inputs: append([]carrier.State(nil), inputs...), epoch: epoch}
	frame.active.open(epoch)
	execution := &activationExecution{owner: compiled, frame: frame, epoch: epoch, row: -1}
	execution.active.open(epoch)
	defer func() {
		if frame.product != nil {
			frame.product.close()
		}
		frame.active.revoke(epoch)
		execution.active.revoke(epoch)
		if recover() != nil {
			selected, reads, accepted, boundary = nil, nil, false, refused(SolveFailureFamilyExecution, "preflight")
		}
	}()
	product, okay := newProductSession(frame, compiled.reads, work, inputs, within)
	if !okay || product == nil || !product.started.CompareAndSwap(false, true) {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-product")
	}
	frame.product = product
	var dispositions []RuleDisposition[ActivationResult]
	if compiled.requiresDerivation() {
		dispositions = make([]RuleDisposition[ActivationResult], 0, product.rows.Count())
	}
	selections := make([]activationSelection, 0)
	for index := 0; index < product.rows.Count(); index++ {
		if !product.requireCheckpoint() {
			return nil, nil, false, stalled(SolveFailureFamilyExecution, "checkpoint")
		}
		region, rowOK := product.rows.At(index)
		if !rowOK {
			return nil, nil, false, refused(SolveFailureFamilyExecution, "preflight")
		}
		execution.row, execution.region = index, region
		execution.selected = execution.selected[:0]
		execution.locators = execution.locators[:0]
		if !compiled.run(execution) || !product.requireCheckpoint() || frame.failed.Load() {
			return nil, nil, false, refused(SolveFailureFamilyExecution, "transfer")
		}
		// Keep every row selection until the complete Product is known. A
		// Member selected on P and Q must be unioned as one support premise
		// before the one carrier-to-equation conversion below.
		selections = append(selections, execution.selected...)
		if compiled.requiresDerivation() {
			locators, canonicalLocators := canonicalActivationLocators(execution.locators)
			if !canonicalLocators {
				return nil, nil, false, refused(SolveFailureFamilyExecution, "derivation")
			}
			dispositions = append(dispositions, RuleDisposition[ActivationResult]{kind: RuleDispositionStaged, value: ActivationResult{selections: locators}, guard: RuleGuard{mask: region}, row: ruleResultRow{index: index}, ordinal: index})
		}
	}
	reads = product.observations()
	if !product.requireCheckpoint() {
		return nil, nil, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	var canonical bool
	selections, canonical = canonicalActivationSelections(selections, product.requireCheckpoint)
	if !canonical {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "derivation")
	}
	derivation, ticket, valid := compiled.derivation(frame, reads, dispositions)
	if !valid {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "derivation")
	}
	defer ticket.invalidate()
	evidence, admitted := compiled.admission.admit(derivation, ticket.proof)
	if !product.requireCheckpoint() {
		return nil, nil, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	if !admitted {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "admission")
	}
	selected, valid = compiled.admitSelections(selections, product)
	if !valid {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "admission")
	}
	if !evidence.consume() {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "publication")
	}
	return selected, reads, true, boundaryNone
}

func canonicalActivationSelections(values []activationSelection, checkpoint func() bool) ([]activationSelection, bool) {
	result := append([]activationSelection(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		comparison, comparable := result[left].member.Compare(result[right].member)
		return comparable && comparison < 0
	})
	retained := 0
	for _, selection := range result {
		if checkpoint != nil && !checkpoint() {
			return nil, false
		}
		if !selection.member.Available() || !selection.application.Available() || !selection.target.Available() || !selection.endpoint.Available() || selection.row < 0 || !selection.region.Valid() {
			return nil, false
		}
		if retained != 0 && result[retained-1].member.Same(selection.member) {
			if result[retained-1].application != selection.application || result[retained-1].target != selection.target || result[retained-1].endpoint != selection.endpoint {
				return nil, false
			}
			merged, ok := support.UnionWithCheckpoint(checkpoint, result[retained-1].region, selection.region)
			if !ok {
				return nil, false
			}
			result[retained-1].region = merged
			continue
		}
		result[retained] = selection
		retained++
	}
	return result[:retained], true
}

func canonicalActivationLocators(values []activationLocator) ([]activationLocator, bool) {
	result := append([]activationLocator(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		if compositionKeyOf(result[left].application) != compositionKeyOf(result[right].application) {
			return lessRuntimeKey(compositionKeyOf(result[left].application), compositionKeyOf(result[right].application))
		}
		if compositionKeyOf(result[left].target) != compositionKeyOf(result[right].target) {
			return lessRuntimeKey(compositionKeyOf(result[left].target), compositionKeyOf(result[right].target))
		}
		return lessRuntimeKey(compositionKeyOf(result[left].endpoint), compositionKeyOf(result[right].endpoint))
	})
	retained := 0
	for _, locator := range result {
		if !locator.application.Available() || !locator.target.Available() || !locator.endpoint.Available() {
			return nil, false
		}
		if retained != 0 && compositionKeyOf(result[retained-1].application) == compositionKeyOf(locator.application) && compositionKeyOf(result[retained-1].target) == compositionKeyOf(locator.target) && compositionKeyOf(result[retained-1].endpoint) == compositionKeyOf(locator.endpoint) {
			continue
		}
		result[retained] = locator
		retained++
	}
	return result[:retained], true
}

func (compiled *compiledActivationRule) admitSelections(values []activationSelection, product *productSession) ([]equation.AcceptedMember, bool) {
	if compiled == nil || compiled.topology == nil || product == nil || !product.requireCheckpoint() {
		return nil, false
	}
	accepted := make([]equation.AcceptedMember, 0, len(values))
	for _, selection := range values {
		if selection.row < 0 || !selection.region.Valid() || !selection.member.Available() {
			return nil, false
		}
		premise, premiseOK := activationPremise(compiled.graph, selection.region, product.requireCheckpoint)
		if !premiseOK || !product.requireCheckpoint() {
			return nil, false
		}
		record, admitted := compiled.topology.Accept(selection.member, premise)
		if !admitted {
			return nil, false
		}
		if record.Available() {
			accepted = append(accepted, record)
		}
	}
	// Sparse selected relations are canonicalized here. There is no bitmap,
	// candidate ordinal, or eager family enumeration on this execution path.
	sort.Slice(accepted, func(left, right int) bool {
		comparison, comparable := accepted[left].Member().Compare(accepted[right].Member())
		return comparable && comparison < 0
	})
	retained := 0
	for _, record := range accepted {
		if !record.Available() {
			return nil, false
		}
		if retained != 0 && accepted[retained-1].Member().Same(record.Member()) {
			merged, ok := compiled.topology.MergeAccepted(accepted[retained-1], record)
			if !ok {
				return nil, false
			}
			accepted[retained-1] = merged
			continue
		}
		accepted[retained] = record
		retained++
	}
	return accepted[:retained], true
}

// activationPremise transposes one exact support BDD into the equation's
// canonical decision DAG. The walk is iterative and linear in the reachable
// BDD: the guard Manager's Rank is the only decision-order authority, graph
// DecisionAt fixes the only atom-to-decision interpretation, and
// NewExprDAG seals the already postordered reduced DAG without quadratic
// repeated Expr imports.
func activationPremise(graph *equation.Graph, region support.Mask, checkpoint func() bool) (equation.Expr, bool) {
	if graph == nil || !region.Valid() || region.Manager() == nil || checkpoint != nil && !checkpoint() {
		return equation.Expr{}, false
	}
	manager := region.Manager()
	root, rootOK := region.Guard()
	if !rootOK {
		return equation.Expr{}, false
	}
	type frame struct {
		guard guard.Guard
		ready bool
	}
	values := make(map[guard.Guard]uint32)
	rows := make([]equation.ExprNode, 0)
	stack := []frame{{guard: root}}
	for len(stack) != 0 {
		if checkpoint != nil && !checkpoint() {
			return equation.Expr{}, false
		}
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if _, done := values[current.guard]; done {
			continue
		}
		view, valid := manager.Decompose(current.guard)
		if !valid {
			return equation.Expr{}, false
		}
		if view.Terminal {
			if view.Value {
				values[current.guard] = 1
			} else {
				values[current.guard] = 0
			}
			continue
		}
		if !current.ready {
			stack = append(stack, frame{guard: current.guard, ready: true}, frame{guard: view.High}, frame{guard: view.Low})
			continue
		}
		low, lowOK := values[view.Low]
		high, highOK := values[view.High]
		rank, rankOK := manager.Rank(view.Atom)
		if !rankOK {
			return equation.Expr{}, false
		}
		decision, decisionOK := graph.DecisionAt(int(rank))
		if !lowOK || !highOK || !decisionOK || !decision.Available() {
			return equation.Expr{}, false
		}
		rows = append(rows, equation.ExprNode{Decision: decision, Low: low, High: high})
		values[current.guard] = uint32(len(rows) + 1)
	}
	ordinal, found := values[root]
	if !found {
		return equation.Expr{}, false
	}
	return equation.NewExprDAGWithCheckpoint(rows, ordinal, checkpoint)
}

func retainActivationRunReceipt(compiled *compiledActivationRule, run func(Activation) bool) func(*activationExecution) bool {
	return func(execution *activationExecution) bool {
		if compiled == nil || compiled.receipt == nil || run == nil || execution == nil || execution.owner != compiled || !execution.active.holds(execution.epoch) || execution.frame == nil || execution.frame.product == nil || !execution.frame.product.requireCheckpoint() || execution.row < 0 {
			return false
		}
		call, issued := execution.nextCall.issue()
		if !issued || !execution.activeCall.claim(call) {
			return false
		}
		defer execution.activeCall.revoke(call)
		result := run(Activation{execution: execution, epoch: execution.epoch, call: call, row: execution.row, region: execution.region})
		return result && execution.frame.product.requireCheckpoint() && !execution.frame.failed.Load()
	}
}

func (compiled *compiledActivationRule) derivation(execution *ruleExecution, reads []demand.Observation, dispositions []RuleDisposition[ActivationResult]) (RuleDerivation[ActivationResult, ruleUnit], *ruleAdmissionTicket, bool) {
	if compiled == nil || execution == nil || execution.owner != compiled || execution.product == nil || !execution.product.requireCheckpoint() || !execution.active.holds(execution.epoch) || !compiled.anchor.Available() || !compiled.admission.valid() || compiled.receipt == nil {
		return RuleDerivation[ActivationResult, ruleUnit]{}, nil, false
	}
	proof := compiled.proof
	if proof == nil || !proof.valid() || !compiled.admission.same(proof.admission) {
		return RuleDerivation[ActivationResult, ruleUnit]{}, nil, false
	}
	compositionID := proof.compositionID()
	ticket := &ruleAdmissionTicket{proof: proof, composition: compositionID, identity: compiled.admission.identity, epoch: execution.epoch, anchor: compiled.anchor, execution: execution, product: execution.product, live: true}
	if !compiled.requiresDerivation() {
		return RuleDerivation[ActivationResult, ruleUnit]{ticket: ticket}, ticket, true
	}
	inputs := make([]RuleInput, len(execution.inputs))
	for index, input := range execution.inputs {
		if !execution.product.requireCheckpoint() || !input.Valid() && !input.Same(execution.base.State()) {
			return RuleDerivation[ActivationResult, ruleUnit]{}, nil, false
		}
		inputs[index] = RuleInput{state: input}
	}
	proofReads := make([]RuleRead, len(reads))
	for index, read := range reads {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[ActivationResult, ruleUnit]{}, nil, false
		}
		proofReads[index] = RuleRead{input: read.Input, unit: read.Unit}
	}
	if !validRuleDispositionCoverage(dispositions, len(execution.product.values)) {
		return RuleDerivation[ActivationResult, ruleUnit]{}, nil, false
	}
	for index := range dispositions {
		if !execution.product.requireCheckpoint() || dispositions[index].row.index != index || dispositions[index].row.index < 0 || dispositions[index].row.index >= len(execution.product.values) {
			return RuleDerivation[ActivationResult, ruleUnit]{}, nil, false
		}
		dispositions[index].row.ticket = ticket
		dispositions[index].ordinal = index
	}
	return RuleDerivation[ActivationResult, ruleUnit]{proof: proof, composition: compositionID, identity: compiled.admission.identity, epoch: execution.epoch, anchor: compiled.anchor, inputs: inputs, reads: proofReads, dispositions: dispositions, product: execution.product, ticket: ticket}, ticket, execution.product.requireCheckpoint()
}

// compileActivationRuleReceipt is the receipt-native activation compiler for a
// compiler. It consumes the exact SchemaBinding proof and graph-owned trigger
// member; it never reconstructs a declaration-shaped rule.
func compileActivationRuleReceipt(implementation *ActivationRuleImplementation, topology *equation.Topology, trigger composition.Key, graph *equation.Graph) (*compiledActivationRule, bool) {
	if implementation == nil || !implementation.binding.valid() || topology == nil || graph == nil ||
		!topology.OwnsComposition(implementation.binding.proof.schema.cold) || !topology.OwnsGraph(graph) || !trigger.Available() {
		return nil, false
	}
	proof := implementation.binding.proof
	shape, shapeOK := proof.schema.ruleShapeAt(proof.ordinal)
	if !shapeOK || shape.OutputKind != composition.StructuralOutput || shape.ActivationCount != 1 || !shape.ActivationFamily.Available() {
		return nil, false
	}
	if !topology.TriggerBound(trigger, shape.ActivationFamily) {
		return nil, false
	}
	application, projected := topology.ActivationApplication(trigger, shape.ActivationFamily)
	if !projected {
		return nil, false
	}
	applicationSemantic, applicationSemanticOK := semanticKeyFromComposition(application)
	if !applicationSemanticOK {
		return nil, false
	}
	compiled := &compiledActivationRule{proof: proof, schema: proof.schema, receipt: implementation, admission: implementation.binding.cell.impl.admission, topology: topology, trigger: trigger, application: applicationSemantic, graph: graph}
	compiled.run = retainActivationRunReceipt(compiled, implementation.binding.cell.impl.run)
	return compiled, true
}

// AdmitActivationByTrustedTheorem names the reviewed receipt-native theorem
// used by mounted activation Rule slots. It carries no declaration
// authority or Composition dependency.
func AdmitActivationByTrustedTheorem(identity identity.SemanticKey) RuleAdmission[ActivationResult, ruleUnit] {
	return AdmitRuleByTrustedTheorem[ActivationResult, ruleUnit](identity)
}
