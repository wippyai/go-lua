// Package propagate computes type constraints at CFG points via forward propagation.
//
// Given edge conditions (constraints on control flow edges from conditionals),
// propagation computes the combined condition at each program point by merging
// conditions from all incoming paths. The result enables flow-sensitive type
// narrowing: at each point, the condition describes what type facts must hold.
//
// The algorithm uses a worklist with RPO ordering for efficient convergence.
// Conditions are represented in DNF (disjunctive normal form) and combined
// using logical AND (through edges) and OR (at join points).
package propagate

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// Graph provides CFG traversal operations for propagation.
//
// This interface abstracts the control flow graph, allowing propagation to
// work with any graph implementation that provides the required operations.
// Typically, this is a *cfg.CFG or cfg.VersionedGraph.
type Graph interface {
	// Entry returns the entry point of the CFG (function start).
	Entry() cfg.Point
	// RPO returns CFG points in reverse postorder (optimal iteration order).
	RPO() []cfg.Point
	// Node returns the CFG node at point p, or nil if none.
	Node(p cfg.Point) *cfg.Node
	// Predecessors returns all CFG points that have edges to p.
	Predecessors(p cfg.Point) []cfg.Point
	// Successors returns all CFG points that p has edges to.
	Successors(p cfg.Point) []cfg.Point
}

type predecessorReadOnly interface {
	PredecessorsReadOnly(p cfg.Point) []cfg.Point
}

type successorReadOnly interface {
	SuccessorsReadOnly(p cfg.Point) []cfg.Point
}

func graphPredecessors(g Graph, p cfg.Point) []cfg.Point {
	if ro, ok := g.(predecessorReadOnly); ok {
		return ro.PredecessorsReadOnly(p)
	}
	return g.Predecessors(p)
}

func graphSuccessors(g Graph, p cfg.Point) []cfg.Point {
	if ro, ok := g.(successorReadOnly); ok {
		return ro.SuccessorsReadOnly(p)
	}
	return g.Successors(p)
}

// EdgeConditions maps CFG edges to their guard conditions.
//
// An edge condition describes what must be true when control flows along
// that edge. For example, an if-then edge has a Truthy condition for the
// test expression, while the else edge has a Falsy condition.
type EdgeConditions map[EdgeKey]constraint.Condition

// EdgeKey identifies a CFG edge by its source and target points.
//
// A CFG edge represents control flow from one program point to another.
// Edges have conditions when they come from conditional branches (if, while, etc.).
type EdgeKey struct {
	From cfg.Point
	To   cfg.Point
}

// Assignment describes a variable assignment at a CFG point.
//
// Used for constraint killing: when a variable is reassigned, constraints
// on that variable's previous value become invalid and must be removed from
// the propagated condition.
//
// For example, after "x = 5", a prior constraint "type(x) == string" no longer
// applies because x now has a different value.
type Assignment struct {
	// Point is the CFG point where the assignment occurs.
	Point cfg.Point
	// TargetSym is the symbol being assigned (provides unique identity).
	TargetSym cfg.SymbolID
	// TargetSegs specifies field path for nested assignments (x.foo.bar = ...).
	// Empty for simple variable assignments.
	TargetSegs []constraint.Segment
}

// Inputs provides all data needed for constraint propagation.
//
// The propagation algorithm uses these inputs to compute which conditions
// hold at each CFG point, accounting for edge conditions, dead code, and
// constraint killing from assignments.
type Inputs struct {
	// Graph provides CFG traversal.
	Graph Graph
	// EdgeConditions maps edges to their guard conditions.
	EdgeConditions EdgeConditions
	// DeadPoints marks CFG points that are unreachable (e.g., after error()).
	// Propagation treats these as having False condition.
	DeadPoints map[cfg.Point]bool
	// Assignments lists all variable assignments for constraint killing.
	Assignments []Assignment
}

// Result holds the computed conditions at each CFG point.
//
// After propagation, PointConditions maps each reachable CFG point to the
// disjunction of all path conditions that reach that point. Unreachable
// points have False conditions or are absent from the map.
type Result struct {
	// PointConditions maps CFG points to the conditions that hold there.
	// A point's condition is the OR of (predecessor condition AND edge condition)
	// for all incoming edges.
	PointConditions map[cfg.Point]constraint.Condition
}

