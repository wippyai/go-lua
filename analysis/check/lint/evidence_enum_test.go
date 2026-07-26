package lint

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestEvidenceEnumConvertersRejectUnknownValues(t *testing.T) {
	if _, err := evidenceKind(diagnostic.EvidenceKind(255)); err == nil {
		t.Fatal("unknown evidence kind was accepted")
	}
	if _, err := evidenceTrust(diagnostic.TrustKind(255)); err == nil {
		t.Fatal("unknown evidence trust was accepted")
	}
	if _, err := evidenceReason(diagnostic.EvidenceReason(255)); err == nil {
		t.Fatal("unknown evidence reason was accepted")
	}
}
