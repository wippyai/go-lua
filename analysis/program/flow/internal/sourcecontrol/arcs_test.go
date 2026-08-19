package sourcecontrol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func TestSemanticStructuralTransitionsBindAssignCallReturn(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 1},
		familyCount{keyspace.FamilyValues, 4},
		familyCount{keyspace.FamilyNil, 5},
		familyCount{keyspace.FamilyCell, 1},
		familyCount{keyspace.FamilyBind, 1},
		familyCount{keyspace.FamilyAssign, 1},
		familyCount{keyspace.FamilyWrite, 1},
		familyCount{keyspace.FamilyCall, 1},
		familyCount{keyspace.FamilyReturn, 1},
	)
	body := term(keyspace.FamilyBody, 1)
	values := []keyspace.Term{term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2), term(keyspace.FamilyValues, 3), term(keyspace.FamilyValues, 4)}
	nilValues := []keyspace.Term{term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2), term(keyspace.FamilyNil, 3), term(keyspace.FamilyNil, 4), term(keyspace.FamilyNil, 5)}
	cell := term(keyspace.FamilyCell, 1)
	bind := term(keyspace.FamilyBind, 1)
	assign := term(keyspace.FamilyAssign, 1)
	call := term(keyspace.FamilyCall, 1)
	returned := term(keyspace.FamilyReturn, 1)
	f := openSemanticFixture(t, semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, assign, call, returned}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body, Fixed: authored.Range{Start: 2, End: 3}}, {Owner: body, Fixed: authored.Range{Start: 3, End: 4}}},
				Terms: nilValues[:4],
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds:   []authored.Bind{{Owner: body, Values: values[0]}},
				Assigns: []authored.Assign{{Owner: body, Values: values[1]}},
				Writes:  []authored.Write{{Assign: assign, Target: cell}},
			},
			Calls:   []authored.Call{{Owner: body, Callee: nilValues[4], Actuals: values[2]}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values[3]}}},
		},
		nilOwners: []keyspace.Term{body, body, body, body, body},
	})
	result := f.result
	if result.NodeCount() != 5 || result.ArcCount() != 3 {
		t.Fatalf("sequential geometry = nodes %d arcs %d, want 5/3", result.NodeCount(), result.ArcCount())
	}
	bindNode, bindOK := result.Coordinate(f.sourceView, bind)
	assignNode, assignOK := result.Coordinate(f.sourceView, assign)
	callNode, callOK := result.Coordinate(f.sourceView, call)
	returnNode, returnOK := result.Coordinate(f.sourceView, returned)
	if !bindOK || !assignOK || !callOK || !returnOK || bindNode != 0 || assignNode != 1 || callNode != 2 || returnNode != 3 {
		t.Fatalf("root coordinates = bind %d/%v assign %d/%v call %d/%v return %d/%v", bindNode, bindOK, assignNode, assignOK, callNode, callOK, returnNode, returnOK)
	}
	assertArc(t, result, bindNode, assignNode, bind, assign, 0, false)
	assertArc(t, result, assignNode, callNode, assign, call, 0, false)
	assertArc(t, result, callNode, returnNode, call, returned, 0, false)
	if result.ArcCountAtSource(bind) != 1 {
		t.Fatalf("grouped Bind witness count = %d, want 1", result.ArcCountAtSource(bind))
	}
	global, grouped, groupedOK := result.ArcAtSource(bind, 0)
	if !groupedOK || global < 0 || grouped.From != bindNode || grouped.To != assignNode || grouped.Source != bind || grouped.Target != assign {
		t.Fatalf("grouped Bind witness = %d/%#v/%v", global, grouped, groupedOK)
	}
	assertCanonicalArcRows(t, result)
	if result.SuccessorCount(returnNode) != 0 {
		t.Fatalf("Return acquired structural fallthrough: %d successors", result.SuccessorCount(returnNode))
	}
}

