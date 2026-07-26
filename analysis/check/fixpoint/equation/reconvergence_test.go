package equation

import (
	"sort"
	"strings"
	"testing"
)

func edgeGuard(name, edge string) Guard {
	return Guard{Body: BodyID{1}, Encoding: []byte("front/branch/" + name + "/" + edge)}
}

// setLattice is a deliberately structural stand-in for a value domain: the join
// of two payloads is the sorted set of their members.  It makes every joined
// leaf observable, so a test can state exactly which guard cubes contributed.
func setLattice() Reconvergence {
	return Reconvergence{
		Current: func(candidates []Fact) (Fact, bool) {
			var latest Fact
			selected := false
			for _, candidate := range candidates {
				if !selected || candidate.Key > latest.Key {
					latest, selected = candidate, true
				}
			}
			return latest, selected
		},
		Join: func(left, right []byte) ([]byte, bool) {
			members := append(strings.Split(string(left), "|"), strings.Split(string(right), "|")...)
			sort.Strings(members)
			unique := members[:0]
			for _, member := range members {
				if len(unique) == 0 || unique[len(unique)-1] != member {
					unique = append(unique, member)
				}
			}
			return []byte(strings.Join(unique, "|")), true
		},
	}
}

func reconvergencePartition(t *testing.T, guards []Guard, facts ...Fact) Partition {
	t.Helper()
	partition, err := PartitionFromClosuresWithGuards(guards, OutputClosure{Values: facts})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

// TestOneArmedWriteJoinsAgainstFallThrough is the defining case: the edge that
// writes nothing is not absent, it carries the incoming state.  Reading only the
// visible pre-branch row would claim a value the other edge does not hold.
func TestOneArmedWriteJoinsAgainstFallThrough(t *testing.T) {
	partition := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000003", Value: []byte("arm"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found {
		t.Fatal("reconvergence withheld a complete two-edge join")
	}
	if string(fact.Value) != "arm|before" {
		t.Fatalf("joined value = %q, want arm|before", fact.Value)
	}
	if len(fact.Guards) != 0 {
		t.Fatalf("joined fact kept guards %v; a join belongs to the residual cube", fact.Guards)
	}
}

