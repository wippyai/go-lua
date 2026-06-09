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
	factsByGraphKeyNext map[api.GraphKey]api.Facts
}

func newFactsWriteStoreStub() *factsWriteStoreStub {
	return &factsWriteStoreStub{
		parentKeyBySymbol:   make(map[cfg.SymbolID]api.GraphKey),
		symbolByFunc:        make(map[*ast.FunctionExpr]cfg.SymbolID),
		factsByGraphKeyNext: make(map[api.GraphKey]api.Facts),
	}
}

func (s *factsWriteStoreStub) MergeProjectionFactsNext(key api.GraphKey, delta api.Facts) {
	s.factsByGraphKeyNext[key] = delta
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

func TestProjectionFactWriter_MergeParentFactsForSymbol(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 7, ParentHash: 11}
	stub.parentKeyBySymbol[3] = key
	writer := newProjectionFactWriter(stub)

	ok := writer.mergeParentFactsForSymbol(3, api.Facts{
		FunctionFacts: api.FunctionFacts{
			3: {Params: product.LiftVector([]typ.Type{typ.String})},
		},
	})
	if !ok {
		t.Fatal("expected update to succeed")
	}
	got := stub.factsByGraphKeyNext[key]
	if params := functionfact.FactsProjection(got.FunctionFacts).PublicParameterEvidence(3); len(params) != 1 || !typ.TypeEquals(params[0], typ.String) {
		t.Fatalf("unexpected parent facts update: %#v", got.FunctionFacts)
	}

	if writer.mergeParentFactsForSymbol(99, api.Facts{}) {
		t.Fatal("expected update to fail for unknown symbol")
	}
}

func TestProjectionFactWriter_WriteLiteralSignatures(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 5, ParentHash: 9}
	stub.graphKeyFor = key
	stub.graphKeyForOK = true
	writer := newProjectionFactWriter(stub)

	graph, fns := graphWithNestedFunctions(t, `return function() end`)
	fn := fns[0]
	sig := typ.Func().Returns(typ.String).Build()
	sigs := api.LiteralSigsLookup{fn: sig}

	writer.writeLiteralSignatures(graph, scope.New(), sigs)

	gotFacts := stub.factsByGraphKeyNext[key]
	if gotFacts.LiteralSigs == nil || gotFacts.LiteralSigs[fn] != sig {
		t.Fatalf("expected literal sig in facts update, got %#v", gotFacts.LiteralSigs)
	}
}

func TestProjectionFactWriter_WriteLiteralSignaturesSkipsCanonicalFunctions(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 5, ParentHash: 9}
	stub.graphKeyFor = key
	stub.graphKeyForOK = true
	writer := newProjectionFactWriter(stub)

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

	gotFacts := stub.factsByGraphKeyNext[key]
	if gotFacts.LiteralSigs == nil {
		t.Fatalf("expected anonymous literal sig in facts update")
	}
	if gotFacts.LiteralSigs[registered] != nil {
		t.Fatalf("registered function must be published through FunctionFact, got literal sig %#v", gotFacts.LiteralSigs[registered])
	}
	if gotFacts.LiteralSigs[anonymous] != anonymousSig {
		t.Fatalf("expected anonymous literal sig to remain, got %#v", gotFacts.LiteralSigs[anonymous])
	}
}

func TestProjectionFactWriter_WriteLiteralSignatures_RequiresGraphKey(t *testing.T) {
	stub := newFactsWriteStoreStub()
	stub.graphKeyForOK = false
	writer := newProjectionFactWriter(stub)

	graph, fns := graphWithNestedFunctions(t, `return function() end`)
	fn := fns[0]
	sig := typ.Func().Returns(typ.Number).Build()
	writer.writeLiteralSignatures(graph, scope.New(), api.LiteralSigsLookup{fn: sig})

	if len(stub.factsByGraphKeyNext) != 0 {
		t.Fatalf("expected no facts writes without graph key, got %#v", stub.factsByGraphKeyNext)
	}
}
