package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/semantic/intrinsic"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestSealedStatementTypeInequalityPublishesExactPredicate(t *testing.T) {
	for _, typeName := range []string{"string", "nil"} {
		t.Run(typeName, func(t *testing.T) {
			fn, bindings, built := parseSemanticFunction(t, `
function f(value: any): ()
    if type(value) ~= "`+typeName+`" then
        local hit = true
    end
end
`, "type")
			branch, ok := fn.Stmts[0].(*ast.IfStmt)
			if !ok {
				t.Fatalf("statement = %T, want if", fn.Stmts[0])
			}
			point := requireBranchPointForStmt(t, built, branch)
			body := wirlower.LowerFunction("statement-type-not-"+typeName, fn, bindings, built)
			facts := LowerDetailed(built.Graph, Config{
				Registry:            standard.Registry(),
				WIR:                 body,
				SealedLuaTypeChecks: true,
			}).Facts

			source, ok := facts.BranchConditionSource(point)
			if !ok || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
				t.Fatalf("branch condition source = %#v/%v, want exact expression", source, ok)
			}
			predicate, ok := facts.ExpressionOperation(source.ExprRef)
			if !ok || predicate.Kind() != factflow.ExpressionOperationBinary || predicate.Op() != "~=" {
				t.Fatalf("predicate operation = %#v/%v, want lua_type(value) ~= %q", predicate, ok, typeName)
			}
			literal := predicate.Right()
			if literal.Kind != factflow.ValueSourceLiteral || literal.LiteralKind != factflow.ValueSourceLiteralString || literal.String != typeName {
				t.Fatalf("predicate literal = %#v, want %q", literal, typeName)
			}
			typeSource := predicate.Left()
			if typeSource.Kind != factflow.ValueSourceExpression || !typeSource.HasExpr {
				t.Fatalf("predicate lhs = %#v, want intrinsic expression", typeSource)
			}
			typeOperation, ok := facts.ExpressionOperation(typeSource.ExprRef)
			kind, intrinsicOK := typeOperation.Intrinsic()
			if !ok || !intrinsicOK || kind != intrinsic.LuaType {
				t.Fatalf("predicate lhs operation = %#v/%v, want sealed lua_type intrinsic", typeOperation, ok)
			}
			operand := typeOperation.Left()
			valuePath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value")
			if operand.Kind != factflow.ValueSourcePath || operand.PathKey != valuePath.Key() {
				t.Fatalf("lua_type operand = %#v, want %s", operand, valuePath)
			}
		})
	}
}

func TestStatementTypePredicateAuthorityNeverFallsBackToOperandTruthiness(t *testing.T) {
	for _, test := range []struct {
		name     string
		sealed   bool
		typeName string
	}{
		{name: "unsealed runtime tag", typeName: "string"},
		{name: "sealed non-runtime tag", sealed: true, typeName: "integer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			point := cfg.Point(1)
			valuePath := path.NewPath(1, "value")
			body := wir.NewBody("unauthorized-statement-type-predicate")
			body.SetSymbolInfo(valuePath.Symbol, wir.SymbolInfoConfig{Kind: wir.SymbolParam, Name: valuePath.Root})
			operand := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(valuePath))}
			start := body.Emit(wir.Instruction{
				Op:    wir.OpBranch,
				Point: point,
				A:     operand,
				Check: body.InternCheck(wir.Check{
					Kind:     wir.CheckTypeNot,
					Path:     valuePath,
					TypeName: test.typeName,
				}),
			})
			body.SetPointRange(point, start, start+1)
			lowered := lowerer{
				wir:                  body,
				sealedLuaTypeChecks:  test.sealed,
				expressionOperations: make(map[factflow.ExprRef]factflow.ExpressionOperation),
			}

			if source, ok := lowered.branchConditionAtWIR(point); ok {
				t.Fatalf("unauthorized type predicate published branch source %#v; operand truthiness fallback is forbidden", source)
			}
			if len(lowered.expressionOperations) != 0 {
				t.Fatalf("unauthorized type predicate published expression operations: %#v", lowered.expressionOperations)
			}
		})
	}
}
