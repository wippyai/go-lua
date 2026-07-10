package typenarrow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
