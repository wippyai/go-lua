package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSemanticMatrixBackwardGotoCarriesLoopReset(t *testing.T) {
	parent := causalTerm(keyspace.FamilyBody, 1)
	loopBody := causalTerm(keyspace.FamilyBody, 2)
	gotoArm := causalTerm(keyspace.FamilyBody, 3)
	fallthroughArm := causalTerm(keyspace.FamilyBody, 4)
	loop := causalTerm(keyspace.FamilyLoop, 1)
	branch := causalTerm(keyspace.FamilyBranch, 1)
	gotoTerm := causalTerm(keyspace.FamilyGoto, 1)
	label := causalTerm(keyspace.FamilyLabel, 1)
	loopCondition := causalTerm(keyspace.FamilyNil, 1)
	branchCondition := causalTerm(keyspace.FamilyNil, 2)
	f := openCausalFixture(t, causalSpec{
		counts: causalCounts(
			causalFamilyCount{keyspace.FamilyBody, 4},
			causalFamilyCount{keyspace.FamilyLoop, 1},
			causalFamilyCount{keyspace.FamilyBranch, 1},
			causalFamilyCount{keyspace.FamilyGoto, 1},
			causalFamilyCount{keyspace.FamilyLabel, 1},
			causalFamilyCount{keyspace.FamilyNil, 2},
		),
		rows:      [][]keyspace.Term{{loop}, {label, branch}, {gotoTerm}, nil},
		nilOwners: []keyspace.Term{parent, loopBody},
		captureArcs: []causalArcSelector{
			{Source: gotoTerm, Target: label},
			{Source: loopBody, Target: loop},
		},
		flow: authored.Input{Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: loopBody}},
			Gotos:  []authored.Goto{{Owner: gotoArm, Target: label}},
			Branches: []authored.Branch{{
				Owner: loopBody, Condition: branchCondition, WhenTrue: gotoArm, WhenFalse: fallthroughArm,
			}},
			Loops: []authored.Loop{{Owner: parent, Body: loopBody, Kind: kind.LoopWhile, Control: loopCondition}},
		}},
	})

	if got := f.result.Successors().Count(gotoTerm); got == 0 {
		t.Fatal("backward Goto has no causal successors")
	}
	normal, normalOK := f.outcomes.BodyExit(loopBody, kind.OutcomeNormal)
	if !normalOK {
		t.Fatal("nested Body Normal Outcome is absent")
	}
	gotoExit, gotoExitOK := f.outcomes.GotoExit(gotoTerm)
	if !gotoExitOK || keyspace.TermFamily(gotoExit) != keyspace.FamilyOutcome {
		t.Fatalf("outward backward Goto Exit = %v/%v, want typed Outcome", gotoExit, gotoExitOK)
	}
	gotoOwner, gotoKind, gotoTarget, gotoRowOK := f.outcomes.Get(gotoExit)
	if !gotoRowOK || gotoOwner != gotoArm || gotoKind != kind.OutcomeGoto || gotoTarget != label {
		t.Fatalf("outward backward Goto Outcome = %v/%v/%v/%v, want %v/%v/%v/true", gotoOwner, gotoKind, gotoTarget, gotoRowOK, gotoArm, kind.OutcomeGoto, label)
	}
	parentGoto, propagated := f.outcomes.Propagation(gotoExit)
	if !propagated {
		t.Fatal("outward backward Goto Outcome did not propagate to loop Body")
	}
	parentOwner, parentKind, parentTarget, parentRowOK := f.outcomes.Get(parentGoto)
	if !parentRowOK || parentOwner != loopBody || parentKind != kind.OutcomeGoto || parentTarget != label {
		t.Fatalf("outward backward Goto parent Outcome = %v/%v/%v/%v, want %v/%v/%v/true", parentOwner, parentKind, parentTarget, parentRowOK, loopBody, kind.OutcomeGoto, label)
	}
	if len(f.capturedArcs) != 2 {
		t.Fatalf("backward Goto Arc capture count = %d, want 2", len(f.capturedArcs))
	}
	gotoArc := f.capturedArcs[0]
	if gotoArc.ordinal < 0 || gotoArc.arc.Source != gotoTerm || gotoArc.arc.Target != label || gotoArc.arc.Decision != 0 || gotoArc.arc.Truth {
		t.Fatalf("backward Goto sourcecontrol Arc snapshot = %#v", gotoArc)
	}
	gotoAnnotation, annotationOK := f.recurrence.ArcAt(gotoArc.ordinal)
	if !annotationOK || gotoAnnotation.Head != label {
		t.Fatalf("backward Goto recurrence annotation = %#v/%v, want Label Mu", gotoAnnotation, annotationOK)
	}
	gotoEdge := -1
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, edgeOK := f.result.Edges().At(index)
		if edgeOK && edge.From == gotoTerm && edge.To == gotoExit && edge.Mu == label {
			gotoEdge = index
			break
		}
	}
	if gotoEdge < 0 {
		t.Fatalf("backward Goto causal Mu edge is absent: %v -> %v", gotoTerm, gotoExit)
	}
	wantGotoReset, wantGotoResetOK := f.recurrence.ResetCount(gotoArc.ordinal)
	gotGotoReset, gotGotoResetOK := f.result.Edges().ResetCount(gotoEdge)
	if !wantGotoResetOK || !gotGotoResetOK || gotGotoReset != wantGotoReset {
		t.Fatalf("backward Goto reset count = %d/%v, want recurrence %d/%v", gotGotoReset, gotGotoResetOK, wantGotoReset, wantGotoResetOK)
	}
	for offset := 0; offset < wantGotoReset; offset++ {
		wantDecision, wantDecisionOK := f.recurrence.ResetAt(gotoArc.ordinal, offset)
		gotDecision, gotDecisionOK := f.result.Edges().ResetAt(gotoEdge, offset)
		if !wantDecisionOK || !gotDecisionOK || gotDecision != wantDecision || !f.result.Edges().ResetContains(gotoEdge, gotDecision) {
			t.Fatalf("backward Goto reset[%d] = %v/%v, want %v/%v and membership", offset, gotDecision, gotDecisionOK, wantDecision, wantDecisionOK)
		}
	}
	propagationEdge := -1
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, edgeOK := f.result.Edges().At(index)
		if edgeOK && edge.From == gotoExit && edge.To == parentGoto {
			propagationEdge = index
			if edge.Mu != 0 {
				t.Fatalf("carrierless outward Goto propagation retained Mu: %#v", edge)
			}
			if _, resetOK := f.result.Edges().ResetCount(index); resetOK {
				t.Fatalf("carrierless outward Goto propagation retained reset: %#v", edge)
			}
			break
		}
	}
	if propagationEdge < 0 {
		t.Fatalf("outward Goto propagation causal edge is absent: %v -> %v", gotoExit, parentGoto)
	}
	loopArc := f.capturedArcs[1]
	if loopArc.ordinal < 0 || loopArc.arc.Source != loopBody || loopArc.arc.Target != loop || loopArc.arc.Decision != 0 || loopArc.arc.Truth {
		t.Fatalf("backward loop sourcecontrol Arc snapshot = %#v", loopArc)
	}
	loopAnnotation, annotationOK := f.recurrence.ArcAt(loopArc.ordinal)
	if !annotationOK || loopAnnotation.Head != label {
		t.Fatalf("backward loop recurrence annotation = %#v/%v", loopAnnotation, annotationOK)
	}
	conditionEntry, conditionEntryOK := f.ports.Entry(loopCondition)
	if !conditionEntryOK {
		t.Fatal("backward loop condition Entry is absent")
	}
	loopEdge := -1
	for index := 0; index < f.result.Edges().Count(); index++ {
		edge, edgeOK := f.result.Edges().At(index)
		if edgeOK && edge.From == normal && edge.To == conditionEntry && edge.Mu == label {
			loopEdge = index
			break
		}
	}
	if loopEdge < 0 {
		t.Fatalf("backward loop causal Mu edge is absent: %v -> %v", normal, conditionEntry)
	}
	wantReset, wantResetOK := f.recurrence.ResetCount(loopArc.ordinal)
	gotReset, gotResetOK := f.result.Edges().ResetCount(loopEdge)
	if !wantResetOK || !gotResetOK || gotReset != wantReset {
		t.Fatalf("backward loop reset count = %d/%v, want recurrence %d/%v", gotReset, gotResetOK, wantReset, wantResetOK)
	}
	for offset := 0; offset < wantReset; offset++ {
		wantDecision, wantDecisionOK := f.recurrence.ResetAt(loopArc.ordinal, offset)
		gotDecision, gotDecisionOK := f.result.Edges().ResetAt(loopEdge, offset)
		if !wantDecisionOK || !gotDecisionOK || gotDecision != wantDecision || !f.result.Edges().ResetContains(loopEdge, gotDecision) {
			t.Fatalf("backward loop reset[%d] = %v/%v, want %v/%v and membership", offset, gotDecision, gotDecisionOK, wantDecision, wantDecisionOK)
		}
	}
}