func TestSemanticStructuralTransitionBodyEntersChildAndResumesTail(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 2},
		familyCount{keyspace.FamilyNil, 1},
		familyCount{keyspace.FamilyValues, 1},
		familyCount{keyspace.FamilyNil, 1},
		familyCount{keyspace.FamilyReturn, 1},
	)
	parent, child := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2)
	returned, values, nilValue := term(keyspace.FamilyReturn, 1), term(keyspace.FamilyValues, 1), term(keyspace.FamilyNil, 1)
	f := openSemanticFixture(t, semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{child}, {returned}},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: child, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{nilValue}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: child, Values: values}}},
		},
		nilOwners: []keyspace.Term{child},
	})
	result := f.result
	parentRoot, parentRootOK := result.Coordinate(f.sourceView, child)
	childStart, childStartOK := result.Cursor(child, 0)
	childTail, childTailOK := result.Tail(child)
	parentTail, parentTailOK := result.Tail(parent)
	if !parentRootOK || !childStartOK || !childTailOK || !parentTailOK || parentRoot != 0 || childStart != 2 || childTail != 3 || parentTail != 1 {
		t.Fatalf("Body coordinates = root %d/%v child %d/%v tail %d/%v parent tail %d/%v", parentRoot, parentRootOK, childStart, childStartOK, childTail, childTailOK, parentTail, parentTailOK)
	}
	assertArc(t, result, parentRoot, childStart, child, child, 0, false)
	assertArc(t, result, childTail, parentTail, child, parent, 0, false)
}

func TestSemanticStructuralTransitionBranchIncludesEmptyArms(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 3},
		familyCount{keyspace.FamilyNil, 1},
		familyCount{keyspace.FamilyBranch, 1},
	)
	parent := term(keyspace.FamilyBody, 1)
	whenTrue, whenFalse := term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3)
	branch, condition := term(keyspace.FamilyBranch, 1), term(keyspace.FamilyNil, 1)
	f := openSemanticFixture(t, semanticSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{branch}, nil, nil},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{Branches: []authored.Branch{{
			Owner: parent, Condition: condition, WhenTrue: whenTrue, WhenFalse: whenFalse,
		}}}},
	})
	result := f.result
	branchNode, branchOK := result.Coordinate(f.sourceView, branch)
	trueStart, trueOK := result.Cursor(whenTrue, 0)
	falseStart, falseOK := result.Cursor(whenFalse, 0)
	parentTail, parentTailOK := result.Tail(parent)
	trueTail, trueTailOK := result.Tail(whenTrue)
	falseTail, falseTailOK := result.Tail(whenFalse)
	if !branchOK || !trueOK || !falseOK || !parentTailOK || !trueTailOK || !falseTailOK || trueTail != trueStart || falseTail != falseStart {
		t.Fatalf("empty Branch geometry missing: branch=%d/%v true=%d/%v false=%d/%v tails=%d/%d/%d", branchNode, branchOK, trueStart, trueOK, falseStart, falseOK, trueTail, falseTail, parentTail)
	}
	assertArc(t, result, branchNode, trueStart, branch, whenTrue, branch, true)
	assertArc(t, result, branchNode, falseStart, branch, whenFalse, branch, false)
	assertArc(t, result, trueStart, parentTail, whenTrue, parent, 0, false)
	assertArc(t, result, falseStart, parentTail, whenFalse, parent, 0, false)
}

