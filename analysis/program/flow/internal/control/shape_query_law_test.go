package control

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func openShapeWithAllControlQueries(t *testing.T) (*Shape, keyspace.Term, keyspace.Term, keyspace.Term) {
	t.Helper()
	counts := controlCounts(2, 0, 1, 0, 0, 1, 1, 1, 1, 0)
	body, child := terms(counts, keyspace.FamilyBody, 1), terms(counts, keyspace.FamilyBody, 2)
	loop := terms(counts, keyspace.FamilyLoop, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	gotoTerm := terms(counts, keyspace.FamilyGoto, 1)
	breakTerm := terms(counts, keyspace.FamilyBreak, 1)
	control := terms(counts, keyspace.FamilyNil, 1)
	input := authored.Input{
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: body}},
			Gotos:  []authored.Goto{{Owner: child, Target: label}},
			Breaks: []authored.Break{{Owner: child}},
			Loops:  []authored.Loop{{Owner: body, Body: child, Kind: kind.LoopWhile, Control: control}},
		},
		Counts: counts,
	}
	f := openShapeFixture(t, shapeSpec{
		counts:    counts,
		rows:      bodyRows([]keyspace.Term{label, loop}, []keyspace.Term{gotoTerm, breakTerm}),
		flow:      input,
		nilOwners: []keyspace.Term{body},
	})
	return f.seal(t), label, gotoTerm, breakTerm
}

func TestShapeQueriesRejectSameFamilyOutOfRange(t *testing.T) {
	shape, _, _, _ := openShapeWithAllControlQueries(t)
	queries := []struct {
		name  string
		term  keyspace.Term
		query func(keyspace.Term) (keyspace.Term, bool)
	}{
		{name: "LabelBody", term: keyspace.MakeTerm(keyspace.FamilyLabel, 2), query: shape.LabelBody},
		{name: "GotoTargetBody", term: keyspace.MakeTerm(keyspace.FamilyGoto, 2), query: shape.GotoTargetBody},
		{name: "BreakLoop", term: keyspace.MakeTerm(keyspace.FamilyBreak, 2), query: shape.BreakLoop},
	}
	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := tc.query(tc.term); ok || got != 0 {
				t.Fatalf("%s(%v) = %v/%v, want zero/false", tc.name, tc.term, got, ok)
			}
		})
	}
}

func TestShapeQueriesDoNotAllocateWithAllControlFamilies(t *testing.T) {
	shape, label, gotoTerm, breakTerm := openShapeWithAllControlQueries(t)
	var labelBody, gotoBody, breakLoop keyspace.Term
	var labelOK, gotoOK, breakOK bool
	allocs := testing.AllocsPerRun(100, func() {
		labelBody, labelOK = shape.LabelBody(label)
		gotoBody, gotoOK = shape.GotoTargetBody(gotoTerm)
		breakLoop, breakOK = shape.BreakLoop(breakTerm)
	})
	if allocs != 0 {
		t.Fatalf("control queries allocated %f times", allocs)
	}
	body := terms(controlCounts(2, 0, 1, 0, 0, 1, 1, 1, 1, 0), keyspace.FamilyBody, 1)
	loop := terms(controlCounts(2, 0, 1, 0, 0, 1, 1, 1, 1, 0), keyspace.FamilyLoop, 1)
	if !labelOK || labelBody != body {
		t.Fatalf("LabelBody = %v/%v, want %v/true", labelBody, labelOK, body)
	}
	if !gotoOK || gotoBody != body {
		t.Fatalf("GotoTargetBody = %v/%v, want %v/true", gotoBody, gotoOK, body)
	}
	if !breakOK || breakLoop != loop {
		t.Fatalf("BreakLoop = %v/%v, want %v/true", breakLoop, breakOK, loop)
	}
}

func TestShapeHandlesDeepNestedScopesAndBodiesIteratively(t *testing.T) {
	const depth = 4097
	counts := controlCounts(depth, 0, depth-1, 0, 0, depth-1, 1, 1, 1, 0)
	root := terms(counts, keyspace.FamilyBody, 1)
	label := terms(counts, keyspace.FamilyLabel, 1)
	gotoTerm := terms(counts, keyspace.FamilyGoto, 1)
	breakTerm := terms(counts, keyspace.FamilyBreak, 1)
	rows := make([][]keyspace.Term, depth)
	loops := make([]authored.Loop, depth-1)
	nilOwners := make([]keyspace.Term, depth-1)
	for index := 0; index < depth-1; index++ {
		owner := terms(counts, keyspace.FamilyBody, uint32(index+1))
		child := terms(counts, keyspace.FamilyBody, uint32(index+2))
		loop := terms(counts, keyspace.FamilyLoop, uint32(index+1))
		control := terms(counts, keyspace.FamilyNil, uint32(index+1))
		rows[index] = []keyspace.Term{loop}
		loops[index] = authored.Loop{Owner: owner, Body: child, Kind: kind.LoopWhile, Control: control}
		nilOwners[index] = owner
	}
	rows[0] = append([]keyspace.Term{label}, rows[0]...)
	deepest := terms(counts, keyspace.FamilyBody, depth)
	rows[depth-1] = []keyspace.Term{gotoTerm, breakTerm}
	input := authored.Input{
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: root}},
			Gotos:  []authored.Goto{{Owner: deepest, Target: label}},
			Breaks: []authored.Break{{Owner: deepest}},
			Loops:  loops,
		},
		Counts: counts,
	}
	f := openShapeFixture(t, shapeSpec{counts: counts, rows: rows, flow: input, nilOwners: nilOwners})
	shape := f.seal(t)
	if got, ok := shape.LabelBody(label); !ok || got != root {
		t.Fatalf("LabelBody(deep) = %v/%v, want %v/true", got, ok, root)
	}
	if got, ok := shape.GotoTargetBody(gotoTerm); !ok || got != root {
		t.Fatalf("GotoTargetBody(deep) = %v/%v, want %v/true", got, ok, root)
	}
	lastLoop := terms(counts, keyspace.FamilyLoop, depth-1)
	if got, ok := shape.BreakLoop(breakTerm); !ok || got != lastLoop {
		t.Fatalf("BreakLoop(deep) = %v/%v, want %v/true", got, ok, lastLoop)
	}
}
