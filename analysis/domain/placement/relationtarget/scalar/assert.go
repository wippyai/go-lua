package placementscalar

import (
	"testing"

	placementquery "github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	scalarfixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/scalar"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func assertScalarFact(t *testing.T, fixture scalarfixture.Fixture, result terminal.Result, rows placementquery.Rows) {
	t.Helper()
	if !result.Available() {
		t.Fatal("target scalar solve result")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("target scalar solve counters: evaluations=%d publications=%d, want 1/1", result.Evaluations(), result.Publications())
	}
	if !rows.Available() || rows.Len() != 1 {
		t.Fatalf("target scalar typed facts: available=%v rows=%d", rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.HasLineage() || !row.Presence().Is(model.Present) {
		t.Fatal("target scalar typed fact metadata")
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
		t.Fatalf("target scalar typed fact=%#v, want=%#v", fact, fixture.Expected())
	}
}