func TestSemanticStructuralTransitionAllLoopForms(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 5},
		familyCount{keyspace.FamilyValues, 2},
		familyCount{keyspace.FamilyNil, 5},
		familyCount{keyspace.FamilyCell, 3},
		familyCount{keyspace.FamilyLoop, 4},
	)
	parent := term(keyspace.FamilyBody, 1)
	children := []keyspace.Term{term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3), term(keyspace.FamilyBody, 4), term(keyspace.FamilyBody, 5)}
	loops := loopTerms()
	nils := []keyspace.Term{term(keyspace.FamilyNil, 1), term(keyspace.FamilyNil, 2), term(keyspace.FamilyNil, 3), term(keyspace.FamilyNil, 4), term(keyspace.FamilyNil, 5)}
	values := []keyspace.Term{term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)}
	cells := []keyspace.Term{term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2), term(keyspace.FamilyCell, 3)}
	f := openSemanticFixture(t, semanticSpec{
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
	result := f.result
	for index, loop := range loops {
		root, rootOK := result.Cursor(parent, uint32(index))
		childStart, childStartOK := result.Cursor(children[index], 0)
		childTail, childTailOK := result.Tail(children[index])
		next, nextOK := result.Cursor(parent, uint32(index+1))
		nextTarget := parent
		if index+1 < len(loops) {
			nextTarget = loops[index+1]
		}
		if !rootOK || !childStartOK || !childTailOK || !nextOK {
			t.Fatalf("loop %d coordinate missing: root=%d/%v child=%d/%v tail=%d/%v next=%d/%v", index, root, rootOK, childStart, childStartOK, childTail, childTailOK, next, nextOK)
		}
		mapped, mappedOK := result.Coordinate(f.sourceView, loop)
		if !mappedOK {
			t.Fatalf("loop %d authored coordinate unavailable", index)
		}
		switch index {
		case 0:
			if mapped != root {
				t.Fatalf("While root coordinate remapped: got %d want %d", mapped, root)
			}
			assertArc(t, result, root, childStart, loop, children[index], loop, true)
			assertArc(t, result, root, next, loop, nextTarget, loop, false)
			assertArc(t, result, childTail, root, children[index], loop, 0, false)
			if _, ok := result.Decision(loop); ok {
				t.Fatal("While loop acquired a hidden decision")
			}
		case 1:
			decision, decisionOK := result.Decision(loop)
			if !decisionOK {
				t.Fatal("Repeat loop lost its hidden decision")
			}
			if mapped != decision || mapped == root {
				t.Fatalf("Repeat root-at-tail frontier = %d/%v, want hidden decision %d distinct from root %d", mapped, mappedOK, decision, root)
			}
			assertArc(t, result, root, childStart, loop, children[index], 0, false)
			assertArc(t, result, childTail, decision, children[index], loop, 0, false)
			// Repeat evaluates its post-body condition: false re-enters the
			// body and true takes the exit.
			assertArc(t, result, decision, childStart, loop, children[index], loop, false)
			assertArc(t, result, decision, next, loop, nextTarget, loop, true)
			// A Repeat emits three witnesses with the exact same Source
			// anchor. ArcAtSource must preserve their canonical global ordinals
			// so recurrence can cite ArcAt without copying the row.
			if result.ArcCountAtSource(loop) != 3 {
				t.Fatalf("grouped Repeat witness count = %d, want 3", result.ArcCountAtSource(loop))
			}
			seen := make(map[int]bool, 3)
			for local := 0; local < result.ArcCountAtSource(loop); local++ {
				global, grouped, groupedOK := result.ArcAtSource(loop, local)
				canonical, canonicalOK := result.ArcAt(global)
				if !groupedOK || !canonicalOK || grouped != canonical || global < 0 || seen[global] {
					t.Fatalf("grouped Repeat witness %d = %d/%#v/%v; canonical = %#v/%v", local, global, grouped, groupedOK, canonical, canonicalOK)
				}
				seen[global] = true
			}
		case 2, 3:
			if mapped != root {
				t.Fatalf("for-loop root coordinate remapped: got %d want %d", mapped, root)
			}
			decision, decisionOK := result.Decision(loop)
			if !decisionOK {
				t.Fatalf("for loop %d lost its hidden decision", index)
			}
			assertArc(t, result, root, decision, loop, loop, 0, false)
			assertArc(t, result, decision, childStart, loop, children[index], loop, true)
			assertArc(t, result, decision, next, loop, nextTarget, loop, false)
			assertArc(t, result, childTail, decision, children[index], loop, 0, false)
		}
	}
	assertCanonicalArcRows(t, result)
}

func TestSemanticRepeatCoordinateDistinguishesChildOccurrenceAndTrailingLabel(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 2},
		familyCount{keyspace.FamilyNil, 1},
		familyCount{keyspace.FamilyValues, 1},
		familyCount{keyspace.FamilyCell, 1},
		familyCount{keyspace.FamilyBind, 1},
		familyCount{keyspace.FamilyLoop, 1},
		familyCount{keyspace.FamilyLabel, 1},
	)
	parent, child := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2)
	loop, bind, label := term(keyspace.FamilyLoop, 1), term(keyspace.FamilyBind, 1), term(keyspace.FamilyLabel, 1)
	nilValue, values, cell := term(keyspace.FamilyNil, 1), term(keyspace.FamilyValues, 1), term(keyspace.FamilyCell, 1)
	f := openSemanticFixture(t, semanticSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop}, {bind, label}},
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{child},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: child}}, Terms: nil},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: child}},
				Binds: []authored.Bind{{Owner: child, Values: values}},
			},
			Control: authored.ControlInput{
				Labels: []authored.Label{{Owner: child}},
				Loops:  []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopRepeat, Control: nilValue}},
			},
		},
	})
	result := f.result
	root, rootOK := result.Cursor(parent, 0)
	childStart, childStartOK := result.Cursor(child, 0)
	childTail, childTailOK := result.Tail(child)
	parentTail, parentTailOK := result.Tail(parent)
	decision, decisionOK := result.Decision(loop)
	loopNode, loopOK := result.Coordinate(f.sourceView, loop)
	bindNode, bindOK := result.Coordinate(f.sourceView, bind)
	labelNode, labelOK := result.Coordinate(f.sourceView, label)
	if !rootOK || !childStartOK || !childTailOK || !parentTailOK || !decisionOK || !loopOK || !bindOK || !labelOK {
		t.Fatalf("Repeat frontier coordinates unavailable: root=%d/%v child=%d/%v childTail=%d/%v parentTail=%d/%v decision=%d/%v loop=%d/%v bind=%d/%v label=%d/%v", root, rootOK, childStart, childStartOK, childTail, childTailOK, parentTail, parentTailOK, decision, decisionOK, loopNode, loopOK, bindNode, bindOK, labelNode, labelOK)
	}
	if loopNode != decision || loopNode == root {
		t.Fatalf("Repeat Loop root did not map to its hidden decision: loop=%d decision=%d ordinary root=%d", loopNode, decision, root)
	}
	if bindNode != childStart || bindNode == decision {
		t.Fatalf("ordinary child Bind was remapped: bind=%d child start=%d decision=%d", bindNode, childStart, decision)
	}
	if labelNode != childTail || labelNode == decision {
		t.Fatalf("trailing child Label was remapped: label=%d child tail=%d decision=%d", labelNode, childTail, decision)
	}
	if parentTail == childTail {
		t.Fatal("parent and child tails unexpectedly share a coordinate")
	}
	assertArc(t, result, root, childStart, loop, child, 0, false)
	assertArc(t, result, childTail, decision, child, loop, 0, false)
	assertArc(t, result, decision, childStart, loop, child, loop, false)
	assertArc(t, result, decision, parentTail, loop, parent, loop, true)
	assertArc(t, result, bindNode, childTail, bind, child, 0, false)
	assertCanonicalArcRows(t, result)
}

