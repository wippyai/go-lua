package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealAllLoopFormsThroughOwners(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 5),
		familyCount(keyspace.FamilyValues, 2),
		familyCount(keyspace.FamilyNil, 5),
		familyCount(keyspace.FamilyCell, 3),
		familyCount(keyspace.FamilyLoop, 4),
	)
	parent := term(keyspace.FamilyBody, 1)
	children := []keyspace.Term{term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3), term(keyspace.FamilyBody, 4), term(keyspace.FamilyBody, 5)}
	loops := loopTerms()
	nils := []keyspace.Term{term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2), term(keyspace.FamilyNil, 3), term(keyspace.FamilyNil, 4), term(keyspace.FamilyNil, 5)}
	values := []keyspace.Term{term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)}
	cells := []keyspace.Term{term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2), term(keyspace.FamilyCell, 3)}
	fixture := openOwnerFixture(t, ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loops[0], loops[1], loops[2], loops[3]}, nil, nil, nil, nil},
		nilOwners: []keyspace.Term{parent, parent, parent, parent, children[1]},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: parent, Fixed: authored.Range{End: 2}}, {Owner: parent, Fixed: authored.Range{Start: 2, End: 3}}},
				Terms: []keyspace.Term{nils[0], nils[1], nils[2]},
			},
			Storage: authored.StorageInput{Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: children[2]},
				{Kind: authored.CellLocal, Body: children[3]},
				{Kind: authored.CellLocal, Body: children[3]},
			}},
			Control: authored.ControlInput{
				Loops: []authored.Loop{
					{Owner: parent, Body: children[0], Kind: kind.LoopWhile, Control: nils[3]},
					{Owner: parent, Body: children[1], Kind: kind.LoopRepeat, Control: nils[4]},
					{Owner: parent, Body: children[2], Kind: kind.LoopNumericFor, Control: values[0], Cells: authored.Range{Start: 0, End: 1}},
					{Owner: parent, Body: children[3], Kind: kind.LoopGenericFor, Control: values[1], Cells: authored.Range{Start: 1, End: 3}},
				},
				Cells: cells,
			},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	for index, loop := range loops {
		count, ok := recurrence.DecisionCount(loop)
		if !ok || count != 1 {
			t.Fatalf("loop %d stream = %d/%v, want 1/true", index, count, ok)
		}
		if got, ok := recurrence.DecisionAt(loop, 0); !ok || got != loop {
			t.Fatalf("loop %d stream head = %v/%v, want %v/true", index, got, ok, loop)
		}
		resetArcs := 0
		for arcIndex := 0; arcIndex < fixture.graph.ArcCount(); arcIndex++ {
			annotation, annotationOK := recurrence.ArcAt(arcIndex)
			if !annotationOK || annotation.Head != loop {
				continue
			}
			resetArcs++
			if count, ok := recurrence.ResetCount(arcIndex); !ok || count != 1 {
				t.Fatalf("loop %d reset %d = %d/%v, want one/true", index, arcIndex, count, ok)
			}
			if got, ok := recurrence.ResetAt(arcIndex, 0); !ok || got != loop {
				t.Fatalf("loop %d reset term = %v/%v, want %v/true", index, got, ok, loop)
			}
		}
		if resetArcs != 1 {
			t.Fatalf("loop %d recurrent Arc count = %d, want 1", index, resetArcs)
		}
	}
}

