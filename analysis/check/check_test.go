package check

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	reg := standard.Registry()
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
	reg := standard.Registry()
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

func TestCheckChunkManifestSameAsSignatureUsesArgumentSourceValue(t *testing.T) {
	reg := standard.Registry()
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	m := manifest.New("test")
	m.DefineFunctionSignature("id", signature.Function{
		Type:   typ.Func().Param("value", typ.Any).Returns(typ.Number).Build(),
		Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
	})
	stmts := parseChunk(t, `local x: string = id("s")`)

	result, err := CheckChunk(stmts, Config{
		Registry: reg,
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return argValue, true
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
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestCheckChunkDefaultExpressionValueProjectsStaticReadOptionality(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local out = t["name"]`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"t"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	local := stmts[0].(*ast.LocalAssignStmt)
	assignPoint := requireCheckStmtPoint(t, built, local)
	attr := local.Exprs[0].(*ast.AttrGetExpr)
	tSym := mustIdentSymbol(t, bindings, attr.Object.(*ast.IdentExpr))
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(assignPoint, tSym, "t")
	resolver := visibility.NewResolver(resolverBuilder.Build())
	entry := state.State{}.WriteValue(
		reg,
		key.SymbolValue(tSym),
		product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table)),
	)

	result, err := CheckBoundChunk(stmts, bindings, Config{
		Registry:   reg,
		Globals:    []string{"t"},
		Visibility: resolver,
		EntryState: entry,
	})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}

	out := mustLocalAt(t, result, local, 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	assertPresence(t, reg, exit.ReadValue(reg, key.SymbolValue(out)), presence.Top())
}

func TestCheckChunkDefaultExpressionValueUsesExactPathPresenceProof(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local out = t.name`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"t"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	local := stmts[0].(*ast.LocalAssignStmt)
	assignPoint := requireCheckStmtPoint(t, built, local)
	attr := local.Exprs[0].(*ast.AttrGetExpr)
	tSym := mustIdentSymbol(t, bindings, attr.Object.(*ast.IdentExpr))
	readPath := path.NewPath(tSym, "t").Field("name")
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(assignPoint, tSym, "t")
	resolver := visibility.NewResolver(resolverBuilder.Build())
	childValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	entry := state.State{}.WritePathKey(reg, resolver.KeyAt(assignPoint, readPath), childValue)

	result, err := CheckBoundChunk(stmts, bindings, Config{
		Registry:   reg,
		Globals:    []string{"t"},
		Visibility: resolver,
		EntryState: entry,
	})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}

	out := mustLocalAt(t, result, local, 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got := exit.ReadValue(reg, key.SymbolValue(out))
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestCheckChunkUserExpressionValueOverridesDefaultStaticReadProjector(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local out = t.name`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"t"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	local := stmts[0].(*ast.LocalAssignStmt)
	assignPoint := requireCheckStmtPoint(t, built, local)
	attr := local.Exprs[0].(*ast.AttrGetExpr)
	tSym := mustIdentSymbol(t, bindings, attr.Object.(*ast.IdentExpr))
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(assignPoint, tSym, "t")
	override := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Absent()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.Nil),
	)

	result, err := CheckBoundChunk(stmts, bindings, Config{
		Registry:   reg,
		Globals:    []string{"t"},
		Visibility: visibility.NewResolver(resolverBuilder.Build()),
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return override, true
		},
	})
	if err != nil {
		t.Fatalf("CheckBoundChunk: %v", err)
	}

	out := mustLocalAt(t, result, local, 0)
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	got := exit.ReadValue(reg, key.SymbolValue(out))
	assertPresence(t, reg, got, presence.Absent())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.Nil))
}

func TestCheckBoundFunctionUsesSuppliedBindingIdentity(t *testing.T) {
	reg, markKey := testRegistry(t)
	want := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), markKey, markLow)
	stmts := parseChunk(t, `
local captured = 1
function f()
	local value = captured
	return value
end`)
	capturedDecl, ok := stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt 0 = %T, want *ast.LocalAssignStmt", stmts[0])
	}
	def, ok := stmts[1].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt 1 = %T, want function definition", stmts[1])
	}
	valueDecl, ok := def.Func.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("function stmt 0 = %T, want *ast.LocalAssignStmt", def.Func.Stmts[0])
	}

	bindings := bind.BindChunk(stmts, bind.Options{})
	captured := mustBoundLocalAt(t, bindings, capturedDecl, 0)
	suppliedLocal := mustBoundLocalAt(t, bindings, valueDecl, 0)
	captures := bindings.DirectCaptures(def.Func)
	if len(captures) != 1 || captures[0].Captured != captured {
		t.Fatalf("DirectCaptures = %+v, want captured symbol %d", captures, captured)
	}

	config := Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, _ factflow.ValueSource, _ state.State) (product.Value, bool) {
			return want, true
		},
	}
	result, err := CheckBoundFunction(def.Func, bindings, config)
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}
	if got := mustLocalAt(t, result, valueDecl, 0); got != suppliedLocal {
		t.Fatalf("bound result local = %d, want supplied binding local %d", got, suppliedLocal)
	}
	exit, ok := result.ExitState()
	if !ok {
		t.Fatalf("missing exit state")
	}
	assertProductEqual(t, reg, exit.ReadValue(reg, key.SymbolValue(suppliedLocal)), want)

	independent, err := CheckFunction(def.Func, config)
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	if got := mustLocalAt(t, independent, valueDecl, 0); got == suppliedLocal {
		t.Fatalf("independent CheckFunction local = %d, want a fresh rebind", got)
	}
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
	reg := standard.Registry()
	stmts := parseChunk(t, "print(value())")

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
		Widen: func(_, next markValue) markValue { return next },
		Hash:  func(v markValue) uint64 { return uint64(v) },
	}.Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes: %v", err)
	}
	return reg, markKey
}
