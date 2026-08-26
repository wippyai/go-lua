package delta

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/eval/step"
	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture/arithmetic"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// TestLaterPublishDifferentialVerticalLaws obtains both Apply extents from the
// mounted arithmetic publication, then redeems those exact application leases
// through the Later Publish bridge. The fixture is production-mounted state;
// this test does not construct a second evaluator or a replacement Door.
func TestLaterPublishDifferentialVerticalLaws(t *testing.T) {
	fixture := arithmetic.New(t, 0xFA)
	execution := fixture.Mounted().Arrangement().Execution()
	schedules := execution.Schedules()
	if len(schedules) != 1 {
		t.Fatalf("arithmetic schedules=%d", len(schedules))
	}
	entry := schedules[0]
	if !entry.Available() || entry.Node().Kind() != algebra.KindPublish {
		t.Fatal("arithmetic Publish entry")
	}

	beforeSession, ok := step.New(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !beforeSession.Available() {
		t.Fatal("before step session")
	}
	beforeResult, ok := beforeSession.Evaluate(entry)
	if !ok || !beforeResult.Available() || len(beforeResult.Applications()) != 1 {
		t.Fatal("before Apply result")
	}
	beforeSettlements := beforeResult.Settlements()
	if len(beforeSettlements) == 0 {
		t.Fatal("before Publish settlements")
	}
	beforeRoot := beforeSettlements[len(beforeSettlements)-1].Next()

	inputDelta, ok := fixture.SourceAChangedDelta(beforeRoot)
	if !ok {
		t.Fatal("source replacement delta")
	}
	later, ok := fixpoint.Later(inputDelta)
	if !ok || !later.Available() {
		t.Fatal("Later root")
	}
	afterSession, ok := step.New(fixture.Mounted(), inputDelta.Next(), fixture.View())
	if !ok || !afterSession.Available() {
		t.Fatal("after step session")
	}
	afterResult, ok := afterSession.Evaluate(entry)
	if !ok || !afterResult.Available() || len(afterResult.Applications()) != 1 {
		t.Fatal("after Apply result")
	}

	beforeApplications := beforeResult.Applications()[0]
	afterApplications := afterResult.Applications()[0]
	replacement, ok := applydifferential.Pair(beforeApplications, afterApplications)
	if !ok || !replacement.Available() || replacement.Len() == 0 {
		t.Fatal("same-address replacement differential")
	}
	for index := 0; index < replacement.Len(); index++ {
		value, valueOK := replacement.At(index)
		if !valueOK || !value.Available() {
			t.Fatalf("replacement differential %d unavailable", index)
		}
		before, beforeOK := value.Before()
		after, afterOK := value.After()
		if !beforeOK || !afterOK || !before.Invocation().Same(after.Invocation()) {
			t.Fatalf("replacement differential %d lost same invocation address", index)
		}
	}

	session, ok := New(fixture.Mounted(), later, fixture.View())
	if !ok || !session.Available() {
		t.Fatal("delta session")
	}
	signed := signedValue{
		node:          entry.Node().Digest(),
		kind:          algebra.KindPublish,
		transitions:   []signedTransition{},
		differentials: []applydifferential.Results{replacement},
		semantic:      true,
	}
	differentialValue, ok := signedPathValue(signed)
	if !ok || !differentialValue.available(session.mounted) {
		t.Fatal("signed Publish path value")
	}

	assertChain := func(t testing.TB, result Result, wantSettlements int) {
		t.Helper()
		if !result.Available() || result.Kind() != algebra.KindPublish {
			t.Fatal("unavailable Publish result")
		}
		if len(result.Applications()) != 0 {
			t.Fatalf("signed transport laundered into positive applications: %d", len(result.Applications()))
		}
		settlements := result.Settlements()
		if len(settlements) != wantSettlements {
			t.Fatalf("settlements=%d want=%d", len(settlements), wantSettlements)
		}
		current := inputDelta.Next()
		for index, settlement := range settlements {
			if !settlement.Available() || !settlement.Base().Same(current) {
				t.Fatalf("settlement %d broke predecessor chain", index)
			}
			delta, deltaOK := settlement.Delta()
			if settlement.Changed() {
				if !deltaOK || !delta.Available() || !delta.Base().Same(settlement.Base()) || !delta.Next().Same(settlement.Next()) {
					t.Fatalf("settlement %d lost exact commit delta", index)
				}
			}
			current = settlement.Next()
		}
		if !result.Next().Same(current) || !result.Base().Same(inputDelta.Base()) {
			t.Fatal("Publish result lost exact base/next chain")
		}
	}

	t.Run("same-address replacement", func(t *testing.T) {
		result, evaluateOK := session.finish(entry, entry.Node(), []pathValue{differentialValue})
		if !evaluateOK {
			t.Fatal("replacement did not reach Publish")
		}
		assertChain(t, result, replacement.Len())
	})

	t.Run("after-only", func(t *testing.T) {
		afterOnly, differentialOK := applydifferential.Pair(apply.Results{}, afterApplications)
		if !differentialOK || !afterOnly.Available() || afterOnly.Len() == 0 {
			t.Fatal("after-only differential")
		}
		signedAfter := signedValue{
			node:          entry.Node().Digest(),
			kind:          algebra.KindPublish,
			transitions:   []signedTransition{},
			differentials: []applydifferential.Results{afterOnly},
			semantic:      true,
		}
		afterValue, valueOK := signedPathValue(signedAfter)
		if !valueOK {
			t.Fatal("after-only Publish path value")
		}
		result, evaluateOK := session.finish(entry, entry.Node(), []pathValue{afterValue})
		if !evaluateOK {
			t.Fatal("after-only did not reach Publish")
		}
		assertChain(t, result, afterOnly.Len())
	})

	t.Run("authored interleaving", func(t *testing.T) {
		ordinary, ordinaryOK := applyValue(entry.Node().Digest(), []apply.Results{beforeApplications})
		if !ordinaryOK {
			t.Fatal("ordinary path value")
		}
		// Differential comes first in authored order. A grouped implementation
		// would publish ordinary first and leave the first settlement as a
		// no-op against the old output root.
		result, evaluateOK := session.finish(entry, entry.Node(), []pathValue{differentialValue, ordinary})
		if !evaluateOK || !result.Available() {
			t.Fatal("interleaved Publish did not finish")
		}
		settlements := result.Settlements()
		if len(settlements) != replacement.Len()+beforeApplications.Len() {
			t.Fatalf("interleaved settlements=%d want=%d", len(settlements), replacement.Len()+beforeApplications.Len())
		}
		if !settlements[0].Changed() {
			t.Fatal("first authored differential was published after ordinary transport")
		}
		current := inputDelta.Next()
		for index, settlement := range settlements {
			if !settlement.Base().Same(current) {
				t.Fatalf("interleaved settlement %d broke root chain", index)
			}
			current = settlement.Next()
		}
		if !result.Next().Same(current) || len(result.Applications()) != 1 {
			t.Fatal("interleaved result lost ordinary observation or final root")
		}
	})
}
