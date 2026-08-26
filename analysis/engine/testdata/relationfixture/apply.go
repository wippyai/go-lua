package testfixture

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// TwoScalarApplyNode redeems the canonical two-child scalar Apply specimen.
// It exists so Apply/evaluator laws exercise a real sealed physical binding,
// rather than manufacturing a second mount in an operator package.
func (fixture Fixture) TwoScalarApplyNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.twoScalarApplyExpression)
}

// TwoScalarApplyFrames returns the serial worker's frames in invocation
// order. It is test-only observation of the already-mounted semantic worker;
// it grants no publication or state capability.
func (fixture Fixture) TwoScalarApplyFrames() []binding.Frame {
	if fixture.twoScalarApplyWorker == nil {
		return nil
	}
	return fixture.twoScalarApplyWorker.Frames()
}

// ScalarCompleteApplyNode redeems the canonical mixed-delivery specimen:
// one scalar input and one sealed Complete(left) range. It proves that span
// input is an indivisible range alternative, never a flattened tuple stream.
func (fixture Fixture) ScalarCompleteApplyNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.scalarSpanApplyExpression)
}

// ScalarCompleteApplyFrames exposes the inert specimen worker's immutable
// invocation history for physical Apply laws only.
func (fixture Fixture) ScalarCompleteApplyFrames() []binding.Frame {
	if fixture.scalarSpanApplyWorker == nil {
		return nil
	}
	return fixture.scalarSpanApplyWorker.Frames()
}

// EmptyInputNode redeems the zero-row relation's ordinary Input producer.
// Its range proof is still canonical; only its mounted denominator happens
// to contain no rows.
func (fixture Fixture) EmptyInputNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.emptyExpression)
}

// EmptyCompleteBinding redeems the Complete boundary over the genuine empty
// denominator. It is distinct from an omitted range over a nonempty owner.
func (fixture Fixture) EmptyCompleteBinding() (arrangement.CompleteBinding, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.CompleteBinding{}, false
	}
	node, ok := execution.Entry(fixture.emptyCompleteExpression)
	if !ok {
		return arrangement.CompleteBinding{}, false
	}
	return node.Complete()
}

// ScalarEmptyCompleteApplyNode redeems the scalar plus genuinely-empty
// CompleteSpan operation used to prove denominator provenance for an empty
// closed world.
func (fixture Fixture) ScalarEmptyCompleteApplyNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.scalarEmptyApplyExpression)
}

// ScalarEmptyCompleteApplyFrames exposes only immutable worker observations
// for the canonical zero-denominator law.
func (fixture Fixture) ScalarEmptyCompleteApplyFrames() []binding.Frame {
	if fixture.scalarEmptyApplyWorker == nil {
		return nil
	}
	return fixture.scalarEmptyApplyWorker.Frames()
}

// CorrelatedApplyNode redeems the mounted heterogeneous Apply whose population
// is the left value coordinate and whose two children are complete range
// authorities. Its replay directory is therefore a real certificate/mount
// posting, not a test-constructed child tuple.
func (fixture Fixture) CorrelatedApplyNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.correlatedApplyExpression)
}

// CorrelatedApplyFrames exposes the mounted worker's invocation order for
// replay laws. It is an immutable copy of the worker's existing serial trace;
// replay itself never relies on this observation or retains it.
func (fixture Fixture) CorrelatedApplyFrames() []binding.Frame {
	if fixture.correlatedApplyWorker == nil {
		return nil
	}
	return fixture.correlatedApplyWorker.Frames()
}

// MixedPopulationApplyNode redeems the two-child mixed correlated Apply:
// child zero is the population Input scalar, while child one is one complete
// selected span carrying three distinct authored delivery columns.
func (fixture Fixture) MixedPopulationApplyNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.mixedPopulationApplyExpression)
}

// MixedPopulationApplyFrames exposes the inert mixed replay worker's exact
// invocation frames for runtime laws.
func (fixture Fixture) MixedPopulationApplyFrames() []binding.Frame {
	if fixture.mixedPopulationApplyWorker == nil {
		return nil
	}
	return fixture.mixedPopulationApplyWorker.Frames()
}

// SharedCompleteApplyNode redeems the generic mixed correlation with one Q
// scalar child and one globally shared Complete(right) child. The latter has
// an empty projection, so its rows are not mirrored into a per-Q relation.
func (fixture Fixture) SharedCompleteApplyNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.sharedCompleteApplyExpression)
}

// SharedCompleteApplyFrames exposes the inert worker trace for the generic
// broadcast law only; replay itself retains no such trace or cache.
func (fixture Fixture) SharedCompleteApplyFrames() []binding.Frame {
	if fixture.sharedCompleteApplyWorker == nil {
		return nil
	}
	return fixture.sharedCompleteApplyWorker.Frames()
}

// SharedEmptyApplyNode is the same broadcast shape over an exact empty global
// Complete denominator. It proves empty is a closed shared span, not a
// missing q-specific posting.
func (fixture Fixture) SharedEmptyApplyNode() (arrangement.Node, bool) {
	execution := fixture.mounted.Arrangement().Execution()
	if !execution.Available() {
		return arrangement.Node{}, false
	}
	return execution.Entry(fixture.sharedEmptyApplyExpression)
}

func (fixture Fixture) SharedEmptyApplyFrames() []binding.Frame {
	if fixture.sharedEmptyApplyWorker == nil {
		return nil
	}
	return fixture.sharedEmptyApplyWorker.Frames()
}

// applyWorker is a deliberately inert semantic worker for the Apply physical
// specimen. It records exact frames and returns NoSelection, so no fixture
// state changes are needed merely to prove cartesian delivery.
type applyWorker struct {
	mu     sync.Mutex
	frames []binding.Frame
}

func (worker *applyWorker) Evaluate(frame binding.Frame, _ *binding.ProposalBuffer) outcome.Result {
	if worker == nil || !frame.Available() {
		return outcome.Result{}
	}
	worker.mu.Lock()
	worker.frames = append(worker.frames, frame)
	worker.mu.Unlock()
	return outcome.Result{Code: outcome.NoSelection}
}

func (worker *applyWorker) Frames() []binding.Frame {
	if worker == nil {
		return nil
	}
	worker.mu.Lock()
	defer worker.mu.Unlock()
	return append([]binding.Frame(nil), worker.frames...)
}