func TestSemanticSameCursorLabelsRetainExactGotoTargets(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 1},
		familyCount{keyspace.FamilyGoto, 2},
		familyCount{keyspace.FamilyLabel, 2},
	)
	parent := term(keyspace.FamilyBody, 1)
	goto1, goto2 := term(keyspace.FamilyGoto, 1), term(keyspace.FamilyGoto, 2)
	label1, label2 := term(keyspace.FamilyLabel, 1), term(keyspace.FamilyLabel, 2)
	f := openSemanticFixture(t, semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{goto1, goto2, label1, label2}},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: parent}, {Owner: parent}},
			Gotos:  []authored.Goto{{Owner: parent, Target: label1}, {Owner: parent, Target: label2}},
		}},
	})
	result := f.result
	goto1Node, goto1OK := result.Coordinate(f.sourceView, goto1)
	goto2Node, goto2OK := result.Coordinate(f.sourceView, goto2)
	label1Node, label1OK := result.Coordinate(f.sourceView, label1)
	label2Node, label2OK := result.Coordinate(f.sourceView, label2)
	tail, tailOK := result.Tail(parent)
	if !goto1OK || !goto2OK || !label1OK || !label2OK || !tailOK {
		t.Fatalf("same-cursor Label/Goto coordinates unavailable: goto1=%d/%v goto2=%d/%v label1=%d/%v label2=%d/%v tail=%d/%v", goto1Node, goto1OK, goto2Node, goto2OK, label1Node, label1OK, label2Node, label2OK, tail, tailOK)
	}
	if goto1Node == goto2Node {
		t.Fatalf("distinct Gotos collapsed onto one cursor: %d", goto1Node)
	}
	if label1Node != tail || label2Node != tail || label1Node != label2Node {
		t.Fatalf("Labels did not share trailing cursor: label1=%d label2=%d tail=%d", label1Node, label2Node, tail)
	}
	if result.ArcCount() != 2 {
		t.Fatalf("same-cursor Goto geometry emitted %d arcs, want 2", result.ArcCount())
	}
	assertArc(t, result, goto1Node, label1Node, goto1, label1, 0, false)
	assertArc(t, result, goto2Node, label2Node, goto2, label2, 0, false)
	firstGlobal, first, firstOK := result.ArcAtSource(goto1, 0)
	secondGlobal, second, secondOK := result.ArcAtSource(goto2, 0)
	firstCanonical, firstCanonicalOK := result.ArcAt(firstGlobal)
	secondCanonical, secondCanonicalOK := result.ArcAt(secondGlobal)
	if !firstOK || !secondOK || !firstCanonicalOK || !secondCanonicalOK || first != firstCanonical || second != secondCanonical {
		t.Fatalf("duplicate-coordinate witnesses lost canonical rows: first=%d/%#v/%v second=%d/%#v/%v", firstGlobal, first, firstOK, secondGlobal, second, secondOK)
	}
	if firstGlobal == secondGlobal || first.From == second.From || first.To != second.To || first.Target == second.Target {
		t.Fatalf("duplicate-coordinate Goto witnesses were collapsed: first=%d/%#v second=%d/%#v", firstGlobal, first, secondGlobal, second)
	}
	for _, row := range []struct {
		source keyspace.Term
		from   uint32
		to     uint32
		target keyspace.Term
	}{
		{source: goto1, from: goto1Node, to: label1Node, target: label1},
		{source: goto2, from: goto2Node, to: label2Node, target: label2},
	} {
		if result.ArcCountAtSource(row.source) != 1 {
			t.Fatalf("grouped Goto %v count = %d, want 1", row.source, result.ArcCountAtSource(row.source))
		}
		global, got, ok := result.ArcAtSource(row.source, 0)
		if !ok || global < 0 || got.From != row.from || got.To != row.to || got.Target != row.target {
			t.Fatalf("grouped Goto %v witness = %d/%#v/%v, want From=%d To=%d Target=%v", row.source, global, got, ok, row.from, row.to, row.target)
		}
	}
	assertCanonicalArcRows(t, result)
}

