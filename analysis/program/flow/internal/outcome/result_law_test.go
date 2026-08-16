package outcome

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// resultFixture is deliberately assembled from the private Result fields. It
// is a law fixture, not a second builder: production sealing owns the only
// construction path. Keeping the rows explicit makes the canonical key order
// and every query's expected coordinate visible in one small test relation.
type resultFixture struct {
	result *Result

	body1, body2    keyspace.Term
	loop1, loop2    keyspace.Term
	label1, label2  keyspace.Term
	return1, break1 keyspace.Term
	goto1, goto2    keyspace.Term

	rows []outcomeRowExpectation
}

type outcomeRowExpectation struct {
	body, target keyspace.Term
	kind         kind.OutcomeKind
}

func canonicalResultFixture() resultFixture {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	loop1 := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	loop2 := keyspace.MakeTerm(keyspace.FamilyLoop, 2)
	label1 := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	label2 := keyspace.MakeTerm(keyspace.FamilyLabel, 2)

	// Rows are the complete lexicographic semantic-key set for two Bodies.
	// Each body owns all four mandatory coordinates and one of each optional
	// Return/Break/Goto coordinate. The target-bearing kinds are ordered by
	// kind ordinal before their target family/ordinal is considered.
	rows := []outcomeRowExpectation{
		{body: body1, kind: kind.OutcomeNormal},
		{body: body1, kind: kind.OutcomeReturn},
		{body: body1, kind: kind.OutcomeThrow},
		{body: body1, kind: kind.OutcomeBreak, target: loop1},
		{body: body1, kind: kind.OutcomeGoto, target: label1},
		{body: body1, kind: kind.OutcomeYield},
		{body: body1, kind: kind.OutcomeCancel},
		{body: body2, kind: kind.OutcomeNormal},
		{body: body2, kind: kind.OutcomeReturn},
		{body: body2, kind: kind.OutcomeThrow},
		{body: body2, kind: kind.OutcomeBreak, target: loop2},
		{body: body2, kind: kind.OutcomeGoto, target: label2},
		{body: body2, kind: kind.OutcomeYield},
		{body: body2, kind: kind.OutcomeCancel},
	}

	result := &Result{
		sourceID:    identity.ContentID{1},
		flowID:      identity.ContentID{2},
		staticID:    identity.ContentID{3},
		moduleID:    identity.ContentID{4},
		bodies:      make([]keyspace.Term, len(rows)+1),
		kinds:       make([]kind.OutcomeKind, len(rows)+1),
		targets:     make([]keyspace.Term, len(rows)+1),
		propagation: make([]keyspace.Term, len(rows)+1),
		returnExit:  make([]keyspace.Term, 3),
		breakExit:   make([]keyspace.Term, 2),
		gotoExit:    make([]keyspace.Term, 3),
	}
	for index, row := range rows {
		ordinal := index + 1
		result.bodies[ordinal] = row.body
		result.kinds[ordinal] = row.kind
		result.targets[ordinal] = row.target
	}
	result.base[kind.OutcomeNormal] = []keyspace.Term{0, outcomeAt(1), outcomeAt(8)}
	result.base[kind.OutcomeThrow] = []keyspace.Term{0, outcomeAt(3), outcomeAt(10)}
	result.base[kind.OutcomeYield] = []keyspace.Term{0, outcomeAt(6), outcomeAt(13)}
	result.base[kind.OutcomeCancel] = []keyspace.Term{0, outcomeAt(7), outcomeAt(14)}

	// These edges represent lexical paths but retain Outcome-only successors.
	// Body 1's Return/Throw paths continue to the corresponding Body 2 row.
	result.propagation[2] = outcomeAt(9)
	result.propagation[3] = outcomeAt(10)

	return1 := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	break1 := keyspace.MakeTerm(keyspace.FamilyBreak, 1)
	goto1 := keyspace.MakeTerm(keyspace.FamilyGoto, 1)
	goto2 := keyspace.MakeTerm(keyspace.FamilyGoto, 2)
	result.returnExit[1] = outcomeAt(2)
	result.returnExit[2] = outcomeAt(9)
	result.breakExit[1] = outcomeAt(4)
	// Goto 1 resumes at its Label directly; Goto 2 takes the outward Outcome.
	result.gotoExit[1] = label1
	result.gotoExit[2] = outcomeAt(12)

	return resultFixture{
		result: result, body1: body1, body2: body2, loop1: loop1, loop2: loop2,
		label1: label1, label2: label2, return1: return1, break1: break1,
		goto1: goto1, goto2: goto2, rows: rows,
	}
}