// loopHeaderWideningThreshold is the K used by the FVS widening policy:
// widening fires at an FVS point only after K state-changing visits.
//
// Per DOMAIN_DESIGN.md §7.2, K=3 lets the precise transfer iterate twice on a
// fresh loop body before widening engages on the third revisit.
const loopHeaderWideningThreshold = 3

// Propagate computes type constraints at each CFG point via forward dataflow.
//
// This is the main entry point for constraint propagation. The algorithm:
//
//  1. Initialize entry point with True condition.
//  2. Process points in worklist order (initially RPO for efficiency).
//  3. For each point, compute new condition from predecessors.
//  4. Join with the existing point condition.
//  5. If the point is in the feedback vertex set (FVS) and has already
//     produced strict state changes at least loopHeaderWideningThreshold
//     times, apply constraint.Domain.Widen to the result.
//  6. If the post-update condition differs, store it and enqueue successors.
//  7. Repeat until no changes (fixed point).
//
// FVS = { p : p.LoopPreheaderSet } ∪ { headers of non-loop SCCs }, where the
// "header" of an SCC is the point with the smallest RPO index in that SCC.
// This is Cousot's classical feedback-vertex-set widening policy: every CFG
// cycle contains at least one FVS point, so widening at FVS suffices to
// terminate every chain, and acyclic high-fan-in joins are NOT in FVS (so
// they keep full precision; see §10.5).
//
// Condition computation at each point (per computeConditionAtPoint):
//   - For each predecessor with non-False condition: AND predecessor
//     condition with the edge condition.
//   - Apply loop preheader reinforcement for loop headers.
//   - OR the incoming conditions together.
//   - Kill literals invalidated by assignments at this point (uses
//     constraint.SemanticAffectedPaths, fixing prior subpath-write
//     unsoundness — DOMAIN_DESIGN.md §8.2).
//
// Determinism: under shuffled RPO the algorithm visits the same set of
// points, applies the same widening policy at the same set of FVS points,
// and arrives at the same fixpoint set. Visit count is bounded by the
// fixpoint height; with FVS widening the height is finite.
func Propagate(inputs *Inputs) *Result {
	if inputs == nil || inputs.Graph == nil {
		return &Result{PointConditions: make(map[cfg.Point]constraint.Condition)}
	}

	g := inputs.Graph
	rpo := g.RPO()
	worklist := append([]cfg.Point(nil), rpo...)
	pointConditions := make(map[cfg.Point]constraint.Condition, len(worklist))
	pointConditions[g.Entry()] = constraint.TrueCondition()

	inQueue := make(map[cfg.Point]bool, len(worklist))
	for _, p := range worklist {
		inQueue[p] = true
	}

	// Cache preheader conditions for monotonic convergence.
	preheaderConds := make(map[cfg.Point]constraint.Condition)

	fvs := computeFeedbackVertexSet(g, rpo)
	visitCount := make(map[cfg.Point]int, len(fvs))

	for len(worklist) > 0 {
		p := worklist[0]
		worklist = worklist[1:]
		inQueue[p] = false

		incoming := computeConditionAtPoint(inputs, pointConditions, preheaderConds, p)
		oldCond := pointConditions[p]

		// candidate = Join(oldCond, incoming). When oldCond is absent (zero
		// value, IsFalse), Or returns incoming.
		candidate := constraint.Or(oldCond, incoming)

		var next constraint.Condition
		if fvs[p] && visitCount[p] >= loopHeaderWideningThreshold {
			next = constraint.Domain.Widen(oldCond, candidate)
		} else {
			next = candidate
		}

		if !constraint.Domain.Equal(next, oldCond) {
			pointConditions[p] = next
			visitCount[p]++
			for _, succ := range graphSuccessors(g, p) {
				if !inQueue[succ] {
					worklist = append(worklist, succ)
					inQueue[succ] = true
				}
			}
		}
	}

	return &Result{PointConditions: pointConditions}
}

