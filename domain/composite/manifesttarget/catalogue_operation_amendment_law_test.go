package manifesttarget

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/type/typ"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

// TestApplyOperationAmendmentsReplaceNormalSetPreservesThrowArmAndResultTail
// pins the ReplaceNormalSet law to replacing exactly the normal-outcome
// arms. The declared Throw arm and the declared result tail belong to the
// operation as authored by its provider, not to the normal-arm split the law
// amends, so both must survive a normal-set replacement unchanged.
func TestApplyOperationAmendmentsReplaceNormalSetPreservesThrowArmAndResultTail(t *testing.T) {
	operation := vocabulary.OperationSpec{
		Outcomes: []vocabulary.OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{
				Fixed:    portableList([]typ.Type{typ.Any}),
				TailType: portable(typ.Integer),
			}},
			{Kind: flowkind.OutcomeThrow, Values: vocabulary.ValuesSpec{
				Fixed: portableList([]typ.Type{typ.Any}),
			}},
		},
	}
	law := moduleio.Operation{
		ReplaceNormalSet: true,
		ReplaceNormal: []moduleio.Values{
			{Fixed: []typ.Type{typ.Integer}, Tail: moduleio.ValuesClosed},
			{Fixed: []typ.Type{typ.Nil}, Tail: moduleio.ValuesClosed},
		},
	}

	applyOperationAmendments(&operation, law)

	var throwCount, normalCount int
	var sawTailType bool
	for _, outcome := range operation.Outcomes {
		switch outcome.Kind {
		case flowkind.OutcomeThrow:
			throwCount++
		case flowkind.OutcomeNormal:
			normalCount++
			if outcome.Values.TailType.Available() {
				sawTailType = true
			}
		}
	}
	if throwCount != 1 {
		t.Fatalf("outcomes after ReplaceNormalSet carry %d throw arms, want 1: %#v", throwCount, operation.Outcomes)
	}
	if normalCount != len(law.ReplaceNormal) {
		t.Fatalf("outcomes after ReplaceNormalSet carry %d normal arms, want %d: %#v", normalCount, len(law.ReplaceNormal), operation.Outcomes)
	}
	if !sawTailType {
		t.Fatal("ReplaceNormalSet dropped the declared result tail")
	}
}