func outcomeAt(ordinal int) keyspace.Term {
	return keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(ordinal))
}

func TestResultCanonicalRowsAndBodyRanges(t *testing.T) {
	fixture := canonicalResultFixture()
	if got, want := fixture.result.Count(), len(fixture.rows); got != want {
		t.Fatalf("Count = %d, want %d", got, want)
	}
	for index, want := range fixture.rows {
		term, ok := fixture.result.At(index)
		if !ok || term != outcomeAt(index+1) {
			t.Fatalf("At(%d) = %v/%v, want %v/true", index, term, ok, outcomeAt(index+1))
		}
		body, outcomeKind, target, ok := fixture.result.Get(term)
		if !ok || body != want.body || outcomeKind != want.kind || target != want.target {
			t.Fatalf("Get(%v) = %v/%v/%v/%v, want %v/%v/%v/true", term, body, outcomeKind, target, ok, want.body, want.kind, want.target)
		}
	}
	if _, ok := fixture.result.At(len(fixture.rows)); ok {
		t.Fatal("At(end) accepted")
	}

	start, end, ok := fixture.result.BodyRange(fixture.body1)
	if !ok || start != 0 || end != 7 {
		t.Fatalf("BodyRange(body1) = %d/%d/%v, want 0/7/true", start, end, ok)
	}
	start, end, ok = fixture.result.BodyRange(fixture.body2)
	if !ok || start != 7 || end != 14 {
		t.Fatalf("BodyRange(body2) = %d/%d/%v, want 7/14/true", start, end, ok)
	}
	if _, _, ok := fixture.result.BodyRange(keyspace.MakeTerm(keyspace.FamilyLoop, 1)); ok {
		t.Fatal("BodyRange accepted a non-Body term")
	}
}

func TestResultMandatoryBodyExitsAndFindUseOneCoordinate(t *testing.T) {
	fixture := canonicalResultFixture()
	mandatory := []kind.OutcomeKind{
		kind.OutcomeNormal, kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel,
	}
	for _, body := range []keyspace.Term{fixture.body1, fixture.body2} {
		for _, outcomeKind := range mandatory {
			expected, found := fixture.result.Find(body, outcomeKind, 0)
			if !found {
				t.Fatalf("Find(%v,%v,0) did not find mandatory Body exit", body, outcomeKind)
			}
			got, ok := fixture.result.BodyExit(body, outcomeKind)
			if !ok || got != expected {
				t.Fatalf("BodyExit(%v,%v) = %v/%v, want %v/true", body, outcomeKind, got, ok, expected)
			}
		}
	}
	if got, ok := fixture.result.BodyExit(fixture.body1, kind.OutcomeReturn); ok || got != 0 {
		t.Fatalf("BodyExit(Return) = %v/%v, want 0/false", got, ok)
	}
	if got, ok := fixture.result.BodyExit(fixture.body1, kind.OutcomeBreak); ok || got != 0 {
		t.Fatalf("BodyExit(Break) = %v/%v, want 0/false", got, ok)
	}
	if got, ok := fixture.result.BodyExit(fixture.body1, kind.OutcomeGoto); ok || got != 0 {
		t.Fatalf("BodyExit(Goto) = %v/%v, want 0/false", got, ok)
	}
	for index, want := range fixture.rows {
		term, ok := fixture.result.Find(want.body, want.kind, want.target)
		if !ok || term != outcomeAt(index+1) {
			t.Fatalf("Find(%v,%v,%v) = %v/%v, want %v/true", want.body, want.kind, want.target, term, ok, outcomeAt(index+1))
		}
	}
}

