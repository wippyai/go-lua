package query_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	fixtureplacement "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/allocationbirth"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	snapshotprojection "github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func TestPlacementFactReadUsesTheRealCodecAndStableMembers(t *testing.T) {
	fixture := fixtureplacement.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !result.Available() {
		t.Fatal("placement solve")
	}
	published, ok := snapshotprojection.Publish(result, fixture.View())
	if !ok || !published.Available() {
		t.Fatal("placement canonical projection")
	}
	column, ok := query.NewFactColumn(fixture.IDs().OutputPayload, fixture.PlacementCodec())
	if !ok {
		t.Fatal("placement fact column")
	}
	rows, ok := query.Read(published, column)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("typed placement rows = available:%v len:%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || row.Key().Relation != fixture.IDs().Output || !row.Key().Scope.Available() || !row.HasLineage() {
		t.Fatal("typed placement row metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) || (!row.Presence().Is(model.Present) && !row.Presence().Is(model.AuthenticatedOpaque)) {
		t.Fatalf("typed placement row = fact:%#v presence:%v", fact, row.Presence())
	}
}

func TestPlacementFactReadNearestNegativesFailClosed(t *testing.T) {
	fixture := fixtureplacement.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok {
		t.Fatal("placement solve")
	}
	published, ok := snapshotprojection.Publish(result, fixture.View())
	if !ok {
		t.Fatal("placement canonical projection")
	}
	ids := fixture.IDs()
	column, ok := query.NewFactColumn(ids.OutputPayload, fixture.PlacementCodec())
	if !ok {
		t.Fatal("placement fact column")
	}
	keys := published.Keys(ids.OutputPayload)
	if len(keys) != 1 {
		t.Fatalf("output members = %d, want one", len(keys))
	}

	// A matching column identity paired with a wrong semantic type is rejected
	// before any token can be redeemed.
	foreignTypeContent, ok := identity.DeriveContentID("placement/query/foreign-type")
	if !ok {
		t.Fatal("foreign type content")
	}
	foreignType, ok := model.IssueTypeID(ids.PlacementOwner, foreignTypeContent)
	if !ok {
		t.Fatal("foreign type")
	}
	foreignStoreTag, ok := identity.DeriveContentID("placement/query/foreign-store")
	if !ok {
		t.Fatal("foreign store content")
	}
	foreignStore, ok := relbindgen.NewStore[placementdomain.Fact](foreignStoreTag, 1)
	if !ok {
		t.Fatal("foreign store")
	}
	foreignCodec, ok := relbindgen.NewColumn(foreignType, foreignStore)
	if !ok {
		t.Fatal("foreign codec")
	}
	foreignColumn, ok := query.NewFactColumn(ids.OutputPayload, foreignCodec)
	if !ok {
		t.Fatal("foreign fact column declaration")
	}
	if _, ok := query.Read(published, foreignColumn); ok {
		t.Fatal("foreign typed column redeemed")
	}

	// A scope token issued by a different mount is invalid even when the
	// relation, row and column shapes are otherwise identical.
	foreignFixture := fixtureplacement.New(t, 0xB2)
	foreignResult, ok := runtime.Solve(foreignFixture.Mounted(), foreignFixture.Base(), foreignFixture.View())
	if !ok {
		t.Fatal("foreign placement solve")
	}
	foreignProjection, ok := snapshotprojection.Publish(foreignResult, foreignFixture.View())
	if !ok {
		t.Fatal("foreign placement projection")
	}
	foreignKeys := foreignProjection.Keys(foreignFixture.IDs().OutputPayload)
	if len(foreignKeys) != 1 {
		t.Fatalf("foreign output members = %d, want one", len(foreignKeys))
	}
	if _, status, ok := query.ReadOne(published, column, foreignKeys[0]); ok || status != canonical.ReadInvalid {
		t.Fatalf("foreign scope read = ok:%v status:%v, want invalid", ok, status)
	}

	// A same-scope row outside the denominator remains a miss, not an
	// invented Bottom/Unknown Fact. The reader exposes that distinction.
	missingContent, ok := identity.DeriveContentID("placement/query/missing-row")
	if !ok {
		t.Fatal("missing row content")
	}
	missingRow, ok := model.IssueRowID(ids.Output, missingContent)
	if !ok {
		t.Fatal("missing row")
	}
	missingKey := keys[0]
	missingKey.Row = missingRow
	if _, status, ok := query.ReadOne(published, column, missingKey); ok || status != canonical.ReadMiss {
		t.Fatalf("missing row read = ok:%v status:%v, want miss", ok, status)
	}
}
