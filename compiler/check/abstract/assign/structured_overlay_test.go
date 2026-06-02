package assign

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyStructuredWriteStringIndexUsesIndexedWrite(t *testing.T) {
	base := typ.NewRecord().Field("stable", typ.Number).Build()
	got := applyStructuredWrite(
		base,
		[]constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "x-y"}},
		typ.String,
	)

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("applyStructuredWrite string index = %T, want record (%v)", got, got)
	}
	stable := rec.GetField("stable")
	if stable == nil || !typ.TypeEquals(stable.Type, typ.Number) {
		t.Fatalf("stable field changed: %v", rec)
	}
	if !rec.HasMapComponent() || !typ.TypeEquals(rec.MapKey, typ.LiteralString("x-y")) || !typ.TypeEquals(rec.MapValue, typ.String) {
		t.Fatalf("map component = [%v]: %v, want [x-y]: string", rec.MapKey, rec.MapValue)
	}
	if rec.GetField("x-y") != nil {
		t.Fatalf("string index write created dot-field x-y: %v", rec)
	}
}

func TestApplyStructuredWriteIntIndexUsesLiteralIndex(t *testing.T) {
	base := typ.NewRecord().Field("stable", typ.Boolean).Build()
	got := applyStructuredWrite(
		base,
		[]constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 2}},
		typ.Number,
	)

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("applyStructuredWrite int index = %T, want record (%v)", got, got)
	}
	stable := rec.GetField("stable")
	if stable == nil || !typ.TypeEquals(stable.Type, typ.Boolean) {
		t.Fatalf("stable field changed: %v", rec)
	}
	if !rec.HasMapComponent() || !typ.TypeEquals(rec.MapKey, typ.LiteralInt(2)) || !typ.TypeEquals(rec.MapValue, typ.Number) {
		t.Fatalf("map component = [%v]: %v, want [2]: number", rec.MapKey, rec.MapValue)
	}
}
