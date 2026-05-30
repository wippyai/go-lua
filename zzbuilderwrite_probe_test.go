package lua

import (
	"testing"

	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// builderRecForKind builds a HandlerBuilder-shaped record whose for_kind field is
// a self-returning method with a literal-union param, mirroring the EXPECTED
// builder field type that ops.checkTableAsRecord compares against (it asks for
// the writable field slot via querycore.IndexWrite).
func builderRecForKind() (*typ.Record, typ.Type) {
	kindUnion := typ.NewUnion(
		typ.LiteralString("reserve"),
		typ.LiteralString("create"),
		typ.LiteralString("complete"),
	)
	rb := typ.NewRecursive("HandlerBuilder", func(self typ.Type) typ.Type {
		forKind := typ.Func().
			Param("self", self).
			Param("kind", kindUnion).
			Returns(self).
			Build()
		return typ.NewRecord().Field("for_kind", forKind).Build()
	})
	body := rb.Body.(*typ.Record)
	return body, kindUnion
}

// forKindParam extracts the declared literal-union param ("kind") from a for_kind
// function-typed slot, reporting whether it stayed the literal-union or widened.
func forKindParam(t *testing.T, slot typ.Type) typ.Type {
	t.Helper()
	fn, ok := slot.(*typ.Function)
	if !ok {
		t.Fatalf("for_kind slot is not a function: %s", typ.FormatShort(slot))
	}
	if len(fn.Params) < 2 {
		t.Fatalf("for_kind has %d params, want >=2: %s", len(fn.Params), typ.FormatShort(slot))
	}
	return fn.Params[1].Type
}

// isLiteralUnion reports whether t is a union of string literals (the declared
// kind domain) rather than the flattened base `string`.
func isLiteralUnion(t typ.Type) bool {
	u, ok := t.(*typ.Union)
	if !ok {
		return false
	}
	for _, m := range u.Members {
		if lit, ok := m.(*typ.Literal); !ok || lit.Base != typ.String.Kind() {
			return false
		}
	}
	return len(u.Members) > 0
}

// TestZZBuilderWriteSlot proves the writable field slot for a builder method
// field preserves its declared literal-union parameter. This is the EXPECTED
// type that ops.checkTableAsRecord compares the GOT builder against; if the
// param is flattened to `string`, the contravariant param check fails and the
// fluent-builder field-type-mismatch false positive fires.
func TestZZBuilderWriteSlot(t *testing.T) {
	body, _ := builderRecForKind()

	slot, ok := querycore.IndexWrite(body, typ.LiteralString("for_kind"))
	if !ok {
		t.Fatalf("IndexWrite(builder, for_kind) returned no slot")
	}
	param := forKindParam(t, slot)
	t.Logf("IndexWrite for_kind param = %s", typ.FormatShort(param))

	if !isLiteralUnion(param) {
		t.Fatalf("EXPECTED for_kind param widened to %s; want the declared literal-union", typ.FormatShort(param))
	}
}
