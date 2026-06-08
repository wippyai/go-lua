package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFlowEvidenceIsZero(t *testing.T) {
	if !(FlowEvidence{}).IsZero() {
		t.Fatal("empty evidence should be zero")
	}
	if (FlowEvidence{Calls: []CallEvidence{{Point: cfg.Point(1)}}}).IsZero() {
		t.Fatal("call evidence should make product non-zero")
	}
	if (FlowEvidence{ParameterUses: []ParameterUseEvidence{{Symbol: cfg.SymbolID(1), Whole: true}}}).IsZero() {
		t.Fatal("parameter-use evidence should make product non-zero")
	}
}

func TestCallContractEvidenceIsSolvedResultCarrier(t *testing.T) {
	args := []callobligation.Obligation{callobligation.Body(typ.String)}
	contracts := NewCallContractEvidence(args)
	args[0] = callobligation.Body(typ.Number)
	if got := contracts.ArgType(0); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ArgType(0) = %v, want string", got)
	}
	if got := contracts.ArgObligation(0); got.Source != callobligation.SourceBody {
		t.Fatalf("ArgObligation(0).Source = %v, want body", got.Source)
	}
	if got := contracts.ArgType(-1); got != nil {
		t.Fatalf("ArgType(-1) = %v, want nil", got)
	}
	if got := contracts.ArgType(1); got != nil {
		t.Fatalf("ArgType(1) = %v, want nil", got)
	}
}

func TestFuncResultSolvedFlowUsesProjectionCarrier(t *testing.T) {
	result := &FuncResult{FlowProjection: mockSolvedFlow{excludes: true}}
	path := constraint.NewPath(cfg.SymbolID(7), "x")
	if got := result.NarrowedTypeAt(cfg.Point(1), path); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("NarrowedTypeAt via FlowProjection = %v, want string", got)
	}
	if got := result.PreStateTypeAt(cfg.Point(1), path); !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("PreStateTypeAt via FlowProjection = %v, want number", got)
	}
	if result.SolvedFlow() == nil {
		t.Fatal("SolvedFlow should expose FlowProjection")
	}
	if !result.ExcludesTypeAt(cfg.Point(1), path, typ.Number) {
		t.Fatal("ExcludesTypeAt should dispatch through FlowProjection")
	}
}

func TestFuncResultFactSurfaceAccessorsPreferFactsAndFallbackToInputs(t *testing.T) {
	const sym = cfg.SymbolID(7)
	result := &FuncResult{
		Facts: resultSurfaceFacts{},
		FlowInputs: &flow.Inputs{
			ConstValues: map[cfg.SymbolID]map[cfg.Point]*flow.ConstValue{
				sym: {cfg.Point(3): {Kind: flow.ConstString, Str: "input"}},
			},
		},
	}

	if cond := result.ConditionProofFacts().ConditionAt(cfg.Point(1)); !cond.IsFalse() {
		t.Fatalf("ConditionProofFacts did not come from Facts: %v", cond)
	}
	if got := result.ConstFacts().ConstValueAtSym(cfg.Point(1), sym); got == nil || got.Str != "facts" {
		t.Fatalf("ConstFacts did not prefer Facts: %#v", got)
	}
	obs := result.PathObservationFacts().ObservePath(flow.PathObservationQuery{
		Point: cfg.Point(1),
		Path:  constraint.NewPath(sym, "value"),
	})
	if !obs.Resolved() || !typ.TypeEquals(obs.Type, typ.String) {
		t.Fatalf("PathObservationFacts did not come from Facts: %#v", obs)
	}
	children := result.PathChildFacts().ObserveChildPaths(flow.PathChildQuery{
		Point: cfg.Point(1),
		Path:  constraint.NewPath(sym, "value"),
	})
	if len(children) != 1 || !typ.TypeEquals(children[0].Type, typ.Number) {
		t.Fatalf("PathChildFacts did not come from Facts: %#v", children)
	}
	if got := result.TransferValueFacts().AssignedValueTypeAt(cfg.Point(1), constraint.NewPath(sym, "value"), typ.String, flow.AssignmentSource{}); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("TransferValueFacts did not come from Facts: %v", got)
	}

	fallback := (&FuncResult{FlowInputs: result.FlowInputs}).ConstFacts()
	if got := fallback.ConstValueAtSym(cfg.Point(3), sym); got == nil || got.Str != "input" {
		t.Fatalf("ConstFacts did not fallback to FlowInputs: %#v", got)
	}

	flowFallback := (&FuncResult{FlowProjection: mockSolvedFlow{}}).PathChildFacts()
	if got := flowFallback.ObserveChildPaths(flow.PathChildQuery{Point: cfg.Point(2), Path: constraint.NewPath(sym, "value")}); len(got) != 1 || !typ.TypeEquals(got[0].Type, typ.Boolean) {
		t.Fatalf("PathChildFacts did not fallback to solved flow projection: %#v", got)
	}
	if got := (&FuncResult{FlowProjection: mockSolvedFlow{}}).TransferValueFacts().AssignedValueTypeAt(cfg.Point(2), constraint.NewPath(sym, "value"), typ.String, flow.AssignmentSource{}); !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("TransferValueFacts did not fallback to solved flow projection: %v", got)
	}
}

