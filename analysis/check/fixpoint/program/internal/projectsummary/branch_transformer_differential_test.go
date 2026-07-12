package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestBranchTransformerMatchesCanonicalConcreteProjection(t *testing.T) {
	reg := standard.Registry()
	fn := parseBranchTransformerFunction(t, `function f(flag: boolean): integer
		if flag then return 1 else return 2 end
	end`)
	prepared, err := body.PrepareFunction(fn, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("PrepareFunction: %v", err)
	}
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 1 {
		t.Fatalf("boundary params = %v, want one", params)
	}
	shape := transformer.Shape{Params: 1}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("branch relation compiled contextually: %s", reason)
	}

	for _, input := range []product.Value{
		typevalue.LiteralBool(reg, true),
		typevalue.LiteralBool(reg, false),
	} {
		cursor, cursorErr := transformer.NewBindingCursor(shape, []product.Value{input}, nil)
		if cursorErr != nil {
			t.Fatal(cursorErr)
		}
		got, exact := relation.Specialize(cursor, nil, nil)
		if !exact {
			t.Fatalf("branch relation did not specialize for %#v", input)
		}
		concrete, solveErr := body.SolvePrepared(prepared, body.SolveConfig{
			EntryState: state.State{}.WriteValue(reg, key.SymbolValue(params[0]), input),
		})
		if solveErr != nil {
			t.Fatalf("SolvePrepared: %v", solveErr)
		}
		want := summary.Normalize(reg, FromResult(concrete))
		if !summary.Equal(reg, got, want) || summary.NormalizedPayloadDigest(reg, got) != summary.NormalizedPayloadDigest(reg, want) {
			var returnAxes []string
			if len(got.Returns) == 1 && len(want.Returns) == 1 {
				returnAxes = product.DifferenceAxes(reg, got.Returns[0], want.Returns[0])
			}
			t.Fatalf("symbolic/canonical concrete Summary differs for %#v (return axes %v)\n got=%#v\nwant=%#v", input, returnAxes, got, want)
		}
	}
}

func parseBranchTransformerFunction(t *testing.T, source string) *ast.FunctionExpr {
	t.Helper()
	stmts, err := parse.ParseString(source, "branch_transformer_differential.lua")
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) != 1 {
		t.Fatalf("statements = %d, want one function", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("statement = %T, want function definition", stmts[0])
	}
	return def.Func
}
