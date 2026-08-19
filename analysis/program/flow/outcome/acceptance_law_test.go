package outcome

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TestSealReturnStaysInsideFunctionActivation exercises the boundary that is
// easy to get wrong when Return closure follows Body parents. The Return is
// nested in a plain Body inside a Function Body: it reaches the Function Body
// but must not manufacture a Return coordinate in the enclosing lexical Body.
func TestSealReturnStaysInsideFunctionActivation(t *testing.T) {
	counts := outcomeCounts(3, 2, 2, 1, 0, 0, 0, 0, 0, 1)
	counts[keyspace.FamilyCall] = 1
	body1 := outcomeTerm(keyspace.FamilyBody, 1)
	body2 := outcomeTerm(keyspace.FamilyBody, 2)
	body3 := outcomeTerm(keyspace.FamilyBody, 3)
	function := outcomeTerm(keyspace.FamilyFunction, 1)
	values := outcomeTerm(keyspace.FamilyValues, 1)
	returnValues := outcomeTerm(keyspace.FamilyValues, 2)
	returned := outcomeTerm(keyspace.FamilyReturn, 1)
	nilTerm := outcomeTerm(keyspace.FamilyNil, 1)
	nilReturn := outcomeTerm(keyspace.FamilyNil, 2)
	call := outcomeTerm(keyspace.FamilyCall, 1)

	f := openOutcomeFixture(t, outcomeSpec{
		counts: counts,
		// Function→Body supplies Body2's parent; the plain Body term supplies
		// Body3's parent. The Return is therefore owned by Body3.
		rows: [][]keyspace.Term{{call}, {body3}, {returned}},
		flow: authored.Input{
			Counts: counts,
			Values: authored.ValuesInput{
				// The Function occurrence is the sole Values member. Its
				// lexical parent is the Values row; the Return may consume
				// that row from its nested Body without changing Return
				// activation closure.
				Rows: []authored.Value{
					{Owner: body1, Fixed: authored.Range{End: 1}},
					{Owner: body3, Fixed: authored.Range{Start: 1, End: 2}},
				},
				Terms: []keyspace.Term{function, nilReturn},
			},
			Functions: authored.FunctionsInput{
				Rows: []authored.Function{{Owner: body1, Body: body2}},
			},
			Calls: []authored.Call{{Owner: body1, Callee: nilTerm, Actuals: values}},
			Control: authored.ControlInput{
				Returns: []authored.Return{{Owner: body3, Values: returnValues}},
			},
		},
		forms:     sourceFunctionForm(function),
		nilOwners: []keyspace.Term{body1, body3},
	})
	result := f.seal(t)

	returnBody3, ok := result.Find(body3, kind.OutcomeReturn, 0)
	if !ok {
		t.Fatal("Function-nested Return coordinate missing at Body3")
	}
	returnBody2, ok := result.Find(body2, kind.OutcomeReturn, 0)
	if !ok {
		t.Fatal("Function-nested Return did not propagate to Body2")
	}
	if _, ok := result.Find(body1, kind.OutcomeReturn, 0); ok {
		t.Fatal("Function-nested Return crossed into enclosing lexical Body1")
	}
	if got, ok := result.ReturnExit(returned); !ok || got != returnBody3 {
		t.Fatalf("ReturnExit = %v,%v; want Body3 Return %v,true", got, ok, returnBody3)
	}
	if got, ok := result.Propagation(returnBody3); !ok || got != returnBody2 {
		t.Fatalf("Body3 Return propagation = %v,%v; want Body2 Return %v,true", got, ok, returnBody2)
	}
	if got, ok := result.Propagation(returnBody2); ok || got != 0 {
		t.Fatalf("Body2 Return propagation crossed Function activation: %v,%v", got, ok)
	}
	if body, outcomeKind, target, ok := result.Get(returnBody2); !ok || body != body2 || outcomeKind != kind.OutcomeReturn || target != 0 {
		t.Fatalf("Get(Body2 Return) = %v,%v,%v,%v", body, outcomeKind, target, ok)
	}
}

// sourceFunctionForm keeps the fixture's Source-owned Function vocabulary in
// one place without making the test construct a foreign owner projection.
func sourceFunctionForm(function keyspace.Term) []source.FunctionFormals {
	return []source.FunctionFormals{{Function: function}}
}

type controlChainTerms struct {
	bodies   [4]keyspace.Term
	loop     keyspace.Term
	labels   [2]keyspace.Term
	breaks   [2]keyspace.Term
	gotos    [4]keyspace.Term
	values   keyspace.Term
	returned keyspace.Term
}