func TestResultPropagationIsOutcomeOnlyAndTerminalPathsAreClosed(t *testing.T) {
	fixture := canonicalResultFixture()
	if got, ok := fixture.result.Propagation(outcomeAt(2)); !ok || got != outcomeAt(9) {
		t.Fatalf("Return propagation = %v/%v, want %v/true", got, ok, outcomeAt(9))
	}
	if got, ok := fixture.result.Propagation(outcomeAt(3)); !ok || got != outcomeAt(10) {
		t.Fatalf("Throw propagation = %v/%v, want %v/true", got, ok, outcomeAt(10))
	}
	for _, ordinal := range []int{1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14} {
		if got, ok := fixture.result.Propagation(outcomeAt(ordinal)); ok || got != 0 {
			t.Fatalf("terminal Propagation(%d) = %v/%v, want 0/false", ordinal, got, ok)
		}
	}

	for _, nonOutcome := range []keyspace.Term{fixture.loop1, fixture.label1, fixture.body1} {
		bad := canonicalResultFixture()
		bad.result.propagation[1] = nonOutcome
		if got, ok := bad.result.Propagation(outcomeAt(1)); ok || got != 0 {
			t.Fatalf("Propagation accepted non-Outcome successor %v: %v/%v", nonOutcome, got, ok)
		}
	}
	for _, badNext := range []int{1, 3, 4} {
		bad := canonicalResultFixture()
		bad.result.propagation[1] = outcomeAt(badNext)
		if got, ok := bad.result.Propagation(outcomeAt(1)); ok || got != 0 {
			t.Fatalf("Propagation accepted incompatible successor %d: %v/%v", badNext, got, ok)
		}
	}
}

func TestResultOccurrenceExitsPreserveTypedTerminalSemantics(t *testing.T) {
	fixture := canonicalResultFixture()
	if got, ok := fixture.result.ReturnExit(fixture.return1); !ok || got != outcomeAt(2) {
		t.Fatalf("ReturnExit = %v/%v, want %v/true", got, ok, outcomeAt(2))
	}
	if got, ok := fixture.result.BreakExit(fixture.break1); !ok || got != outcomeAt(4) {
		t.Fatalf("BreakExit = %v/%v, want %v/true", got, ok, outcomeAt(4))
	}
	if body, outcomeKind, target, ok := fixture.result.Get(outcomeAt(4)); !ok || body != fixture.body1 || outcomeKind != kind.OutcomeBreak || target != fixture.loop1 {
		t.Fatalf("Break Outcome = %v/%v/%v/%v, want %v/%v/%v/true", body, outcomeKind, target, ok, fixture.body1, kind.OutcomeBreak, fixture.loop1)
	}
	if got, ok := fixture.result.GotoExit(fixture.goto1); !ok || got != fixture.label1 {
		t.Fatalf("same-Body GotoExit = %v/%v, want label %v/true", got, ok, fixture.label1)
	}
	if got, ok := fixture.result.GotoExit(fixture.goto2); !ok || got != outcomeAt(12) {
		t.Fatalf("outward GotoExit = %v/%v, want %v/true", got, ok, outcomeAt(12))
	}
	if body, outcomeKind, target, ok := fixture.result.Get(outcomeAt(12)); !ok || body != fixture.body2 || outcomeKind != kind.OutcomeGoto || target != fixture.label2 {
		t.Fatalf("Goto Outcome = %v/%v/%v/%v, want %v/%v/%v/true", body, outcomeKind, target, ok, fixture.body2, kind.OutcomeGoto, fixture.label2)
	}
}

