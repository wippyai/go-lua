package typ

import (
	"strings"
	"testing"
)

func TestFormatShortBoundsRecursiveProduct(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("next", NewOptional(self)).
			Field("payload", NewMap(String, self)).
			Build()
	})

	members := []Type{node}
	for i := 0; i < DefaultFormatOptions.MaxUnionMembers+6; i++ {
		field := "case" + string(rune('a'+i))
		members = append(members, NewRecord().
			Field(field, node).
			Field("meta", NewMap(String, node)).
			Build())
	}
	huge := NewUnion(members...)

	got := FormatShort(huge)
	if len(got) > DefaultFormatOptions.MaxBytes {
		t.Fatalf("FormatShort exceeded byte budget: len=%d max=%d", len(got), DefaultFormatOptions.MaxBytes)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected recursive product rendering to show truncation, got %q", got)
	}
}

func TestFormatHonorsDepthBudget(t *testing.T) {
	tp := Type(String)
	for i := 0; i < DefaultFormatOptions.MaxDepth+4; i++ {
		tp = NewRecord().Field("next", tp).Build()
	}

	got := Format(tp, FormatOptions{
		MaxDepth:        2,
		MaxNodes:        20,
		MaxUnionMembers: 4,
		MaxRecordFields: 4,
		MaxTupleElems:   4,
		MaxTypeParams:   4,
		MaxParams:       4,
		MaxReturns:      4,
		MaxBytes:        160,
	})
	if !strings.Contains(got, "...") {
		t.Fatalf("expected depth budget truncation, got %q", got)
	}
}
