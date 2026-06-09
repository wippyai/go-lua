package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

type factsWriteStoreStub struct {
	graphKeyFor         api.GraphKey
	graphKeyForOK       bool
	parentKeyBySymbol   map[cfg.SymbolID]api.GraphKey
	symbolByFunc        map[*ast.FunctionExpr]cfg.SymbolID
	functionFactsByKey  map[api.GraphKey]api.FunctionFacts
	literalSigsByKey    map[api.GraphKey]api.LiteralSigs
	capturedFieldsByKey map[api.GraphKey]api.CapturedFieldAssigns
}

func newFactsWriteStoreStub() *factsWriteStoreStub {
	return &factsWriteStoreStub{
		parentKeyBySymbol:   make(map[cfg.SymbolID]api.GraphKey),
		symbolByFunc:        make(map[*ast.FunctionExpr]cfg.SymbolID),
		functionFactsByKey:  make(map[api.GraphKey]api.FunctionFacts),
		literalSigsByKey:    make(map[api.GraphKey]api.LiteralSigs),
		capturedFieldsByKey: make(map[api.GraphKey]api.CapturedFieldAssigns),
	}
}

func (s *factsWriteStoreStub) MergeFunctionFactProjection(key api.GraphKey, sym cfg.SymbolID, fact api.FunctionFact) {
	if s.functionFactsByKey[key] == nil {
		s.functionFactsByKey[key] = make(api.FunctionFacts)
	}
	s.functionFactsByKey[key][sym] = fact
}

func (s *factsWriteStoreStub) MergeLiteralSignatureProjection(key api.GraphKey, fn *ast.FunctionExpr, sig *typ.Function) {
	if s.literalSigsByKey[key] == nil {
		s.literalSigsByKey[key] = make(api.LiteralSigs)
	}
	s.literalSigsByKey[key][fn] = sig
}

func (s *factsWriteStoreStub) MergeCapturedFieldProjection(key api.GraphKey, nestedSym cfg.SymbolID, capturedSym cfg.SymbolID, fields api.FieldValues) {
	if s.capturedFieldsByKey[key] == nil {
		s.capturedFieldsByKey[key] = make(api.CapturedFieldAssigns)
	}
	if s.capturedFieldsByKey[key][nestedSym] == nil {
		s.capturedFieldsByKey[key][nestedSym] = make(map[cfg.SymbolID]api.FieldValues)
	}
	s.capturedFieldsByKey[key][nestedSym][capturedSym] = fields
}

func (s *factsWriteStoreStub) GraphKeyFor(_ *cfg.Graph, _ *scope.State) (api.GraphKey, bool) {
	return s.graphKeyFor, s.graphKeyForOK
}

func (s *factsWriteStoreStub) ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool) {
	key, ok := s.parentKeyBySymbol[sym]
	return key, ok
}

func (s *factsWriteStoreStub) SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool) {
	sym, ok := s.symbolByFunc[fn]
	return sym, ok
}

func graphWithNestedFunctions(t *testing.T, src string) (*cfg.Graph, []*ast.FunctionExpr) {
	t.Helper()
	stmts, err := parse.ParseString(src, "literal_sigs.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	graph := cfg.Build(root)
	var fns []*ast.FunctionExpr
	for _, nested := range graph.NestedFunctions() {
		if nested.Func != nil {
			fns = append(fns, nested.Func)
		}
	}
	if len(fns) == 0 {
		t.Fatalf("expected nested functions in %q", src)
	}
	return graph, fns
}

func TestPostflowProjectionWriter_MergeParentFactsForSymbol(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 7, ParentHash: 11}
	stub.parentKeyBySymbol[3] = key
	writer := newPostflowProjectionWriter(stub)

	ok := writer.mergeParentFunctionFacts(api.FunctionFacts{
		3: {Call: api.FunctionCallProjection{Params: product.LiftVector([]typ.Type{typ.String})}},
	})
	if !ok {
		t.Fatal("expected update to succeed")
	}
	got := stub.functionFactsByKey[key]
	if params := functionfact.FactsProjection(got).PublicParameterEvidence(3); len(params) != 1 || !typ.TypeEquals(params[0], typ.String) {
		t.Fatalf("unexpected parent facts update: %#v", got)
	}

	if writer.mergeParentFunctionFacts(api.FunctionFacts{99: {}}) {
		t.Fatal("expected update to fail for unknown symbol")
	}
}

func TestPostflowProjectionWriter_WriteLiteralSignatures(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 5, ParentHash: 9}
	stub.graphKeyFor = key
	stub.graphKeyForOK = true
	writer := newPostflowProjectionWriter(stub)

	graph, fns := graphWithNestedFunctions(t, `return function() end`)
	fn := fns[0]
	sig := typ.Func().Returns(typ.String).Build()
	sigs := api.LiteralSigsLookup{fn: sig}

	writer.writeLiteralSignatures(graph, scope.New(), sigs)

	got := stub.literalSigsByKey[key]
	if got == nil || got[fn] != sig {
		t.Fatalf("expected literal sig in facts update, got %#v", got)
	}
}

func TestPostflowProjectionWriter_WriteLiteralSignaturesSkipsCanonicalFunctions(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 5, ParentHash: 9}
	stub.graphKeyFor = key
	stub.graphKeyForOK = true
	writer := newPostflowProjectionWriter(stub)

	graph, fns := graphWithNestedFunctions(t, `
		local registered = function() end
		local anonymous = function() end
	`)
	registered := fns[0]
	anonymous := fns[1]
	stub.symbolByFunc[registered] = 42

	registeredSig := typ.Func().Returns(typ.String).Build()
	anonymousSig := typ.Func().Returns(typ.Number).Build()
	writer.writeLiteralSignatures(graph, scope.New(), api.LiteralSigsLookup{
		registered: registeredSig,
		anonymous:  anonymousSig,
	})

	got := stub.literalSigsByKey[key]
	if got == nil {
		t.Fatalf("expected anonymous literal sig in facts update")
	}
	if got[registered] != nil {
		t.Fatalf("registered function must be published through FunctionFact, got literal sig %#v", got[registered])
	}
	if got[anonymous] != anonymousSig {
		t.Fatalf("expected anonymous literal sig to remain, got %#v", got[anonymous])
	}
}

func TestPostflowProjectionWriter_WriteLiteralSignatures_RequiresGraphKey(t *testing.T) {
	stub := newFactsWriteStoreStub()
	stub.graphKeyForOK = false
	writer := newPostflowProjectionWriter(stub)

	graph, fns := graphWithNestedFunctions(t, `return function() end`)
	fn := fns[0]
	sig := typ.Func().Returns(typ.Number).Build()
	writer.writeLiteralSignatures(graph, scope.New(), api.LiteralSigsLookup{fn: sig})

	if len(stub.literalSigsByKey) != 0 {
		t.Fatalf("expected no literal signature writes without graph key, got %#v", stub.literalSigsByKey)
	}
}
