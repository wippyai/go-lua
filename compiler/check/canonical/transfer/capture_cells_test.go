package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCaptureCellReadUsesCells(t *testing.T) {
	tr, ident, sym := captureCellTestTransfer(t)
	out := flow.PointState{
		Env:   map[flow.ValueKey]product.AbstractValue{flow.SymbolValueKey(sym): product.FromType(typ.String)},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
	}

	got, ok := tr.evalExpr(&out, ident, nil)
	if !ok {
		t.Fatal("captured ident did not resolve")
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("captured ident = %v, want number from Cells", got.ProjectValue())
	}
}

func TestClonePointStatePreservesPrototypeSelf(t *testing.T) {
	proto := cfg.SymbolID(42)
	self := product.FromType(typ.NewRecord().Field("node_id", typ.String).Build())
	in := flow.PointState{
		PrototypeSelf: flow.PrototypeSelfOf([]flow.PrototypeSelfEntry{{
			Prototype: proto,
			Value:     self,
		}}),
	}

	got := flow.ClonePointState(in)
	gotSelf, ok := got.PrototypeSelf.Value(proto)
	if !ok {
		t.Fatal("clone dropped PrototypeSelf entry")
	}
	if !product.Domain.Equal(gotSelf, self) {
		t.Fatalf("clone PrototypeSelf = %v, want %v", gotSelf.ProjectValue(), self.ProjectValue())
	}
}

func TestSetMetatablePrototypeFallsBackToMetatableSymbol(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	mtSym := cfg.SymbolID(100)
	protoSym := cfg.SymbolID(200)
	mtIdent := &ast.IdentExpr{Value: "mt"}
	in.Graph.Bindings().Bind(mtIdent, mtSym)
	in.Graph.Bindings().SetName(mtSym, "mt")

	tr := New(in, Config{
		MetatableIndexes: []metatable.Index{{
			MetatableSym: mtSym,
			PrototypeSym: protoSym,
		}},
	})

	got, ok := tr.setMetatablePrototype(cfg.Point(999), &ast.FuncCallExpr{
		Args: []ast.Expr{&ast.TableExpr{}, mtIdent},
	})
	if !ok || got != protoSym {
		t.Fatalf("setmetatable prototype = %d/%v, want %d", got, ok, protoSym)
	}
}

func TestCaptureEntrySeedsCellsAtFunctionEntry(t *testing.T) {
	tr, ident, sym := captureCellTestTransfer(t)

	out := tr.Transfer(tr.in.Graph, tr.in.Graph.Entry(), flow.PointState{
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.Number)}}),
	}, nil, nil)
	got, ok := tr.evalExpr(&out, ident, nil)
	if !ok {
		t.Fatal("captured ident did not resolve after entry seed")
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("captured ident = %v, want entry-seeded number", got.ProjectValue())
	}
}

func TestCapturedIdentAssignmentWritesCellNotEnv(t *testing.T) {
	tr, _, sym := captureCellTestTransfer(t)
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Name: "cap", Symbol: sym}},
		Sources: []ast.Expr{&ast.StringExpr{Value: "ok"}},
	}, nil)

	if _, ok := out.Env[flow.SymbolValueKey(sym)]; ok {
		t.Fatalf("captured assignment wrote Env[%s], want only Cells", flow.SymbolValueKey(sym))
	}
	got, ok := out.Cells.Value(sym)
	if !ok {
		t.Fatal("captured assignment did not write cell")
	}
	if !typ.TypeEquals(got.ProjectValue(), typ.LiteralString("ok")) {
		t.Fatalf("captured cell = %v, want literal string", got.ProjectValue())
	}
	effects := out.CellEffects.Entries()
	if len(effects) != 1 || effects[0].Symbol != sym || !effects[0].MustWrite {
		t.Fatalf("captured assignment effects = %s, want one must-write", out.CellEffects.Format())
	}
}

