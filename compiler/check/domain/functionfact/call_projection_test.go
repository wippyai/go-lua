package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProjectCallContract_DoesNotPromotePublicParamEvidenceIntoCallableShape(t *testing.T) {
	payload := typ.NewRecord().Field("parent_id", typ.String).Build()
	callee := typ.Func().OptParam("payload", typ.Any).Returns(typ.Boolean).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"payload"}}}

	got := projectCallContract(callContractInput{
		Fact:   api.FunctionFact{Signature: callee, Params: product.LiftVector([]typ.Type{typ.Any})},
		Sym:    cfg.SymbolID(1),
		Source: source,
		Args:   []typ.Type{payload},
	})
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected function projection, got %T", got)
	}
	if !typ.TypeEquals(fn.Params[0].Type, typ.Any) {
		t.Fatalf("parameter evidence rewrote callable shape to %v, want source any", fn.Params[0].Type)
	}
	if !fn.Params[0].Optional {
		t.Fatalf("parameter evidence made optional source parameter required: %#v", fn.Params[0])
	}
}

func TestProjectCallContract_DoesNotUseBodyEvidenceAsPreciseCallContext(t *testing.T) {
	payload := typ.NewRecord().Field("parent_id", typ.String).Build()
	callee := typ.Func().OptParam("payload", typ.Any).Returns(typ.Boolean).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"payload"}}}

	got := projectCallContract(callContractInput{
		Fact:   api.FunctionFact{Signature: callee, Params: product.LiftVector([]typ.Type{payload})},
		Sym:    cfg.SymbolID(1),
		Source: source,
		Args:   []typ.Type{payload},
	})
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected function projection, got %T", got)
	}
	if !typ.TypeEquals(fn.Params[0].Type, typ.Any) {
		t.Fatalf("body/public evidence leaked into callable shape: got %v, want any", fn.Params[0].Type)
	}
}

func TestProjectObservedDynamicCallParams_DoesNotRewriteAnnotatedParam(t *testing.T) {
	payload := typ.NewRecord().Field("parent_id", typ.String).Build()
	callee := typ.Func().Param("payload", payload).Returns(typ.Boolean).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{
		Names: []string{"payload"},
		Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "Payload"}},
	}}

	got := projectCallContract(callContractInput{
		Fact:   api.FunctionFact{Signature: callee, Params: product.LiftVector([]typ.Type{typ.Any})},
		Sym:    cfg.SymbolID(1),
		Source: source,
		Args:   []typ.Type{typ.Any},
	})
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected function projection, got %T", got)
	}
	if !typ.TypeEquals(fn.Params[0].Type, payload) {
		t.Fatalf("expected annotated parameter to remain structural, got %v", fn.Params[0].Type)
	}
}

func TestProjectCallContract_KeepsSourceScalarSignature(t *testing.T) {
	callee := typ.Func().Param("name", typ.String).Returns(typ.String).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"name"}}}

	got := projectCallContract(callContractInput{
		Fact:   api.FunctionFact{Signature: callee, Params: product.LiftVector([]typ.Type{typ.Any})},
		Sym:    cfg.SymbolID(1),
		Source: source,
		Args:   []typ.Type{typ.Any},
	})
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected function projection, got %T", got)
	}
	if !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		t.Fatalf("expected source scalar signature to remain string, got %v", fn.Params[0].Type)
	}
}

func TestProjectCallContract_PublicAnyEvidenceDoesNotWeakenSourceScalarSignature(t *testing.T) {
	callee := typ.Func().Param("name", typ.String).Returns(typ.String).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"name"}}}

	got := projectCallContract(callContractInput{
		Fact:   api.FunctionFact{Signature: callee, Params: product.LiftVector([]typ.Type{typ.Any})},
		Sym:    cfg.SymbolID(1),
		Source: source,
		Args:   []typ.Type{typ.Any},
	})
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected function projection, got %T", got)
	}
	if !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		t.Fatalf("expected public any evidence not to weaken source signature, got %v", fn.Params[0].Type)
	}
}

