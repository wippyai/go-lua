package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestOptionalUnknownAcceptsEveryValue(t *testing.T) {
	target := typ.NewOptional(typ.Unknown)
	for _, sub := range []typ.Type{typ.Unknown, typ.Any, typ.String, typ.Nil, target} {
		if !IsSubtype(sub, target) {
			t.Fatalf("%v must be a subtype of unknown?", sub)
		}
	}
}

func TestAdmitsEveryValue(t *testing.T) {
	admitting := []typ.Type{
		typ.Any,
		typ.Unknown,
		typ.NewOptional(typ.Unknown),
		typ.NewUnion(typ.String, typ.Any),
	}
	for _, tp := range admitting {
		if !AdmitsEveryValue(tp) {
			t.Errorf("%s admits every value", typ.FormatShort(tp))
		}
	}
	restricting := []typ.Type{
		typ.String,
		typ.NewOptional(typ.String),
		typ.NewRecord().Field("id", typ.String).Build(),
		typ.NewUnion(typ.String, typ.Number),
	}
	for _, tp := range restricting {
		if AdmitsEveryValue(tp) {
			t.Errorf("%s does not admit every value", typ.FormatShort(tp))
		}
	}
}