func TestCapturedAssignmentFromPendingParamDoesNotTopEffect(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"opts"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one param")
	}
	tr := New(in, Config{})
	paramSym := in.Scope.ParamSymbols[0]
	capturedSym := cfg.SymbolID(9002)
	opts := &ast.IdentExpr{Value: "opts"}
	captured := &ast.IdentExpr{Value: "captured"}
	in.Graph.Bindings().Bind(opts, paramSym)
	in.Graph.Bindings().Bind(captured, capturedSym)
	in.Graph.Bindings().SetName(capturedSym, "captured")

	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Name: "captured", Symbol: capturedSym}},
		Sources: []ast.Expr{opts},
	}, nil)
	if got, ok := out.Cells.Value(capturedSym); ok {
		t.Fatalf("pending param write polluted captured cell with %v", got.ProjectValue())
	}
	if effects := out.CellEffects.Entries(); len(effects) != 0 {
		t.Fatalf("pending param write emitted effects: %s", out.CellEffects.Format())
	}

	precise := product.FromType(typ.NewRecord().
		Field("retry", typ.NewRecord().
			Field("max_attempts", typ.Number).
			Build()).
		Build())
	out = flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.SeedEntryValues(&out, map[int]product.AbstractValue{0: precise})
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Name: "captured", Symbol: capturedSym}},
		Sources: []ast.Expr{opts},
	}, nil)
	got, ok := out.Cells.Value(capturedSym)
	if !ok {
		t.Fatal("seeded param write did not update captured cell")
	}
	if !product.Domain.Equal(got, precise) {
		t.Fatalf("captured cell = %v, want seeded param value %v", got.ProjectValue(), precise.ProjectValue())
	}
	effects := out.CellEffects.Entries()
	if len(effects) != 1 || effects[0].Symbol != capturedSym || !effects[0].MustWrite ||
		!product.Domain.Equal(effects[0].Value, precise) {
		t.Fatalf("seeded param effects = %s, want precise must-write", out.CellEffects.Format())
	}
}

func TestOwnerCellBackedParamSeedsCellWithoutEffect(t *testing.T) {
	tr, ident, sym := ownerCellParamTestTransfer(t)
	out := tr.Transfer(tr.in.Graph, tr.in.Graph.Entry(), flow.PointState{}, paramevidence.Contracts{
		0: paramevidence.DemandFromType(typ.String),
	}, nil)

	if _, ok := out.Env[flow.SymbolValueKey(sym)]; ok {
		t.Fatalf("owner cell-backed param seeded Env[%s], want only Cells", flow.SymbolValueKey(sym))
	}
	got, ok := out.Cells.Value(sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("owner cell-backed param cell = %v/%v, want string", got.ProjectValue(), ok)
	}
	if effects := out.CellEffects.Entries(); len(effects) != 0 {
		t.Fatalf("owner entry seed emitted cell effects: %s", out.CellEffects.Format())
	}
	read, ok := tr.evalExpr(&out, ident, nil)
	if !ok || !typ.TypeEquals(read.ProjectValue(), typ.String) {
		t.Fatalf("owner cell-backed read = %v/%v, want string", read.ProjectValue(), ok)
	}
}

func TestOwnerCellBackedAssignmentWritesCellWithoutEffect(t *testing.T) {
	tr, _, sym := ownerCellParamTestTransfer(t)
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Name: "cap", Symbol: sym}},
		Sources: []ast.Expr{&ast.StringExpr{Value: "ok"}},
	}, nil)

	if _, ok := out.Env[flow.SymbolValueKey(sym)]; ok {
		t.Fatalf("owner cell-backed assignment wrote Env[%s], want only Cells", flow.SymbolValueKey(sym))
	}
	got, ok := out.Cells.Value(sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.LiteralString("ok")) {
		t.Fatalf("owner cell-backed assignment cell = %v/%v, want literal string", got.ProjectValue(), ok)
	}
	if effects := out.CellEffects.Entries(); len(effects) != 0 {
		t.Fatalf("owner cell-backed assignment emitted caller effects: %s", out.CellEffects.Format())
	}
}

func TestCapturedFieldWriteUpdatesCellThroughEntryBase(t *testing.T) {
	tr, _, sym := captureCellTestTransfer(t)
	out := flow.PointState{
		Env:   map[flow.ValueKey]product.AbstractValue{},
		Cells: flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: sym, Value: product.FromType(typ.NewRecord().Build())}}),
	}
	tr.applyContainerWrite(&out, cfg.AssignTarget{
		Kind:       cfg.TargetField,
		BaseName:   "cap",
		BaseSymbol: sym,
		FieldPath:  []string{"value"},
	}, &ast.StringExpr{Value: "stored"}, nil)

	if _, ok := out.Env[flow.SymbolValueKey(sym)]; ok {
		t.Fatalf("captured field write wrote Env[%s], want only Cells", flow.SymbolValueKey(sym))
	}
	got, ok := out.Cells.Value(sym)
	if !ok {
		t.Fatal("captured field write did not write cell")
	}
	field, ok := product.FieldOf(got, "value")
	if !ok || field.IsZero() {
		t.Fatalf("captured field missing from cell: %v", got.ProjectValue())
	}
	if !typ.TypeEquals(field.ProjectValue(), typ.LiteralString("stored")) {
		t.Fatalf("captured field = %v, want literal string", field.ProjectValue())
	}
	effects := out.CellEffects.Entries()
	if len(effects) != 1 || effects[0].Symbol != sym || !effects[0].MustWrite {
		t.Fatalf("captured field effects = %s, want one must-write", out.CellEffects.Format())
	}
}

