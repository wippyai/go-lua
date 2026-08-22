// runtime_executor.go drives the region fixpoint and hosts the public Solve entry points.

package engine

import (
	"context"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
)

type SolveStatus uint8

const (
	SolveIncomplete SolveStatus = iota + 1
	SolveComplete
	SolveCanceled
	SolvePanicked
	// SolveInvalid is returned only by a diagnostic entry point before any
	// solver work when its closed options value is invalid.
	SolveInvalid
)

// advanceNarrow moves only currently-ascent recurrence episodes into their
// narrow phase.  The root-to-leaf traversal preserves the ownership invariant
// parent-Ascent => descendant-Ascent at every observable solver boundary.
// A child may be re-ascending beneath a narrowed parent after a localized
// restart; that parent remains narrow until the changed child result reaches
// its own exact-RHS check and causes its own reset.
func (epoch *executorEpoch) advanceNarrow() (advanced, ok bool) {
	if epoch == nil || epoch.runtime == nil || len(epoch.runtime.regions) != len(epoch.regions) || len(epoch.runtime.regionChildren) != len(epoch.runtime.regions) {
		return false, false
	}
	stack := epoch.regionScratch[:0]
	for index, region := range epoch.runtime.regions {
		if !epoch.activeRegion(index) {
			continue
		}
		if region.parent == schedule.NoRegion {
			stack = append(stack, index)
			continue
		}
		if region.parent < 0 || region.parent >= len(epoch.runtime.regions) || !epoch.activeRegion(region.parent) {
			return false, false
		}
	}
	for next := 0; next < len(stack); next++ {
		index := stack[next]
		if !epoch.activeRegion(index) {
			return false, false
		}
		region, episode := &epoch.runtime.regions[index], &epoch.regions[index]
		switch episode.phase {
		case phaseAscent:
			if !episode.hasExact {
				return false, false
			}
			episode.phase = phaseNarrow
			episode.postfixAt = 0
			// A narrow episode descends. Its retained ascent accumulator is no
			// longer an under-approximation of the recurrence row, so it is
			// dropped at the phase cut rather than guarded at every reader.
			if episode.hasAccumulator {
				dbgEngine.DropNarrowPhase++
			}
			episode.accumulator, episode.hasAccumulator = carrier.PointRHS{}, false
			if !epoch.markPostfixDirty(region.head) || !epoch.enqueuePoint(region.head) {
				return false, false
			}
			advanced = true
		case phaseNarrow:
			// The parent was already narrowed before this local episode. Its
			// child may still need a new narrow pass, but no parent history is
			// rewritten here.
		default:
			return false, false
		}
		for _, child := range epoch.runtime.regionChildren[index] {
			if child < 0 || child >= len(epoch.runtime.regions) || !epoch.activeRegion(child) || epoch.runtime.regions[child].parent != index {
				return false, false
			}
			stack = append(stack, child)
		}
	}
	epoch.regionScratch = stack
	return advanced, true
}

func (epoch *executorEpoch) allRegionsNarrow() bool {
	if epoch == nil || epoch.runtime == nil || len(epoch.runtime.regions) != len(epoch.regions) {
		return false
	}
	for index := range epoch.runtime.regions {
		if !epoch.activeRegion(index) {
			continue
		}
		if epoch.regions[index].phase != phaseNarrow {
			return false
		}
	}
	return true
}

