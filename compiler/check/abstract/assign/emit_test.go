package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type preciseSourceSynthStub struct {
	preciseType typ.Type
}

type expandedValueSynthStub struct {
	values []typ.Type
}

type testGraphProvider struct {
	bindings *bind.BindingTable
	cache    map[*ast.FunctionExpr]*cfg.Graph
}

func TestOverlayTypesAt_ReconcilesInferredSelfWithScopedReceiver(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"self"}}}
	graph := cfg.Build(fn)
	params := graph.ParamSymbols()
	if len(params) != 1 || params[0] == 0 {
		t.Fatal("expected self parameter symbol")
	}
	selfSym := params[0]
	scopeSelf := typ.NewAlias("Builder", typ.NewRecord().
		Field("build", typ.Func().Param("self", typ.Self).Returns(typ.String).Build()).
		OptField("prefix", typ.String).
		Build())
	declared := typ.NewRecord().
		SetOpen(true).
		OptField("prefix", typ.String).
		Build()
	inferred := typ.NewRecord().
		SetOpen(true).
		Field("prefix", typ.String).
		Build()
	state := &assignmentExtractionState{
		fc: &core.FlowContext{
			Graph: graph,
			Base:  scope.New().WithSelf(scopeSelf),
		},
		inputs: &flow.Inputs{
			DeclaredTypes: map[cfg.SymbolID]typ.Type{selfSym: declared},
		},
		inferredTypes: api.SpecTypes{selfSym: inferred},
		paramSet:      map[cfg.SymbolID]bool{selfSym: true},
	}

	got := state.overlayTypesAt(1)[selfSym]
	alias, ok := got.(*typ.Alias)
	if !ok || alias.Name != "Builder" {
		t.Fatalf("overlay self type = %T %v, want Builder alias", got, got)
	}
	target, ok := alias.Target.(*typ.Record)
	if !ok || target.GetField("build") == nil {
		t.Fatalf("overlay self alias lost scoped receiver contract: %v", alias.Target)
	}
	prefix := target.GetField("prefix")
	if prefix == nil || prefix.Optional {
		t.Fatalf("overlay self alias should include present prefix evidence, got %v", alias.Target)
	}
}

func newTestGraphProvider(bindings *bind.BindingTable) *testGraphProvider {
	return &testGraphProvider{
		bindings: bindings,
		cache:    make(map[*ast.FunctionExpr]*cfg.Graph),
	}
}

func (p *testGraphProvider) GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil {
		return nil
	}
	if graph := p.cache[fn]; graph != nil {
		return graph
	}
	var graph *cfg.Graph
	if p.bindings != nil {
		graph = cfg.BuildWithBindings(fn, p.bindings)
	} else {
		graph = cfg.Build(fn)
	}
	p.cache[fn] = graph
	return graph
}

func (p *testGraphProvider) EvidenceForGraph(graph *cfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	return trace.GraphEvidence(graph, graph.Bindings())
}

func (s *preciseSourceSynthStub) TypeOf(expr ast.Expr, _ cfg.Point) typ.Type {
	switch expr.(type) {
	case *ast.LogicalOpExpr:
		return s.preciseType
	default:
		return typ.Unknown
	}
}

func (s *preciseSourceSynthStub) ExpandValues([]ast.Expr, int, cfg.Point) []typ.Type { return nil }

func (s *preciseSourceSynthStub) InferIterVars([]ast.Expr, int, cfg.Point) []typ.Type { return nil }

