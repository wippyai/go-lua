// runtime_rule.go runs the canonical bound Rule member and its Fold.

package engine

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/lifetime"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ruleOperand is the private typed bridge used only while attaching a
// SchemaBinding selected read. It exposes the canonical bound operand to the
// locator without putting an operand into SelectorContext or retaining a
// mutable snapshot.
func (bound *boundRuleMember[V, O]) ruleOperand() O {
	if bound == nil {
		var zero O
		return zero
	}
	return bound.operand
}

func (bound *boundRuleMember[V, O]) transformedCarry() (identity.SemanticKey, []carrier.Target, func(V) (V, bool), bool) {
	if bound == nil || !bound.carrySemantic.Available() || bound.carryApply == nil {
		return identity.SemanticKey{}, nil, nil, false
	}
	return bound.carrySemantic, bound.transformedTargets, bound.carryApply, true
}

func (bound *boundRuleMember[V, O]) transformedCarryRoute() bool {
	return bound != nil && bound.routeTransform
}

func (bound *boundRuleMember[V, O]) initialRuleReads() []demand.Observation {
	if bound == nil {
		return nil
	}
	result := make([]demand.Observation, 0, len(bound.reads))
	for _, read := range bound.reads {
		result = append(result, read.observations()...)
	}
	return result
}

func (bound *boundRuleMember[V, O]) dynamicRuleReads() []demand.DynamicRead {
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
	epoch   identity.Generation
	active  lifetime.Cell
	failed  atomic.Bool
	product *productSession
	output  outputSession
}

// anyRule contains only the canonical sealed Rule address needed by ordinary
// runtime Read fences. Construction proofs are never retained by a runtime
// member or execution.
type anyRule interface {
	runtimeRuleCell() schemaRuleBindingCell
	runtimeRuleOrdinal() uint64
}

func (bound *boundRuleMember[V, O]) runtimeRuleCell() schemaRuleBindingCell {
	if bound == nil {
		return nil
	}
	return bound.cell
}

func (bound *boundRuleMember[V, O]) runtimeRuleOrdinal() uint64 {
	if bound == nil {
		return 0
	}
	return bound.ordinal
}

// readBinding is the one private E-side sink for a cold Rule's ordered typed
// read projection. Factor and structural support Rules use this exact path;
// only Factor Rules additionally install an output Patch session.
type readBinding interface {
	appendReadRuntime(readRuntime) bool
}

// bindRuntimeRuleReads consumes a Rule's sealed positional read binders in
// Graph member order. The compiler supplies only the factor catalog; every
// typed normalization, equality, and selector callback remains schema-owned.
func (bound *boundRuleMember[V, O]) appendReadRuntime(read readRuntime) bool {
	if bound == nil || read == nil || bound.cell == nil || bound.ordinal != bound.cell.schemaRuleOrdinal() || uint64(len(bound.reads)) >= bound.expectedReadCount {
		return false
	}
	bound.reads = append(bound.reads, read)
	return true
}

func (bound *boundRuleMember[V, O]) executeRule(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) (carrier.Patch, []demand.Observation, bool, bool, solveBoundary) {
	if bound == nil || bound.fold == nil || work == nil || !work.OwnsRuleContributionStates(base, inputs) {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	epoch, issued := bound.nextEpoch.Issue()
	if !issued {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	execution := &ruleExecution{owner: bound, work: work, base: base, epoch: epoch}
	execution.active.Open(epoch)
	defer func() {
		if execution.output != nil {
			execution.output.discard()
		}
		if execution.product != nil {
			execution.product.close()
		}
		execution.active.Revoke(epoch)
	}()
	product, ok := newProductSession(execution, bound.reads, work, inputs, within)
	if !ok {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	execution.product = product
	execution.output = bound.output.begin(execution)
	if execution.output == nil {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	if !product.started.CompareAndSwap(false, true) {
		return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "preflight")
	}
	for row := 0; row < product.rows.Count(); row++ {
		product.current = row
		frame := Frame[V, O]{execution: execution, owner: bound, epoch: epoch, row: row}
		result := bound.fold(frame)
		settled := bound.settleRuleResult(execution, epoch, row, result)
		if execution.failed.Load() || !product.requireCheckpoint() || !settled || !product.requireCheckpoint() {
			product.current = -1
			return carrier.Patch{}, nil, false, false, refused(SolveFailureFamilyExecution, "fold")
		}
		product.current = -1
	}
	reads := product.observations()
	if !product.requireCheckpoint() {
		return carrier.Patch{}, nil, false, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	if !product.started.Load() || !execution.output.complete() {
		return carrier.Patch{}, nil, false, false, stalled(SolveFailureFamilyExecution, "checkpoint")
	}
	// An empty support intersection has no Product row and therefore invokes no
	// Fold. It is a successful structural no-op retaining resolved reads only.
	if product.rows.Count() == 0 {
		return carrier.Patch{}, reads, false, true, boundaryNone
	}
	if !execution.output.hasStaged() {
		// An explicit all-omitted Product is a valid empty successor, not a
		// sparse Default write, and publishes no Factor patch.
		execution.output.discard()
		execution.output = nil
		return carrier.Patch{}, reads, false, true, boundaryNone
	}
	patch, ok := execution.output.publish()
	execution.output = nil
	if !ok {
		return patch, reads, true, false, refused(SolveFailureFamilyExecution, "publication")
	}
	return patch, reads, true, true, boundaryNone
}

func (bound *boundRuleMember[V, O]) settleRuleResult(execution *ruleExecution, epoch identity.Generation, row int, result RuleResult[V]) bool {
	if bound == nil || execution == nil || execution.owner != bound || execution.product == nil || execution.product.current != row || !execution.active.Holds(epoch) || result.execution != execution || result.epoch != epoch || result.row != row {
		return false
	}
	switch result.kind {
	case ruleResultNoCandidate:
		return bound.output.noCandidate != nil && bound.output.noCandidate(execution, epoch, row)
	case ruleResultStaged:
		return bound.output.stage != nil && bound.output.stage(execution, epoch, row, result.value)
	case ruleResultRouted:
		if bound.output.routeRelease != nil {
			defer func() {
				_ = bound.output.routeRelease(execution, epoch, row, result.route.read, result.route.selectionID, result.route.lease)
			}()
		}
		if len(result.route.refs) == 0 {
			return bound.output.noSelection != nil && bound.output.noSelection(execution, epoch, row, result.route)
		}
		return bound.output.stageSelection != nil && bound.output.stageSelection(execution, epoch, row, result.route)
	default:
		return false
	}
}