func (epoch *executorEpoch) visitPoints() (visited bool, ok bool) {
	if epoch == nil || epoch.canceled() || epoch.runtime == nil || epoch.runtime.points == nil {
		return false, false
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordPass(len(epoch.frames))
	}
	frames := epoch.frames[:0]
	defer func() { epoch.frames = frames[:0] }()
	for index := 0; index < epoch.runtime.executionEventCount(); index++ {
		if epoch.canceled() {
			return false, false
		}
		event, eventOK := epoch.runtime.executionEventAt(index)
		if !eventOK {
			return false, false
		}
		switch event.Kind {
		case schedule.EventEnter:
			region, regionOK := epoch.runtime.executionRegionAt(event.Region)
			parent := schedule.NoRegion
			if len(frames) != 0 {
				parent = frames[len(frames)-1].region
			}
			if !regionOK || !epoch.activeRegion(event.Region) || region.Head != event.Node || region.Parent != parent {
				return false, false
			}
			if !epoch.snapshotRegion(event.Region) {
				return false, false
			}
			frames = append(frames, pointWTOFrame{region: event.Region})
		case schedule.EventExit:
			if len(frames) == 0 || frames[len(frames)-1].region != event.Region {
				return false, false
			}
			region, regionOK := epoch.runtime.executionRegionAt(event.Region)
			if !regionOK || !epoch.activeRegion(event.Region) || region.Head != event.Node {
				return false, false
			}
			settled, valid := epoch.regionPostfixed(event.Region)
			if !valid {
				return false, false
			}
			if !settled {
				// Keep the child logically active. Its queued head contributes to
				// the enclosing nested count, so the parent cannot run ahead.
				return visited, true
			}
			changed := epoch.regionSnapshotChanged(event.Region)
			frames = frames[:len(frames)-1]
			if changed && len(frames) != 0 {
				parent := frames[len(frames)-1].region
				if !epoch.activeRegion(parent) || !epoch.enqueuePoint(epoch.runtime.regions[parent].head) {
					return false, false
				}
			}
		case schedule.EventNode:
			if (len(frames) == 0 && event.Region != schedule.NoRegion) || (len(frames) != 0 && event.Region != frames[len(frames)-1].region) {
				return false, false
			}
			stateIndex := int(event.Node)
			point, _, _, pointOK := epoch.runtime.graphPointAtState(stateIndex)
			if !pointOK {
				return false, false
			}
			// The retained plan schedule is static over all compact states. Demand
			// activation is epoch-local, so an event for a non-admitted state is a
			// valid schedule row with no work in this epoch.
			if !epoch.activeState(stateIndex) {
				continue
			}
			if event.Region != schedule.NoRegion {
				if !epoch.activeRegion(event.Region) {
					return false, false
				}
				if epoch.runtime.regions[event.Region].head == stateIndex && epoch.nested[event.Region] != 0 {
					// The child frame owns progress first. Leaving this Point queued
					// makes the next iterative WTO pass revisit the parent head only
					// after all nested readiness has drained.
					continue
				}
			}
			if !epoch.takePoint(stateIndex) {
				continue
			}
			visited = true
			headRegion := schedule.NoRegion
			if event.Region != schedule.NoRegion {
				if !epoch.activeRegion(event.Region) {
					return false, false
				}
				candidate := &epoch.runtime.regions[event.Region]
				if candidate.head == stateIndex {
					headRegion = event.Region
				}
			}
			if _, pointOK := epoch.refreshPoint(point, stateIndex, headRegion); !pointOK {
				epoch.recordPointFailure(SolveFailureReasonExecution, point)
				return false, false
			}
		default:
			return false, false
		}
	}
	return visited, len(frames) == 0
}

func (epoch *executorEpoch) run() bool {
	for epoch != nil && epoch.checkpoint() {
		for epoch.queue.pending() {
			visited, ok := epoch.visitPoints()
			if epoch.canceledByContext() {
				return false
			}
			if !ok {
				epoch.recordRunFailure(refused(SolveFailureFamilySchedule, "visit"))
				return false
			}
			if !visited {
				epoch.recordRunFailure(stalled(SolveFailureFamilySchedule, "visit-no-progress"))
				return false
			}
		}
		postfixed, ok := epoch.demandedPostfix()
		if epoch.canceledByContext() {
			return false
		}
		if !ok {
			epoch.recordRunFailure(refused(SolveFailureFamilySchedule, "postfix"))
			return false
		}
		if !postfixed {
			if !epoch.queue.pending() {
				epoch.recordRunFailure(stalled(SolveFailureFamilySchedule, "postfix-stalled"))
				return false
			}
			continue
		}
		if epoch.allRegionsNarrow() {
			return true
		}
		advanced, advancedOK := epoch.advanceNarrow()
		if !advancedOK {
			epoch.recordRunFailure(refused(SolveFailureFamilySchedule, "narrow"))
			return false
		}
		if !advanced {
			epoch.recordRunFailure(stalled(SolveFailureFamilySchedule, "narrow-no-progress"))
			return false
		}
	}
	return false
}

