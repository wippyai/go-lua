package equation

import (
	"bytes"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

// Reconvergence is the evaluator's control-flow join.  Direct visibility --
// `required guards subset of active guards` -- answers what one path partition
// currently holds.  It cannot answer what an unguarded point holds when the
// latest publication for a term lives on a branch edge, because no single edge
// row is the value there.  The value at such a point is the lattice join of the
// current value on every incoming edge of exactly one certified decision,
// including the edge that publishes nothing and therefore carries the incoming
// state unchanged.
//
// The guard algebra is owned here: which rows are alternatives of each other,
// which decision separates them, and whether their coverage is complete.  The
// value domain stays with the fact owner, which supplies Current and Join.  The
// two halves are deliberately separate so that this file cannot decide a type
// and the fact owner cannot decide a control-flow question.
type Reconvergence struct {
	// Current selects the fact a single fully-decided guard cube holds.  It is
	// the same selection an ordinary read performs, applied to the rows that
	// survive one cube rather than to the whole partition.  Reporting false
	// means the cube publishes nothing.
	Current func(candidates []Fact) (Fact, bool)
	// Join is the value lattice.  Reporting false withholds the whole join.
	Join func(left, right []byte) ([]byte, bool)
	// Support names further fact families whose rows Current needs in order to
	// decide one cube's value.  They are collected and guard-filtered exactly
	// like the primary family, so Current observes only the support rows the
	// cube itself can observe and never a row belonging to the other edge.
	// They decide no control flow: the residual guards a join is peeled from
	// still come from the row Current returns.
	Support []string
}

// reconvergenceBudget caps the guard cubes one read may expand.  Decisions are
// finite and frozen by the artifact, so the recursion always terminates without
// it; the cap keeps a pathological nest of live decisions over one term from
// spending exponential time inside a single partition read.  Exceeding it
// withholds the result rather than returning a partially explored join.
const reconvergenceBudget = 96

// branchDecision is one certified CFG branch.  Its two edges are mutually
// exclusive and jointly exhaustive, which is what makes them joinable
// alternatives.  Guards outside this family name no decision and are therefore
// never treated as alternatives of anything.
type branchDecision struct {
	body BodyID
	name string
}

func decisionOf(guard Guard) (branchDecision, bool, bool) {
	parsed, ok := factkey.ParseBranchGuard(string(guard.Encoding))
	if !ok {
		return branchDecision{}, false, false
	}
	return branchDecision{body: guard.Body, name: parsed.Name}, parsed.TrueEdged(), true
}

func (d branchDecision) edge(taken bool) Guard {
	edge := factkey.FalseEdge
	if taken {
		edge = factkey.TrueEdge
	}
	return Guard{Body: d.body, Encoding: []byte(factkey.BranchGuard{Name: d.name, Edge: edge}.Encoding())}
}

// withdrawContradictoryBranchProofs removes both edge proofs of one decision
// when a body proved each of them under the same guard cube.  Only a recurrent
// evaluation can reach that state: one trip of a loop decides the condition and
// the next decides it the other way, and each trip publishes the proof its own
// edge carries under a key of its own, so no per-key merge can see the pair.
//
// Keeping both would resolve both edges of the decision at once and make every
// arm-local publication visible together -- the peeled reading that a fixed
// point over the recurrence exists to replace.  A decision two trips disagree
// about is simply not decided, so neither proof survives.  Proofs published
// under different cubes describe different program regions and are untouched:
// a decision that depends on an enclosing branch legitimately holds one way on
// one edge and the other way on the other.
func withdrawContradictoryBranchProofs(facts []Fact) []Fact {
	type proofKey struct {
		decision branchDecision
		cube     string
	}
	edges := make(map[proofKey]bool, len(facts))
	var contradicted map[proofKey]bool
	classify := func(fact Fact) (proofKey, bool) {
		guard, ok := branchProofGuard(fact.Key)
		if !ok || string(fact.Value) != "proven" {
			return proofKey{}, false
		}
		decision, _, peelable := decisionOf(guard)
		if !peelable {
			return proofKey{}, false
		}
		return proofKey{decision: decision, cube: guardsKey(fact.Guards)}, true
	}
	for _, fact := range facts {
		key, ok := classify(fact)
		if !ok {
			continue
		}
		guard, _ := branchProofGuard(fact.Key)
		_, edge, _ := decisionOf(guard)
		if prior, seen := edges[key]; seen && prior != edge {
			if contradicted == nil {
				contradicted = make(map[proofKey]bool, 1)
			}
			contradicted[key] = true
			continue
		}
		edges[key] = edge
	}
	if len(contradicted) == 0 {
		return facts
	}
	kept := facts[:0]
	for _, fact := range facts {
		if key, ok := classify(fact); ok && contradicted[key] {
			continue
		}
		kept = append(kept, fact)
	}
	return kept
}

// guardsConflict reports whether required and active fix opposite edges of the
// same decision.  Such rows describe disjoint program regions: they are not
// alternatives to join, they are simply not present together.  Guards for
// unrelated decisions never conflict, which is what keeps a parallel guard's
// publication out of another decision's join.
func guardsConflict(required, active []Guard) bool {
	for _, guard := range required {
		decision, edge, ok := decisionOf(guard)
		if !ok {
			continue
		}
		opposite := decision.edge(!edge)
		for _, candidate := range active {
			if candidate.Body == opposite.Body && bytes.Equal(candidate.Encoding, opposite.Encoding) {
				return true
			}
		}
	}
	return false
}

// guardsBeyond returns the guards a fact still requires that the active cube
// does not already fix, in canonical order.  An empty result is exactly direct
// visibility.
func guardsBeyond(required, active []Guard) []Guard {
	var out []Guard
	for _, guard := range required {
		if !guardsIncluded([]Guard{guard}, active) {
			out = append(out, guard)
		}
	}
	return out
}

// Reconverged resolves the current publication under prefix at this partition's
// point.  Inside a guard cube that already fixes every decision the latest row
// depends on, it is the ordinary read.  At a point where the latest row is
// edge-guarded, it peels one decision at a time and joins that decision's
// edges; an edge whose own state publishes nothing under prefix contributes its
// incoming value, so a one-armed write joins against the value before the
// branch rather than replacing it.
//
// The join fails closed.  A feasible edge with no current value, an unpeelable
// guard family, a lattice that cannot merge two payloads, and an exhausted
// expansion budget all withhold the result instead of narrowing it.  Missing
// coverage is never read as lattice bottom.
func (p Partition) Reconverged(prefix string, lattice Reconvergence) (Fact, bool) {
	if lattice.Current == nil || lattice.Join == nil || prefix == "" {
		return Fact{}, false
	}
	var candidates []Fact
	for _, fact := range p.closure.Values {
		if reconvergenceFamily(fact.Key, prefix, lattice.Support) {
			candidates = append(candidates, fact)
		}
	}
	if len(candidates) == 0 {
		return Fact{}, false
	}
	budget := reconvergenceBudget
	active := resolvedBranchGuards(p.closure.Values, p.guards)
	active = activePastRecurrence(p.closure.Values, candidates, active, lattice)
	return reconverge(candidates, active, lattice, &budget)
}

// activePastRecurrence removes the arm through which this cube left a loop, when
// the publication the cube currently selects is older than the loop's own
// decision.  Such a publication is the value the loop received: the arm that
// stays inside the loop republishes on every trip, so a point the leaving arm
// alone reaches stands after all of them and holds what they carried.  Removing
// the arm leaves the decision undecided for this read, so the ordinary peel
// joins both alternatives and the point reports the recurrence's fixed point
// rather than its seed.
//
// A cube that already selects the decision's own publication, or a later one,
// keeps its arm.  That row was derived from the value entering the decision --
// which is the join over every trip -- so peeling it again would only widen a
// result the loop has already accounted for.
//
// Which decision leaves which loop is the deciding body's own publication.  A
// read that finds no such row treats every decision as ordinary, which is the
// acyclic reading.
func activePastRecurrence(evidence, candidates []Fact, active []Guard, lattice Reconvergence) []Guard {
	var stale map[string]bool
	for _, guard := range active {
		decision, edge, ok := decisionOf(guard)
		if !ok || !recurrenceExitEdge(evidence, decision.name, edge) {
			continue
		}
		if !recurrenceSeeded(candidates, active, lattice, decision.name) {
			continue
		}
		if stale == nil {
			stale = make(map[string]bool, 1)
		}
		stale[decision.name] = true
	}
	if len(stale) == 0 {
		return active
	}
	kept := make([]Guard, 0, len(active))
	for _, guard := range active {
		if decision, _, ok := decisionOf(guard); ok && stale[decision.name] {
			continue
		}
		kept = append(kept, guard)
	}
	return kept
}

// recurrenceExitEdge reports whether the deciding body published this edge of
// this decision as the arm that leaves a loop.
func recurrenceExitEdge(evidence []Fact, name string, edge bool) bool {
	key := factkey.RecurrenceExitPrefix + name
	stated := factkey.FalseEdge
	if edge {
		stated = factkey.TrueEdge
	}
	for _, fact := range evidence {
		if fact.Key == key && string(fact.Value) == stated {
			return true
		}
	}
	return false
}

// recurrenceSeeded reports whether the row this cube selects was published
// before the named decision.  Publication order is the artifact's own occurrence
// order, the same order by which every read already selects its latest row.
func recurrenceSeeded(candidates []Fact, active []Guard, lattice Reconvergence, name string) bool {
	compatible := make([]Fact, 0, len(candidates))
	for _, fact := range candidates {
		if !guardsConflict(fact.Guards, active) {
			compatible = append(compatible, fact)
		}
	}
	chosen, found := lattice.Current(compatible)
	return found && factOccurrence(chosen.Key) < name
}

// factOccurrence is the occurrence a key ends with, which names the operation
// that published the fact.
func factOccurrence(key string) string {
	if cut := strings.LastIndexByte(key, '/'); cut >= 0 {
		return key[cut+1:]
	}
	return key
}

// WithoutOwnDecision is this partition with the arm publications of one
// decision removed. A decision's arms are its results, not its inputs: the
// value it tests is the one that reached it, and no execution routes an arm's
// own publication back into the test that selected that arm.
//
// A straight-line evaluation never holds those rows when the decision runs, so
// this restriction is that evaluation's own state. A recurrent evaluation does
// hold them, published by the previous trip, and reading them would let a
// decision narrow what it had already narrowed -- a peeled reading of its own
// output that loses every arm the earlier narrowing dropped. Publications
// guarded by other decisions, and the unguarded value this decision consumes,
// are untouched: only the rows this decision itself produced are withheld.
func (p Partition) WithoutOwnDecision(name string) Partition {
	if name == "" {
		return p
	}
	owned := func(guards []Guard) bool {
		for _, guard := range guards {
			if decision, _, ok := decisionOf(guard); ok && decision.name == name {
				return true
			}
		}
		return false
	}
	filtered := OutputClosure{
		Values:           copyFacts(p.closure.Values, func(fact Fact) bool { return !owned(fact.Guards) }),
		Outcomes:         copyFacts(p.closure.Outcomes, func(fact Fact) bool { return !owned(fact.Guards) }),
		Diagnostics:      p.closure.Diagnostics,
		AllocationRekeys: p.closure.AllocationRekeys,
	}
	return Partition{closure: filtered, guards: cloneGuards(p.guards)}
}

// Edge is this partition restricted to one alternative of a single decision.
// Its Partition answers every read as that edge answers it, so a consumer whose
// conclusion is not a lattice payload -- a callee identity, a whole evaluation --
// can be resolved per edge instead of being forced through a value join it has
// no lattice for.  Guard is the alternative itself, which is what a publication
// derived inside the edge must carry in order to reconverge later.
type Edge struct {
	Guard     Guard
	Partition Partition
}

// DecisionEdges restricts this partition to both alternatives of exactly one
// decision that the current publication under prefix still depends on.  It is
// the same peel Reconverged performs, exposed for the readers whose result is
// not a value: those consumers resolve themselves inside each edge and publish
// their own guarded conclusions, which the ordinary value lane then joins at the
// point both edges reach.
//
// Reporting false means no split is owed: the family publishes nothing here, its
// current row is already fully decided by this cube, or the guard it still needs
// names no decision.  Callers therefore keep their existing single-partition
// behaviour unless the point genuinely holds two alternatives.
func (p Partition) DecisionEdges(prefix string) ([2]Edge, bool) {
	if prefix == "" {
		return [2]Edge{}, false
	}
	active := resolvedBranchGuards(p.closure.Values, p.guards)
	var chosen Fact
	found := false
	for _, fact := range p.closure.Values {
		if !strings.HasPrefix(fact.Key, prefix) || guardsConflict(fact.Guards, active) {
			continue
		}
		if !found || fact.Key > chosen.Key {
			chosen, found = fact, true
		}
	}
	if !found {
		return [2]Edge{}, false
	}
	extra := guardsBeyond(chosen.Guards, active)
	if len(extra) == 0 {
		return [2]Edge{}, false
	}
	decision, _, peelable := decisionOf(extra[0])
	if !peelable {
		return [2]Edge{}, false
	}
	var out [2]Edge
	for index, taken := range [2]bool{true, false} {
		guard := decision.edge(taken)
		out[index] = Edge{Guard: guard, Partition: Partition{closure: p.closure, guards: canonicalGuards(append(cloneGuards(p.guards), guard))}}
	}
	return out, true
}

// reconvergenceFamily reports whether a key belongs to the read's primary
// family or to one of its support families.
func reconvergenceFamily(key, prefix string, support []string) bool {
	if strings.HasPrefix(key, prefix) {
		return true
	}
	for _, family := range support {
		if family != "" && strings.HasPrefix(key, family) {
			return true
		}
	}
	return false
}

func reconverge(candidates []Fact, active []Guard, lattice Reconvergence, budget *int) (Fact, bool) {
	if *budget <= 0 {
		return Fact{}, false
	}
	*budget--
	compatible := make([]Fact, 0, len(candidates))
	for _, fact := range candidates {
		if !guardsConflict(fact.Guards, active) {
			compatible = append(compatible, fact)
		}
	}
	chosen, found := lattice.Current(compatible)
	if !found {
		return Fact{}, false
	}
	// The chosen row is the latest one compatible with this cube.  When it needs
	// no further decision it is already the value on every completion of the
	// cube, so the expansion stops here and no join is performed.
	extra := guardsBeyond(chosen.Guards, active)
	if len(extra) == 0 {
		return cloneFact(chosen), true
	}
	decision, _, peelable := decisionOf(extra[0])
	if !peelable {
		return Fact{}, false
	}
	var joined []byte
	merged := false
	for _, taken := range [2]bool{true, false} {
		edge := canonicalGuards(append(cloneGuards(active), decision.edge(taken)))
		side, ok := reconverge(candidates, edge, lattice, budget)
		if !ok || len(side.Value) == 0 {
			return Fact{}, false
		}
		if !merged {
			joined, merged = side.Value, true
			continue
		}
		value, ok := lattice.Join(joined, side.Value)
		if !ok || len(value) == 0 {
			return Fact{}, false
		}
		joined = value
	}
	// The joined value belongs to the cube that survives the decision, not to
	// either edge, so it is stamped with the residual guards.  Its key is the
	// latest contributing publication: consumers order publications by key, and
	// a join must not appear older than a row it already subsumes.
	return Fact{Key: chosen.Key, Value: joined, Guards: cloneGuards(active)}, true
}
