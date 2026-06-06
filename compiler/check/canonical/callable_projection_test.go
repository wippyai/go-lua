package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/signature"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/modules"
	"github.com/wippyai/go-lua/compiler/check/scope"
	checkstore "github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/compiler/check/synth/resolve"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestSignatureForRefUsesMethodSurfaceAndDeclaredReturn(t *testing.T) {
	t.Parallel()

	ref := summary.FuncRef{GraphID: 42}
	app := typ.NewRecord().Build()
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"name"},
			Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{
				Name: "string",
			}},
		},
		ReturnTypes: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"App"}}},
	}
	info := &cfg.FuncDefInfo{
		ReceiverName: "App",
		IsMethod:     true,
		FuncExpr:     fn,
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins().WithType("App", app)})
	prog := &program{
		funcTopology: topology.NewFunctionTopology([]topology.FunctionEntry{
			{Ref: ref, Function: fn, MethodDef: info},
		}),
	}

	got := driver.signatureForRefWithMode(prog, ref, signature.ReturnDeclaredThenInferred, func(*ast.FunctionExpr) []typ.Type {
		return []typ.Type{typ.NewOptional(app)}
	})

	if got == nil || len(got.Params) != 2 {
		t.Fatalf("signature params = %#v, want self + name", got)
	}
	if got.Params[0].Name != "self" || got.Params[0].Optional || !typ.TypeEquals(got.Params[0].Type, app) {
		t.Fatalf("self param = %#v, want required App", got.Params[0])
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], app) {
		t.Fatalf("returns = %#v, want declared App", got.Returns)
	}
}

func TestDriverProgramMethodRefSignaturePreservesRecursiveDeclaredReturn(t *testing.T) {
	t.Parallel()

	chunk, err := parse.ParseString(`
type Handler = (string) -> string
type App = {
    handlers: {[string]: Handler},
    register: (self: App, name: string, handler: Handler) -> App,
}

local App = {}

function App:register(name: string, handler: Handler): App
    self.handlers[name] = handler
    return self
end
`, "method-ref.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession("method-ref.lua")
	root := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: chunk}
	sess.SetRootFuncNode(root)
	moduleBindings := bind.Bind(root, driver.globalTypes.Names())
	if store := sess.StoreHandle(); store != nil {
		store.SetModuleBindings(moduleBindings)
	}
	rootGraph := sess.GetOrBuildCFG(root)
	if rootGraph == nil {
		t.Fatal("root graph not built")
	}
	sess.RegisterGraphHierarchy(rootGraph)
	moduleAliases := topology.DiscoverModuleAliases(topology.ModuleAliasDiscoveryInput{
		Root:         rootGraph,
		GraphForFunc: sess.GetOrBuildCFG,
		AliasesForGraph: func(g *cfg.Graph) map[cfg.SymbolID]string {
			evidence := sess.EvidenceForGraph(g)
			return modules.AliasesFromAssignments(evidence.Assignments, g)
		},
	})
	driver.resolver = resolve.New(resolve.Config{
		ModuleBindings: moduleBindings,
		ModuleAliases:  moduleAliases,
	})
	driver.typedefCache = make(map[ast.TypeExpr]typ.Type)
	driver.moduleScope = driver.buildModuleScope(sess, rootGraph)
	driver.pointScopes = driver.buildHierarchyScopes(sess, rootGraph)
	prog := driver.buildProgram(sess, rootGraph, topology.ResolveModuleAliases(moduleAliases, driver.cfg.Manifests))

	var methodRef summary.FuncRef
	for _, ref := range prog.refs {
		info := prog.methodDef(ref)
		if info != nil && info.ReceiverName == "App" && info.Name == "register" {
			methodRef = ref
			break
		}
	}
	if methodRef == (summary.FuncRef{}) {
		t.Fatal("App:register method ref not found")
	}

	sig := driver.signatureForRef(prog, methodRef)
	if sig == nil || len(sig.Params) != 3 {
		t.Fatalf("signature = %#v, want self + name + handler", sig)
	}
	app := driver.resolveType(&ast.TypeRefExpr{Path: []string{"App"}}, driver.baseScope())
	if app == nil {
		t.Fatal("App type did not resolve")
	}
	if len(sig.Returns) != 1 || !typ.TypeEquals(sig.Returns[0], app) {
		t.Fatalf("returns = %#v, want declared App %v", sig.Returns, app)
	}
}

