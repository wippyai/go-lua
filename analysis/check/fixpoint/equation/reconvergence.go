package equation

import (
	"bytes"
	"strings"
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
}

// reconvergenceBudget caps the guard cubes one read may expand.  Decisions are
// finite and frozen by the artifact, so the recursion always terminates without
// it; the cap keeps a pathological nest of live decisions over one term from
// spending exponential time inside a single partition read.  Exceeding it
// withholds the result rather than returning a partially explored join.
const reconvergenceBudget = 96

const branchGuardPrefix = "front/branch/"

// branchDecision is one certified CFG branch.  Its two edges are mutually
// exclusive and jointly exhaustive, which is what makes them joinable
// alternatives.  Guards outside this family name no decision and are therefore
// never treated as alternatives of anything.
type branchDecision struct {
	body BodyID
	name string
}

func decisionOf(guard Guard) (branchDecision, bool, bool) {
	encoding := string(guard.Encoding)
	if !strings.HasPrefix(encoding, branchGuardPrefix) {
		return branchDecision{}, false, false
	}
	rest := encoding[len(branchGuardPrefix):]
	cut := strings.LastIndexByte(rest, '/')
	if cut <= 0 {
		return branchDecision{}, false, false
	}
	switch rest[cut+1:] {
	case "true":
		return branchDecision{body: guard.Body, name: rest[:cut]}, true, true
	case "false":
		return branchDecision{body: guard.Body, name: rest[:cut]}, false, true
	}
	return branchDecision{}, false, false
}

func (d branchDecision) edge(taken bool) Guard {
	suffix := "/false"
	if taken {
		suffix = "/true"
	}
	return Guard{Body: d.body, Encoding: []byte(branchGuardPrefix + d.name + suffix)}
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
		if strings.HasPrefix(fact.Key, prefix) {
			candidates = append(candidates, fact)
		}
	}
	if len(candidates) == 0 {
		return Fact{}, false
	}
	budget := reconvergenceBudget
	return reconverge(candidates, resolvedBranchGuards(p.closure.Values, p.guards), lattice, &budget)
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
