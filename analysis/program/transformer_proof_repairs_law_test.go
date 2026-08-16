package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
)

func TestTransformerStructuralRouteGuardProofs(t *testing.T) {
	base := lowerProofRepairProgram(t, "route-guard.lua", `
local function target(value)
  if value then return value else return 0 end
end
return target(1)
`)
	replay := lowerProofRepairProgram(t, "route-guard.lua", `
local function target(value)
  if value then return value else return 0 end
end
return target(1)
`)
	foreign := lowerProofRepairProgram(t, "route-guard-foreign.lua", `return 0`)

	trueCount, falseCount, unguardedCount, recurrenceCount := 0, 0, 0, 0
	for index := 0; index < base.TransformerInput().Routes().Count(); index++ {
		route, ok := base.TransformerInput().Routes().At(index)
		if !ok {
			t.Fatalf("Routes.At(%d) unavailable", index)
		}
		guard, guarded := route.Guard()
		if !guarded {
			unguardedCount++
		} else {
			truth, truthOK := guard.Truth()
			if !truthOK || !guard.Available() || !base.TransformerInput().OwnsRouteGuard(guard) {
				t.Fatalf("route %d guard proof unavailable or foreign", index)
			}
			if truth {
				trueCount++
			} else {
				falseCount++
			}
		}
		recurrence, recurrenceOK := route.Recurrence()
		if recurrenceOK {
			recurrenceCount++
			routeID, routeIDOK := route.RouteID()
			proofRouteID, proofRouteIDOK := recurrence.RouteID()
			if !base.TransformerInput().OwnsRouteRecurrence(recurrence) || !recurrence.ComponentID().Available() || !routeIDOK || !proofRouteIDOK || routeID != proofRouteID {
				t.Fatalf("route %d recurrence proof unavailable or foreign", index)
			}
			if _, componentSiteOK := route.Component(); !componentSiteOK {
				if _, componentOK := recurrence.Component(); !componentOK {
					t.Fatalf("route %d recurrence component disappeared", index)
				}
			}
		}
		for replayIndex := 0; replayIndex < replay.TransformerInput().Routes().Count(); replayIndex++ {
			replayed, replayedOK := replay.TransformerInput().Routes().At(replayIndex)
			if !replayedOK {
				continue
			}
			digest, digestOK := route.RouteDigest()
			replayDigest, replayDigestOK := replayed.RouteDigest()
			if digestOK && replayDigestOK && digest == replayDigest {
				if recurrenceOK {
					replayRecurrence, replayRecurrenceOK := replayed.Recurrence()
					if !replayRecurrenceOK || !recurrence.Equal(replayRecurrence) || base.TransformerInput().OwnsRouteRecurrence(replayRecurrence) {
						t.Fatal("replayed route recurrence identity/owner fence failed")
					}
				}
				if !guarded {
					continue
				}
				replayGuard, replayGuardOK := replayed.Guard()
				if !replayGuardOK || replayGuard.ContextID() != guard.ContextID() || base.TransformerInput().OwnsRouteGuard(replayGuard) {
					t.Fatal("replayed route guard identity/owner fence failed")
				}
			}
		}
		for foreignIndex := 0; foreignIndex < foreign.TransformerInput().Routes().Count(); foreignIndex++ {
			foreignRoute, foreignOK := foreign.TransformerInput().Routes().At(foreignIndex)
			if !foreignOK {
				continue
			}
			foreignGuard, foreignGuardOK := foreignRoute.Guard()
			if foreignGuardOK && base.TransformerInput().OwnsRouteGuard(foreignGuard) {
				t.Fatal("foreign route guard crossed Program owner fence")
			}
			foreignRecurrence, foreignRecurrenceOK := foreignRoute.Recurrence()
			if foreignRecurrenceOK && base.TransformerInput().OwnsRouteRecurrence(foreignRecurrence) {
				t.Fatal("foreign route recurrence crossed Program owner fence")
			}
		}
	}
	if trueCount == 0 || falseCount == 0 || unguardedCount == 0 || recurrenceCount == 0 {
		t.Fatalf("route coverage = true %d, false %d, unguarded %d, recurrence %d", trueCount, falseCount, unguardedCount, recurrenceCount)
	}
}