func (s *preciseSourceSynthStub) ExpandValuesWithSpecTypes([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type {
	return []typ.Type{typ.Any}
}

func (s *preciseSourceSynthStub) InferIterVarsWithSpecTypes([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type {
	return nil
}

func (s *expandedValueSynthStub) TypeOf(ast.Expr, cfg.Point) typ.Type { return typ.Unknown }

func (s *expandedValueSynthStub) ExpandValues([]ast.Expr, int, cfg.Point) []typ.Type {
	return s.values
}

func (s *expandedValueSynthStub) InferIterVars([]ast.Expr, int, cfg.Point) []typ.Type { return nil }

func (s *expandedValueSynthStub) ExpandValuesWithSpecTypes([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type {
	return s.values
}

func (s *expandedValueSynthStub) InferIterVarsWithSpecTypes([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type {
	return nil
}

func TestExtractAssignments_NilConfig(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments:           []flow.UnifiedAssignment{},
		MapMutatorAssignments: []flow.MapMutatorAssignment{},
		SiblingAssignments:    make(map[flow.SiblingKey]*flow.SiblingAssignment),
		PredicateLinks:        make(map[string]flow.PredicateLink),
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

func TestExtractFuncDefAssignments_UsesCanonicalNestedTargetPath(t *testing.T) {
	code := `
		local session = {
			_session_contexts_repo = {},
		}

		function session._session_contexts_repo.list_by_type()
			return {}
		end
	`
	chunk, err := parse.ParseString(code, "emit_nested_funcdef_path.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk})
	evidence := trace.GraphEvidence(graph, graph.Bindings())
	inputs := &flow.Inputs{}

	ExtractFuncDefAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: evidence,
	}, inputs)

	for _, assignment := range inputs.Assignments {
		if assignment.TargetPath.Root != "session" || len(assignment.TargetPath.Segments) != 2 {
			continue
		}
		if assignment.TargetPath.Segments[0].Kind == constraint.SegmentField &&
			assignment.TargetPath.Segments[0].Name == "_session_contexts_repo" &&
			assignment.TargetPath.Segments[1].Kind == constraint.SegmentField &&
			assignment.TargetPath.Segments[1].Name == "list_by_type" {
			return
		}
	}
	t.Fatalf("missing nested function-definition assignment path; got %#v", inputs.Assignments)
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
	_, _, _ = extractCallCorrelations(callInfo, nil, wantPoint, symResolver, nil, nil, nil)
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
				Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
				Build(),
		},
	})

	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == receiverSym {
			return receiverType, true
		}
		return nil, false
	}

	inverse, co, _ := extractCallCorrelations(callInfo, nil, 1, symResolver, nil, nil, nil)
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
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
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
	if msgAssign.Source.Kind != flow.AssignmentSourceContainerElement {
		t.Fatal("expected container element source for trailing target from receive()")
	}
	if msgAssign.Source.ContainerPath.Symbol != chSym {
		t.Fatalf("container source symbol = %d, want %d", msgAssign.Source.ContainerPath.Symbol, chSym)
	}
	if msgAssign.Source.ReturnIndex != 1 {
		t.Fatalf("container return index = %d, want 1", msgAssign.Source.ReturnIndex)
	}
}

func TestExtractAssignments_CallReturnSourceFromMethodReceiver(t *testing.T) {
	code := `
		local test_version = get_version()
		local test_id = test_version:id()
	`
	chunk, err := parse.ParseString(code, "emit_call_return.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "get_version")
	exit := graph.Exit()
	versionSym, ok := graph.SymbolAt(exit, "test_version")
	if !ok || versionSym == 0 {
		t.Fatal("expected symbol for test_version")
	}
	idSym, ok := graph.SymbolAt(exit, "test_id")
	if !ok || idSym == 0 {
		t.Fatal("expected symbol for test_id")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type { return typ.Unknown },
		},
	}, inputs, nil)

	var idAssign *flow.UnifiedAssignment
	for i := range inputs.Assignments {
		assign := &inputs.Assignments[i]
		if assign.TargetPath.Symbol == idSym {
			idAssign = assign
			break
		}
	}
	if idAssign == nil {
		t.Fatal("expected assignment for test_id")
	}
	if idAssign.Source.Kind != flow.AssignmentSourceCallReturn {
		t.Fatalf("source kind = %v, want call return", idAssign.Source.Kind)
	}
	if idAssign.Source.ReceiverPath.Symbol != versionSym {
		t.Fatalf("receiver source symbol = %d, want %d", idAssign.Source.ReceiverPath.Symbol, versionSym)
	}
	if idAssign.Source.Method != "id" {
		t.Fatalf("method = %q, want id", idAssign.Source.Method)
	}
	if idAssign.Source.ReturnIndex != 0 {
		t.Fatalf("return index = %d, want 0", idAssign.Source.ReturnIndex)
	}
}

func TestExtractAssignments_FunctionLiteralUsesCanonicalInputProjection(t *testing.T) {
	code := `
		local group_by_suite = function(entries)
			return {}, {}
		end
	`
	chunk, err := parse.ParseString(code, "emit_function_literal_projection.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk})
	exit := graph.Exit()
	fnSym, ok := graph.SymbolAt(exit, "group_by_suite")
	if !ok || fnSym == 0 {
		t.Fatal("expected symbol for group_by_suite")
	}

	entry := typ.NewRecord().Field("id", typ.String).Build()
	entries := typ.NewArray(entry)
	precise := typ.Func().
		Param("entries", entries).
		Returns(typ.NewMap(typ.Unknown, entries), entries).
		Build()
	stale := typ.Func().
		Param("entries", entries).
		Returns(typ.NewMap(typ.String, typ.NewArray(typ.Unknown)), typ.NewArray(typ.Unknown)).
		Build()

	inputs := &flow.Inputs{
		DeclaredTypes:      map[cfg.SymbolID]typ.Type{fnSym: precise},
		LiteralTypes:       map[cfg.SymbolID]typ.Type{fnSym: precise},
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(expr ast.Expr, _ cfg.Point) typ.Type {
				if _, ok := expr.(*ast.FunctionExpr); ok {
					return stale
				}
				return typ.Unknown
			},
		},
	}, inputs, nil)

	for _, assign := range inputs.Assignments {
		if assign.TargetPath.Symbol != fnSym {
			continue
		}
		if !typ.TypeEquals(assign.Type, precise) {
			t.Fatalf("function literal assignment type = %s, want canonical projection %s", typ.FormatShort(assign.Type), typ.FormatShort(precise))
		}
		return
	}
	t.Fatal("expected assignment for group_by_suite")
}

func TestExtractAssignments_GenericForEmitsIteratorSource(t *testing.T) {
	code := `
		local items: {string} = {"a", "b"}
		for i, item in ipairs(items) do
		end

		local counts: {[string]: number} = {a = 1}
		for key, value in pairs(counts) do
		end
	`
	chunk, err := parse.ParseString(code, "emit_generic_for_iterator_source.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "ipairs", "pairs")
	exit := graph.Exit()
	itemsSym, ok := graph.SymbolAt(exit, "items")
	if !ok || itemsSym == 0 {
		t.Fatal("expected symbol for items")
	}
	countsSym, ok := graph.SymbolAt(exit, "counts")
	if !ok || countsSym == 0 {
		t.Fatal("expected symbol for counts")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type { return typ.Unknown },
		},
	}, inputs, nil)

	assertIteratorAssignment := func(name string, sourceSym cfg.SymbolID, kind flow.IteratorKind, index int) {
		t.Helper()
		for _, assign := range inputs.Assignments {
			if assign.TargetPath.Root != name {
				continue
			}
			if assign.Source.Kind != flow.AssignmentSourceIterator {
				t.Fatalf("%s source kind = %v, want iterator", name, assign.Source.Kind)
			}
			if assign.Source.Path.Symbol != sourceSym {
				t.Fatalf("%s iterator source symbol = %d, want %d", name, assign.Source.Path.Symbol, sourceSym)
			}
			if assign.Source.IteratorKind != kind {
				t.Fatalf("%s iterator kind = %v, want %v", name, assign.Source.IteratorKind, kind)
			}
			if assign.Source.VarIndex != index {
				t.Fatalf("%s var index = %d, want %d", name, assign.Source.VarIndex, index)
			}
			return
		}
		t.Fatalf("missing assignment for %s; assignments=%#v", name, inputs.Assignments)
	}

	assertIteratorAssignment("i", itemsSym, flow.IterateIndexed, 0)
	assertIteratorAssignment("item", itemsSym, flow.IterateIndexed, 1)
	assertIteratorAssignment("key", countsSym, flow.IterateKeyed, 0)
	assertIteratorAssignment("value", countsSym, flow.IterateKeyed, 1)
}

func TestExtractAssignments_SelectResultVariantOriginsFromEffectContract(t *testing.T) {
	code := `
		local events_ch = nil
		local timeout = nil
		local result = channel.select({
			events_ch:case_receive(),
			timeout:case_receive(),
		})
	`
	chunk, err := parse.ParseString(code, "emit_select_variant_origin.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "channel")
	exit := graph.Exit()
	resultSym, ok := graph.SymbolAt(exit, "result")
	if !ok || resultSym == 0 {
		t.Fatal("expected symbol for result")
	}
	eventsSym, ok := graph.SymbolAt(exit, "events_ch")
	if !ok || eventsSym == 0 {
		t.Fatal("expected symbol for events_ch")
	}
	timeoutSym, ok := graph.SymbolAt(exit, "timeout")
	if !ok || timeoutSym == 0 {
		t.Fatal("expected symbol for timeout")
	}
	channelSym, ok := graph.SymbolAt(exit, "channel")
	if !ok || channelSym == 0 {
		t.Fatal("expected symbol for channel")
	}

	selectFunc := typ.Func().
		Param("cases", typ.Any).
		Returns(typ.NewRecord().
			Field(effect.SelectResultChannelField, typ.Any).
			Field(effect.SelectResultValueField, typ.Unknown).
			Field("ok", typ.Boolean).
			Build()).
		Spec(contract.NewSpec().WithEffects(effect.Return{
			ReturnIndex: 0,
			Transform: effect.SelectResultOfCases{
				Cases:   effect.ParamRef{Index: 0},
				Default: effect.ParamRef{Index: -1},
			},
		})).
		Build()
	channelType := typ.NewInterface("channel", []typ.Method{{Name: "select", Type: selectFunc}})

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(expr ast.Expr, _ cfg.Point) typ.Type {
				if attr, ok := expr.(*ast.AttrGetExpr); ok {
					if key, ok := attr.Key.(*ast.StringExpr); ok && key.Value == "select" {
						return selectFunc
					}
				}
				return nil
			},
			SymResolver: func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
				if sym == channelSym {
					return channelType, true
				}
				return nil, false
			},
		},
	}, inputs, nil)

	if !hasVariantOrigin(inputs.VariantFieldOrigins, resultSym, eventsSym, 0) {
		t.Fatalf("missing events_ch select variant origin: %#v", inputs.VariantFieldOrigins)
	}
	if !hasVariantOrigin(inputs.VariantFieldOrigins, resultSym, timeoutSym, 1) {
		t.Fatalf("missing timeout select variant origin: %#v", inputs.VariantFieldOrigins)
	}
}

