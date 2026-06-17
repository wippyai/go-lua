package sourceprovenance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func call(name string) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident(name)}
}

func wrappedCall(name string) ast.Expr {
	return &ast.CastExpr{
		Expr: &ast.NonNilAssertExpr{
			Expr: call(name),
		},
	}
}

func pointResolver(points map[int]cfg.Point) CallPointResolver {
	return func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		point, ok := points[exprIndex]
		return point, ok
	}
}

func TestAssignmentSourcesShape(t *testing.T) {
	t.Run("fixed targets and expanding final call", func(t *testing.T) {
		exprs := []ast.Expr{ident("x"), call("pack")}
		sources := AssignmentSources(exprs, 4, pointResolver(map[int]cfg.Point{1: cfg.Point(10)}))
		if len(sources) != 4 {
			t.Fatalf("len(sources) = %d, want 4", len(sources))
		}

		if got := sources[0]; got.Kind != SourceExpression || got.Expr != exprs[0] || got.ExprIndex != 0 || got.TargetIndex != 0 || got.ResultIndex != 0 || got.Final || got.Expanded || got.Adjusted {
			t.Fatalf("fixed target source = %#v", got)
		}
		if got := sources[1]; got.Kind != SourceCall || got.Expr != exprs[1] || got.ExprIndex != 1 || got.TargetIndex != 1 || got.ResultIndex != 0 || !got.Final || !got.Expanded || got.Adjusted {
			t.Fatalf("final call source = %#v", got)
		}
		if got := sources[2]; got.TargetIndex != 2 || got.ResultIndex != 1 || !got.Final || !got.Expanded || got.Kind != SourceCall {
			t.Fatalf("expanded target source = %#v", got)
		}
		if got := sources[3]; got.TargetIndex != 3 || got.ResultIndex != 2 || !got.Final || !got.Expanded || got.Kind != SourceCall {
			t.Fatalf("later expanded target source = %#v", got)
		}
	})

	t.Run("non-expanding final fills nil", func(t *testing.T) {
		sources := AssignmentSources([]ast.Expr{ident("x"), ident("y")}, 3, nil)
		if len(sources) != 3 {
			t.Fatalf("len(sources) = %d, want 3", len(sources))
		}

		if got := sources[2]; got.Kind != SourceNil || got.Expr != nil || got.ExprIndex != NoSourceIndex || got.TargetIndex != 2 || got.ResultIndex != NoSourceIndex {
			t.Fatalf("nil fill source = %#v", got)
		}
	})
}

func TestValueListSourcesShape(t *testing.T) {
	exprs := []ast.Expr{call("left"), call("tail")}
	resolver := pointResolver(map[int]cfg.Point{
		0: cfg.Point(10),
		1: cfg.Point(11),
	})

	t.Run("return open tail final call expands and keeps open tail", func(t *testing.T) {
		sources := ValueListSources(exprs, true, resolver)
		if len(sources) != 2 {
			t.Fatalf("len(sources) = %d, want 2", len(sources))
		}

		if got := sources[1]; got.Kind != SourceCall || !got.Final || !got.Expanded || !got.OpenTail || got.Adjusted {
			t.Fatalf("return tail source = %#v", got)
		}
	})

	t.Run("closed tail final call expands without open tail", func(t *testing.T) {
		sources := ValueListSources(exprs, false, resolver)
		if len(sources) != 2 {
			t.Fatalf("len(sources) = %d, want 2", len(sources))
		}

		if got := sources[1]; got.Kind != SourceCall || !got.Final || !got.Expanded || got.OpenTail || got.Adjusted {
			t.Fatalf("closed tail source = %#v", got)
		}
	})
}

