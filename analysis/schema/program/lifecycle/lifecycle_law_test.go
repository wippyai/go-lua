package lifecycle

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func lifecycleLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("program-lifecycle-law/"+name, nil)
	if !ok {
		t.Fatalf("derive %s", name)
	}
	return id
}

func lifecycleLawView(t *testing.T, publication Publication, catalog identity.ContentID) View {
	t.Helper()
	builder := snapshot.NewFrozen(catalog, identity.StoreID(1))
	// The lifecycle publication owns storage-cell-lifetime, the liveness span
	// plane and the yield boundary sequence; call-result sits between them and
	// belongs to another publication, so the filler supplies it.
	for slot := uint32(0); slot < 59; slot++ {
		if slot >= 55 && slot <= 57 {
			continue
		}
		axis := snapshot.Axis[uint32, uint32]{SchemaID: catalog, Slot: slot}
		content := snapshot.Content[uint32, uint32]{
			Sequence:    []uint32{},
			Denominator: lifecycleLawID(t, fmt.Sprintf("filler-%d", slot)),
		}
		if err := snapshot.PutFrozenColumn(&builder, axis, content); err != nil {
			t.Fatalf("put filler slot %d: %v", slot, err)
		}
	}
	if !publication.Append(&builder, catalog) {
		t.Fatal("lifecycle publication append")
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal lifecycle publication: %v", err)
	}
	state, ok := programstate.New(frozen, catalog)
	if !ok {
		t.Fatal("open program state")
	}
	view, ok := NewView(state)
	if !ok {
		t.Fatal("open lifecycle view")
	}
	return view
}

func TestLifecycleFamiliesBindCanonicalSlots(t *testing.T) {
	if got, want := StorageCellLifetimeFamily().Definition(), programcatalog.StorageCellLifetime(); got != want {
		t.Fatalf("storage lifetime definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
	if got, want := SubjectYieldBoundaryFamily().Definition(), programcatalog.SubjectYieldBoundary(); got != want {
		t.Fatalf("subject yield boundary definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
	if got, want := SubjectLivenessSpanFamily().Definition(), programcatalog.SubjectLivenessSpan(); got != want {
		t.Fatalf("subject liveness span definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
	if got, want := SubjectAliasRouteScopeFamily().Definition(), programcatalog.SubjectAliasRouteScope(); got != want {
		t.Fatalf("alias route scope definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
	if got, want := SubjectAliasRouteScopeMemberFamily().Definition(), programcatalog.SubjectAliasRouteScopeMember(); got != want {
		t.Fatalf("alias route member definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
	if got, want := SubjectAliasCandidateFamily().Definition(), programcatalog.SubjectAliasCandidate(); got != want {
		t.Fatalf("alias candidate definition = %d/%s, want %d/%s", got.Slot(), got.Name(), want.Slot(), want.Name())
	}
}

func TestLifecyclePublicationAppendsAllEmptyColumns(t *testing.T) {
	view := lifecycleLawView(t, Publication{}, lifecycleLawID(t, "empty-catalog"))
	if count, published := view.StorageCellLifetimeCount(); !published || count != 0 {
		t.Fatalf("storage lifetime count/published = %d/%v", count, published)
	}
	if count, published := view.SubjectYieldBoundaryCount(); !published || count != 0 {
		t.Fatalf("subject yield boundary count/published = %d/%v", count, published)
	}
	if count, published := view.SubjectLivenessSpanCount(); !published || count != 0 {
		t.Fatalf("subject liveness span count/published = %d/%v", count, published)
	}
	if count, published := view.AliasRouteScopeCount(); !published || count != 0 {
		t.Fatalf("alias route scope count/published = %d/%v", count, published)
	}
	if count, published := view.AliasRouteScopeMemberCount(); !published || count != 0 {
		t.Fatalf("alias route member count/published = %d/%v", count, published)
	}
	if count, published := view.AliasCandidateCount(); !published || count != 0 {
		t.Fatalf("alias candidate count/published = %d/%v", count, published)
	}
	if _, held := view.StorageCellLifetimeForID(lifecycleLawID(t, "missing-cell")); held {
		t.Fatal("missing storage lifetime resolved")
	}
	if _, held := view.SubjectYieldBoundaryFor(lifecycleLawID(t, "route")); held {
		t.Fatal("missing subject yield boundary resolved")
	}
	if _, held := view.SubjectLivenessAtBoundary(lifecycleLawID(t, "route"), SubjectLivenessRoot, lifecycleLawID(t, "subject")); held {
		t.Fatal("missing subject liveness resolved")
	}
}

func TestAliasRouteScopeViewBorrowsOneCanonicalMemberSpan(t *testing.T) {
	sourceScope := lifecycleLawID(t, "source-scope")
	body := lifecycleLawID(t, "body")
	routes := []identity.ContentID{lifecycleLawID(t, "route-a"), lifecycleLawID(t, "route-b")}
	if bytes.Compare(routes[0][:], routes[1][:]) > 0 {
		routes[0], routes[1] = routes[1], routes[0]
	}
	scopeID, scopeIDOK := SubjectAliasRouteScopeIdentity(sourceScope, SubjectAliasRouteScopeBody, body, routes)
	scope, scopeOK := NewSubjectAliasRouteScope(scopeID, sourceScope, SubjectAliasRouteScopeBody, body, 0, routes)
	if !scopeIDOK || !scopeOK {
		t.Fatal("alias route scope")
	}
	members := make([]SubjectAliasRouteScopeMember, len(routes))
	for ordinal, route := range routes {
		id, idOK := SubjectAliasRouteScopeMemberIdentity(scopeID, uint32(ordinal), route)
		member, memberOK := NewSubjectAliasRouteScopeMember(id, scopeID, uint32(ordinal), route)
		if !idOK || !memberOK {
			t.Fatal("alias route scope member")
		}
		members[ordinal] = member
	}
	view := lifecycleLawView(t, Publication{AliasRouteScopes: []SubjectAliasRouteScope{scope}, AliasRouteMembers: members}, lifecycleLawID(t, "scope-catalog"))
	resolved, resolvedOK := view.AliasRouteScopeForID(scopeID)
	if !resolvedOK || resolved.ID() != scopeID {
		t.Fatal("alias route scope lookup")
	}
	var borrowed []SubjectAliasRouteScopeMember
	allocations := testing.AllocsPerRun(100, func() {
		borrowed, _ = view.AliasRouteScopeMembers(resolved)
	})
	if allocations != 0 || len(borrowed) != len(routes) {
		t.Fatalf("borrowed scope members=%d allocations=%v", len(borrowed), allocations)
	}
	for index, member := range borrowed {
		if member.RouteID() != routes[index] {
			t.Fatal("borrowed scope member order changed")
		}
	}
}