// completedState returns the runtime's installed immutable result only while
// the caller owns solver.mu. The retained work lease and State's existing
// completion authority bind it to this exact Solver revision.
func (solver *Solver) completedState(runtime *solverRuntime) *State {
	if solver == nil || runtime == nil || solver.runtime != runtime {
		return nil
	}
	state := runtime.completed
	if state == nil || runtime.retained == nil || !runtime.retained.Live() {
		return nil
	}
	if state.completion == nil || state.completion.store != solver.store || !state.completion.serial.Available() || state.completion.serial != solver.completion || state.completion.relation != solver.relation.Generation() {
		return nil
	}
	return state
}

// publishCompleted is the one terminal publication cut.  Its inputs have
// already passed every fallible operation while epoch is Running: query
// materialization, retention of the new root arena, and eviction of any prior
// lease.  Once complete wins, these assignments are deliberately infallible;
// cancellation after that cut is non-operative.
func (solver *Solver) publishCompleted(epoch *executorEpoch, runtime *solverRuntime, state *State, completion identity.Generation, retained *carrier.RetainedWork) bool {
	if solver == nil || epoch == nil || runtime == nil || state == nil || retained == nil || solver.runtime != runtime || state.completion == nil || state.completion.store != solver.store || state.completion.serial != completion || state.completion.relation != solver.relation.Generation() || !epoch.complete() {
		return false
	}
	runtime.retained = retained
	solver.completion = completion
	runtime.completed = state
	return true
}

// Solve executes runtime revisions iteratively. Materialized activations
// advance only the relation stamp; direct transports extend the runtime's
// structural overlay without rebuilding the sealed program.
func (solver *Solver) Solve(ctx context.Context) (state *State, status SolveStatus) {
	return solver.solve(ctx, nil, nil)
}

// SolveWithReport uses the same solve implementation as Solve and returns a
// detached first-failure certificate only when that call is incomplete. The
// report is call-local; the Solver never retains it.
func (solver *Solver) SolveWithReport(ctx context.Context) (state *State, status SolveStatus, report SolveReport) {
	state, status = solver.solve(ctx, &report, nil)
	return state, status, report
}

// SolveWithDiagnostics executes one solve with a detached, bounded aggregate
// collector and its existing first-incomplete certificate. A zero presentation
// selection avoids aggregate collection but still returns that scalar
// certificate if the single solve is incomplete. Presentation and resource
// settings never enter Snapshot identity. Invalid options return SolveInvalid
// with an empty snapshot before any solver work begins.
func (solver *Solver) SolveWithDiagnostics(ctx context.Context, options SolveDiagnosticOptions) (state *State, status SolveStatus, diagnostics SolveDiagnostics) {
	if !options.Valid() {
		return nil, SolveInvalid, SolveDiagnostics{}
	}
	collector := newSolveDiagnosticState(options)
	var failure SolveReport
	state, status = solver.solve(ctx, &failure, collector)
	if collector != nil {
		diagnostics = collector.snapshot()
	}
	diagnostics.Failure = failure
	return state, status, diagnostics
}

