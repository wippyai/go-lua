package outcome

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealCanonicalMandatoryRowsAndBodyRanges(t *testing.T) {
	counts := outcomeCounts(2, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	body1, body2 := outcomeTerm(keyspace.FamilyBody, 1), outcomeTerm(keyspace.FamilyBody, 2)
	f := openOutcomeFixture(t, outcomeSpec{
		counts: counts,
		// A plain nested Body is sufficient for the lexical Body proof. It has
		// no authored control, so this test isolates the four mandatory planes.
		rows: [][]keyspace.Term{{body2}, nil},
		flow: authored.Input{Counts: counts},
	})
	result := f.seal(t)
	if got, want := result.Count(), 8; got != want {
		t.Fatalf("Outcome Count = %d, want %d", got, want)
	}

	wantKinds := [...]kind.OutcomeKind{
		kind.OutcomeNormal, kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel,
	}
	for bodyIndex, body := range []keyspace.Term{body1, body2} {
		start, end, ok := result.BodyRange(body)
		if !ok || end-start != len(wantKinds) || start != bodyIndex*len(wantKinds) {
			t.Fatalf("BodyRange(%v) = %d,%d,%v; want %d,%d,true", body, start, end, ok, bodyIndex*4, bodyIndex*4+4)
		}
		for kindIndex, wantKind := range wantKinds {
			term, ok := result.At(start + kindIndex)
			if !ok {
				t.Fatalf("At(%d) unavailable", start+kindIndex)
			}
			gotBody, gotKind, gotTarget, ok := result.Get(term)
			if !ok || gotBody != body || gotKind != wantKind || gotTarget != 0 {
				t.Fatalf("Get(%v) = %v,%v,%v,%v; want %v,%v,0,true", term, gotBody, gotKind, gotTarget, ok, body, wantKind)
			}
			if found, ok := result.Find(body, wantKind, 0); !ok || found != term {
				t.Fatalf("Find(%v,%v,0) = %v,%v; want %v,true", body, wantKind, found, ok, term)
			}
			if exit, ok := result.BodyExit(body, wantKind); !ok || exit != term {
				t.Fatalf("BodyExit(%v,%v) = %v,%v; want %v,true", body, wantKind, exit, ok, term)
			}
			next, propagated := result.Propagation(term)
			if body == body1 {
				if propagated || next != 0 {
					t.Fatalf("root mandatory %v propagation = %v,%v; want terminal 0,false", term, next, propagated)
				}
			} else if wantKind == kind.OutcomeNormal {
				if propagated || next != 0 {
					t.Fatalf("normal Body2 propagation = %v,%v; want terminal 0,false", next, propagated)
				}
			} else {
				parentExit, parentOK := result.BodyExit(body1, wantKind)
				if !propagated || !parentOK || next != parentExit {
					t.Fatalf("Body2 %v propagation = %v,%v; want %v,true", wantKind, next, propagated, parentExit)
				}
			}
		}
	}
	if _, ok := result.At(-1); ok {
		t.Fatal("At(-1) returned an Outcome")
	}
	if _, ok := result.At(result.Count()); ok {
		t.Fatal("At(Count) returned an Outcome")
	}
}

func TestSealDeduplicatesReturnsAndChainsWithinActivation(t *testing.T) {
	counts := outcomeCounts(2, 2, 2, 2, 0, 0, 0, 0, 0, 0)
	body1, body2 := outcomeTerm(keyspace.FamilyBody, 1), outcomeTerm(keyspace.FamilyBody, 2)
	returned1, returned2 := outcomeTerm(keyspace.FamilyReturn, 1), outcomeTerm(keyspace.FamilyReturn, 2)
	values1, values2 := outcomeTerm(keyspace.FamilyValues, 1), outcomeTerm(keyspace.FamilyValues, 2)
	f := openOutcomeFixture(t, outcomeSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{body2}, {returned1, returned2}},
		flow: authored.Input{
			Counts: counts,
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body2, Fixed: authored.Range{End: 1}}, {Owner: body2, Fixed: authored.Range{Start: 1, End: 2}}},
				Terms: []keyspace.Term{outcomeTerm(keyspace.FamilyNil, 1), outcomeTerm(keyspace.FamilyNil, 2)},
			},
			Control: authored.ControlInput{Returns: []authored.Return{
				{Owner: body2, Values: values1}, {Owner: body2, Values: values2},
			}},
		},
		nilOwners: []keyspace.Term{body2, body2},
	})
	result := f.seal(t)
	if got, want := result.Count(), 10; got != want {
		t.Fatalf("Outcome Count = %d, want %d (one Return key per Body)", got, want)
	}
	returnBody2, ok := result.Find(body2, kind.OutcomeReturn, 0)
	if !ok {
		t.Fatal("Body2 Return key missing")
	}
	returnBody1, ok := result.Find(body1, kind.OutcomeReturn, 0)
	if !ok {
		t.Fatal("Body1 Return key missing from propagation chain")
	}
	for _, returned := range []keyspace.Term{returned1, returned2} {
		exit, ok := result.ReturnExit(returned)
		if !ok || exit != returnBody2 {
			t.Fatalf("ReturnExit(%v) = %v,%v; want shared %v,true", returned, exit, ok, returnBody2)
		}
	}
	if next, ok := result.Propagation(returnBody2); !ok || next != returnBody1 {
		t.Fatalf("Body2 Return propagation = %v,%v; want Body1 Return %v,true", next, ok, returnBody1)
	}
	if next, ok := result.Propagation(returnBody1); ok || next != 0 {
		t.Fatalf("Body1 Return propagation = %v,%v; want terminal 0,false", next, ok)
	}
	if gotBody, gotKind, gotTarget, ok := result.Get(returnBody2); !ok || gotBody != body2 || gotKind != kind.OutcomeReturn || gotTarget != 0 {
		t.Fatalf("Get(Body2 Return) = %v,%v,%v,%v", gotBody, gotKind, gotTarget, ok)
	}
}

