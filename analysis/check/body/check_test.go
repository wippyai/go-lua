package body

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func requireLocalAssignmentExprByName(t *testing.T, result *Result, name string) (cfg.Point, ast.Expr) {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || fact.Name != name || fact.Expr == nil {
			continue
		}
		return point, fact.Expr
	}
	t.Fatalf("local assignment %q not found", name)
	return 0, nil
}

func onlyCallPoint(t *testing.T, result *Result) (cfg.Point, bool) {
	t.Helper()
	found := cfg.Point(0)
	for _, point := range result.Graph().RPO() {
		if _, ok := result.CallSiteView(point); !ok {
			continue
		}
		if found != 0 {
			t.Fatalf("found multiple call points: %d and %d", found, point)
		}
		found = point
	}
	return found, found != 0
}

func assertSourceLiteralInt(t *testing.T, reg *axis.Registry, facts factflow.Facts, source factflow.ValueSource, want int64) {
	t.Helper()
	switch source.Kind {
	case factflow.ValueSourceLiteral:
		if source.LiteralKind != factflow.ValueSourceLiteralInteger || source.Int != want {
			t.Fatalf("literal source = %#v, want integer literal %d", source, want)
		}
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			t.Fatalf("expression source has no expr: %#v", source)
		}
		value, ok := facts.ExpressionValue(source.ExprRef)
		if !ok {
			t.Fatalf("source expr %d has no expression value", source.ExprRef)
		}
		if got, ok := typevalue.TypeOf(reg, value); !ok || !typ.TypeEquals(got, typ.LiteralInt(want)) {
			t.Fatalf("source expr type = %v/%v, want literal %d", got, ok, want)
		}
	default:
		t.Fatalf("source = %#v, want integer literal source", source)
	}
}

func collectConcatLeaves(expr ast.Expr, first *ast.Expr, last *ast.Expr) {
	if concat, ok := expr.(*ast.StringConcatOpExpr); ok {
		collectConcatLeaves(concat.Lhs, first, last)
		collectConcatLeaves(concat.Rhs, first, last)
		return
	}
	if *first == nil {
		*first = expr
	}
	*last = expr
}

func tableHasNestedValueField(table *ast.TableExpr) bool {
	if table == nil {
		return false
	}
	for _, field := range table.Fields {
		key, ok := field.Key.(*ast.StringExpr)
		if !ok || key.Value != "value" {
			continue
		}
		_, nested := field.Value.(*ast.TableExpr)
		return nested
	}
	return false
}

func suffixNames(p path.Path) string {
	names := make([]string, 0, len(p.Segments))
	for _, seg := range p.Segments {
		names = append(names, seg.Name)
	}
	return strings.Join(names, ".")
}

func localSourcePath(t *testing.T, result *Result, name string) (cfg.Point, path.Path) {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if !ok || fact.Name != name || fact.Expr == nil {
			continue
		}
		exprPath, ok := result.ExpressionPath(fact.Expr)
		if !ok {
			t.Fatalf("local %s at point %d has no expression path", name, point)
		}
		return point, exprPath
	}
	t.Fatalf("local %s not found", name)
	return 0, path.Path{}
}

func postAssignmentSymbolValue(t *testing.T, result *Result, point cfg.Point, sym symbol.ID) (product.Value, bool) {
	t.Helper()
	succs := result.Graph().Successors(point)
	if len(succs) != 1 {
		t.Fatalf("assignment %d successors = %v, want one", point, succs)
	}
	post, ok := result.StateAt(succs[0])
	if !ok {
		t.Fatalf("missing state after assignment %d", point)
	}
	value := post.ReadValue(result.registry, key.SymbolValue(sym))
	if product.Equal(result.registry, value, product.Bottom(result.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func TestCopyConfigCopiesMutableFields(t *testing.T) {
	reg := standard.Registry()
	expr := factflow.ExprRef(42)
	initial := map[factflow.ExprRef]product.Value{
		expr: product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
	}
	config := Config{
		Registry:         reg,
		Globals:          []string{"before"},
		GlobalTypes:      map[string]typ.Type{"typed": typ.String},
		ExpressionValues: initial,
	}

	copied := copyConfig(config)
	config.Globals[0] = "after"
	config.GlobalTypes["typed"] = typ.Number
	initial[expr] = product.Absent(reg)

	if got := copied.Globals; len(got) != 1 || got[0] != "before" {
		t.Fatalf("copied globals = %v, want [before]", got)
	}
	if got := copied.GlobalTypes["typed"]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("copied global type = %s, want string", got)
	}
	gotValue := copied.ExpressionValues[expr]
	if gotPresence := product.PresenceOf(gotValue); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("copied expression presence = %s, want present", gotPresence)
	}
}

func parseChunk(t testing.TB, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "check_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func parseFunction(t testing.TB, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := parseChunk(t, src)
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	return def.Func
}

func mustLocalAt(t *testing.T, result *Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := result.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("local symbol at %d is zero", index)
	}
	return locals[index]
}