// openControlChainFixture builds one legitimate four-Body lexical chain. A
// loop owns Body2; Body3 and Body4 are plain nested Bodies. Two Breaks and two
// Gotos per target deliberately share the same semantic request so the
// Outcome seal must deduplicate rows while retaining every occurrence exit.
func openControlChainFixture(t *testing.T, withReturn ...bool) (*outcomeFixture, controlChainTerms) {
	t.Helper()
	includeReturn := len(withReturn) != 0 && withReturn[0]
	terms := controlChainTerms{
		bodies: [4]keyspace.Term{
			outcomeTerm(keyspace.FamilyBody, 1), outcomeTerm(keyspace.FamilyBody, 2),
			outcomeTerm(keyspace.FamilyBody, 3), outcomeTerm(keyspace.FamilyBody, 4),
		},
		loop: outcomeTerm(keyspace.FamilyLoop, 1),
		labels: [2]keyspace.Term{
			outcomeTerm(keyspace.FamilyLabel, 1), outcomeTerm(keyspace.FamilyLabel, 2),
		},
		breaks: [2]keyspace.Term{
			outcomeTerm(keyspace.FamilyBreak, 1), outcomeTerm(keyspace.FamilyBreak, 2),
		},
		gotos: [4]keyspace.Term{
			outcomeTerm(keyspace.FamilyGoto, 1), outcomeTerm(keyspace.FamilyGoto, 2),
			outcomeTerm(keyspace.FamilyGoto, 3), outcomeTerm(keyspace.FamilyGoto, 4),
		},
		values:   outcomeTerm(keyspace.FamilyValues, 1),
		returned: outcomeTerm(keyspace.FamilyReturn, 1),
	}
	counts := outcomeCounts(4, 0, 1, 0, 2, 2, 4, 0, 1, 0)
	if includeReturn {
		counts = outcomeCounts(4, 1, 2, 1, 2, 2, 4, 0, 1, 0)
	}
	rows := [][]keyspace.Term{
		{terms.labels[0], terms.labels[1], terms.loop},
		{terms.bodies[2]},
		{terms.bodies[3]},
		{terms.breaks[0], terms.breaks[1], terms.gotos[0], terms.gotos[1], terms.gotos[2], terms.gotos[3]},
	}
	if includeReturn {
		rows[3] = append(rows[3], terms.returned)
	}
	nilOwners := []keyspace.Term{terms.bodies[0]}
	if includeReturn {
		nilOwners = append(nilOwners, terms.bodies[3])
	}
	flow := authored.Input{
		Counts: counts,
		Control: authored.ControlInput{
			Labels: []authored.Label{{Owner: terms.bodies[0]}, {Owner: terms.bodies[0]}},
			Gotos: []authored.Goto{
				{Owner: terms.bodies[3], Target: terms.labels[0]},
				{Owner: terms.bodies[3], Target: terms.labels[0]},
				{Owner: terms.bodies[3], Target: terms.labels[1]},
				{Owner: terms.bodies[3], Target: terms.labels[1]},
			},
			Breaks: []authored.Break{{Owner: terms.bodies[3]}, {Owner: terms.bodies[3]}},
			Loops: []authored.Loop{{
				Owner: terms.bodies[0], Body: terms.bodies[1], Kind: kind.LoopWhile,
				Control: outcomeTerm(keyspace.FamilyNil, 1),
			}},
		},
	}
	if includeReturn {
		flow.Values = authored.ValuesInput{
			Rows:  []authored.Value{{Owner: terms.bodies[3], Fixed: authored.Range{End: 1}}},
			Terms: []keyspace.Term{outcomeTerm(keyspace.FamilyNil, 2)},
		}
		flow.Control.Returns = []authored.Return{{Owner: terms.bodies[3], Values: terms.values}}
	}
	f := openOutcomeFixture(t, outcomeSpec{
		counts: counts,
		// Loop and labels establish Body1's roots. Body2 is owned by the
		// loop, while the two plain Body terms continue the lexical chain.
		rows:      rows,
		flow:      flow,
		nilOwners: nilOwners,
	})
	return f, terms
}

