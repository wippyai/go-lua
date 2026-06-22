package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestTypePairClearlyCompatible(t *testing.T) {
	if !typePairClearlyCompatible(typ.String, typ.String) {
		t.Fatalf("equal types should be clearly compatible")
	}
	if !typePairClearlyCompatible(typ.String, typeexpr.Union(typ.String, typ.Number)) {
		t.Fatalf("subtype should be clearly compatible with its union")
	}
	if typePairClearlyCompatible(typ.String, typ.Number) {
		t.Fatalf("distinct scalar types should not be clearly compatible")
	}
}