func TestSealWhileRangeIncludesShortCircuitAndNestedBranch(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 4),
		familyCount(keyspace.FamilyNil, 3),
		familyCount(keyspace.FamilySelect, 1),
		familyCount(keyspace.FamilyBranch, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	loopBody := term(keyspace.FamilyBody, 2)
	whenTrue := term(keyspace.FamilyBody, 3)
	whenFalse := term(keyspace.FamilyBody, 4)
	loop := term(keyspace.FamilyLoop, 1)
	branch := term(keyspace.FamilyBranch, 1)
	selectTerm := term(keyspace.FamilySelect, 1)
	nils := []keyspace.Term{term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2), term(keyspace.FamilyNil, 3)}
	fixture := openOwnerFixture(t, ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop}, {branch}, nil, nil},
		nilOwners: []keyspace.Term{parent, parent, loopBody},
		flow: authored.Input{
			Operators: authored.OperatorsInput{Selects: []authored.Select{{Owner: parent, Op: kind.SelectAnd, Left: nils[0], Right: nils[1]}}},
			Control: authored.ControlInput{
				Branches: []authored.Branch{{Owner: loopBody, Condition: nils[2], WhenTrue: whenTrue, WhenFalse: whenFalse}},
				Loops:    []authored.Loop{{Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: selectTerm}},
			},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	count, ok := recurrence.DecisionCount(loop)
	if !ok || count != 3 {
		t.Fatalf("while decision stream = %d/%v, want 3/true", count, ok)
	}
	want := []keyspace.Term{selectTerm, loop, branch}
	for index, expected := range want {
		got, ok := recurrence.DecisionAt(loop, index)
		if !ok || got != expected {
			t.Fatalf("while decision %d = %v/%v, want %v/true", index, got, ok, expected)
		}
	}
	resets := 0
	for arcIndex := 0; arcIndex < fixture.graph.ArcCount(); arcIndex++ {
		annotation, annotationOK := recurrence.ArcAt(arcIndex)
		if !annotationOK || annotation.Head != loop {
			continue
		}
		resets++
		if count, ok := recurrence.ResetCount(arcIndex); !ok || count != 3 {
			t.Fatalf("while reset = %d/%v, want 3/true", count, ok)
		}
		for index, expected := range want {
			if !recurrence.ResetContains(arcIndex, expected) {
				t.Fatalf("while reset omitted decision %d (%v)", index, expected)
			}
		}
	}
	if resets != 1 {
		t.Fatalf("while recurrent Arc count = %d, want 1", resets)
	}
	replay, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("replayed recurrence.Seal: %v", err)
	}
	if replay.ArcCount() != recurrence.ArcCount() {
		t.Fatalf("replayed Arc denominator = %d, want %d", replay.ArcCount(), recurrence.ArcCount())
	}
	for index := 0; index < recurrence.ArcCount(); index++ {
		first, firstOK := recurrence.ArcAt(index)
		second, secondOK := replay.ArcAt(index)
		if !firstOK || !secondOK || first != second {
			t.Fatalf("replayed Arc %d = %#v/%v, want %#v/true", index, second, secondOK, first)
		}
	}
	if got := testing.AllocsPerRun(100, func() {
		_, _ = recurrence.DecisionCount(loop)
		_, _ = recurrence.DecisionAt(loop, 0)
		for index := 0; index < recurrence.ArcCount(); index++ {
			_, _ = recurrence.ResetCount(index)
			_, _ = recurrence.ResetAt(index, 0)
			_ = recurrence.ResetContains(index, loop)
		}
	}); got != 0 {
		t.Fatalf("recurrence queries allocate %f times", got)
	}
}

func TestSealWhileUsesTypedSelectOrderThroughOwners(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyNil, 3),
		familyCount(keyspace.FamilySelect, 2),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	child := term(keyspace.FamilyBody, 2)
	inner := term(keyspace.FamilySelect, 1)
	outer := term(keyspace.FamilySelect, 2)
	loop := term(keyspace.FamilyLoop, 1)
	nils := []keyspace.Term{term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2), term(keyspace.FamilyNil, 3)}
	fixture := openOwnerFixture(t, ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent, parent, parent},
		flow: authored.Input{
			Operators: authored.OperatorsInput{Selects: []authored.Select{
				// The authored rows are intentionally postorder: the inner
				// Select is allocated before the outer one. Evaluation order for
				// this right-nested condition is outer, then inner.
				{Owner: parent, Op: kind.SelectAnd, Left: nils[0], Right: nils[1]},
				{Owner: parent, Op: kind.SelectOr, Left: nils[2], Right: inner},
			}},
			Control: authored.ControlInput{Loops: []authored.Loop{{
				Owner: parent, Body: child, Kind: kind.LoopWhile, Control: outer,
			}}},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	want := []keyspace.Term{outer, inner, loop}
	if count, ok := recurrence.DecisionCount(loop); !ok || count != len(want) {
		t.Fatalf("typed Select stream = %d/%v, want %d/true", count, ok, len(want))
	}
	for index, expected := range want {
		got, ok := recurrence.DecisionAt(loop, index)
		if !ok || got != expected {
			t.Fatalf("typed Select event %d = %v/%v, want %v/true", index, got, ok, expected)
		}
	}
}