func TestSealControlChainsPropagateEveryBodyAndDeduplicateOccurrences(t *testing.T) {
	f, terms := openControlChainFixture(t)
	result := f.seal(t)
	if got, want := result.Count(), 25; got != want {
		t.Fatalf("Outcome Count = %d, want %d", got, want)
	}

	// A Break targets the loop Body2. An outward Goto targets Body1's label.
	// Both paths therefore publish the same three Body coordinates: Body4,
	// Body3, and the terminal Body2.
	paths := []struct {
		name   string
		kind   kind.OutcomeKind
		target keyspace.Term
	}{
		{name: "Break", kind: kind.OutcomeBreak, target: terms.loop},
		{name: "Goto label1", kind: kind.OutcomeGoto, target: terms.labels[0]},
		{name: "Goto label2", kind: kind.OutcomeGoto, target: terms.labels[1]},
	}
	for _, path := range paths {
		var rows [3]keyspace.Term
		for index, body := range []keyspace.Term{terms.bodies[3], terms.bodies[2], terms.bodies[1]} {
			row, ok := result.Find(body, path.kind, path.target)
			if !ok {
				t.Fatalf("%s row missing at Body%d", path.name, index+2)
			}
			rows[index] = row
			gotBody, gotKind, gotTarget, ok := result.Get(row)
			if !ok || gotBody != body || gotKind != path.kind || gotTarget != path.target {
				t.Fatalf("%s Get(%v) = %v,%v,%v,%v; want %v,%v,%v,true", path.name, row, gotBody, gotKind, gotTarget, ok, body, path.kind, path.target)
			}
		}
		for index := 0; index < len(rows)-1; index++ {
			got, ok := result.Propagation(rows[index])
			if !ok || got != rows[index+1] {
				t.Fatalf("%s propagation[%d] = %v,%v; want %v,true", path.name, index, got, ok, rows[index+1])
			}
		}
		if got, ok := result.Propagation(rows[2]); ok || got != 0 {
			t.Fatalf("%s terminal propagation = %v,%v; want 0,false", path.name, got, ok)
		}
	}

	breakRow, ok := result.Find(terms.bodies[3], kind.OutcomeBreak, terms.loop)
	if !ok {
		t.Fatal("deduplicated Break row missing")
	}
	for _, occurrence := range terms.breaks {
		if got, ok := result.BreakExit(occurrence); !ok || got != breakRow {
			t.Fatalf("BreakExit(%v) = %v,%v; want shared %v,true", occurrence, got, ok, breakRow)
		}
	}
	for index, occurrence := range terms.gotos {
		label := terms.labels[index/2]
		row, ok := result.Find(terms.bodies[3], kind.OutcomeGoto, label)
		if !ok {
			t.Fatalf("deduplicated Goto row missing for label %v", label)
		}
		if got, ok := result.GotoExit(occurrence); !ok || got != row {
			t.Fatalf("GotoExit(%v) = %v,%v; want shared %v,true", occurrence, got, ok, row)
		}
	}
	if _, ok := result.Find(terms.bodies[0], kind.OutcomeBreak, terms.loop); ok {
		t.Fatal("Break propagated beyond its loop Body")
	}
	if _, ok := result.Find(terms.bodies[0], kind.OutcomeGoto, terms.labels[0]); ok {
		t.Fatal("Goto propagated into its target Body")
	}
}

func TestSealAtEnumeratesExactMixedSemanticTupleOrder(t *testing.T) {
	f, terms := openControlChainFixture(t, true)
	result := f.seal(t)
	want := make([]struct {
		body, target keyspace.Term
		kind         kind.OutcomeKind
	}, 0, result.Count())
	mandatory := []kind.OutcomeKind{kind.OutcomeNormal, kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel}
	for bodyIndex, body := range terms.bodies {
		// At follows the full semantic tuple order, not the order in
		// which the mandatory planes are stored. Break/Goto ordinals lie
		// between Throw and Yield, so insert those targets at that point.
		for _, outcomeKind := range mandatory[:1] {
			want = append(want, struct {
				body, target keyspace.Term
				kind         kind.OutcomeKind
			}{body: body, kind: outcomeKind})
		}
		// The one Return occurrence closes through every Body in this
		// activation, so each Body receives its kind-2 coordinate.
		want = append(want, struct {
			body, target keyspace.Term
			kind         kind.OutcomeKind
		}{body: body, kind: kind.OutcomeReturn})
		for _, outcomeKind := range mandatory[1:2] {
			want = append(want, struct {
				body, target keyspace.Term
				kind         kind.OutcomeKind
			}{body: body, kind: outcomeKind})
		}
		if bodyIndex != 0 {
			want = append(want,
				struct {
					body, target keyspace.Term
					kind         kind.OutcomeKind
				}{body: body, kind: kind.OutcomeBreak, target: terms.loop},
				struct {
					body, target keyspace.Term
					kind         kind.OutcomeKind
				}{body: body, kind: kind.OutcomeGoto, target: terms.labels[0]},
				struct {
					body, target keyspace.Term
					kind         kind.OutcomeKind
				}{body: body, kind: kind.OutcomeGoto, target: terms.labels[1]},
			)
		}
		want = append(want,
			struct {
				body, target keyspace.Term
				kind         kind.OutcomeKind
			}{body: body, kind: mandatory[2]},
			struct {
				body, target keyspace.Term
				kind         kind.OutcomeKind
			}{body: body, kind: mandatory[3]},
		)
	}
	if len(want) != result.Count() {
		t.Fatalf("expected tuple count = %d, Outcome Count = %d", len(want), result.Count())
	}
	for index, expected := range want {
		term, ok := result.At(index)
		if !ok {
			t.Fatalf("At(%d) unavailable", index)
		}
		body, outcomeKind, target, ok := result.Get(term)
		if !ok || body != expected.body || outcomeKind != expected.kind || target != expected.target {
			t.Fatalf("At(%d)=%v Get=%v,%v,%v,%v; want %v,%v,%v,true", index, term, body, outcomeKind, target, ok, expected.body, expected.kind, expected.target)
		}
	}
	if _, ok := result.At(len(want)); ok {
		t.Fatal("At(Count) returned an extra Outcome")
	}
}
