package target

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestContentIDOperationEncodingTracksCanonicalOutcomeRows(t *testing.T) {
	base := []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testString}, Tail: ValuesClosed}}}
	left := mustSeal(t, Spec{Operations: []OperationSpec{contentIDOperation("operation", base)}}).ContentID()
	changed := mustSeal(t, Spec{Operations: []OperationSpec{contentIDOperation("operation", []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []schematype.Type{testBoolean}, Tail: ValuesClosed}}})}}).ContentID()
	if left == changed {
		t.Fatal("operation outcome type mutation was omitted from ContentID")
	}
}
