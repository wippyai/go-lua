package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtractAssignments_NilConfig(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments:        []flow.UnifiedAssignment{},
		IndexerAssignments: []flow.IndexerAssignment{},
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
		PredicateLinks:     make(map[string]flow.PredicateLink),
	}
	fc := &core.FlowContext{}
	ExtractAssignments(fc, inputs, nil)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments with nil graph, got %d", len(inputs.Assignments))
	}
}

func TestExtractIterSource_NilArgs(t *testing.T) {
	result := resolve.ExtractIteratorSource(nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil iterExprs, got %v", result)
	}
}

func TestExtractIterSource_EmptyExprs(t *testing.T) {
	result := resolve.ExtractIteratorSource([]ast.Expr{}, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for empty iterExprs, got %v", result)
	}
}

func TestExtractIterSource_WithSynth(t *testing.T) {
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return nil, false
	}
	constResolver := func(name string) *flow.ConstValue {
		return nil
	}
	result := resolve.ExtractIteratorSource(
		[]ast.Expr{&ast.IdentExpr{Value: "test"}},
		0,
		synth,
		symResolver,
		constResolver,
		nil,
	)
	_ = result
}

func TestExtractFuncDefAssignments_NilConfig(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	fc := &core.FlowContext{}
	ExtractFuncDefAssignments(fc, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments with nil graph, got %d", len(inputs.Assignments))
	}
}

func TestExtractCallCorrelations_PassesPointToSymResolver(t *testing.T) {
	callInfo := &cfg.CallInfo{
		CalleeSymbol: 7,
	}
	wantPoint := cfg.Point(99)
	seenPoint := cfg.Point(0)
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		seenPoint = p
		return typ.Integer, true
	}
	_, _ = extractCallCorrelations(callInfo, nil, wantPoint, symResolver)
	if seenPoint != wantPoint {
		t.Fatalf("symResolver point = %d, want %d", seenPoint, wantPoint)
	}
}

func TestExtractCallCorrelations_MethodUsesCanonicalCalleeResolution(t *testing.T) {
	receiverSym := cfg.SymbolID(42)
	callInfo := &cfg.CallInfo{
		Method:         "receive",
		Receiver:       &ast.IdentExpr{Value: "ch"},
		ReceiverSymbol: receiverSym,
	}
	receiverType := typ.NewInterface("Channel", []typ.Method{
		{
			Name: "receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.String, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == receiverSym {
			return receiverType, true
		}
		return nil, false
	}

	inverse, co := extractCallCorrelations(callInfo, nil, 1, symResolver)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one inverse correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected inverse correlation: %+v", inverse[0])
	}
}

func TestIterSourceInfo_ZeroValue(t *testing.T) {
	var info resolve.IteratorSourceInfo
	if info.Path.Root != "" {
		t.Errorf("expected empty Path.Root for zero value")
	}
}

func TestExtractIterSource_WithBindings(t *testing.T) {
	bindings := &bind.BindingTable{}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Unknown
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return nil, false
	}
	constResolver := func(name string) *flow.ConstValue {
		return nil
	}
	result := resolve.ExtractIteratorSource(
		[]ast.Expr{&ast.IdentExpr{Value: "iter"}},
		1,
		synth,
		symResolver,
		constResolver,
		bindings,
	)
	_ = result
}

func TestExtractAssignments_ContainerElementSourceFromTrailingCall(t *testing.T) {
	code := `
		local ch = new_channel()
		local lead, ok, msg = "x", ch:receive()
	`
	chunk, err := parse.ParseString(code, "emit_container_trailing.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "new_channel")
	exit := graph.Exit()
	chSym, ok := graph.SymbolAt(exit, "ch")
	if !ok || chSym == 0 {
		t.Fatal("expected symbol for ch")
	}
	msgSym, ok := graph.SymbolAt(exit, "msg")
	if !ok || msgSym == 0 {
		t.Fatal("expected symbol for msg")
	}

	elemType := typ.NewRecord().Field("id", typ.String).Build()
	receiveSpec := contract.NewSpec().WithEffects(effect.Return{
		ReturnIndex: 1,
		Transform:   effect.ElementOf{Source: effect.ParamRef{Index: 0}},
	})
	channelType := typ.NewInterface("Channel", []typ.Method{
		{
			Name: "receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, elemType).
				Spec(receiveSpec).
				Build(),
		},
	})
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == chSym {
			return channelType, true
		}
		return nil, false
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			SymResolver: symResolver,
		},
	}, inputs, nil)

	var msgAssign *flow.UnifiedAssignment
	for i := range inputs.Assignments {
		assign := &inputs.Assignments[i]
		if assign.TargetPath.Symbol == msgSym {
			msgAssign = assign
			break
		}
	}
	if msgAssign == nil {
		t.Fatal("expected assignment for msg")
	}
	if msgAssign.ContainerElementSource == nil {
		t.Fatal("expected container element source for trailing target from receive()")
	}
	if msgAssign.ContainerElementSource.ContainerPath.Symbol != chSym {
		t.Fatalf("container source symbol = %d, want %d", msgAssign.ContainerElementSource.ContainerPath.Symbol, chSym)
	}
	if msgAssign.ContainerElementSource.ReturnIndex != 1 {
		t.Fatalf("container return index = %d, want 1", msgAssign.ContainerElementSource.ReturnIndex)
	}
}

func TestCorrelationsFromFunctionType_NoImplicitErrorConvention(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.Number)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("expected no implicit correlations without explicit spec effects, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_ExplicitErrorReturn(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{
			ValueIndex: 0,
			ErrorIndex: 1,
		})).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one explicit error correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_ImplicitLuaErrorConvention(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.LuaError)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one convention-based correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected convention correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_ImplicitStringErrorConvention(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.String)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one convention-based correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected convention correlation: %+v", inverse[0])
	}
}
