package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// gotoArcIndex resolves the canonical sourcecontrol witness for one authored
// Goto.  The test deliberately walks the sealed owner result instead of
// manufacturing an Arc or relying on recurrence's construction state.
func gotoArcIndex(t *testing.T, fixture *ownerFixture, gotoTerm keyspace.Term) int {
	t.Helper()
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		arc, ok := fixture.graph.ArcAt(index)
		if ok && arc.Source == gotoTerm {
			if keyspace.TermFamily(arc.Target) != keyspace.FamilyLabel || arc.Decision != 0 || arc.Truth {
				t.Fatalf("Goto %v witness = %#v, want an unguarded Label transfer", gotoTerm, arc)
			}
			return index
		}
	}
	t.Fatalf("sealed sourcecontrol graph has no witness for Goto %v", gotoTerm)
	return -1
}

func assertGotoMu(t *testing.T, recurrence *Result, arcIndex int, head keyspace.Term, wantCount int) {
	t.Helper()
	annotation, ok := recurrence.ArcAt(arcIndex)
	if !ok {
		t.Fatalf("recurrence Arc %d is unavailable", arcIndex)
	}
	if annotation.Head != head {
		t.Fatalf("recurrence Arc %d head = %v, want %v", arcIndex, annotation.Head, head)
	}
	count, ok := recurrence.ResetCount(arcIndex)
	if !ok || count != int(annotation.Past-annotation.First) {
		t.Fatalf("recurrence Arc %d reset = %d/%v, want exact half-open range", arcIndex, count, ok)
	}
	if count != wantCount {
		t.Fatalf("Goto recurrence Arc %d reset %d semantic decisions, want %d", arcIndex, count, wantCount)
	}
	if wantCount == 0 {
		if _, ok := recurrence.ResetAt(arcIndex, 0); ok {
			t.Fatalf("empty Goto recurrence Arc %d returned a reset decision", arcIndex)
		}
	}
	if wantCount > 0 {
		if got, ok := recurrence.ResetAt(arcIndex, 0); !ok || got == 0 {
			t.Fatalf("non-empty Goto recurrence Arc %d returned %v/%v", arcIndex, got, ok)
		}
	}
}

func assertGotoNoMu(t *testing.T, recurrence *Result, arcIndex int) {
	t.Helper()
	annotation, ok := recurrence.ArcAt(arcIndex)
	if !ok {
		t.Fatalf("recurrence Arc %d is unavailable", arcIndex)
	}
	if annotation.Head != 0 {
		t.Fatalf("non-backward Goto recurrence Arc %d received Mu head %v", arcIndex, annotation.Head)
	}
	if count, ok := recurrence.ResetCount(arcIndex); ok || count != 0 {
		t.Fatalf("non-backward Goto recurrence Arc %d has reset range %d/%v", arcIndex, count, ok)
	}
	if _, ok := recurrence.ResetAt(arcIndex, 0); ok {
		t.Fatalf("non-backward Goto recurrence Arc %d returned a reset decision", arcIndex)
	}
}

// An outward jump from a nested Body is legal only to an enclosing lexical
// scope.  The child is visited before the parent's later Label: the first
// jump is therefore a backward transfer, while the second is a forward
// transfer.  Only the backward edge may carry the enclosing Mu head.
func TestSealNestedOutwardGotoMatrix(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyLabel, 2),
		familyCount(keyspace.FamilyGoto, 2),
	)
	parent := term(keyspace.FamilyBody, 1)
	child := term(keyspace.FamilyBody, 2)
	before := term(keyspace.FamilyLabel, 1)
	after := term(keyspace.FamilyLabel, 2)
	backward := term(keyspace.FamilyGoto, 1)
	forward := term(keyspace.FamilyGoto, 2)
	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		rows: [][]keyspace.Term{
			{before, child, after},
			{backward, forward},
		},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}, {Owner: parent}},
			Gotos: []authored.Goto{
				{Owner: child, Target: before},
				{Owner: child, Target: after},
			},
		}},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	backwardIndex := gotoArcIndex(t, fixture, backward)
	forwardIndex := gotoArcIndex(t, fixture, forward)
	assertGotoMu(t, recurrence, backwardIndex, before, 0)
	assertGotoNoMu(t, recurrence, forwardIndex)
	if count, ok := recurrence.DecisionCount(before); !ok || count != 0 {
		t.Fatalf("outward backward Goto head stream = %d/%v, want empty/true", count, ok)
	}
}

// Crossed Gotos form an irreducible sourcecontrol component: the first edge
// jumps forward to Label3, while the later two edges jump backward across
// distinct boundaries.  The forward edge is an ingress/transfer in the
// structural SCC but is not a recurrence reset; both exact backward edges
// select the one canonical minimum Label head.
func TestSealCrossedIrreducibleGotoMatrix(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyNil, 1),
		familyCount(keyspace.FamilyLabel, 3),
		familyCount(keyspace.FamilyGoto, 3),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	loopBody := term(keyspace.FamilyBody, 2)
	label1 := term(keyspace.FamilyLabel, 1)
	label2 := term(keyspace.FamilyLabel, 2)
	label3 := term(keyspace.FamilyLabel, 3)
	loop := term(keyspace.FamilyLoop, 1)
	goto1 := term(keyspace.FamilyGoto, 1)
	goto2 := term(keyspace.FamilyGoto, 2)
	goto3 := term(keyspace.FamilyGoto, 3)
	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		rows: [][]keyspace.Term{
			{label1, loop, goto1, label2, goto2, label3, goto3},
			nil,
		},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}, {Owner: parent}, {Owner: parent}},
			Gotos: []authored.Goto{
				{Owner: parent, Target: label3},
				{Owner: parent, Target: label1},
				{Owner: parent, Target: label2},
			},
			Loops: []authored.Loop{{Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 1)}},
		}},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	assertGotoNoMu(t, recurrence, gotoArcIndex(t, fixture, goto1))
	goto2Index := gotoArcIndex(t, fixture, goto2)
	goto3Index := gotoArcIndex(t, fixture, goto3)
	assertGotoMu(t, recurrence, goto2Index, label1, 1)
	assertGotoMu(t, recurrence, goto3Index, label1, 0)
	if count, ok := recurrence.DecisionCount(label1); !ok || count != 1 {
		t.Fatalf("crossed Goto head stream = %d/%v, want one/true", count, ok)
	}
	if got, ok := recurrence.DecisionAt(label1, 0); !ok || got != loop {
		t.Fatalf("crossed Goto head decision = %v/%v, want %v/true", got, ok, loop)
	}
	if !recurrence.ResetContains(goto2Index, loop) {
		t.Fatal("backward crossed Goto reset omitted the loop decision")
	}
	if recurrence.ResetContains(goto3Index, loop) {
		t.Fatal("backward crossed Goto reset leaked the loop decision across Label2")
	}
}