func TestStatementCallAppliesCalleeCellEffects(t *testing.T) {
	effects := flow.CaptureMustWrite(cfg.SymbolID(55), product.FromType(typ.String))
	tr := New(input.Inputs{}, Config{CallTyper: captureEffectTyper{effects: effects}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	dead := tr.applyCallArgs(&out, 0, &cfg.CallInfo{
		Call: &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "callee"}},
	}, nil)
	if dead {
		t.Fatal("test call unexpectedly marked dead")
	}

	got, ok := out.Cells.Value(cfg.SymbolID(55))
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("cell after call = %v/%v, want string", got.ProjectValue(), ok)
	}
	if !flow.CaptureEffectsDomain.Equal(out.CellEffects, effects) {
		t.Fatalf("function effects after call = %s, want %s", out.CellEffects.Format(), effects.Format())
	}
}

func TestStatementCallCellEffectsUseProductArgEvidence(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"arg"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one parameter")
	}
	arg := &ast.IdentExpr{Value: "arg"}
	in.Graph.Bindings().Bind(arg, in.Scope.ParamSymbols[0])
	in.Graph.Bindings().SetName(in.Scope.ParamSymbols[0], "arg")

	effects := flow.CaptureMustWrite(cfg.SymbolID(57), product.FromType(typ.Boolean))
	typer := &productCaptureEffectTyper{captureEffectTyper: captureEffectTyper{effects: effects}}
	tr := New(in, Config{CallTyper: typer})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	dead := tr.applyCallArgs(&out, 0, &cfg.CallInfo{
		Call: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "callee"},
			Args: []ast.Expr{arg},
		},
	}, nil)
	if dead {
		t.Fatal("test call unexpectedly marked dead")
	}
	if len(typer.args) != 1 || typer.args[0].IsZero() || !typer.args[0].IsGradualTop() {
		t.Fatalf("product cell-effect args = %#v, want one gradual-top product value", typer.args)
	}
	got, ok := out.Cells.Value(cfg.SymbolID(57))
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Boolean) {
		t.Fatalf("cell after product-effect call = %v/%v, want boolean", got.ProjectValue(), ok)
	}
	if !flow.CaptureEffectsDomain.Equal(out.CellEffects, effects) {
		t.Fatalf("function effects after product-effect call = %s, want %s", out.CellEffects.Format(), effects.Format())
	}
}

func TestExpressionCallAppliesCalleeCellEffectsWithoutReturns(t *testing.T) {
	effects := flow.CaptureMustWrite(cfg.SymbolID(56), product.FromType(typ.Number))
	tr := New(input.Inputs{}, Config{CallTyper: captureEffectTyper{effects: effects}})
	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}

	if _, ok := tr.evalCall(&out, &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "callee"}}, nil); ok {
		t.Fatal("test call unexpectedly returned values")
	}
	got, ok := out.Cells.Value(cfg.SymbolID(56))
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("cell after expression call = %v/%v, want number", got.ProjectValue(), ok)
	}
}

func TestClosureCallCellEffectsUpdateStoredClosureEnvironment(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	calleeSym := cfg.SymbolID(701)
	callee := &ast.IdentExpr{Value: "fn"}
	in.Graph.Bindings().Bind(callee, calleeSym)
	in.Graph.Bindings().SetName(calleeSym, "fn")

	cellSym := cfg.SymbolID(702)
	effects := flow.CaptureMustWrite(cellSym, product.FromType(typ.String))
	tr := New(in, Config{CallTyper: &productCaptureEffectTyper{captureEffectTyper: captureEffectTyper{effects: effects}}})
	path := constraint.NewPath(calleeSym, "fn").Key()
	closure := flow.ClosureRefOf(
		flow.FunctionRef{GraphID: 703},
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: cellSym, Value: product.FromType(typ.Number)}}),
		nil,
	)
	out := flow.PointState{
		Env:         map[flow.ValueKey]product.AbstractValue{},
		ClosureRefs: flow.WithClosureRef(nil, path, flow.ClosureRefSetOf(closure)),
	}

	if _, ok := tr.evalCall(&out, &ast.FuncCallExpr{Func: callee}, nil); !ok {
		t.Fatal("test call did not resolve")
	}
	if _, ok := out.Cells.Value(cellSym); ok {
		t.Fatalf("escaped closure effect polluted caller cells: %s", out.Cells.Format())
	}
	if !flow.CaptureEffectsDomain.Equal(out.CellEffects, effects) {
		t.Fatalf("escaped closure effect was not recorded for summary: %s, want %s", out.CellEffects.Format(), effects.Format())
	}
	refs, ok := flow.ClosureRefAt(out.ClosureRefs, path)
	if !ok {
		t.Fatalf("closure refs missing after call: %#v", out.ClosureRefs)
	}
	got, singleton := refs.Singleton()
	if !singleton {
		t.Fatalf("closure refs after call = %s, want singleton", refs.Format())
	}
	if av, ok := got.EntryCells().Value(cellSym); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("closure env after call = %v/%v, want string", av.ProjectValue(), ok)
	}
}

