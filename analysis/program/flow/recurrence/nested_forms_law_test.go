package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSealNestedGenericHeaderIsOutsideInnerReset(t *testing.T) {
	parent := term(keyspace.FamilyBody, 1)
	outerBody := term(keyspace.FamilyBody, 2)
	innerBody := term(keyspace.FamilyBody, 3)
	outer := term(keyspace.FamilyLoop, 1)
	inner := term(keyspace.FamilyLoop, 2)
	outerValues := term(keyspace.FamilyValues, 1)
	innerValues := term(keyspace.FamilyValues, 2)
	outerSelect := term(keyspace.FamilySelect, 1)
	innerSelect := term(keyspace.FamilySelect, 2)
	cellOne := term(keyspace.FamilyCell, 1)
	cellTwo := term(keyspace.FamilyCell, 2)
	nils := []keyspace.Term{
		term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2),
		term(keyspace.FamilyNil, 3), term(keyspace.FamilyNil, 4),
	}
	fixture := openOwnerFixture(t, ownerSpec{
		counts: countsWith(
			familyCount(keyspace.FamilyBody, 3),
			familyCount(keyspace.FamilyNil, uint32(len(nils))),
			familyCount(keyspace.FamilyValues, 2),
			familyCount(keyspace.FamilySelect, 2),
			familyCount(keyspace.FamilyCell, 2),
			familyCount(keyspace.FamilyLoop, 2),
		),
		rows:      [][]keyspace.Term{{outer}, {inner}, nil},
		nilOwners: []keyspace.Term{parent, parent, outerBody, outerBody},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: parent, Fixed: authored.Range{End: 1}},
					{Owner: outerBody, Fixed: authored.Range{Start: 1, End: 2}},
				},
				Terms: []keyspace.Term{outerSelect, innerSelect},
			},
			Operators: authored.OperatorsInput{Selects: []authored.Select{
				{Owner: parent, Op: kind.SelectAnd, Left: nils[0], Right: nils[1]},
				{Owner: outerBody, Op: kind.SelectOr, Left: nils[2], Right: nils[3]},
			}},
			Storage: authored.StorageInput{Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: outerBody},
				{Kind: authored.CellLocal, Body: innerBody},
			}},
			Control: authored.ControlInput{
				Cells: []keyspace.Term{cellOne, cellTwo},
				Loops: []authored.Loop{
					{Owner: parent, Body: outerBody, Kind: kind.LoopGenericFor, Control: outerValues, Cells: authored.Range{End: 1}},
					{Owner: outerBody, Body: innerBody, Kind: kind.LoopGenericFor, Control: innerValues, Cells: authored.Range{Start: 1, End: 2}},
				},
			},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	wantDecisions := []keyspace.Term{outer, innerSelect, inner}
	if count, ok := recurrence.DecisionCount(outer); !ok || count != len(wantDecisions) {
		t.Fatalf("nested generic stream = %d/%v, want %d/true", count, ok, len(wantDecisions))
	}
	for index, expected := range wantDecisions {
		got, ok := recurrence.DecisionAt(outer, index)
		if !ok || got != expected {
			t.Fatalf("nested generic decision %d = %v/%v, want %v/true", index, got, ok, expected)
		}
	}
	if _, ok := recurrence.DecisionCount(inner); ok {
		t.Fatal("nested generic inner Loop became a second Mu head")
	}
	assertOwnerArcRange(t, fixture, recurrence, outerBody, outer, 0, outer, 0, 3,
		map[keyspace.Term]bool{outer: true, innerSelect: true, inner: true})
	assertOwnerArcRange(t, fixture, recurrence, innerBody, inner, 0, outer, 2, 3,
		map[keyspace.Term]bool{outer: false, innerSelect: false, inner: true})
	recurrent := 0
	for index := 0; index < recurrence.ArcCount(); index++ {
		annotation, ok := recurrence.ArcAt(index)
		if ok && annotation.Head != 0 {
			recurrent++
		}
	}
	if recurrent != 2 {
		t.Fatalf("nested generic recurrent Arc count = %d, want 2", recurrent)
	}
}

func assertOwnerArcRange(
	t *testing.T,
	fixture *ownerFixture,
	recurrence *Result,
	sourceTerm, targetTerm, decision, head keyspace.Term,
	wantFirst, wantPast uint32,
	want map[keyspace.Term]bool,
) {
	t.Helper()
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		arc, ok := fixture.graph.ArcAt(index)
		if !ok || arc.Source != sourceTerm || arc.Target != targetTerm || arc.Decision != decision {
			continue
		}
		annotation, ok := recurrence.ArcAt(index)
		if !ok {
			t.Fatalf("owner Arc %d has no recurrence annotation", index)
		}
		wantAnnotation := Annotation{Head: head, First: wantFirst, Past: wantPast}
		if annotation != wantAnnotation {
			t.Fatalf("owner Arc %d annotation = %#v, want %#v", index, annotation, wantAnnotation)
		}
		if count, ok := recurrence.ResetCount(index); !ok || count != int(wantPast-wantFirst) {
			t.Fatalf("owner Arc %d reset count = %d/%v, want %d/true", index, count, ok, wantPast-wantFirst)
		}
		for decision, expected := range want {
			if got := recurrence.ResetContains(index, decision); got != expected {
				t.Fatalf("owner Arc %d ResetContains(%v) = %v, want %v", index, decision, got, expected)
			}
		}
		return
	}
	t.Fatalf("owner Arc %v -> %v was not found", sourceTerm, targetTerm)
}

