package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestDiagnosticObservationModelRequiresUniqueEvidencePoints(t *testing.T) {
	point := identity.ContentID{1}
	if !validDiagnosticEvidencePoints([]identity.ContentID{point}) {
		t.Fatal("single evidence point was rejected")
	}
	if validDiagnosticEvidencePoints([]identity.ContentID{point, point}) {
		t.Fatal("duplicate evidence point was admitted")
	}
	if validDiagnosticEvidencePoints(nil) {
		t.Fatal("empty evidence set was admitted")
	}
}
