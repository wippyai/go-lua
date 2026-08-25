package apply

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Executor is a solve-local, signature-bound semantic worker.
//
// Prepare performs the one mount lookup and creates one typed worker. Invoke
// then performs one semantic application over one already assembled frame.
// The worker is deliberately hidden: callers cannot bypass the declared
// signature, replace the mounted output witness, or publish directly from a
// semantic operation. An Executor is reusable by one serial solve lane; it is
// not safe for concurrent Invoke calls because the generated worker owns
// solve-local scratch.
type Executor struct {
	data *executorData
}

type executorData struct {
	operation signature.Signature
	worker    binding.Worker
	fence     binding.Fence
	witness   binding.DenominatorWitness
	scope     binding.ScopeToken
}

// Prepare binds one mounted operation to one authenticated invocation scope.
// All mount and output-authority checks happen here, before any frame or
// semantic worker can run. The returned executor owns no store or publication
// capability.
func Prepare(mounted witness.Mounted, operation signature.Identity, scope witness.Scope) (Executor, bool) {
	if !mounted.Available() || !operation.Available() || !scope.Available() {
		return Executor{}, false
	}
	bound, ok := mounted.Binding(operation)
	if !ok || bound == nil {
		return Executor{}, false
	}
	sealed := bound.Signature()
	if !sealed.Available() || sealed.Identity() != operation || sealed.Fence().Schema != mounted.RuntimeFence().Schema() {
		return Executor{}, false
	}
	fence := mounted.RuntimeFence()
	scopeToken, ok := mounted.ScopeToken(scope)
	if !ok || !scopeToken.ValidFor(fence) {
		return Executor{}, false
	}
	authority := sealed.Authority()
	if !authority.Available() {
		return Executor{}, false
	}
	denominator, ok := mounted.Denominator(authority.Denominator)
	if !ok || !denominator.ValidFor(fence) || !denominator.Matches(authority.Denominator) {
		return Executor{}, false
	}
	worker, ok := bound.NewWorker(fence)
	if !ok || worker == nil {
		return Executor{}, false
	}
	return Executor{data: &executorData{
		operation: sealed,
		worker:    worker,
		fence:     fence,
		witness:   denominator,
		scope:     scopeToken,
	}}, true
}

// Apply is the one-shot convenience entry point. It prepares one executor,
// assembles the declared frame, invokes the typed worker exactly once, and
// returns staged proposals without touching state or publication.
func Apply(mounted witness.Mounted, operation signature.Identity, scope witness.Scope, slots ...binding.Slot) (Application, bool) {
	executor, ok := Prepare(mounted, operation, scope)
	if !ok {
		return Application{}, false
	}
	frame, ok := binding.NewFrame(executor.data.scope, slots...)
	if !ok {
		return Application{}, false
	}
	return executor.Invoke(frame)
}

// Available reports whether the executor still carries a complete sealed
// operation, mounted denominator, and runtime-fenced scope.
func (executor Executor) Available() bool {
	return executor.data != nil && executor.data.operation.Available() && executor.data.worker != nil && executor.data.fence.Available() && executor.data.witness.ValidFor(executor.data.fence) && executor.data.witness.Matches(executor.data.operation.Authority().Denominator) && executor.data.scope.ValidFor(executor.data.fence)
}

// Signature returns the immutable operation contract redeemed by this
// executor.
func (executor Executor) Signature() signature.Signature {
	if !executor.Available() {
		return signature.Signature{}
	}
	return executor.data.operation
}

// Invoke validates the frame against the exact sealed input contract, invokes
// the typed worker once, and atomically seals its bounded proposals. A
// semantic Refused outcome is a valid application result but has no proposal
// batch. Invalid frames, malformed worker outcomes, and failed sealing return
// false and cannot yield cells.
func (executor Executor) Invoke(frame binding.Frame) (Application, bool) {
	if !executor.Available() || !frame.Validate(executor.data.operation, executor.data.fence) {
		return Application{}, false
	}
	buffer, ok := binding.NewProposalBuffer(executor.data.operation, executor.data.fence, executor.data.witness, executor.data.scope)
	if !ok {
		return Application{}, false
	}
	result := executor.data.worker.Evaluate(frame, &buffer)
	if !result.Available() || !executor.data.operation.Allows(result.Code) {
		buffer.Abandon()
		return Application{}, false
	}
	if result.Code == outcome.Refused {
		// A refusal is an outcome, never a cell. Abandoning also handles a
		// malformed worker that staged a proposal before refusing.
		buffer.Abandon()
		return newRefusedApplication(result), true
	}
	batch, ok := buffer.Seal(result)
	if !ok || !batch.Available() || batch.Outcome() != result || (result.Code != outcome.Produced && batch.Len() != 0) {
		buffer.Abandon()
		return Application{}, false
	}
	return newApplication(result, batch), true
}

// Frame assembles an authenticated frame under the executor's fixed
// invocation scope. It is a convenience for physical operators; the frame is
// still checked again by Invoke before the worker runs.
func (executor Executor) Frame(slots ...binding.Slot) (binding.Frame, bool) {
	if !executor.Available() {
		return binding.Frame{}, false
	}
	return binding.NewFrame(executor.data.scope, slots...)
}

// Application is the semantic result of one invocation. Outcome and cells
// are separate by construction: NoCandidate/NoSelection/Opaque carry an
// empty sealed batch, while Refused carries only its refusal identity. No
// method here can mutate a store or submit a transaction.
type Application struct {
	result   outcome.Result
	batch    binding.ProposalBatch
	hasBatch bool
}

func newApplication(result outcome.Result, batch binding.ProposalBatch) Application {
	return Application{result: result, batch: batch, hasBatch: true}
}

func newRefusedApplication(result outcome.Result) Application {
	return Application{result: result}
}

// Available reports whether the outcome is closed and its cell sidecar, if
// present, is a live proposal lease with the same outcome.
func (application Application) Available() bool {
	if !application.result.Available() {
		return false
	}
	if application.result.Code == outcome.Refused {
		return !application.hasBatch
	}
	return application.hasBatch && application.batch.Available() && application.batch.Outcome() == application.result
}

// Outcome returns the closed terminal disposition. It remains available for a
// valid Refused result even though that result intentionally has no batch.
func (application Application) Outcome() outcome.Result {
	if !application.Available() {
		return outcome.Result{}
	}
	return application.result
}

// Proposals returns the sealed proposal sidecar for a non-refusal outcome.
// Refusal is intentionally represented by (zero, false), never by a fake
// empty cell or a fabricated bottom value.
func (application Application) Proposals() (binding.ProposalBatch, bool) {
	if !application.Available() || !application.hasBatch {
		return binding.ProposalBatch{}, false
	}
	return application.batch, true
}

// Len reports the number of staged output cells. Refusal has no cells.
func (application Application) Len() int {
	batch, ok := application.Proposals()
	if !ok {
		return 0
	}
	return batch.Len()
}

// Refusal returns the stable refusal identity only for a valid Refused
// outcome. No other semantic disposition can leak a refusal reason.
func (application Application) Refusal() (model.RefusalID, bool) {
	result := application.Outcome()
	if !result.Available() || result.Code != outcome.Refused {
		return model.RefusalID{}, false
	}
	return result.RefusalID, true
}