func TestResultQueriesFailClosedForNilZeroForeignAndMalformedResults(t *testing.T) {
	var nilResult *Result
	assertResultUnavailable(t, nilResult)
	assertResultUnavailable(t, &Result{})

	fixture := canonicalResultFixture()
	foreign := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	for _, term := range []keyspace.Term{0, foreign, keyspace.MakeTerm(keyspace.FamilyLoop, 1), keyspace.MakeTerm(keyspace.FamilyOutcome, 99)} {
		if body, outcomeKind, target, ok := fixture.result.Get(term); ok || body != 0 || outcomeKind != 0 || target != 0 {
			t.Fatalf("Get(%v) = %v/%v/%v/%v, want zero/false", term, body, outcomeKind, target, ok)
		}
		if got, ok := fixture.result.Propagation(term); ok || got != 0 {
			t.Fatalf("Propagation(%v) = %v/%v, want 0/false", term, got, ok)
		}
	}
	if _, ok := fixture.result.At(-1); ok {
		t.Fatal("At(-1) accepted")
	}
	if _, ok := fixture.result.ReturnExit(keyspace.MakeTerm(keyspace.FamilyBool, 1)); ok {
		t.Fatal("ReturnExit accepted wrong family")
	}
	if _, ok := fixture.result.BreakExit(keyspace.MakeTerm(keyspace.FamilyLabel, 1)); ok {
		t.Fatal("BreakExit accepted wrong family")
	}
	if _, ok := fixture.result.GotoExit(keyspace.MakeTerm(keyspace.FamilyReturn, 1)); ok {
		t.Fatal("GotoExit accepted wrong family")
	}
	if _, ok := fixture.result.GotoExit(keyspace.MakeTerm(keyspace.FamilyGoto, 3)); ok {
		t.Fatal("GotoExit accepted same-family out-of-range term")
	}
	if _, ok := fixture.result.ReturnExit(keyspace.MakeTerm(keyspace.FamilyReturn, 2)); !ok {
		t.Fatal("valid Return occurrence unexpectedly rejected")
	}
	if _, ok := fixture.result.BreakExit(keyspace.MakeTerm(keyspace.FamilyBreak, 2)); ok {
		t.Fatal("BreakExit accepted same-family out-of-range occurrence")
	}
	if _, _, ok := fixture.result.BodyRange(keyspace.MakeTerm(keyspace.FamilyBody, 3)); ok {
		t.Fatal("BodyRange accepted same-family out-of-range Body")
	}

	malformed := []struct {
		name             string
		result           *Result
		term             keyspace.Term
		checkCount       bool
		checkGet         bool
		checkBodyQueries bool
		checkReturnExit  bool
	}{
		{name: "mismatchedRows", result: &Result{bodies: []keyspace.Term{0, fixture.body1}, kinds: []kind.OutcomeKind{0}, targets: []keyspace.Term{0, 0}, propagation: []keyspace.Term{0, 0}}, term: outcomeAt(1), checkCount: true, checkGet: true, checkBodyQueries: true},
		{name: "badNormalPlane", result: cloneResult(fixture.result), term: outcomeAt(1), checkBodyQueries: true},
		{name: "badBodyRow", result: cloneResult(fixture.result), term: outcomeAt(1), checkGet: true, checkBodyQueries: true},
		{name: "badOccurrenceExit", result: cloneResult(fixture.result), term: keyspace.MakeTerm(keyspace.FamilyReturn, 1), checkReturnExit: true},
	}
	malformed[1].result.base[kind.OutcomeNormal] = []keyspace.Term{0, keyspace.MakeTerm(keyspace.FamilyBool, 1), outcomeAt(8)}
	malformed[2].result.bodies[1] = fixture.loop1
	malformed[3].result.returnExit[1] = keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	for _, tc := range malformed {
		if got := tc.result.Count(); tc.checkCount && got != 0 {
			t.Errorf("%s Count = %d, want 0", tc.name, got)
		}
		if body, outcomeKind, target, ok := tc.result.Get(tc.term); tc.checkGet && (ok || body != 0 || outcomeKind != 0 || target != 0) {
			t.Errorf("%s Get = %v/%v/%v/%v, want zero/false", tc.name, body, outcomeKind, target, ok)
		}
		if tc.checkBodyQueries {
			if _, _, ok := tc.result.BodyRange(fixture.body1); ok {
				t.Errorf("%s BodyRange accepted malformed proof", tc.name)
			}
			if _, ok := tc.result.BodyExit(fixture.body1, kind.OutcomeNormal); ok {
				t.Errorf("%s BodyExit accepted malformed proof", tc.name)
			}
		}
		if _, ok := tc.result.ReturnExit(keyspace.MakeTerm(keyspace.FamilyReturn, 1)); tc.checkReturnExit && ok {
			t.Errorf("%s ReturnExit accepted malformed exit", tc.name)
		}
	}
	// A dense slice can have internally sized rows while still containing an
	// impossible Body identity. Every query must reject that malformed proof;
	// query bounds cannot be inferred from the caller's requested Term.
	badIdentity := cloneResult(fixture.result)
	badIdentity.bodies[1] = keyspace.MakeTerm(keyspace.FamilyBody, 99)
	if body, outcomeKind, target, ok := badIdentity.Get(outcomeAt(1)); ok || body != 0 || outcomeKind != 0 || target != 0 {
		t.Fatalf("malformed Body Get = %v/%v/%v/%v, want zero/false", body, outcomeKind, target, ok)
	}
	if got, ok := badIdentity.Find(keyspace.MakeTerm(keyspace.FamilyBody, 99), kind.OutcomeNormal, 0); ok || got != 0 {
		t.Fatalf("Find accepted malformed Body identity: %v/%v", got, ok)
	}
	badIdentity.propagation[1] = outcomeAt(8)
	if got, ok := badIdentity.Propagation(outcomeAt(1)); ok || got != 0 {
		t.Fatalf("Propagation accepted malformed Body identity: %v/%v", got, ok)
	}
	badReturn := cloneResult(fixture.result)
	badReturn.bodies[2] = keyspace.MakeTerm(keyspace.FamilyBody, 99)
	badReturn.returnExit[1] = outcomeAt(2)
	if got, ok := badReturn.ReturnExit(keyspace.MakeTerm(keyspace.FamilyReturn, 1)); ok || got != 0 {
		t.Fatalf("ReturnExit accepted malformed Body identity: %v/%v", got, ok)
	}
}