func TestSealNestedRepeatUsesBodyThenConditionInsideOuterMu(t *testing.T) {
	parent := term(keyspace.FamilyBody, 1)
	outerBody := term(keyspace.FamilyBody, 2)
	repeatBody := term(keyspace.FamilyBody, 3)
	outer := term(keyspace.FamilyLoop, 1)
	repeat := term(keyspace.FamilyLoop, 2)
	repeatSelect := term(keyspace.FamilySelect, 1)
	nils := []keyspace.Term{
		term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2),
		term(keyspace.FamilyNil, 3),
	}
	fixture := openOwnerFixture(t, ownerSpec{
		counts: countsWith(
			familyCount(keyspace.FamilyBody, 3),
			familyCount(keyspace.FamilyNil, uint32(len(nils))),
			familyCount(keyspace.FamilySelect, 1),
			familyCount(keyspace.FamilyLoop, 2),
		),
		rows:      [][]keyspace.Term{{outer}, {repeat}, nil},
		nilOwners: []keyspace.Term{parent, repeatBody, repeatBody},
		flow: authored.Input{
			Operators: authored.OperatorsInput{Selects: []authored.Select{
				{Owner: repeatBody, Op: kind.SelectAnd, Left: nils[1], Right: nils[2]},
			}},
			Control: authored.ControlInput{Loops: []authored.Loop{
				{Owner: parent, Body: outerBody, Kind: kind.LoopWhile, Control: nils[0]},
				{Owner: outerBody, Body: repeatBody, Kind: kind.LoopRepeat, Control: repeatSelect},
			}},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	wantDecisions := []keyspace.Term{outer, repeatSelect, repeat}
	if count, ok := recurrence.DecisionCount(outer); !ok || count != len(wantDecisions) {
		t.Fatalf("outer/repeat stream = %d/%v, want %d/true", count, ok, len(wantDecisions))
	}
	for index, expected := range wantDecisions {
		got, ok := recurrence.DecisionAt(outer, index)
		if !ok || got != expected {
			t.Fatalf("outer/repeat decision %d = %v/%v, want %v/true", index, got, ok, expected)
		}
	}
	if _, ok := recurrence.DecisionCount(repeat); ok {
		t.Fatal("nested Repeat became a second Mu head")
	}
	assertOwnerArcRange(t, fixture, recurrence, outerBody, outer, 0, outer, 0, 3,
		map[keyspace.Term]bool{outer: true, repeatSelect: true, repeat: true})
	assertOwnerArcRange(t, fixture, recurrence, repeat, repeatBody, repeat, outer, 1, 3,
		map[keyspace.Term]bool{outer: false, repeatSelect: true, repeat: true})
}

func TestSealTranslatesIndependentSCCBoundariesThroughOwners(t *testing.T) {
	parent := term(keyspace.FamilyBody, 1)
	firstBody := term(keyspace.FamilyBody, 2)
	secondBody := term(keyspace.FamilyBody, 3)
	firstTrue := term(keyspace.FamilyBody, 4)
	firstFalse := term(keyspace.FamilyBody, 5)
	secondTrue := term(keyspace.FamilyBody, 6)
	secondFalse := term(keyspace.FamilyBody, 7)
	first := term(keyspace.FamilyLoop, 1)
	second := term(keyspace.FamilyLoop, 2)
	firstBranch := term(keyspace.FamilyBranch, 1)
	secondBranch := term(keyspace.FamilyBranch, 2)
	nils := []keyspace.Term{
		term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2),
		term(keyspace.FamilyNil, 3), term(keyspace.FamilyNil, 4),
	}
	fixture := openOwnerFixture(t, ownerSpec{
		counts: countsWith(
			familyCount(keyspace.FamilyBody, 7),
			familyCount(keyspace.FamilyNil, uint32(len(nils))),
			familyCount(keyspace.FamilyBranch, 2),
			familyCount(keyspace.FamilyLoop, 2),
		),
		rows:      [][]keyspace.Term{{first, second}, {firstBranch}, {secondBranch}, nil, nil, nil, nil},
		nilOwners: []keyspace.Term{parent, parent, firstBody, secondBody},
		flow: authored.Input{Control: authored.ControlInput{
			Branches: []authored.Branch{
				{Owner: firstBody, Condition: nils[2], WhenTrue: firstTrue, WhenFalse: firstFalse},
				{Owner: secondBody, Condition: nils[3], WhenTrue: secondTrue, WhenFalse: secondFalse},
			},
			Loops: []authored.Loop{
				{Owner: parent, Body: firstBody, Kind: kind.LoopWhile, Control: nils[0]},
				{Owner: parent, Body: secondBody, Kind: kind.LoopWhile, Control: nils[1]},
			},
		},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticView.ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	assertOwnerStream(t, recurrence, first, []keyspace.Term{first, firstBranch})
	assertOwnerStream(t, recurrence, second, []keyspace.Term{second, secondBranch})
	assertOwnerArcRange(t, fixture, recurrence, firstBody, first, 0, first, 0, 2,
		map[keyspace.Term]bool{first: true, firstBranch: true, second: false, secondBranch: false})
	assertOwnerArcRange(t, fixture, recurrence, secondBody, second, 0, second, 0, 2,
		map[keyspace.Term]bool{first: false, firstBranch: false, second: true, secondBranch: true})
}

func assertOwnerStream(t *testing.T, recurrence *Result, head keyspace.Term, want []keyspace.Term) {
	t.Helper()
	count, ok := recurrence.DecisionCount(head)
	if !ok || count != len(want) {
		t.Fatalf("head %v stream = %d/%v, want %d/true", head, count, ok, len(want))
	}
	for index, expected := range want {
		got, ok := recurrence.DecisionAt(head, index)
		if !ok || got != expected {
			t.Fatalf("head %v decision %d = %v/%v, want %v/true", head, index, got, ok, expected)
		}
	}
}
