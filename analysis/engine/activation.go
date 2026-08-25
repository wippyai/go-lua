package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/lifetime"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ActivationLocator is one immutable semantic relation requested by an
// activation Fold. It carries no topology authority or resolved Member.
type ActivationLocator struct {
	application   identity.SemanticKey
	target        identity.SemanticKey
	endpoint      identity.SemanticKey
	transitionID  identity.ContentID
	fromContextID identity.ContentID
	toContextID   identity.ContentID
}

// NewActivationLocator constructs one pure semantic activation relation.
func NewActivationLocator(application, target, endpoint identity.SemanticKey) (ActivationLocator, bool) {
	if !application.Available() || !target.Available() || !endpoint.Available() {
		return ActivationLocator{}, false
	}
	return ActivationLocator{application: application, target: target, endpoint: endpoint}, true
}

func (locator ActivationLocator) contextual() equation.ActivationContext {
	return equation.ActivationContext{TransitionID: locator.transitionID, FromContextID: locator.fromContextID, ToContextID: locator.toContextID}
}

// ActivationResult is the opaque result of one activation Product row. Its
// zero value is invalid; Fold must return Activated, including for zero rows.
type ActivationResult struct {
	execution *activationExecution
	epoch     identity.Generation
	call      identity.Generation
	row       int
	locators  []ActivationLocator
}

// ActivationFrame is the read-only frame passed to an activation Fold. It can
// resolve declared typed reads but carries no topology or publication power.
type ActivationFrame struct {
	execution *activationExecution
	epoch     identity.Generation
	call      identity.Generation
	row       int
	region    support.Mask
}

func (value ActivationFrame) live() bool {
	return value.execution != nil && value.epoch.Available() && value.execution.epoch == value.epoch &&
		value.execution.active.Holds(value.epoch) && value.execution.activeCall.Holds(value.call) &&
		value.row >= 0 && value.execution.row == value.row && value.region.Valid() &&
		value.execution.frame != nil && value.execution.frame.active.Holds(value.epoch) &&
		value.execution.frame.product != nil && value.row < len(value.execution.frame.product.values) &&
		value.execution.region.Valid() && value.region.Manager() == value.execution.region.Manager() &&
		value.region.Entails(value.execution.region) && value.execution.frame.product.requireCheckpoint()
}

// Activated seals an immutable locator batch for the exact live frame. The
// engine resolves the complete batch against the sealed topology only after
// Fold returns, so partial publication is impossible.
func Activated(value ActivationFrame, locators ...ActivationLocator) ActivationResult {
	if !value.live() {
		return ActivationResult{}
	}
	result := ActivationResult{execution: value.execution, epoch: value.epoch, call: value.call, row: value.row}
	result.locators = append([]ActivationLocator(nil), locators...)
	for _, locator := range result.locators {
		if !locator.application.Available() || !locator.target.Available() || !locator.endpoint.Available() || !locator.contextual().WellFormed() {
			return ActivationResult{}
		}
	}
	return result
}

