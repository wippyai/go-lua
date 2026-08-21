package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// The two retained causal planes are proven at their construction gate. The
// route-index cut is the last phase that writes an Edge or CallBoundary row,
// so sealRows proves every row there, once, in the exact form every later
// reader observes. A read after that gate projects a proven row: it must not
// restate the structural derivation per successor reference, which is the only
// reason the same proof was previously paid once per read.

// TestSealedCausalRowsAreNotRereadPerQuery corrupts a clause the construction
// gate proves and asserts the sealed query answers exactly as before. A query
// that re-derived the row would now fail closed on data it already accepted.
func TestSealedCausalRowsAreNotRereadPerQuery(t *testing.T) {
	fixture := openCausalFixture(t, wideCallSpec(8))
	result := fixture.result
	if !result.rowsSealed {
		t.Fatal("a published causal Result did not seal its retained row planes")
	}
	if len(result.boundaries.rows) == 0 || len(result.edges.rows) == 0 {
		t.Fatalf("fixture retained %d Edge and %d CallBoundary rows; the law needs both planes",
			len(result.edges.rows), len(result.boundaries.rows))
	}

	// An unissued component head is false for every clause validEdgeRow and
	// validBoundaryProof state about membership, and it is carried by neither
	// published projection: the answer below can only change by rederivation.
	unissued := keyspace.MakeTerm(keyspace.FamilyLoop, result.index.familyCounts[keyspace.FamilyLoop]+1)
	if result.componentIssued(unissued) {
		t.Fatal("the corruption term is an issued component head")
	}

	edgeBefore, edgeOK := result.Edges().At(0)
	if !edgeOK {
		t.Fatal("sealed local Edge row 0 was not published")
	}
	boundaryBefore, boundaryOK := result.Boundaries().At(0)
	if !boundaryOK {
		t.Fatal("sealed CallBoundary row 0 was not published")
	}
	total := result.Successors().TotalCount()
	if total == 0 {
		t.Fatal("sealed successor plane is empty")
	}
	routesBefore := make([]Successor, total)
	for index := 0; index < total; index++ {
		route, ok := result.Successors().TotalAt(index)
		if !ok {
			t.Fatalf("sealed successor %d was not published", index)
		}
		routesBefore[index] = route
	}

	result.edges.rows[0].component = unissued
	result.boundaries.rows[0].components[BoundaryThrow] = unissued

	edgeAfter, edgeAfterOK := result.Edges().At(0)
	if !edgeAfterOK || edgeAfter != edgeBefore {
		t.Fatal("the local Edge query re-derived a sealed row instead of projecting it")
	}
	boundaryAfter, boundaryAfterOK := result.Boundaries().At(0)
	if !boundaryAfterOK || boundaryAfter != boundaryBefore {
		t.Fatal("the CallBoundary query re-derived a sealed row instead of projecting it")
	}
	if _, ok := result.Boundaries().For(boundaryBefore.Call); !ok {
		t.Fatal("the typed Call lookup re-derived a sealed row instead of projecting it")
	}
	for index := 0; index < total; index++ {
		route, ok := result.Successors().TotalAt(index)
		if !ok {
			t.Fatalf("successor %d disappeared from the sealed union plane", index)
		}
		before := routesBefore[index]
		if route.From != before.From || route.To != before.To || route.Decision != before.Decision ||
			route.Truth != before.Truth || route.Mu != before.Mu || route.Arm != before.Arm ||
			route.routeDigest != before.routeDigest {
			t.Fatalf("successor %d changed after a sealed row was corrupted", index)
		}
	}

	result.edges.rows[0].component = edgeRow{}.component
	result.boundaries.rows[0].components[BoundaryThrow] = 0
}

// TestRowSealFailsClosedOnMalformedRow is the other half of the same judgment.
// Moving the proof to the seal is only sound because the seal refuses to
// publish a plane it cannot prove.
func TestRowSealFailsClosedOnMalformedRow(t *testing.T) {
	result := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)

	if err := result.sealRows(); err == nil {
		t.Fatal("row seal ran before the route directory was cut")
	}

	result.routesReady = true
	result.edges.rows = []edgeRow{{Edge: Edge{From: body, To: body}}}
	if err := result.sealRows(); err == nil {
		t.Fatal("row seal admitted a Mu-less self Edge")
	}
	if result.rowsSealed {
		t.Fatal("a failed row seal published a sealed plane")
	}

	result.edges.rows = []edgeRow{{Edge: Edge{From: body, To: outcome}}}
	if err := result.sealRows(); err != nil {
		t.Fatalf("row seal refused a well-formed plane: %v", err)
	}
	if !result.rowsSealed {
		t.Fatal("a successful row seal did not publish its proof")
	}
	if err := result.sealRows(); err == nil {
		t.Fatal("row seal ran a second time on an already sealed plane")
	}
}
