package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenType_Nil(t *testing.T) {
	result := WidenType(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestWidenType_BooleanLiteral(t *testing.T) {
	lit := typ.LiteralBool(true)
	result := WidenType(lit)
	if result != typ.Boolean {
		t.Errorf("expected Boolean, got %v", result)
	}
}

func TestWidenType_IntegerLiteral(t *testing.T) {
	lit := typ.LiteralInt(42)
	result := WidenType(lit)
	if result != typ.Integer {
		t.Errorf("expected Integer, got %v", result)
	}
}

func TestWidenType_NumberLiteral(t *testing.T) {
	lit := typ.LiteralNumber(3.14)
	result := WidenType(lit)
	if result != typ.Number {
		t.Errorf("expected Number, got %v", result)
	}
}

func TestWidenType_StringLiteral(t *testing.T) {
	lit := typ.LiteralString("hello")
	result := WidenType(lit)
	if result != typ.String {
		t.Errorf("expected String, got %v", result)
	}
}

func TestNormalizeBodyType_PreservesTupleDiscriminants(t *testing.T) {
	in := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.LiteralString("function_result")).
			Field("content", typ.LiteralString("ok")).
			Field("function_call_id", typ.LiteralString("tool")).
			Build(),
		typ.NewRecord().
			Field("role", typ.LiteralString("developer")).
			Field("content", typ.LiteralString("merge")).
			Build(),
	)
	got := NormalizeBodyType(in)
	if !typ.TypeEquals(got, in) {
		t.Fatalf("NormalizeBodyType() = %v, want %v", got, in)
	}
}

func TestMergeBodyAt_PreservesTupleDiscriminants(t *testing.T) {
	observed := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.LiteralString("function_result")).
			Field("content", typ.LiteralString("ok")).
			Field("function_call_id", typ.LiteralString("tool")).
			Build(),
		typ.NewRecord().
			Field("role", typ.LiteralString("developer")).
			Field("content", typ.LiteralString("merge")).
			Build(),
	)
	got, changed := MergeBodyAt(nil, 0, observed, typ.JoinPreferNonSoft)
	if !changed || len(got) != 1 || !typ.TypeEquals(got[0], observed) {
		t.Fatalf("MergeBodyAt() = %v changed=%v, want %v", got, changed, observed)
	}
}

func TestJoinBodyVectors_IsIdempotentForTupleDiscriminants(t *testing.T) {
	observed := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.LiteralString("function_result")).
			Field("content", typ.LiteralString("ok")).
			Field("function_call_id", typ.LiteralString("tool")).
			Build(),
		typ.NewRecord().
			Field("role", typ.LiteralString("developer")).
			Field("content", typ.LiteralString("merge")).
			Build(),
	)
	got := JoinBodyVectors([]typ.Type{observed}, []typ.Type{observed})
	if len(got) != 1 || !typ.TypeEquals(got[0], observed) {
		t.Fatalf("JoinBodyVectors() = %v, want %v", got, observed)
	}
}

func TestJoinBodyVectors_PreservesDiscriminatedArrayElementVariants(t *testing.T) {
	functionResult := typ.NewRecord().
		Field("role", typ.LiteralString("function_result")).
		Field("function_call_id", typ.String).
		Build()
	functionCall := typ.NewRecord().
		Field("role", typ.LiteralString("function_call")).
		Field("function_call", typ.NewRecord().Field("id", typ.String).Build()).
		Build()
	content := typ.NewRecord().
		Field("content", typ.String).
		Build()

	got := JoinBodyVectors(
		JoinBodyVectors([]typ.Type{typ.NewArray(functionResult)}, []typ.Type{typ.NewArray(functionCall)}),
		[]typ.Type{typ.NewArray(content)},
	)
	want := typ.NewArray(typ.NewUnion(functionResult, functionCall, content))
	if len(got) != 1 || !typ.TypeEquals(got[0], want) {
		t.Fatalf("JoinBodyVectors() = %v, want %v", got, want)
	}
}

func TestSourceParamAnnotated(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{
		Names: []string{"plain", "typed"},
		Types: []ast.TypeExpr{nil, &ast.PrimitiveTypeExpr{Name: "number"}},
	}}

	if SourceParamAnnotated(fn, 0) {
		t.Fatal("plain parameter reported annotated")
	}
	if !SourceParamAnnotated(fn, 1) {
		t.Fatal("typed parameter not reported annotated")
	}
	if SourceParamAnnotated(fn, -1) || SourceParamAnnotated(fn, 2) || SourceParamAnnotated(nil, 0) {
		t.Fatal("out-of-range/nil parameter reported annotated")
	}
}

func TestParamSlotForCallArgUsesCanonicalParamSlots(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"opts"}}}
	g := cfg.Build(fn)

	source, slot, ok := ParamSlotForCallArg(g, fn, &ast.FuncCallExpr{}, 0)
	if !ok || source != 0 || slot != 0 {
		t.Fatalf("plain call arg maps to source/slot %d/%d ok=%v, want 0/0 true", source, slot, ok)
	}
}

func TestParamSlotForCallArgShiftsMethodReceiver(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"self", "opts"}}}
	g := cfg.Build(fn)

	source, slot, ok := ParamSlotForCallArg(g, fn, &ast.FuncCallExpr{Method: "with_options"}, 0)
	if !ok || source != 1 || slot != 1 {
		t.Fatalf("method call arg maps to source/slot %d/%d ok=%v, want 1/1 true", source, slot, ok)
	}
}

func TestParamSlotForCallArgUsesRuntimeSlotsForImplicitSelf(t *testing.T) {
	fn, g := implicitSelfMethodGraph(t)

	source, slot, ok := ParamSlotForCallArg(g, fn, &ast.FuncCallExpr{Method: "run"}, 0)
	if !ok || source != 0 || slot != 1 {
		t.Fatalf("method call arg maps to source/slot %d/%d ok=%v, want 0/1 true", source, slot, ok)
	}

	source, slot, ok = ParamSlotForCallArg(g, fn, &ast.FuncCallExpr{}, 0)
	if !ok || source != -1 || slot != 0 {
		t.Fatalf("plain call arg0 maps to source/slot %d/%d ok=%v, want -1/0 true", source, slot, ok)
	}

	source, slot, ok = ParamSlotForCallArg(g, fn, &ast.FuncCallExpr{Args: []ast.Expr{
		&ast.IdentExpr{Value: "selfArg"},
		&ast.IdentExpr{Value: "opts"},
	}}, 1)
	if !ok || source != 0 || slot != 1 {
		t.Fatalf("plain call arg1 maps to source/slot %d/%d ok=%v, want 0/1 true", source, slot, ok)
	}
}

func TestParamSlotForRuntimeArgIncludesImplicitSelfSlot(t *testing.T) {
	fn, g := implicitSelfMethodGraph(t)

	source, slot, ok := ParamSlotForRuntimeArg(g, fn, 0)
	if !ok || source != -1 || slot != 0 {
		t.Fatalf("runtime arg0 maps to source/slot %d/%d ok=%v, want implicit self -1/0 true", source, slot, ok)
	}

	source, slot, ok = ParamSlotForRuntimeArg(g, fn, 1)
	if !ok || source != 0 || slot != 1 {
		t.Fatalf("runtime arg1 maps to source/slot %d/%d ok=%v, want first source param 0/1 true", source, slot, ok)
	}
}

func TestCallArgContractTypesProjectsBodyDemandThroughParamSlots(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"self", "opts"}}}
	g := cfg.Build(fn)
	call := &ast.FuncCallExpr{
		Method: "with_options",
		Args:   []ast.Expr{&ast.IdentExpr{Value: "opts"}},
	}

	got := CallArgContractTypes(CallArgContractConfig{
		Graph:     g,
		Function:  fn,
		Call:      call,
		Contracts: Contracts{1: DemandFromType(typ.String)},
	})
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("method call arg demand = %v, want string", got)
	}
}

