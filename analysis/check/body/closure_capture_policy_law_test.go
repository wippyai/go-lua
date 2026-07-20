package body

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// TestClosureCapturePolicyTierIsCentrallySelectedByWriteStatus pins
// invariants.md #11: capture precision is tiered and centrally selected.
// closureCapturePolicy is the sole selector every capture routes through: a
// captured symbol that is never reassigned gets the precise full-graph tier,
// and a captured symbol that is reassigned anywhere in the body gets the
// coarser write-invariant tier, regardless of which capture asset is
// involved. inner captures both a plain scalar (never written) and a scalar
// written after the closure is created, so the same ForEachClosureCaptureFact
// walk exercises both tiers under one central law.
func TestClosureCapturePolicyTierIsCentrallySelectedByWriteStatus(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function outer()
	local unwritten = 1
	local written = 2
	local function inner()
		return unwritten, written
	end
	written = 3
	return inner
end
`)
	prepared, err := PrepareFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	entry, initial := factory.SeedEntry(state.State{}, nil)
	coordinates := reachableFactoryPublicationCoordinates(factory)
	result, err := factory.PublishResult(ResultPublicationConfig{
		Coordinates: coordinates,
		Solve:       SolveConfig{Context: ctx},
		SeededEntry: entry,
		Initial:     initial,
	})
	if err != nil {
		t.Fatal(err)
	}

	gotPolicy := map[string]ClosureCapturePolicy{}
	if !result.ForEachClosureCaptureFact(func(fact ClosureCaptureFact) bool {
		gotPolicy[fact.Name] = fact.Policy
		return true
	}) {
		t.Fatal("no closure capture facts were exported")
	}

	unwritten, ok := gotPolicy["unwritten"]
	if !ok {
		t.Fatal("capture fact for \"unwritten\" is missing")
	}
	if unwritten != ClosureCapturePolicyFull {
		t.Fatalf("policy(unwritten) = %s, want full", unwritten)
	}

	written, ok := gotPolicy["written"]
	if !ok {
		t.Fatal("capture fact for \"written\" is missing")
	}
	if written != ClosureCapturePolicyWriteInvariant {
		t.Fatalf("policy(written) = %s, want write-invariant", written)
	}
}
