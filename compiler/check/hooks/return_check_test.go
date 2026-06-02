package hooks

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnTypeCompatibleRejectsNilableActualAgainstNonNilDeclared(t *testing.T) {
	action := typ.NewUnion(
		typ.NewRecord().
			Field("kind", typ.LiteralString("a")).
			Field("x", typ.String).
			Build(),
		typ.NewRecord().
			Field("kind", typ.LiteralString("b")).
			Field("y", typ.String).
			Build(),
	)

	if returnTypeCompatible(typ.NewOptional(action), action) {
		t.Fatal("return boundary accepted nilable actual for non-nil declared return")
	}
	if returnTypeCompatible(typ.Nil, action) {
		t.Fatal("return boundary accepted nil for non-nil declared return")
	}
}