func TestContractTypesProjectsSolvedContracts(t *testing.T) {
	got := ContractTypes(Contracts{
		-1: DemandFromType(typ.Number),
		0:  DemandFromType(nil),
		1:  DemandFromType(typ.String),
	})
	if len(got) != 1 || !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("ContractTypes() = %#v, want only slot 1 string", got)
	}
	got[1] = typ.Number
	again := ContractTypes(Contracts{1: DemandFromType(typ.String)})
	if !typ.TypeEquals(again[1], typ.String) {
		t.Fatalf("ContractTypes exposed mutable backing state: %#v", again)
	}
}

func TestCallArgContractTypesProjectsImplicitSelfPlainCallBySlot(t *testing.T) {
	fn, g := implicitSelfMethodGraph(t)
	call := &ast.FuncCallExpr{Args: []ast.Expr{
		&ast.IdentExpr{Value: "runner"},
		&ast.IdentExpr{Value: "opts"},
	}}

	got := CallArgContractTypes(CallArgContractConfig{
		Graph:     g,
		Function:  fn,
		Call:      call,
		Contracts: Contracts{0: DemandFromType(typ.NewRecord().ReadonlyField("run", typ.Any).Build()), 1: DemandFromType(typ.String)},
	})
	if len(got) != 2 || !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("plain method-function arg demand = %v, want slot1 string demand", got)
	}
	if got[0] == nil {
		t.Fatalf("plain method-function arg0 should receive implicit self slot demand, got %v", got)
	}
}

func TestCallArgContractTypesAnnotatedParamKeepsDeclaredContract(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{
		Names: []string{"value"},
		Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
	}}
	g := cfg.Build(fn)
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "value"}}}

	got := CallArgContractTypes(CallArgContractConfig{
		Graph:     g,
		Function:  fn,
		Call:      call,
		Contracts: Contracts{0: DemandFromType(typ.String)},
		DeclaredSlotType: func(slot int) typ.Type {
			if slot == 0 {
				return typ.Number
			}
			return nil
		},
	})
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Number) {
		t.Fatalf("annotated call arg demand = %v, want declared number", got)
	}
}

func TestFunctionCallArgContractTypesProjectsRequiredAndVariadicParams(t *testing.T) {
	fn := typ.Func().
		Param("required", typ.String).
		OptParam("optional", typ.Number).
		Variadic(typ.Boolean).
		Build()
	call := &ast.FuncCallExpr{Args: []ast.Expr{
		&ast.IdentExpr{Value: "a"},
		&ast.IdentExpr{Value: "b"},
		&ast.IdentExpr{Value: "c"},
	}}

	got := FunctionCallArgContractTypes(call, fn)
	if len(got) != 3 {
		t.Fatalf("signature call arg contracts = %v, want 3 slots", got)
	}
	if !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("arg0 contract = %v, want string", got[0])
	}
	if got[1] != nil {
		t.Fatalf("optional arg1 should not produce obligation, got %v", got[1])
	}
	if !typ.TypeEquals(got[2], typ.Boolean) {
		t.Fatalf("arg2 variadic contract = %v, want boolean", got[2])
	}
}

func TestJoinCallArgContractTypesOwnsContractVectorAlgebra(t *testing.T) {
	out := JoinCallArgContractTypes(nil, []typ.Type{typ.String, nil})
	out = JoinCallArgContractTypes(out, []typ.Type{typ.Number, typ.Boolean})

	if len(out) != 2 {
		t.Fatalf("joined contracts = %v, want 2 slots", out)
	}
	if !typ.IsNever(out[0]) {
		t.Fatalf("incompatible arg0 obligation = %v, want never", out[0])
	}
	if !typ.TypeEquals(out[1], typ.Boolean) {
		t.Fatalf("arg1 obligation = %v, want boolean", out[1])
	}
	if NormalizeCallArgContractTypes([]typ.Type{nil, typ.Any, typ.Unknown}) != nil {
		t.Fatal("all-empty call arg contract vector must normalize to nil")
	}
}

func TestCallArgDemandProjectionJoinsMultipleTargets(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"value"}}}
	g := cfg.Build(fn)
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "value"}}}

	got := CallArgDemandProjection{
		Call: call,
		Targets: []CallArgDemandTarget{
			{
				Graph:     g,
				Function:  fn,
				Contracts: Contracts{0: DemandFromType(typ.String)},
			},
			{
				Graph:     g,
				Function:  fn,
				Contracts: Contracts{0: DemandFromType(typ.Number)},
			},
		},
	}.DemandTypes()

	if len(got) != 1 || !typ.IsNever(got[0]) {
		t.Fatalf("joined target demands = %v, want [never]", got)
	}
}

func TestCallArgDemandProjectionFiltersAnnotatedSourceParams(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{
		Names: []string{"value"},
		Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
	}}
	g := cfg.Build(fn)
	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "value"}}}

	got := CallArgDemandProjection{
		Call: call,
		Targets: []CallArgDemandTarget{
			{
				Graph:     g,
				Function:  fn,
				Contracts: Contracts{0: DemandFromType(typ.String)},
				DeclaredSlotType: func(slot int) typ.Type {
					if slot == 0 {
						return typ.Number
					}
					return nil
				},
				SourceParamAnnotated: func(sourceParam int) bool {
					return sourceParam == 0
				},
			},
		},
	}.DemandTypes()

	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Number) {
		t.Fatalf("annotated projection demand = %v, want number", got)
	}
}

func TestCallArgDemandProjectionEmptyTargetsNormalizeNil(t *testing.T) {
	got := CallArgDemandProjection{
		Call: &ast.FuncCallExpr{
			Args: []ast.Expr{&ast.IdentExpr{Value: "value"}},
		},
	}.DemandTypes()
	if got != nil {
		t.Fatalf("empty projection demand = %v, want nil", got)
	}
}

