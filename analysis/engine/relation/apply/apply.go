package apply

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
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
	// destinations contains the exact owner-issued witness for every sealed
	// output denominator. It is immutable solve-local capability, never a
	// route inferred from the parent row.
	destinations []binding.DenominatorWitness
	scope        binding.ScopeToken
	lineage      lineage.Authority
}

// Prepare binds one mounted operation to one authenticated invocation scope.
// All mount and output-destination checks happen here, before any frame or
// semantic worker can run. The returned executor owns no store or publication
// capability.
func Prepare(mounted witness.Mounted, operation signature.Identity, scope witness.Scope) (Executor, bool) {
	core, ok := prepareCore(mounted, operation)
	if !ok {
		return Executor{}, false
	}
	return core.atScope(mounted, scope)
}

// prepareCore performs the mount lookup and creates the one solve-local
// worker for an Apply evaluation. It intentionally has no invocation scope:
// a multi-input Apply may yield several compatible cofibers, while its worker
// remains the same serial worker across those frames. Only atScope can turn
// this core into an invocable Executor.
func prepareCore(mounted witness.Mounted, operation signature.Identity) (Executor, bool) {
	if !mounted.Available() || !operation.Available() {
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
	destinations := make([]binding.DenominatorWitness, 0, sealed.OutputLen())
	seenDestinations := make(map[model.DenominatorRef]struct{}, sealed.OutputLen())
	for _, output := range sealed.Outputs() {
		ref := output.Denominator
		if !ref.Available() {
			return Executor{}, false
		}
		if _, seen := seenDestinations[ref]; seen {
			continue
		}
		outputWitness, witnessOK := mounted.Denominator(ref)
		if !witnessOK || !outputWitness.ValidFor(fence) || !outputWitness.Matches(ref) {
			return Executor{}, false
		}
		destinations = append(destinations, outputWitness)
		seenDestinations[ref] = struct{}{}
	}
	worker, ok := bound.NewWorker(fence)
	if !ok || worker == nil {
		return Executor{}, false
	}
	lineageAuthority, ok := mounted.Lineage()
	if !ok || lineageAuthority == nil || !lineageAuthority.Fence().Same(fence) || !lineageAuthority.Owner().Available() || !lineageAuthority.Identity().Available() {
		return Executor{}, false
	}
	return Executor{data: &executorData{
		operation:    sealed,
		worker:       worker,
		fence:        fence,
		destinations: destinations,
		lineage:      lineageAuthority,
	}}, true
}

// atScope rebinds one already-created serial worker to an exact mounted
// cofiber without asking the binding to manufacture another worker. The
// operation and output witnesses remain immutable; only the frame-local
// scope changes. It is private because callers must reach it through the sealed
// physical Apply operator, never construct a scope adapter themselves.
func (executor Executor) atScope(mounted witness.Mounted, scope witness.Scope) (Executor, bool) {
	if !executor.coreAvailable() || !mounted.Available() || !scope.ValidFor(mounted.RuntimeFence()) || !executor.data.fence.Same(mounted.RuntimeFence()) {
		return Executor{}, false
	}
	scopeToken, ok := mounted.ScopeToken(scope)
	if !ok || !scopeToken.ValidFor(executor.data.fence) {
		return Executor{}, false
	}
	data := *executor.data
	data.scope = scopeToken
	result := Executor{data: &data}
	return result, result.Available()
}

// Apply is the one-shot convenience entry point. It prepares one executor,
// assembles the declared frame, redeems the supplied mounted provenance,
// invokes the typed worker exactly once, and returns staged proposals without
// touching state or publication.
func Apply(mounted witness.Mounted, operation signature.Identity, scope witness.Scope, provenance model.LineageRef, destination binding.DestinationView, slots ...binding.Slot) (Application, bool) {
	executor, ok := Prepare(mounted, operation, scope)
	if !ok {
		return Application{}, false
	}
	frame, ok := binding.NewFrame(executor.data.scope, slots...)
	if !ok {
		return Application{}, false
	}
	return executor.Invoke(frame, provenance, destination)
}

// Available reports whether the executor still carries a complete sealed
// operation, mounted denominator, and runtime-fenced invocation scope.
func (executor Executor) Available() bool {
	return executor.coreAvailable() && executor.data.scope.ValidFor(executor.data.fence)
}

func (executor Executor) coreAvailable() bool {
	if executor.data == nil || !executor.data.operation.Available() || executor.data.worker == nil || !executor.data.fence.Available() || executor.data.lineage == nil || !executor.data.lineage.Fence().Same(executor.data.fence) || !executor.data.lineage.Owner().Available() || !executor.data.lineage.Identity().Available() {
		return false
	}
	for _, output := range executor.data.operation.Outputs() {
		if !output.Denominator.Available() {
			return false
		}
		found := false
		for _, witness := range executor.data.destinations {
			if witness.Matches(output.Denominator) && witness.ValidFor(executor.data.fence) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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
// batch. Invalid frames, unadmitted provenance, malformed worker outcomes, and
// failed sealing return false and cannot yield cells.
func (executor Executor) Invoke(frame binding.Frame, provenance model.LineageRef, destination binding.DestinationView) (Application, bool) {
	if !frame.Available() {
		return Application{}, false
	}
	children := make([]invocation.SourceVector, frame.Len())
	for childIndex := 0; childIndex < frame.Len(); childIndex++ {
		slot, slotOK := frame.At(childIndex)
		if !slotOK || !slot.Available() {
			return Application{}, false
		}
		rows := make([]model.RowID, slot.Len())
		for rowIndex := 0; rowIndex < slot.Len(); rowIndex++ {
			cell, cellOK := slot.At(rowIndex)
			if !cellOK || !cell.Available() {
				return Application{}, false
			}
			rows[rowIndex] = cell.Address().Row()
		}
		tuple, tupleOK := invocation.NewTupleSources(rows)
		if !tupleOK {
			return Application{}, false
		}
		children[childIndex], tupleOK = invocation.NewSourceVector([]invocation.TupleSources{tuple})
		if !tupleOK {
			return Application{}, false
		}
	}
	address, ok := invocation.New(frame.Scope(), children)
	if !ok {
		return Application{}, false
	}
	return executor.invoke(frame, provenance, address, destination)
}

// invoke is the sealed physical Apply entry. The richer child/source address
// is supplied by Execute after it has redeemed the selected tuple vectors;
// the public Executor ABI keeps its existing frame-only form for generated
// workers and laws.
func (executor Executor) invoke(frame binding.Frame, provenance model.LineageRef, address invocation.InvocationAddress, destination binding.DestinationView) (Application, bool) {
	if !executor.Available() || !executor.data.lineage.Validate(provenance) || !frame.Validate(executor.data.operation, executor.data.fence) || !frame.Scope().Same(executor.data.scope) || !address.ValidFor(executor.data.fence) || !address.Scope().Same(frame.Scope()) || !destination.Available() {
		return Application{}, false
	}
	buffer, ok := binding.NewProposalBuffer(executor.data.operation, executor.data.fence, executor.data.destinations, executor.data.scope, destination)
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
		return newRefusedApplication(result, provenance, address, executor.data.operation.Identity(), executor.data.fence), true
	}
	batch, ok := buffer.Seal(result)
	if !ok || !batch.Available() || batch.Outcome() != result || (!result.Code.Publishes() && batch.Len() != 0) {
		buffer.Abandon()
		return Application{}, false
	}
	return newApplication(result, batch, provenance, address, executor.data.operation.Identity(), executor.data.fence), true
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

// Application is the semantic result of one invocation. Runtime fence,
// outcome, exact input provenance, and cells are separate by construction:
// NoCandidate/NoSelection/Refused carry no proposal rows, while Opaque may
// carry authenticated rows alongside Produced. No method here can mutate a
// store or submit a transaction.
type Application struct {
	result     outcome.Result
	batch      binding.ProposalBatch
	hasBatch   bool
	lineage    model.LineageRef
	invocation invocation.InvocationAddress
	operation  signature.Identity
	fence      binding.Fence
}

func newApplication(result outcome.Result, batch binding.ProposalBatch, provenance model.LineageRef, invocation invocation.InvocationAddress, operation signature.Identity, fence binding.Fence) Application {
	return Application{result: result, batch: batch, hasBatch: true, lineage: provenance, invocation: invocation, operation: operation, fence: fence}
}

func newRefusedApplication(result outcome.Result, provenance model.LineageRef, invocation invocation.InvocationAddress, operation signature.Identity, fence binding.Fence) Application {
	return Application{result: result, lineage: provenance, invocation: invocation, operation: operation, fence: fence}
}

// Available reports whether the outcome is closed and its cell sidecar, if
// present, is a live proposal lease with the same outcome.
func (application Application) Available() bool {
	if !application.result.Available() || !application.lineage.Available() || !application.operation.Available() || !application.fence.Available() || !application.invocation.ValidFor(application.fence) {
		return false
	}
	if application.result.Code == outcome.Refused {
		return !application.hasBatch
	}
	return application.hasBatch && application.batch.Available() && application.batch.Operation() == application.operation && application.batch.Outcome() == application.result
}

// Invocation returns the immutable positional source/scope address captured
// at Apply execution. It is provenance only; it is not an application ordinal
// and cannot be used to address a store row without a schema descriptor.
func (application Application) Invocation() invocation.InvocationAddress {
	if !application.Available() {
		return invocation.InvocationAddress{}
	}
	return application.invocation
}

// Fence returns the exact solve-local runtime that authenticated the
// application. Publish uses it to reject even a valid no-write application
// from a sibling mount, whose outcome has no proposal token carrying a fence.
func (application Application) Fence() binding.Fence {
	if !application.Available() {
		return binding.Fence{}
	}
	return application.fence
}

// Operation returns the exact schema-sealed operation that produced this
// application. Snapshot descriptors compare it before projecting any row;
// a sibling operation on the same mount cannot be reinterpreted by shape.
func (application Application) Operation() signature.Identity {
	if !application.Available() {
		return signature.Identity{}
	}
	return application.operation
}

// Lineage returns the exact authenticated provenance supplied with the frame
// that produced this application. It is the only provenance authority Publish
// may redeem; callers cannot pair this result with an unrelated sidecar.
func (application Application) Lineage() model.LineageRef {
	if !application.Available() {
		return model.LineageRef{}
	}
	return application.lineage
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
