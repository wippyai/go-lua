package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestZZGenFieldProbe checks whether reading the `value` field off the ok-variant
// of an instantiated Validation<{string}> yields {string} (substituted) or the
// bare type param T. Read-only diagnostic probe for gradual root 3b.
func TestZZGenFieldProbe(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	okVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", tp).Build()
	errVariant := typ.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).Build()
	body := typ.NewUnion(okVariant, errVariant)
	validation := typ.NewGeneric("Validation", []*typ.TypeParam{tp}, body)

	strArr := typ.NewArray(typ.String)
	inst := typ.Instantiate(validation, strArr) // Validation<{string}>
	t.Logf("inst=%s", inst.String())

	// Field read of `value` on the whole instantiated union.
	if ft, ok := Field(inst, "value"); ok {
		t.Logf("Field(Validation<string[]>, value) = %s", ft.String())
	} else {
		t.Logf("Field(Validation<string[]>, value) -> not found")
	}

	// Field read on the ok-variant alone, instantiated.
	okInst := typ.Instantiate(typ.NewGeneric("OkV", []*typ.TypeParam{tp}, okVariant), strArr)
	if ft, ok := Field(okInst, "value"); ok {
		t.Logf("Field(Ok<string[]>, value) = %s", ft.String())
	}

	// Field read on the raw (un-instantiated) ok-variant body — what narrowing the
	// instantiated union without substitution would expose.
	if ft, ok := Field(okVariant, "value"); ok {
		t.Logf("Field(rawOkVariant, value) = %s  (kind=%v)", ft.String(), ft.Kind())
	}
}