func hasVariantOrigin(origins []flow.VariantFieldOrigin, targetSym, sourceSym cfg.SymbolID, caseID int64) bool {
	for _, origin := range origins {
		if origin.Target.Symbol == targetSym &&
			origin.Field == effect.SelectResultChannelField &&
			origin.Source.Symbol == sourceSym &&
			origin.DiscriminatorField == effect.SelectResultCaseIDField &&
			typ.TypeEquals(origin.DiscriminatorValue, typ.LiteralInt(caseID)) {
			return true
		}
	}
	return false
}

func TestExtractAssignments_KeysCollectorEffectFallbackIgnoresNonCollectorEffects(t *testing.T) {
	code := `
		local function passthrough(a, b)
			return b
		end
		local t1 = {}
		local t2 = {}
		local keys = passthrough(t1, t2)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_noncollector.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_noncollector")
	exit := graph.Exit()
	keysSym, ok := graph.SymbolAt(exit, "keys")
	if !ok || keysSym == 0 {
		t.Fatal("expected symbol for keys")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			RefinementBySym: func(cfg.SymbolID) *constraint.FunctionRefinement {
				// Non-collector effect (no KeyOf constraint).
				return &constraint.FunctionRefinement{}
			},
		},
	}, inputs, nil)

	if src, ok := inputs.KeysProvenance[keysSym]; ok && src != 0 {
		t.Fatalf("unexpected keys provenance for non-collector effect: keys sym %d -> %d", keysSym, src)
	}
}

func TestExtractAssignments_PrefersPreciseDirectTypeOverExpandedAnyForLogicalOr(t *testing.T) {
	code := `
		local left = nil
		local right = nil
		local ctx = left or right
	`
	chunk, err := parse.ParseString(code, "emit_precise_or.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_precise_or")
	exit := graph.Exit()
	ctxSym, ok := graph.SymbolAt(exit, "ctx")
	if !ok || ctxSym == 0 {
		t.Fatal("expected symbol for ctx")
	}

	contextAlias := typ.NewAlias("Context", typ.NewMap(typ.String, typ.Any))
	synthAPI := &preciseSourceSynthStub{preciseType: contextAlias}
	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		API:      synthAPI,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: synthAPI.TypeOf,
			SymResolver: func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
				return nil, false
			},
		},
	}, inputs, nil)

	for _, assign := range inputs.Assignments {
		if assign.TargetPath.Symbol != ctxSym {
			continue
		}
		if !typ.TypeEquals(assign.Type, contextAlias) {
			t.Fatalf("ctx assignment type = %v, want %v", assign.Type, contextAlias)
		}
		return
	}

	t.Fatal("expected assignment for ctx")
}

func TestExtractAssignments_LocalRHSExpansionBeatsStaleTargetResolver(t *testing.T) {
	code := `
		local existing_summaries, ctx_err = fetch()
	`
	chunk, err := parse.ParseString(code, "emit_local_rhs_expansion.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_local_rhs_expansion")
	exit := graph.Exit()
	summariesSym, ok := graph.SymbolAt(exit, "existing_summaries")
	if !ok || summariesSym == 0 {
		t.Fatal("expected symbol for existing_summaries")
	}

	entry := typ.NewRecord().Field("text", typ.String).Build()
	expanded := typ.NewArray(entry)
	staleTarget := typ.NewRecord().SetOpen(true).Build()
	synthAPI := &expandedValueSynthStub{values: []typ.Type{expanded, typ.Nil}}
	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		API:      synthAPI,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: synthAPI.TypeOf,
			SymResolver: func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
				if sym == summariesSym {
					return staleTarget, true
				}
				return nil, false
			},
		},
	}, inputs, nil)

	for _, assign := range inputs.Assignments {
		if assign.TargetPath.Symbol != summariesSym {
			continue
		}
		if !typ.TypeEquals(assign.Type, expanded) {
			t.Fatalf("existing_summaries assignment type = %v, want %v", assign.Type, expanded)
		}
		return
	}

	t.Fatal("expected assignment for existing_summaries")
}

func TestExtractAssignments_KeysCollectorEffectFallbackRespectsReturnIndex(t *testing.T) {
	code := `
		local function two_returns(tbl)
			return 0, 0
		end
		local t = {}
		local first, second = two_returns(t)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_return_index.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_return_index")
	exit := graph.Exit()
	tSym, ok := graph.SymbolAt(exit, "t")
	if !ok || tSym == 0 {
		t.Fatal("expected symbol for t")
	}
	firstSym, ok := graph.SymbolAt(exit, "first")
	if !ok || firstSym == 0 {
		t.Fatal("expected symbol for first")
	}
	secondSym, ok := graph.SymbolAt(exit, "second")
	if !ok || secondSym == 0 {
		t.Fatal("expected symbol for second")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			RefinementBySym: func(cfg.SymbolID) *constraint.FunctionRefinement {
				return &constraint.FunctionRefinement{
					OnReturn: constraint.FromConstraints(constraint.KeyOf{
						Table: constraint.ParamPath(0),
						Key:   constraint.RetPath(1),
					}),
				}
			},
		},
	}, inputs, nil)

	if src, ok := inputs.KeysProvenance[firstSym]; ok && src != 0 {
		t.Fatalf("unexpected keys provenance for first return target: %d -> %d", firstSym, src)
	}
	src, ok := inputs.KeysProvenance[secondSym]
	if !ok || src != tSym {
		t.Fatalf("expected keys provenance for second target %d -> %d, got %d (present=%v)", secondSym, tSym, src, ok)
	}
}

