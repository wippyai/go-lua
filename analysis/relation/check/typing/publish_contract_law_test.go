package typing_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/check/typing"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// Publish is a relational boundary, not an Apply-syntax boundary. A sealed
// destination row layout is a valid child shape even when it carries no new
// Apply proposal; physical evaluation may redeem it as an empty/no-op
// publication, but typing must not reject the composition merely because it
// is not a direct Apply expression.
func TestPublishAcceptsSealedRelationChildAtSchemaSeal(t *testing.T) {
	value := newFixture(t)
	input := algebra.NewInput(value.relationB)
	publish := algebra.NewPublish(input, algebra.NewPublishContract(value.relationB, value.keyB))
	report := typing.Check(schemaWith(t, value, []algebra.Expression{publish}, true))
	if !report.Valid() {
		t.Fatalf("Publish rejected a sealed relation child: %v", report.Error())
	}
}