func TestTransformerCallBoundaryArmReceipts(t *testing.T) {
	programValue := lowerProofRepairProgram(t, "call-boundary.lua", `
local function sink(value) return value end
return sink(1)
`)
	replay := lowerProofRepairProgram(t, "call-boundary.lua", `
local function sink(value) return value end
return sink(1)
`)
	foreign := lowerProofRepairProgram(t, "call-boundary-foreign.lua", `return 0`)
	input := programValue.TransformerInput()
	calls := programValue.Flow().Authored().Calls()
	sawExecutable := false
	for index := 0; index < input.CallCount(); index++ {
		call, callOK := input.CallAt(index)
		term, termOK := calls.At(index)
		if !callOK || !termOK {
			t.Fatalf("CallAt(%d) unavailable", index)
		}
		if !programValue.Flow().Executable().Contains(term) {
			continue
		}
		sawExecutable = true
		boundary, boundaryOK := call.Boundary()
		if !boundaryOK || !call.Executable() || call.Disposition() != program.CallDispositionExecutableRouted || boundary.ArmCount() == 0 {
			t.Fatal("executable call did not retain an exact sealed boundary")
		}
		for armIndex := 0; armIndex < boundary.ArmCount(); armIndex++ {
			arm, armOK := boundary.ArmAt(armIndex)
			if !armOK || !arm.Available() {
				t.Fatalf("boundary arm %d unavailable", armIndex)
			}
		}
		replayedCall, replayedCallOK := replay.TransformerInput().CallAt(index)
		replayedBoundary, replayedBoundaryOK := replayedCall.Boundary()
		if !replayedCallOK || !replayedBoundaryOK || input.OwnsCallBoundary(replayedBoundary) {
			t.Fatal("replayed CallBoundary crossed exact owner fence")
		}
		foreignCall, foreignCallOK := foreign.TransformerInput().CallAt(index)
		if foreignCallOK {
			if foreignBoundary, foreignBoundaryOK := foreignCall.Boundary(); foreignBoundaryOK && input.OwnsCallBoundary(foreignBoundary) {
				t.Fatal("foreign CallBoundary crossed exact owner fence")
			}
		}
		if allocations := testing.AllocsPerRun(10000, func() {
			for armIndex := 0; armIndex < boundary.ArmCount(); armIndex++ {
				_, _ = boundary.ArmAt(armIndex)
			}
		}); allocations != 0 {
			t.Fatalf("CallBoundary arm lookup allocated %v times", allocations)
		}
	}
	if !sawExecutable {
		t.Fatal("fixture did not lower an executable call")
	}
}

