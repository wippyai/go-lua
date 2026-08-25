package route

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
)

func TestFoldRefusesAnUnissuedRoutePredicate(t *testing.T) {
	if fact, outcome := Fold(calldomain.CallCoordinate{}, calldomain.Key{}, 1, calldomain.Value{}); outcome != structure.Refuse || fact.IsTop() || fact.IsOpen() || fact.IsComplete() {
		t.Fatalf("unissued route = outcome:%d top:%t open:%t complete:%t", outcome, fact.IsTop(), fact.IsOpen(), fact.IsComplete())
	}
}
