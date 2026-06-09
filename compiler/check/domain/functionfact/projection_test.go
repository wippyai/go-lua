package functionfact_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func projectionFunctionFact(opts ...func(*api.FunctionFact)) api.FunctionFact {
	var ff api.FunctionFact
	for _, opt := range opts {
		if opt != nil {
			opt(&ff)
		}
	}
	return ff
}

func projectionFactSignature(sig *typ.Function) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Public.Signature = sig
	}
}

func projectionFactCallParams(params ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Call.Params = product.LiftVector(params)
	}
}

func projectionFactBodyParams(params ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Body.Params = product.LiftVector(params)
	}
}

func projectionFactEntryParams(params ...typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Entry.Params = product.LiftVector(params)
	}
}

func projectionFactReturns(preflow, postflow []typ.Type) func(*api.FunctionFact) {
	return func(ff *api.FunctionFact) {
		ff.Returns.Preflow = product.LiftVector(preflow)
		ff.Returns.Postflow = product.LiftVector(postflow)
	}
}

func TestStoreProjectionGraphSymbol_UsesCanonicalParent(t *testing.T) {
	st := store.NewSessionStore()
	fn := &ast.FunctionExpr{}
	graph := cfg.Build(fn)
	st.RegisterGraph(graph, fn)

	storedParent := scope.New().WithType("stored_parent", typ.String)
	defaultParent := scope.New().WithType("default_parent", typ.Number)
	registerGraphParent(t, st, graph, storedParent)

	sym := cfg.SymbolID(7)
	first := typ.Func().Returns(typ.String).Build()
	second := typ.Func().Returns(typ.Number).Build()
	writeFunctionFactType(st, graph, storedParent, sym, first)

	view := functionfact.StoreProjection(st, defaultParent)
	sv, ok := view.GraphSymbol(graph, sym, api.SynthModeDeclared)
	if !ok {
		t.Fatal("GraphSymbol() did not resolve")
	}
	if got := sv.Type(functionfact.ProjectionSibling, api.SynthModeDeclared); !typ.TypeEquals(got, first) {
		t.Fatalf("GraphSymbol().Type() = %v, want %v", got, first)
	}

	writeFunctionFactType(st, graph, storedParent, sym, second)
	sv, ok = view.GraphSymbol(graph, sym, api.SynthModeDeclared)
	if !ok {
		t.Fatal("GraphSymbol() after update did not resolve")
	}
	if got := sv.Type(functionfact.ProjectionSibling, api.SynthModeDeclared); !typ.TypeEquals(got, second) {
		t.Fatalf("GraphSymbol().Type() after update = %v, want %v", got, second)
	}
}

func TestStoreProjectionSymbol_ResolvesOwningParentGraph(t *testing.T) {
	st := store.NewSessionStore()
	parentFn := &ast.FunctionExpr{}
	childFn := &ast.FunctionExpr{}
	parentGraph := cfg.Build(parentFn)
	childGraph := cfg.Build(childFn)
	st.RegisterGraph(parentGraph, parentFn)
	st.RegisterGraph(childGraph, childFn)

	parent := scope.New().WithType("parent", typ.String)
	registerGraphParent(t, st, parentGraph, parent)

	sym := cfg.SymbolID(11)
	fnType := typ.Func().Returns(typ.Boolean).Build()
	st.RegisterFunctionRef(sym, childFn, childGraph, parentGraph.ID(), 0)
	writeFunctionFactType(st, parentGraph, parent, sym, fnType)

	sv, ok := functionfact.StoreProjection(st, nil).Symbol(sym, api.SynthModeDeclared)
	if !ok {
		t.Fatal("Symbol() did not resolve")
	}
	if got := sv.Type(functionfact.ProjectionSibling, api.SynthModeDeclared); !typ.TypeEquals(got, fnType) {
		t.Fatalf("Symbol().Type() = %v, want %v", got, fnType)
	}
	if sv.Key.GraphID != parentGraph.ID() || sv.Key.ParentHash != parent.Hash() {
		t.Fatalf("Symbol().Key = %#v, want graph %d parent %d", sv.Key, parentGraph.ID(), parent.Hash())
	}
}

