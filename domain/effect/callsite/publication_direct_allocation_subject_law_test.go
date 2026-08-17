package callsite

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func directAllocationSubjectLawID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte("publication-direct-allocation-subject/" + label)))
}

// The positive owner join is intentionally exercised at the Plan boundary,
// where a real solved Effect proof and live Pack/Heap binding coexist. This
// package law verifies the detached carrier cannot be made valid by changing
// either of its committed cross-owner identities, and that zero or missing
// runtime evidence never fabricates a direct allocation admission.
func TestPublicationDirectAllocationSubjectDetachedSealLaw(t *testing.T) {
	receipt := PublicationDirectAllocationSubject{
		correlation: directAllocationSubjectLawID("correlation"),
		direct:      directAllocationSubjectLawID("direct"),
	}
	receipt.id = publicationDirectAllocationSubjectID(receipt.correlation, receipt.direct)
	if !receipt.Valid() {
		t.Fatal("detached direct allocation subject invalid")
	}
	splicedCorrelation := receipt
	splicedCorrelation.correlation = directAllocationSubjectLawID("foreign-correlation")
	if splicedCorrelation.Valid() {
		t.Fatal("correlation splice survived detached seal")
	}
	splicedDirect := receipt
	splicedDirect.direct = directAllocationSubjectLawID("foreign-direct")
	if splicedDirect.Valid() {
		t.Fatal("direct receipt splice survived detached seal")
	}
	if _, admitted := NewPublicationDirectAllocationSubject(PublicationPlacementCorrelationCandidate{}, pack.RuntimeAllocationContextBinding{}, valuedomain.DirectAllocationSubject{}); admitted {
		t.Fatal("missing correlation/runtime/direct evidence issued identity")
	}
}