func TestSemanticCoordinateRejectsNilResultAndForeignSource(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 1},
		familyCount{keyspace.FamilyValues, 1},
		familyCount{keyspace.FamilyCell, 1},
		familyCount{keyspace.FamilyBind, 1},
	)
	body, bind, values, cell := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBind, 1), term(keyspace.FamilyValues, 1), term(keyspace.FamilyCell, 1)
	spec := semanticSpec{
		name:   "coordinate-owner.lua",
		counts: counts,
		rows:   [][]keyspace.Term{{bind}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}, Terms: nil},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
		},
	}
	first := openSemanticFixture(t, spec)
	foreignSpec := spec
	foreignSpec.name = "foreign-coordinate-owner.lua"
	foreign := openSemanticFixture(t, foreignSpec)
	if first.sourceView.Identity().ContentID() == foreign.sourceView.Identity().ContentID() {
		t.Fatal("foreign Source fixtures unexpectedly share ContentID")
	}
	var nilResult *Result
	if got, ok := nilResult.Coordinate(first.sourceView, bind); ok || got != 0 {
		t.Fatalf("nil Result.Coordinate accepted: %d/%v", got, ok)
	}
	if got, ok := first.result.Coordinate(foreign.sourceView, bind); ok || got != 0 {
		t.Fatalf("foreign Source.Coordinate accepted: %d/%v", got, ok)
	}
	if got, ok := first.result.Coordinate(first.sourceView, bind); !ok || got != 0 {
		t.Fatalf("same Source.Coordinate rejected or moved: %d/%v", got, ok)
	}
}

