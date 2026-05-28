package flow

import (
	"slices"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// buildEdgeNumericConstraints populates the edge numeric constraint map.
//
// Numeric constraints arise from comparison operators (x < y, x >= 10, etc.)
// and are used to detect unreachable edges and track integer bounds. This
// method collects all numeric constraints from the inputs and indexes them
// by edge key for efficient lookup during constraint checking.
//
// Multiple constraints on the same edge are accumulated into a slice,
// as they all must be satisfied when traversing that edge.
func (s *Solution) buildEdgeNumericConstraints() {
	if s.inputs == nil {
		return
	}
	for _, edge := range s.inputs.EdgeNumericConstraints {
		if len(edge.Constraints) == 0 {
			continue
		}
		if s.edgeNumericConstraints == nil {
			s.edgeNumericConstraints = make(map[edgeKey][]constraint.NumericConstraint, len(s.inputs.EdgeNumericConstraints))
		}
		key := edgeKey{from: edge.From, to: edge.To}
		s.edgeNumericConstraints[key] = append(s.edgeNumericConstraints[key], edge.Constraints...)
	}
}

// numericWorklistState carries the per-point bookkeeping the numeric component
// of the worklist fixpoint needs across visits: whether a point has produced its
// first non-Top state (seeded) and whether a loop header has already widened to
// Top (widenedTop). It is shared by the unified value worklist (solve) and the
// value-less numeric fixpoint of the condition view.
type numericWorklistState struct {
	seeded     map[cfg.Point]bool
	widenedTop map[cfg.Point]bool
}

func newNumericWorklistState() *numericWorklistState {
	return &numericWorklistState{
		seeded:     make(map[cfg.Point]bool),
		widenedTop: make(map[cfg.Point]bool),
	}
}

// numericTransferAt recomputes the numeric component at point p and updates
// s.numericAt in place, returning whether the stored numeric state changed.
//
// This is the per-point numeric transfer of the unified worklist: it joins the
// edge-constrained predecessor states (computeNumericStateAt), then merges that
// raw contribution into the prior stored state with plain Join at ordinary
// points and Cousot Widen at loop headers (pointIsWideningSite), preserving the
// first-non-Top seeding discipline and the widened-to-Top latch. Top is stored
// as the absent slot.
func (s *Solution) numericTransferAt(p cfg.Point, ws *numericWorklistState) bool {
	c := s.inputs.Graph
	// The entry point starts the analysis at Top and is never assigned a numeric
	// state: it carries no incoming numeric edge contribution. Skipping it keeps
	// the entry slot Top even for degenerate CFGs with an edge back to entry.
	if p == c.Entry() {
		return false
	}
	oldState := s.numericAt[p]
	rawState := s.computeNumericStateAt(c, p, s.numericAt)
	rawState = s.applyNumericLengthEffects(p, rawState)
	var newState *numeric.State
	switch {
	case ws.widenedTop[p]:
		newState = nil
	case !ws.seeded[p]:
		// First-visit seed; widening at an uninitialized point would
		// return Top because the corrected numeric.Widen treats nil as
		// the lattice Top, not as "no state yet".
		newState = rawState
	case !pointIsWideningSite(c, p):
		// Cousot widening points: only loop headers can host infinite
		// ascending chains. Non-loop-header points use plain Join, which
		// preserves precision and terminates because chain length is
		// bounded by the lattice height of predecessor contributions.
		newState = numeric.Join(oldState, rawState)
	default:
		// Loop header — Cousot widening to guarantee termination on
		// infinite-height interval chains.
		newState = numeric.Widen(oldState, rawState)
		if numericStateWidenedToTop(oldState, rawState, newState) {
			ws.widenedTop[p] = true
			newState = nil
		}
	}

	// Mark seeded only when the visit produces a meaningful (non-Top)
	// state. Top (nil) is the join-absorbing element under the corrected
	// numeric.Join — once we mark Top as "seeded" and a later visit
	// produces a real rawState, Join(Top, rawState) = Top destroys the
	// fact. By gating on non-Top, the worklist re-enters the
	// first-visit-seed branch until the first real numeric observation
	// reaches p, then Join thereafter accumulates correctly.
	if newState != nil && !newState.IsTop() {
		ws.seeded[p] = true
	}

	if newState.Equals(oldState) {
		return false
	}
	if newState == nil || newState.IsTop() {
		delete(s.numericAt, p)
	} else {
		if s.numericAt == nil {
			s.numericAt = make(map[cfg.Point]*numeric.State)
		}
		s.numericAt[p] = newState
		s.registerPointNumericShapeReachabilityDeps(p)
	}
	// p's shape-reachability and length-shape projection read its numeric state,
	// so a numeric change invalidates any cached reachability for p.
	if s.reachabilityCache != nil {
		delete(s.reachabilityCache, p)
	}
	return true
}

// finalizeUnsatEdges marks edges whose numeric constraints are unsatisfiable
// given the converged numeric state at the edge source. It runs once after the
// numeric component of the worklist has reached its fixed point.
func (s *Solution) finalizeUnsatEdges() {
	if len(s.edgeNumericConstraints) == 0 {
		return
	}
	edgeKeys := make([]edgeKey, 0, len(s.edgeNumericConstraints))
	for key := range s.edgeNumericConstraints {
		edgeKeys = append(edgeKeys, key)
	}
	slices.SortFunc(edgeKeys, func(a, b edgeKey) int {
		if a.from != b.from {
			return int(a.from) - int(b.from)
		}
		return int(a.to) - int(b.to)
	})
	for _, key := range edgeKeys {
		edgeConstraints := s.edgeNumericConstraints[key]
		if len(edgeConstraints) == 0 {
			continue
		}
		fromState := s.numericAt[key.from]
		edgeState := fromState.Clone()
		if edgeState == nil {
			edgeState = numeric.NewState()
		}
		// Resolve constraint paths at edge source point
		resolver := s.resolverAt(key.from)
		for _, nc := range edgeConstraints {
			edgeState.ApplyConstraintWithResolver(nc, resolver)
		}
		if !edgeState.CheckSatisfiability() {
			if s.unsatEdges == nil {
				s.unsatEdges = make(map[edgeKey]bool)
			}
			s.unsatEdges[key] = true
		}
	}
}

// runNumericWorklist computes the numeric component as a value-less fixpoint over
// all RPO points and finalizes unsatisfiable edges. It backs the condition view,
// which performs no assignment/phi value transfer; the main solve path computes
// the same numeric component inline in its unified worklist instead.
func (s *Solution) runNumericWorklist() {
	if s.inputs == nil || s.inputs.Graph == nil || len(s.edgeNumericConstraints) == 0 {
		return
	}
	c := s.inputs.Graph

	ws := newNumericWorklistState()
	s.numericAt = make(map[cfg.Point]*numeric.State)

	worklist := c.RPO()
	inQueue := make(map[cfg.Point]bool, len(worklist))
	for _, p := range worklist {
		inQueue[p] = true
	}

	for len(worklist) > 0 {
		p := worklist[0]
		worklist = worklist[1:]
		inQueue[p] = false

		if s.numericTransferAt(p, ws) {
			for _, succ := range graphSuccessors(c, p) {
				if !inQueue[succ] {
					worklist = append(worklist, succ)
					inQueue[succ] = true
				}
			}
		}
	}

	s.finalizeUnsatEdges()
}

// hasNumericLengthEffects reports whether any point carries a table-mutator
// length effect. The numeric component must run when length facts can be seeded
// even if no comparison produced an edge numeric constraint, so a bare
// table.insert sequence still proves an in-range index read.
func (s *Solution) hasNumericLengthEffects() bool {
	if s == nil || s.inputs == nil {
		return false
	}
	for _, tm := range s.inputs.TableMutatorAssignments {
		if tm.LengthDelta > 0 && tm.KeySymbol == 0 && tm.KeyType == nil && tm.Target.Symbol != 0 {
			return true
		}
	}
	return false
}

// applyNumericLengthEffects applies a point's value-domain length effects to the
// numeric state: a table.insert-like mutator with a constant LengthDelta and a
// non-nil inserted value raises its sequence target's length lower bound by the
// delta over the pre-state; any other length-affecting mutation on a sequence
// (table.remove, arr[k] = nil, an unknown mutator) degrades the lower bound,
// since the post-state length is no longer provably at the prior floor. The
// effect feeds the index-read presence proof through s.numericAt.
func (s *Solution) applyNumericLengthEffects(p cfg.Point, state *numeric.State) *numeric.State {
	if s == nil || s.pkResolver == nil {
		return state
	}
	raises := s.tableInsertLengthRaises(p)
	kills := s.lengthDegradingTargets(p)
	if len(raises) == 0 && len(kills) == 0 {
		return state
	}
	if state == nil {
		state = numeric.NewState()
	} else {
		state = state.Clone()
	}
	for key := range kills {
		if _, ok := raises[key]; ok {
			continue
		}
		state.DropLenBound(key)
	}
	for key, delta := range raises {
		lower := int64(0)
		if curLower, _, ok := state.LenBoundsFor(key); ok && curLower > 0 {
			lower = curLower
		}
		state.ApplyLenGeConst(key, lower+delta)
	}
	return state
}

// tableInsertLengthRaises maps each direct-sequence table.insert target at p to
// the constant length increase it guarantees, when the inserted value is non-nil
// (a nil insert does not grow a Lua sequence's border).
func (s *Solution) tableInsertLengthRaises(p cfg.Point) map[constraint.PathKey]int64 {
	var raises map[constraint.PathKey]int64
	for _, tm := range s.tableMutatorAssignmentsAt(p) {
		if tm.LengthDelta <= 0 || tm.KeySymbol != 0 || tm.KeyType != nil {
			continue
		}
		if tm.Target.Symbol == 0 {
			continue
		}
		// The non-nil check reads the STATIC inserted value type only. Reading the
		// flow-resolved value here would re-enter the value domain during the
		// numeric transfer (which runs first at p, before the value transfer),
		// observing premature state. A static nil/optional insert is conservatively
		// treated as no length growth.
		value := tm.ValueType
		if value != nil && (value.Kind() == kind.Nil || unwrap.IsOptionalLike(value)) {
			continue
		}
		key := s.pkResolver.KeyAt(p, tm.Target)
		if key == "" {
			continue
		}
		if raises == nil {
			raises = make(map[constraint.PathKey]int64, 1)
		}
		if delta, ok := raises[key]; !ok || tm.LengthDelta > delta {
			raises[key] = tm.LengthDelta
		}
	}
	return raises
}

// lengthDegradingTargets collects sequence paths whose length lower bound must be
// dropped at p because a mutation may shrink or arbitrarily reshape them: a
// direct-index write that may assign nil, or an unknown table-mutator without a
// positive length delta.
func (s *Solution) lengthDegradingTargets(p cfg.Point) map[constraint.PathKey]bool {
	var kills map[constraint.PathKey]bool
	add := func(path constraint.Path) {
		if path.Symbol == 0 {
			return
		}
		key := s.pkResolver.KeyAt(p, path)
		if key == "" {
			return
		}
		if kills == nil {
			kills = make(map[constraint.PathKey]bool, 1)
		}
		kills[key] = true
	}
	for _, tm := range s.tableMutatorAssignmentsAt(p) {
		if tm.LengthDelta > 0 && tm.KeySymbol == 0 && tm.KeyType == nil {
			continue
		}
		add(constraint.Path{Root: tm.Target.Root, Symbol: tm.Target.Symbol, Segments: tm.Target.Segments})
	}
	for _, mm := range s.mapMutatorAssignmentsAt(p) {
		// A direct integer-index write can leave a hole (arr[k] = nil) or extend
		// past the tracked floor; either way the prior length lower bound no longer
		// holds. Conservatively drop it.
		add(constraint.Path{Root: mm.Target.Root, Symbol: mm.Target.Symbol, Segments: mm.Target.Segments})
	}
	return kills
}

func numericStateWidenedToTop(oldState, rawState, widened *numeric.State) bool {
	if oldState == nil || oldState.IsTop() {
		return false
	}
	if widened != nil && !widened.IsTop() {
		return false
	}
	return !rawState.Equals(oldState)
}

// computeNumericStateAt computes the numeric state at a CFG point by joining predecessors.
//
// For each predecessor, this method:
//  1. Gets the predecessor's numeric state
//  2. Clones and applies edge constraints (narrowing bounds/orderings)
//  3. Rekeys bounds for phi nodes (mapping old versions to new)
//  4. Checks if the resulting state is still satisfiable
//
// Multiple predecessor states are joined (intersection of facts) to produce
// the final state at p. States that become "top" (no constraints) are filtered
// out to avoid losing precision in the join.
//
// Returns nil if no predecessor has a non-top state (point has no numeric facts).
func (s *Solution) computeNumericStateAt(c cfg.Graph, p cfg.Point, state map[cfg.Point]*numeric.State) *numeric.State {
	preds := graphPredecessors(c, p)
	if len(preds) == 0 {
		return nil
	}

	var result *numeric.State
	for _, pred := range preds {
		predState := state[pred]
		edgeConstraints := s.edgeNumericConstraints[edgeKey{from: pred, to: p}]

		if predState == nil && len(edgeConstraints) == 0 {
			continue
		}

		edgeState := predState.Clone()
		if edgeState == nil {
			edgeState = numeric.NewState()
		}

		if len(edgeConstraints) > 0 {
			// Resolve constraint paths at edge source point (pred)
			resolver := s.resolverAt(pred)
			for _, nc := range edgeConstraints {
				edgeState.ApplyConstraintWithResolver(nc, resolver)
			}
		}

		// Rekey bounds for phi nodes at join point p
		edgeState = s.rekeyForPhis(edgeState, pred, p)

		if edgeState.IsTop() {
			continue
		}
		if result == nil {
			result = edgeState
			continue
		}
		result = numeric.Join(result, edgeState)
	}

	return result
}

// resolverAt creates a path resolver for a specific CFG point.
//
// The resolver converts constraint paths to their canonical versioned keys
// at the given point. This is needed because numeric constraints reference
// paths, but the state tracks bounds by versioned keys. The resolver handles
// SSA version lookup for each path.
//
// Returns nil if no path key resolver is available.
func (s *Solution) resolverAt(p cfg.Point) func(path constraint.Path) constraint.PathKey {
	if s.pkResolver == nil {
		return nil
	}
	return func(path constraint.Path) constraint.PathKey {
		return s.pkResolver.KeyAt(p, path)
	}
}

// rekeyForPhis renames keys in the state for phi nodes at point p.
//
// At phi nodes, SSA versions from different predecessors merge into a new
// version. Numeric bounds tracked for the predecessor version must be renamed
// to the phi target version to remain useful after the merge.
//
// For example, if phi at point 5 merges x@1 (from pred 3) and x@2 (from pred 4)
// into x@5, bounds on x@1 arriving from predecessor 3 must be rekeyed to x@5.
//
// The method builds a remap table from operand versions to target versions,
// then applies it to create a new state with renamed keys. The original state
// is not modified.
func (s *Solution) rekeyForPhis(state *numeric.State, pred, p cfg.Point) *numeric.State {
	if state == nil || s.inputs == nil || s.pkResolver == nil {
		return state
	}

	var keyRemap map[constraint.PathKey]constraint.PathKey
	for _, phi := range s.inputs.Graph.PhiNodes() {
		if phi.Point != p {
			continue
		}
		newKey := s.pkResolver.KeyAtVersion(phi.Target.Symbol, phi.Target.ID, nil)
		if newKey == "" {
			continue
		}
		for _, op := range phi.Operands {
			if op.From != pred {
				continue
			}
			oldKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
			if oldKey != "" && newKey != "" && oldKey != newKey {
				if keyRemap == nil {
					keyRemap = make(map[constraint.PathKey]constraint.PathKey)
				}
				keyRemap[oldKey] = newKey
			}
		}
	}

	if len(keyRemap) == 0 {
		return state
	}

	// Create new state with rekeyed bounds
	return state.Rekey(keyRemap)
}

// pointIsWideningSite reports whether p is a feedback-vertex-set point at which
// widening must be applied to ensure termination on infinite-height ascending
// chains. It is shared by the numeric worklist and the value-domain phi merge.
// Loop headers (marked by CFG extraction via Node.LoopPreheaderSet) cover every
// structured-loop cycle. Non-loop SCC headers are not currently handled — if a
// future fixture exposes a non-reducible CFG cycle (Lua goto) that diverges,
// extend this to the full feedback vertex set (propagate.computeFeedbackVertexSet).
// The per-fixture deadline (commit 930068c9) catches such divergences cleanly.
func pointIsWideningSite(g cfg.Graph, p cfg.Point) bool {
	n := g.Node(p)
	if n == nil {
		return false
	}
	return n.LoopPreheaderSet
}