func TestFuncAnalysisViewFactSurfaceAccessorsMirrorResult(t *testing.T) {
	view := ViewFromResult(&FuncResult{Facts: resultSurfaceFacts{}})

	if cond := view.ConditionProofFacts().ConditionAt(cfg.Point(1)); !cond.IsFalse() {
		t.Fatalf("view ConditionProofFacts did not come from Facts: %v", cond)
	}
	if got := view.ConstFacts().ConstValueAtSym(cfg.Point(1), cfg.SymbolID(7)); got == nil || got.Str != "facts" {
		t.Fatalf("view ConstFacts did not come from Facts: %#v", got)
	}
	obs := view.PathObservationFacts().ObservePath(flow.PathObservationQuery{
		Point: cfg.Point(1),
		Path:  constraint.NewPath(cfg.SymbolID(7), "value"),
	})
	if !obs.Resolved() || !typ.TypeEquals(obs.Type, typ.String) {
		t.Fatalf("view PathObservationFacts did not come from Facts: %#v", obs)
	}
	children := view.PathChildFacts().ObserveChildPaths(flow.PathChildQuery{
		Point: cfg.Point(1),
		Path:  constraint.NewPath(cfg.SymbolID(7), "value"),
	})
	if len(children) != 1 || !typ.TypeEquals(children[0].Type, typ.Number) {
		t.Fatalf("view PathChildFacts did not come from Facts: %#v", children)
	}
	if got := view.TransferValueFacts().AssignedValueTypeAt(cfg.Point(1), constraint.NewPath(cfg.SymbolID(7), "value"), typ.String, flow.AssignmentSource{}); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("view TransferValueFacts did not come from Facts: %v", got)
	}
}

func TestFuncResultGlobalTypeOverlayPrefersCarrierAndClones(t *testing.T) {
	result := &FuncResult{
		GlobalTypes: map[string]typ.Type{"print": typ.String},
		GlobalTypeBindings: globalenv.TypeOverlay{
			{Name: globalenv.Name("print"), Type: typ.Number},
		},
	}
	overlay := result.GlobalTypeOverlay()
	if got, ok := overlay.Type("print"); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("GlobalTypeOverlay(print) = %v/%v, want number/true", got, ok)
	}
	overlay[0].Type = typ.Boolean
	if got, ok := result.GlobalTypeOverlay().Type("print"); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("mutating returned overlay changed result: %v/%v", got, ok)
	}

	view := ViewFromResult(result)
	viewOverlay := view.GlobalTypeOverlay()
	if got, ok := viewOverlay.Type("print"); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("view GlobalTypeOverlay(print) = %v/%v, want number/true", got, ok)
	}
	viewOverlay[0].Type = typ.String
	if got, ok := view.GlobalTypeOverlay().Type("print"); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("mutating returned view overlay changed view: %v/%v", got, ok)
	}
}

func TestFuncResultGlobalTypeOverlayNormalizesExternalMap(t *testing.T) {
	result := &FuncResult{GlobalTypes: map[string]typ.Type{"print": typ.String}}
	if got, ok := result.GlobalTypeOverlay().Type("print"); !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("GlobalTypeOverlay external map projection = %v/%v, want string/true", got, ok)
	}
}

