package project

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

// TestCallsForCallReissuesOnlyExecutableSourceOccurrences exercises the
// owner-local inverse at both ends of the source Call sequence.  The dead
// Call remains in the authored Program relation but is deliberately absent
// from Project Applications.
func TestCallsForCallReissuesOnlyExecutableSourceOccurrences(t *testing.T) {
	p := projectProgram(t, `
first()
local value = 1 + 2
last()
do return end
dead()
`)
	draft := projectDraft(t, []Module{{Name: "main", Program: p}})
	shard, ok := draft.Mounts().At(0)
	if !ok {
		t.Fatal("missing Project Shard")
	}
	calls := draft.Applications().Calls()
	authored := p.Flow().Authored().Calls()
	var live []keyspace.Term
	var dead keyspace.Term
	for index := 0; index < authored.Count(); index++ {
		term, termOK := authored.At(index)
		if !termOK {
			t.Fatalf("authored Call %d unavailable", index)
		}
		if p.Flow().Executable().Contains(term) {
			live = append(live, term)
		} else {
			dead = term
		}
	}
	if len(live) < 2 || dead == 0 {
		t.Fatalf("fixture Calls live=%d dead=%v, want first/last live and one dead", len(live), dead)
	}
	first, firstOK := calls.ForCall(shard, p, live[0])
	last, lastOK := calls.ForCall(shard, p, live[len(live)-1])
	if !firstOK || !lastOK || first == (Application{}) || last == (Application{}) {
		t.Fatalf("first/last inverse = %#v/%v %#v/%v", first, firstOK, last, lastOK)
	}
	if first == last {
		t.Fatal("distinct source Calls collapsed to one Application")
	}
	for _, application := range []Application{first, last} {
		gotShard, gotTerm, ok := draft.Applications().Call(application)
		if !ok || gotShard != shard || gotTerm == 0 || !p.Flow().Executable().Contains(gotTerm) {
			t.Fatalf("Application round-trip = %v/%v/%v", gotShard, gotTerm, ok)
		}
		if roundTrip, ok := calls.ForCall(gotShard, p, gotTerm); !ok || roundTrip != application {
			t.Fatalf("Call inverse round-trip = %#v/%v", roundTrip, ok)
		}
	}
	if _, ok := calls.ForCall(shard, p, dead); ok {
		t.Fatal("dead authored Call entered Project Applications")
	}
	if _, ok := calls.ForCall(shard, p, 0); ok {
		t.Fatal("zero Call term accepted")
	}
	if _, ok := calls.ForCall(shard, p, keyspace.Term(^uint32(0))); ok {
		t.Fatal("out-of-range Call term accepted")
	}

	arithmetic, arithmeticOK := p.Flow().Candidates().Binary().ArithmeticAt(0)
	if !arithmeticOK {
		t.Fatal("missing arithmetic source for wrong-family check")
	}
	if _, ok := calls.ForCall(shard, p, arithmetic); ok {
		t.Fatal("non-Call operator term accepted as Call Application")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, ok := calls.ForCall(shard, p, live[0]); !ok {
			panic("live Call inverse disappeared")
		}
	}); allocations != 0 {
		t.Fatalf("Calls.ForCall allocated %v times per query", allocations)
	}
}

// TestCallsForCallFencesEquivalentProjectsAndSeparatesDuplicateMounts proves
// that the inverse is keyed by the exact owner-fenced Shard, not by Program
// content or a bare source term. Duplicate mounts therefore have two exact
// applications while equivalent reseals cannot exchange either handle.
func TestCallsForCallFencesEquivalentProjectsAndSeparatesDuplicateMounts(t *testing.T) {
	p := projectProgram(t, `run()`)
	foreignProgram := projectProgram(t, `run()`)
	first := projectDraft(t, []Module{{Name: "left", Program: p}, {Name: "right", Program: p}})
	second := projectDraft(t, []Module{{Name: "left", Program: p}, {Name: "right", Program: p}})
	authored := p.Flow().Authored().Calls()
	var call keyspace.Term
	for index := 0; index < authored.Count(); index++ {
		candidate, ok := authored.At(index)
		if ok && p.Flow().Executable().Contains(candidate) {
			call = candidate
			break
		}
	}
	if call == 0 {
		t.Fatal("missing executable source Call")
	}
	firstShard, firstOK := first.Mounts().At(0)
	secondShard, secondOK := first.Mounts().At(1)
	foreignShard, foreignOK := second.Mounts().At(0)
	if !firstOK || !secondOK || !foreignOK {
		t.Fatal("duplicate Project mounts unavailable")
	}
	left, leftOK := first.Applications().Calls().ForCall(firstShard, p, call)
	right, rightOK := first.Applications().Calls().ForCall(secondShard, p, call)
	if !leftOK || !rightOK || left == right {
		t.Fatalf("duplicate mount inverse left=%#v/%v right=%#v/%v", left, leftOK, right, rightOK)
	}
	if _, ok := first.Applications().Calls().ForCall(foreignShard, p, call); ok {
		t.Fatal("equivalent foreign Project Shard crossed owner fence")
	}
	if _, ok := first.Applications().Calls().ForCall(firstShard, foreignProgram, call); ok {
		t.Fatal("equal-term foreign Program crossed mounted-Program fence")
	}
	if _, ok := second.Applications().Calls().ForCall(firstShard, p, call); ok {
		t.Fatal("foreign Project inverse accepted first Project Shard")
	}
	if gotShard, gotTerm, ok := first.Applications().Call(left); !ok || gotShard != firstShard || gotTerm != call {
		t.Fatalf("left duplicate mount round-trip = %v/%v/%v", gotShard, gotTerm, ok)
	}
	if gotShard, gotTerm, ok := first.Applications().Call(right); !ok || gotShard != secondShard || gotTerm != call {
		t.Fatalf("right duplicate mount round-trip = %v/%v/%v", gotShard, gotTerm, ok)
	}
}
