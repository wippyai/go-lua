package engine

import "sort"

func (solver *Solver) rebuildEpoch(relations []activeRelation) bool {
	if solver == nil {
		return false
	}
	// Build every derived carrier and schedule field on a private Solver copy.
	// Relation membership and coordinate interning are already monotone
	// structural inputs; cancellation can discard this derived candidate but
	// cannot restore an older accepted relation set.
	candidate := *solver
	if !candidate.refreshCarrier(relations) || !candidate.compileQueries(relations) {
		return false
	}
	// Candidate Factor planes remain private until both the fresh carrier and
	// every derived schedule/query table have compiled.  Commit is the sole
	// binding-adoption cut; it has no rollback or alternate carrier path.
	if candidate.epoch == nil || !candidate.epoch.commit() {
		return false
	}
	// The complete private Facts generation and its schedule belong to one
	// accepted relation epoch.  Swap them only after carrier and schedule
	// construction succeed; no partial schema or compiler view can escape a
	// failed reformation.
	solver.guards, solver.decisionAtoms = candidate.guards, candidate.decisionAtoms
	solver.epoch = candidate.epoch
	solver.facts, solver.zero, solver.entry = candidate.facts, candidate.zero, candidate.entry
	solver.actions, solver.schedule, solver.regions = candidate.actions, candidate.schedule, candidate.regions
	solver.presenceFollowers = candidate.presenceFollowers
	solver.roots, solver.queryRoots, solver.queryResults, solver.entrySeeds = candidate.roots, candidate.queryRoots, candidate.queryResults, candidate.entrySeeds
	solver.supportCatalog, solver.supportTargets = candidate.supportCatalog, candidate.supportTargets
	return true
}

// nextAcceptedRelations computes one complete canonical monotone relation
// update. The caller performs the single acceptance assignment afterwards;
// this calculation deliberately does not poll cancellation once that cut
// begins.
func (transaction *transaction) nextAcceptedRelations() ([]activeRelation, bool) {
	if transaction == nil || transaction.solver == nil {
		return nil, false
	}
	active := transaction.solver.active
	for index := range active {
		if !transaction.solver.validActiveRelation(active[index]) {
			return nil, false
		}
		if index != 0 && transaction.solver.compareActiveRelation(active[index-1], active[index]) >= 0 {
			// The compiled epoch owns one already-canonical active universe.
			return nil, false
		}
	}

	// A discovery has no within-epoch support: the rebuilt carrier recomputes
	// support from its selectors.  Sorting its whole finite batch once makes
	// repeated selections idempotent without a per-discovery scan or an
	// auxiliary identity table.
	// Canonicalization owns only this discovery batch. The Relation operands
	// themselves were allocated at Bind and are immutable thereafter, so a
	// shallow sequence copy is enough to protect transaction scratch while
	// avoiding a second copy of every ordered operand tuple.
	discovered := append([]activeRelation(nil), transaction.discovered...)
	for index := range discovered {
		if !transaction.solver.validActiveRelation(discovered[index]) {
			return nil, false
		}
	}
	sort.Slice(discovered, func(left, right int) bool {
		return transaction.solver.compareActiveRelation(discovered[left], discovered[right]) < 0
	})
	unique := discovered[:0]
	for index := range discovered {
		if len(unique) == 0 || transaction.solver.compareActiveRelation(unique[len(unique)-1], discovered[index]) != 0 {
			unique = append(unique, discovered[index])
		}
	}

	// Both inputs are canonical strict sequences.  The sole epoch transition
	// is their linear set union, preserving deterministic order and copying no
	// duplicate relation into the next carrier.
	merged := make([]activeRelation, 0, len(active)+len(unique))
	left, right := 0, 0
	for left < len(active) && right < len(unique) {
		order := transaction.solver.compareActiveRelation(active[left], unique[right])
		switch {
		case order < 0:
			merged = append(merged, active[left])
			left++
		case order > 0:
			merged = append(merged, unique[right])
			right++
		default:
			merged = append(merged, active[left])
			left++
			right++
		}
	}
	merged = append(merged, active[left:]...)
	merged = append(merged, unique[right:]...)
	return merged, true
}