// solve is the one execution route. report is nil for ordinary Solve, keeping
// its successful and failure paths free of diagnostic allocation.
func (solver *Solver) solve(ctx context.Context, report *SolveReport, diagnostics *solveDiagnosticState) (state *State, status SolveStatus) {
	// A callback can panic from anywhere beneath epoch.run or query
	// materialization. Keep both ownership forms reachable by recovery: Retain
	// moves Work ownership into prepared before the epoch becomes terminal.
	var current *executorEpoch
	var prepared *carrier.RetainedWork
	defer func() {
		if recover() != nil {
			if prepared != nil {
				prepared.Close()
				prepared = nil
			}
			if current != nil {
				current.incomplete()
				current.discard()
			}
			state, status = nil, SolvePanicked
		}
		if report == nil {
			return
		}
		if status == SolveIncomplete {
			if !report.Available() {
				report.record(SolveFailureReasonExecution, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
			}
			return
		}
		// Canceled, Complete, and Panicked calls do not publish an incomplete
		// certificate, even if an earlier internal branch recorded a candidate
		// before the terminal status changed.
		*report = SolveReport{}
	}()
	if solver == nil || ctx == nil || ctx.Err() != nil {
		return nil, SolveCanceled
	}
	solver.mu.Lock()
	defer solver.mu.Unlock()
	for {
		runtime := solver.runtime
		if runtime == nil {
			return nil, SolveCanceled
		}
		if state = solver.completedState(runtime); state != nil {
			return state, SolveComplete
		}
		epoch, ok := newRuntimeEpoch(runtime, solver.relation, ctx)
		if !ok {
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			if report != nil {
				report.record(SolveFailureReasonEpoch, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
			}
			return nil, SolveIncomplete
		}
		epoch.report = report
		epoch.diagnostics = diagnostics
		epoch.diagnosticRevision = solver.relation.Generation()
		if diagnostics != nil {
			diagnostics.epochStarted(epoch, solver.relation.Generation())
		}
		generation := solver.completion.Next()
		publication, opened := beginSolvedPublication(solver, epoch, generation)
		if !opened {
			epoch.incomplete()
			epoch.discard()
			if report != nil {
				report.record(SolveFailureReasonEpoch, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
			}
			return nil, SolveIncomplete
		}
		epoch.storePub = publication
		current = epoch
		for {
			if !epoch.run() {
				epoch.incomplete()
				epoch.discard()
				if ctx.Err() != nil {
					return nil, SolveCanceled
				}
				if report != nil {
					report.record(SolveFailureReasonExecution, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			if !epoch.activationPending {
				break
			}
			frontier, canonical := canonicalizeAcceptedActivations(runtime.topology, epoch.activations)
			if !canonical {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			delta, subtracted := subtractAcceptedActivations(runtime.topology, frontier, solver.relation.Rows())
			if !subtracted {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			epoch.activations = nil
			epoch.activationPending = false
			if len(delta) == 0 {
				// Every observed Member and premise is already represented by the
				// committed relation. Keep this completed epoch for publication.
				break
			}
			accepted, merged := mergeAcceptedActivations(runtime.topology, solver.relation.Rows(), delta)
			if !merged || sameAcceptedActivations(solver.relation.Rows(), accepted) || !runtime.topology.ValidAccepted(accepted) {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			// The next relation is derived exactly once, here: Publish stamps the
			// following Generation and stores the structural digest derived for
			// it. Both installation paths below consume that one publication and
			// neither re-derives its identity. A saturated stamp fails closed.
			published, publishedOK := runtime.topology.Publish(solver.relation, accepted)
			// A materialized activation changes only the accepted relation: all of
			// its structural rows already belong to the sealed runtime. Publish the
			// new stamp without manufacturing an empty program rebuild.
			if publishedOK {
				selected, selectedOK := runtime.topology.SelectedStructuralFactorEdges(runtime.graph, delta)
				if selectedOK && len(selected) == 0 {
					solver.relation = published
					epoch.diagnosticRevision = published.Generation()
					if diagnostics != nil {
						diagnostics.observeRevision(published.Generation())
						diagnostics.resetRevisionEvidence()
					}
					continue
				}
			}
			// Direct transport must pass the overlay's exact structural fences. A
			// false result fails closed below; there is no second compiler authority.
			if publishedOK {
				overlay, preparedOverlay := runtime.prepareSelectedFactorOverlay(delta, published)
				installedOverlay := preparedOverlay && overlay != nil && epoch.installSelectedFactorOverlay(overlay)
				if installedOverlay {
					solver.relation = published
					epoch.diagnosticRevision = published.Generation()

					if diagnostics != nil {
						diagnostics.observeRevision(published.Generation())
						diagnostics.resetRevisionEvidence()
					}
					if ctx.Err() != nil {
						epoch.incomplete()
						epoch.discard()
						current = nil
						return nil, SolveCanceled
					}
					continue
				}
			}

			epoch.incomplete()
			epoch.discard()
			current = nil
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			if !publishedOK {
				if report != nil {
					report.record(SolveFailureReasonActivationRevisionOverflow, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			// Immutable program tables have no recompile path. A legal activation
			// revision must be representable as a prepared structural overlay; an
			// unsupported shape fails closed at this exact boundary.
			if report != nil {
				report.record(SolveFailureReasonActivationCompile, boundaryNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
			}
			return nil, SolveIncomplete
		}
		publication = epoch.storePub
		if publication == nil || !publication.generation.Available() {
			epoch.incomplete()
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, identity.SemanticKey{})
			return nil, SolveIncomplete
		}
		nextCompletion := publication.generation
		for index := 0; index < runtime.program.queryCount(); index++ {
			if epoch.canceled() {
				epoch.incomplete()
				epoch.discard()
				return nil, SolveCanceled
			}
			row, rowOK := runtime.program.queryAt(index)
			query, queryOK := runtime.graph.QueryAt(index)
			if !rowOK || !queryOK || !query.Key().Available() || row.point < 0 || int(row.point) >= runtime.graph.PointCount() || uint64(row.state) >= uint64(len(epoch.points)) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, identity.SemanticKey{})
				return nil, SolveIncomplete
			}
			point := query.Point()
			held, heldOK := publication.readPoint(epoch, int(row.state))
			if !heldOK {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, reportedSemanticKey(point.Key()))
				return nil, SolveIncomplete
			}
			value, queryPhase, ok := runtime.program.materializeQuery(index, epoch.work, held.State())
			if !ok {
				if epoch.canceled() {
					epoch.incomplete()
					epoch.discard()
					return nil, SolveCanceled
				}
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonQuery, queryPhase, reportedSemanticKey(point.Key()), identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			if epoch.canceled() {
				epoch.incomplete()
				epoch.discard()
				return nil, SolveCanceled
			}
			key := solvedRowKey(query.Key())
			if value != nil && !value.rowPresent() {
				value = nil
			}
			if !key.Available() || !publication.writeQuery(key, value) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, reportedSemanticKey(point.Key()))
				return nil, SolveIncomplete
			}
		}
		for index := 0; index < runtime.program.observationCount(); index++ {
			if epoch.canceled() {
				epoch.incomplete()
				epoch.discard()
				return nil, SolveCanceled
			}
			observation, observed := runtime.program.observationAt(index)
			if !observed || !observation.valid() || observation.point < 0 || int(observation.point) >= runtime.graph.PointCount() || uint64(observation.state) >= uint64(len(epoch.points)) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, identity.SemanticKey{})
				return nil, SolveIncomplete
			}
			id := observation.id
			point, pointIndex, _, pointOK := runtime.graphPointAtState(int(observation.state))
			if !id.Available() || !pointOK || pointIndex != int(observation.point) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, reportedSemanticKey(point.Key()))
				return nil, SolveIncomplete
			}
			held, heldOK := publication.readPoint(epoch, int(observation.state))
			if !heldOK {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, reportedSemanticKey(point.Key()))
				return nil, SolveIncomplete
			}
			value, observationPhase, ok := runtime.program.materializeObservation(index, epoch.work, held.State())
			if !ok || value == nil || !publication.writeObservation(id, value) {
				if epoch.canceled() {
					epoch.incomplete()
					epoch.discard()
					return nil, SolveCanceled
				}
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonQuery, observationPhase, reportedSemanticKey(point.Key()), identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
		}
		if epoch.canceled() {
			epoch.incomplete()
			epoch.discard()
			return nil, SolveCanceled
		}
		solved, committed := publication.commit(solver)
		if !committed {
			epoch.incomplete()
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, identity.SemanticKey{})
			return nil, SolveIncomplete
		}
		state = &State{completion: &completionAuthority{store: solver.store, serial: nextCompletion, relation: solver.relation.Generation()}, solved: solved}
		// Retain and eviction are preparation, not publication.  They must
		// finish while cancellation can still win the epoch terminal race.
		retained, retainedOK := epoch.work.Retain()
		if !retainedOK {
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, identity.SemanticKey{})
			return nil, SolveIncomplete
		}
		epoch.work = nil
		prepared = retained
		if epoch.canceled() {
			retained.Close()
			prepared = nil
			epoch.discard()
			return nil, SolveCanceled
		}
		prior := runtime.retained
		if prior != nil && !prior.Close() {
			retained.Close()
			prepared = nil
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, identity.SemanticKey{})
			return nil, SolveIncomplete
		}
		// A successfully evicted lease is no longer a cache, even if the
		// following cancellation prevents this candidate from publishing.
		runtime.retained = nil
		if epoch.canceled() {
			retained.Close()
			prepared = nil
			epoch.discard()
			return nil, SolveCanceled
		}
		if !solver.publishCompleted(epoch, runtime, state, nextCompletion, retained) {
			retained.Close()
			prepared = nil
			epoch.discard()
			return nil, SolveCanceled
		}
		prepared = nil
		epoch.discard()
		current = nil
		return state, SolveComplete
	}
}
