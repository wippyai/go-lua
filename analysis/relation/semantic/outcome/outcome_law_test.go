package outcome_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

func content(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("semantic-outcome-law", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func TestResultRequiresOnlyRefusedToCarryAReason(t *testing.T) {
	if result, ok := outcome.NewResult(outcome.Produced, model.RefusalID{}); !ok || !result.Available() {
		t.Fatalf("produced result rejected")
	}
	owner, ok := model.IssueOwnerID(content(t, "refusal-owner"))
	if !ok {
		t.Fatalf("issue refusal owner")
	}
	reason, ok := model.IssueRefusalID(owner, content(t, "reason"))
	if !ok {
		t.Fatalf("issue refusal reason")
	}
	if result, ok := outcome.NewResult(outcome.Refused, reason); !ok || !result.Available() {
		t.Fatalf("refused result rejected")
	}
	if _, ok := outcome.NewResult(outcome.Refused, model.RefusalID{}); ok {
		t.Fatalf("refused result without reason accepted")
	}
	if _, ok := outcome.NewResult(outcome.Produced, reason); ok {
		t.Fatalf("non-refusal result carried refusal reason")
	}
}

func TestOutcomeVocabularyIsClosedAndAbsenceShapedOutcomesStayDistinct(t *testing.T) {
	if outcome.Invalid.Available() || outcome.Code(255).Available() {
		t.Fatalf("invalid outcome entered closed vocabulary")
	}
	if !outcome.NoCandidate.Available() || !outcome.NoSelection.Available() || outcome.NoCandidate == outcome.NoSelection {
		t.Fatalf("absence-shaped outcomes collapsed")
	}
	if _, ok := outcome.NewSet(outcome.Produced, outcome.Produced); ok {
		t.Fatalf("duplicate outcome arm accepted")
	}
	if _, ok := outcome.NewSet(outcome.Invalid); ok {
		t.Fatalf("invalid outcome arm accepted")
	}
	ordered, ok := outcome.NewSet(outcome.NoCandidate, outcome.NoSelection)
	if !ok || !ordered.Equal(ordered) {
		t.Fatalf("valid ordered outcome set rejected")
	}
	if reversed, ok := outcome.NewSet(outcome.NoSelection, outcome.NoCandidate); !ok || ordered.Equal(reversed) {
		t.Fatalf("outcome order was erased")
	}
}