func TestExtractAssignments_KeysCollectorEffectFallback_TriesAllNameCandidates(t *testing.T) {
	code := `
		local t = {}
		local keys = collect_keys(t)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_name_candidates.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_name_candidates")
	exit := graph.Exit()
	tSym, ok := graph.SymbolAt(exit, "t")
	if !ok || tSym == 0 {
		t.Fatal("expected symbol for t")
	}
	keysSym, ok := graph.SymbolAt(exit, "keys")
	if !ok || keysSym == 0 {
		t.Fatal("expected symbol for keys")
	}

	moduleBindings := bind.NewBindingTable()
	const mismatchSym cfg.SymbolID = 101
	const matchSym cfg.SymbolID = 102
	moduleBindings.SetName(mismatchSym, "collect_keys")
	moduleBindings.SetName(matchSym, "collect_keys")

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:          graph,
		ModuleBindings: moduleBindings,
		Evidence:       trace.GraphEvidence(graph, moduleBindings),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			RefinementBySym: func(sym cfg.SymbolID) *constraint.FunctionRefinement {
				switch sym {
				case mismatchSym:
					return &constraint.FunctionRefinement{
						OnReturn: constraint.FromConstraints(constraint.KeyOf{
							Table: constraint.ParamPath(0),
							Key:   constraint.RetPath(1),
						}),
					}
				case matchSym:
					return &constraint.FunctionRefinement{
						OnReturn: constraint.FromConstraints(constraint.KeyOf{
							Table: constraint.ParamPath(0),
							Key:   constraint.RetPath(0),
						}),
					}
				default:
					return nil
				}
			},
		},
	}, inputs, nil)

	src, ok := inputs.KeysProvenance[keysSym]
	if !ok || src != tSym {
		t.Fatalf("expected keys provenance for target %d -> %d, got %d (present=%v)", keysSym, tSym, src, ok)
	}
}

func TestExtractAssignments_KeysCollector_WithFilterBranch(t *testing.T) {
	code := `
		local function sorted_keys(t)
			local keys = {}
			for k in pairs(t) do
				table.insert(keys, k)
			end
			table.sort(keys)
			return keys
		end

		local function filter_tests(entries, patterns)
			if not patterns or #patterns == 0 then
				return entries
			end
			local filtered = {}
			for _, entry in ipairs(entries) do
				for _, pattern in ipairs(patterns) do
					if entry.id:find(pattern, 1, true) then
						table.insert(filtered, entry)
						break
					end
				end
			end
			return filtered
		end

		local entries = {}
		local args = {}
		if args and #args > 0 then
			entries = filter_tests(entries, args)
		end

		local suites = {}
		local suite_names = sorted_keys(suites)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_filter_branch.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_filter_branch")
	exit := graph.Exit()
	suitesSym, ok := graph.SymbolAt(exit, "suites")
	if !ok || suitesSym == 0 {
		t.Fatal("expected symbol for suites")
	}
	suiteNamesSym, ok := graph.SymbolAt(exit, "suite_names")
	if !ok || suiteNamesSym == 0 {
		t.Fatal("expected symbol for suite_names")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	evidence := trace.GraphEvidence(graph, graph.Bindings())
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: evidence,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, keyscoll.BuildKeysCollectorDetector(graph, evidence, nil, newTestGraphProvider(graph.Bindings())))

	src, ok := inputs.KeysProvenance[suiteNamesSym]
	if !ok || src != suitesSym {
		t.Fatalf("expected keys provenance for suite_names %d -> suites %d, got %d (present=%v)", suiteNamesSym, suitesSym, src, ok)
	}
}

