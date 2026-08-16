package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func continuationIDs() (identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID) {
	return identity.ContentID{0: 1}, identity.ContentID{0: 2}, identity.ContentID{0: 3}, identity.ContentID{0: 4}
}

func emptyContinuationResult() *Result {
	call := keyspace.FamilyCall
	cellRoots := [keyspace.FamilyCount][]uint32{}
	guardRoots := [keyspace.FamilyCount][]uint32{}
	var counts [keyspace.FamilyCount]uint32
	counts[call] = 1
	cellRoots[call] = []uint32{absentRoot, 0}
	guardRoots[call] = []uint32{absentRoot, 0}
	cellRecords := [keyspace.FamilyCount][]cellRootRecord{}
	guardRecords := [keyspace.FamilyCount][]guardRootRecord{}
	cellRecords[call] = []cellRootRecord{{}, {root: 0, present: true}}
	guardRecords[call] = []guardRootRecord{{}, {root: 0, present: true}}
	sourceID, flowID, staticID, moduleID := continuationIDs()
	return &Result{
		sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID,
		cells:  cellProjection{roots: cellRoots, records: cellRecords, nodes: []scopeNode{{}}, terms: nil, counts: counts},
		guards: guardProjection{roots: guardRoots, records: guardRecords, nodes: []guardNode{{}}, counts: counts},
	}
}

func TestContinuationQueryLaws(t *testing.T) {
	result := emptyContinuationResult()
	sourceID, flowID, staticID, moduleID := continuationIDs()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("exact four-owner provenance did not match")
	}
	ids := []identity.ContentID{sourceID, flowID, staticID, moduleID}
	for index := range ids {
		foreign := ids[index]
		foreign[0]++
		candidate := []identity.ContentID{sourceID, flowID, staticID, moduleID}
		candidate[index] = foreign
		if Matches(result, candidate[0], candidate[1], candidate[2], candidate[3]) {
			t.Fatalf("foreign owner %d matched", index)
		}
	}
	if Matches(result, identity.ContentID{}, flowID, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) {
		t.Fatal("unavailable owner matched")
	}
	if count, ok := result.CellCount(call); !ok || count != 0 {
		t.Fatalf("empty Cell root = %d/%v", count, ok)
	}
	if count, ok := result.GuardCount(call); !ok || count != 0 {
		t.Fatalf("empty Guard root = %d/%v", count, ok)
	}
	if _, ok := result.CellAt(call, 0); ok {
		t.Fatal("empty Cell root exposed an element")
	}
	if _, ok := result.GuardAt(call, 0); ok {
		t.Fatal("empty Guard root exposed an element")
	}
	for _, term := range []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
		keyspace.MakeTerm(keyspace.FamilyCall, 2),
		0,
	} {
		if _, ok := result.CellCount(term); ok {
			t.Fatalf("non-subject Cell %08x matched", uint32(term))
		}
		if _, ok := result.GuardCount(term); ok {
			t.Fatalf("non-subject Guard %08x matched", uint32(term))
		}
	}
}

func TestContinuationQueriesRejectMalformedRoots(t *testing.T) {
	result := emptyContinuationResult()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	result.cells.roots[keyspace.FamilyCall] = []uint32{absentRoot, 0, 0}
	if _, ok := result.CellCount(call); ok {
		t.Fatal("CellCount accepted an overlong subject root plane")
	}

	result = emptyContinuationResult()
	result.guards.roots[keyspace.FamilyCall] = []uint32{absentRoot, 0, 0}
	if _, ok := result.GuardCount(call); ok {
		t.Fatal("GuardCount accepted an overlong subject root plane")
	}

	result = emptyContinuationResult()
	result.cells.nodes = []scopeNode{{total: 1}}
	if _, ok := result.CellCount(call); ok {
		t.Fatal("malformed Cell sentinel was accepted")
	}

	result = emptyContinuationResult()
	result.guards.nodes = []guardNode{{}, {count: 0, term: keyspace.MakeTerm(keyspace.FamilySelect, 1)}}
	result.guards.roots[keyspace.FamilyCall][1] = 1
	if _, ok := result.GuardCount(call); ok {
		t.Fatal("malformed Guard leaf was accepted")
	}

	result = emptyContinuationResult()
	result.guards.nodes = []guardNode{{}, {prev: 0, jump: 0, count: 1, term: keyspace.MakeTerm(keyspace.FamilySelect, 1)}, {prev: 0, jump: 0, count: 2, term: keyspace.MakeTerm(keyspace.FamilySelect, 2)}}
	result.guards.roots[keyspace.FamilyCall][1] = 2
	if _, ok := result.GuardCount(call); ok {
		t.Fatal("malformed Guard count was accepted")
	}
}

func TestContinuationQueriesDeepWideAndAllocationFree(t *testing.T) {
	result := emptyContinuationResult()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	terms := make([]keyspace.Term, 1024)
	nodes := make([]scopeNode, len(terms)+1)
	for index := range terms {
		terms[index] = keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))
		nodes[index+1] = scopeNode{
			parent: uint32(index), start: uint32(index), count: 1, total: uint32(index + 1),
		}
	}
	result.cells.nodes = nodes
	result.cells.terms = terms
	result.cells.counts[keyspace.FamilyCell] = uint32(len(terms))
	result.cells.roots[keyspace.FamilyCall] = []uint32{absentRoot, uint32(len(nodes) - 1)}
	result.cells.records[keyspace.FamilyCall] = []cellRootRecord{{}, {root: uint32(len(nodes) - 1), count: uint32(len(terms)), present: true, node: nodes[len(nodes)-1]}}
	if count, ok := result.CellCount(call); !ok || count != len(terms) {
		t.Fatalf("deep CellCount = %d/%v, want %d/true", count, ok, len(terms))
	}
	for index, want := range []keyspace.Term{terms[len(terms)-1], terms[0]} {
		queryIndex := 0
		if index == 1 {
			queryIndex = len(terms) - 1
		}
		got, ok := result.CellAt(call, queryIndex)
		if !ok || got != want {
			t.Fatalf("deep CellAt(%d) = %08x/%v, want %08x/true", queryIndex, uint32(got), ok, uint32(want))
		}
	}
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = result.CellCount(call)
		_, _ = result.CellAt(call, len(terms)-1)
		_, _ = result.GuardCount(keyspace.MakeTerm(keyspace.FamilyCall, 1))
	})
	if allocs != 0 {
		t.Fatalf("continuation queries allocated %v objects per run", allocs)
	}
}
