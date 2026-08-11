package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// Support is the synchronous support subset capability passed to the one
// callback owned by a sealed SupportRule. Its fields are private: D can name
// it in a callback signature but cannot construct a support, a guard, or a
// carrier. E issues it only for the rule's declared completion/prune pair.
// A copied Support becomes invalid once that callback returns.
type Support struct {
	execution *supportExecution
	epoch     uint64
	call      uint64
	row       int
	region    support.Mask
}

// Empty returns the exact empty subset of this callback input. It is a
// contradiction in the shared support algebra, not a second reachability
// representation or a sentinel value.
func (value Support) Empty() (Support, bool) {
	if !value.live() {
		return Support{}, false
	}
	empty, ok := support.FromGuard(value.region.Manager(), value.region.Manager().False())
	if !ok {
		return Support{}, false
	}
	return value.retain(empty)
}

// retain is the sole runtime-owned subset issuer. It stays package-private so
// a declaration callback cannot manufacture a guard or bypass the shared
// support representation; later source lowering may hand an already-proved
// subset through this one cut.
func (value Support) retain(region support.Mask) (Support, bool) {
	if !value.live() || !region.Valid() || region.Manager() != value.region.Manager() || !region.Entails(value.region) {
		return Support{}, false
	}
	return Support{execution: value.execution, epoch: value.epoch, call: value.call, row: value.row, region: region}, true
}

func (value Support) live() bool {
	return value.execution != nil && value.epoch != 0 && value.execution.epoch == value.epoch &&
		value.execution.active.Load() == value.epoch && value.region.Valid() &&
		value.execution.frame != nil && value.execution.frame.active.Load() == value.epoch &&
		value.call != 0 && value.execution.activeCall.Load() == value.call &&
		value.row >= 0 && value.execution.row == value.row && value.execution.frame.product != nil &&
		value.row < len(value.execution.frame.product.values) &&
		value.execution.input.Valid() && value.region.Manager() == value.execution.input.Manager() &&
		value.region.Entails(value.execution.input) && value.execution.frame.product.requireCheckpoint()
}

// supportExecution is the private synchronous frame for one support Rule. It
// borrows the one Rule Product frame for typed reads, but exposes neither a
// Factor output nor a carrier patch surface.
type supportExecution struct {
	owner      *compiledSupportRule
	frame      *ruleExecution
	epoch      uint64
	active     atomic.Uint64
	activeCall atomic.Uint64
	nextCall   atomic.Uint64
	row        int
	input      support.Mask
}