func TestExtractAssignments_IndexAssign_NonIdentifierStringKey_UsesIndexStringSegment(t *testing.T) {
	code := `
		local t = {}
		local src = "v"
		t["x-y"] = src
	`
	chunk, err := parse.ParseString(code, "emit_index_string_key.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_index_string_key")
	exit := graph.Exit()
	tSym, ok := graph.SymbolAt(exit, "t")
	if !ok || tSym == 0 {
		t.Fatal("expected symbol for t")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, nil)

	var got *flow.UnifiedAssignment
	for i := range inputs.Assignments {
		assign := &inputs.Assignments[i]
		if assign.TargetPath.Symbol != tSym || len(assign.TargetPath.Segments) != 1 {
			continue
		}
		seg := assign.TargetPath.Segments[0]
		if seg.Name == "x-y" {
			got = assign
			break
		}
	}
	if got == nil {
		t.Fatal("expected assignment for t[\"x-y\"]")
	}
	seg := got.TargetPath.Segments[0]
	if seg.Kind != constraint.SegmentIndexString {
		t.Fatalf("expected SegmentIndexString, got %v", seg.Kind)
	}
}

func TestExtractAssignments_LengthIndexReadCarriesSemanticSource(t *testing.T) {
	code := `
		local messages = {}
		if #messages > 0 then
			local last = messages[#messages]
		end
	`
	chunk, err := parse.ParseString(code, "emit_length_index_read.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_length_index_read")
	exit := graph.Exit()
	messagesSym, ok := graph.SymbolAt(exit, "messages")
	if !ok || messagesSym == 0 {
		t.Fatal("expected symbol for messages")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, nil)

	for i := range inputs.Assignments {
		assign := inputs.Assignments[i]
		if assign.TargetPath.Root != "last" {
			continue
		}
		if assign.Source.Kind != flow.AssignmentSourceLengthIndex {
			t.Fatalf("expected length-index source for last assignment, got %#v", assign)
		}
		if assign.Source.ContainerPath.Symbol != messagesSym {
			t.Fatalf("length-index container symbol = %d, want %d", assign.Source.ContainerPath.Symbol, messagesSym)
		}
		if assign.Source.Offset != 0 {
			t.Fatalf("length-index offset = %d, want 0", assign.Source.Offset)
		}
		return
	}
	t.Fatalf("expected assignment to last, got %#v", inputs.Assignments)
}

