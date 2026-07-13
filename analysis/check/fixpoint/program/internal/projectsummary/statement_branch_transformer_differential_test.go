package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestParsedStatementBranchTransformerMatchesCanonicalConcreteProjection(t *testing.T) {
	reg := standard.Registry()
	statements, err := parse.ParseString(`local function str(value: any): string
		if type(value) == "string" then
			return value
		end
		return ""
	end`, "statement_branch_transformer_differential.lua")
	if err != nil || len(statements) != 1 {
		t.Fatalf("parse statement branch: %v (%d statements)", err, len(statements))
	}
	assignment, ok := statements[0].(*ast.LocalAssignStmt)
	if !ok || len(assignment.Exprs) != 1 {
		t.Fatalf("statement = %T, want one local function assignment", statements[0])
	}
	fn, ok := assignment.Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("local assignment expression = %T, want function", assignment.Exprs[0])
	}
	prepared, err := body.PrepareFunction(fn, body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}})
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 1 {
		t.Fatalf("boundary params = %v, want one", params)
	}
	for point := cfg.Point(0); int(point) < prepared.Graph().Size(); point++ {
		if prepared.Graph().IsBranch(point) {
			if _, ok := plan.Facts().BranchConditionSource(point); !ok {
				t.Fatalf("statement branch %d has no canonical condition source", point)
			}
		}
	}

	shape := transformer.Shape{Params: 1, Globals: uint32(len(plan.BoundaryGlobals()))}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("statement branch relation compiled contextually: %s", reason)
	}
	for _, test := range []struct {
		input product.Value
	}{
		{input: typevalue.LiteralString(reg, "value")},
		{input: typevalue.LiteralNumber(reg, 7)},
		{input: product.Top()},
	} {
		input := test.input
		bindings := make([]product.Value, shape.ValueCount())
		bindings[0] = input
		for i := 1; i < len(bindings); i++ {
			bindings[i] = product.Top()
		}
		paths := make([]pathdom.Path, shape.ValueCount())
		paths[0] = pathdom.NewPlaceholder(0)
		cursor, cursorErr := transformer.NewBindingCursor(shape, bindings, paths)
		if cursorErr != nil {
			t.Fatal(cursorErr)
		}
		got, exact := relation.Specialize(cursor, nil, nil)
		if !exact {
			t.Fatalf("statement branch relation did not specialize for %#v", input)
		}
		if len(got.ReturnParamPathAliases) != 1 || got.ReturnParamPathAliases[0].ReturnIndex != 0 || got.ReturnParamPathAliases[0].Source != "$0" {
			t.Fatalf("refined parameter return aliases = %#v, want return 0 <- $0", got.ReturnParamPathAliases)
		}
		if len(got.ReturnFlows) != 0 {
			t.Fatalf("row-local refined return invented whole-function flows: %#v", got.ReturnFlows)
		}
		concrete, solveErr := body.SolvePrepared(prepared, body.SolveConfig{
			EntryState: state.State{}.WriteValue(reg, key.SymbolValue(params[0]), input),
		})
		if solveErr != nil {
			t.Fatal(solveErr)
		}
		want := summary.Normalize(reg, FromResult(concrete))
		if !summary.Equal(reg, got, want) || summary.NormalizedPayloadDigest(reg, got) != summary.NormalizedPayloadDigest(reg, want) {
			t.Fatalf("statement branch symbolic/concrete summary differs\n got=%#v\nwant=%#v", got, want)
		}
	}
}