// TestUnrelatedDecisionsAreNotAlternatives is the precision guardrail.  Two
// guards that are simply inactive are not complementary edges of one decision;
// each contributes through its own join and neither stands in for the other's
// fall-through.
func TestUnrelatedDecisionsAreNotAlternatives(t *testing.T) {
	partition := reconvergencePartition(t, []Guard{edgeGuard("op-00000004", "false")},
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000003", Value: []byte("first"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
		Fact{Key: "value/x/op-00000005", Value: []byte("second"), Guards: []Guard{edgeGuard("op-00000004", "true")}},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found {
		t.Fatal("reconvergence withheld a complete join")
	}
	if string(fact.Value) != "before|first" {
		t.Fatalf("joined value = %q, want before|first -- a parallel decision's row leaked", fact.Value)
	}
}

// TestNestedDecisionsPeelOneAtATime checks that an inner arm write reaches the
// outermost point through two joins rather than one flat merge, and that the
// inner join is still guarded by the outer edge.
func TestNestedDecisionsPeelOneAtATime(t *testing.T) {
	outer, inner := edgeGuard("op-00000002", "true"), edgeGuard("op-00000003", "true")
	facts := []Fact{
		{Key: "value/x/op-00000001", Value: []byte("before")},
		{Key: "value/x/op-00000004", Value: []byte("deep"), Guards: []Guard{outer, inner}},
	}
	under := reconvergencePartition(t, []Guard{outer}, facts...)
	fact, found := under.Reconverged("value/x/", setLattice())
	if !found || string(fact.Value) != "before|deep" {
		t.Fatalf("inner join = %q / %v, want before|deep", fact.Value, found)
	}
	if len(fact.Guards) != 1 || string(fact.Guards[0].Encoding) != string(outer.Encoding) {
		t.Fatalf("inner join guards = %v, want the outer edge", fact.Guards)
	}
	full := reconvergencePartition(t, nil, facts...)
	fact, found = full.Reconverged("value/x/", setLattice())
	if !found || string(fact.Value) != "before|deep" {
		t.Fatalf("outer join = %q / %v, want before|deep", fact.Value, found)
	}
}

// TestPeelOrderDoesNotChangeTheResult states the determinism property the
// evaluator depends on: the join is the same value however the decisions are
// spelled, because it is the join over the same set of complete cubes.
func TestPeelOrderDoesNotChangeTheResult(t *testing.T) {
	forward := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000003", Value: []byte("a"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
		Fact{Key: "value/x/op-00000005", Value: []byte("b"), Guards: []Guard{edgeGuard("op-00000004", "true")}},
	)
	reversed := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000005", Value: []byte("b"), Guards: []Guard{edgeGuard("op-00000004", "true")}},
		Fact{Key: "value/x/op-00000003", Value: []byte("a"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	)
	left, leftFound := forward.Reconverged("value/x/", setLattice())
	right, rightFound := reversed.Reconverged("value/x/", setLattice())
	if !leftFound || !rightFound || string(left.Value) != string(right.Value) {
		t.Fatalf("join is order dependent: %q / %v vs %q / %v", left.Value, leftFound, right.Value, rightFound)
	}
	if string(left.Value) != "a|b|before" {
		t.Fatalf("joined value = %q, want a|b|before", left.Value)
	}
}

// TestIncompleteCoverageWithholds is the fail-closed rule.  An edge with no
// value is missing coverage, never lattice bottom, so the single surviving arm
// must not be published as the value of a point both edges reach.
func TestIncompleteCoverageWithholds(t *testing.T) {
	partition := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000003", Value: []byte("arm"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	)
	if fact, found := partition.Reconverged("value/x/", setLattice()); found {
		t.Fatalf("single-edge value %q escaped its guard", fact.Value)
	}
}

// TestUnpeelableGuardWithholds keeps the join inside the guard family it can
// certify.  A guard that names no branch decision has no complement, so it can
// never be joined away.
func TestUnpeelableGuardWithholds(t *testing.T) {
	partition := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000003", Value: []byte("arm"), Guards: []Guard{{Body: BodyID{1}, Encoding: []byte("front/unknown-family/op-00000002")}}},
	)
	if fact, found := partition.Reconverged("value/x/", setLattice()); found {
		t.Fatalf("value %q escaped an uncertified guard family", fact.Value)
	}
}

// TestRefusedLatticeJoinWithholds keeps a domain that cannot merge two payloads
// from being papered over with one of them.
func TestRefusedLatticeJoinWithholds(t *testing.T) {
	lattice := setLattice()
	lattice.Join = func(left, right []byte) ([]byte, bool) { return nil, false }
	partition := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000003", Value: []byte("arm"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	)
	if fact, found := partition.Reconverged("value/x/", lattice); found {
		t.Fatalf("refused join still published %q", fact.Value)
	}
}

// TestSupersededArmRowIsNotResurrected covers revocation: a row a later write on
// the same edge replaced is dead on that edge and must not return as that edge's
// contribution to the join.
func TestSupersededArmRowIsNotResurrected(t *testing.T) {
	guard := edgeGuard("op-00000002", "true")
	partition := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000003", Value: []byte("dead"), Guards: []Guard{guard}},
		Fact{Key: "value/x/op-00000004", Value: []byte("live"), Guards: []Guard{guard}},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found {
		t.Fatal("reconvergence withheld a complete join")
	}
	if string(fact.Value) != "before|live" {
		t.Fatalf("joined value = %q, want before|live", fact.Value)
	}
}

