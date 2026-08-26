package derivation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Complete replay is deliberately narrower than Complete itself.  A shape
// outside Complete(Select(Input)) may remain a valid full-evaluation plan,
// but it never receives a delta replay witness that could be mistaken for a
// complete successor extent.
func TestCompleteReplayRefusesUnsealedChildShape(t *testing.T) {
	fixture := newShapeFixture(t)
	expression := algebra.NewComplete(algebra.NewInput(fixture.relationC), fixtureDenominator(t, fixture))
	plan, ok := Build(fixture.root, expression, fixture.bindings, fixture.inputs, nil)
	if !ok || !plan.Available() || plan.Len() != 1 {
		t.Fatalf("unsealed Complete plan = (%v, %v), want ordinary plan", ok, plan.Len())
	}
	path, ok := plan.PathAt(0)
	if !ok || path.FrameCount() != 1 {
		t.Fatal("unsealed Complete path")
	}
	frame, ok := path.FrameAt(0)
	if !ok || frame.Kind() != algebra.KindComplete {
		t.Fatal("unsealed Complete frame")
	}
	if replay := frame.CompleteReplay(); replay.Available() {
		t.Fatal("arbitrary Complete child received a replay witness")
	}
}

func fixtureDenominator(t *testing.T, fixture shapeFixture) (result model.DenominatorRef) {
	t.Helper()
	key, ok := model.IssueKeyID(fixture.relationC, identity.ContentID{13})
	if !ok {
		t.Fatal("fixture denominator key")
	}
	result, ok = model.NewDenominatorRef(fixture.relationC, key)
	if !ok {
		t.Fatal("fixture denominator")
	}
	return result
}