func implicitSelfMethodGraph(t *testing.T) (*ast.FunctionExpr, *cfg.Graph) {
	t.Helper()
	stmts, err := parse.ParseString(`
local Runner = {}
function Runner:run(options)
	return options
end
`, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := cfg.BuildBlock(stmts)
	if root == nil {
		t.Fatal("BuildBlock returned nil")
	}
	var fn *ast.FunctionExpr
	root.EachFuncDef(func(_ cfg.Point, info *cfg.FuncDefInfo) {
		if fn != nil || info == nil || !info.IsMethod {
			return
		}
		fn = info.FuncExpr
	})
	if fn == nil {
		t.Fatal("method function not found")
	}
	g := cfg.BuildWithBindings(fn, root.Bindings())
	if g == nil {
		t.Fatal("BuildWithBindings returned nil")
	}
	return fn, g
}

func TestMergeBodyCallArgAt_PreservesArrayElementDiscriminants(t *testing.T) {
	functionResult := typ.NewRecord().
		Field("role", typ.LiteralString("function_result")).
		Field("function_call_id", typ.LiteralString("tool")).
		Field("content", typ.LiteralString("ok")).
		Build()
	developer := typ.NewRecord().
		Field("role", typ.LiteralString("developer")).
		Field("content", typ.LiteralString("merge")).
		Build()
	observed := typ.NewArray(typ.NewUnion(functionResult, developer))

	body, changed := MergeBodyCallArgAt(nil, 0, observed, typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected body-effective evidence")
	}
	if !typ.TypeEquals(body[0], observed) {
		t.Fatalf("expected body evidence to preserve array element discriminants as %v, got %v", observed, body[0])
	}
}

func TestJoinEntryVectors_PreservesExplicitNilRuntimeState(t *testing.T) {
	rec := typ.NewRecord().Field("id", typ.String).Build()
	got := JoinEntryVectors([]typ.Type{typ.Nil}, []typ.Type{rec})
	want := typ.NewOptional(rec)
	if len(got) != 1 || !typ.TypeEquals(got[0], want) {
		t.Fatalf("JoinEntryVectors() = %v, want %v", got, want)
	}
}

func TestJoinEntryVectors_RecursiveAndBroadMapEntriesShareUpperBound(t *testing.T) {
	baseMeta := typ.NewMap(typ.String, typ.Any)
	flow := typ.NewRecursive("SuiteFlow", func(self typ.Type) typ.Type {
		entry := typ.NewRecord().
			Field("id", typ.String).
			OptField("meta", self).
			Field("name", typ.String).
			Build()
		return typ.NewMap(typ.String, typ.NewArray(entry))
	})
	recursiveEntries := typ.NewArray(typ.NewRecord().
		Field("id", typ.String).
		OptField("meta", flow).
		Field("name", typ.String).
		Build())
	broadEntries := typ.NewArray(typ.NewRecord().
		Field("id", typ.String).
		OptField("meta", baseMeta).
		Field("name", typ.String).
		Build())
	recursiveEntry := recursiveEntries.Element
	broadEntry := broadEntries.Element

	if !subtype.IsSubtype(flow, baseMeta) || subtype.IsSubtype(baseMeta, flow) {
		t.Fatalf("unexpected recursive/broad map subtype relation: flow<:map=%v map<:flow=%v", subtype.IsSubtype(flow, baseMeta), subtype.IsSubtype(baseMeta, flow))
	}
	joinedNonNilMeta := joinNonNilBody(flow, baseMeta)
	if !subtype.IsSubtype(flow, joinedNonNilMeta) || !subtype.IsSubtype(baseMeta, joinedNonNilMeta) {
		t.Fatalf("joined non-nil meta must admit recursive and broad observations, got %v", joinedNonNilMeta)
	}
	joinedMeta := JoinBody(typ.NewOptional(flow), typ.NewOptional(baseMeta))
	if !subtype.IsSubtype(typ.NewOptional(flow), joinedMeta) || !subtype.IsSubtype(typ.NewOptional(baseMeta), joinedMeta) {
		t.Fatalf("joined meta must admit recursive and broad observations, got %v", joinedMeta)
	}
	joinedEntry := JoinEntry(recursiveEntry, broadEntry)
	if !subtype.IsSubtype(recursiveEntry, joinedEntry) || !subtype.IsSubtype(broadEntry, joinedEntry) {
		t.Fatalf("joined entry must admit recursive and broad observations, got %v", joinedEntry)
	}

	got := JoinEntryVectors([]typ.Type{recursiveEntries}, []typ.Type{broadEntries})
	if len(got) != 1 {
		t.Fatalf("JoinEntryVectors() len = %d, want 1: %v", len(got), got)
	}
	if !subtype.IsSubtype(recursiveEntries, got[0]) {
		t.Fatalf("joined entry evidence must admit recursive entries: got %v", got[0])
	}
	if !subtype.IsSubtype(broadEntries, got[0]) {
		t.Fatalf("joined entry evidence must admit broad-map entries: got %v", got[0])
	}
}

func TestMergeBodyCallArgAt_PreservesOptionalContextTableKey(t *testing.T) {
	context := typ.NewOptional(typ.NewMap(typ.String, typ.Any))
	openContext := typ.NewOptional(typ.NewRecord().MapComponent(typ.Any, typ.Any).Build())

	got, _ := MergeBodyCallArgAt([]typ.Type{context}, 0, openContext, typ.JoinPreferNonSoft, false)
	if len(got) != 1 || !typ.TypeEquals(got[0], context) {
		t.Fatalf("MergeBodyCallArgAt() = %v, want %v", got, context)
	}
}

func TestMergeBodyUnannotatedParam_PreservesTupleDiscriminantsWhenPublicSignatureIsWidened(t *testing.T) {
	bodyEvidence := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.LiteralString("function_result")).
			Field("content", typ.LiteralString("ok")).
			Field("function_call_id", typ.LiteralString("tool")).
			Build(),
		typ.NewRecord().
			Field("role", typ.LiteralString("developer")).
			Field("content", typ.LiteralString("merge")).
			Build(),
	)
	publicSignature := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.String).
			Field("content", typ.String).
			Field("function_call_id", typ.String).
			Build(),
		typ.NewRecord().
			Field("role", typ.String).
			Field("content", typ.String).
			Build(),
	)

	got, _ := MergeBodyUnannotatedParam(typ.Param{Name: "messages", Type: publicSignature}, bodyEvidence)
	if !typ.TypeEquals(got, bodyEvidence) {
		t.Fatalf("MergeBodyUnannotatedParam() = %v, want %v", got, bodyEvidence)
	}

	publicGot, _ := MergeUnannotatedParam(typ.Param{Name: "messages", Type: publicSignature}, bodyEvidence)
	if typ.TypeEquals(publicGot, bodyEvidence) {
		t.Fatalf("MergeUnannotatedParam() preserved body-only literals in public signature: %v", publicGot)
	}
}

func TestWidenType_NonLiteral(t *testing.T) {
	result := WidenType(typ.String)
	if result != typ.String {
		t.Errorf("expected String unchanged, got %v", result)
	}
}

func TestWidenType_Alias(t *testing.T) {
	alias := typ.NewAlias("NumAlias", typ.Number)
	result := WidenType(alias)
	if result != typ.Number {
		t.Errorf("expected alias to widen to Number, got %v", result)
	}
}

func TestWidenType_Optional(t *testing.T) {
	lit := typ.LiteralString("hello")
	opt := typ.NewOptional(lit)
	result := WidenType(opt)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	optResult, ok := result.(*typ.Optional)
	if !ok {
		t.Fatalf("expected Optional, got %T", result)
	}
	if optResult.Inner != typ.String {
		t.Errorf("expected inner to be String, got %v", optResult.Inner)
	}
}