func TestStoreProjectionOwner_ResolvesBeforeFactsExist(t *testing.T) {
	st := store.NewSessionStore()
	parentFn := &ast.FunctionExpr{}
	childFn := &ast.FunctionExpr{}
	parentGraph := cfg.Build(parentFn)
	childGraph := cfg.Build(childFn)
	st.RegisterGraph(parentGraph, parentFn)
	st.RegisterGraph(childGraph, childFn)

	parent := scope.New().WithType("parent", typ.String)
	registerGraphParent(t, st, parentGraph, parent)

	sym := cfg.SymbolID(12)
	st.RegisterFunctionRef(sym, childFn, childGraph, parentGraph.ID(), 0)

	owner, ok := functionfact.StoreProjection(st, nil).Owner(sym)
	if !ok {
		t.Fatal("Owner() did not resolve empty product owner")
	}
	if owner.Key.GraphID != parentGraph.ID() || owner.Key.ParentHash != parent.Hash() {
		t.Fatalf("Owner().Key = %#v, want graph %d parent %d", owner.Key, parentGraph.ID(), parent.Hash())
	}
	if sv, ok := functionfact.StoreProjection(st, nil).Symbol(sym, api.SynthModeDeclared); ok {
		t.Fatalf("Symbol() resolved empty fact product: %#v", sv)
	}
}

func TestFactsProjectionReturns_SelectsNarrowingProjection(t *testing.T) {
	facts := api.FunctionFacts{
		1: projectionFunctionFact(projectionFactReturns([]typ.Type{typ.Nil}, []typ.Type{typ.String})),
	}
	view := functionfact.FactsProjection(facts)

	if got := view.Returns(1, api.SynthModeDeclared); len(got) != 1 || !typ.TypeEquals(got[0], typ.Nil) {
		t.Fatalf("scope returns = %v, want nil summary", got)
	}
	if got := view.Returns(1, api.SynthModeFlow); len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("narrow returns = %v, want string narrow summary", got)
	}
}

func TestFactsProjectionReturns_NarrowingRepairsWithoutDroppingSummaryTop(t *testing.T) {
	sym := cfg.SymbolID(1)
	dynamic := []typ.Type{typ.Any}
	narrow := []typ.Type{typ.NewRecord().Field("data", typ.String).Build()}
	facts := api.FunctionFacts{
		sym: projectionFunctionFact(
			projectionFactSignature(typ.Func().Returns(typ.Any).Build()),
			projectionFactReturns(dynamic, narrow),
		),
	}

	if got := functionfact.FactsProjection(facts).Returns(sym, api.SynthModeFlow); len(got) != 1 || !typ.TypeEquals(got[0], typ.Any) {
		t.Fatalf("narrowing returns = %v, want whole-slot any summary", got)
	}

	projected := functionfact.ProjectType(facts[sym], functionfact.ProjectionExport, api.SynthModeFlow)
	fn, ok := projected.(*typ.Function)
	if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.Any) {
		t.Fatalf("export projection = %v, want any return", projected)
	}
}

func TestFactsProjectionTypeLookup_ProjectsCanonicalFunctionTypes(t *testing.T) {
	sym := cfg.SymbolID(3)
	fnType := typ.Func().Returns(typ.String).Build()
	facts := api.FunctionFacts{
		sym: projectionFunctionFact(projectionFactSignature(fnType)),
		4:   projectionFunctionFact(projectionFactCallParams(typ.String)),
	}

	lookup := functionfact.FactsProjection(facts).TypeLookup(functionfact.ProjectionSibling, api.SynthModeDeclared)
	if lookup == nil {
		t.Fatal("TypeLookup() returned nil")
	}
	if got := lookup(sym); !typ.TypeEquals(got, fnType) {
		t.Fatalf("lookup(%d) = %v, want %v", sym, got, fnType)
	}
	if got := lookup(4); got != nil {
		t.Fatalf("lookup for param-only fact = %v, want nil", got)
	}
}

