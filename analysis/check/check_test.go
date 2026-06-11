package check

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestNewRequiresRegistry(t *testing.T) {
	_, err := New(Config{})
	if !errors.Is(err, ErrRegistryRequired) {
		t.Fatalf("New error = %v, want ErrRegistryRequired", err)
	}
}

func TestCheckChunkAssignsLocalFromExpressionValue(t *testing.T) {
	reg, markKey := testRegistry(t)
	stmts := parseChunk(t, "local x = 1")
	want := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), markKey, markLow)

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return want, true
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	x := mustLocalAt(t, result, stmts[0].(*ast.LocalAssignStmt), 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got := exit.ReadValue(reg, key.SymbolValue(x))
	assertProductEqual(t, reg, got, want)
	if gotMark := product.Get(reg, got, markKey); gotMark != markLow {
		t.Fatalf("custom axis = %v, want %v", gotMark, markLow)
	}
}

func TestCheckFunctionRunsIntraprocedurally(t *testing.T) {
	reg := product.DefaultRegistry()
	fn := parseFunction(t, "function f(a) local b = a return b end")

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	if result.Registry() != reg {
		t.Fatalf("result registry = %p, want %p", result.Registry(), reg)
	}
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	if len(graph.RPO()) == 0 {
		t.Fatalf("CFG RPO is empty")
	}
	if _, ok := result.StateAt(graph.Entry()); !ok {
		t.Fatalf("flow has no entry state")
	}
	if _, ok := result.ExitState(); !ok {
		t.Fatalf("flow has no exit state")
	}
	returnPoints := result.ReturnPoints()
	if len(returnPoints) != 1 {
		t.Fatalf("return points = %v, want one point", returnPoints)
	}
	returnFact, ok := result.ReturnFact(returnPoints[0])
	if !ok {
		t.Fatalf("missing return fact at %v", returnPoints[0])
	}
	if len(returnFact.Exprs) != 1 {
		t.Fatalf("return fact has %d exprs, want 1", len(returnFact.Exprs))
	}
}

func TestCheckFunctionReturnArityUsesLoweredFacts(t *testing.T) {
	reg := product.DefaultRegistry()
	fn := parseFunction(t, "function f(a) return a, nil end")

	result, err := CheckFunction(fn, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	returnPoints := result.ReturnPoints()
	if len(returnPoints) != 1 {
		t.Fatalf("return points = %v, want one point", returnPoints)
	}
	arity, ok := result.ReturnArity(returnPoints[0])
	if !ok {
		t.Fatalf("missing lowered return arity at %v", returnPoints[0])
	}
	if arity != 2 {
		t.Fatalf("return arity = %d, want 2", arity)
	}
}

func TestCopyConfigCopiesMutableFields(t *testing.T) {
	reg := product.DefaultRegistry()
	expr := factflow.ExprRef(42)
	initial := map[factflow.ExprRef]product.Value{
		expr: product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
	}
	config := Config{
		Registry:         reg,
		Globals:          []string{"before"},
		ExpressionValues: initial,
	}

	copied := copyConfig(config)
	config.Globals[0] = "after"
	initial[expr] = product.Absent(reg)

	if got := copied.Globals; len(got) != 1 || got[0] != "before" {
		t.Fatalf("copied globals = %v, want [before]", got)
	}
	gotValue := copied.ExpressionValues[expr]
	if gotPresence := product.PresenceOf(gotValue); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("copied expression presence = %s, want present", gotPresence)
	}
}

func TestCheckChunkReturnsUnsupportedCFG(t *testing.T) {
	reg := product.DefaultRegistry()
	stmts := parseChunk(t, "local f = function() end")

	_, err := CheckChunk(stmts, Config{Registry: reg})
	if !errors.Is(err, ErrUnsupportedCFG) {
		t.Fatalf("CheckChunk error = %v, want ErrUnsupportedCFG", err)
	}
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "check_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func parseFunction(t *testing.T, src string) *ast.FunctionExpr {
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

func assertProductEqual(t *testing.T, reg *axis.Registry, got, want product.Value) {
	t.Helper()
	if !product.Equal(reg, got, want) {
		t.Fatalf("value = %v, want %v", got, want)
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
	reg, err := product.DefaultRegistryWithAxes(axis.Spec[markValue]{
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
		Widen: func(_, next markValue) markValue { return next },
		Hash:  func(v markValue) uint64 { return uint64(v) },
	}.Erase())
	if err != nil {
		t.Fatalf("DefaultRegistryWithAxes: %v", err)
	}
	return reg, markKey
}