func TestSealTraversesBodyRootChildrenWithoutSourcePosition(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 3),
		familyCount(keyspace.FamilyNil, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	child := term(keyspace.FamilyBody, 2)
	loopBody := term(keyspace.FamilyBody, 3)
	loop := term(keyspace.FamilyLoop, 1)
	fixture := openOwnerFixture(t, ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{child}, {loop}, nil},
		nilOwners: []keyspace.Term{child},
		flow: authored.Input{Control: authored.ControlInput{
			Loops: []authored.Loop{{Owner: child, Body: loopBody, Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 1)}},
		}},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	if count, ok := recurrence.DecisionCount(loop); !ok || count != 1 {
		t.Fatalf("nested Body loop stream = %d/%v, want 1/true", count, ok)
	}
}

func TestSealRejectsSameShapeForeignSourceControl(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyNil, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	child := term(keyspace.FamilyBody, 2)
	loop := term(keyspace.FamilyLoop, 1)
	spec := ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Loops: []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 1)}},
		}},
	}
	spec.name = "recurrence-owner-a.lua"
	first := openOwnerFixture(t, spec)
	spec.name = "recurrence-owner-b.lua"
	second := openOwnerFixture(t, spec)
	if _, err := Seal(first.sourceView, first.flow, first.bodies, first.forest, second.graph,
		first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("recurrence accepted a same-shape foreign sourcecontrol proof")
	}
}

func TestSealFailsClosedForUnavailableOwners(t *testing.T) {
	if _, err := Seal(source.View{}, authored.View{}, nil, nil, nil, identity.ContentID{}, identity.ContentID{}); err == nil {
		t.Fatal("recurrence accepted unavailable owners")
	}
}

func TestSealBackwardGotoKeepsEmptyRecurrenceRange(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 1),
		familyCount(keyspace.FamilyLabel, 1),
		familyCount(keyspace.FamilyGoto, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	label := term(keyspace.FamilyLabel, 1)
	gotoTerm := term(keyspace.FamilyGoto, 1)
	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{label, gotoTerm}},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}},
			Gotos:  []authored.Goto{{Owner: parent, Target: label}},
		}},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	if count, ok := recurrence.DecisionCount(label); !ok || count != 0 {
		t.Fatalf("backward Goto head stream = %d/%v, want 0/true", count, ok)
	}
	recurrent := 0
	for index := 0; index < recurrence.ArcCount(); index++ {
		annotation, ok := recurrence.ArcAt(index)
		if !ok || annotation.Head != label {
			continue
		}
		recurrent++
		if count, ok := recurrence.ResetCount(index); !ok || count != 0 {
			t.Fatalf("backward Goto empty reset = %d/%v, want 0/true", count, ok)
		}
		if _, ok := recurrence.ResetAt(index, 0); ok {
			t.Fatal("backward Goto empty reset returned a decision")
		}
	}
	if recurrent != 1 {
		t.Fatalf("backward Goto recurrent Arc count = %d, want 1", recurrent)
	}
}

func TestSealForwardGotoDoesNotCreateRecurrence(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 1),
		familyCount(keyspace.FamilyLabel, 1),
		familyCount(keyspace.FamilyGoto, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	label := term(keyspace.FamilyLabel, 1)
	gotoTerm := term(keyspace.FamilyGoto, 1)
	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{gotoTerm, label}},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}},
			Gotos:  []authored.Goto{{Owner: parent, Target: label}},
		}},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	if count, ok := recurrence.DecisionCount(label); ok || count != 0 {
		t.Fatalf("forward Goto unexpectedly has a head stream = %d/%v", count, ok)
	}
}