// computeFeedbackVertexSet identifies CFG points where widening must fire.
//
// FVS = loop headers (LoopPreheaderSet) ∪ headers of non-loop SCCs. Every
// cycle in the CFG contains at least one FVS point, so widening at FVS is
// sufficient to terminate every ascending chain. Non-cyclic high-fan-in
// merge points are NOT in FVS — they keep precision (DOMAIN_DESIGN.md §7.4).
//
// SCC detection runs Tarjan's algorithm over the CFG's successor relation.
// Trivial SCCs (single point, no self-loop) are skipped. Loop-header SCCs
// (already covered by LoopPreheaderSet) are absorbed by the union and do not
// require a separate entry. Lua's structured control flow typically yields
// zero non-loop SCCs.
func computeFeedbackVertexSet(g Graph, rpo []cfg.Point) map[cfg.Point]bool {
	fvs := make(map[cfg.Point]bool)
	for _, p := range rpo {
		n := g.Node(p)
		if n != nil && n.LoopPreheaderSet {
			fvs[p] = true
		}
	}

	// RPO index lookup, so we can pick the smallest-RPO point per SCC.
	rpoIndex := make(map[cfg.Point]int, len(rpo))
	for i, p := range rpo {
		rpoIndex[p] = i
	}

	tarjan := newTarjanState(g, rpo, rpoIndex)
	for _, p := range rpo {
		if _, seen := tarjan.indexOf[p]; !seen {
			tarjan.strongConnect(p)
		}
	}

	for _, scc := range tarjan.sccs {
		if len(scc) < 2 {
			// Singleton SCC: only counts as a cycle if it has a self-edge.
			only := scc[0]
			selfLoop := false
			for _, succ := range graphSuccessors(g, only) {
				if succ == only {
					selfLoop = true
					break
				}
			}
			if !selfLoop {
				continue
			}
		}
		// Pick the SCC member with the smallest RPO index as the header.
		header := scc[0]
		headerIdx := rpoIndex[header]
		for _, q := range scc[1:] {
			if idx, ok := rpoIndex[q]; ok && idx < headerIdx {
				header = q
				headerIdx = idx
			}
		}
		fvs[header] = true
	}

	return fvs
}

// tarjanState implements Tarjan's SCC algorithm on the propagate.Graph
// interface. It is run once per Propagate call; allocations scale O(|V|+|E|).
type tarjanState struct {
	g        Graph
	rpoIndex map[cfg.Point]int
	indexOf  map[cfg.Point]int
	lowlink  map[cfg.Point]int
	onStack  map[cfg.Point]bool
	stack    []cfg.Point
	counter  int
	sccs     [][]cfg.Point
}

func newTarjanState(g Graph, rpo []cfg.Point, rpoIndex map[cfg.Point]int) *tarjanState {
	return &tarjanState{
		g:        g,
		rpoIndex: rpoIndex,
		indexOf:  make(map[cfg.Point]int, len(rpo)),
		lowlink:  make(map[cfg.Point]int, len(rpo)),
		onStack:  make(map[cfg.Point]bool, len(rpo)),
	}
}