// ActivationApplication returns the exact application sealed on the current
// trigger binding. It is a projection of existing topology authority, not a
// reconstruction from caller facts, target descriptors, or product rows.
func ActivationApplication(value ActivationFrame) (identity.SemanticKey, bool) {
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
func ActivationReadValue[S any](value ActivationFrame, read Read[S]) (S, bool) {
	var zero S
	if !value.live() || value.execution.owner == nil || !read.matchesActivationOwner(value.execution.owner) || read.index < 0 || read.index >= len(value.execution.frame.product.reads) || read.resolve == nil {
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
	active     lifetime.Cell
	activeCall lifetime.Cell
	nextCall   lifetime.GenerationSequence
	// fromContextID is the exact mounted source context for this execution.
	// It is issued by the runtime state plan, never inferred from a module or
	// application key.
	fromContextID identity.ContentID
	row           int
	region        support.Mask
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
	// cell+ordinal are the sealed owner row. Runtime retains no copied Schema
	// semantic/output/read geometry; readCount is executable vector shape.
	cell        *schemaActivationRuleBindingCell
	ordinal     uint64
	readCount   uint64
	fold        func(*activationExecution) (ActivationResult, bool)
	topology    *equation.Topology
	trigger     composition.Key
	application identity.SemanticKey
	graph       *equation.Graph
	anchor      identity.SemanticKey
	reads       []readRuntime
	nextEpoch   lifetime.GenerationSequence
}

func (compiled *compiledActivationRule) executableInstance() bool {
	return compiled != nil && compiled.cell != nil && compiled.cell.state != nil &&
		compiled.cell.state.phase == schemaBindingSealed && compiled.cell.schema == compiled.cell.state.schema &&
		compiled.cell.ordinal == compiled.ordinal && uint64(len(compiled.reads)) == compiled.readCount &&
		compiled.fold != nil && compiled.topology != nil && compiled.trigger.Available() &&
		compiled.application.Available() && compiled.anchor.Available() && compiled.graph != nil
}

func (compiled *compiledActivationRule) runtimeRuleCell() schemaRuleBindingCell {
	if compiled == nil {
		return nil
	}
	return compiled.cell
}

func (compiled *compiledActivationRule) runtimeRuleOrdinal() uint64 {
	if compiled == nil {
		return 0
	}
	return compiled.ordinal
}

func (compiled *compiledActivationRule) declaredReadCount() uint64 {
	if compiled == nil {
		return 0
	}
	return compiled.readCount
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
	return compiled.executeAt(work, base, inputs, within, identity.ContentID{})
}

func (compiled *compiledActivationRule) executeAt(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask, fromContextID identity.ContentID) (selected []equation.AcceptedMember, reads []demand.Observation, accepted bool, boundary solveBoundary) {
	if !compiled.executableInstance() {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-instance")
	}
	if !fromContextID.Available() && fromContextID != (identity.ContentID{}) {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-context")
	}
	if work == nil || !work.OwnsRuleContributionStates(base, inputs) {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-contribution")
	}
	if uint64(len(compiled.reads)) != compiled.declaredReadCount() {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-reads")
	}
	epoch, issued := compiled.nextEpoch.Issue()
	if !issued {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-epoch")
	}
	frame := &ruleExecution{owner: compiled, work: work, base: base, epoch: epoch}
	frame.active.Open(epoch)
	execution := &activationExecution{owner: compiled, frame: frame, epoch: epoch, fromContextID: fromContextID, row: -1}
	execution.active.Open(epoch)
	defer func() {
		if frame.product != nil {
			frame.product.close()
		}
		frame.active.Revoke(epoch)
		execution.active.Revoke(epoch)
		if recover() != nil {
			selected, reads, accepted, boundary = nil, nil, false, refused(SolveFailureFamilyExecution, "preflight")
		}
	}()
	product, okay := newProductSession(frame, compiled.reads, work, inputs, within)
	if !okay || product == nil || !product.started.CompareAndSwap(false, true) {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "activation-product")
	}
	frame.product = product
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
		result, folded := compiled.fold(execution)
		if !folded || !product.requireCheckpoint() || frame.failed.Load() {
			return nil, nil, false, refused(SolveFailureFamilyExecution, "fold")
		}
		rowSelections, settled := compiled.settleActivationResult(execution, result, region)
		if !settled || !product.requireCheckpoint() {
			return nil, nil, false, refused(SolveFailureFamilyExecution, "result")
		}
		// Keep every row selection until the complete Product is known. A
		// Member selected on P and Q must be unioned as one support premise
		// before the one carrier-to-equation conversion below.
		selections = append(selections, rowSelections...)
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
	selected, valid := compiled.admitSelections(selections, product)
	if !valid {
		return nil, nil, false, refused(SolveFailureFamilyExecution, "publication")
	}
	return selected, reads, true, boundaryNone
}

func (compiled *compiledActivationRule) settleActivationResult(execution *activationExecution, result ActivationResult, region support.Mask) ([]activationSelection, bool) {
	if compiled == nil || execution == nil || execution.owner != compiled || execution.frame == nil || execution.frame.product == nil ||
		result.execution != execution || result.epoch != execution.epoch || result.row != execution.row || !result.call.Available() ||
		!region.Valid() || !execution.frame.product.requireCheckpoint() {
		return nil, false
	}
	locators, canonical := canonicalActivationLocators(result.locators)
	if !canonical {
		return nil, false
	}
	selections := make([]activationSelection, 0, len(locators))
	for _, locator := range locators {
		if !execution.frame.product.requireCheckpoint() {
			return nil, false
		}
		context := locator.contextual()
		pair := equation.PairLocator{
			Application: compositionKeyOf(locator.application),
			Target:      compositionKeyOf(locator.target),
			Endpoint:    compositionKeyOf(locator.endpoint),
			Context:     context,
		}
		var member equation.Member
		var selected bool
		if execution.fromContextID.Available() {
			member, selected = compiled.topology.SelectActivationMemberForContext(compiled.trigger, pair, execution.fromContextID)
		} else {
			member, selected = compiled.topology.SelectActivationMember(compiled.trigger, pair)
		}
		if !selected || !member.Available() {
			return nil, false
		}
		selections = append(selections, activationSelection{
			member: member, application: locator.application, target: locator.target, endpoint: locator.endpoint, row: execution.row, region: region,
		})
	}
	return selections, true
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

func canonicalActivationLocators(values []ActivationLocator) ([]ActivationLocator, bool) {
	result := append([]ActivationLocator(nil), values...)
	sort.Slice(result, func(left, right int) bool {
		if compositionKeyOf(result[left].application) != compositionKeyOf(result[right].application) {
			return lessRuntimeKey(compositionKeyOf(result[left].application), compositionKeyOf(result[right].application))
		}
		if compositionKeyOf(result[left].target) != compositionKeyOf(result[right].target) {
			return lessRuntimeKey(compositionKeyOf(result[left].target), compositionKeyOf(result[right].target))
		}
		if compositionKeyOf(result[left].endpoint) != compositionKeyOf(result[right].endpoint) {
			return lessRuntimeKey(compositionKeyOf(result[left].endpoint), compositionKeyOf(result[right].endpoint))
		}
		return activationLocatorContextKey(result[left]) < activationLocatorContextKey(result[right])
	})
	retained := 0
	for _, locator := range result {
		if !locator.application.Available() || !locator.target.Available() || !locator.endpoint.Available() || !locator.contextual().WellFormed() {
			return nil, false
		}
		if retained != 0 && compositionKeyOf(result[retained-1].application) == compositionKeyOf(locator.application) && compositionKeyOf(result[retained-1].target) == compositionKeyOf(locator.target) && compositionKeyOf(result[retained-1].endpoint) == compositionKeyOf(locator.endpoint) && result[retained-1].contextual() == locator.contextual() {
			continue
		}
		result[retained] = locator
		retained++
	}
	return result[:retained], true
}

func activationLocatorContextKey(locator ActivationLocator) string {
	context := locator.contextual()
	return context.TransitionID.String() + context.FromContextID.String() + context.ToContextID.String()
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
	return canonicalAcceptedMembers(compiled.topology, accepted)
}

// canonicalAcceptedMembers is the one canonical order and merge for a set of
// accepted activation members, shared by the hand lane above and the generated
// structural member.
//
// Sparse selected relations are canonicalized here. There is no bitmap,
// candidate ordinal, or eager family enumeration on this execution path. One
// member accepted twice is one member: the two premises are merged rather than
// published as two rows, because the same edge reached under two supports is
// still one edge.
func canonicalAcceptedMembers(topology *equation.Topology, accepted []equation.AcceptedMember) ([]equation.AcceptedMember, bool) {
	if topology == nil {
		return nil, false
	}
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
			merged, ok := topology.MergeAccepted(accepted[retained-1], record)
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

func retainActivationFold(compiled *compiledActivationRule, fold func(ActivationFrame) ActivationResult) func(*activationExecution) (ActivationResult, bool) {
	return func(execution *activationExecution) (ActivationResult, bool) {
		if compiled == nil || fold == nil || execution == nil || execution.owner != compiled || !execution.active.Holds(execution.epoch) || execution.frame == nil || execution.frame.product == nil || !execution.frame.product.requireCheckpoint() || execution.row < 0 {
			return ActivationResult{}, false
		}
		call, issued := execution.nextCall.Issue()
		if !issued || !execution.activeCall.Claim(call) {
			return ActivationResult{}, false
		}
		defer execution.activeCall.Revoke(call)
		result := fold(ActivationFrame{execution: execution, epoch: execution.epoch, call: call, row: execution.row, region: execution.region})
		return result, execution.frame.product.requireCheckpoint() && !execution.frame.failed.Load()
	}
}

// compileActivationRule is the generation-fenced activation compiler. It
// consumes the exact sealed activation cell and canonical ordinal.
func compileActivationRule(implementation *ActivationRuleImplementation, topology *equation.Topology, trigger composition.Key, graph *equation.Graph) (*compiledActivationRule, bool) {
	if implementation == nil {
		return nil, false
	}
	cell, cellOK := implementation.sealedActivationCell()
	if !cellOK {
		return nil, false
	}
	return compileActivationCellRule(cell, implementation.ordinal, topology, trigger, graph)
}

func compileActivationCellRule(cell *schemaActivationRuleBindingCell, ordinal uint64, topology *equation.Topology, trigger composition.Key, graph *equation.Graph) (*compiledActivationRule, bool) {
	if cell == nil || !cell.schemaRuleComplete() || topology == nil || graph == nil ||
		!topology.OwnsComposition(cell.schema.cold) || !topology.OwnsGraph(graph) || !trigger.Available() {
		return nil, false
	}
	state := cell.state
	shape, shapeOK := state.schema.ruleShapeAt(ordinal)
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
	readCount := shape.ReadCount
	if readCount > 1 {
		// The only activation read issuer is the one exact-read lane. Any
		// unsupported inventory must be rejected before a runtime row exists.
		return nil, false
	}
	compiled := &compiledActivationRule{
		cell:        cell,
		ordinal:     ordinal,
		readCount:   readCount,
		topology:    topology,
		trigger:     trigger,
		application: applicationSemantic,
		graph:       graph,
	}
	compiled.fold = retainActivationFold(compiled, cell.impl.fold)
	return compiled, true
}
