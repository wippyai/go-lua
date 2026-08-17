package typeauthority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestPrimitiveFunctionRetainsEstablishedMeaning(t *testing.T) {
	got, ok := primitiveKind(uint8(static.PrimitiveFunction))
	if !ok {
		t.Fatal("function primitive was unsupported")
	}
	want, ok := typ.BuiltinPrimitiveType("function")
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("function primitive = %v, want established %v", got, want)
	}
}