func TestResultQueriesDoNotAllocate(t *testing.T) {
	fixture := canonicalResultFixture()
	var (
		count, start, end                                                              int
		at, gotBody, gotTarget, bodyExit, found, next, returnExit, breakExit, gotoExit keyspace.Term
		gotOutcome                                                                     kind.OutcomeKind
		ok, rangeOK, getOK, bodyExitOK, findOK, nextOK, returnOK, breakOK, gotoOK      bool
	)
	allocs := testing.AllocsPerRun(1000, func() {
		count = fixture.result.Count()
		at, ok = fixture.result.At(2)
		gotBody, gotOutcome, gotTarget, getOK = fixture.result.Get(at)
		start, end, rangeOK = fixture.result.BodyRange(fixture.body1)
		bodyExit, bodyExitOK = fixture.result.BodyExit(fixture.body1, kind.OutcomeNormal)
		found, findOK = fixture.result.Find(fixture.body1, kind.OutcomeNormal, 0)
		next, nextOK = fixture.result.Propagation(outcomeAt(2))
		returnExit, returnOK = fixture.result.ReturnExit(fixture.return1)
		breakExit, breakOK = fixture.result.BreakExit(fixture.break1)
		gotoExit, gotoOK = fixture.result.GotoExit(fixture.goto1)
	})
	if allocs != 0 {
		t.Fatalf("Outcome queries allocated %f times", allocs)
	}
	if count != len(fixture.rows) || !ok || at != outcomeAt(3) || !getOK || gotBody != fixture.body1 || gotOutcome != kind.OutcomeThrow || gotTarget != 0 || !rangeOK || start != 0 || end != 7 || !bodyExitOK || bodyExit != outcomeAt(1) || !findOK || found != outcomeAt(1) || !nextOK || next != outcomeAt(9) || !returnOK || returnExit != outcomeAt(2) || !breakOK || breakExit != outcomeAt(4) || !gotoOK || gotoExit != fixture.label1 {
		t.Fatal("allocation probe did not retain the expected query results")
	}
}

func TestResultRejectsInvalidFindKeysAndBodyExitKinds(t *testing.T) {
	fixture := canonicalResultFixture()
	invalidKinds := []kind.OutcomeKind{0, kind.OutcomeKind(8), kind.OutcomeKind(255)}
	for _, outcomeKind := range invalidKinds {
		if got, ok := fixture.result.Find(fixture.body1, outcomeKind, 0); ok || got != 0 {
			t.Fatalf("Find invalid kind %d = %v/%v, want 0/false", outcomeKind, got, ok)
		}
		if got, ok := fixture.result.BodyExit(fixture.body1, outcomeKind); ok || got != 0 {
			t.Fatalf("BodyExit invalid kind %d = %v/%v, want 0/false", outcomeKind, got, ok)
		}
	}
	invalidKeys := []struct {
		body, target keyspace.Term
		kind         kind.OutcomeKind
	}{
		{body: keyspace.MakeTerm(keyspace.FamilyLoop, 1), kind: kind.OutcomeNormal},
		{body: keyspace.MakeTerm(keyspace.FamilyBody, 0), kind: kind.OutcomeNormal},
		{body: fixture.body1, kind: kind.OutcomeBreak, target: 0},
		{body: fixture.body1, kind: kind.OutcomeBreak, target: fixture.label1},
		{body: fixture.body1, kind: kind.OutcomeGoto, target: 0},
		{body: fixture.body1, kind: kind.OutcomeGoto, target: fixture.loop1},
		{body: fixture.body1, kind: kind.OutcomeNormal, target: fixture.loop1},
	}
	for _, key := range invalidKeys {
		if got, ok := fixture.result.Find(key.body, key.kind, key.target); ok || got != 0 {
			t.Fatalf("Find invalid key %v/%d/%v = %v/%v, want 0/false", key.body, key.kind, key.target, got, ok)
		}
	}
}

