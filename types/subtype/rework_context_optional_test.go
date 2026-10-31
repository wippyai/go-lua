package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestOptionalLiteralSlotPreservesWideningContext(t *testing.T) {
	for _, mode := range []Assignability{Gradual, Strict} {
		values := typ.NewAlias("Phase", typ.NewUnion(typ.LiteralString("ready"), typ.LiteralString("done")))
		target := typ.NewRecord().Field("phase", typ.NewOptional(values)).Build()
		for _, value := range []typ.Type{typ.LiteralString("ready"), typ.Nil} {
			if !mode.Assignable(typ.NewRecord().Field("phase", value).Build(), target) {
				t.Fatalf("mode=%v: %v must initialize the optional literal slot", mode, value)
			}
		}
		if mode.Assignable(typ.NewRecord().Field("phase", typ.LiteralString("wrong")).Build(), target) {
			t.Fatal("wrong literal accepted")
		}
	}
}