func TestWidenType_Union(t *testing.T) {
	lit1 := typ.LiteralString("a")
	lit2 := typ.LiteralNumber(1.0)
	union := typ.NewUnion(lit1, lit2)
	result := WidenType(union)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestNormalizeType_TableTopAbsorbsPreciseTableMembers(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	preciseA := typ.NewRecord().
		Field("name", typ.String).
		Field("tools", typ.NewArray(typ.String)).
		Build()
	preciseB := typ.NewMap(typ.String, typ.Integer)
	evidence := typ.NewUnion(typ.NewOptional(tableTop), preciseA, preciseB, typ.String)

	got := NormalizeType(evidence)
	want := typ.NewUnion(typ.NewOptional(tableTop), typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected table top to absorb precise table members as %v, got %v", want, got)
	}
}

func TestWidenType_RecordPreservesClosedShape(t *testing.T) {
	rec := typ.NewRecord().
		Field("pid", typ.LiteralString("abc")).
		Field("topic", typ.LiteralString("test:update")).
		Build()

	result := WidenType(rec)
	widened, ok := result.(*typ.Record)
	if !ok {
		t.Fatalf("expected record result, got %T", result)
	}
	if widened.Open {
		t.Fatalf("expected parameter evidence to preserve closed call-site shape, got open: %v", widened)
	}

	pid := widened.GetField("pid")
	if pid == nil || !typ.TypeEquals(pid.Type, typ.String) {
		t.Fatalf("expected pid field widened to string, got %v", pid)
	}
	topic := widened.GetField("topic")
	if topic == nil || !typ.TypeEquals(topic.Type, typ.String) {
		t.Fatalf("expected topic field widened to string, got %v", topic)
	}
}

func TestMergeIntoSignature_ImplicitSelfUsesEffectiveHintSlots(t *testing.T) {
	fn := functionWithParams("name")
	sig := typ.Func().
		Param("self", typ.Unknown).
		Param("name", typ.Unknown).
		Build()
	selfType := typ.NewRecord().Field("prefix", typ.String).Build()

	got := MergeIntoSignature(fn, []typ.Type{selfType, typ.String}, sig)
	if got == nil || len(got.Params) != 2 {
		t.Fatalf("unexpected merged signature: %v", got)
	}
	if !typ.TypeEquals(got.Params[0].Type, selfType) {
		t.Fatalf("self evidence should use effective slot 0, got %v", got.Params[0].Type)
	}
	if !typ.TypeEquals(got.Params[1].Type, typ.String) {
		t.Fatalf("source parameter evidence should use effective slot 1, got %v", got.Params[1].Type)
	}
}

func TestMergeIntoSignature_SoftArrayAnnotationPreservesContainerShape(t *testing.T) {
	fn := functionWithParams("responses")
	fn.ParList.Types = []ast.TypeExpr{&ast.ArrayTypeExpr{
		Element: &ast.PrimitiveTypeExpr{Name: "any"},
	}}
	sig := typ.Func().
		Param("responses", typ.NewArray(typ.Any)).
		Build()
	evidence := typ.NewTuple(
		typ.NewRecord().Field("ok", typ.Boolean).Build(),
		typ.NewRecord().Field("ok", typ.Boolean).Build(),
	)

	got := MergeIntoSignature(fn, []typ.Type{evidence}, sig)
	if got == nil || len(got.Params) != 1 {
		t.Fatalf("unexpected merged signature: %v", got)
	}
	want := typ.NewArray(typ.NewRecord().Field("ok", typ.Boolean).Build())
	if !typ.TypeEquals(got.Params[0].Type, want) {
		t.Fatalf("soft {any} parameter annotation must refine element domain without becoming a tuple, got %v", got.Params[0].Type)
	}
}

func TestMergeIntoSignature_HardAnyAnnotationRemainsAuthoritative(t *testing.T) {
	fn := functionWithParams("value")
	fn.ParList.Types = []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "any"}}
	sig := typ.Func().
		Param("value", typ.Any).
		Build()
	evidence := typ.NewRecord().Field("id", typ.String).Build()

	got := MergeIntoSignature(fn, []typ.Type{evidence}, sig)
	if got != sig {
		t.Fatalf("hard any annotation must stay authoritative, got %v", got)
	}
}

func TestMergeIntoSignature_PreservesExplicitNilabilityOnOptionalSlot(t *testing.T) {
	fn := functionWithParams("context")
	context := typ.NewRecord().
		MapComponent(typ.String, typ.Any).
		SetOpen(true).
		Build()
	sig := typ.Func().OptParam("context", typ.Any).Build()

	got := MergeIntoSignature(fn, []typ.Type{typ.NewOptional(context)}, sig)
	if got == nil || len(got.Params) != 1 {
		t.Fatalf("unexpected merged signature: %v", got)
	}
	if !got.Params[0].Optional {
		t.Fatalf("expected parameter slot to remain optional: %v", got)
	}
	want := typ.NewOptional(context)
	if !typ.TypeEquals(got.Params[0].Type, want) {
		t.Fatalf("expected nilability to remain in the value type, got %v", got.Params[0].Type)
	}
}

func TestProjectToParameterUse_KeepsDemandedRecordFields(t *testing.T) {
	fn := functionWithParams("client", "model_id")
	fn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "client"},
					Key:    &ast.StringExpr{Value: "invoke"},
				},
				Args: []ast.Expr{
					&ast.IdentExpr{Value: "model_id"},
					&ast.TableExpr{},
					&ast.TableExpr{},
				},
			},
		}},
	}
	graph := cfg.Build(fn)
	invoke := typ.Func().Param("model_id", typ.String).Returns(typ.Unknown).Build()
	client := typ.NewRecord().
		Field("invoke", invoke).
		Field("process_converse_stream", typ.Func().Returns(typ.String).Build()).
		Field("_credentials", typ.String).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{client, typ.String})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected client evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("invoke") == nil {
		t.Fatalf("projected client evidence lost demanded invoke field: %v", rec)
	}
	for _, unused := range []string{"process_converse_stream", "_credentials"} {
		if rec.GetField(unused) != nil {
			t.Fatalf("projected client evidence kept unused field %q: %v", unused, rec)
		}
	}
	if !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("directly used scalar evidence should stay intact, got %v", got[1])
	}
}

func TestProjectToParameterUse_KeepsDemandedAbsentRecordFieldsAsNil(t *testing.T) {
	fn := functionWithParams("options")
	fn.Stmts = []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "stream"},
			},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{
				&ast.AttrGetExpr{
					Object: &ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "options"},
						Key:    &ast.StringExpr{Value: "headers"},
					},
					Key: &ast.StringExpr{Value: "Accept"},
				},
			},
			Rhs: []ast.Expr{&ast.StringExpr{Value: "application/json"}},
		},
	}
	graph := cfg.Build(fn)
	evidence := typ.NewRecord().
		Field("headers", typ.NewRecord().Build()).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected options evidence = %T, want record (%v)", got[0], got[0])
	}
	stream := rec.GetField("stream")
	if stream == nil || !typ.TypeEquals(stream.Type, typ.Nil) {
		t.Fatalf("demanded absent stream field should project as nil, got %v in %v", stream, rec)
	}
	if !stream.Optional {
		t.Fatalf("demanded absent stream field should stay optional, got %v in %v", stream, rec)
	}
	headers := rec.GetField("headers")
	if headers == nil {
		t.Fatalf("projected options evidence lost demanded headers field: %v", rec)
	}
}

func TestProjectToParameterUse_WholeForwardingCompletesDemandedFields(t *testing.T) {
	fn := functionWithParams("options")
	fn.Stmts = []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "stream"},
			},
		},
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "forward"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "options"}},
			},
		}},
	}
	graph := cfg.Build(fn, "forward")
	evidence := typ.NewRecord().
		Field("headers", typ.NewRecord().Build()).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected options evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("headers") == nil {
		t.Fatalf("whole forwarding should retain existing evidence fields: %v", rec)
	}
	stream := rec.GetField("stream")
	if stream == nil || !typ.TypeEquals(stream.Type, typ.Nil) {
		t.Fatalf("direct field demand should complete forwarded evidence with stream:nil, got %v in %v", stream, rec)
	}
	if !stream.Optional {
		t.Fatalf("direct field demand should complete forwarded evidence with optional stream:nil, got %v in %v", stream, rec)
	}
}

func TestProjectToParameterUse_SingleFieldWriteDoesNotDemandInputField(t *testing.T) {
	fn := functionWithParams("schema")
	fn.Stmts = []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "schema"},
				Key:    &ast.StringExpr{Value: "examples"},
			}},
			Rhs: []ast.Expr{&ast.NilExpr{}},
		},
		&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "schema"}}},
	}
	graph := cfg.Build(fn)
	evidence := typ.NewRecord().Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected schema evidence = %T, want record (%v)", got[0], got[0])
	}
	if field := rec.GetField("examples"); field != nil {
		t.Fatalf("single-segment field write must not become a caller requirement, got %v in %v", field, rec)
	}
}

