package target

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestContentIDValueEncodingTracksFrozenTypeDeclarations(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{contentIDOperation("value-id", []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}}})}}).ContentID()
	right := mustSeal(t, Spec{Operations: []OperationSpec{contentIDOperation("value-id", []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testNumber}, Tail: ValuesClosed}}})}}).ContentID()
	if left == right {
		t.Fatal("frozen value type mutation was omitted from ContentID")
	}
}
