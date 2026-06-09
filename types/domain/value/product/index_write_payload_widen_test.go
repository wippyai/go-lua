package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestWidenRecordVariantsKeepsCommonRecordFields(t *testing.T) {
	funcNode := FromType(typ.NewRecord().
		Field("node_id", typ.String).
		Field("config", typ.NewRecord().Field("func_id", typ.Any).Build()).
		Build())
	agentNode := FromType(typ.NewRecord().
		Field("node_id", typ.String).
		Field("config", typ.NewRecord().Field("agent", typ.Any).Build()).
		Build())

	got := Widen(funcNode, agentNode).ProjectValue()
	rec, ok := unwrap.Alias(got).(*typ.Record)
	if !ok {
		t.Fatalf("widened node variants = %T %[1]v, want record", got)
	}
	if rec.GetField("config") == nil {
		t.Fatalf("widened node variants lost common config field: %v", got)
	}
}