func TestProjectSignatureToParamUse_CompletesDemandedAbsentFields(t *testing.T) {
	fn := functionWithParams("info")
	fn.Stmts = []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IdentExpr{Value: "info"}},
			Rhs: []ast.Expr{&ast.LogicalOpExpr{
				Operator: "or",
				Lhs:      &ast.IdentExpr{Value: "info"},
				Rhs:      &ast.TableExpr{},
			}},
		},
		&ast.LocalAssignStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "info"},
				Key:    &ast.StringExpr{Value: "message"},
			},
		}},
		&ast.LocalAssignStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "info"},
				Key:    &ast.StringExpr{Value: "status_code"},
			},
		}},
	}
	graph := cfg.Build(fn)
	info := typ.NewRecord().
		OptField("message", typ.String).
		Build()
	sig := typ.Func().
		Param("info", info).
		Returns(typ.String).
		Build()

	got := ProjectSignatureToParamUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), sig)
	rec, ok := got.Params[0].Type.(*typ.Record)
	if !ok {
		t.Fatalf("projected param = %T, want record (%v)", got.Params[0].Type, got.Params[0].Type)
	}
	if rec.GetField("message") == nil {
		t.Fatalf("projected signature lost existing demanded message field: %v", rec)
	}
	status := rec.GetField("status_code")
	if status == nil || !typ.TypeEquals(status.Type, typ.Nil) {
		t.Fatalf("projected signature should include demanded absent status_code as nil, got %v in %v", status, rec)
	}
	if !status.Optional {
		t.Fatalf("projected signature should include demanded absent status_code as optional nil, got %v in %v", status, rec)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.String) {
		t.Fatalf("projected signature lost returns: %v", got)
	}
}

func TestProjectToParameterUse_TypeGuardDoesNotKeepWholeRecord(t *testing.T) {
	fn := functionWithParams("params")
	fn.Stmts = []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.RelationalOpExpr{
				Operator: "~=",
				Lhs: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "type"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "params"}},
				},
				Rhs: &ast.StringExpr{Value: "table"},
			},
		},
		&ast.IfStmt{
			Condition: &ast.UnaryNotOpExpr{
				Expr: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "params"},
					Key:    &ast.StringExpr{Value: "agent"},
				},
			},
		},
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "params"},
				Key:    &ast.StringExpr{Value: "kind"},
			},
		}},
	}
	graph := cfg.Build(fn)
	evidence := typ.NewRecord().
		OptField("kind", typ.String).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected params evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("kind") == nil {
		t.Fatalf("projected params evidence lost demanded kind field: %v", rec)
	}
	agent := rec.GetField("agent")
	if agent == nil || !typ.TypeEquals(agent.Type, typ.Nil) {
		t.Fatalf("type(params) should not force whole-record evidence; agent = %v in %v", agent, rec)
	}
}

func TestProjectToParameterUse_SelfDefaultDoesNotKeepWholeRecord(t *testing.T) {
	fn := functionWithParams("options")
	optionsIdent := &ast.IdentExpr{Value: "options"}
	fn.Stmts = []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{optionsIdent},
			Rhs: []ast.Expr{&ast.LogicalOpExpr{
				Operator: "or",
				Lhs:      &ast.IdentExpr{Value: "options"},
				Rhs:      &ast.TableExpr{},
			}},
		},
		&ast.LocalAssignStmt{
			Names: []string{"method"},
			Exprs: []ast.Expr{&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "method"},
			}},
		},
		&ast.LocalAssignStmt{
			Names: []string{"timeout"},
			Exprs: []ast.Expr{&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "timeout"},
			}},
		},
	}
	graph := cfg.Build(fn)
	evidence := typ.NewRecord().
		OptField("method", typ.String).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected options evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("method") == nil {
		t.Fatalf("projected options evidence lost demanded method field: %v", rec)
	}
	timeout := rec.GetField("timeout")
	if timeout == nil || !typ.TypeEquals(timeout.Type, typ.Nil) {
		t.Fatalf("self-default assignment should not force whole-record evidence; timeout = %v in %v", timeout, rec)
	}
}

func TestProjectToParameterUse_DedupsUnionAfterProjection(t *testing.T) {
	fn := functionWithParams("client")
	fn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.AttrGetExpr{
					Object: &ast.IdentExpr{Value: "client"},
					Key:    &ast.StringExpr{Value: "invoke"},
				},
			},
		}},
	}
	graph := cfg.Build(fn)
	invoke := typ.Func().Returns(typ.Unknown).Build()
	broad := typ.NewRecord().
		Field("invoke", invoke).
		Field("stream", typ.Func().Returns(typ.String).Build()).
		Build()
	narrow := typ.NewRecord().
		Field("invoke", invoke).
		Field("stream", typ.Func().Returns(typ.LiteralString("invalid")).Build()).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{typ.NewUnion(broad, narrow)})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected union evidence = %T, want coalesced record (%v)", got[0], got[0])
	}
	if rec.GetField("invoke") == nil || rec.GetField("stream") != nil {
		t.Fatalf("projected union should keep only invoke, got %v", rec)
	}
}

func TestProjectToParameterUse_PreservesStaticStringIndexKey(t *testing.T) {
	fn := functionWithParams("options")
	fn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "options"},
				Key:    &ast.StringExpr{Value: "x-y"},
			},
		}},
	}
	graph := cfg.Build(fn)
	uses := trace.ParameterUses(graph, fn)
	if len(uses) != 1 || len(uses[0].Fields) != 1 {
		t.Fatalf("parameter uses = %#v, want one structural field key", uses)
	}
	if got := uses[0].Fields[0]; got.Kind != constraint.SegmentIndexString || got.Name != "x-y" {
		t.Fatalf("parameter use field = %#v, want static string-index x-y", got)
	}
	evidence := typ.NewRecord().
		Field("x-y", typ.String).
		Field("unused", typ.Number).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), uses, []typ.Type{evidence})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected options evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("x-y") == nil {
		t.Fatalf("projected evidence lost demanded static string-index field: %v", rec)
	}
	if rec.GetField("unused") != nil {
		t.Fatalf("projected evidence kept unused field: %v", rec)
	}
}

func TestProjectToParameterUse_WholeParameterUseKeepsEvidence(t *testing.T) {
	fn := functionWithParams("client")
	fn.Stmts = []ast.Stmt{
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "use_client"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "client"}},
			},
		}},
	}
	graph := cfg.Build(fn, "use_client")
	client := typ.NewRecord().Field("invoke", typ.Func().Returns(typ.Unknown).Build()).Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{client})
	if !typ.TypeEquals(got[0], client) {
		t.Fatalf("whole-parameter use should keep full evidence, got %v", got[0])
	}
}

func TestProjectToParameterUse_RecursiveForwardingDoesNotKeepWholeEvidence(t *testing.T) {
	recursiveIdent := &ast.IdentExpr{Value: "visit"}
	selfIdent := &ast.IdentExpr{Value: "self"}
	valueIdent := &ast.IdentExpr{Value: "value"}
	fn := functionWithParams("self", "value")
	fn.Stmts = []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.AttrGetExpr{
				Object: valueIdent,
				Key:    &ast.StringExpr{Value: "next"},
			},
			Then: []ast.Stmt{
				&ast.ReturnStmt{Exprs: []ast.Expr{
					&ast.FuncCallExpr{
						Func: recursiveIdent,
						Args: []ast.Expr{
							selfIdent,
							&ast.AttrGetExpr{
								Object: &ast.IdentExpr{Value: "value"},
								Key:    &ast.StringExpr{Value: "next"},
							},
						},
					},
				}},
			},
		},
		&ast.ReturnStmt{Exprs: []ast.Expr{
			&ast.AttrGetExpr{
				Object: &ast.IdentExpr{Value: "self"},
				Key:    &ast.StringExpr{Value: "id"},
			},
		}},
	}
	graph := cfg.Build(fn, "visit")
	if sym, ok := graph.Bindings().SymbolOf(recursiveIdent); ok {
		graph.Bindings().SetFuncLitSymbol(fn, sym)
	}
	selfEvidence := typ.NewRecord().
		Field("id", typ.String).
		Field("command", typ.Func().Returns(typ.Nil, typ.String).Build()).
		Build()

	got := ProjectToParameterUse(graph.ParamSlotsReadOnly(), trace.ParameterUses(graph, fn), []typ.Type{selfEvidence, typ.NewRecord().Field("next", typ.Any).Build()})
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("projected self evidence = %T, want record (%v)", got[0], got[0])
	}
	if rec.GetField("id") == nil {
		t.Fatalf("projected self evidence lost demanded id field: %v", rec)
	}
	if rec.GetField("command") != nil {
		t.Fatalf("recursive forwarding should not keep unused command field: %v", rec)
	}
}

