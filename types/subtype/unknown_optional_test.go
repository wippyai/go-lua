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