func TestSemanticStructuralBreakGotoAndTrailingLabel(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 2},
		familyCount{keyspace.FamilyNil, 1},
		familyCount{keyspace.FamilyLoop, 1},
		familyCount{keyspace.FamilyBreak, 1},
		familyCount{keyspace.FamilyGoto, 1},
		familyCount{keyspace.FamilyLabel, 1},
	)
	parent, child := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2)
	loop, breaker := term(keyspace.FamilyLoop, 1), term(keyspace.FamilyBreak, 1)
	jmp, label := term(keyspace.FamilyGoto, 1), term(keyspace.FamilyLabel, 1)
	f := openSemanticFixture(t, semanticSpec{
		counts:    counts,
		rows:      [][]keyspace.Term{{loop, jmp, label}, {breaker}},
		nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{
			Breaks: []authored.Break{{Owner: child}},
			Labels: []authored.Label{{Owner: parent}},
			Gotos:  []authored.Goto{{Owner: parent, Target: label}},
			Loops:  []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopWhile, Control: term(keyspace.FamilyNil, 1)}},
		}},
	})
	result := f.result
	loopNode, loopOK := result.Cursor(parent, 0)
	breakNode, breakOK := result.Coordinate(f.sourceView, breaker)
	gotoNode, gotoOK := result.Coordinate(f.sourceView, jmp)
	labelNode, labelOK := result.Coordinate(f.sourceView, label)
	parentTail, parentTailOK := result.Tail(parent)
	childTail, childTailOK := result.Tail(child)
	if !loopOK || !breakOK || !gotoOK || !labelOK || !parentTailOK || !childTailOK || labelNode != parentTail {
		t.Fatalf("Label/trailing coordinates loop=%d/%v break=%d/%v goto=%d/%v label=%d/%v parent tail=%d/%v", loopNode, loopOK, breakNode, breakOK, gotoNode, gotoOK, labelNode, labelOK, parentTail, parentTailOK)
	}
	loopNext, nextOK := result.Cursor(parent, 1)
	if !nextOK {
		t.Fatal("loop post-root cursor unavailable")
	}
	assertArc(t, result, breakNode, loopNext, breaker, loop, 0, false)
	assertArc(t, result, gotoNode, labelNode, jmp, label, 0, false)
	assertArc(t, result, loopNode, resultMustStart(t, result, child), loop, child, loop, true)
	assertArc(t, result, childTail, loopNode, child, loop, 0, false)
	if result.SuccessorCount(labelNode) != 0 {
		t.Fatal("trailing Label acquired an implicit fallthrough arc")
	}
}

func resultMustStart(t *testing.T, result *Result, body keyspace.Term) uint32 {
	t.Helper()
	start, ok := result.Cursor(body, 0)
	if !ok {
		t.Fatalf("Cursor(%v, 0) unavailable", body)
	}
	return start
}

func TestSemanticStaticRootExcludedWithoutAdvancingCursor(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 1},
		familyCount{keyspace.FamilyBind, 1},
		familyCount{keyspace.FamilyValues, 1},
		familyCount{keyspace.FamilyCell, 1},
		familyCount{keyspace.FamilyTypeAlias, 1},
		familyCount{keyspace.FamilyTypePrimitive, 1},
	)
	body, bind, values, cell := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBind, 1), term(keyspace.FamilyValues, 1), term(keyspace.FamilyCell, 1)
	alias, primitive := term(keyspace.FamilyTypeAlias, 1), term(keyspace.FamilyTypePrimitive, 1)
	aliasCoordinate, coordinateOK := source.CoordinateFromParts(1, 1, 1, 2)
	if !coordinateOK {
		t.Fatal("source.CoordinateFromParts rejected alias coordinate")
	}
	f := openSemanticFixture(t, semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{alias, bind}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		static: static.Input{
			Types:        statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveString}}},
			Declarations: staticdecl.Input{Alias: []staticdecl.TypeAlias{{Owner: body, Target: primitive, Name: 1, NameCoordinate: aliasCoordinate}}},
		},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
		},
	})
	result := f.result
	bindNode, bindOK := result.Coordinate(f.sourceView, bind)
	if !bindOK || bindNode != 0 {
		t.Fatalf("dynamic Bind moved across static Alias: %d/%v", bindNode, bindOK)
	}
	// Static membership is owned by containment, not by sourcecontrol.
	// The Alias may project to the same Source cursor, but it must create no
	// coordinate or arc of its own and must not advance the dynamic Bind.
	tailNode, tailOK := result.Tail(body)
	if !tailOK || tailNode != 1 {
		t.Fatalf("static Alias advanced Body cursor: tail=%d/%v", tailNode, tailOK)
	}
	if result.ArcCount() != 1 {
		t.Fatalf("static root changed dynamic structural arc count: %d", result.ArcCount())
	}
	assertArc(t, result, bindNode, tailNode, bind, body, 0, false)
}