func functionWithParams(names ...string) *ast.FunctionExpr {
	return &ast.FunctionExpr{ParList: &ast.ParList{Names: names}}
}

func TestIsInformative(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "any", in: typ.Any, want: false},
		{name: "unknown", in: typ.Unknown, want: false},
		{name: "never", in: typ.Never, want: false},
		{name: "nil type", in: typ.Nil, want: false},
		{name: "empty record", in: typ.NewRecord().Build(), want: false},
		{name: "map with string key", in: typ.NewMap(typ.String, typ.NewArray(typ.Any)), want: true},
		{name: "record map component", in: typ.NewRecord().MapComponent(typ.String, typ.Any).Build(), want: true},
		{name: "string", in: typ.String, want: true},
		{name: "literal", in: typ.LiteralString("x"), want: true},
		{name: "type param", in: typ.NewTypeParam("T", nil), want: false},
		{name: "ref", in: typ.NewRef("", "Foo"), want: false},
		{name: "optional unknown", in: typ.NewOptional(typ.Unknown), want: false},
		{name: "optional string", in: typ.NewOptional(typ.String), want: true},
		{name: "union placeholders", in: typ.NewUnion(typ.Unknown, typ.Nil), want: false},
		{name: "union with informative member", in: typ.NewUnion(typ.Unknown, typ.String), want: true},
	}

	for _, tt := range tests {
		if got := IsInformative(tt.in); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestEnsureCapacity(t *testing.T) {
	base := []typ.Type{typ.String}
	got := EnsureCapacity(base, 3)
	if len(got) != 3 {
		t.Fatalf("EnsureCapacity len = %d, want 3", len(got))
	}
	if got[0] != typ.String {
		t.Fatalf("EnsureCapacity preserved value = %v, want string", got[0])
	}
}

func TestMergeAt(t *testing.T) {
	join := func(prev, next typ.Type) typ.Type { return typ.JoinPreferNonSoft(prev, next) }

	t.Run("filters non-informative", func(t *testing.T) {
		evidence := []typ.Type{typ.String}
		got, changed := MergeAt(evidence, 1, typ.Unknown, join)
		if changed {
			t.Fatal("expected no change for unknown evidence")
		}
		if len(got) != 1 {
			t.Fatalf("expected unchanged slice len 1, got %d", len(got))
		}
	})

	t.Run("normalizes literal and merges", func(t *testing.T) {
		got, changed := MergeAt(nil, 0, typ.LiteralString("x"), join)
		if !changed {
			t.Fatal("expected merge change for informative literal")
		}
		if len(got) != 1 {
			t.Fatalf("expected one evidence, got %d", len(got))
		}
		if !typ.TypeEquals(got[0], typ.String) {
			t.Fatalf("expected normalized string evidence, got %v", got[0])
		}
	})
}

func TestMergeCallArgAt_PreservesExplicitNilArgument(t *testing.T) {
	got, changed := MergeCallArgAt(nil, 0, typ.Nil, typ.JoinPreferNonSoft, true)
	if !changed {
		t.Fatal("expected nil argument to be recorded")
	}
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Nil) {
		t.Fatalf("expected nil evidence, got %v", got)
	}

	rec := typ.NewRecord().Field("id", typ.String).Build()
	got, changed = MergeCallArgAt(got, 0, rec, typ.JoinPreferNonSoft, true)
	if !changed {
		t.Fatal("expected record call to merge with nil evidence")
	}
	if !typ.TypeEquals(got[0], typ.NewOptional(rec)) {
		t.Fatalf("expected nil plus record calls to produce optional record evidence, got %v", got[0])
	}
}

func TestMergeBodyCallArgAt_PreservesExplicitNilArgument(t *testing.T) {
	got, changed := MergeBodyCallArgAt(nil, 0, typ.Nil, typ.JoinPreferNonSoft, true)
	if !changed {
		t.Fatal("expected nil argument to be recorded")
	}
	got, changed = MergeBodyCallArgAt(got, 0, typ.Number, typ.JoinPreferNonSoft, true)
	if !changed {
		t.Fatal("expected number call to merge with nil evidence")
	}
	want := typ.NewOptional(typ.Number)
	if len(got) != 1 || !typ.TypeEquals(got[0], want) {
		t.Fatalf("expected body call evidence %v, got %v", want, got)
	}
}

func TestHardContractJoin_ConcreteDominatesDynamicSeed(t *testing.T) {
	if got := HardContractJoin(typ.Any, typ.String); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("HardContractJoin(any, string) = %v, want string", got)
	}
	if got := HardContractJoin(typ.Unknown, typ.Integer); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("HardContractJoin(unknown, integer) = %v, want integer", got)
	}
}

func TestHardContractJoin_FoldsRecursiveProductBeforeIntersection(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	base := typ.NewMap(typ.String, entry)
	recursiveObservation := typ.NewMap(typ.String,
		typ.NewRecord().
			Field("child", base).
			Build(),
	)

	got := HardContractJoin(base, recursiveObservation)
	if _, ok := got.(*typ.Recursive); !ok {
		t.Fatalf("HardContractJoin(self-embedding contract) = %T %[1]v, want recursive product", got)
	}
}

func TestHardContractJoin_RecursiveUpperBoundStabilizes(t *testing.T) {
	stable := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewMap(typ.String, typ.NewOptional(self))
	})
	observation := typ.NewMap(typ.String, typ.NewOptional(stable))

	got := HardContractJoin(stable, observation)
	if !typ.TypeEquals(got, stable) {
		t.Fatalf("HardContractJoin(recursive upper, observation) = %v, want %v", got, stable)
	}
}

func TestBodyEntryContractJoin_RecursiveContractCoversEntryObservation(t *testing.T) {
	contract := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})
	entry := typ.NewRecord().
		Field("name", typ.String).
		Field("children", typ.NewArray(contract)).
		Field("full_path", typ.String).
		Build()

	got := BodyEntryContractJoin(entry, contract)
	if !typ.SameNode(got, entry) {
		t.Fatalf("BodyEntryContractJoin(entry, recursive contract) = %T %[1]v, want entry", got)
	}
}

func TestBodyEntryContractJoin_PreservesPreciseContainerEntryUnderBroadContract(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()

	cases := []struct {
		name     string
		entry    typ.Type
		contract typ.Type
	}{
		{
			name:     "map value",
			entry:    typ.NewMap(typ.String, typ.NewArray(entry)),
			contract: typ.NewMap(typ.String, typ.Any),
		},
		{
			name:     "array element",
			entry:    typ.NewArray(entry),
			contract: typ.NewArray(typ.Any),
		},
	}

	for _, tc := range cases {
		got := BodyEntryContractJoin(tc.entry, tc.contract)
		if !typ.TypeEquals(got, tc.entry) {
			t.Fatalf("%s: BodyEntryContractJoin(%v, %v) = %v, want %v", tc.name, tc.entry, tc.contract, got, tc.entry)
		}
	}
}