func TestResultFindFailsClosedForMalformedRowStorage(t *testing.T) {
	fixture := canonicalResultFixture()
	tests := []struct {
		name   string
		mutate func(*Result)
	}{
		{name: "shortKinds", mutate: func(result *Result) {
			result.kinds = result.kinds[:len(result.kinds)-1]
		}},
		{name: "shortTargets", mutate: func(result *Result) {
			result.targets = result.targets[:len(result.targets)-1]
		}},
		{name: "shortPropagation", mutate: func(result *Result) {
			result.propagation = result.propagation[:len(result.propagation)-1]
		}},
		{name: "bodySentinel", mutate: func(result *Result) {
			result.bodies[0] = fixture.body1
		}},
		{name: "kindSentinel", mutate: func(result *Result) {
			result.kinds[0] = kind.OutcomeNormal
		}},
		{name: "targetSentinel", mutate: func(result *Result) {
			result.targets[0] = fixture.loop1
		}},
		{name: "propagationSentinel", mutate: func(result *Result) {
			result.propagation[0] = outcomeAt(1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := cloneResult(fixture.result)
			test.mutate(result)
			if got, ok := result.Find(fixture.body1, kind.OutcomeNormal, 0); ok || got != 0 {
				t.Fatalf("Find on malformed rows = %v/%v, want 0/false", got, ok)
			}
		})
	}
}

func assertResultUnavailable(t *testing.T, result *Result) {
	t.Helper()
	if result.Count() != 0 {
		t.Fatal("unavailable Result reported a nonzero Count")
	}
	if term, ok := result.At(0); ok || term != 0 {
		t.Fatalf("unavailable At = %v/%v, want 0/false", term, ok)
	}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	for _, outcomeKind := range []kind.OutcomeKind{kind.OutcomeNormal, kind.OutcomeReturn, kind.OutcomeThrow, kind.OutcomeBreak, kind.OutcomeGoto, kind.OutcomeYield, kind.OutcomeCancel} {
		if term, ok := result.BodyExit(body, outcomeKind); ok || term != 0 {
			t.Fatalf("unavailable BodyExit(%d) = %v/%v", outcomeKind, term, ok)
		}
		if term, ok := result.Find(body, outcomeKind, 0); ok || term != 0 {
			t.Fatalf("unavailable Find(%d) = %v/%v", outcomeKind, term, ok)
		}
	}
	if _, _, ok := result.BodyRange(body); ok {
		t.Fatal("unavailable BodyRange returned true")
	}
	if body, outcomeKind, target, ok := result.Get(outcomeAt(1)); ok || body != 0 || outcomeKind != 0 || target != 0 {
		t.Fatalf("unavailable Get = %v/%v/%v/%v", body, outcomeKind, target, ok)
	}
	if term, ok := result.Propagation(outcomeAt(1)); ok || term != 0 {
		t.Fatalf("unavailable Propagation = %v/%v", term, ok)
	}
	if term, ok := result.ReturnExit(keyspace.MakeTerm(keyspace.FamilyReturn, 1)); ok || term != 0 {
		t.Fatalf("unavailable ReturnExit = %v/%v", term, ok)
	}
	if term, ok := result.BreakExit(keyspace.MakeTerm(keyspace.FamilyBreak, 1)); ok || term != 0 {
		t.Fatalf("unavailable BreakExit = %v/%v", term, ok)
	}
	if term, ok := result.GotoExit(keyspace.MakeTerm(keyspace.FamilyGoto, 1)); ok || term != 0 {
		t.Fatalf("unavailable GotoExit = %v/%v", term, ok)
	}
}

func cloneResult(source *Result) *Result {
	clone := *source
	clone.bodies = append([]keyspace.Term(nil), source.bodies...)
	clone.kinds = append([]kind.OutcomeKind(nil), source.kinds...)
	clone.targets = append([]keyspace.Term(nil), source.targets...)
	clone.propagation = append([]keyspace.Term(nil), source.propagation...)
	for index := range source.base {
		clone.base[index] = append([]keyspace.Term(nil), source.base[index]...)
	}
	clone.returnExit = append([]keyspace.Term(nil), source.returnExit...)
	clone.breakExit = append([]keyspace.Term(nil), source.breakExit...)
	clone.gotoExit = append([]keyspace.Term(nil), source.gotoExit...)
	return &clone
}