func TestExtractAssignments_NestedDynamicIndex_LiftsToRootIndexer(t *testing.T) {
	code := `
		local subscribers = {}
		local cid = "c1"
		local sub_pid = "p1"

		if not subscribers[cid] then
			subscribers[cid] = {}
		end
		subscribers[cid][sub_pid] = true
	`

	chunk, err := parse.ParseString(code, "emit_nested_dynamic_index.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_nested_dynamic_index")
	exit := graph.Exit()
	subscribersSym, ok := graph.SymbolAt(exit, "subscribers")
	if !ok || subscribersSym == 0 {
		t.Fatal("expected symbol for subscribers")
	}
	cidSym, ok := graph.SymbolAt(exit, "cid")
	if !ok || cidSym == 0 {
		t.Fatal("expected symbol for cid")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, nil)

	var lifted *flow.MapMutatorAssignment
	for i := range inputs.MapMutatorAssignments {
		assign := &inputs.MapMutatorAssignments[i]
		if assign.Target.Symbol != subscribersSym || assign.KeySymbol != cidSym || len(assign.Target.Segments) != 0 {
			continue
		}
		if _, ok := assign.ValueType.(*typ.Map); ok {
			lifted = assign
			break
		}
	}
	if lifted == nil {
		t.Fatalf("expected lifted map mutator assignment for nested dynamic write, got %#v", inputs.MapMutatorAssignments)
	}
}

