package format_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/format"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestShortBoundsRecursiveProduct(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return &typ.Record{
			Fields: []typ.Field{
				{Name: "next", Type: typ.MaterializeOptional(self)},
				{Name: "payload", Type: typ.NewMap(typ.String, self)},
			},
		}
	})

	members := []typ.Type{node}
	for i := 0; i < format.DefaultOptions.MaxUnionMembers+6; i++ {
		field := "case" + string(rune('a'+i))
		members = append(members, &typ.Record{
			Fields: []typ.Field{
				{Name: field, Type: node},
				{Name: "meta", Type: typ.NewMap(typ.String, node)},
			},
		})
	}
	huge := typ.MaterializeUnion(members)

	got := format.Short(huge)
	if len(got) > format.DefaultOptions.MaxBytes {
		t.Fatalf("format.Short exceeded byte budget: len=%d max=%d", len(got), format.DefaultOptions.MaxBytes)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected recursive product rendering to show truncation, got %q", got)
	}
}

func TestTypeHonorsDepthBudget(t *testing.T) {
	tp := typ.Type(typ.String)
	for i := 0; i < format.DefaultOptions.MaxDepth+4; i++ {
		tp = &typ.Record{
			Fields: []typ.Field{
				{Name: "next", Type: tp},
			},
		}
	}

	got := format.Type(tp, format.Options{
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