func TestCallableProjectorUsesSummaryReaderForFunctionRefReturns(t *testing.T) {
	t.Parallel()

	ref := summary.FuncRef{GraphID: 7}
	path := constraint.NewPath(cfg.SymbolID(10), "make")
	state := flow.PointState{
		FunctionRefs: flow.WithFunctionRef(nil, path.Key(), flow.FunctionRefSetOf(canonref.ToFlow(ref))),
	}
	projector := callableProjector{
		prog: &program{declaredReturns: map[summary.FuncRef][]typ.Type{}},
		reader: summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{
			ref: {Returns: []product.AbstractValue{product.FromType(typ.Number)}},
		}),
		baseSignature: func(got summary.FuncRef) *typ.Function {
			if got != ref {
				t.Fatalf("base signature ref = %#v, want %#v", got, ref)
			}
			return typ.Func().Param("x", typ.String).Build()
		},
	}

	got := projector.TypeAt(state, path)
	fn := unwrap.Function(got)
	if fn == nil {
		t.Fatalf("TypeAt = %v, want function", got)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		t.Fatalf("params = %#v, want original string param", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.Number) {
		t.Fatalf("returns = %#v, want [number]", fn.Returns)
	}
}

type canonicalTestSession struct {
	ctx       *db.QueryContext
	name      string
	store     *checkstore.SessionStore
	results   map[*ast.FunctionExpr]*api.FuncResult
	root      *ast.FunctionExpr
	rootValue *api.FuncResult
	graphs    map[*ast.FunctionExpr]*cfg.Graph
	diags     []diag.Diagnostic
	scopeDiag map[*ast.FunctionExpr]bool
}

func newCanonicalTestSession(name string) *canonicalTestSession {
	ctx := db.NewQueryContext(db.New())
	store := checkstore.NewSessionStoreWithDB(ctx.DB())
	api.AttachStore(ctx, store)
	sess := &canonicalTestSession{
		ctx:       ctx,
		name:      name,
		store:     store,
		results:   make(map[*ast.FunctionExpr]*api.FuncResult),
		graphs:    make(map[*ast.FunctionExpr]*cfg.Graph),
		scopeDiag: make(map[*ast.FunctionExpr]bool),
	}
	api.AttachGraphs(ctx, sess)
	return sess
}

func (s *canonicalTestSession) Context() *db.QueryContext { return s.ctx }
func (s *canonicalTestSession) Source() string            { return s.name }
func (s *canonicalTestSession) StoreHandle() api.IterationStore {
	if s == nil {
		return nil
	}
	return s.store
}
func (s *canonicalTestSession) ResultsMap() map[*ast.FunctionExpr]*api.FuncResult { return s.results }
func (s *canonicalTestSession) RootFuncNode() *ast.FunctionExpr                   { return s.root }
func (s *canonicalTestSession) SetRootFuncNode(fn *ast.FunctionExpr)              { s.root = fn }
func (s *canonicalTestSession) RootResultValue() *api.FuncResult                  { return s.rootValue }
func (s *canonicalTestSession) SetRootResultValue(result *api.FuncResult)         { s.rootValue = result }
func (s *canonicalTestSession) ResetDiagnostics()                                 { s.diags = nil }
func (s *canonicalTestSession) AppendDiagnostics(diags ...diag.Diagnostic) {
	s.diags = append(s.diags, diags...)
}
func (s *canonicalTestSession) DiagnosticsSlice() []diag.Diagnostic {
	return append([]diag.Diagnostic(nil), s.diags...)
}
func (s *canonicalTestSession) ScopeDepthDiagState() map[*ast.FunctionExpr]bool { return s.scopeDiag }