func mustBoundLocalAt(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := bindings.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("bound local index %d out of range for %d locals", index, len(locals))
	}
	if locals[index] == 0 {
		t.Fatalf("bound local symbol at %d is zero", index)
	}
	return locals[index]
}

func mustIdentSymbol(t *testing.T, bindings *bind.Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := bindings.SymbolOf(ident)
	if !ok || id == 0 {
		t.Fatalf("missing symbol for ident %q", ident.Value)
	}
	return id
}

func mustParamSlot(t *testing.T, bindings *bind.Result, fn *ast.FunctionExpr, index int) bind.ParamSlot {
	t.Helper()
	slots := bindings.ParamSlots(fn)
	if index < 0 || index >= len(slots) {
		t.Fatalf("param slot index %d out of range for %d slots", index, len(slots))
	}
	if slots[index].Symbol == 0 {
		t.Fatalf("param slot %d has zero symbol", index)
	}
	return slots[index]
}

func requireCheckStmtPoint(t *testing.T, built *cfgbuild.Result, stmt ast.Stmt) cfg.Point {
	t.Helper()
	if built == nil {
		t.Fatalf("missing cfg build result")
	}
	points := built.StmtPoints.PointsFor(stmt)
	if len(points) != 1 {
		t.Fatalf("stmt points = %v, want one point", points)
	}
	return points[0]
}

func requireBranchPointForStmt(t *testing.T, result *Result, stmt ast.Stmt) cfg.Point {
	t.Helper()
	if result == nil || result.cfg == nil || result.Graph() == nil {
		t.Fatalf("missing result CFG")
	}
	points := result.cfg.StmtPoints.PointsFor(stmt)
	for i := len(points) - 1; i >= 0; i-- {
		if result.Graph().IsBranch(points[i]) {
			return points[i]
		}
	}
	t.Fatalf("stmt points %v contain no branch point", points)
	return 0
}

func requireLocalAssignmentPoint(t *testing.T, result *Result, stmt *ast.LocalAssignStmt, index int) cfg.Point {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.LocalAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point
		}
	}
	t.Fatalf("missing local assignment point for index %d", index)
	return 0
}

func requireOrdinaryAssignmentPoint(t *testing.T, result *Result, stmt *ast.AssignStmt, index int) cfg.Point {
	t.Helper()
	for _, point := range result.Graph().RPO() {
		fact, ok := result.OrdinaryAssignment(point)
		if ok && fact.Stmt == stmt && fact.Index == index {
			return point
		}
	}
	t.Fatalf("missing ordinary assignment point for index %d", index)
	return 0
}

func assertProductEqual(t *testing.T, reg *axis.Registry, got, want product.Value) {
	t.Helper()
	if !product.Equal(reg, got, want) {
		t.Fatalf("value = %v, want %v", got, want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtimekind = %s, want %s", kind, want)
	}
}

func assertPresence(t *testing.T, _ *axis.Registry, got product.Value, want presence.Value) {
	t.Helper()
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("presence = %s, want %s", gotPresence, want)
	}
}

func assertConcreteTypeWitness(t *testing.T, reg *axis.Registry, got product.Value) {
	t.Helper()
	witness := product.Get(reg, got, typewitness.Key)
	if _, ok := witness.Type(); !ok {
		t.Fatalf("type witness = %v, want concrete witness", witness)
	}
}

type markValue uint8

const (
	markBottom markValue = iota
	markLow
	markHigh
)

func testRegistry(t *testing.T) (*axis.Registry, axis.Key[markValue]) {
	t.Helper()
	markKey := axis.NewKey[markValue]("check.test.mark." + strings.ReplaceAll(t.Name(), "/", "."))
	reg, err := standard.RegistryWithAxes(axis.Spec[markValue]{
		Key:    markKey,
		Bottom: func() markValue { return markBottom },
		Top:    func() markValue { return markHigh },
		Equal:  func(a, b markValue) bool { return a == b },
		LessOrEq: func(a, b markValue) bool {
			return a == b || a == markBottom || b == markHigh
		},
		Join: func(a, b markValue) markValue {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b markValue) markValue {
			if a < b {
				return a
			}
			return b
		},
		Widen:     func(_, next markValue) markValue { return next },
		Hash:      func(v markValue) uint64 { return uint64(v) },
		Boundary:  axis.PortableIdentity,
		Retention: axis.ImmutableRetention[markValue](),
		Canonical: axis.PendingCanonical[markValue]("test-only axis"),
	}.Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes: %v", err)
	}
	return reg, markKey
}
