package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestLocalProjectionUsesOnlyIssuedCausalRows(t *testing.T) {
	build := func(t *testing.T) (*Result, Successor, Site, Region) {
		t.Helper()
		r := syntheticResult()
		body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
		selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
		if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: body, To: selectTerm}, component: loop}}, nil); err != nil {
			t.Fatal(err)
		}
		issueSyntheticSites(r, []siteRow{
			{term: body, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, body)},
			{term: selectTerm, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, selectTerm)},
		})
		if !r.buildLocal() {
			t.Fatal("failed to project issued local component")
		}
		successor, ok := r.Successors().At(body, 0)
		if !ok {
			t.Fatal("issued local successor disappeared")
		}
		site, ok := r.SiteForTerm(body)
		if !ok {
			t.Fatal("issued local site disappeared")
		}
		region, ok := r.Local().ForSuccessor(successor)
		if !ok || !region.Available() || !region.ContainsSuccessor(successor) || !region.ContainsSite(site) {
			t.Fatal("region lost its existing causal route/site references")
		}
		if head, ok := region.Head(); !ok || head != loop {
			t.Fatalf("region head = %v/%v, want %v/true", head, ok, loop)
		}
		if region.SuccessorCount() != 1 || region.SiteCount() != 2 {
			t.Fatalf("region traversal counts = routes %d sites %d, want 1/2", region.SuccessorCount(), region.SiteCount())
		}
		if route, ok := region.SuccessorAt(0); !ok || !route.IsLocal() || route.From != body || route.To != selectTerm {
			t.Fatalf("region route traversal lost existing successor: %#v/%v", route, ok)
		}
		if endpoint, ok := region.SiteAt(0); !ok || !endpoint.Available() {
			t.Fatal("region site traversal lost existing site")
		}
		return r, successor, site, region
	}
	_, successor, site, first := build(t)
	_, foreignSuccessor, foreignSite, second := build(t)
	if first.ID() != second.ID() {
		t.Fatal("equivalent issued component changed stable local identity")
	}
	if _, ok := first.result.Local().ForSuccessor(foreignSuccessor); ok {
		t.Fatal("foreign successor entered local inverse")
	}
	if regions := first.result.Local().RegionCountForSite(foreignSite); regions != 0 {
		t.Fatal("foreign site entered local inverse")
	}
	if !first.ContainsSuccessor(successor) || !first.ContainsSite(site) {
		t.Fatal("local membership was not stable")
	}
}