func TestSemanticFunctionBodiesReachNestedClosuresWithoutTopologyArcs(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 3},
		familyCount{keyspace.FamilyValues, 2},
		familyCount{keyspace.FamilyFunction, 2},
		familyCount{keyspace.FamilyBind, 2},
		familyCount{keyspace.FamilyCell, 2},
	)
	body1, body2, body3 := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3)
	bind1, bind2 := term(keyspace.FamilyBind, 1), term(keyspace.FamilyBind, 2)
	cell1, cell2 := term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)
	values1, values2 := term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2)
	function1, function2 := term(keyspace.FamilyFunction, 1), term(keyspace.FamilyFunction, 2)
	f := openSemanticFixture(t, semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind1}, {bind2}, nil},
		binds:  []source.BindCells{{Bind: bind1, Cells: []keyspace.Term{cell1}}, {Bind: bind2, Cells: []keyspace.Term{cell2}}},
		forms:  []source.FunctionFormals{{Function: function1}, {Function: function2}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body2, Fixed: authored.Range{Start: 1, End: 2}}},
				Terms: []keyspace.Term{function1, function2},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body1}, {Kind: authored.CellLocal, Body: body2}},
				Binds: []authored.Bind{{Owner: body1, Values: values1}, {Owner: body2, Values: values2}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{
				{Owner: body1, Body: body2},
				{Owner: body2, Body: body3},
			}},
		},
	})
	result := f.result
	entryStart := resultMustStart(t, result, body1)
	closure1, closure1OK := result.Coordinate(f.sourceView, function1)
	closure2, closure2OK := result.Coordinate(f.sourceView, function2)
	body2Start := resultMustStart(t, result, body2)
	body3Start := resultMustStart(t, result, body3)
	if !closure1OK || !closure2OK {
		t.Fatalf("closure frontiers unavailable: %d/%v %d/%v", closure1, closure1OK, closure2, closure2OK)
	}
	if !result.Reachable(entryStart) || !result.Reachable(body2Start) || !result.Reachable(body3Start) {
		t.Fatalf("function Body reachability lost: entry=%v body2=%v body3=%v", result.Reachable(entryStart), result.Reachable(body2Start), result.Reachable(body3Start))
	}
	if result.ArcCount() != 2 {
		t.Fatalf("function availability leaked into structural Arc count: %d", result.ArcCount())
	}
	bind1Node, bind1OK := result.Coordinate(f.sourceView, bind1)
	bind2Node, bind2OK := result.Coordinate(f.sourceView, bind2)
	bind1Tail, bind1TailOK := result.Tail(body1)
	bind2Tail, bind2TailOK := result.Tail(body2)
	if !bind1OK || !bind2OK || !bind1TailOK || !bind2TailOK {
		t.Fatal("function structural coordinates unavailable")
	}
	assertArc(t, result, bind1Node, bind1Tail, bind1, body1, 0, false)
	assertArc(t, result, bind2Node, bind2Tail, bind2, body2, 0, false)
	assertNoTopologyArc(t, result, closure1, body2Start)
	assertNoTopologyArc(t, result, closure2, body3Start)
	if !result.Dominates(entryStart, entryStart) || !result.Dominates(body2Start, body2Start) || !result.Dominates(body3Start, body3Start) {
		t.Fatal("activation roots lost reflexive dominance")
	}
	if result.Dominates(entryStart, body2Start) || result.Dominates(entryStart, body3Start) || result.Dominates(body2Start, body3Start) {
		t.Fatal("independent function Body roots were incorrectly dominated by another activation")
	}
}

