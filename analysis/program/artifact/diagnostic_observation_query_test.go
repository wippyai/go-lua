package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestDiagnosticObservationQueryDoesNotExposeOutOfRangeEvidence(t *testing.T) {
	payload := diagnosticBranchConditionRow{decision: identity.ContentID{1}, value: identity.ContentID{2}, points: []identity.ContentID{{3}}}
	if point, ok := payload.EvidencePointAt(0); !ok || point != (identity.ContentID{3}) {
		t.Fatal("evidence point query lost its value")
	}
	if _, ok := payload.EvidencePointAt(-1); ok {
		t.Fatal("negative evidence index was admitted")
	}
	if _, ok := payload.EvidencePointAt(payload.EvidencePointCount()); ok {
		t.Fatal("evidence denominator was admitted")
	}
}