// SupportReadValue resolves one declared typed Read on the current structural
// Product row. It shares Factor Rules' existing Read resolver and cannot
// escape a synchronous Run callback.
func SupportReadValue[S any](value Support, read Read[S]) (S, bool) {
	var zero S
	if !value.live() || read.rule == nil || value.execution.owner == nil || read.rule != value.execution.owner.rule || read.index < 0 || read.index >= len(value.execution.frame.product.reads) || read.resolve == nil {
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

// compiledSupportRule is the runtime form of exactly one immutable cold
// support Rule schema. Its schema carries the one declared completion/prune
// pair; no legacy declaration owner or parallel support authority exists.
type compiledSupportRule struct {
	rule      *ruleSchema
	admission RuleAdmission[Support, ruleUnit]
	run       func(*supportExecution) (support.Mask, bool)
	anchor    SemanticKey
	reads     []readRuntime
	nextEpoch atomic.Uint64
}

func (compiled *compiledSupportRule) executableInstance() bool {
	return compiled != nil && compiled.rule != nil && compiled.rule.support != nil && compiled.rule.output == nil && compiled.rule.inputs >= 0 &&
		compiled.rule.support.completion != nil && compiled.rule.support.prune != nil && compiled.rule.support.completion.prune == compiled.rule.support.prune && compiled.admission.same(compiled.rule.admission) && compiled.run != nil && compiled.anchor.Available() && compiled.rule.semantic.Available()
}

func (compiled *compiledSupportRule) ruleSchema() *ruleSchema {
	if compiled == nil {
		return nil
	}
	return compiled.rule
}

func (compiled *compiledSupportRule) requiresDerivation() bool {
	return compiled != nil && compiled.admission.kind == ruleAdmissionDerivation
}

// appendReadRuntime is the shared typed-read binding sink for an output-free
// structural Rule. The E binder still supplies the exact RuleMember surface;
// this type merely keeps the same private product projection used by Factor
// Rules, without granting a Patch/Carry/Write capability.
func (compiled *compiledSupportRule) appendReadRuntime(read readRuntime) bool {
	if compiled == nil || compiled.rule == nil || read == nil || len(compiled.reads) >= len(compiled.rule.reads) {
		return false
	}
	compiled.reads = append(compiled.reads, read)
	return true
}

func (compiled *compiledSupportRule) initialReads() []demand.Observation {
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
// Product; they are never compiled into this structural rule.
func (compiled *compiledSupportRule) dynamicReads() []demand.DynamicRead {
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

// execute is the one structural evaluator. It deliberately builds the same
// typed Product and RuleAdmission proof as a Factor Rule, but returns only a
// retained support result. The caller must place that result in the Group's
// already-open contribution; it cannot publish independently.
func (compiled *compiledSupportRule) execute(work *carrier.Work, base carrier.ContributionBase, inputs []carrier.State, within support.Mask) (retained support.Mask, reads []demand.Observation, accepted bool) {
	if !compiled.executableInstance() || work == nil || !work.OwnsContributionStates(base, inputs) || len(compiled.reads) != len(compiled.rule.reads) {
		return support.Mask{}, nil, false
	}
	epoch := compiled.nextEpoch.Add(1)
	if epoch == 0 {
		return support.Mask{}, nil, false
	}
	frame := &ruleExecution{owner: compiled, work: work, base: base, inputs: append([]carrier.State(nil), inputs...), epoch: epoch}
	frame.active.Store(epoch)
	execution := &supportExecution{owner: compiled, frame: frame, epoch: epoch}
	execution.active.Store(epoch)
	defer func() {
		if frame.product != nil {
			frame.product.close()
		}
		frame.active.CompareAndSwap(epoch, 0)
		execution.active.CompareAndSwap(epoch, 0)
		if recover() != nil {
			retained, reads, accepted = support.Mask{}, nil, false
		}
	}()
	product, okay := newProductSession(frame, compiled.reads, work, inputs, within)
	if !okay || product == nil {
		return support.Mask{}, nil, false
	}
	frame.product = product
	if !product.started.CompareAndSwap(false, true) {
		return support.Mask{}, nil, false
	}
	retained, okay = support.FromGuard(base.State().Support().Manager(), base.State().Support().Manager().False())
	if !okay {
		return support.Mask{}, nil, false
	}
	requiresDerivation := compiled.requiresDerivation()
	var dispositions []RuleDisposition[Support]
	if requiresDerivation {
		dispositions = make([]RuleDisposition[Support], 0, product.rows.Count())
	}
	for index := 0; index < product.rows.Count(); index++ {
		if !product.requireCheckpoint() {
			return support.Mask{}, nil, false
		}
		region, valid := product.rows.At(index)
		if !valid {
			return support.Mask{}, nil, false
		}
		execution.input = region
		execution.row = index
		pruned, valid := compiled.run(execution)
		if !product.requireCheckpoint() || !valid || frame.failed.Load() || !pruned.Valid() || !pruned.Entails(region) {
			return support.Mask{}, nil, false
		}
		if requiresDerivation {
			dispositions = append(dispositions, RuleDisposition[Support]{kind: RuleDispositionStaged, value: Support{execution: execution, epoch: epoch, row: index, region: pruned}, guard: RuleGuard{mask: pruned}, row: ruleResultRow{index: index}, ordinal: index})
		}
		if index == 0 {
			// The union identity is false, so the first row's retained support
			// is already the result without a disposable guard Work.
			retained = pruned
		} else {
			retained, valid = support.UnionWithCheckpoint(product.checkpoint, retained, pruned)
			if !valid {
				return support.Mask{}, nil, false
			}
		}
	}
	reads = product.observations()
	if !product.requireCheckpoint() {
		return support.Mask{}, nil, false
	}
	derivation, ticket, valid := compiled.derivation(frame, reads, dispositions)
	if !valid {
		return support.Mask{}, nil, false
	}
	defer ticket.invalidate()
	evidence, admitted := compiled.admission.admit(derivation, compiled.rule.composition, compiled.rule)
	if !product.requireCheckpoint() || !admitted || !evidence.consume() {
		return support.Mask{}, nil, false
	}
	return retained, reads, true
}

// retainSupportRun binds typed support behavior to the one immutable support
// Rule chosen at declaration. The closure accepts only its
// exact current input and proves synchronous provenance and subset inclusion
// before any runtime can publish the result.
func retainSupportRun(rule *ruleSchema, run func(Support) (Support, bool)) func(*supportExecution) (support.Mask, bool) {
	return func(execution *supportExecution) (support.Mask, bool) {
		if rule == nil || rule.support == nil || rule.support.completion == nil || rule.support.prune == nil || rule.support.completion.prune != rule.support.prune || run == nil || execution == nil || execution.owner == nil || execution.owner.rule != rule || execution.epoch == 0 || execution.active.Load() != execution.epoch || execution.frame == nil || execution.frame.product == nil || !execution.frame.product.requireCheckpoint() || !execution.input.Valid() {
			return support.Mask{}, false
		}
		call := execution.nextCall.Add(1)
		if call == 0 || !execution.activeCall.CompareAndSwap(0, call) {
			return support.Mask{}, false
		}
		defer execution.activeCall.CompareAndSwap(call, 0)
		result, ok := run(Support{execution: execution, epoch: execution.epoch, call: call, row: execution.row, region: execution.input})
		if !execution.frame.product.requireCheckpoint() || !ok || !result.live() || result.execution != execution || result.epoch != execution.epoch || !result.region.Entails(execution.input) {
			return support.Mask{}, false
		}
		return result.region, true
	}
}

func (compiled *compiledSupportRule) derivation(execution *ruleExecution, reads []demand.Observation, dispositions []RuleDisposition[Support]) (RuleDerivation[Support, ruleUnit], *ruleAdmissionTicket, bool) {
	if compiled == nil || compiled.rule == nil || !compiled.admission.same(compiled.rule.admission) || execution == nil || execution.owner != compiled || execution.product == nil || !execution.product.requireCheckpoint() || execution.epoch == 0 || execution.active.Load() != execution.epoch || !compiled.anchor.Available() || compiled.rule.composition == nil || !compiled.rule.composition.Sealed() {
		return RuleDerivation[Support, ruleUnit]{}, nil, false
	}
	ticket := &ruleAdmissionTicket{rule: compiled.rule, composition: compiled.rule.composition.ID(), identity: compiled.admission.identity, epoch: execution.epoch, anchor: compiled.anchor, execution: execution, product: execution.product, live: true}
	if !compiled.requiresDerivation() {
		return RuleDerivation[Support, ruleUnit]{ticket: ticket}, ticket, true
	}
	inputs := make([]RuleInput, len(execution.inputs))
	for index, input := range execution.inputs {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[Support, ruleUnit]{}, nil, false
		}
		if !input.Valid() && !input.Same(execution.base.State()) {
			return RuleDerivation[Support, ruleUnit]{}, nil, false
		}
		inputs[index] = RuleInput{state: input}
	}
	proofReads := make([]RuleRead, len(reads))
	for index, read := range reads {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[Support, ruleUnit]{}, nil, false
		}
		proofReads[index] = RuleRead{input: read.Input, unit: read.Unit}
	}
	if !validRuleDispositionCoverage(dispositions, len(execution.product.values)) {
		return RuleDerivation[Support, ruleUnit]{}, nil, false
	}
	for index := range dispositions {
		if !execution.product.requireCheckpoint() {
			return RuleDerivation[Support, ruleUnit]{}, nil, false
		}
		if dispositions[index].row.index != index || dispositions[index].row.index < 0 || dispositions[index].row.index >= len(execution.product.values) {
			return RuleDerivation[Support, ruleUnit]{}, nil, false
		}
		dispositions[index].row.ticket = ticket
		dispositions[index].ordinal = index
	}
	if !execution.product.requireCheckpoint() {
		return RuleDerivation[Support, ruleUnit]{}, nil, false
	}
	return RuleDerivation[Support, ruleUnit]{rule: compiled.rule, composition: compiled.rule.composition.ID(), identity: compiled.admission.identity, epoch: execution.epoch, anchor: compiled.anchor, inputs: inputs, reads: proofReads, dispositions: dispositions, product: execution.product, ticket: ticket}, ticket, true
}

// compileSupportRule is the sole private handoff from a sealed cold support
// declaration to its synchronous runtime wrapper. It neither chooses a
// Program coordinate nor attaches a carrier/action; Wave E owns those later
// bindings. Keeping this cut narrow prevents an old Solver declaration path
// from becoming a second support authority.
func compileSupportRule(rule *SupportRule) (*compiledSupportRule, bool) {
	if rule == nil || !rule.available() || !rule.composition.Sealed() || rule.schema == nil || !rule.admission.same(rule.schema.admission) || !validColdSupportRule(rule.composition, rule.schema) || rule.run == nil {
		return nil, false
	}
	compiled := &compiledSupportRule{rule: rule.schema, admission: rule.admission}
	compiled.run = retainSupportRun(rule.schema, rule.run)
	return compiled, true
}