func (s *tarjanState) strongConnect(v cfg.Point) {
	// Iterative implementation to avoid Go stack overflow on deep CFGs.
	type frame struct {
		v        cfg.Point
		succs    []cfg.Point
		succIdx  int
		childRet cfg.Point
		hasRet   bool
	}
	var frames []frame
	push := func(v cfg.Point) {
		s.indexOf[v] = s.counter
		s.lowlink[v] = s.counter
		s.counter++
		s.stack = append(s.stack, v)
		s.onStack[v] = true
		frames = append(frames, frame{v: v, succs: graphSuccessors(s.g, v)})
	}
	push(v)

	for len(frames) > 0 {
		top := &frames[len(frames)-1]

		if top.hasRet {
			// Returning from a recursive strongConnect on a successor.
			child := top.childRet
			if s.lowlink[child] < s.lowlink[top.v] {
				s.lowlink[top.v] = s.lowlink[child]
			}
			top.hasRet = false
		}

		if top.succIdx < len(top.succs) {
			w := top.succs[top.succIdx]
			top.succIdx++
			if _, seen := s.indexOf[w]; !seen {
				// Recurse via stack.
				push(w)
				continue
			}
			if s.onStack[w] {
				if s.indexOf[w] < s.lowlink[top.v] {
					s.lowlink[top.v] = s.indexOf[w]
				}
			}
			continue
		}

		// All successors processed; if v is an SCC root, pop one SCC.
		if s.lowlink[top.v] == s.indexOf[top.v] {
			var scc []cfg.Point
			for {
				if len(s.stack) == 0 {
					break
				}
				last := s.stack[len(s.stack)-1]
				s.stack = s.stack[:len(s.stack)-1]
				s.onStack[last] = false
				scc = append(scc, last)
				if last == top.v {
					break
				}
			}
			s.sccs = append(s.sccs, scc)
		}

		// Pop frame and propagate lowlink to parent.
		v := top.v
		frames = frames[:len(frames)-1]
		if len(frames) > 0 {
			parent := &frames[len(frames)-1]
			parent.hasRet = true
			parent.childRet = v
		}
	}
}

// computeConditionAtPoint computes the condition for a single CFG point.
//
// The condition is computed by combining all incoming paths:
//   - For each predecessor, AND its condition with the edge condition
//   - Apply loop preheader reinforcement if at a loop header
//   - OR all the incoming conditions together
//   - Kill constraints for variables reassigned at this point
//
// Special handling for loop headers: The preheader condition (before loop entry)
// is reinforced on backedge paths to preserve invariants. Loop-variant variables
// are filtered out to avoid unsound conclusions.
func computeConditionAtPoint(
	inputs *Inputs,
	pointConditions map[cfg.Point]constraint.Condition,
	preheaderConds map[cfg.Point]constraint.Condition,
	p cfg.Point,
) constraint.Condition {
	return newPointConditionComputer(inputs, pointConditions, preheaderConds, p).compute()
}

// FilterConditionSymbols removes constraints that reference specified symbols.
//
// This is used to filter out loop-variant constraints when reinforcing loop
// preheader conditions. If a variable is modified in the loop body, constraints
// about its preheader value are invalid on backedges.
//
// For example, given loop "for i = 1, 10 do ... end", constraints on i from
// the preheader (i == 1) don't hold after the first iteration.
//
// If any disjunct becomes empty after filtering, returns True (the constraint
// had no non-loop-variant component and should not restrict narrowing).
func FilterConditionSymbols(cond constraint.Condition, syms []cfg.SymbolID) constraint.Condition {
	if len(syms) == 0 || !cond.HasConstraints() {
		return cond
	}

	symSet := make(map[cfg.SymbolID]struct{}, len(syms))
	for _, sym := range syms {
		if sym != 0 {
			symSet[sym] = struct{}{}
		}
	}
	if len(symSet) == 0 {
		return cond
	}

	var newDisjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		if len(d) == 0 {
			return constraint.TrueCondition()
		}
		var kept []constraint.Constraint
		for _, c := range d {
			shouldKeep := true
			constraint.VisitPaths(c, func(p constraint.Path) bool {
				if p.Symbol == 0 {
					return false
				}
				if _, ok := symSet[p.Symbol]; ok {
					shouldKeep = false
					return true
				}
				return false
			})
			if shouldKeep {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			return constraint.TrueCondition()
		}
		newDisjuncts = append(newDisjuncts, kept)
	}

	if len(newDisjuncts) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromDisjuncts(newDisjuncts)
}