func (s *canonicalTestSession) GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil {
		return nil
	}
	if g := s.graphs[fn]; g != nil {
		return g
	}
	g := cfg.BuildWithBindings(fn, s.store.ModuleBindings())
	s.graphs[fn] = g
	return g
}

func (s *canonicalTestSession) EvidenceForGraph(graph *cfg.Graph) api.FlowEvidence {
	if s == nil || s.store == nil {
		return api.FlowEvidence{}
	}
	return s.store.EvidenceForGraph(graph)
}

func (s *canonicalTestSession) RegisterGraphHierarchy(root *cfg.Graph) {
	if s == nil || root == nil || s.store == nil {
		return
	}
	visited := make(map[uint64]bool)
	var walk func(*cfg.Graph)
	walk = func(g *cfg.Graph) {
		if g == nil || visited[g.ID()] {
			return
		}
		visited[g.ID()] = true
		if fn := g.Func(); fn != nil {
			s.store.RegisterGraph(g, fn)
			if bindings := g.Bindings(); bindings != nil {
				if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
					s.store.RegisterFunctionRef(sym, fn, g, 0, 0)
				}
			}
		}
		evidence := s.store.EvidenceForGraph(g)
		for _, def := range evidence.FunctionDefinitions {
			if def.Nested.Func == nil {
				continue
			}
			child := s.GetOrBuildCFG(def.Nested.Func)
			if child == nil {
				continue
			}
			s.store.RegisterGraph(child, def.Nested.Func)
			s.store.RegisterNestedMeta(child.ID(), g.ID(), def.Nested.Point)
			nestedSym := def.Symbol
			if nestedSym == 0 && child.Bindings() != nil {
				if sym, ok := child.Bindings().FuncLitSymbol(def.Nested.Func); ok {
					nestedSym = sym
				}
			}
			if nestedSym != 0 {
				s.store.RegisterFunctionRef(nestedSym, def.Nested.Func, child, g.ID(), def.Nested.Point)
			}
			walk(child)
		}
	}
	walk(root)
}

func TestCallableProjectorFunctionTypeByRefUsesSameProjection(t *testing.T) {
	t.Parallel()

	ref := summary.FuncRef{GraphID: 17}
	projector := callableProjector{
		prog: &program{declaredReturns: map[summary.FuncRef][]typ.Type{}},
		reader: summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{
			ref: {Returns: []product.AbstractValue{product.FromType(typ.Boolean)}},
		}),
		baseSignature: func(got summary.FuncRef) *typ.Function {
			if got != ref {
				t.Fatalf("base signature ref = %#v, want %#v", got, ref)
			}
			return typ.Func().Param("x", typ.Number).Build()
		},
	}

	fn := unwrap.Function(projector.FunctionTypeByRef(
		canonref.ToFlow(ref),
		flow.ReferenceContextOf(
			flow.CaptureCellsDomain.Bottom(),
			flow.FunctionRefsDomain.Bottom(),
			flow.ClosureRefsDomain.Bottom(),
		),
	))
	if fn == nil {
		t.Fatal("FunctionTypeByRef = nil, want projected function")
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.Number) {
		t.Fatalf("params = %#v, want original number param", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.Boolean) {
		t.Fatalf("returns = %#v, want summary boolean return", fn.Returns)
	}
}

func TestCallableProjectorPreservesDeclaredReturnSignature(t *testing.T) {
	t.Parallel()

	ref := summary.FuncRef{GraphID: 8}
	path := constraint.NewPath(cfg.SymbolID(11), "declared")
	state := flow.PointState{
		FunctionRefs: flow.WithFunctionRef(nil, path.Key(), flow.FunctionRefSetOf(canonref.ToFlow(ref))),
	}
	projector := callableProjector{
		prog: &program{declaredReturns: map[summary.FuncRef][]typ.Type{
			ref: {typ.Boolean},
		}},
		reader: summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{
			ref: {Returns: []product.AbstractValue{product.FromType(typ.Number)}},
		}),
		baseSignature: func(summary.FuncRef) *typ.Function {
			return typ.Func().Returns(typ.Boolean).Build()
		},
		hasDeclaredReturns: func(got summary.FuncRef) bool {
			return got == ref
		},
	}

	fn := unwrap.Function(projector.TypeAt(state, path))
	if fn == nil || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.Boolean) {
		t.Fatalf("returns = %#v, want declared [boolean]", fn.Returns)
	}
}