func TestSealNestedNumericRangesRemainNested(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 3),
		familyCount(keyspace.FamilyNil, 4),
		familyCount(keyspace.FamilyValues, 2),
		familyCount(keyspace.FamilyCell, 2),
		familyCount(keyspace.FamilyLoop, 2),
	)
	parent := term(keyspace.FamilyBody, 1)
	outerBody := term(keyspace.FamilyBody, 2)
	innerBody := term(keyspace.FamilyBody, 3)
	outer := term(keyspace.FamilyLoop, 1)
	inner := term(keyspace.FamilyLoop, 2)
	nils := []keyspace.Term{term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2), term(keyspace.FamilyNil, 3), term(keyspace.FamilyNil, 4)}
	values := []keyspace.Term{term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)}
	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{outer}, {inner}, nil},
		nilOwners: []keyspace.Term{
			parent,
			parent,
			outerBody,
			outerBody,
		},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: parent, Fixed: authored.Range{End: 2}}, {Owner: outerBody, Fixed: authored.Range{Start: 2, End: 4}}},
				Terms: nils,
			},
			Storage: authored.StorageInput{Cells: []authored.Cell{
				{Kind: authored.CellLocal, Body: outerBody},
				{Kind: authored.CellLocal, Body: innerBody},
			}},
			Control: authored.ControlInput{Cells: []keyspace.Term{term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)}, Loops: []authored.Loop{
				{Owner: parent, Body: outerBody, Kind: kind.LoopNumericFor, Control: values[0], Cells: authored.Range{End: 1}},
				{Owner: outerBody, Body: innerBody, Kind: kind.LoopNumericFor, Control: values[1], Cells: authored.Range{Start: 1, End: 2}},
			}},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	count, ok := recurrence.DecisionCount(outer)
	if !ok || count != 2 {
		t.Fatalf("outer nested stream = %d/%v, want 2/true", count, ok)
	}
	if got, ok := recurrence.DecisionAt(outer, 0); !ok || got != outer {
		t.Fatalf("outer stream first = %v/%v, want %v/true", got, ok, outer)
	}
	if got, ok := recurrence.DecisionAt(outer, 1); !ok || got != inner {
		t.Fatalf("outer stream second = %v/%v, want %v/true", got, ok, inner)
	}
	outerResets, innerResets := 0, 0
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		annotation, annotationOK := recurrence.ArcAt(index)
		if !annotationOK || annotation.Head != outer {
			continue
		}
		count, ok := recurrence.ResetCount(index)
		if !ok {
			t.Fatalf("nested reset %d is not queryable", index)
		}
		if annotation.First == 0 && count == 2 {
			outerResets++
			if !recurrence.ResetContains(index, outer) || !recurrence.ResetContains(index, inner) {
				t.Fatal("outer reset omitted one nested decision")
			}
		} else if count == 1 {
			innerResets++
			if recurrence.ResetContains(index, outer) || !recurrence.ResetContains(index, inner) {
				t.Fatal("inner reset does not isolate the inner decision")
			}
		}
	}
	if outerResets != 1 || innerResets != 1 {
		t.Fatalf("nested reset ranges = outer %d, inner %d; want one each", outerResets, innerResets)
	}
}

