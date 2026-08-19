// runtime_point_refresh.go publishes point state and refreshes one point.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// installPoint is the epoch's one install cut into the published point plane.
// Every path that replaces a point passes the evidence its producing
// operation classified; a path that publishes no predecessor-to-successor
// transition passes the zero Set, which reads as unclassified and which every
// admissibility consumer refuses. Keeping the install behind one function is
// what makes that obligation structural instead of conventional.
func (epoch *executorEpoch) installPoint(point int, next carrier.PointState, evidence change.Set) bool {
	if epoch == nil || point < 0 || point >= len(epoch.points) {
		return false
	}
	epoch.points[point] = next
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordPublicationEvidence(evidence)
	}
	return true
}

func (epoch *executorEpoch) publish(point int, current, next carrier.PointState, changes carrier.ChangeSet, order pointPublication) (bool, bool) {
	if epoch == nil || epoch.canceled() || point < 0 || point >= len(epoch.points) || point >= len(epoch.structural.pointDescent) || !epoch.work.OwnsPointState(current) || !epoch.work.OwnsPointState(next) || !epoch.runtime.carrier.OwnsChangeSet(changes) || order != publicationAscending && order != publicationMayDescend {
		return false, false
	}
	coverageChanges, coverageOK := epoch.work.CoverageWakeChangesPointStates(current, next)
	if !coverageOK {
		return false, false
	}
	semanticChanged := !epoch.work.EqualPointState(current, next)
	// A compact Target-row alias can preserve the lifted semantic Point while
	// replacing the exact State+C header. Structural consumers cache a source
	// version, not an extensional quotient, so that replacement must advance
	// the publication generation and wake their canonical refold. Otherwise a
	// later additive ticket could be replayed over the stale alias.
	changed := !epoch.work.ExactSamePointRepresentation(current, next)
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordPublication(semanticChanged, changed)
	}
	evidence := changes.Evidence().Union(coverageChanges.Evidence())
	if !epoch.installPoint(point, next, evidence) {
		return false, epoch.fail()
	}
	if epoch.storePub != nil && !epoch.storePub.writePoint(epoch, point, next) {
		return false, epoch.fail()
	}
	var sourcePoint equation.Point
	if changed {
		// The operand plane is the sole representation identity. Marking every
		// operand this Point mints a value for is the whole publication record:
		// no Region, producer or edge keeps a version beside it.
		if !epoch.markSourceOperands(point, evidence) {
			return false, epoch.fail()
		}
		if epoch.diagnostics != nil {
			epoch.diagnostics.recordVersionBump()
		}
		if order == publicationMayDescend && semanticChanged && !epoch.recordPointDescent(point) {
			return false, epoch.fail()
		}
		if !epoch.markPostfixDirty(point) || !epoch.invalidatePostfixAncestors(point) {
			return false, epoch.fail()
		}
		var sourceOK bool
		sourcePoint, sourceOK = epoch.runtime.graph.PointAt(schedule.Node(point))
		if !sourceOK {
			return false, epoch.fail()
		}
		if !epoch.markStructuralSuccessors(sourcePoint) {
			return false, epoch.fail()
		}
	}
	// Semantic and coverage evidence share one accumulator, so a Group reached
	// through both channels arrives once. There is no cross-channel comparison
	// left for the consumer to perform.
	wakes, ok := epoch.demand.Route(point, changes, coverageChanges)
	if !ok {
		return false, epoch.fail()
	}
	for _, wake := range wakes {
		if !epoch.markDirty(wake.Group) {
			return false, epoch.fail()
		}
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordWakes(wakes)
	}
	if changed && !epoch.markPublishedInputConsumers(sourcePoint) {
		return false, epoch.fail()
	}
	return changed, true
}