func TestCallableProjectorUsesStaticFunctionRefWhenLiveAxesAbsent(t *testing.T) {
	t.Parallel()

	sym := cfg.SymbolID(31)
	ref := summary.FuncRef{GraphID: 18}
	path := constraint.NewPath(sym, "later")
	projector := callableProjector{
		prog: &program{
			funcTopology: topology.NewFunctionTopology([]topology.FunctionEntry{
				{Ref: ref, Symbols: []cfg.SymbolID{sym}},
			}),
			declaredReturns: map[summary.FuncRef][]typ.Type{},
		},
		reader: summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{
			ref: {Returns: []product.AbstractValue{product.FromType(typ.Number)}},
		}),
		baseSignature: func(got summary.FuncRef) *typ.Function {
			if got != ref {
				t.Fatalf("base signature ref = %#v, want %#v", got, ref)
			}
			return typ.Func().Build()
		},
	}

	fn := unwrap.Function(projector.TypeAt(flow.PointState{}, path))
	if fn == nil || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.Number) {
		t.Fatalf("static callable returns = %#v, want number", fn.Returns)
	}
}

func TestCallableProjectorLiveFunctionTopBlocksStaticFallback(t *testing.T) {
	t.Parallel()

	sym := cfg.SymbolID(32)
	ref := summary.FuncRef{GraphID: 19}
	path := constraint.NewPath(sym, "maybe")
	state := flow.PointState{
		FunctionRefs: flow.WithFunctionRef(nil, path.Key(), flow.FunctionRefSetTop()),
	}
	projector := callableProjector{
		prog: &program{
			funcTopology: topology.NewFunctionTopology([]topology.FunctionEntry{
				{Ref: ref, Symbols: []cfg.SymbolID{sym}},
			}),
			declaredReturns: map[summary.FuncRef][]typ.Type{},
		},
		reader: summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{
			ref: {Returns: []product.AbstractValue{product.FromType(typ.Number)}},
		}),
		baseSignature: func(summary.FuncRef) *typ.Function {
			t.Fatal("static signature should be blocked by live top FunctionRefs")
			return nil
		},
	}

	if got := projector.TypeAt(state, path); got != nil {
		t.Fatalf("TypeAt with live top = %v, want nil", got)
	}
}

func TestCallableProjectorClosureRefsDominateFunctionRefs(t *testing.T) {
	t.Parallel()

	direct := summary.FuncRef{GraphID: 1}
	closureRef := summary.FuncRef{GraphID: 2}
	path := constraint.NewPath(cfg.SymbolID(12), "fn")
	closure := flow.ClosureRefOf(canonref.ToFlow(closureRef), flow.CaptureCellsDomain.Bottom(), nil)
	state := flow.PointState{
		FunctionRefs: flow.WithFunctionRef(nil, path.Key(), flow.FunctionRefSetOf(canonref.ToFlow(direct))),
		ClosureRefs:  flow.WithClosureRef(nil, path.Key(), flow.ClosureRefSetOf(closure)),
	}
	projector := callableProjector{
		prog: &program{declaredReturns: map[summary.FuncRef][]typ.Type{}},
		reader: summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{
			closureRef: {Returns: []product.AbstractValue{product.FromType(typ.String)}},
		}),
		baseSignature: func(got summary.FuncRef) *typ.Function {
			if got == direct {
				t.Fatal("direct function ref used despite finite closure ref")
			}
			return typ.Func().Build()
		},
	}

	fn := unwrap.Function(projector.TypeAt(state, path))
	if fn == nil || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want closure summary [string]", fn.Returns)
	}
}