func TestStoreProjectionTypeLookup_UsesOwningFunctionFactProduct(t *testing.T) {
	st := store.NewSessionStore()
	parentFn := &ast.FunctionExpr{}
	childFn := &ast.FunctionExpr{}
	parentGraph := cfg.Build(parentFn)
	childGraph := cfg.Build(childFn)
	st.RegisterGraph(parentGraph, parentFn)
	st.RegisterGraph(childGraph, childFn)

	parent := scope.New().WithType("parent", typ.String)
	registerGraphParent(t, st, parentGraph, parent)

	sym := cfg.SymbolID(13)
	fnType := typ.Func().Returns(typ.Boolean).Build()
	st.RegisterFunctionRef(sym, childFn, childGraph, parentGraph.ID(), 0)
	writeFunctionFactType(st, parentGraph, parent, sym, fnType)

	lookup := functionfact.StoreProjection(st, nil).TypeLookup(functionfact.ProjectionSibling, api.SynthModeDeclared)
	if lookup == nil {
		t.Fatal("TypeLookup() returned nil")
	}
	if got := lookup(sym); !typ.TypeEquals(got, fnType) {
		t.Fatalf("lookup(%d) = %v, want %v", sym, got, fnType)
	}
	if got := lookup(0); got != nil {
		t.Fatalf("lookup(0) = %v, want nil", got)
	}
}

func TestRecursiveTypeProjection_ReadsCurrentReturnProduct(t *testing.T) {
	sym := cfg.SymbolID(7)
	sig := typ.Func().Param("x", typ.String).Build()
	facts := api.FunctionFacts{
		sym: projectionFunctionFact(
			projectionFactSignature(sig),
			projectionFactReturns([]typ.Type{typ.Integer}, nil),
		),
	}

	got := functionfact.RecursiveTypeProjection(sig, nil, facts, sym, api.SynthModeDeclared)
	if got == nil || len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.Integer) {
		t.Fatalf("RecursiveTypeProjection() = %v, want integer return from product", got)
	}
}

func TestRecursiveTypeProjection_UsesExpectedReturnsWithoutProduct(t *testing.T) {
	sig := typ.Func().Param("x", typ.String).Build()
	expected := typ.Func().Param("x", typ.String).Returns(typ.Boolean).Build()

	got := functionfact.RecursiveTypeProjection(sig, expected, nil, 0, api.SynthModeDeclared)
	if got == nil || len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.Boolean) {
		t.Fatalf("RecursiveTypeProjection() = %v, want expected boolean return", got)
	}
}

func TestSignatureWithReturnSummary_AppliesProductReturnProjection(t *testing.T) {
	sym := cfg.SymbolID(9)
	sig := typ.Func().Returns(typ.Unknown).Build()
	facts := api.FunctionFacts{
		sym: projectionFunctionFact(
			projectionFactSignature(sig),
			projectionFactReturns([]typ.Type{typ.String}, nil),
		),
	}

	got := functionfact.SignatureWithReturnSummary(facts, sym, sig)
	if got == nil || len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.String) {
		t.Fatalf("SignatureWithReturnSummary() = %v, want string return", got)
	}
}

func TestProjectTypePreservesDeclaredReturnsOverBodySummary(t *testing.T) {
	declared := typ.NewRecord().Field("ok", typ.LiteralBool(true)).Field("value", typ.String).Build()
	body := typ.NewRecord().Field("ok", typ.LiteralBool(true)).Field("value", typ.Integer).Build()
	ff := projectionFunctionFact(
		projectionFactSignature(typ.Func().Returns(declared).Build()),
		projectionFactReturns([]typ.Type{body}, []typ.Type{body}),
	)

	for _, mode := range []api.SynthMode{api.SynthModeDeclared, api.SynthModeFlow} {
		projected := functionfact.ProjectType(ff, functionfact.ProjectionSibling, mode)
		fn, ok := projected.(*typ.Function)
		if !ok || len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], declared) {
			t.Fatalf("ProjectType(%v) = %v, want declared return %v", mode, projected, declared)
		}
	}
}

func TestProjectionBodyDoesNotAssumeBodyContractAsEntryProof(t *testing.T) {
	sig := typ.Func().Param("value", typ.Any).Returns(typ.Boolean).Build()
	ff := projectionFunctionFact(
		projectionFactSignature(sig),
		projectionFactBodyParams(typ.String),
	)

	body := functionfact.ProjectType(ff, functionfact.ProjectionBody, api.SynthModeDeclared)
	bodyFn, ok := body.(*typ.Function)
	if !ok || len(bodyFn.Params) != 1 || !typ.TypeEquals(bodyFn.Params[0].Type, typ.Any) {
		t.Fatalf("body projection = %v, want source any parameter", body)
	}

	sibling := functionfact.ProjectType(ff, functionfact.ProjectionSibling, api.SynthModeDeclared)
	siblingFn, ok := sibling.(*typ.Function)
	if !ok || len(siblingFn.Params) != 1 || !typ.TypeEquals(siblingFn.Params[0].Type, typ.Any) {
		t.Fatalf("sibling projection = %v, want caller-facing any parameter", sibling)
	}
}