// publishAcyclicExact installs one already-complete PointRHS without deriving
// an old-to-new typed ChangeSet. Acyclic scheduling already owns the exact
// static consumer incidence: conservatively waking that row is semantically
// equivalent to routing every changed Unit, while avoiding a second FDD
// zipper solely for invalidation evidence. This changes scheduling precision,
// never abstract-state precision. Recurrence publication retains the exact
// ChangeSet path above for its phase/order proofs.
func (epoch *executorEpoch) publishAcyclicExact(point int, current, next carrier.PointState, order pointPublication, evidence change.Set) (bool, bool) {
	if epoch == nil || epoch.canceled() || point < 0 || point >= len(epoch.points) || point >= len(epoch.structural.pointDescent) ||
		!epoch.work.OwnsPointState(current) || !epoch.work.OwnsPointState(next) || order != publicationAscending && order != publicationMayDescend {
		return false, false
	}
	semanticChanged := !epoch.work.EqualPointState(current, next)
	// See publish: raw compact-C replacement is a versioned structural event
	// even when the lifted Point value is equal. The acyclic path installs the
	// evidence its producing operation classified but routes no typed
	// ChangeSet, so its conservative graph wake remains the barrier that
	// prevents a stale cursor from bridging the representation change.
	changed := !epoch.work.ExactSamePointRepresentation(current, next)
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordPublication(semanticChanged, changed)
	}
	// Install the exact RHS representation even when it is observationally
	// equal under current support. Replace has the same law: a later support
	// growth must see the newly recomputed latent representation, not the old
	// PointState's hidden branch.
	if !epoch.installPoint(point, next, evidence) {
		return false, epoch.fail()
	}
	if epoch.storePub != nil && !epoch.storePub.writePoint(epoch, point, next) {
		return false, epoch.fail()
	}
	if !changed {
		return semanticChanged, true
	}
	// A Region accumulator reads this value only through an ingress operand,
	// and it needs both axes classified: the fold ChangeSet carries the state
	// axis, so the authored-coverage direction is derived here and nowhere
	// else. Every other acyclic publication keeps its single traversal.
	if epoch.operands.classifiesCoverage(point) {
		coverageChanges, coverageOK := epoch.work.CoverageWakeChangesPointStates(current, next)
		if !coverageOK {
			return false, epoch.fail()
		}
		evidence = evidence.Union(coverageChanges.Evidence())
	}
	if !epoch.markSourceOperands(point, evidence) {
		return false, epoch.fail()
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordVersionBump()
	}
	if order == publicationMayDescend && semanticChanged && !epoch.recordPointDescent(point) {
		return false, epoch.fail()
	}
	sourcePoint, sourceOK := epoch.runtime.graph.PointAt(schedule.Node(point))
	if !sourceOK || !epoch.markStructuralSuccessors(sourcePoint) {
		return false, epoch.fail()
	}
	if !epoch.markPostfixDirty(point) || !epoch.invalidatePostfixAncestors(point) {
		return false, epoch.fail()
	}
	// Every exact or dynamic typed read of this Point belongs to one ordinary
	// graph consumer. Waking the canonical consumer row subsumes unit/factor
	// routing without inventing a parallel dependency graph. Clean-only wake is
	// safe after typed/coverage routing has already scheduled any pending row.
	if !epoch.markPublishedInputConsumers(sourcePoint) {
		return false, epoch.fail()
	}
	return true, true
}

