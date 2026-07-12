package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestStringConcatValueMatchesCanonicalKernelAndRebases(t *testing.T) {
	reg := standard.Registry()
	callee := NewArena(reg)
	left := callee.Constant(typevalue.LiteralString(reg, "Undefined: "))
	right := callee.Root(Root{Kind: RootParam})
	concat := callee.StringConcatValue(left, right)

	param := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{param}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, exact := callee.evalValue(concat, cursor, SpecializationContext{})
	want, wantExact := luasourcevalue.BinaryOperationValue(reg, nil, "..", typevalue.LiteralString(reg, "Undefined: "), param)
	if !exact || !wantExact || !product.Equal(reg, got, want) {
		t.Fatalf("concat exact=%v canonical=%v equal=%v", exact, wantExact, product.Equal(reg, got, want))
	}

	caller := NewArena(reg)
	bound := caller.Root(Root{Kind: RootParam})
	bindings, err := NewTermRootBindings(Shape{Params: 1}, Shape{Params: 1}, []ValueTerm{bound}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{concat}})
	if err != nil {
		t.Fatal(err)
	}
	rebasedValue, ok := caller.evalValue(rebased.Values[0], cursor, SpecializationContext{})
	if !ok || !product.Equal(reg, rebasedValue, want) {
		t.Fatal("rebased concat differs from canonical value")
	}

	numberCursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralInt(reg, 7)}, nil)
	if value, ok := callee.evalValue(concat, numberCursor, SpecializationContext{}); ok || !product.Equal(reg, value, product.Value{}) {
		t.Fatal("non-string concat operand did not fail closed")
	}
}

func TestPlanCompilerLowersSourceOwnedStringConcatExactly(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	left, _ := factflow.NewStringLiteralValueSource("Undefined: ", 0, 0, 0, shape)
	param := symbol.ID(7)
	paramPath := pathdom.NewPath(param, "name")
	paramSource, _ := factflow.NewPathValueSource(paramPath.Key(), 0, 0, 0, shape)
	ref := factflow.ExprRef(11)
	concatSource, _ := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
	op, _ := factflow.NewBinaryExpressionOperation("..", left, paramSource)
	local := symbol.ID(8)
	localPath := pathdom.NewPath(local, "message")
	localSource, _ := factflow.NewPathValueSource(localPath.Key(), 0, 0, 0, shape)
	plan := operationplan.New(graph, factflow.FactsInput{
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{ref: op},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, local, localPath, concatSource),
		},
		Returns: map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{localSource})},
	}).WithBoundaryParams([]symbol.ID{param})

	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{Params: 1})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("concat relation contextual: %s", reason)
	}
	paramValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	cursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{paramValue}, nil)
	got, exact := relation.Specialize(cursor, nil, nil)
	wantValue, _ := luasourcevalue.BinaryOperationValue(reg, nil, "..", typevalue.LiteralString(reg, "Undefined: "), paramValue)
	want := summary.Normalize(reg, summary.Summary{Returns: []product.Value{wantValue}, NormalReturnParams: []product.Value{product.Top()}})
	if !exact || !summary.Equal(reg, got, want) {
		t.Fatalf("concat summary exact=%v\n got=%#v\nwant=%#v", exact, got, want)
	}
}

func TestPlanCompilerStringConcatRejectsUnsupportedOperator(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	left, _ := factflow.NewStringLiteralValueSource("a", 0, 0, 0, shape)
	right, _ := factflow.NewStringLiteralValueSource("b", 0, 0, 0, shape)
	ref := factflow.ExprRef(9)
	source, _ := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
	op, _ := factflow.NewBinaryExpressionOperation("+", left, right)
	plan := operationplan.New(graph, factflow.FactsInput{
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{ref: op},
		Returns:              map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{source})},
	})
	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	if reason := relation.ContextualReason(); !strings.Contains(reason, "not exact string concatenation") || relation.Rows() != 0 {
		t.Fatalf("unsupported operator reason/rows = %q/%d", reason, relation.Rows())
	}
}
