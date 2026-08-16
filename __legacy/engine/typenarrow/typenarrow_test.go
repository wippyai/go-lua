package typenarrow

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestMatchRefinementTableCarriesBuiltinTableWitness(t *testing.T) {
	reg := standard.Registry()
	refinement := MatchRefinement(reg, runtimekind.Table)
	constraint, ok := refinement.Constraint()
	if !ok {
		t.Fatal("table type() refinement has no constraint")
	}
	got, ok := typevalue.TypeOf(reg, constraint)
	if !ok {
		t.Fatal("table type() refinement has no static type witness")
	}
	if !typ.TypeEquals(got, typetable.BuiltinTopMarker()) {
		t.Fatalf("table type() witness = %v, want builtin table marker", got)
	}
}
