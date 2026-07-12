package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLowerBranchGuardsUsesConcreteFactapplyAlgebra(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4)
	shape, ok := factflow.NewValueSourceShape(false, false, false, false)
	if !ok {
		t.Fatal("scalar source shape rejected")
	}
	source, ok := factflow.NewPathValueSource("arg", 0, 0, 0, shape)
	if !ok {
		t.Fatal("path source rejected")
	}
	branch := factapply.NewBranchAlgebra(factflow.NewFacts(factflow.FactsInput{
		BranchConditionSources: map[cfg.Point]factflow.ValueSource{point: source},
	}), point)
	arena := NewArena(reg)
	param := arena.Root(Root{Kind: RootParam, Index: 0})
	truthy, falsy, err := LowerBranchGuards(arena, branch, func(got factflow.ValueSource) (ValueTerm, bool) {
		return param, got == source
	})
	if err != nil {
		t.Fatal(err)
	}
	truthyCursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralString(reg, "yes")}, nil)
	falsyCursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralBool(reg, false)}, nil)
	if got, valid := arena.evalGuard(truthy, truthyCursor, SpecializationContext{}); !valid || !got {
		t.Fatalf("truthy guard = %v/%v", got, valid)
	}
	if got, valid := arena.evalGuard(falsy, falsyCursor, SpecializationContext{}); !valid || !got {
		t.Fatalf("falsy guard = %v/%v", got, valid)
	}
}

func TestLowerBranchGuardsFailsClosedForEveryEffectFamily(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(2)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, _ := factflow.NewPathValueSource("arg", 0, 0, 0, shape)
	left := pathdom.NewPath(symbol.ID(1), "left")
	right := pathdom.NewPath(symbol.ID(2), "right")
	constraint := factflow.NewValueConstraint(typevalue.LiteralString(reg, "ready"))

	tests := []struct {
		name  string
		input factflow.FactsInput
		want  string
	}{
		{name: "refinement", input: factflow.FactsInput{BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			point: factflow.NewBranchRefinementSet(factflow.NewBranchRefinement(left, constraint, true, factflow.ValueRefinement{}, false)),
		}}, want: "branch:refinement"},
		{name: "path relation", input: factflow.FactsInput{BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			point: factflow.NewBranchPathRelationSet(factflow.NewBranchPathEquality(left, right, true, false)),
		}}, want: "branch:path-relation"},
		{name: "path evidence", input: factflow.FactsInput{BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
			point: factflow.NewBranchPathEvidenceSet(factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(left, true)),
		}}, want: "branch:path-evidence"},
		{name: "sufficient literal", input: factflow.FactsInput{BranchSufficientLiteralCases: map[cfg.Point]factflow.BranchSufficientLiteralCaseSet{
			point: factflow.NewBranchSufficientLiteralCaseSet(factflow.NewBranchSufficientLiteralCase(left, typevalue.LiteralString(reg, "ready"), true)),
		}}, want: "branch:sufficient-literal-case"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.BranchConditionSources = map[cfg.Point]factflow.ValueSource{point: source}
			branch := factapply.NewBranchAlgebra(factflow.NewFacts(tc.input), point)
			arena := NewArena(reg)
			_, _, err := LowerBranchGuards(arena, branch, func(factflow.ValueSource) (ValueTerm, bool) {
				return arena.Constant(product.Top()), true
			})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
