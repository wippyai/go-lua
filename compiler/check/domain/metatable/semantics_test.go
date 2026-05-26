package metatable

import (
	"testing"

	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWith_AttachesMetatablePrototypeMethods(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	meta := typ.NewRecord().Field("__index", prototype).Build()
	table := typ.NewRecord().Field("id", typ.String).Build()

	got := With(table, meta)
	if _, ok := querycore.Method(got, "ready"); !ok {
		t.Fatalf("expected metatable method on result, got %s", typ.FormatShort(got))
	}
}

func TestWith_OptionalMetatableKeepsPlainVariant(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("ready", method).Build()
	meta := typ.NewRecord().Field("__index", prototype).Build()
	table := typ.NewRecord().Field("id", typ.String).Build()

	got := With(table, typ.NewOptional(meta))
	if _, ok := querycore.Method(got, "ready"); ok {
		t.Fatalf("optional metatable must not prove method on every variant, got %s", typ.FormatShort(got))
	}
	union, ok := got.(*typ.Union)
	if !ok || len(union.Members) != 2 {
		t.Fatalf("optional metatable result = %T %s, want two-variant union", got, typ.FormatShort(got))
	}
}
