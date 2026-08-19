package typeauthority

import (
	"testing"

	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestPrimitiveFunctionRetainsEstablishedMeaning(t *testing.T) {
	got, ok := primitiveKind(uint8(statictypes.PrimitiveFunction))
	if !ok {
		t.Fatal("function primitive was unsupported")
	}
	want, ok := typ.BuiltinPrimitiveType("function")
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("function primitive = %v, want established %v", got, want)
	}
}