func TestFuncResultLiteralSignatureLookupPrefersProvider(t *testing.T) {
	fn := &ast.FunctionExpr{}
	stale := typ.Func().Returns(typ.String).Build()
	want := typ.Func().Returns(typ.Number).Build()
	result := &FuncResult{
		LiteralSignatures: map[*ast.FunctionExpr]*typ.Function{fn: stale},
		LiteralSignatureProvider: LiteralSigsLookup{
			fn: want,
		},
	}
	if got := result.LiteralSignatureLookup().Lookup(fn); got != want {
		t.Fatalf("LiteralSignatureLookup() = %v, want provider signature", got)
	}
	view := ViewFromResult(result)
	if got := view.LiteralSignatureLookup().Lookup(fn); got != want {
		t.Fatalf("view LiteralSignatureLookup() = %v, want provider signature", got)
	}
}

func TestFuncResultLiteralSignatureLookupNormalizesExternalMap(t *testing.T) {
	fn := &ast.FunctionExpr{}
	want := typ.Func().Returns(typ.String).Build()
	result := &FuncResult{LiteralSignatures: map[*ast.FunctionExpr]*typ.Function{fn: want}}
	if got := result.LiteralSignatureLookup().Lookup(fn); got != want {
		t.Fatalf("LiteralSignatureLookup external map projection = %v, want map signature", got)
	}
}

func TestObservationStateNormalizesSolvedResultSurfaces(t *testing.T) {
	fn := &ast.FunctionExpr{}
	literal := typ.Func().Returns(typ.String).Build()
	flowProjection := mockSolvedFlow{}
	resolve := func(ast.TypeExpr, *scope.State) typ.Type { return typ.Boolean }
	result := &FuncResult{
		Facts:          resultSurfaceFacts{},
		FlowProjection: flowProjection,
		NarrowSynth:    observationStateSynth{mockSynth: &mockSynth{}, resolve: resolve},
		GlobalTypeBindings: globalenv.TypeOverlay{
			{Name: globalenv.Name("decode"), Type: typ.Number},
		},
		LiteralSignatureProvider: LiteralSigsLookup{fn: literal},
	}

	state := result.ObservationState()
	if state.Flow != flowProjection {
		t.Fatalf("ObservationState.Flow = %#v, want solved flow projection", state.Flow)
	}
	if state.Facts != result.Facts {
		t.Fatal("ObservationState.Facts did not preserve result facts")
	}
	if got := state.LiteralSignatureProvider.Lookup(fn); got != literal {
		t.Fatalf("ObservationState literal signature = %v, want %v", got, literal)
	}
	if got, ok := state.GlobalTypeOverlay.Type("decode"); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("ObservationState overlay = %v/%v, want number/true", got, ok)
	}
	state.GlobalTypeOverlay[0].Type = typ.String
	if got, ok := result.GlobalTypeOverlay().Type("decode"); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("mutating ObservationState overlay changed result: %v/%v", got, ok)
	}
	if state.ResolveType == nil || !typ.TypeEquals(state.ResolveType(nil, nil), typ.Boolean) {
		t.Fatalf("ObservationState resolver missing or wrong")
	}

	viewState := ViewFromResult(result).ObservationState()
	if viewState.Flow != flowProjection {
		t.Fatalf("view ObservationState.Flow = %#v, want solved flow projection", viewState.Flow)
	}
	if got := viewState.LiteralSignatureProvider.Lookup(fn); got != literal {
		t.Fatalf("view ObservationState literal signature = %v, want %v", got, literal)
	}
	if got, ok := viewState.GlobalTypeOverlay.Type("decode"); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("view ObservationState overlay = %v/%v, want number/true", got, ok)
	}
}

type observationStateSynth struct {
	*mockSynth
	resolve func(ast.TypeExpr, *scope.State) typ.Type
}

func (s observationStateSynth) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if s.resolve == nil {
		return nil
	}
	return s.resolve(expr, sc)
}

type mockSolvedFlow struct {
	excludes bool
}

func (mockSolvedFlow) NarrowedTypeAt(cfg.Point, constraint.Path) typ.Type { return typ.String }