func TestBodyContractJoin_ContractDominatesCompatibleCallShape(t *testing.T) {
	contract := typ.NewRecord().
		OptField("headers", typ.NewMap(typ.String, typ.String)).
		OptField("body", typ.String).
		OptField("stream", typ.Boolean).
		Build()
	callShape := typ.NewRecord().
		Field("headers", typ.NewMap(typ.String, typ.String)).
		Field("stream", typ.True).
		Build()

	if got := BodyContractJoin(callShape, contract); !typ.TypeEquals(got, contract) {
		t.Fatalf("BodyContractJoin(callShape, contract) = %v, want %v", got, contract)
	}
}

func TestMergeCallArgAt_JoinsTupleAndArrayAsSequence(t *testing.T) {
	node := typ.NewRecord().Field("node_id", typ.String).Build()
	got, changed := MergeCallArgAt(nil, 0, typ.NewTuple(node), typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected tuple call evidence")
	}
	got, changed = MergeCallArgAt(got, 0, typ.NewArray(node), typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected array call evidence to widen tuple")
	}
	want := typ.NewArray(node)
	if !typ.TypeEquals(got[0], want) {
		t.Fatalf("expected tuple and array calls to canonicalize as %v, got %v", want, got[0])
	}
}

func TestMergeCallArgAt_ConcreteArrayReplacesSoftTupleElement(t *testing.T) {
	node := typ.NewRecord().Field("node_id", typ.String).Build()
	got, changed := MergeCallArgAt(nil, 0, typ.NewTuple(typ.Any), typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected tuple call evidence")
	}
	got, changed = MergeCallArgAt(got, 0, typ.NewArray(node), typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected concrete array call evidence to refine soft tuple")
	}
	want := typ.NewArray(node)
	if !typ.TypeEquals(got[0], want) {
		t.Fatalf("expected concrete array evidence %v, got %v", want, got[0])
	}
}

func TestMergeCallArgAt_MissingRecordFieldsStayOptionalAcrossCalls(t *testing.T) {
	failRecord := typ.NewRecord().Field("fail", typ.Boolean).Build()
	got, changed := MergeCallArgAt(nil, 0, typ.NewTuple(typ.NewRecord().Build()), typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected empty record tuple evidence")
	}
	got, changed = MergeCallArgAt(got, 0, typ.NewArray(failRecord), typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected array evidence to merge with tuple")
	}

	want := typ.NewArray(typ.NewRecord().OptField("fail", typ.Boolean).Build())
	if !typ.TypeEquals(got[0], want) {
		t.Fatalf("expected missing field to remain optional at call boundary as %v, got %v", want, got[0])
	}
}

func TestMergeCallArgAt_RecordFieldsUseParameterEvidenceJoin(t *testing.T) {
	left := typ.NewArray(typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Unknown).
		Field("trigger_reason", typ.Unknown).
		Build())
	right := typ.NewArray(typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Any).
		Field("trigger_reason", typ.LiteralString("input_ready")).
		Build())

	got, changed := MergeCallArgAt(nil, 0, left, typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected first array evidence")
	}
	got, changed = MergeCallArgAt(got, 0, right, typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected second array evidence to refine record fields")
	}

	want := typ.NewArray(typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Any).
		Field("trigger_reason", typ.String).
		Build())
	if !typ.TypeEquals(got[0], want) {
		t.Fatalf("expected parameter-domain record join %v, got %v", want, got[0])
	}
}

func TestPublicContractJoin_PreciseCollectionRefinesOpenTableObligation(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		OptField("meta", typ.NewMap(typ.String, typ.Any)).
		Build()
	openTable := typ.NewRecord().SetOpen(true).Build()
	entries := typ.NewArray(entry)

	if got := PublicContractJoin(openTable, entries); !typ.TypeEquals(got, entries) {
		t.Fatalf("PublicContractJoin(open table, entries) = %v, want %v", got, entries)
	}
	if got := PublicContractJoin(entries, openTable); !typ.TypeEquals(got, entries) {
		t.Fatalf("PublicContractJoin(entries, open table) = %v, want %v", got, entries)
	}
}

func TestMergeBodyCallArgAt_PreservesStructuralDiscriminants(t *testing.T) {
	functionResult := typ.NewRecord().
		Field("role", typ.LiteralString("function_result")).
		Field("function_call_id", typ.LiteralString("tool")).
		Field("content", typ.LiteralString("ok")).
		Build()
	developer := typ.NewRecord().
		Field("role", typ.LiteralString("developer")).
		Field("content", typ.LiteralString("merge")).
		Build()
	observed := typ.NewTuple(functionResult, developer)

	body, changed := MergeBodyCallArgAt(nil, 0, observed, typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected body-effective evidence")
	}
	if !typ.TypeEquals(body[0], observed) {
		t.Fatalf("expected body evidence to preserve structural literals as %v, got %v", observed, body[0])
	}

	public, changed := MergeCallArgAt(nil, 0, observed, typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatal("expected public call-boundary evidence")
	}
	widened := typ.NewTuple(
		typ.NewRecord().
			Field("role", typ.String).
			Field("function_call_id", typ.String).
			Field("content", typ.String).
			Build(),
		typ.NewRecord().
			Field("role", typ.String).
			Field("content", typ.String).
			Build(),
	)
	if !typ.TypeEquals(public[0], widened) {
		t.Fatalf("expected public evidence to widen literal constants as %v, got %v", widened, public[0])
	}
}

func TestNormalizeBodyType_WidensMutableContainerLiterals(t *testing.T) {
	input := typ.NewMap(typ.String, typ.NewArray(typ.LiteralInt(1)))
	got := NormalizeBodyType(input)
	var want typ.Type = typ.NewMap(typ.String, typ.NewArray(typ.Integer))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected body evidence to widen mutable container literals as %v, got %v", want, got)
	}

	discriminated := typ.NewRecord().
		Field("kind", typ.LiteralString("tool_call")).
		Field("temperature", typ.LiteralNumber(0.8)).
		Field("items", typ.NewArray(typ.LiteralInt(1))).
		Build()
	got = NormalizeBodyType(discriminated)
	want = typ.NewRecord().
		Field("kind", typ.LiteralString("tool_call")).
		Field("temperature", typ.LiteralNumber(0.8)).
		Field("items", typ.NewArray(typ.Integer)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected record discriminants preserved and container values widened as %v, got %v", want, got)
	}
}

func TestMergeCallArgAt_DisjointPartialRecordsBecomeOptionalFields(t *testing.T) {
	var evidence []typ.Type
	var changed bool
	for _, observed := range []typ.Type{
		typ.NewRecord().Field("promptTokenCount", typ.Integer).Build(),
		typ.NewRecord().Field("candidatesTokenCount", typ.Integer).Build(),
		typ.NewRecord().Field("thoughtsTokenCount", typ.Integer).Build(),
	} {
		evidence, changed = MergeCallArgAt(evidence, 0, observed, typ.JoinPreferNonSoft, false)
		if !changed {
			t.Fatalf("expected evidence change for %v", observed)
		}
	}

	want := typ.NewRecord().
		OptField("candidatesTokenCount", typ.Integer).
		OptField("promptTokenCount", typ.Integer).
		OptField("thoughtsTokenCount", typ.Integer).
		Build()
	if !typ.TypeEquals(evidence[0], want) {
		t.Fatalf("expected partial records to join as %v, got %v", want, evidence[0])
	}
}

