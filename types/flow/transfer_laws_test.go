package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestNormalizeDynamicKeyTypeTruthifiesBeforeWidening(t *testing.T) {
	if got := NormalizeDynamicKeyType(typ.NewOptional(typ.LiteralString("id"))); !typ.TypeEquals(got, typ.LiteralString("id")) {
		t.Fatalf("NormalizeDynamicKeyType(optional literal) = %v, want literal id", got)
	}
	if got := NormalizeDynamicKeyType(typ.NewOptional(typ.String)); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("NormalizeDynamicKeyType(optional string) = %v, want string", got)
	}
}
