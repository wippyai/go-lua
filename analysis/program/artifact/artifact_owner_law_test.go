package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

// TestArtifactRejectsForeignFinalRoute uses an equivalent replay: IDs may
// agree, but its endpoint and recurrence proofs belong to another exact Flow
// owner and cannot be admitted into the left artifact transaction.
func TestArtifactRejectsForeignFinalRoute(t *testing.T) {
	source := lower.Source{Name: "transformer-foreign-route.lua", Text: []byte(`
local function loop(value)
  while value do value = value - 1 end
  return value
end
return loop(2)
`)}
	left, leftErr := lower.Lower(source)
	right, rightErr := lower.Lower(source)
	if leftErr != nil || rightErr != nil {
		t.Fatalf("lower equivalent Programs: %v / %v", leftErr, rightErr)
	}
	transaction := compiler{input: left, points: make(map[identity.ContentID]struct{})}
	if !transaction.copyLocalWTO() {
		t.Fatal("left LocalWTO schedule unavailable")
	}
	foreignRoutes := right.Flow().Causal().Successors()
	var foreign flow.FinalRoute
	for index := 0; index < foreignRoutes.TotalCount(); index++ {
		candidate, candidateOK := foreignRoutes.FinalAt(index)
		if candidateOK {
			if _, recurrenceOK := candidate.Recurrence(); recurrenceOK {
				foreign = candidate
				break
			}
		}
	}
	if !foreign.Available() || left.Flow().Causal().OwnsFinalRoute(foreign) {
		t.Fatal("foreign final route owner fence unavailable")
	}
	if transaction.admitEnvironment(foreign) {
		t.Fatal("foreign/spliced endpoint or recurrence proof was admitted")
	}
}