// KillRedefinedConditions removes constraints invalidated by assignments at p.
//
// Per DOMAIN_DESIGN.md §8.2, a literal L is killed by an assignment to path w
// iff w is a (non-strict) prefix of at least one path in
// constraint.SemanticAffectedPaths(L). The semantic visitor exposes every
// path L reads, including synthetic field/index sub-paths — fixing the prior
// unsoundness where a write like `x.kind = …` did not kill
// `FieldEquals{x, "kind", lit}`.
//
// The descendant direction in the design's "ancestor or descendant" wording
// is omitted intentionally: it over-kills (writing x.value would invalidate
// reads of x.kind). The ancestor-only check is sound — every path the
// literal reads is checked individually — and preserves precision for
// disjoint subpath writes. See PathAffectedByAssignment for details.
//
// If every literal in a disjunct is killed, that disjunct collapses to TRUE
// (no constraints, fully unrestricted), which propagates to the whole
// condition. If every literal across all disjuncts is killed, the result is
// TrueCondition — the assignment removed every refinement.
func KillRedefinedConditions(cond constraint.Condition, p cfg.Point, assignments []Assignment) constraint.Condition {
	if !cond.HasConstraints() {
		return cond
	}

	var assignedPaths []Assignment
	for _, assign := range assignments {
		if assign.Point == p && assign.TargetSym != 0 {
			assignedPaths = append(assignedPaths, assign)
		}
	}

	if len(assignedPaths) == 0 {
		return cond
	}

	var newDisjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		var kept []constraint.Constraint
		for _, c := range d {
			if literalKilledByAssignments(c, assignedPaths) {
				continue
			}
			kept = append(kept, c)
		}
		if len(kept) == 0 {
			return constraint.TrueCondition()
		}
		newDisjuncts = append(newDisjuncts, kept)
	}

	if len(newDisjuncts) == 0 {
		return constraint.TrueCondition()
	}

	return constraint.FromDisjuncts(newDisjuncts)
}

// literalKilledByAssignments returns true if any of the literal's semantic
// access paths is affected by any assignment at the current point.
func literalKilledByAssignments(c constraint.Constraint, assigns []Assignment) bool {
	paths := constraint.SemanticAffectedPaths(c)
	for _, cpath := range paths {
		if cpath.Symbol == 0 {
			continue
		}
		for _, ap := range assigns {
			if PathAffectedByAssignment(cpath, ap.TargetSym, ap.TargetSegs) {
				return true
			}
		}
	}
	return false
}

// PathAffectedByAssignment reports whether the constraint's access path cpath
// is invalidated by an assignment to (assignSym, assignSegs).
//
// Rule (precise + sound, refined from DOMAIN_DESIGN.md §8.2 to preserve
// precision): cpath is affected iff
//  1. cpath's symbol matches the assignment symbol; AND
//  2. assignSegs is a (non-strict) prefix of cpath.Segments — i.e., the
//     assignment writes AT or ABOVE the constraint's read path, so the read
//     is shadowed by the new value.
//
// The reverse direction (cpath.Segments is a strict prefix of assignSegs —
// the assignment writes to a deeper SUBPATH of cpath) intentionally does
// NOT kill: writing `x.value = …` does not change the value of `x.kind`,
// so a constraint reading `x.kind` is preserved. Combined with
// SemanticAffectedPaths exposing the constraint's full read paths
// (e.g., FieldEquals{x, "kind"} reads BOTH x and x.kind), the original
// unsoundness Codex flagged — `x.kind = …` not killing FieldEquals{x, kind} —
// is fixed: w=x.kind matches the literal's x.kind read path.
//
// Examples:
//   - Assigning to x affects x, x.foo, x.foo.bar  — assignSegs empty, prefix
//     of every cpath.
//   - Assigning to x.foo affects x.foo, x.foo.bar — assignSegs prefix of
//     cpath.
//   - Assigning to x.foo does NOT affect x.bar    — neither is a prefix of
//     the other.
//   - Assigning to x.foo does NOT affect x          — assignment to a deeper
//     subpath does not shadow the parent read.
func PathAffectedByAssignment(cpath constraint.Path, assignSym cfg.SymbolID, assignSegs []constraint.Segment) bool {
	if cpath.Symbol != assignSym {
		return false
	}
	return segmentsPrefix(assignSegs, cpath.Segments)
}

// segmentsPrefix reports whether prefix's segments match the first
// len(prefix) segments of full.
func segmentsPrefix(prefix, full []constraint.Segment) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i, seg := range prefix {
		fseg := full[i]
		if fseg.Kind != seg.Kind || fseg.Name != seg.Name || fseg.Index != seg.Index {
			return false
		}
	}
	return true
}