func TestProjectionSiblingUsesEntryEvidenceWithoutLeakingToPublic(t *testing.T) {
	entry := typ.NewArray(typ.NewRecord().Field("id", typ.String).Build())
	publicParam := typ.NewArray(typ.Any)
	ff := projectionFunctionFact(
		projectionFactSignature(typ.Func().Param("tests", publicParam).Build()),
		projectionFactEntryParams(entry),
	)

	sibling := functionfact.ProjectType(ff, functionfact.ProjectionSibling, api.SynthModeDeclared)
	siblingFn, ok := sibling.(*typ.Function)
	if !ok || len(siblingFn.Params) != 1 || !typ.TypeEquals(siblingFn.Params[0].Type, entry) {
		t.Fatalf("sibling projection = %v, want entry parameter %v", sibling, entry)
	}

	public := functionfact.ProjectType(ff, functionfact.ProjectionPublic, api.SynthModeDeclared)
	publicFn, ok := public.(*typ.Function)
	if !ok || len(publicFn.Params) != 1 || !typ.TypeEquals(publicFn.Params[0].Type, publicParam) {
		t.Fatalf("public projection = %v, want public parameter %v", public, publicParam)
	}

	exported := functionfact.ProjectType(ff, functionfact.ProjectionExport, api.SynthModeDeclared)
	exportFn, ok := exported.(*typ.Function)
	if !ok || len(exportFn.Params) != 1 || !typ.TypeEquals(exportFn.Params[0].Type, publicParam) {
		t.Fatalf("export projection = %v, want public parameter %v", exported, publicParam)
	}
}

func TestProjectionFlowInputExcludesEntryAndBodyEvidence(t *testing.T) {
	entry := typ.NewArray(typ.NewRecord().Field("id", typ.String).Build())
	body := typ.NewArray(typ.NewRecord().Field("id", typ.String).Field("name", typ.String).Build())
	publicParam := typ.NewArray(typ.Any)
	ff := projectionFunctionFact(
		projectionFactSignature(typ.Func().Param("tests", publicParam).Build()),
		projectionFactCallParams(publicParam),
		projectionFactBodyParams(body),
		projectionFactEntryParams(entry),
		projectionFactReturns([]typ.Type{typ.Boolean}, nil),
	)

	projected := functionfact.ProjectType(ff, functionfact.ProjectionFlowInput, api.SynthModeDeclared)
	fn, ok := projected.(*typ.Function)
	if !ok || len(fn.Params) != 1 || len(fn.Returns) != 1 {
		t.Fatalf("flow-input projection = %v, want one-param one-return function", projected)
	}
	if !typ.TypeEquals(fn.Params[0].Type, publicParam) {
		t.Fatalf("flow-input projection parameter = %v, want public %v", fn.Params[0].Type, publicParam)
	}
	if !typ.TypeEquals(fn.Returns[0], typ.Boolean) {
		t.Fatalf("flow-input projection return = %v, want summary boolean", fn.Returns[0])
	}
}

func TestFactsProjectionSynthesisTypeUsesFlowInputBeforeNarrowing(t *testing.T) {
	sym := cfg.SymbolID(21)
	entry := typ.NewRecord().Field("id", typ.String).Build()
	publicParam := typ.Any
	facts := api.FunctionFacts{
		sym: projectionFunctionFact(
			projectionFactSignature(typ.Func().Param("value", publicParam).Build()),
			projectionFactEntryParams(entry),
		),
	}

	scopeType := functionfact.FactsProjection(facts).SynthesisType(sym, api.SynthModeDeclared)
	scopeFn, ok := scopeType.(*typ.Function)
	if !ok || len(scopeFn.Params) != 1 || !typ.TypeEquals(scopeFn.Params[0].Type, publicParam) {
		t.Fatalf("scope synthesis projection = %v, want public parameter", scopeType)
	}

	narrowType := functionfact.FactsProjection(facts).SynthesisType(sym, api.SynthModeFlow)
	narrowFn, ok := narrowType.(*typ.Function)
	if !ok || len(narrowFn.Params) != 1 || !typ.TypeEquals(narrowFn.Params[0].Type, entry) {
		t.Fatalf("narrow synthesis projection = %v, want entry parameter", narrowType)
	}
}

