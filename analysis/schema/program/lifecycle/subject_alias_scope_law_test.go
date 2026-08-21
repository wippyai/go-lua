package lifecycle

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func subjectAliasScopeLawID(t *testing.T, seed byte) identity.ContentID {
	t.Helper()
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestSubjectAliasRouteScopeRequiresCanonicalRoutesAndChecksum(t *testing.T) {
	sourceScope := subjectAliasScopeLawID(t, 1)
	body := subjectAliasScopeLawID(t, 33)
	routes := []identity.ContentID{subjectAliasScopeLawID(t, 65), subjectAliasScopeLawID(t, 97)}
	id, idOK := SubjectAliasRouteScopeIdentity(sourceScope, SubjectAliasRouteScopeBody, body, routes)
	if !idOK {
		t.Fatal("route scope identity")
	}
	row, rowOK := NewSubjectAliasRouteScope(id, sourceScope, SubjectAliasRouteScopeBody, body, 0, routes)
	if !rowOK || !row.Available() || row.Kind() != SubjectAliasRouteScopeBody || row.BodyID() != body {
		t.Fatal("body route scope row was not admitted")
	}
	offset, count, spanOK := row.MemberSpan()
	if !spanOK || offset != 0 || count != uint32(len(routes)) {
		t.Fatalf("member span = %d/%d, want 0/%d", offset, count, len(routes))
	}
	if _, ok := SubjectAliasRouteScopeIdentity(sourceScope, SubjectAliasRouteScopeBody, body, []identity.ContentID{routes[1], routes[0]}); ok {
		t.Fatal("unsorted route denominator was admitted")
	}
	forged, forgedOK := NewSubjectAliasRouteScope(id, sourceScope, SubjectAliasRouteScopeBody, subjectAliasScopeLawID(t, 200), 0, routes)
	if forgedOK || forged.Available() {
		t.Fatal("body scope reused a checksum computed for a different body")
	}
}

func TestSubjectAliasRouteScopeMemberBindsOneRouteToItsScope(t *testing.T) {
	sourceScope := subjectAliasScopeLawID(t, 1)
	body := subjectAliasScopeLawID(t, 33)
	routes := []identity.ContentID{subjectAliasScopeLawID(t, 65), subjectAliasScopeLawID(t, 97)}
	scopeID, scopeIDOK := SubjectAliasRouteScopeIdentity(sourceScope, SubjectAliasRouteScopeBody, body, routes)
	if !scopeIDOK {
		t.Fatal("route scope identity")
	}
	for ordinal, route := range routes {
		memberID, memberIDOK := SubjectAliasRouteScopeMemberIdentity(scopeID, uint32(ordinal), route)
		if !memberIDOK {
			t.Fatal("route scope member identity")
		}
		member, memberOK := NewSubjectAliasRouteScopeMember(memberID, scopeID, uint32(ordinal), route)
		if !memberOK || !member.Available() || member.RouteID() != route || member.ScopeID() != scopeID {
			t.Fatal("route scope member row was not admitted")
		}
		if got, ok := member.Ordinal(); !ok || got != uint32(ordinal) {
			t.Fatal("route scope member did not preserve its ordinal")
		}
	}
	if _, ok := NewSubjectAliasRouteScopeMember(scopeID, scopeID, 0, routes[0]); ok {
		t.Fatal("member accepted an identity computed for a different ordinal")
	}
}

func TestSubjectAliasCandidateCarriesExplicitOpenClosedState(t *testing.T) {
	sourceCandidate := subjectAliasScopeLawID(t, 1)
	candidate := subjectAliasScopeLawID(t, 33)
	scope := subjectAliasScopeLawID(t, 65)
	closedID, closedIDOK := SubjectAliasCandidateIdentity(sourceCandidate, SubjectLivenessCell, candidate, scope, true)
	openID, openIDOK := SubjectAliasCandidateIdentity(sourceCandidate, SubjectLivenessCell, candidate, scope, false)
	if !closedIDOK || !openIDOK {
		t.Fatal("candidate identity")
	}
	closed, closedOK := NewSubjectAliasCandidate(closedID, sourceCandidate, SubjectLivenessCell, candidate, scope, true)
	open, openOK := NewSubjectAliasCandidate(openID, sourceCandidate, SubjectLivenessCell, candidate, scope, false)
	if !closedOK || !closed.Available() || !closed.Closed() || !openOK || !open.Available() || open.Closed() || open.ID() == closed.ID() {
		t.Fatal("open and closed candidates over one scope collapsed")
	}
	if forged, forgedOK := NewSubjectAliasCandidate(closedID, sourceCandidate, SubjectLivenessCell, candidate, scope, false); forgedOK || forged.Available() {
		t.Fatal("closed state reused an open checksum")
	}
}

func TestSubjectAliasColdRowsContainNoVariableWidthDenominator(t *testing.T) {
	for _, rowType := range []reflect.Type{
		reflect.TypeOf(SubjectAliasRouteScope{}),
		reflect.TypeOf(SubjectAliasRouteScopeMember{}),
		reflect.TypeOf(SubjectAliasCandidate{}),
	} {
		for index := 0; index < rowType.NumField(); index++ {
			kind := rowType.Field(index).Type.Kind()
			if kind == reflect.Slice || kind == reflect.Map {
				t.Fatalf("%s.%s retains a variable-width denominator", rowType.Name(), rowType.Field(index).Name)
			}
		}
	}
}

func TestSubjectEventAdmitsOnlyAuthenticatedAliasAndUnknownKinds(t *testing.T) {
	sourceEvent := subjectAliasScopeLawID(t, 3)
	path := subjectAliasScopeLawID(t, 4)
	subject := subjectAliasScopeLawID(t, 5)
	related := subjectAliasScopeLawID(t, 6)
	eventID, eventIDOK := SubjectEventIdentity(sourceEvent, path, SubjectEventAlias, 1, 0, SubjectLivenessCell, subject, SubjectLivenessValue, related)
	if !eventIDOK {
		t.Fatal("alias event identity")
	}
	event, eventOK := NewSubjectEvent(
		eventID,
		sourceEvent,
		path,
		SubjectEventAlias,
		1,
		0,
		SubjectLivenessCell,
		subject,
		SubjectLivenessValue,
		related,
	)
	if !eventOK || !event.Available() || event.Kind() != SubjectEventAlias {
		t.Fatal("alias event row was not admitted")
	}
	unknownID, idOK := SubjectEventIdentity(event.SourceEventID(), event.PathID(), SubjectEventUnknown, event.Role(), 0, event.SubjectKind(), event.SubjectID(), event.RelatedKind(), event.RelatedID())
	if !idOK {
		t.Fatal("unknown event identity")
	}
	unknown, unknownOK := NewSubjectEvent(unknownID, event.SourceEventID(), event.PathID(), SubjectEventUnknown, event.Role(), 0, event.SubjectKind(), event.SubjectID(), event.RelatedKind(), event.RelatedID())
	if !unknownOK || unknown.Kind() != SubjectEventUnknown {
		t.Fatal("unknown event row was not admitted")
	}
	if _, invalid := NewSubjectEvent(identity.ContentID{}, event.SourceEventID(), event.PathID(), SubjectEventAlias, event.Role(), 0, event.SubjectKind(), event.SubjectID(), event.RelatedKind(), event.RelatedID()); invalid {
		t.Fatal("unavailable event identity was admitted")
	}
}