func TestSealApplicationOutcomesPropagateButFunctionStopsActivation(t *testing.T) {
	body1, body2 := outcomeTerm(keyspace.FamilyBody, 1), outcomeTerm(keyspace.FamilyBody, 2)
	counts := outcomeCounts(2, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	f := openOutcomeFixture(t, outcomeSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{body2}, nil},
		flow:   authored.Input{Counts: counts},
	})
	result := f.seal(t)
	for _, wantKind := range []kind.OutcomeKind{kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel} {
		child, ok := result.BodyExit(body2, wantKind)
		if !ok {
			t.Fatalf("Body2 %v exit missing", wantKind)
		}
		parent, ok := result.BodyExit(body1, wantKind)
		if !ok {
			t.Fatalf("Body1 %v exit missing", wantKind)
		}
		if next, ok := result.Propagation(child); !ok || next != parent {
			t.Fatalf("Body2 %v propagation = %v,%v; want %v,true", wantKind, next, ok, parent)
		}
	}

	// The same lexical shape becomes a separate activation when Body2 is the
	// Function body. Its application outcomes are visible at that boundary,
	// but never propagate into the enclosing Body.
	function := outcomeTerm(keyspace.FamilyFunction, 1)
	returned := outcomeTerm(keyspace.FamilyReturn, 1)
	values := outcomeTerm(keyspace.FamilyValues, 1)
	functionCounts := counts
	functionCounts[keyspace.FamilyFunction] = 1
	functionCounts[keyspace.FamilyValues] = 1
	functionCounts[keyspace.FamilyReturn] = 1
	f2 := openOutcomeFixture(t, outcomeSpec{
		counts: functionCounts,
		// Function ownership supplies Body2's lexical parent; repeating Body2
		// as a direct Body root would create two parent authorities.
		rows: [][]keyspace.Term{{returned}, nil},
		flow: authored.Input{
			Counts:    functionCounts,
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{function}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body2}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body1, Values: values}}},
		},
		forms: []source.FunctionFormals{{Function: function}},
	})
	result2 := f2.seal(t)
	for _, wantKind := range []kind.OutcomeKind{kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel} {
		child, ok := result2.BodyExit(body2, wantKind)
		if !ok {
			t.Fatalf("Function Body2 %v exit missing", wantKind)
		}
		if next, ok := result2.Propagation(child); ok || next != 0 {
			t.Fatalf("Function Body2 %v crossed activation boundary: %v,%v", wantKind, next, ok)
		}
	}
}