func TestTransformerStorageAssignmentSemanticPaths(t *testing.T) {
	base := lowerProofRepairProgram(t, "storage-assignment.lua", `
local left, right = 1, 2
left, right = right, left
left, right = right, left
return left, right
`)
	prior := lowerProofRepairProgram(t, "storage-assignment-prior.lua", `
local unrelated = {}
local left, right = 1, 2
left, right = right, left
left, right = right, left
return left, right
`)
	replay := lowerProofRepairProgram(t, "storage-assignment.lua", `
local left, right = 1, 2
left, right = right, left
left, right = right, left
return left, right
`)
	foreign := lowerProofRepairProgram(t, "storage-assignment-foreign.lua", `return 0`)
	input := base.TransformerInput()
	if input.StorageAssignmentCount() < 2 {
		t.Fatal("fixture did not lower two assignments")
	}
	baseAssignments, priorAssignments := make([]program.StorageAssignment, 0, 2), make([]program.StorageAssignment, 0, 2)
	for index := 0; index < input.StorageAssignmentCount(); index++ {
		assignment, ok := input.StorageAssignmentAt(index)
		if ok {
			baseAssignments = append(baseAssignments, assignment)
		}
	}
	for index := 0; index < prior.TransformerInput().StorageAssignmentCount(); index++ {
		assignment, ok := prior.TransformerInput().StorageAssignmentAt(index)
		if ok {
			priorAssignments = append(priorAssignments, assignment)
		}
	}
	if len(baseAssignments) != 2 || len(priorAssignments) != 2 {
		t.Fatalf("assignment proof counts = %d/%d, want 2/2", len(baseAssignments), len(priorAssignments))
	}
	if baseAssignments[0].ContextID() == baseAssignments[1].ContextID() || baseAssignments[0].ContextID() != priorAssignments[0].ContextID() || baseAssignments[1].ContextID() != priorAssignments[1].ContextID() {
		t.Fatal("assignment structural IDs were not distinct and insertion-stable")
	}
	for index, assignment := range baseAssignments {
		replayed, ok := replay.TransformerInput().StorageAssignmentAt(index)
		if !ok || replayed.ContextID() != assignment.ContextID() || input.OwnsStorageAssignment(replayed) {
			t.Fatalf("assignment %d replay identity/owner fence failed", index)
		}
		if foreignAssignment, foreignOK := foreign.TransformerInput().StorageAssignmentAt(index); foreignOK && input.OwnsStorageAssignment(foreignAssignment) {
			t.Fatalf("assignment %d foreign owner fence failed", index)
		}
		if span, spanOK := assignment.Span(); !spanOK || !input.OwnsSpan(span) {
			t.Fatalf("assignment %d lost exact Span proof", index)
		}
	}
}

func TestTransformerValueSourceChildPaths(t *testing.T) {
	base := lowerProofRepairProgram(t, "value-source-path.lua", `
local values = { 1, 1 }
return values
`)
	prior := lowerProofRepairProgram(t, "value-source-path-prior.lua", `
local unrelated = {}
local values = { 1, 1 }
return values
`)
	replay := lowerProofRepairProgram(t, "value-source-path.lua", `
local values = { 1, 1 }
return values
`)
	foreign := lowerProofRepairProgram(t, "value-source-path-foreign.lua", `return { 1, 1 }`)
	input, priorInput := base.TransformerInput(), prior.TransformerInput()
	if input.IntegerSourceCount() < 2 || priorInput.IntegerSourceCount() < 2 {
		t.Fatal("fixture did not lower same-kind literal siblings")
	}
	first, firstOK := input.IntegerSourceAt(0)
	second, secondOK := input.IntegerSourceAt(1)
	priorFirst, priorFirstOK := priorInput.IntegerSourceAt(0)
	priorSecond, priorSecondOK := priorInput.IntegerSourceAt(1)
	if !firstOK || !secondOK || !priorFirstOK || !priorSecondOK || first.ContextID() == second.ContextID() || first.ContextID() != priorFirst.ContextID() || second.ContextID() != priorSecond.ContextID() {
		t.Fatal("value-source child paths were not distinct and insertion-stable")
	}
	replayedFirst, replayedOK := replay.TransformerInput().IntegerSourceAt(0)
	if !replayedOK || replayedFirst.ContextID() != first.ContextID() || input.OwnsValueSourceOccurrence(replayedFirst) {
		t.Fatal("value-source replay identity/owner fence failed")
	}
	foreignFirst, foreignOK := foreign.TransformerInput().IntegerSourceAt(0)
	if !foreignOK || input.OwnsValueSourceOccurrence(foreignFirst) {
		t.Fatal("foreign value-source path crossed owner fence")
	}
}

func lowerProofRepairProgram(t testing.TB, name, text string) *program.Program {
	t.Helper()
	result, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatalf("Lower(%s): %v", name, err)
	}
	return result
}