func TestSemanticCanonicalEnumerationIsDeterministic(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 3},
		familyCount{keyspace.FamilyBranch, 1},
		familyCount{keyspace.FamilyNil, 1},
	)
	parent, whenTrue, whenFalse := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2), term(keyspace.FamilyBody, 3)
	branch, condition := term(keyspace.FamilyBranch, 1), term(keyspace.FamilyNil, 1)
	spec := semanticSpec{
		counts: counts, rows: [][]keyspace.Term{{branch}, nil, nil}, nilOwners: []keyspace.Term{parent},
		flow: authored.Input{Control: authored.ControlInput{Branches: []authored.Branch{{Owner: parent, Condition: condition, WhenTrue: whenTrue, WhenFalse: whenFalse}}}},
	}
	first, second := openSemanticFixture(t, spec), openSemanticFixture(t, spec)
	if first.result.NodeCount() != second.result.NodeCount() || first.result.ArcCount() != second.result.ArcCount() {
		t.Fatalf("canonical denominators changed: nodes=%d/%d arcs=%d/%d", first.result.NodeCount(), second.result.NodeCount(), first.result.ArcCount(), second.result.ArcCount())
	}
	for index := 0; index < first.result.ArcCount(); index++ {
		left, leftOK := first.result.ArcAt(index)
		right, rightOK := second.result.ArcAt(index)
		if leftOK != rightOK || (leftOK && left != right) {
			t.Fatalf("ArcAt(%d) changed: %#v/%v vs %#v/%v", index, left, leftOK, right, rightOK)
		}
	}
	for _, term := range []keyspace.Term{branch, whenTrue, whenFalse} {
		left, leftOK := first.result.Coordinate(first.sourceView, term)
		right, rightOK := second.result.Coordinate(second.sourceView, term)
		if leftOK != rightOK || (leftOK && left != right) {
			t.Fatalf("Coordinate(%v) changed: %d/%v vs %d/%v", term, left, leftOK, right, rightOK)
		}
	}
	for node := uint32(0); node < first.result.NodeCount(); node++ {
		if first.result.Reachable(node) != second.result.Reachable(node) {
			t.Fatalf("Reachable(%d) changed", node)
		}
		for other := uint32(0); other < first.result.NodeCount(); other++ {
			if first.result.Dominates(node, other) != second.result.Dominates(node, other) {
				t.Fatalf("Dominates(%d,%d) changed", node, other)
			}
		}
	}
	assertCanonicalArcRows(t, first.result)
}

func TestSemanticRejectsMalformedOwners(t *testing.T) {
	counts := countsWith(familyCount{keyspace.FamilyBody, 1})
	first := openSemanticFixture(t, semanticSpec{counts: counts, rows: [][]keyspace.Term{{}}})
	entry := term(keyspace.FamilyBody, 1)
	cases := []struct {
		name string
		call func() error
	}{
		{name: "zero Source owner", call: func() error {
			_, err := Seal(source.View{}, first.flow, first.bodies, first.forest, first.shape, entry,
				first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
			return err
		}},
		{name: "zero Flow owner", call: func() error {
			_, err := Seal(first.sourceView, authored.View{}, first.bodies, first.forest, first.shape, entry,
				first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
			return err
		}},
		{name: "nil Body proof", call: func() error {
			_, err := Seal(first.sourceView, first.flow, nil, first.forest, first.shape, entry,
				first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
			return err
		}},
		{name: "nil containment proof", call: func() error {
			_, err := Seal(first.sourceView, first.flow, first.bodies, nil, first.shape, entry,
				first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
			return err
		}},
		{name: "nil control proof", call: func() error {
			_, err := Seal(first.sourceView, first.flow, first.bodies, first.forest, nil, entry,
				first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
			return err
		}},
		{name: "foreign Entry", call: func() error {
			_, err := Seal(first.sourceView, first.flow, first.bodies, first.forest, first.shape, term(keyspace.FamilyBody, 2),
				first.staticFinalize.View().ContentID(), first.moduleFinalize.View().ContentID())
			return err
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil {
				t.Fatal("malformed or foreign owner was accepted")
			}
		})
	}
}
