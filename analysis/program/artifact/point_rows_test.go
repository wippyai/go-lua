package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestPointRowsKeepRouteAndTransferVocabulariesSeparate(t *testing.T) {
	if !RouteLocal.Valid() || !RouteCancel.Valid() || RouteInvalid.Valid() {
		t.Fatal("route vocabulary validity is not closed")
	}
	point := Point{id: valuesLawID(1), decisions: []identity.ContentID{valuesLawID(2)}, initial: true}
	if !point.Available() || point.DecisionCount() != 1 {
		t.Fatal("valid point row unavailable")
	}
	transfer := LocalTransfer{id: valuesLawID(3), from: valuesLawID(4), to: valuesLawID(5), full: true}
	if !transfer.Available() || !transfer.FullEnvironment() || transfer.FactorRoleCount() != 0 {
		t.Fatal("full local transfer row unavailable")
	}
}