func TestMergeCallArgAt_NilableStructuralUnionIsIdempotent(t *testing.T) {
	yieldPayload := typ.NewRecord().
		Field("parent_id", typ.Any).
		Field("reply_to", typ.Any).
		Field("results", typ.Any).
		Field("yield_id", typ.Any).
		Build()
	messagePayload := typ.NewRecord().
		Field("message", typ.String).
		Build()
	mergedPayload := typ.NewRecord().
		OptField("message", typ.String).
		OptField("parent_id", typ.Any).
		OptField("reply_to", typ.Any).
		OptField("results", typ.Any).
		OptField("yield_id", typ.Any).
		Build()

	evidence := []typ.Type{typ.NewUnion(typ.Nil, yieldPayload, mergedPayload, messagePayload)}
	var changed bool
	evidence, changed = MergeCallArgAt(evidence, 0, yieldPayload, typ.JoinPreferNonSoft, false)
	if !changed {
		t.Fatalf("expected noncanonical union seed to collapse")
	}
	evidence, changed = MergeCallArgAt(evidence, 0, messagePayload, typ.JoinPreferNonSoft, false)
	if changed {
		t.Fatalf("expected canonical equivalent message payload not to change evidence, got %v", evidence[0])
	}
	evidence, changed = MergeCallArgAt(evidence, 0, yieldPayload, typ.JoinPreferNonSoft, false)
	if changed {
		t.Fatalf("expected canonical equivalent yield payload not to change evidence, got %v", evidence[0])
	}

	want := typ.NewOptional(mergedPayload)
	if !typ.TypeEquals(evidence[0], want) {
		t.Fatalf("expected nilable payload union to canonicalize as %v, got %v", want, evidence[0])
	}
}

func TestNormalizeType_CollapsesSequenceUnion(t *testing.T) {
	yieldNode := typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Unknown).
		Field("parent_id", typ.Any).
		Field("path", typ.Any).
		Field("trigger_reason", typ.LiteralString("yield_driven")).
		Build()
	inputNode := typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Any).
		Field("path", typ.NewRecord().SetOpen(true).Build()).
		Field("trigger_reason", typ.LiteralString("input_ready")).
		Build()

	got := NormalizeType(typ.NewUnion(
		typ.Nil,
		typ.NewTuple(yieldNode),
		typ.NewArray(inputNode),
	))
	want := typ.NewOptional(typ.NewArray(typ.NewRecord().
		Field("node_id", typ.Any).
		Field("node_type", typ.Any).
		OptField("parent_id", typ.Any).
		Field("path", typ.Any).
		Field("trigger_reason", typ.String).
		Build()))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected sequence union to collapse to %v, got %v", want, got)
	}
}

func TestJoin_NilableTupleAndArrayBecomeNilableSequence(t *testing.T) {
	node := typ.NewRecord().Field("node_id", typ.String).Build()
	got := Join(typ.NewOptional(typ.NewTuple(node)), typ.NewArray(node))
	want := typ.NewOptional(typ.NewArray(node))
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected nilable tuple plus array to produce %v, got %v", want, got)
	}
}

func TestConvergeContractJoin_StructurallyIncompatibleEvidenceStaysBounded(t *testing.T) {
	mig := typ.NewInterface("migration.Transaction", []typ.Method{
		{Name: "query", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})
	db := typ.NewInterface("sql.DB", []typ.Method{
		{Name: "type", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	tx := typ.NewInterface("sql.Transaction", []typ.Method{
		{Name: "commit", Type: typ.Func().Param("self", typ.Self).Returns(typ.Boolean).Build()},
	})
	contract := typ.NewUnion(db, tx)

	// Repeatedly meeting a structurally-incompatible observation against the
	// converged contract must reach a fixpoint instead of forming an
	// ever-growing intersection of intersections.
	state := contract
	prev := state
	for i := 0; i < 8; i++ {
		state = ConvergeContractJoin(state, mig)
		if i > 0 && !typ.TypeEquals(state, prev) {
			t.Fatalf("Params convergence not bounded at iteration %d: %v -> %v", i, prev, state)
		}
		prev = state
	}
	// The widened contract must still admit the original contract values.
	if !subtype.IsSubtype(contract, state) {
		t.Fatalf("widened contract %v dropped the original obligation %v", state, contract)
	}
}

func TestConvergeContractJoin_NeverSeedYieldsToConcreteEvidence(t *testing.T) {
	concreteElem := typ.NewRecord().Field("node_id", typ.Any).Build()
	concrete := typ.NewArray(concreteElem)

	// never[]? is the empty-array "no evidence yet" seed; concrete call evidence
	// must replace it rather than being absorbed by the never-collapsing
	// intersection.
	seed := typ.NewOptional(typ.NewArray(typ.Never))
	if got := ConvergeContractJoin(seed, concrete); !typ.TypeEquals(got, concrete) {
		t.Fatalf("ConvergeContractJoin(never[]?, concrete) = %v, want %v", got, concrete)
	}
	if got := ConvergeContractJoin(concrete, seed); !typ.TypeEquals(got, concrete) {
		t.Fatalf("ConvergeContractJoin(concrete, never[]?) = %v, want %v", got, concrete)
	}

	// A bare-never seed yields the same way.
	if got := ConvergeContractJoin(typ.Never, concrete); !typ.TypeEquals(got, concrete) {
		t.Fatalf("ConvergeContractJoin(never, concrete) = %v, want %v", got, concrete)
	}
}

func TestBodyContractJoin_IncompatibleEntryRefinesMemberWiseAndDropsSeedNil(t *testing.T) {
	mig := typ.NewInterface("migration.Transaction", []typ.Method{
		{Name: "query", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})
	db := typ.NewInterface("sql.DB", []typ.Method{
		{Name: "type", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	tx := typ.NewInterface("sql.Transaction", []typ.Method{
		{Name: "commit", Type: typ.Func().Param("self", typ.Self).Returns(typ.Boolean).Build()},
	})
	contract := typ.NewUnion(db, tx)

	// An optional entry observation refined by a non-nilable hard contract must
	// drop the seed nil (the contract is the body's non-nil precondition) and
	// stay bounded under repeated refinement rather than cross-distributing into
	// a growing intersection.
	entry := typ.NewOptional(mig)
	first := BodyContractJoin(entry, contract)
	if hasNilMember(first) {
		t.Fatalf("non-nilable contract should refine away seed nil, got nilable %v", first)
	}
	if !subtype.IsSubtype(first, contract) {
		t.Fatalf("refined entry %v must satisfy the contract %v", first, contract)
	}
	firstCount := unionMemberCount(first)
	state := first
	for i := 0; i < 6; i++ {
		state = BodyContractJoin(state, contract)
		if unionMemberCount(state) > firstCount {
			t.Fatalf("BodyContractJoin grew unbounded at iteration %d: %d -> %d members (%v)", i, firstCount, unionMemberCount(state), state)
		}
	}
}

func TestEntryContradictsBodyContract(t *testing.T) {
	record := typ.NewRecord().Field("name", typ.String).Build()
	if !EntryContradictsBodyContract(record, typ.Number) {
		t.Fatalf("record entry should contradict numeric body contract")
	}
	if EntryContradictsBodyContract(typ.Integer, typ.Number) {
		t.Fatalf("integer entry should be compatible with numeric body contract")
	}
	if EntryContradictsBodyContract(typ.Unknown, typ.Number) {
		t.Fatalf("unknown entry is not precise enough to contradict a body contract")
	}
	if EntryContradictsBodyContract(typ.NewOptional(typ.Integer), typ.Number) {
		t.Fatalf("integer? entry has a non-nil numeric member compatible with number")
	}
}

func hasNilMember(t typ.Type) bool {
	if _, ok := typ.UnwrapAnnotated(t).(*typ.Optional); ok {
		return true
	}
	if u, ok := typ.UnwrapAnnotated(t).(*typ.Union); ok {
		for _, m := range u.Members {
			if typ.UnwrapAnnotated(m).Kind() == kind.Nil {
				return true
			}
		}
	}
	return typ.UnwrapAnnotated(t).Kind() == kind.Nil
}

func unionMemberCount(t typ.Type) int {
	if u, ok := typ.UnwrapAnnotated(t).(*typ.Union); ok {
		return len(u.Members)
	}
	return 1
}
