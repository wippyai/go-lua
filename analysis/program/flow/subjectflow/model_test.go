package subjectflow

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func subjectID(seed byte) (id identity.ContentID) {
	id[0] = seed
	return id
}

func TestEventAndBoundaryIDsAreFramedAndStable(t *testing.T) {
	subject := Subject{Kind: SubjectValue, ID: subjectID(1), Term: keyspace.MakeTerm(keyspace.FamilyInteger, 1)}
	related := Subject{Kind: SubjectCell, ID: subjectID(2), Term: keyspace.MakeTerm(keyspace.FamilyCell, 1)}
	path := subjectID(3)
	first := rowID(EventAlias, RoleWrite, 2, subject, related, subject.Term, path)
	second := rowID(EventAlias, RoleWrite, 2, subject, related, subject.Term, path)
	if !first.Available() || first != second {
		t.Fatalf("row identity = %v/%v, want stable available identity", first, second)
	}
	changedSlot := rowID(EventAlias, RoleWrite, 3, subject, related, subject.Term, path)
	if changedSlot == first {
		t.Fatal("row identity collapsed distinct authored slots")
	}
	yieldRoute, reentryRoute := subjectID(4), subjectID(5)
	firstBoundary := boundaryID(subject.Term, path, BoundaryPaired, causal.BoundaryYield, causal.BoundaryResume, yieldRoute, reentryRoute, path, subjectID(6), path, subjectID(7))
	secondBoundary := boundaryID(subject.Term, path, BoundaryPaired, causal.BoundaryYield, causal.BoundaryResume, yieldRoute, reentryRoute, path, subjectID(6), path, subjectID(7))
	if !firstBoundary.Available() || firstBoundary != secondBoundary {
		t.Fatal("boundary identity was not stable")
	}
}

func TestResultProjectionFailsClosedAndCountsUnknownBoundaries(t *testing.T) {
	sourceID, flowID, staticID, moduleID := subjectID(11), subjectID(12), subjectID(13), subjectID(14)
	event := Event{ID: subjectID(21), Kind: EventUnknown, Role: RoleCapture, Subject: Subject{Kind: SubjectCell, ID: subjectID(22), Term: keyspace.MakeTerm(keyspace.FamilyCell, 1)}}
	boundary := Boundary{ID: subjectID(23), State: BoundaryUnknown, Call: keyspace.MakeTerm(keyspace.FamilyCall, 1), YieldArm: causal.BoundaryYield, YieldRoute: subjectID(24)}
	result := &Result{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID, events: []Event{event}, boundaries: []Boundary{boundary}}
	if !result.Available() || !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("valid result did not pass owner fence")
	}
	if result.EventCount() != 1 || result.BoundaryCount() != 1 || result.UnknownCount() != 2 || !result.HasUnknown() {
		t.Fatalf("counts = events %d boundaries %d unknown %d", result.EventCount(), result.BoundaryCount(), result.UnknownCount())
	}
	if _, ok := result.EventAt(-1); ok {
		t.Fatal("EventAt accepted a negative index")
	}
	foreign := sourceID
	foreign[0]++
	if Matches(result, foreign, flowID, staticID, moduleID) || result.EventCount() != 1 {
		t.Fatal("foreign owner tuple remained publishable")
	}
}

func TestAliasUnknownAbsorbsExactAliasComponent(t *testing.T) {
	left := Subject{Kind: SubjectValue, ID: subjectID(31), Term: keyspace.MakeTerm(keyspace.FamilyInteger, 1)}
	right := Subject{Kind: SubjectValue, ID: subjectID(32), Term: keyspace.MakeTerm(keyspace.FamilyInteger, 2)}
	leftKey, rightKey := makeSubjectKey(left), makeSubjectKey(right)
	candidates := map[subjectKey]*aliasCandidateState{
		leftKey:  {subject: left},
		rightKey: {subject: right, unknown: true},
	}
	aliases := map[subjectKey][]subjectKey{
		leftKey:  {rightKey},
		rightKey: {leftKey},
	}
	foldAliasUnknown(candidates, aliases)
	if !candidates[leftKey].unknown || !candidates[rightKey].unknown {
		t.Fatal("Unknown did not absorb the exact alias component")
	}
}

func TestAliasCandidateCarriesExplicitEmptyScopeState(t *testing.T) {
	candidate := Subject{Kind: SubjectCell, ID: subjectID(41), Term: keyspace.MakeTerm(keyspace.FamilyCell, 1)}
	body := subjectID(42)
	scope := newAliasRouteScope(AliasRouteScopeBody, body, nil)
	if !scope.Available() || scope.RouteCount() != 0 {
		t.Fatal("explicit empty route scope is unavailable")
	}
	closed := newAliasCandidate(candidate, scope.ID, true)
	open := newAliasCandidate(candidate, scope.ID, false)
	if !closed.Available() || !closed.Closed || !open.Available() || open.Closed || open.ID == closed.ID {
		t.Fatal("open and closed candidates over one empty scope collapsed")
	}
}

func TestAliasCandidatesShareOneRouteScopeWithoutRouteCopies(t *testing.T) {
	routes := make([]identity.ContentID, 32)
	for index := range routes {
		routes[index] = subjectID(byte(80 + index))
	}
	scope := newAliasRouteScope(AliasRouteScopeBody, subjectID(70), routes)
	if !scope.Available() {
		t.Fatal("shared body scope")
	}
	candidates := make([]AliasCandidate, 128)
	for index := range candidates {
		subject := Subject{Kind: SubjectValue, ID: subjectID(byte(index + 1)), Term: keyspace.MakeTerm(keyspace.FamilyInteger, uint32(index+1))}
		candidates[index] = newAliasCandidate(subject, scope.ID, true)
		if !candidates[index].Available() || candidates[index].ScopeID() != scope.ID {
			t.Fatal("candidate did not reference the shared scope")
		}
	}
	result := &Result{
		sourceID:    subjectID(1),
		flowID:      subjectID(2),
		staticID:    subjectID(3),
		moduleID:    subjectID(4),
		routeScopes: []AliasRouteScope{scope},
		candidates:  candidates,
	}
	if result.AliasRouteScopeCount() != 1 || result.AliasCandidateCount() != len(candidates) {
		t.Fatal("candidate cardinality multiplied route-scope storage")
	}
	typeOfCandidate := reflect.TypeOf(AliasCandidate{})
	for index := 0; index < typeOfCandidate.NumField(); index++ {
		kind := typeOfCandidate.Field(index).Type.Kind()
		if kind == reflect.Slice || kind == reflect.Map {
			t.Fatalf("AliasCandidate field %s retains a variable-width denominator", typeOfCandidate.Field(index).Name)
		}
	}
}
