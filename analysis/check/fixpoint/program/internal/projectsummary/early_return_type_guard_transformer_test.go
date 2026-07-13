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
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// This is the canonical early-return guard shape used by small trim/string
// helpers. The normal-return path is dominated by type(value) == "string";
// the symbolic relation must retain that exact branch fact rather than forcing
// the whole function back through contextual solving.
func TestParsedEarlyReturnTypeGuardTransformerIsExact(t *testing.T) {
	reg := standard.Registry()
	statements, err := parse.ParseString(`local function str(value: any): string
		if type(value) ~= "string" then
			return ""
		end
		return value
	end`, "early_return_type_guard_transformer.lua")
	if err != nil || len(statements) != 1 {
		t.Fatalf("parse early-return guard: %v (%d statements)", err, len(statements))
	}
	assignment, ok := statements[0].(*ast.LocalAssignStmt)
	if !ok || len(assignment.Exprs) != 1 {
		t.Fatalf("statement = %T, want one local function assignment", statements[0])
	}
	fn, ok := assignment.Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("local assignment expression = %T, want function", assignment.Exprs[0])
	}
	prepared, err := body.PrepareFunction(fn, body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	params := plan.BoundaryParams()
	if len(params) != 1 {
		t.Fatalf("boundary params = %v, want one", params)
	}
	shape := transformer.Shape{
		Params:  1,
		Globals: uint32(len(plan.BoundaryGlobals())),
	}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("early-return type guard compiled contextually: %s", reason)
	}
	for _, input := range []product.Value{
		typevalue.LiteralString(reg, " value "),
		typevalue.LiteralNumber(reg, 7),
		typevalue.Nil(reg),
		product.Top(),
	} {
		bindings := make([]product.Value, shape.ValueCount())
		paths := make([]pathdom.Path, shape.ValueCount())
		bindings[0], paths[0] = input, pathdom.NewPlaceholder(0)
		for i := 1; i < len(bindings); i++ {
			bindings[i] = product.Top()
		}
		cursor, cursorErr := transformer.NewBindingCursor(shape, bindings, paths)
		if cursorErr != nil {
			t.Fatal(cursorErr)
		}
		got, exact := relation.Specialize(cursor, nil, nil)
		if !exact {
			t.Fatalf("early-return relation did not specialize for %#v", input)
		}
		concrete, solveErr := body.SolvePrepared(prepared, body.SolveConfig{
			EntryState: state.State{}.WriteValue(reg, key.SymbolValue(params[0]), input),
		})
		if solveErr != nil {
			t.Fatal(solveErr)
		}
		want := summary.Normalize(reg, FromResult(concrete))
		if !summary.Equal(reg, got, want) || summary.NormalizedPayloadDigest(reg, got) != summary.NormalizedPayloadDigest(reg, want) {
			t.Fatalf("early-return symbolic/concrete summary differs for %#v\n got=%#v\nwant=%#v", input, got, want)
		}
	}
}
