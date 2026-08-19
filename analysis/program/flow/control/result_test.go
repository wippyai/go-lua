package control

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// provenanceShapeSpec keeps the source denominator fixed while changing only
// the authored order or loop kind. Both variants are honest seals; the
// identity fence, rather than a cardinality check, must distinguish them.
func provenanceShapeSpec(loopKind kind.LoopKind, reorder bool) shapeSpec {
	counts := controlCounts(2, 0, 1, 0, 0, 1, 1, 1, 1, 0)
	body, child := keyspace.MakeTerm(keyspace.FamilyBody, 1), keyspace.MakeTerm(keyspace.FamilyBody, 2)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	gotoTerm := keyspace.MakeTerm(keyspace.FamilyGoto, 1)
	breakTerm := keyspace.MakeTerm(keyspace.FamilyBreak, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	nilTerm := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	first := []keyspace.Term{label, loop}
	if reorder {
		first = []keyspace.Term{loop, label}
	}
	nilOwner := body
	if loopKind == kind.LoopRepeat {
		nilOwner = child
	}
	return shapeSpec{
		counts: counts,
		rows:   bodyRows(first, []keyspace.Term{gotoTerm, breakTerm}),
		flow: authored.Input{
			Counts: counts,
			Control: authored.ControlInput{
				Labels: []authored.Label{{Owner: body}},
				Gotos:  []authored.Goto{{Owner: child, Target: label}},
				Breaks: []authored.Break{{Owner: child}},
				Loops:  []authored.Loop{{Owner: body, Body: child, Kind: loopKind, Control: nilTerm}},
			},
		},
		nilOwners: []keyspace.Term{nilOwner},
	}
}

func TestShapeProvenanceRejectsEqualDenominatorForeignOwners(t *testing.T) {
	current := openShapeFixture(t, provenanceShapeSpec(kind.LoopWhile, false))
	foreignSource := openShapeFixture(t, provenanceShapeSpec(kind.LoopWhile, true))
	foreignFlow := openShapeFixture(t, provenanceShapeSpec(kind.LoopRepeat, false))

	shape := current.seal(t)
	sourceID := current.preimage.Identity().ContentID()
	flowID := current.flow.Cold().ContentID()
	staticID := current.staticView.ContentID()
	moduleID := current.moduleFinalize.View().ContentID()
	if !Matches(shape, sourceID, flowID, staticID, moduleID) {
		t.Fatal("Shape did not retain its exact Source/Flow identities")
	}
	if sourceID == foreignSource.preimage.Identity().ContentID() ||
		flowID == foreignFlow.flow.Cold().ContentID() {
		t.Fatal("foreign fixtures did not preserve equal denominators with distinct identities")
	}
	foreignStatic := staticID
	foreignStatic[0] ^= 0xff
	foreignModule := moduleID
	foreignModule[0] ^= 0xff
	if Matches(shape, foreignSource.preimage.Identity().ContentID(), flowID, staticID, moduleID) ||
		Matches(shape, sourceID, foreignFlow.flow.Cold().ContentID(), staticID, moduleID) ||
		Matches(shape, sourceID, flowID, foreignStatic, moduleID) ||
		Matches(shape, sourceID, flowID, staticID, foreignModule) ||
		Matches(foreignSource.seal(t), sourceID, flowID, staticID, moduleID) {
		t.Fatal("Shape provenance accepted a foreign owner")
	}

	if _, err := Seal(current.preimage, current.flow, foreignSource.bodies, current.binding, current.forest, staticID, moduleID); err == nil {
		t.Fatal("control accepted a foreign Body result")
	}
	if _, err := Seal(current.preimage, current.flow, current.bodies, current.binding, foreignSource.forest, staticID, moduleID); err == nil {
		t.Fatal("control accepted a foreign Containment result")
	}
	if _, err := Seal(current.preimage, current.flow, current.bodies, foreignFlow.binding, current.forest, staticID, moduleID); err == nil {
		t.Fatal("control accepted a foreign Binding result")
	}
}

func TestShapeProvenanceFailsClosedForNilAndZero(t *testing.T) {
	counts := controlCounts(1, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	shape := openShapeFixture(t, shapeSpec{
		counts: counts,
		rows:   bodyRows(nil),
		flow:   authored.Input{Counts: counts},
	}).seal(t)
	sourceID := shape.sourceID
	flowID := shape.flowID
	staticID := shape.staticID
	moduleID := shape.moduleID
	if Matches(nil, sourceID, flowID, staticID, moduleID) || Matches(shape, identity.ContentID{}, flowID, staticID, moduleID) ||
		Matches(shape, sourceID, identity.ContentID{}, staticID, moduleID) ||
		Matches(shape, sourceID, flowID, identity.ContentID{}, moduleID) || Matches(shape, sourceID, flowID, staticID, identity.ContentID{}) {
		t.Fatal("Shape Matches did not fail closed for nil or zero identity")
	}
	zero := &Shape{labelBody: shape.labelBody, breakLoop: shape.breakLoop, gotoTargetBody: shape.gotoTargetBody}
	if Matches(zero, sourceID, flowID, staticID, moduleID) {
		t.Fatal("zero-provenance Shape bypassed Matches")
	}
	if got, ok := zero.LabelBody(keyspace.MakeTerm(keyspace.FamilyLabel, 1)); ok || got != 0 {
		t.Fatalf("zero-provenance Shape query = %v/%v, want zero/false", got, ok)
	}
}
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