func TestProjectCallContract_DoesNotWidenStructuralCallableShapeFromParamEvidence(t *testing.T) {
	node := typ.NewRecord().Field("node_id", typ.String).Build()
	tupleParam := typ.NewTuple(node)
	arrayParam := typ.NewArray(node)
	callee := typ.Func().Param("nodes", tupleParam).Returns(typ.String).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"nodes"}}}

	got := projectCallContract(callContractInput{
		Fact:   api.FunctionFact{Signature: callee, Params: product.LiftVector([]typ.Type{arrayParam})},
		Sym:    cfg.SymbolID(1),
		Source: source,
		Args:   []typ.Type{arrayParam},
	})
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 {
		t.Fatalf("expected function projection, got %T", got)
	}
	if !typ.TypeEquals(fn.Params[0].Type, tupleParam) {
		t.Fatalf("parameter evidence widened callable shape to %v, want source tuple %v", fn.Params[0].Type, tupleParam)
	}
}

func TestSelectCallProjectionSourceLocalPreservesOptionalCurrentCallee(t *testing.T) {
	current := typ.NewOptional(typ.Func().Returns(typ.Unknown).Build())
	fact := typ.Func().Returns(typ.String).Build()

	got := selectCallProjection(current, fact, nil, true)
	inner, optional := typ.SplitNilableFieldType(got)
	if !optional {
		t.Fatalf("selectCallProjection = %v, want optional fact projection", got)
	}
	fn, ok := inner.(*typ.Function)
	if !ok {
		t.Fatalf("selectCallProjection inner = %T, want function", inner)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want [string]", fn.Returns)
	}
}

func TestSelectCallProjectionSourceLocalKeepsDefiniteCurrentCalleePrecise(t *testing.T) {
	current := typ.Func().Returns(typ.Unknown).Build()
	fact := typ.Func().Returns(typ.String).Build()

	got := selectCallProjection(current, fact, nil, true)
	fn, ok := got.(*typ.Function)
	if !ok {
		t.Fatalf("selectCallProjection = %T, want function", got)
	}
	if _, optional := typ.SplitNilableFieldType(got); optional {
		t.Fatalf("selectCallProjection = %v, want definite function", got)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want [string]", fn.Returns)
	}
}

func TestProjectCallFactType_UsesPublicProjectionForSourceLocalCall(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	sym := cfg.SymbolID(42)
	ff := api.FunctionFact{
		Signature: typ.Func().Param("tests", typ.NewArray(typ.Any)).Build(),
		EntryParams: product.LiftVector([]typ.Type{
			typ.NewArray(entry),
		}),
	}

	local := unwrapFunctionForCallProjectionTest(t, projectCallFactType(ff, sym))
	if !typ.TypeEquals(local.Params[0].Type, typ.NewArray(typ.Any)) {
		t.Fatalf("source-local call projection = %v, want public signature", local.Params[0].Type)
	}

	boundary := unwrapFunctionForCallProjectionTest(t, projectCallFactType(ff, sym))
	if !typ.TypeEquals(boundary.Params[0].Type, typ.NewArray(typ.Any)) {
		t.Fatalf("boundary call projection = %v, want public signature", boundary.Params[0].Type)
	}
}

func TestSelectCallProjection_SourceLocalFactProjectionIsAuthoritative(t *testing.T) {
	bodyOnly := typ.NewRecord().
		Field("role", typ.LiteralString("function_call")).
		Field("function_call", typ.NewRecord().Field("id", typ.String).Build()).
		Build()
	entry := typ.NewRecord().
		Field("role", typ.String).
		Field("function_call_id", typ.String).
		Build()
	current := typ.Func().Param("messages", typ.NewArray(bodyOnly)).Build()
	sibling := typ.Func().Param("messages", typ.NewTuple(entry)).Build()

	got := unwrapFunctionForCallProjectionTest(t, selectCallProjection(current, sibling, []typ.Type{typ.NewTuple(entry)}, true))
	if !typ.TypeEquals(got.Params[0].Type, typ.NewTuple(entry)) {
		t.Fatalf("source-local projection kept body-only parameter %v, want sibling %v", got.Params[0].Type, typ.NewTuple(entry))
	}
}

