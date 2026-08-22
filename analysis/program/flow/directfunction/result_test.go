package directfunction

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestResultQueriesRetainOnlyExactDensePlanes(t *testing.T) {
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	result := &Result{
		sourceID:          flowtest.ContentIDAt(1),
		flowID:            flowtest.ContentIDAt(2),
		staticID:          flowtest.ContentIDAt(3),
		moduleID:          flowtest.ContentIDAt(4),
		readFunctions:     []keyspace.Term{0, function},
		callFunctions:     []keyspace.Term{0, function},
		loopFunctions:     []keyspace.Term{0, function},
		functionCount:     1,
		selectedBodyPaths: []identity.ContentID{flowtest.ContentIDAt(5)},
	}
	if got, ok := result.For(function); !ok || got != function {
		t.Fatalf("dead Function DirectFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.For(read); !ok || got != function {
		t.Fatalf("Read DirectFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.Read(read); !ok || got != function {
		t.Fatalf("ReadFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.Call(call); !ok || got != function {
		t.Fatalf("CallFunction = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := result.GenericLoop(loop); !ok || got != function {
		t.Fatalf("GenericLoopFunction = %v/%v, want %v/true", got, ok, function)
	}
	if selected, ok := result.SelectedBodyPath(flowtest.ContentIDAt(5)); !ok || !selected {
		t.Fatalf("SelectedBodyPath = %v/%v, want true/true", selected, ok)
	}
	if selected, ok := result.SelectedBodyPath(flowtest.ContentIDAt(6)); !ok || selected {
		t.Fatalf("unselected BodyPath = %v/%v, want false/true", selected, ok)
	}
	for _, term := range []keyspace.Term{
		0,
		keyspace.MakeTerm(keyspace.FamilyFunction, 2),
		keyspace.MakeTerm(keyspace.FamilyRead, 2),
		keyspace.MakeTerm(keyspace.FamilyCall, 2),
		keyspace.MakeTerm(keyspace.FamilyLoop, 2),
		keyspace.MakeTerm(keyspace.FamilyValues, 1),
	} {
		if got, ok := result.For(term); ok || got != 0 {
			t.Fatalf("For(%v) = %v/%v, want 0/false", term, got, ok)
		}
	}
	var nilResult *Result
	if got, ok := nilResult.For(function); ok || got != 0 {
		t.Fatalf("nil DirectFunction = %v/%v, want 0/false", got, ok)
	}
}

func TestResultQueriesAllocateNothing(t *testing.T) {
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	result := &Result{
		sourceID:          flowtest.ContentIDAt(1),
		flowID:            flowtest.ContentIDAt(2),
		staticID:          flowtest.ContentIDAt(3),
		moduleID:          flowtest.ContentIDAt(4),
		readFunctions:     []keyspace.Term{0, function},
		callFunctions:     []keyspace.Term{0, function},
		loopFunctions:     []keyspace.Term{0, function},
		functionCount:     1,
		selectedBodyPaths: []identity.ContentID{flowtest.ContentIDAt(5)},
	}
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = result.For(function)
		_, _ = result.Read(read)
		_, _ = result.Call(call)
		_, _ = result.GenericLoop(loop)
		_, _ = result.SelectedBodyPath(flowtest.ContentIDAt(5))
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
	if got, ok := result.For(function); ok || got != 0 {
		t.Fatalf("malformed DirectFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.Read(read); ok || got != 0 {
		t.Fatalf("malformed ReadFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.Call(call); ok || got != 0 {
		t.Fatalf("malformed CallFunction = %v/%v, want 0/false", got, ok)
	}
	if got, ok := result.GenericLoop(loop); ok || got != 0 {
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
