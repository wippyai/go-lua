package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSemanticRouteIdentityResolvesThroughExistingRef(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: body, To: outcome}}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.buildRouteIndex(); err != nil {
		t.Fatal(err)
	}
	successor, ok := r.Successors().At(body, 0)
	if !ok {
		t.Fatal("sealed successor is unavailable")
	}
	routeIdentity, ok := successor.Identity()
	if !ok || !routeIdentity.available() {
		t.Fatal("sealed successor did not publish an owner-fenced identity")
	}
	resolved, ok := r.Successors().Resolve(routeIdentity)
	if !ok || resolved.From != successor.From || resolved.To != successor.To || resolved.Arm != successor.Arm {
		t.Fatalf("Resolve(identity) = %#v/%v, want %#v/true", resolved, ok, successor)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		_, _ = r.Successors().Resolve(routeIdentity)
	}); allocs != 0 {
		t.Fatalf("Resolve(identity) allocates %v times", allocs)
	}
	foreign := routeIdentity
	foreign.SourceID = identity.ContentID{9}
	if _, ok := r.Successors().Resolve(foreign); ok {
		t.Fatal("foreign owner identity resolved in this causal authority")
	}
}