func TestConditionSourceShape(t *testing.T) {
	source := ConditionSource(wrappedCall("pred"), pointResolver(map[int]cfg.Point{0: cfg.Point(12)}))
	if source.Kind != SourceCall {
		t.Fatalf("kind = %v, want call", source.Kind)
	}
	if source.ExprIndex != 0 || source.TargetIndex != NoSourceIndex || source.ResultIndex != 0 {
		t.Fatalf("indexes = %#v, want condition source shape", source)
	}
	if !source.Final || !source.Adjusted || source.Expanded || source.OpenTail {
		t.Fatalf("condition flags = %#v, want adjusted multi-value without expansion", source)
	}
}

func TestUnresolvedCallPointSourcesAreExplicitUnknown(t *testing.T) {
	source := SourceForExpr(call("missing"), 0, 0, 0, true, false, nil)
	if source.Kind != SourceUnknown {
		t.Fatalf("kind = %v, want unknown", source.Kind)
	}
	if source.HasCallPoint || source.CallPoint != 0 {
		t.Fatalf("call point = %#v, want none", source)
	}
	if source.Expr != nil || source.ExprIndex != NoSourceIndex || source.ResultIndex != NoSourceIndex {
		t.Fatalf("unknown source fields = %#v", source)
	}
	if !source.Valid() {
		t.Fatalf("unknown source is invalid: %#v", source)
	}

	condition := ConditionSource(wrappedCall("pred"), nil)
	if condition.Kind != SourceUnknown || !condition.Valid() {
		t.Fatalf("unresolved condition source = %#v, want valid unknown", condition)
	}
}

func TestNilExprPublicSourcesAreExplicitUnknown(t *testing.T) {
	source := SourceForExpr(nil, 0, 0, 0, true, false, nil)
	if source.Kind != SourceUnknown || source.TargetIndex != 0 || !source.Valid() {
		t.Fatalf("nil expr source = %#v, want valid unknown for target 0", source)
	}

	condition := ConditionSource(nil, nil)
	if condition.Kind != SourceUnknown || condition.TargetIndex != NoSourceIndex || !condition.Valid() {
		t.Fatalf("nil condition source = %#v, want valid condition unknown", condition)
	}

	values := ValueListSources([]ast.Expr{nil}, false, nil)
	if len(values) != 1 || values[0].Kind != SourceUnknown || values[0].TargetIndex != 0 || !values[0].Valid() {
		t.Fatalf("nil value-list source = %#v, want one valid unknown", values)
	}

	assignments := AssignmentSources([]ast.Expr{nil}, 2, nil)
	if len(assignments) != 2 {
		t.Fatalf("assignment sources len = %d, want 2", len(assignments))
	}
	if assignments[0].Kind != SourceUnknown || assignments[0].TargetIndex != 0 || !assignments[0].Valid() {
		t.Fatalf("nil assignment expr source = %#v, want valid unknown", assignments[0])
	}
	if assignments[1].Kind != SourceNil || assignments[1].TargetIndex != 1 || !assignments[1].Valid() {
		t.Fatalf("nil-filled assignment source = %#v, want valid nil fill", assignments[1])
	}
}

func TestTypedNilExprPublicSourcesAreExplicitUnknown(t *testing.T) {
	var callExpr *ast.FuncCallExpr
	source := SourceForExpr(callExpr, 0, 0, 0, true, false, func(int, *ast.FuncCallExpr) (cfg.Point, bool) {
		t.Fatal("typed nil call should not reach resolver")
		return 0, false
	})
	if source.Kind != SourceUnknown || source.TargetIndex != 0 || !source.Valid() {
		t.Fatalf("typed nil call source = %#v, want valid unknown", source)
	}

	var castExpr *ast.CastExpr
	condition := ConditionSource(castExpr, nil)
	if condition.Kind != SourceUnknown || condition.TargetIndex != NoSourceIndex || !condition.Valid() {
		t.Fatalf("typed nil condition source = %#v, want valid unknown", condition)
	}
}