// refreshPoint performs the only candidate replacement and sole Point
// publication. A region is admitted only for its head; nonheads exact-replace
// their complete RHS even while enclosed by the same WTO region.
func (epoch *executorEpoch) refreshPoint(point equation.Point, pointIndex, regionIndex int) (changed, ok bool) {
	refreshBoundary := refused(SolveFailureFamilyRefresh, "validation")
	defer func() {
		if !ok {
			epoch.recordRefreshPointFailure(refreshBoundary, point)
		}
	}()
	if epoch == nil || epoch.canceled() || !point.Available() || pointIndex < 0 || pointIndex >= len(epoch.points) || pointIndex >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[pointIndex] {
		return false, false
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordRefresh()
	}
	current := epoch.points[pointIndex]
	if !epoch.work.OwnsPointState(current) {
		return false, false
	}
	structuralChanged := pointIndex < len(epoch.structuralDirty) && epoch.structuralDirty[pointIndex]
	anyCandidateChanged := false
	candidateRequiresCanonicalFold := false
	// Outside recurrence, an ascending replacement c<=c' may be installed in
	// the existing exact point aggregate by joining the complete c'. If
	// X=base join rest join c, then X join c'=base join rest join c'. This is
	// the seminaive scalar law; it needs neither subtraction nor a retained
	// operand tree. Structural changes remain on the exact rebuild path below
	// because a source Point may have descended during a narrow episode.
	appended, appendedOK := epoch.work.PointRHSFromPointState(current)
	if !appendedOK {
		return false, false
	}
	// An RHS that never leaves its predecessor is a classified no-movement
	// publication. Each overlay unions its own issued evidence into this
	// accumulator; a branch that publishes no transition contributes an
	// unclassified Set and the accumulation stays unclassified.
	appendEvidence := change.Classified()
	refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate")
	for index := 0; index < epoch.runtime.graph.ProducerCount(point); index++ {
		group, groupOK := epoch.runtime.graph.ProducerAt(point, index)
		groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
		if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) {
			return false, false
		}
		producer := epoch.runtime.producers[groupIndex]
		state := &epoch.producers[groupIndex]
		if producer.group.Output() != point || state.applied == state.generation {
			continue
		}
		next, reads, ok := epoch.evaluate(producer, state)
		if !ok || epoch.canceled() {
			if !ok {
				epoch.recordGroupFailure(SolveFailureReasonExecution, point, producer.group)
			}
			return false, false
		}
		// The only acyclic append proof is carrier-owned. Let its success prove
		// both lifted order and raw compact-row additivity in one traversal; on
		// failure, run the ordinary order check once solely to distinguish a
		// lawful raw-alias replacement (canonical fold) from a broken Rule law.
		refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate-order")
		thisCandidateChanged := !state.hasValue
		candidateAppendable := true
		candidateOrdered := true
		// candidateEvidence is this operand's classified transition. Only a
		// carrier proof establishes a direction: the lifted order alone does
		// not, because LessOrEq is satisfied by a contribution whose authored
		// coverage shrank under equal support, and an accumulator that retains
		// this operand's earlier rows would then keep coverage the successor
		// dropped. The raw-additivity proof is exactly the missing premise, so
		// an ascent is issued only when the carrier grants it.
		candidateEvidence := change.Set{}
		candidateAdditive := false
		if state.hasValue {
			thisCandidateChanged = !epoch.work.ExactSameRuleContributionRepresentation(state.candidate, next)
			if regionIndex == schedule.NoRegion {
				if thisCandidateChanged {
					candidateAppendable = epoch.work.CanAppendAscendingRuleContribution(state.candidate, next)
					candidateOrdered = candidateAppendable
					candidateAdditive = candidateAppendable
					if !candidateOrdered {
						candidateOrdered = epoch.work.LessOrEqRuleContribution(state.candidate, next)
					}
				}
			} else {
				// A Region head publication needs only the ordinary
				// monotonicity law. The recurrence accumulator needs more, so
				// the raw-row append certificate is taken here as this
				// operand's ascent evidence and nowhere else.
				candidateOrdered = epoch.work.LessOrEqRuleContribution(state.candidate, next)
				if thisCandidateChanged && candidateOrdered {
					candidateAdditive = epoch.work.CanAppendAscendingRuleContribution(state.candidate, next)
				}
			}
		}
		environmentChanged := producer.environment != nil && epoch.producerInputChanged(groupIndex, producer.group.InputCount(), state.rememberAt)
		if state.hasValue && thisCandidateChanged && candidateOrdered && candidateAdditive {
			candidateEvidence = change.Set{Reasons: change.ChangedUnit, Direction: change.Known | change.Ascends}
		}
		if state.hasValue && !candidateOrdered {
			// A wake generation is not semantic evidence. A candidate decrease or
			// incomparability is lawful only while an unchanged narrow episode is
			// propagating its smaller exact head around an internal edge. During
			// ascent, unchanged interfaces imply that every changed input belongs to
			// the same ascending Kleene chain. Inclusion is sufficient; a defined
			// Widen that dominates both cells is the same progress a recurrent
			// head uses, so a replaced summand may change control family. Fail
			// only a replacement that has no such upper bound. A genuinely
			// changed external interface still begins a fresh episode below.
			if (state.rememberAt == 0 || !epoch.producerInputsChanged(groupIndex, producer.group.InputCount(), state.rememberAt)) && !environmentChanged {
				refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate-order-stable-inputs")
				epoch.recordCandidateOrderFailure(refreshBoundary, point, producer.group)
				return false, false
			}
			region := epoch.runtime.pointRegion[pointIndex]
			if region == schedule.NoRegion || !epoch.activeRegion(region) {
				if !epoch.work.AscentOrderedRuleContribution(state.candidate, next) {
					refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate-order-region")
					epoch.recordCandidateOrderFailure(refreshBoundary, point, producer.group)
					return false, false
				}
			} else {
				phase := epoch.regions[region].phase
				if phase != phaseAscent && phase != phaseNarrow {
					refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate-order-region")
					epoch.recordCandidateOrderFailure(refreshBoundary, point, producer.group)
					return false, false
				}
				// Candidate tokens are the complete producer input proof. Only a
				// changed source outside this immutable Region is an interface
				// restart; internal changes remain subject to the phase order law.
				candidateExternal := false
				candidateExternalOK := true
				if state.rememberAt != 0 {
					for inputIndex := 0; inputIndex < producer.group.InputCount(); inputIndex++ {
						if !epoch.producerInputChanged(groupIndex, inputIndex, state.rememberAt) {
							continue
						}
						input, inputOK := producer.group.InputAt(inputIndex)
						if !inputOK {
							candidateExternalOK = false
							break
						}
						inside, contained := epoch.regionContains(region, input.Point())
						if !contained {
							candidateExternalOK = false
							break
						}
						if !inside {
							candidateExternal = true
							break
						}
					}
				}
				if candidateExternalOK && !candidateExternal && environmentChanged {
					environment, inputOK := producer.group.EnvironmentInput()
					if !inputOK || producer.environment == nil {
						candidateExternalOK = false
					} else {
						inside, contained := epoch.regionContains(region, environment.Point())
						candidateExternalOK = contained
						candidateExternal = !inside && contained
					}
				}
				if !candidateExternalOK {
					return false, false
				}
				if candidateExternal || epoch.regionExternalIngressChanged(region) {
					if !epoch.restartRegion(region, solveDiagnosticRestartCandidateInterface, solveDiagnosticRestartCandidateNotOrdered, groupIndex, next) {
						return false, false
					}
					return false, true
				}
				if phase != phaseNarrow {
					if !epoch.work.AscentOrderedRuleContribution(state.candidate, next) {
						refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate-order-region")
						epoch.recordCandidateOrderFailure(refreshBoundary, point, producer.group)
						return false, false
					}
				} else if !epoch.work.LessOrEqRuleContribution(next, state.candidate) {
					// An unchanged narrow interface proves only where the wake came
					// from. It does not turn an incomparable Rule result into a
					// descent. Admit exactly next <= old; every other local result
					// fails closed instead of being published as an exact candidate.
					refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate-order-descent")
					epoch.recordCandidateOrderFailure(refreshBoundary, point, producer.group)
					return false, false
				} else {
					// next <= old is the only local result a narrow episode
					// admits, so reaching here is a classified descent.
					candidateEvidence = change.Set{Reasons: change.ChangedUnit, Direction: change.Known | change.Descends}
				}
			}
		}
		refreshBoundary = refused(SolveFailureFamilyRefresh, "demand-commit")
		if epoch.canceled() || !epoch.demand.Replace(groupIndex, reads) {
			return false, false
		}
		// Candidate identity is State+C, not merely its lifted extensional
		// value. An alias replacement can be semantically equal while changing
		// the compact row retained by the current Point RHS; appending the new
		// candidate to that old RHS would retain both aliases. The carrier owns
		// the raw-additivity proof, and a failure selects the existing canonical
		// fold rather than a compensating merge path.
		changed := thisCandidateChanged
		appendable := candidateAppendable
		if !epoch.updateCandidatesPending(pointIndex, -1) {
			return false, false
		}
		state.candidate, state.hasValue, state.applied = next, true, state.generation
		// The install closes this candidate's input epoch: every operand this
		// evaluation read lies at or below the stamp, and every later mark on
		// one of them is a changed input.
		state.rememberAt = epoch.operands.advance()
		if changed {
			if !epoch.markGroupOperands(groupIndex, candidateEvidence) {
				return false, false
			}
			// A dirty structural row takes the complete canonical fold below,
			// which already reads every freshly installed producer candidate.
			// Building an append-only RHS here would publish intermediate roots
			// only to discard them immediately before that exact reconstruction.
			if regionIndex == schedule.NoRegion && !structuralChanged && !appendable {
				candidateRequiresCanonicalFold = true
			}
			// Once any replacement is non-additive, ignore any partial append
			// already accumulated this refresh and rebuild in canonical input
			// order below. This is one Point RHS authority, not a side cache.
			if regionIndex == schedule.NoRegion && !structuralChanged && !candidateRequiresCanonicalFold {
				var merged bool
				var appendChanges carrier.ChangeSet
				appended, appendChanges, merged = epoch.work.AddRuleContribution(appended, next)
				if !merged || epoch.canceled() {
					return false, false
				}
				appendEvidence = appendEvidence.Union(appendChanges.Evidence())
			}
		}
		if epoch.canceled() {
			return false, false
		}
		anyCandidateChanged = anyCandidateChanged || changed
		refreshBoundary = refused(SolveFailureFamilyRefresh, "candidate")
	}
	if regionIndex == schedule.NoRegion && !anyCandidateChanged && !structuralChanged {
		refreshBoundary = refused(SolveFailureFamilyRefresh, "acyclic-publication")
		return false, epoch.settlePostfix(pointIndex)
	}
	if regionIndex == schedule.NoRegion {
		rhs := appended
		evidence := appendEvidence
		order := publicationAscending
		if structuralChanged || candidateRequiresCanonicalFold {
			refreshBoundary = stalled(SolveFailureFamilyRefresh, "acyclic-structural-inputs")
			ascending, valid := epoch.structuralInputsAscending(pointIndex)
			if !valid {
				return false, false
			}
			refreshBoundary = refused(SolveFailureFamilyRefresh, "acyclic-point-base")
			base, ok := epoch.pointBase(point, pointIndex)
			if !ok {
				return false, false
			}
			refreshBoundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-point")
			foldBoundary := boundaryNone
			var foldChanges carrier.ChangeSet
			rhs, foldChanges, foldBoundary, ok = epoch.foldPointWithBoundary(current, base, point)
			if !ok {
				refreshBoundary = foldBoundary
				return false, false
			}
			// The canonical fold committed reference==current to this RHS, so
			// its own ChangeSet is exactly this publication's evidence and
			// replaces whatever the abandoned append accumulated.
			evidence = foldChanges.Evidence()
			if !ascending && !epoch.work.LessOrEqPointStateRHS(current, rhs) {
				order = publicationMayDescend
			}
		}
		refreshBoundary = refused(SolveFailureFamilyRefresh, "acyclic-publication")
		selfDescent := epoch.structural.pointDescent[pointIndex]
		published, ok := epoch.work.PublishPointRHS(rhs)
		if !ok || epoch.canceled() {
			return false, false
		}
		changed, publishedOK := epoch.publishAcyclicExact(pointIndex, current, published, order, evidence)
		if !publishedOK || !epoch.settlePostfix(pointIndex) {
			return false, false
		}
		if structuralChanged && !epoch.rememberStructuralInputs(pointIndex, selfDescent) {
			return false, false
		}
		epoch.structuralDirty[pointIndex] = false
		return changed, true
	}
	refreshBoundary = refused(SolveFailureFamilyRefresh, "region-interface")
	if !epoch.activeRegion(regionIndex) || epoch.runtime.regions[regionIndex].head != pointIndex {
		return false, false
	}
	episode := &epoch.regions[regionIndex]
	phase := episode.phase
	if phase != phaseAscent && phase != phaseNarrow {
		return false, false
	}
	externalIngressChanged := epoch.regionExternalIngressChanged(regionIndex)
	if externalIngressChanged {
		if phase == phaseAscent && episode.hasExact {
			// A stale ascent boundary is a localized refresh, not automatically a
			// new exact episode. Publication routing has already dirtied ordinary
			// consumers; the barrier waits for their candidates before rebuilding E/R.
			if !episode.interfaceRefreshPending && !epoch.beginRegionInterfaceRefresh(regionIndex) {
				return false, false
			}
		} else {
			if !epoch.restartRegion(regionIndex, solveDiagnosticRestartHeadInterface, solveDiagnosticRestartInterfaceChanged, -1, carrier.RuleContribution{}) {
				return false, false
			}
			return false, true
		}
	}
	exactInputsChanged := episode.hasExact && epoch.regionExactInputsChanged(regionIndex)
	if episode.invalid {
		if !epoch.regionCandidatesSettled(regionIndex) {
			return epoch.enqueuePoint(pointIndex), true
		}
		episode.invalid = false
	}
	region := epoch.runtime.regions[regionIndex]
	// An unchanged ascent RHS is already represented by episode.exact and the
	// current widened Point. Re-running Widen would only rebuild the same roots
	// and coverage before proving the same postfix relation again.
	if phase == phaseAscent && episode.hasExact && !exactInputsChanged {
		epoch.structuralDirty[pointIndex] = false
		return false, epoch.settlePostfix(pointIndex)
	}
	refreshPending := phase == phaseAscent && episode.hasExact && episode.interfaceRefreshPending
	refreshOldExact := episode.exact
	var exact, selected carrier.PointRHS
	var exactChanges carrier.ChangeSet
	var exactOK bool
	structuralFolded := false
	refreshBoundary = refused(SolveFailureFamilyRefresh, "region-rhs")
	if phase == phaseAscent && episode.hasExact {
		// A changed Region rebuilds its E+B carrier in canonical input order.
		// The operands it must admit are the ones the operand plane says moved,
		// folded onto this episode's retained accumulator, whenever their
		// accumulated evidence admits reuse; every other case rebuilds the
		// complete row from Init. regionRHS owns that choice.
		exact, exactChanges, exactOK = epoch.regionRHS(point, pointIndex, regionIndex, region, current)
		structuralFolded = exactOK
		if exactOK {
			if refreshPending {
				// The carrier law for an ascending cached exact proves
				// Widen(current, R, R) preserves current and carries R. Avoid
				// rebuilding the ordinary X+B selected fold in this refresh.
				selected = exact
			} else {
				selected, exactOK = epoch.regionSelected(current, region)
			}
		}
	} else if phase == phaseNarrow && episode.hasExact && !exactInputsChanged {
		// Narrow may need several semantic descents against one unchanged exact
		// RHS. Reuse that owner-issued carrier rather than reconstructing E+B on
		// every narrow step.
		exact, selected, exactOK = episode.exact, episode.exact, true
	} else {
		exact, exactChanges, exactOK = epoch.regionRHS(point, pointIndex, regionIndex, region, current)
		structuralFolded = exactOK
	}
	if !exactOK || epoch.canceled() {
		return false, false
	}
	// Support-axis widening is the ascent's second recurrence operator. Value
	// widening bounds how far a coordinate climbs; this bounds how finely the
	// head partitions its guard support while it climbs, by discharging the
	// coordinates the Region's own cycle introduces at its head. It applies to
	// exactly the operands region.widen applies to, in exactly the phase that
	// widens; the narrow descent below reads the exact relations unchanged.
	if phase == phaseAscent {
		refreshBoundary = refused(SolveFailureFamilyRefresh, "region-discharge")
		if exact, exactOK = epoch.dischargeAscentRHS(region, exact); !exactOK {
			return false, false
		}
		switch {
		case refreshPending:
			// A pending refresh already selected the exact RHS itself, so the one
			// discharge above is the whole widening for both operands.
			selected = exact
		case episode.hasExact:
			if selected, exactOK = epoch.dischargeAscentRHS(region, selected); !exactOK {
				return false, false
			}
		}
	}
	// An interface refresh rebuilds this head against a boundary that moved
	// underneath the cached episode row, so it may continue only when its
	// complete exact RHS grows from that row; anything else is a stale
	// refresh and restarts the region from Init.
	//
	// An ordinary ascent step carries no such demand. Only the head is
	// admitted as a Region; every enclosed nonhead exact-replaces its complete
	// RHS, so the head's one-step image is the Bourdoncle image of a vector
	// whose interior components are assigned rather than accumulated. That
	// image is free to move between steps. The ascent is carried by the head
	// publication below, where Widen bounds the new row above the current
	// point, and each interior replacement is separately admitted at the
	// candidate boundary; demanding inclusion on the image itself refuses a
	// lawful step with no path forward.
	refreshBoundary = refused(SolveFailureFamilyRefresh, "region-ascent-monotone")
	if phase == phaseAscent && refreshPending && episode.hasExact && !epoch.work.LessOrEqPointRHS(episode.exact, exact) {
		if epoch.diagnostics != nil {
			epoch.diagnostics.recordInterfaceRefreshOutcome(epoch, regionIndex, refreshOldExact, exact, false, true)
		}
		if !epoch.restartRegion(regionIndex, solveDiagnosticRestartAscentIngress, solveDiagnosticRestartExactIncomparable, -1, carrier.RuleContribution{}) {
			return false, false
		}
		return false, true
	}
	if phase == phaseNarrow {
		if !episode.hasExact {
			return false, false
		}
		// A descent may follow a smaller exact RHS.  A larger or incomparable
		// one, however, invalidates every narrowed history even if it still fits
		// below the current widened head.  Restart clears the complete region and
		// its descendants from Init before any new ascent publication.
		if !epoch.work.EqualPointRHS(episode.exact, exact) && !epoch.work.LessOrEqPointRHS(exact, episode.exact) {
			if !epoch.restartRegion(regionIndex, solveDiagnosticRestartNarrowExact, solveDiagnosticRestartExactIncomparable, -1, carrier.RuleContribution{}) {
				return false, false
			}
			return false, true
		}
		if !epoch.work.LessOrEqPointRHSPoint(exact, current) {
			if !epoch.restartRegion(regionIndex, solveDiagnosticRestartNarrowCurrent, solveDiagnosticRestartExactNotBelowCurrent, -1, carrier.RuleContribution{}) {
				return false, false
			}
			return false, true
		}
	}
	refreshBoundary = refused(SolveFailureFamilyRefresh, "region-merge")
	var published carrier.PointState
	var changes carrier.ChangeSet
	var publishedOK bool
	if phase == phaseAscent && !episode.hasExact {
		// The canonical fold already committed reference==current to exact and
		// issued this ChangeSet. Consume it directly only while no support-axis
		// discharge has transformed exact; otherwise the folded evidence no longer
		// matches the published RHS and the existing replacement cut remains the
		// correct owner operation.
		if !region.discharge.available() && epoch.runtime.carrier.OwnsChangeSet(exactChanges) {
			published, publishedOK = epoch.work.PublishPointRHS(exact)
			changes = exactChanges
		} else {
			published, changes, publishedOK = epoch.work.ReplacePointWithRHS(current, exact)
		}
	} else if phase == phaseAscent {
		published, changes, publishedOK = epoch.work.MergeSelectedPointState(carrier.Widen, current, selected, exact, region.widen)
	} else {
		published, changes, publishedOK = epoch.work.MergeSelectedPointState(carrier.Narrow, current, exact, exact, region.narrow)
	}
	if !publishedOK || epoch.canceled() {
		return false, false
	}
	refreshBoundary = refused(SolveFailureFamilyRefresh, "region-publication")
	if !episode.dropPostfixProof() {
		return false, false
	}
	order := publicationAscending
	if phase == phaseNarrow {
		order = publicationMayDescend
	}
	selfDescent := epoch.structural.pointDescent[pointIndex]
	changed, publishedOK = epoch.publish(pointIndex, current, published, changes, order)
	if !publishedOK || epoch.canceled() {
		return false, false
	}
	episode.exact, episode.hasExact = exact, true
	if epoch.canceled() || !epoch.rememberRegionInterfaces(regionIndex) {
		return false, false
	}
	episode.interfaceRefreshPending = false
	if refreshPending && epoch.diagnostics != nil {
		// A refresh is complete only after the new head publication and its
		// authoritative interface/version snapshot both succeed.
		epoch.diagnostics.recordInterfaceRefreshOutcome(epoch, regionIndex, refreshOldExact, exact, true, false)
	}
	if structuralFolded && !epoch.rememberStructuralInputs(pointIndex, selfDescent) {
		return false, false
	}
	epoch.structuralDirty[pointIndex] = false
	if phase == phaseNarrow && changed && !epoch.enqueuePoint(pointIndex) {
		return false, false
	}
	return changed, true
}