// TestProvenEdgePromotionSkipsTheJoin keeps a decided branch exact: once a
// branch proof is published the infeasible edge is not an alternative and the
// surviving arm is read directly.
func TestProvenEdgePromotionSkipsTheJoin(t *testing.T) {
	partition := reconvergencePartition(t, nil,
		Fact{Key: "value/x/op-00000001", Value: []byte("before")},
		Fact{Key: "value/x/op-00000003", Value: []byte("arm"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
		Fact{Key: "branch-proof/0100000000000000000000000000000000000000000000000000000000000000/op-00000002/true", Value: []byte("proven")},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found || string(fact.Value) != "arm" {
		t.Fatalf("proven-edge read = %q / %v, want the sole feasible arm", fact.Value, found)
	}
}

// TestDeepDecisionNestTerminatesWithinBudget states the termination property: a
// pathological number of live decisions over one term never diverges, it
// withholds.  The expansion is finite in every case; the budget only bounds how
// much of it one read may pay for.
func TestDeepDecisionNestTerminatesWithinBudget(t *testing.T) {
	facts := []Fact{{Key: "value/x/op-00000000", Value: []byte("before")}}
	var nest []Guard
	for index := 1; index <= 12; index++ {
		nest = append(nest, edgeGuard("op-"+string(rune('a'+index)), "true"))
		facts = append(facts, Fact{Key: "value/x/op-0000000" + string(rune('a'+index)), Value: []byte("arm"), Guards: append([]Guard(nil), nest...)})
	}
	partition := reconvergencePartition(t, nil, facts...)
	done := make(chan struct{})
	go func() {
		defer close(done)
		partition.Reconverged("value/x/", setLattice())
	}()
	<-done
}

// TestDecisionEdgesRestrictsToBothAlternatives is the peel Reconverged performs,
// exposed for a reader whose conclusion is not a lattice payload.  Each returned
// partition must answer the family exactly as its own edge answers it.
func TestDecisionEdgesRestrictsToBothAlternatives(t *testing.T) {
	partition := reconvergencePartition(t, nil,
		Fact{Key: "callee/f/op-00000001", Value: []byte("first")},
		Fact{Key: "callee/f/op-00000003", Value: []byte("second"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	)
	edges, split := partition.DecisionEdges("callee/f/")
	if !split {
		t.Fatal("an edge-guarded current row owes a split")
	}
	if string(edges[0].Guard.Encoding) != "front/branch/op-00000002/true" || string(edges[1].Guard.Encoding) != "front/branch/op-00000002/false" {
		t.Fatalf("edges name %q and %q, want the two alternatives of op-00000002", edges[0].Guard.Encoding, edges[1].Guard.Encoding)
	}
	taken, found := edges[0].Partition.LatestValuePrefix("callee/f/")
	if !found || string(taken.Value) != "second" {
		t.Fatalf("taken edge holds %q / %v, want the arm write", taken.Value, found)
	}
	untaken, found := edges[1].Partition.LatestValuePrefix("callee/f/")
	if !found || string(untaken.Value) != "first" {
		t.Fatalf("untaken edge holds %q / %v, want the pre-branch write", untaken.Value, found)
	}
}

// TestDecisionEdgesWithholdsWhenTheCubeAlreadyDecides keeps the split from
// firing where there is nothing to peel: inside the arm, and where the family's
// current row carries no guard at all.  A consumer must keep its ordinary
// single-partition evaluation in both cases.
func TestDecisionEdgesWithholdsWhenTheCubeAlreadyDecides(t *testing.T) {
	facts := []Fact{
		{Key: "callee/f/op-00000001", Value: []byte("first")},
		{Key: "callee/f/op-00000003", Value: []byte("second"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	}
	inside := reconvergencePartition(t, []Guard{edgeGuard("op-00000002", "true")}, facts...)
	if _, split := inside.DecisionEdges("callee/f/"); split {
		t.Fatal("a cube that already fixes the decision owes no split")
	}
	unguarded := reconvergencePartition(t, nil, facts[0])
	if _, split := unguarded.DecisionEdges("callee/f/"); split {
		t.Fatal("a fully decided current row owes no split")
	}
	if _, split := unguarded.DecisionEdges("callee/absent/"); split {
		t.Fatal("a family that publishes nothing owes no split")
	}
}

// TestDecisionEdgesRefusesAnUnpeelableGuard fails closed on a guard family that
// names no decision: two rows under unrelated guards are not alternatives, and
// splitting on them would invent a control-flow relation the front never
// certified.
func TestDecisionEdgesRefusesAnUnpeelableGuard(t *testing.T) {
	partition := reconvergencePartition(t, nil,
		Fact{Key: "callee/f/op-00000001", Value: []byte("first")},
		Fact{Key: "callee/f/op-00000003", Value: []byte("second"), Guards: []Guard{{Body: BodyID{1}, Encoding: []byte("front/select/op-00000002/arm")}}},
	)
	if _, split := partition.DecisionEdges("callee/f/"); split {
		t.Fatal("a guard naming no decision owes no split")
	}
}

// TestLoopExitJoinsTheArmThatStaysInsideTheLoop pins the recurrence exit. The
// arm that re-enters a loop republishes on every trip, so a point the leaving
// arm alone reaches stands after all of them and holds what they carried. A cube
// that still selects the value the loop received must therefore join both arms
// instead of reporting that seed.
func TestLoopExitJoinsTheArmThatStaysInsideTheLoop(t *testing.T) {
	partition := reconvergencePartition(t, []Guard{edgeGuard("op-00000002", "false")},
		Fact{Key: "front/recurrence-exit/op-00000002", Value: []byte("false")},
		Fact{Key: "value/x/op-00000001", Value: []byte("seed")},
		Fact{Key: "value/x/op-00000003", Value: []byte("carried"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found {
		t.Fatal("reconvergence withheld the loop-exit join")
	}
	if string(fact.Value) != "carried|seed" {
		t.Fatalf("loop-exit value = %q, want carried|seed -- the exit reported the value the loop received", fact.Value)
	}
}

// TestLoopExitKeepsAPublicationTheDecisionOwns is the precision guardrail for
// the same rule. A row the decision itself published, or any row past it, was
// derived from the value entering the decision -- which is already the join over
// every trip -- so the exit keeps it exactly instead of widening it again.
func TestLoopExitKeepsAPublicationTheDecisionOwns(t *testing.T) {
	partition := reconvergencePartition(t, []Guard{edgeGuard("op-00000002", "false")},
		Fact{Key: "front/recurrence-exit/op-00000002", Value: []byte("false")},
		Fact{Key: "value/x/op-00000001", Value: []byte("seed")},
		Fact{Key: "value/x/op-00000003", Value: []byte("carried"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
		Fact{Key: "value/x/op-00000004", Value: []byte("narrowed"), Guards: []Guard{edgeGuard("op-00000002", "false")}},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found {
		t.Fatal("reconvergence withheld a decided loop-exit read")
	}
	if string(fact.Value) != "narrowed" {
		t.Fatalf("loop-exit value = %q, want narrowed -- what the exit itself established was joined away", fact.Value)
	}
}

// TestRecurrenceExitAppliesOnlyToTheArmThatLeaves keeps the rule off the arm
// that stays inside. A point inside the loop is separated from the exit by the
// decision exactly as any arm is separated from its alternative.
func TestRecurrenceExitAppliesOnlyToTheArmThatLeaves(t *testing.T) {
	partition := reconvergencePartition(t, []Guard{edgeGuard("op-00000002", "true")},
		Fact{Key: "front/recurrence-exit/op-00000002", Value: []byte("false")},
		Fact{Key: "value/x/op-00000001", Value: []byte("seed")},
		Fact{Key: "value/x/op-00000004", Value: []byte("past"), Guards: []Guard{edgeGuard("op-00000002", "false")}},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found {
		t.Fatal("reconvergence withheld a read inside the loop")
	}
	if string(fact.Value) != "seed" {
		t.Fatalf("in-loop value = %q, want seed -- a publication past the loop reached a point inside it", fact.Value)
	}
}

// TestDecisionWithNoPublishedExitStaysExclusive is the acyclic reading: without
// the deciding body's own exit publication a decision's arms are alternatives,
// and one arm's row never reaches the other.
func TestDecisionWithNoPublishedExitStaysExclusive(t *testing.T) {
	partition := reconvergencePartition(t, []Guard{edgeGuard("op-00000002", "false")},
		Fact{Key: "value/x/op-00000001", Value: []byte("seed")},
		Fact{Key: "value/x/op-00000003", Value: []byte("arm"), Guards: []Guard{edgeGuard("op-00000002", "true")}},
	)
	fact, found := partition.Reconverged("value/x/", setLattice())
	if !found {
		t.Fatal("reconvergence withheld an ordinary decided read")
	}
	if string(fact.Value) != "seed" {
		t.Fatalf("decided value = %q, want seed -- an arm reached its own alternative", fact.Value)
	}
}