func TestBrokenAssertionWrapperSourcesAreExplicitUnknown(t *testing.T) {
	broken := &ast.CastExpr{
		Expr: &ast.NonNilAssertExpr{},
		Type: &ast.PrimitiveTypeExpr{Name: "number"},
	}
	source := SourceForExpr(broken, 0, 0, 0, true, false, nil)
	if source.Kind != SourceUnknown || source.TargetIndex != 0 || !source.Valid() {
		t.Fatalf("broken wrapper source = %#v, want valid unknown", source)
	}

	condition := ConditionSource(broken, nil)
	if condition.Kind != SourceUnknown || condition.TargetIndex != NoSourceIndex || !condition.Valid() {
		t.Fatalf("broken wrapper condition source = %#v, want valid unknown", condition)
	}
}

func TestASTSourceConstructorsRejectInvalidCombinations(t *testing.T) {
	invalidShape := SourceShape{Final: true, Expanded: true, Adjusted: true}
	if _, ok := NewExpressionSource(ident("x"), 0, 0, 0, invalidShape); ok {
		t.Fatalf("expression source accepted invalid shape")
	}
	if _, ok := NewCallSource(call("f"), 0, 0, 0, cfg.Point(1), invalidShape); ok {
		t.Fatalf("call source accepted invalid shape")
	}
	if _, ok := NewVarargSource(&ast.Comma3Expr{}, 0, 0, 0, invalidShape); ok {
		t.Fatalf("vararg source accepted invalid shape")
	}

	plainShape, ok := NewSourceShape(false, false, false, false)
	if !ok {
		t.Fatalf("plain shape rejected")
	}
	if _, ok := NewExpressionSource(nil, 0, 0, 0, plainShape); ok {
		t.Fatalf("expression source accepted nil expr")
	}
	if _, ok := NewVarargSource(nil, 0, 0, 0, plainShape); ok {
		t.Fatalf("vararg source accepted nil expr")
	}
	if _, ok := NewCallSource(call("missing"), 0, 0, 0, 0, plainShape); ok {
		t.Fatalf("call source accepted missing call point")
	}
	if _, ok := NewCallSource(call("missing"), 0, 0, NoSourceIndex, cfg.Point(1), plainShape); ok {
		t.Fatalf("call source accepted missing result index")
	}

	var typedNilIdent *ast.IdentExpr
	if _, ok := NewExpressionSource(typedNilIdent, 0, 0, 0, plainShape); ok {
		t.Fatalf("expression source accepted typed nil expr")
	}
	var typedNilCall *ast.FuncCallExpr
	if _, ok := NewCallSource(typedNilCall, 0, 0, 0, cfg.Point(1), plainShape); ok {
		t.Fatalf("call source accepted typed nil expr")
	}
	var typedNilVararg *ast.Comma3Expr
	if _, ok := NewVarargSource(typedNilVararg, 0, 0, 0, plainShape); ok {
		t.Fatalf("vararg source accepted typed nil expr")
	}
}

func TestSourceForExprResolvesTopLevelProducerCallPointThroughWrappers(t *testing.T) {
	inner := call("resolve")
	expr := &ast.CastExpr{
		Expr: &ast.NonNilAssertExpr{
			Expr: inner,
		},
	}
	var gotIndex int
	var gotCall *ast.FuncCallExpr

	source := SourceForExpr(expr, 7, 2, 3, true, false, func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		gotIndex = exprIndex
		gotCall = call
		return cfg.Point(42), true
	})

	if gotIndex != 7 {
		t.Fatalf("resolver exprIndex = %d, want 7", gotIndex)
	}
	if gotCall != inner {
		t.Fatalf("resolver call = %p, want inner call", gotCall)
	}
	if !source.HasCallPoint || source.CallPoint != cfg.Point(42) {
		t.Fatalf("call point = %#v, want 42", source)
	}
	if source.Expr != expr {
		t.Fatalf("expr = %T %p, want wrapped expr %p", source.Expr, source.Expr, expr)
	}
}
