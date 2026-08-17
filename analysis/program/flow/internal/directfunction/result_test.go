package directfunction

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func directTestSourceID() identity.ContentID {
	var id identity.ContentID
	id[0] = 1
	return id
}

func directTestFlowID() identity.ContentID {
	var id identity.ContentID
	id[0] = 2
	return id
}

func directTestStaticID() identity.ContentID {
	var id identity.ContentID
	id[0] = 3
	return id
}

func directTestModuleID() identity.ContentID {
	var id identity.ContentID
	id[0] = 4
	return id
}

func TestResultQueriesRetainOnlyExactDensePlanes(t *testing.T) {
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	result := &Result{
		sourceID:      directTestSourceID(),
		flowID:        directTestFlowID(),
		staticID:      directTestStaticID(),
		moduleID:      directTestModuleID(),
		readFunctions: []keyspace.Term{0, function},
		callFunctions: []keyspace.Term{0, function},
		loopFunctions: []keyspace.Term{0, function},
		functionCount: 1,
	}
	if got, ok := result.DirectFunction(function); !ok || got != function {
		t.Fatalf("dead Function DirectFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.DirectFunction(read); !ok || got != function {
		t.Fatalf("Read DirectFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.ReadFunction(read); !ok || got != function {
		t.Fatalf("ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.CallFunction(call); !ok || got != function {
		t.Fatalf("CallFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.GenericLoopFunction(loop); !ok || got != function {
		t.Fatalf("GenericLoopFunction = %v/%v, want %v/true", got, ok, function)
	}
	for _, term := range []keyspace.Term{
		0,
		keyspace.MakeTerm(keyspace.FamilyFunction, 2),
		keyspace.MakeTerm(keyspace.FamilyRead, 2),
		keyspace.MakeTerm(keyspace.FamilyCall, 2),
		keyspace.MakeTerm(keyspace.FamilyLoop, 2),
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
	} {
		if got, ok := result.DirectFunction(term); ok || got != 0 {
			t.Fatalf("DirectFunction(%v) = %v/%v, want 0/false", term, got, ok)
		}
	}
	var nilResult *Result
	if got, ok := nilResult.DirectFunction(function); ok || got != 0 {
		t.Fatalf("nil DirectFunction = %v/%v, want 0/false", got, ok)
	}
}

func TestResultQueriesAllocateNothing(t *testing.T) {
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	result := &Result{
		sourceID:      directTestSourceID(),
		flowID:        directTestFlowID(),
		staticID:      directTestStaticID(),
		moduleID:      directTestModuleID(),
		readFunctions: []keyspace.Term{0, function},
		callFunctions: []keyspace.Term{0, function},
		loopFunctions: []keyspace.Term{0, function},
		functionCount: 1,
	}
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = result.DirectFunction(function)
		_, _ = result.ReadFunction(read)
		_, _ = result.CallFunction(call)
		_, _ = result.GenericLoopFunction(loop)
	})
	if allocations != 0 {
		t.Fatalf("Result queries allocated %.2f objects per run", allocations)
	}
}

func TestResultProvenanceFailsClosedForNilAndMalformed(t *testing.T) {
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	var sourceID, flowID identity.ContentID
	var staticID, moduleID identity.ContentID
	sourceID[0] = 0x11
	flowID[0] = 0x22
	staticID[0] = 0x33
	moduleID[0] = 0x44
	result := &Result{
		// Keep plausible query planes so this exercises the owner fence rather
		// than an empty-result special case.
		readFunctions: []keyspace.Term{0, function},
		functionCount: 1,
	}
	if Matches(nil, sourceID, flowID, staticID, moduleID) || Matches(result, sourceID, flowID, staticID, moduleID) ||
		Matches(result, identity.ContentID{}, flowID, staticID, moduleID) ||
		Matches(result, sourceID, identity.ContentID{}, staticID, moduleID) ||
		Matches(result, sourceID, flowID, identity.ContentID{}, moduleID) ||
		Matches(result, sourceID, flowID, staticID, identity.ContentID{}) {
		t.Fatal("direct-function provenance accepted nil or zero/unavailable owner identity")
	}
	if got, ok := result.DirectFunction(function); ok || got != 0 {
		t.Fatalf("malformed DirectFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.ReadFunction(read); ok || got != 0 {
		t.Fatalf("malformed ReadFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.CallFunction(call); ok || got != 0 {
		t.Fatalf("malformed CallFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.GenericLoopFunction(loop); ok || got != 0 {
		t.Fatalf("malformed GenericLoopFunction = %v/%v, want 0/false", got, ok)
	}
}

func TestTerminalCellsIsIterativeAndRejectsCycles(t *testing.T) {
	const depth = 100_000
	captureOuter := make([]keyspace.Term, depth+1)
	for ordinal := 1; ordinal < depth; ordinal++ {
		captureOuter[ordinal] = keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal+1))
	}
	terminal, err := terminalCells(captureOuter)
	if err != nil {
		t.Fatalf("deep Capture forest rejected: %v", err)
	}
	base := keyspace.MakeTerm(keyspace.FamilyCell, uint32(depth))
	if terminal[1] != base || terminal[depth] != base {
		t.Fatalf("deep Capture terminal = %v/%v, want %v", terminal[1], terminal[depth], base)
	}
	captureOuter[depth] = keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if _, err := terminalCells(captureOuter); err == nil {
		t.Fatal("cyclic Capture forest was accepted")
	}
}