func TestSealNumericHeaderSelectIsOutsideResetRange(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyNil, 3),
		familyCount(keyspace.FamilyValues, 1),
		familyCount(keyspace.FamilySelect, 1),
		familyCount(keyspace.FamilyCell, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	child := term(keyspace.FamilyBody, 2)
	values := term(keyspace.FamilyValues, 1)
	selectTerm := term(keyspace.FamilySelect, 1)
	loop := term(keyspace.FamilyLoop, 1)
	nils := []keyspace.Term{term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2)}
	fixture := openOwnerFixture(t, ownerSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop}, nil},
		nilOwners: []keyspace.Term{parent, parent},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: parent, Fixed: authored.Range{End: 2}}},
				Terms: []keyspace.Term{selectTerm, term(keyspace.FamilyNil, 3)},
			},
			Operators: authored.OperatorsInput{Selects: []authored.Select{{
				Owner: parent, Op: kind.SelectAnd, Left: nils[0], Right: nils[1],
			}}},
			Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: child}}},
			Control: authored.ControlInput{Cells: []keyspace.Term{term(keyspace.FamilyCell, 1)}, Loops: []authored.Loop{{
				Owner: parent, Body: child, Kind: kind.LoopNumericFor, Control: values, Cells: authored.Range{End: 1},
			}}},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	count, ok := recurrence.DecisionCount(loop)
	if !ok || count == 0 {
		t.Fatalf("numeric stream = %d/%v, want a Loop decision", count, ok)
	}
	loopRank := -1
	for index := 0; index < count; index++ {
		if got, gotOK := recurrence.DecisionAt(loop, index); gotOK && got == loop {
			loopRank = index
		}
	}
	if loopRank < 0 {
		t.Fatal("numeric stream omitted Loop decision")
	}
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		annotation, annotationOK := recurrence.ArcAt(index)
		if !annotationOK || annotation.Head != loop {
			continue
		}
		reset, resetOK := recurrence.ResetCount(index)
		if !resetOK || reset != count-loopRank {
			t.Fatalf("numeric reset = %d/%v, want %d/true", reset, resetOK, count-loopRank)
		}
		if recurrence.ResetContains(index, selectTerm) {
			t.Fatal("numeric header Select was included in the Loop reset range")
		}
		if !recurrence.ResetContains(index, loop) {
			t.Fatal("numeric reset omitted the Loop decision")
		}
		return
	}
	t.Fatal("numeric recurrence Arc was not found")
}

func TestSealRepeatPlacesBodyBeforeCondition(t *testing.T) {
	counts := countsWith(
		familyCount(keyspace.FamilyBody, 2),
		familyCount(keyspace.FamilyNil, 2),
		familyCount(keyspace.FamilySelect, 1),
		familyCount(keyspace.FamilyLoop, 1),
	)
	parent := term(keyspace.FamilyBody, 1)
	repeatBody := term(keyspace.FamilyBody, 2)
	selectTerm := term(keyspace.FamilySelect, 1)
	repeat := term(keyspace.FamilyLoop, 1)
	fixture := openOwnerFixture(t, ownerSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{repeat}, nil},
		nilOwners: []keyspace.Term{
			repeatBody,
			repeatBody,
		},
		flow: authored.Input{
			Operators: authored.OperatorsInput{Selects: []authored.Select{{Owner: repeatBody, Op: kind.SelectAnd, Left: term(keyspace.FamilyNil, 1), Right: term(keyspace.FamilyNil, 2)}}},
			Control:   authored.ControlInput{Loops: []authored.Loop{{Owner: parent, Body: repeatBody, Kind: kind.LoopRepeat, Control: selectTerm}}},
		},
	})
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	want := []keyspace.Term{selectTerm, repeat}
	count, ok := recurrence.DecisionCount(repeat)
	if !ok || count != len(want) {
		t.Fatalf("repeat stream = %d/%v, want %d/true", count, ok, len(want))
	}
	for index, expected := range want {
		got, ok := recurrence.DecisionAt(repeat, index)
		if !ok || got != expected {
			t.Fatalf("repeat stream %d = %v/%v, want %v/true", index, got, ok, expected)
		}
	}
	for index := 0; index < fixture.graph.ArcCount(); index++ {
		annotation, annotationOK := recurrence.ArcAt(index)
		if !annotationOK || annotation.Head != repeat {
			continue
		}
		if count, ok := recurrence.ResetCount(index); !ok || count != 2 {
			t.Fatalf("repeat reset = %d/%v, want 2/true", count, ok)
		}
		if !recurrence.ResetContains(index, selectTerm) || !recurrence.ResetContains(index, repeat) {
			t.Fatal("repeat reset omitted condition or Loop decision")
		}
		return
	}
	t.Fatal("repeat recurrence Arc was not found")
}