func TestProjectionBodyPreservesNilEntryStateForUnannotatedParam(t *testing.T) {
	ff := projectionFunctionFact(
		projectionFactSignature(typ.Func().Param("base", typ.Any).Build()),
		projectionFactEntryParams(typ.Nil),
	)

	body := functionfact.ProjectType(ff, functionfact.ProjectionBody, api.SynthModeDeclared)
	bodyFn, ok := body.(*typ.Function)
	if !ok || len(bodyFn.Params) != 1 || !typ.TypeEquals(bodyFn.Params[0].Type, typ.Nil) {
		t.Fatalf("body projection = %v, want nil entry state", body)
	}

	public := functionfact.ProjectType(ff, functionfact.ProjectionPublic, api.SynthModeDeclared)
	publicFn, ok := public.(*typ.Function)
	if !ok || len(publicFn.Params) != 1 || !typ.TypeEquals(publicFn.Params[0].Type, typ.Any) {
		t.Fatalf("public projection = %v, want public any", public)
	}
}

func TestProjectionPublicKeepsExplicitSoftAnnotationBroad(t *testing.T) {
	publicParam := typ.NewArray(typ.Any)
	observed := typ.NewTuple(
		typ.NewRecord().Field("ok", typ.Boolean).Field("value", typ.String).Build(),
		typ.NewRecord().Field("ok", typ.Boolean).Build(),
	)
	ff := projectionFunctionFact(
		projectionFactSignature(typ.Func().Param("responses", publicParam).Returns(typ.Any).Build()),
		projectionFactCallParams(observed),
	)

	public := functionfact.ProjectType(ff, functionfact.ProjectionPublic, api.SynthModeDeclared)
	publicFn, ok := public.(*typ.Function)
	if !ok || len(publicFn.Params) != 1 {
		t.Fatalf("public projection = %v, want one-param function", public)
	}
	if !typ.TypeEquals(publicFn.Params[0].Type, publicParam) {
		t.Fatalf("public projection narrowed explicit {any}: got %v, want %v", publicFn.Params[0].Type, publicParam)
	}

	body := functionfact.ProjectType(projectionFunctionFact(
		projectionFactSignature(ff.Public.Signature),
		projectionFactEntryParams(observed),
	), functionfact.ProjectionBody, api.SynthModeDeclared)
	bodyFn, ok := body.(*typ.Function)
	if !ok || len(bodyFn.Params) != 1 {
		t.Fatalf("body projection = %v, want one-param function", body)
	}
	if typ.TypeEquals(bodyFn.Params[0].Type, publicParam) {
		t.Fatalf("body projection should refine soft annotation from observed evidence")
	}
}

func TestProjectionBodyRefinesSoftArrayAnnotationWithEntryEvidence(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build()
	publicParam := typ.NewArray(typ.Any)
	ff := projectionFunctionFact(
		projectionFactSignature(typ.Func().Param("tests", publicParam).Build()),
		projectionFactEntryParams(typ.NewOptional(typ.NewArray(entry))),
	)

	body := functionfact.ProjectType(ff, functionfact.ProjectionBody, api.SynthModeDeclared)
	bodyFn, ok := body.(*typ.Function)
	if !ok || len(bodyFn.Params) != 1 {
		t.Fatalf("body projection = %v, want one-param function", body)
	}
	want := typ.NewArray(entry)
	if !typ.TypeEquals(bodyFn.Params[0].Type, want) {
		t.Fatalf("body projection = %v, want %v", bodyFn.Params[0].Type, want)
	}

	public := functionfact.ProjectType(ff, functionfact.ProjectionPublic, api.SynthModeDeclared)
	publicFn, ok := public.(*typ.Function)
	if !ok || len(publicFn.Params) != 1 {
		t.Fatalf("public projection = %v, want one-param function", public)
	}
	if !typ.TypeEquals(publicFn.Params[0].Type, publicParam) {
		t.Fatalf("entry/body evidence leaked into public projection: got %v, want %v", publicFn.Params[0].Type, publicParam)
	}
}

