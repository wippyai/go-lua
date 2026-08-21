package outcome

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func provenanceOutcomeSpec(loopKind kind.LoopKind, reorder bool) outcomeSpec {
	counts := outcomeCounts(2, 0, 1, 0, 1, 1, 1, 0, 1, 0)
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
	return outcomeSpec{
		counts: counts,
		rows:   [][]keyspace.Term{first, {gotoTerm, breakTerm}},
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

func TestOutcomeProvenanceRejectsEqualDenominatorForeignOwners(t *testing.T) {
	current := openOutcomeFixture(t, provenanceOutcomeSpec(kind.LoopWhile, false))
	foreignSource := openOutcomeFixture(t, provenanceOutcomeSpec(kind.LoopWhile, true))
	foreignFlow := openOutcomeFixture(t, provenanceOutcomeSpec(kind.LoopRepeat, false))

	result := current.seal(t)
	sourceID := current.preimage.Identity().ContentID()
	flowID := current.flow.ContentID()
	staticID := current.staticView.ContentID()
	moduleID := current.flow.ModuleID()
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("Outcome did not retain its exact Source/Flow identities")
	}
	foreignSourceID := foreignSource.preimage.Identity().ContentID()
	foreignFlowID := foreignFlow.flow.ContentID()
	if sourceID == foreignSourceID || flowID == foreignFlowID {
		t.Fatal("foreign fixtures did not preserve equal denominators with distinct identities")
	}
	foreignStaticID := staticID
	foreignStaticID[0] ^= 0xff
	foreignModuleID := moduleID
	foreignModuleID[0] ^= 0xff
	if Matches(result, foreignSourceID, flowID, staticID, moduleID) || Matches(result, sourceID, foreignFlowID, staticID, moduleID) ||
		Matches(result, sourceID, flowID, foreignStaticID, moduleID) || Matches(result, sourceID, flowID, staticID, foreignModuleID) {
		t.Fatal("Outcome provenance accepted a foreign owner")
	}

	if _, err := Seal(current.preimage.Identity(), current.flow, foreignSource.bodies, current.shape, staticID, moduleID); err == nil {
		t.Fatal("outcome accepted a foreign Body result")
	}
	if _, err := Seal(current.preimage.Identity(), current.flow, current.bodies, foreignFlow.shape, staticID, moduleID); err == nil {
		t.Fatal("outcome accepted a foreign Shape result")
	}
}

func TestOutcomeProvenanceFailsClosedForNilAndZero(t *testing.T) {
	var nilResult *Result
	if Matches(nilResult, identity.ContentID{0: 1}, identity.ContentID{1: 1}, identity.ContentID{2: 1}, identity.ContentID{3: 1}) {
		t.Fatal("nil Outcome result bypassed Matches")
	}
	current := openOutcomeFixture(t, provenanceOutcomeSpec(kind.LoopWhile, false))
	result := current.seal(t)
	sourceID := current.preimage.Identity().ContentID()
	flowID := current.flow.ContentID()
	staticID := current.staticView.ContentID()
	moduleID := current.flow.ModuleID()
	if Matches(result, identity.ContentID{}, flowID, staticID, moduleID) || Matches(result, sourceID, identity.ContentID{}, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) || Matches(result, sourceID, flowID, staticID, identity.ContentID{}) {
		t.Fatal("Outcome Matches did not fail closed for zero identity")
	}
	zeroValue := *result
	zeroValue.sourceID = identity.ContentID{}
	zeroValue.flowID = identity.ContentID{}
	zero := &zeroValue
	if Matches(zero, sourceID, flowID, staticID, moduleID) {
		t.Fatal("zero-provenance Outcome result bypassed Matches")
	}
	if got := zero.Count(); got != 0 {
		t.Fatalf("zero-provenance Outcome Count = %d, want 0", got)
	}
	if term, ok := zero.At(0); ok || term != 0 {
		t.Fatalf("zero-provenance Outcome At = %v/%v, want 0/false", term, ok)
	}
	if body, outcomeKind, target, ok := zero.Get(resultAt(result, 0)); ok || body != 0 || outcomeKind != 0 || target != 0 {
		t.Fatalf("zero-provenance Outcome Get = %v/%v/%v/%v, want zero/false", body, outcomeKind, target, ok)
	}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if start, end, ok := zero.BodyRange(body); ok || start != 0 || end != 0 {
		t.Fatalf("zero-provenance Outcome BodyRange = %d/%d/%v, want 0/0/false", start, end, ok)
	}
	if term, ok := zero.BodyExit(body, kind.OutcomeNormal); ok || term != 0 {
		t.Fatalf("zero-provenance Outcome BodyExit = %v/%v, want 0/false", term, ok)
	}
	if term, ok := zero.Find(body, kind.OutcomeNormal, 0); ok || term != 0 {
		t.Fatalf("zero-provenance Outcome Find = %v/%v, want 0/false", term, ok)
	}
	if term, ok := zero.Propagation(resultAt(result, 0)); ok || term != 0 {
		t.Fatalf("zero-provenance Outcome Propagation = %v/%v, want 0/false", term, ok)
	}
	if term, ok := zero.ReturnExit(keyspace.MakeTerm(keyspace.FamilyReturn, 1)); ok || term != 0 {
		t.Fatalf("zero-provenance Outcome ReturnExit = %v/%v, want 0/false", term, ok)
	}
	if term, ok := zero.BreakExit(keyspace.MakeTerm(keyspace.FamilyBreak, 1)); ok || term != 0 {
		t.Fatalf("zero-provenance Outcome BreakExit = %v/%v, want 0/false", term, ok)
	}
	if term, ok := zero.GotoExit(keyspace.MakeTerm(keyspace.FamilyGoto, 1)); ok || term != 0 {
		t.Fatalf("zero-provenance Outcome GotoExit = %v/%v, want 0/false", term, ok)
	}
}

func resultAt(result *Result, index int) keyspace.Term {
	if result == nil {
		return 0
	}
	term, _ := result.At(index)
	return term
}