func TestSealCanonicalTargetOrderAndExactOriginBody(t *testing.T) {
	counts := outcomeCounts(2, 0, 1, 0, 1, 2, 2, 0, 1, 0)
	body1, body2 := outcomeTerm(keyspace.FamilyBody, 1), outcomeTerm(keyspace.FamilyBody, 2)
	loop := outcomeTerm(keyspace.FamilyLoop, 1)
	breakTerm := outcomeTerm(keyspace.FamilyBreak, 1)
	label1, label2 := outcomeTerm(keyspace.FamilyLabel, 1), outcomeTerm(keyspace.FamilyLabel, 2)
	goto1, goto2 := outcomeTerm(keyspace.FamilyGoto, 1), outcomeTerm(keyspace.FamilyGoto, 2)
	nilTerm := outcomeTerm(keyspace.FamilyNil, 1)
	f := openOutcomeFixture(t, outcomeSpec{
		counts: counts,
		// Labels are ancestor scope anchors; the loop owns Body2, whose
		// Gotos and Break are the only nonlocal outcomes in that Body.
		rows: [][]keyspace.Term{{label1, label2, loop}, {goto1, goto2, breakTerm}},
		flow: authored.Input{
			Counts: counts,
			Control: authored.ControlInput{
				Labels: []authored.Label{{Owner: body1}, {Owner: body1}},
				Gotos:  []authored.Goto{{Owner: body2, Target: label1}, {Owner: body2, Target: label2}},
				Breaks: []authored.Break{{Owner: body2}},
				Loops:  []authored.Loop{{Owner: body1, Body: body2, Kind: kind.LoopWhile, Control: nilTerm}},
			},
		},
		nilOwners: []keyspace.Term{body1},
	})
	result := f.seal(t)
	if got, want := result.Count(), 11; got != want {
		t.Fatalf("Outcome Count = %d, want %d", got, want)
	}
	breakExit, ok := result.BreakExit(breakTerm)
	if !ok {
		t.Fatal("BreakExit missing")
	}
	if gotBody, gotKind, gotTarget, ok := result.Get(breakExit); !ok || gotBody != body2 || gotKind != kind.OutcomeBreak || gotTarget != loop {
		t.Fatalf("Get(BreakExit) = %v,%v,%v,%v; want %v,%v,%v,true", gotBody, gotKind, gotTarget, ok, body2, kind.OutcomeBreak, loop)
	}
	gotoExit1, ok := result.GotoExit(goto1)
	if !ok || keyspace.TermFamily(gotoExit1) != keyspace.FamilyOutcome {
		t.Fatalf("GotoExit(1) = %v,%v; want Outcome,true", gotoExit1, ok)
	}
	gotoExit2, ok := result.GotoExit(goto2)
	if !ok || keyspace.TermFamily(gotoExit2) != keyspace.FamilyOutcome {
		t.Fatalf("GotoExit(2) = %v,%v; want Outcome,true", gotoExit2, ok)
	}
	if keyspace.TermOrdinal(gotoExit1) >= keyspace.TermOrdinal(gotoExit2) {
		t.Fatalf("Goto target order is not canonical: %v before %v", gotoExit1, gotoExit2)
	}
	for index := 0; index < result.Count(); index++ {
		term, ok := result.At(index)
		if !ok {
			t.Fatalf("At(%d) unavailable", index)
		}
		body, _, _, ok := result.Get(term)
		if !ok || body != body1 && body != body2 {
			t.Fatalf("At(%d) has wrong exact origin Body: %v,%v", index, body, ok)
		}
	}
}

func TestSealSameBodyGotoResumesLabelWithoutOutcome(t *testing.T) {
	counts := outcomeCounts(1, 0, 0, 0, 0, 1, 1, 0, 0, 0)
	body := outcomeTerm(keyspace.FamilyBody, 1)
	label := outcomeTerm(keyspace.FamilyLabel, 1)
	jump := outcomeTerm(keyspace.FamilyGoto, 1)
	f := openOutcomeFixture(t, outcomeSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{label, jump}},
		flow: authored.Input{
			Counts: counts,
			Control: authored.ControlInput{
				Labels: []authored.Label{{Owner: body}},
				Gotos:  []authored.Goto{{Owner: body, Target: label}},
			},
		},
	})
	result := f.seal(t)
	if got, want := result.Count(), 4; got != want {
		t.Fatalf("Outcome Count = %d, want %d", got, want)
	}
	if got, ok := result.GotoExit(jump); !ok || got != label {
		t.Fatalf("GotoExit = %v,%v; want Label %v,true", got, ok, label)
	}
	if got, ok := result.Find(body, kind.OutcomeGoto, label); ok || got != 0 {
		t.Fatalf("same-Body Goto acquired Outcome %v,%v; want 0,false", got, ok)
	}
}

func TestResultQueriesFailClosed(t *testing.T) {
	var result *Result
	body := outcomeTerm(keyspace.FamilyBody, 1)
	term := outcomeTerm(keyspace.FamilyOutcome, 1)
	if result.Count() != 0 {
		t.Fatal("nil Result Count was nonzero")
	}
	if _, _, _, ok := result.Get(term); ok {
		t.Fatal("nil Result Get returned an Outcome")
	}
	if _, ok := result.Find(body, kind.OutcomeNormal, 0); ok {
		t.Fatal("nil Result Find returned an Outcome")
	}
	if _, ok := result.Propagation(term); ok {
		t.Fatal("nil Result Propagation returned an edge")
	}
	if _, ok := result.ReturnExit(outcomeTerm(keyspace.FamilyReturn, 1)); ok {
		t.Fatal("nil Result ReturnExit returned an Outcome")
	}
}