func (mockSolvedFlow) NarrowedTypeAtWithCondition(cfg.Point, constraint.Path, constraint.Condition) typ.Type {
	return typ.String
}

func (mockSolvedFlow) PreStateTypeAt(cfg.Point, constraint.Path) typ.Type { return typ.Number }

func (m mockSolvedFlow) ExcludesTypeAt(cfg.Point, constraint.Path, typ.Type) bool { return m.excludes }

func (mockSolvedFlow) NumericBoundsAt(cfg.Point, cfg.SymbolID) (int64, int64, bool) {
	return 0, 0, false
}

func (mockSolvedFlow) ArrayLenRefPathAt(cfg.Point, cfg.SymbolID) (constraint.Path, int64, bool) {
	return constraint.Path{}, 0, false
}

func (mockSolvedFlow) LengthBoundsAt(cfg.Point, constraint.Path) (int64, int64, bool) {
	return 0, 0, false
}

func (mockSolvedFlow) IsPointDead(cfg.Point) bool { return false }

func (mockSolvedFlow) HasKeyOf(cfg.Point, constraint.Path, constraint.Path) bool { return false }

func (mockSolvedFlow) IndexReadPointFacts(cfg.Point, flow.PathReadView) flow.PointFacts {
	return flow.PointFactsOf(flow.PointState{})
}

func (mockSolvedFlow) ObserveChildPaths(q flow.PathChildQuery) []flow.PathFact {
	return []flow.PathFact{{Path: q.Path.Field("child"), Type: typ.Boolean}}
}

func (mockSolvedFlow) AssignedValueTypeAt(cfg.Point, constraint.Path, typ.Type, flow.AssignmentSource) typ.Type {
	return typ.Boolean
}

func (mockSolvedFlow) MutatorValueTypeAt(cfg.Point, constraint.Path, typ.Type, flow.ValueTemplate) typ.Type {
	return typ.Boolean
}

func (mockSolvedFlow) MutatorKeyTypeAt(cfg.Point, constraint.Path, typ.Type) typ.Type {
	return typ.String
}

type resultSurfaceFacts struct{}

func (resultSurfaceFacts) DeclaredAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (resultSurfaceFacts) RefinedAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

func (resultSurfaceFacts) EffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (resultSurfaceFacts) IsAnnotated(cfg.SymbolID) bool { return false }

func (resultSurfaceFacts) ConditionAt(cfg.Point) constraint.Condition {
	return constraint.FalseCondition()
}

func (resultSurfaceFacts) ProvesTypeAt(cfg.Point, constraint.Path, typ.Type) bool {
	return false
}

func (resultSurfaceFacts) ConditionTypeAt(cfg.Point, constraint.Path) typ.Type {
	return nil
}

func (resultSurfaceFacts) ConditionedTypeAt(cfg.Point, constraint.Path, constraint.Condition) typ.Type {
	return nil
}

func (resultSurfaceFacts) ConditionedSeedTypeAt(cfg.Point, constraint.Path, typ.Type, constraint.Path, constraint.Condition) typ.Type {
	return nil
}

func (resultSurfaceFacts) ConstValueAtSym(cfg.Point, cfg.SymbolID) *flow.ConstValue {
	return &flow.ConstValue{Kind: flow.ConstString, Str: "facts"}
}

func (resultSurfaceFacts) ObservePath(flow.PathObservationQuery) flow.PathObservation {
	return flow.PathObservation{
		Type:   typ.String,
		State:  flow.StateResolved,
		Source: flow.PathObservationFactProjection,
	}
}

func (resultSurfaceFacts) ObserveChildPaths(q flow.PathChildQuery) []flow.PathFact {
	return []flow.PathFact{{Path: q.Path.Field("child"), Type: typ.Number}}
}

func (resultSurfaceFacts) AssignedValueTypeAt(cfg.Point, constraint.Path, typ.Type, flow.AssignmentSource) typ.Type {
	return typ.Integer
}

func (resultSurfaceFacts) MutatorValueTypeAt(cfg.Point, constraint.Path, typ.Type, flow.ValueTemplate) typ.Type {
	return typ.Integer
}

func (resultSurfaceFacts) MutatorKeyTypeAt(cfg.Point, constraint.Path, typ.Type) typ.Type {
	return typ.String
}