func TestExtractAssignments_NestedDynamicFieldAndSiblingMutatorStayOnSeparatePaths(t *testing.T) {
	code := `
		local self = { nodes = {}, queued_commands = {} }
		local node_id = "root"
		self.nodes[node_id].status = "completed"
		table.insert(self.queued_commands, { type = "UPDATE_NODE" })
		local node_data = self.nodes[node_id]
	`

	chunk, err := parse.ParseString(code, "emit_nested_dynamic_sibling_paths.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_nested_dynamic_sibling_paths")
	exit := graph.Exit()
	selfSym, ok := graph.SymbolAt(exit, "self")
	if !ok || selfSym == 0 {
		t.Fatal("expected symbol for self")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:    graph,
		Evidence: trace.GraphEvidence(graph, graph.Bindings()),
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, nil)

	var sawNodesWrite bool
	for i := range inputs.MapMutatorAssignments {
		assign := inputs.MapMutatorAssignments[i]
		if assign.Target.Symbol != selfSym || len(assign.Target.Segments) != 1 {
			continue
		}
		seg := assign.Target.Segments[0]
		if seg.Kind == constraint.SegmentField && seg.Name == "nodes" {
			if assign.ValueMode != flow.MapMutationValueUpdate {
				t.Fatalf("expected nested dynamic write to use value-update mode, got %v", assign.ValueMode)
			}
			if rec, ok := assign.ValueType.(*typ.Record); !ok || rec.GetField("status") == nil {
				t.Fatalf("expected nested dynamic write value to preserve .status field, got %#v", assign.ValueType)
			}
			if !assign.ValuePath.IsEmpty() {
				t.Fatalf("expected shaped nested write not to use raw source value path, got %+v", assign.ValuePath)
			}
			sawNodesWrite = true
		}
	}
	if !sawNodesWrite {
		t.Fatalf("expected nested dynamic write under self.nodes, got %#v", inputs.MapMutatorAssignments)
	}

	var sawNodeRead bool
	for i := range inputs.Assignments {
		assign := inputs.Assignments[i]
		if assign.Source.Kind != flow.AssignmentSourceMapElement || assign.Source.MapPath.Symbol != selfSym {
			continue
		}
		segs := assign.Source.MapPath.Segments
		if len(segs) == 1 && segs[0].Kind == constraint.SegmentField && segs[0].Name == "nodes" {
			sawNodeRead = true
		}
	}
	if !sawNodeRead {
		t.Fatalf("expected dynamic read from self.nodes, got %#v", inputs.Assignments)
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

func TestCorrelationsFromFunctionType_ShapeOnlyLuaErrorDoesNotInferCorrelation(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.LuaError)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("shape-only returns must not infer correlations, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_ShapeOnlyExtraReturnsDoNotInferCorrelation(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.LuaError), typ.NewOptional(typ.Boolean)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("shape-only returns must not infer correlations, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_ShapeOnlyStringErrorDoesNotInferCorrelation(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.String)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("shape-only returns must not infer correlations, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_ShapeOnlyStructuredErrorDoesNotInferCorrelation(t *testing.T) {
	errorType := typ.NewRecord().
		Field("status_code", typ.Number).
		Field("message", typ.String).
		Build()
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(errorType)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("shape-only returns must not infer correlations, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_ShapeOnlyUnionErrorDoesNotInferCorrelation(t *testing.T) {
	errorType := typ.NewRecord().
		Field("message", typ.String).
		Build()
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.NewUnion(typ.String, typ.LuaError, errorType))).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("shape-only returns must not infer correlations, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_NoImplicitStructuredErrorWithoutMessage(t *testing.T) {
	auxType := typ.NewRecord().
		Field("status_code", typ.Number).
		Build()
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(auxType)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("expected no implicit correlations without message-like error slot, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_UnionMembersShareErrorReturn(t *testing.T) {
	first := typ.Func().
		Param("a", typ.String).
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{
			ValueIndex: 0,
			ErrorIndex: 1,
		})).
		Build()
	second := typ.Func().
		Param("a", typ.Number).
		Returns(typ.Number, typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{
			ValueIndex: 0,
			ErrorIndex: 1,
		})).
		Build()
	fnType := typ.NewUnion(first, second)
	if _, ok := fnType.(*typ.Union); !ok {
		t.Fatalf("test precondition: expected a genuine union of functions, got %T", fnType)
	}
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one error correlation shared by all union members, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_UnionMemberMissingErrorReturnIsUnsound(t *testing.T) {
	withLabel := typ.Func().
		Param("a", typ.String).
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{
			ValueIndex: 0,
			ErrorIndex: 1,
		})).
		Build()
	withoutLabel := typ.Func().
		Param("a", typ.Number).
		Returns(typ.Number, typ.NewOptional(typ.LuaError)).
		Build()
	fnType := typ.NewUnion(withLabel, withoutLabel)
	if _, ok := fnType.(*typ.Union); !ok {
		t.Fatalf("test precondition: expected a genuine union of functions, got %T", fnType)
	}
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("correlation must not fire when a callable union member lacks the label, got inverse=%v co=%v", inverse, co)
	}
}

func TestGuardedTypeCorrelationsFromCall_CallbackReturnOnTruthy(t *testing.T) {
	fnType := typ.Func().
		Param("f", typ.Any).
		Returns(typ.Boolean, typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Return{
			ReturnIndex: 1,
			Transform:   effect.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		})).
		Build()

	callInfo := &cfg.CallInfo{
		Args: []ast.Expr{&ast.FunctionExpr{}},
	}
	synth := func(ast.Expr, cfg.Point) typ.Type {
		return typ.Func().Returns(typ.String).Build()
	}

	got := guardedTypeCorrelationsFromCall(fnType, callInfo, synth, 1)
	if len(got) != 1 {
		t.Fatalf("expected one guarded correlation, got %v", got)
	}
	if got[0].GuardIndex != 0 || got[0].TargetIndex != 1 || !got[0].GuardOnTruthy {
		t.Fatalf("unexpected guarded correlation shape: %+v", got[0])
	}
	if !typ.TypeEquals(got[0].TargetType, typ.String) {
		t.Fatalf("expected guarded target type string, got %v", got[0].TargetType)
	}
}
