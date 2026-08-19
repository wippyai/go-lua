package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestContinuationSealQueryIntegrityAndRepeatFrontier(t *testing.T) {
	fixture := openContinuationFixture(t, continuationBindPhaseSpec())
	term := continuationTerm(keyspace.FamilyUnary, 2)
	firstCount, firstOK := fixture.result.CellCount(term)
	firstCell, firstCellOK := fixture.result.CellAt(term, 0)
	secondCount, secondOK := fixture.result.CellCount(term)
	secondCell, secondCellOK := fixture.result.CellAt(term, 0)
	if !firstOK || !secondOK || firstCount != secondCount || !firstCellOK || !secondCellOK || firstCell != secondCell {
		t.Fatalf("repeat Frontier query changed result: first %d/%v %08x/%v, second %d/%v %08x/%v", firstCount, firstOK, uint32(firstCell), firstCellOK, secondCount, secondOK, uint32(secondCell), secondCellOK)
	}

	fixture.result.cells.nodes = []scopeNode{{}, {parent: 1, start: 0, count: 1, total: 1}}
	fixture.result.cells.terms = []keyspace.Term{continuationTerm(keyspace.FamilyCell, 1)}
	fixture.result.cells.roots[keyspace.FamilyUnary][2] = 1
	if _, ok := fixture.result.CellCount(term); ok {
		t.Fatal("CellCount accepted a cyclic scope root")
	}
	if _, ok := fixture.result.CellAt(term, 0); ok {
		t.Fatal("CellAt accepted a cyclic scope root")
	}

	fixture = openContinuationFixture(t, directContinuationSpec("continuation-query-integrity-guard.lua"))
	call := continuationTerm(keyspace.FamilyCall, 1)
	fixture.result.guards.nodes = []guardNode{{}, {prev: 2, jump: 0, term: keyspace.MakeTerm(keyspace.FamilySelect, 1), count: 1}, {prev: 1, jump: 0, term: keyspace.MakeTerm(keyspace.FamilySelect, 1), count: 1}}
	fixture.result.guards.roots[keyspace.FamilyCall][1] = 1
	if _, ok := fixture.result.GuardCount(call); ok {
		t.Fatal("GuardCount accepted a cyclic Guard-chain root")
	}
	if _, ok := fixture.result.GuardAt(call, 0); ok {
		t.Fatal("GuardAt accepted a cyclic Guard-chain root")
	}
}

func TestContinuationSealQueriesRejectOffPathRootsAndAgree(t *testing.T) {
	fixture := openContinuationFixture(t, continuationBindPhaseSpec())
	first := continuationTerm(keyspace.FamilyUnary, 1)
	firstRoot := fixture.result.cells.roots[keyspace.FamilyUnary][1]
	secondRoot := fixture.result.cells.roots[keyspace.FamilyUnary][2]
	if firstRoot == secondRoot || firstRoot == absentRoot || secondRoot == absentRoot {
		t.Fatalf("Bind phase did not retain distinct Cell roots: %d/%d", firstRoot, secondRoot)
	}
	fixture.result.cells.roots[keyspace.FamilyUnary][1] = secondRoot
	if _, ok := fixture.result.CellCount(first); ok {
		t.Fatal("CellCount accepted an off-path fabricated root")
	}
	if _, ok := fixture.result.CellAt(first, 0); ok {
		t.Fatal("CellAt accepted an off-path fabricated root")
	}

	fixture = openContinuationFixture(t, continuationLoopCellSpec())
	loop := continuationTerm(keyspace.FamilyLoop, 1)
	loopRoot := fixture.result.guards.roots[keyspace.FamilyLoop][1]
	childRoot := fixture.result.guards.roots[keyspace.FamilyUnary][1]
	if loopRoot == childRoot || loopRoot == absentRoot || childRoot == absentRoot {
		t.Fatalf("Loop fixture did not retain distinct Guard roots: %d/%d", loopRoot, childRoot)
	}
	fixture.result.guards.roots[keyspace.FamilyLoop][1] = childRoot
	if _, ok := fixture.result.GuardCount(loop); ok {
		t.Fatal("GuardCount accepted an off-path fabricated root")
	}
	if _, ok := fixture.result.GuardAt(loop, 0); ok {
		t.Fatal("GuardAt accepted an off-path fabricated root")
	}
}

func TestContinuationSealDeepBodyAndWideAllocation(t *testing.T) {
	deep := openContinuationFixture(t, continuationDeepBodySpec(64))
	if deep.result == nil {
		t.Fatal("deep Body continuation Result is nil")
	}
	wide := openContinuationFixture(t, continuationWideCallSpec(64))
	allocs := testing.AllocsPerRun(500, func() {
		for ordinal := uint32(1); ordinal <= 64; ordinal++ {
			term := continuationTerm(keyspace.FamilyCall, ordinal)
			_, _ = wide.result.CellCount(term)
			_, _ = wide.result.CellAt(term, 0)
			_, _ = wide.result.GuardCount(term)
			_, _ = wide.result.GuardAt(term, 0)
		}
	})
	if allocs != 0 {
		t.Fatalf("wide continuation queries allocated %v objects per run", allocs)
	}
}

func continuationDeepBodySpec(depth int) continuationSpec {
	counts := testContinuationCounts(familyCount(keyspace.FamilyBody, uint32(depth)))
	rows := make([][]keyspace.Term, depth)
	for index := 0; index+1 < depth; index++ {
		rows[index] = []keyspace.Term{continuationTerm(keyspace.FamilyBody, uint32(index+2))}
	}
	return continuationSpec{name: "continuation-deep-bodies.lua", counts: counts, rows: rows}
}

func continuationWideCallSpec(width int) continuationSpec {
	body := continuationTerm(keyspace.FamilyBody, 1)
	counts := testContinuationCounts(familyCount(keyspace.FamilyBody, 1), familyCount(keyspace.FamilyCall, uint32(width)), familyCount(keyspace.FamilyValues, uint32(width)), familyCount(keyspace.FamilyNil, uint32(width)))
	rows := make([]keyspace.Term, width)
	values := make([]authored.Value, width)
	calls := make([]authored.Call, width)
	nilOwners := make([]keyspace.Term, width)
	for index := 0; index < width; index++ {
		ordinal := uint32(index + 1)
		rows[index] = continuationTerm(keyspace.FamilyCall, ordinal)
		values[index] = authored.Value{Owner: body}
		calls[index] = authored.Call{Owner: body, Callee: continuationTerm(keyspace.FamilyNil, ordinal), Actuals: continuationTerm(keyspace.FamilyValues, ordinal)}
		nilOwners[index] = body
	}
	return continuationSpec{name: "continuation-wide-calls.lua", counts: counts, rows: [][]keyspace.Term{rows}, nilOwners: nilOwners, flow: authored.Input{Values: authored.ValuesInput{Rows: values}, Calls: calls}}
}