func TestBodyInputProjectionRefinesSoftAnnotationWithEntryEvidence(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	publicParam := typ.NewArray(typ.Any)
	signature := typ.Func().Param("tests", publicParam).Build()

	body := functionfact.BodyInputProjection(signature, nil, []typ.Type{typ.NewArray(entry)})
	if body == nil || len(body.Params) != 1 {
		t.Fatalf("body input projection = %v, want one-param function", body)
	}
	if want := typ.NewArray(entry); !typ.TypeEquals(body.Params[0].Type, want) {
		t.Fatalf("body input projection param = %v, want %v", body.Params[0].Type, want)
	}
}

func TestBodyInputProjectionClosesGenericSignatureFromWholeEvidenceVector(t *testing.T) {
	tParam := typ.NewTypeParam("T", nil)
	uParam := typ.NewTypeParam("U", nil)
	boxParam := typ.NewTypeParam("X", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typ.NewRecord().Field("value", boxParam).Build())
	envelope := typ.NewRecord().Field("id", typ.String).Build()
	view := typ.NewRecord().Field("label", typ.String).Build()

	signature := typ.Func().
		TypeParamRef(tParam).
		TypeParamRef(uParam).
		Param("box", typ.Instantiate(box, tParam)).
		Param("fn", typ.Func().Param("value", tParam).Returns(uParam).Build()).
		Returns(typ.Instantiate(box, uParam)).
		Build()

	outerT := typ.NewTypeParam("T", nil)
	body := functionfact.BodyInputProjection(signature, nil, []typ.Type{
		typ.Instantiate(box, outerT),
		typ.Func().Param("env", envelope).Returns(view).Build(),
	})
	if body == nil {
		t.Fatal("body input projection returned nil")
	}
	if len(body.TypeParams) != 0 {
		t.Fatalf("body input projection left generic binders = %v", body.TypeParams)
	}
	if len(body.Params) != 2 || len(body.Returns) != 1 {
		t.Fatalf("body input projection shape = %v", body)
	}
	fn := body.Params[1].Type.(*typ.Function)
	if !typ.TypeEquals(fn.Params[0].Type, envelope) || !typ.TypeEquals(fn.Returns[0], view) {
		t.Fatalf("callback param = %v, want Envelope -> View", fn)
	}
	if !typ.TypeEquals(body.Returns[0], typ.Instantiate(box, view)) {
		t.Fatalf("body return = %v, want Box<View>", body.Returns[0])
	}
}

func TestParameterEvidenceSignatures_NilInputs(t *testing.T) {
	if got := functionfact.ParameterEvidenceSignatures(nil, nil, nil, nil); got != nil {
		t.Fatalf("ParameterEvidenceSignatures() = %v, want nil", got)
	}
}

func TestParameterEvidenceSignatures_ProjectsCurrentGraphFacts(t *testing.T) {
	st := store.NewSessionStore()
	fn := &ast.FunctionExpr{}
	graph := cfg.Build(fn)
	st.RegisterGraph(graph, fn)
	parent := scope.New().WithType("parent", typ.String)
	registerGraphParent(t, st, graph, parent)

	sym := cfg.SymbolID(21)
	st.RegisterFunctionRef(sym, fn, graph, 0, 0)
	writeFunctionFacts(st, graph, parent, functionfact.BuildOne(sym, functionfact.Evidence{
		EntryParams: []typ.Type{typ.String},
	}))

	got := functionfact.ParameterEvidenceSignatures(st, graph, parent, nil)
	evidence := got[fn]
	if len(evidence) != 1 || !typ.TypeEquals(evidence[0], typ.String) {
		t.Fatalf("signature evidence = %v, want string", evidence)
	}
}

