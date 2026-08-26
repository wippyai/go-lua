// Package relationtarget_test keeps discoverable external consumers for the
// neutral targetfixture Placement specimens. Schema construction remains in
// the family packages, so this package only observes their public API.
package relationtarget_test

import (
	"testing"

	placementquery "github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

type targetFactFixture interface {
	Solve() (terminal.Result, bool)
	Facts(terminal.Result) (placementquery.Rows, bool)
	Expected() placementdomain.Fact
}

func assertTargetFact(t *testing.T, name string, fixture targetFactFixture) {
	t.Helper()
	result, ok := fixture.Solve()
	if !ok || !result.Available() {
		t.Fatalf("%s target solve unavailable", name)
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("%s target counters evaluations=%d publications=%d, want 1/1", name, result.Evaluations(), result.Publications())
	}
	rows, ok := fixture.Facts(result)
	if !ok || !rows.Available() || rows.Len() != 1 {
		t.Fatalf("%s target rows available=%t count=%d, want one", name, ok && rows.Available(), rows.Len())
	}
	row, ok := rows.At(0)
	if !ok || !row.Available() || !row.Presence().Is(model.Present) || !row.HasLineage() {
		t.Fatalf("%s target output metadata", name)
	}
	fact, ok := row.Fact()
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected()) {
		t.Fatalf("%s target fact=%#v expected=%#v decoded=%t", name, fact, fixture.Expected(), ok)
	}
}