func TestProjectCallContract_PublicBoundaryDoesNotAdmitUnobservedDynamicAnnotatedParam(t *testing.T) {
	callee := typ.Func().Param("session_id", typ.String).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{
		Names: []string{"session_id"},
		Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
	}}

	got := projectCallContract(callContractInput{
		Fact:             api.FunctionFact{Signature: callee},
		Sym:              cfg.SymbolID(1),
		Source:           source,
		Args:             []typ.Type{typ.Any},
		UnobservedParams: []bool{true},
	})
	fn := unwrapFunctionForCallProjectionTest(t, got)
	if !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		t.Fatalf("public annotated unobserved parameter was weakened to %v, want string", fn.Params[0].Type)
	}
}

func TestProjectCallContract_AdmitsDynamicTopForUnobservedLocalAnnotatedParam(t *testing.T) {
	callee := typ.Func().Param("session_id", typ.String).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{
		Names: []string{"session_id"},
		Types: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "string"}},
	}}

	got := projectCallContract(callContractInput{
		Fact:             api.FunctionFact{Signature: callee},
		Sym:              cfg.SymbolID(1),
		Source:           source,
		Args:             []typ.Type{typ.Any},
		ClosedWorldLocal: true,
		UnobservedParams: []bool{true},
	})
	fn := unwrapFunctionForCallProjectionTest(t, got)
	if !typ.TypeEquals(fn.Params[0].Type, typ.Any) {
		t.Fatalf("local unobserved dynamic parameter = %v, want any", fn.Params[0].Type)
	}
}

func TestProjectCallContract_AdmitsDynamicTopForUnobservedLocalUnannotatedParam(t *testing.T) {
	callee := typ.Func().Param("session_id", typ.String).Build()
	source := &ast.FunctionExpr{ParList: &ast.ParList{
		Names: []string{"session_id"},
	}}

	got := projectCallContract(callContractInput{
		Fact:             api.FunctionFact{Signature: callee},
		Sym:              cfg.SymbolID(1),
		Source:           source,
		Args:             []typ.Type{typ.Any},
		ClosedWorldLocal: true,
		UnobservedParams: []bool{true},
	})
	fn := unwrapFunctionForCallProjectionTest(t, got)
	if !typ.TypeEquals(fn.Params[0].Type, typ.Any) {
		t.Fatalf("unannotated unobserved parameter = %v, want any", fn.Params[0].Type)
	}
}

func TestHasWiderParams_SequenceFactReplacesTupleProjection(t *testing.T) {
	node := typ.NewRecord().Field("node_id", typ.String).Build()
	current := typ.Func().OptParam("nodes", typ.NewTuple(node)).Build()
	fact := typ.Func().OptParam("nodes", typ.NewArray(node)).Build()

	if !hasWiderParams(current, fact) {
		t.Fatal("expected array sequence fact to replace tuple-shaped projection")
	}
}

func TestHasWiderParams_ConcreteFactDoesNotReplaceDynamicParam(t *testing.T) {
	payload := typ.NewRecord().Field("fail", typ.Boolean).Build()
	current := typ.Func().OptParam("tests", typ.Any).Build()
	fact := typ.Func().OptParam("tests", typ.NewArray(payload)).Build()

	if hasWiderParams(current, fact) {
		t.Fatal("concrete fact must not replace dynamic parameter projection")
	}
}

func unwrapFunctionForCallProjectionTest(t *testing.T, got typ.Type) *typ.Function {
	t.Helper()
	fn, ok := got.(*typ.Function)
	if !ok || fn == nil || len(fn.Params) == 0 {
		t.Fatalf("expected function projection, got %T", got)
	}
	return fn
}