func TestParameterEvidenceSignatures_UsesEntryParamsNotPublicOrBodyContracts(t *testing.T) {
	st := store.NewSessionStore()
	fn := &ast.FunctionExpr{}
	graph := cfg.Build(fn)
	st.RegisterGraph(graph, fn)
	parent := scope.New().WithType("parent", typ.String)
	registerGraphParent(t, st, graph, parent)

	sym := cfg.SymbolID(22)
	st.RegisterFunctionRef(sym, fn, graph, 0, 0)
	entryParam := typ.NewRecord().OptField("message", typ.String).Build()
	bodyParam := typ.NewRecord().Field("message", typ.String).Build()
	facts := functionfact.BuildOne(sym, functionfact.Evidence{
		Params:      []typ.Type{typ.Any},
		BodyParams:  []typ.Type{bodyParam},
		EntryParams: []typ.Type{entryParam},
	})
	writeFunctionFacts(st, graph, parent, facts)

	got := functionfact.ParameterEvidenceSignatures(st, graph, parent, nil)
	evidence := got[fn]
	if len(evidence) != 1 || !typ.TypeEquals(evidence[0], entryParam) {
		t.Fatalf("signature evidence = %v, want entry param %v", evidence, entryParam)
	}
	if public := functionfact.FactsProjection(st.FunctionFactsProjection(graph, parent)).PublicParameterEvidence(sym); len(public) != 1 || !typ.TypeEquals(public[0], typ.Any) {
		t.Fatalf("public evidence = %v, want any", public)
	}
}

func TestVisibleFactsForGraph_ProjectsParentScopeFunctionFacts(t *testing.T) {
	st := store.NewSessionStore()
	parentFn := &ast.FunctionExpr{}
	childFn := &ast.FunctionExpr{}
	siblingFn := &ast.FunctionExpr{}
	parentGraph := cfg.Build(parentFn)
	childGraph := cfg.Build(childFn)
	st.RegisterGraph(parentGraph, parentFn)
	st.RegisterGraph(childGraph, childFn)
	parent := scope.New().WithType("parent", typ.String)
	registerGraphParent(t, st, parentGraph, parent)

	sym := cfg.SymbolID(23)
	siblingSym := cfg.SymbolID(24)
	st.RegisterFunctionRef(sym, childFn, childGraph, parentGraph.ID(), 1)
	st.RegisterFunctionRef(siblingSym, siblingFn, parentGraph, parentGraph.ID(), 2)
	st.RegisterNestedMeta(childGraph.ID(), parentGraph.ID(), 1)
	writeFunctionFacts(st, parentGraph, parent, functionfact.Build(map[cfg.SymbolID]functionfact.Evidence{
		sym: {
			EntryParams: []typ.Type{typ.String},
		},
		siblingSym: {
			EntryParams: []typ.Type{typ.Number},
		},
	}))

	got := functionfact.VisibleFactsForGraph(st, childGraph, nil, parent)
	evidence := functionfact.FactsProjection(got).BodyEntryEvidence(sym)
	if len(evidence) != 1 || !typ.TypeEquals(evidence[0], typ.String) {
		t.Fatalf("visible entry evidence = %v, want string", evidence)
	}
	siblingEvidence := functionfact.FactsProjection(got).BodyEntryEvidence(siblingSym)
	if len(siblingEvidence) != 1 || !typ.TypeEquals(siblingEvidence[0], typ.Number) {
		t.Fatalf("visible sibling entry evidence = %v, want number", siblingEvidence)
	}
}

func registerGraphParent(t *testing.T, st *store.SessionStore, graph *cfg.Graph, parent *scope.State) {
	t.Helper()
	if graph == nil || graph.ID() == 0 {
		t.Fatal("test graph has no ID")
	}
	if parent == nil || parent.Hash() == 0 {
		t.Fatal("test parent has no hash")
	}
	st.SetParentScope(parent.Hash(), parent)
	st.SetGraphParentHash(graph.ID(), parent.Hash())
}

func writeFunctionFactType(st *store.SessionStore, graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID, fnType *typ.Function) {
	writeFunctionFacts(st, graph, parent, api.FunctionFacts{
		sym: projectionFunctionFact(projectionFactSignature(fnType)),
	})
}

func writeFunctionFacts(st *store.SessionStore, graph *cfg.Graph, parent *scope.State, facts api.FunctionFacts) {
	st.ClearProjectionFactState()
	key := api.KeyForGraph(graph, parent.Hash())
	st.MergeProjectionFactsNext(key, api.Facts{FunctionFacts: facts})
	st.AdvanceProjectionFacts()
}
