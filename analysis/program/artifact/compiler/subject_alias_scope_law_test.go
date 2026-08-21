package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestAuthenticatedCausalRoutesPreservesEmptyScope(t *testing.T) {
	if !authenticatedCausalRoutes(nil, nil, false) {
		t.Fatal("empty route scope must not require a causal projection")
	}
	if !authenticatedCausalRoutes([]identity.ContentID{}, nil, false) {
		t.Fatal("empty route scope must remain admitted with an unavailable multiplicity")
	}
}

func TestAuthenticatedCausalRoutesRequiresOneValidPublishedRoute(t *testing.T) {
	route := identity.ContentID{1}
	cases := []struct {
		name         string
		routes       []identity.ContentID
		multiplicity map[identity.ContentID]int
		available    bool
		want         bool
	}{
		{name: "unique", routes: []identity.ContentID{route}, multiplicity: map[identity.ContentID]int{route: 1}, available: true, want: true},
		{name: "duplicate", routes: []identity.ContentID{route}, multiplicity: map[identity.ContentID]int{route: 2}, available: true, want: false},
		{name: "missing", routes: []identity.ContentID{route}, multiplicity: map[identity.ContentID]int{}, available: true, want: false},
		{name: "unavailable-plane", routes: []identity.ContentID{route}, multiplicity: nil, available: false, want: false},
		{name: "invalid-route", routes: []identity.ContentID{{}}, multiplicity: map[identity.ContentID]int{{}: 1}, available: true, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := authenticatedCausalRoutes(test.routes, test.multiplicity, test.available); got != test.want {
				t.Fatalf("authenticatedCausalRoutes() = %v, want %v", got, test.want)
			}
		})
	}
}
