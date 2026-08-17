package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestMalformedEdgeAndResetCombinationsFailClosed(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	r.reset.headRanges[keyspace.FamilyLoop] = make([]range32, 2)
	r.reset.headRanges[keyspace.FamilyLoop][1] = range32{start: 0, end: 2}
	r.reset.streams = []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilySelect, 1),
		keyspace.MakeTerm(keyspace.FamilyBranch, 1),
	}
	cases := []edgeRow{
		{Edge: Edge{From: body, To: outcome, Truth: true}},
		{Edge: Edge{From: body, To: outcome, Decision: body}},
		{Edge: Edge{From: body, To: outcome}, resetStart: 1, resetPast: 1},
		{Edge: Edge{From: body, To: outcome, Mu: body}},
		{Edge: Edge{From: body, To: outcome, Mu: loop}, resetStart: 1, resetPast: 0},
		{Edge: Edge{From: body, To: outcome, Mu: loop}, resetStart: 0, resetPast: 3},
		{Edge: Edge{From: body, To: outcome, Mu: loop}, resetDigest: identity.ContentID{7}},
	}
	for index, row := range cases {
		r.edges.rows = []edgeRow{row}
		if _, ok := r.Edges().At(0); ok {
			t.Fatalf("malformed Edge combination %d was observable: %#v", index, row)
		}
	}

	// The stream term itself must also be inside the sealed Select/Branch/Loop
	// denominators; family-shaped terms with a foreign ordinal are malformed.
	r.reset.streams = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilySelect, 2)}
	r.edges.rows = []edgeRow{{Edge: Edge{From: body, To: outcome, Mu: loop}, resetStart: 0, resetPast: 1}}
	if err := rebuildSyntheticSuccessors(r, r.edges.rows, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.buildRouteIndex(); err == nil {
		t.Fatal("reset stream term outside the sealed family denominator was accepted")
	}
}
