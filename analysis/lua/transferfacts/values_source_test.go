package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestValueSourceLowersExplicitUnknown(t *testing.T) {
	l := lowerer{exprs: make(map[any]factflow.ExprRef)}

	source := l.valueSource(sourceprovenance.NewUnknownSource(2))
	if source.Kind != factflow.ValueSourceUnknown || source.TargetIndex != 2 || !source.Valid() {
		t.Fatalf("unknown value source = %#v, want valid unknown for target 2", source)
	}
	if source.HasExpr || source.ExprRef != 0 || source.HasCallPoint || source.CallPoint != 0 {
		t.Fatalf("unknown value source retained evidence refs: %#v", source)
	}
}

func TestValueSourceExprRefDistinguishesWrappedVarargSlots(t *testing.T) {
	inner := &ast.Comma3Expr{}
	wrapped := &ast.NonNilAssertExpr{Expr: inner}
	shape, ok := sourceprovenance.NewSourceShape(true, true, false, true)
	if !ok {
		t.Fatalf("expanded open-tail shape rejected")
	}

	firstSource, ok := sourceprovenance.NewVarargSource(wrapped, 0, 0, 0, shape)
	if !ok {
		t.Fatalf("first wrapped vararg source rejected")
	}
	secondSource, ok := sourceprovenance.NewVarargSource(wrapped, 0, 1, 1, shape)
	if !ok {
		t.Fatalf("second wrapped vararg source rejected")
	}

	l := lowerer{exprs: make(map[any]factflow.ExprRef)}
	first := l.valueSource(firstSource)
	second := l.valueSource(secondSource)
	if first.Kind != factflow.ValueSourceVararg || second.Kind != factflow.ValueSourceVararg {
		t.Fatalf("wrapped vararg sources = %#v / %#v, want vararg", first, second)
	}
	if first.ExprRef == 0 || second.ExprRef == 0 || first.ExprRef == second.ExprRef {
		t.Fatalf("wrapped vararg refs = %d / %d, want distinct non-zero refs", first.ExprRef, second.ExprRef)
	}
	if first.ResultIndex != 0 || second.ResultIndex != 1 || first.TargetIndex != 0 || second.TargetIndex != 1 {
		t.Fatalf("wrapped vararg slots = %#v / %#v, want slot-specific evidence", first, second)
	}
}

func TestWIRPathOperandValidatesRootOnlyAndSymbolKind(t *testing.T) {
	stmts, bindings, _, _ := parseSemanticChunk(t, `
local root = { child = 1 }
local value = root.child
`)
	rootStmt := mustLocalStmt(t, stmts, 0)
	rootPath := path.NewPath(mustLocalAt(t, bindings, rootStmt, 0), "root")
	childPath := rootPath.Field("child")

	body := wir.NewBody("path-operand-validation")
	rootOp := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(rootPath))}
	childOp := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(childPath))}
	l := lowerer{wir: body, bindings: bindings}

	gotRoot, ok := l.wirPathOperand(rootOp, true, symbol.Local)
	if !ok || !gotRoot.Equal(rootPath) {
		t.Fatalf("root operand = %s/%v, want %s", gotRoot, ok, rootPath)
	}
	gotChild, ok := l.wirPathOperand(childOp, false, symbol.Local)
	if !ok || !gotChild.Equal(childPath) {
		t.Fatalf("child operand = %s/%v, want %s", gotChild, ok, childPath)
	}
	if got, ok := l.wirPathOperand(childOp, true, symbol.Local); ok {
		t.Fatalf("root-only child operand = %s/%v, want rejected", got, ok)
	}
	if got, ok := l.wirPathOperand(rootOp, true, symbol.Global); ok {
		t.Fatalf("disallowed local operand = %s/%v, want rejected", got, ok)
	}
}

func TestWIRPathExpressionSourcesPublishRootAndProjectedWitnesses(t *testing.T) {
	stmts, bindings, _, _ := parseSemanticChunk(t, `
local root = { child = 1 }
local value = root.child
`)
	rootStmt := mustLocalStmt(t, stmts, 0)
	rootPath := path.NewPath(mustLocalAt(t, bindings, rootStmt, 0), "root")
	childPath := rootPath.Field("child")
	rootType := typetable.NewRecord().
		Field("child", typ.Number).
		Build()

	body := wir.NewBody("path-source-witness")
	rootOp := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(rootPath))}
	childOp := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(childPath))}
	reg := standard.Registry()
	l := lowerer{
		registry:         reg,
		wir:              body,
		bindings:         bindings,
		symbolTypes:      map[symbol.ID]typ.Type{rootPath.Symbol: rootType},
		exprs:            make(map[any]factflow.ExprRef),
		expressionPaths:  make(map[factflow.ExprRef]path.Path),
		expressionValues: make(map[factflow.ExprRef]product.Value),
	}
	rootSource, ok := l.rootPathExpressionSourceFromWIR("test", 1, rootOp, 0, 0, true, false, false, symbol.Local)
	if !ok {
		t.Fatal("rootPathExpressionSourceFromWIR failed")
	}
	childSource, ok := l.pathExpressionSourceFromWIR("test", 2, childOp, 0, 0, true, false, false, symbol.Local)
	if !ok {
		t.Fatal("pathExpressionSourceFromWIR failed")
	}
	assertExpressionWitnessType(t, reg, l.expressionValues[rootSource.ExprRef], rootType)
	assertExpressionWitnessType(t, reg, l.expressionValues[childSource.ExprRef], typ.Number)
}

func assertExpressionWitnessType(t *testing.T, reg *axis.Registry, value product.Value, want typ.Type) {
	t.Helper()
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("expression witness = %v/%v, want %v", got, ok, want)
	}
}