func captureCellTestTransfer(t *testing.T) (*Transfer, *ast.IdentExpr, cfg.SymbolID) {
	t.Helper()
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || in.Graph.Bindings() == nil {
		t.Fatal("test graph not built")
	}
	sym := cfg.SymbolID(9001)
	ident := &ast.IdentExpr{Value: "cap"}
	in.Graph.Bindings().Bind(ident, sym)
	in.Graph.Bindings().SetName(sym, "cap")
	return New(in, Config{}), ident, sym
}

func ownerCellParamTestTransfer(t *testing.T) (*Transfer, *ast.IdentExpr, cfg.SymbolID) {
	t.Helper()
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"cap"}}}
	in := input.BuildFromFunction(fn, nil, nil)
	if in.Graph == nil || len(in.Scope.ParamSymbols) != 1 {
		t.Fatal("test graph did not build one param")
	}
	sym := in.Scope.ParamSymbols[0]
	in.Scope.CellSymbols = []cfg.SymbolID{sym}
	ident := &ast.IdentExpr{Value: "cap"}
	in.Graph.Bindings().Bind(ident, sym)
	return New(in, Config{}), ident, sym
}

type captureEffectTyper struct {
	effects flow.CaptureEffects
}

type numberReturnTyper struct{}

func (numberReturnTyper) CallReturns(*ast.FuncCallExpr, []typ.Type, func(ast.Expr) typ.Type, flow.CaptureCells, flow.FunctionRefs) ([]typ.Type, bool) {
	return []typ.Type{typ.Number}, true
}

var _ typeCallReturnProvider = numberReturnTyper{}

func (c captureEffectTyper) IterVars(*ast.FuncCallExpr, int, func(ast.Expr) typ.Type) ([]typ.Type, bool) {
	return nil, false
}

func (c captureEffectTyper) KeyedIterSource(*ast.FuncCallExpr) (ast.Expr, bool) {
	return nil, false
}

func (c captureEffectTyper) IndexedIterSource(*ast.FuncCallExpr) (constraint.Path, bool) {
	return constraint.Path{}, false
}

func (c captureEffectTyper) KeysCollectorContainer(*cfg.CallInfo, int) (constraint.Path, bool) {
	return constraint.Path{}, false
}

func (c captureEffectTyper) ParamNarrows(*ast.FuncCallExpr) []ParamNarrow {
	return nil
}

func (c captureEffectTyper) IsNoReturn(*ast.FuncCallExpr, ProductCallContext) bool {
	return false
}

func (c captureEffectTyper) TypeCastTarget(*ast.FuncCallExpr, func(ast.Expr) typ.Type) (typ.Type, bool) {
	return nil, false
}

func (c captureEffectTyper) ReturnRelationsFromValues(*ast.FuncCallExpr, ProductCallContext) flow.ReturnRelations {
	return flow.ReturnRelationsDomain.Top()
}

func (c captureEffectTyper) CellEffectsFromValues(*ast.FuncCallExpr, ProductCallContext) flow.CaptureEffects {
	return c.effects
}

var _ productCellEffectProvider = captureEffectTyper{}

type productCaptureEffectTyper struct {
	captureEffectTyper
	numberReturnTyper
	args []product.AbstractValue
}

func (p *productCaptureEffectTyper) CellEffectsFromValues(
	_ *ast.FuncCallExpr,
	ctx ProductCallContext,
) flow.CaptureEffects {
	p.args = append([]product.AbstractValue(nil), ctx.ArgValues...)
	return p.effects
}

var _ productCellEffectProvider = (*productCaptureEffectTyper)(nil)
var _ typeCallReturnProvider = (*productCaptureEffectTyper)(nil)
