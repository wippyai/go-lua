package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

type factsWriteStoreStub struct {
	graphKeyFor         api.GraphKey
	graphKeyForOK       bool
	parentKeyBySymbol   map[cfg.SymbolID]api.GraphKey
	factsByGraphKeyNext map[api.GraphKey]api.Facts
}

func newFactsWriteStoreStub() *factsWriteStoreStub {
	return &factsWriteStoreStub{
		parentKeyBySymbol:   make(map[cfg.SymbolID]api.GraphKey),
		factsByGraphKeyNext: make(map[api.GraphKey]api.Facts),
	}
}

func (s *factsWriteStoreStub) MergeInterprocFactsNext(key api.GraphKey, delta api.Facts) {
	s.factsByGraphKeyNext[key] = delta
}

func (s *factsWriteStoreStub) GraphKeyFor(_ *cfg.Graph, _ *scope.State) (api.GraphKey, bool) {
	return s.graphKeyFor, s.graphKeyForOK
}

func (s *factsWriteStoreStub) ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool) {
	key, ok := s.parentKeyBySymbol[sym]
	return key, ok
}

func TestInterprocFactWriter_MergeParentFactsForSymbol(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 7, ParentHash: 11}
	stub.parentKeyBySymbol[3] = key
	writer := newInterprocFactWriter(stub)

	ok := writer.mergeParentFactsForSymbol(3, api.Facts{
		FunctionFacts: api.FunctionFacts{
			3: {Params: []typ.Type{typ.String}},
		},
	})
	if !ok {
		t.Fatal("expected update to succeed")
	}
	got := stub.factsByGraphKeyNext[key]
	if params := got.FunctionFacts.Params(3); len(params) != 1 || !typ.TypeEquals(params[0], typ.String) {
		t.Fatalf("unexpected parent facts update: %#v", got.FunctionFacts)
	}

	if writer.mergeParentFactsForSymbol(99, api.Facts{}) {
		t.Fatal("expected update to fail for unknown symbol")
	}
}

func TestInterprocFactWriter_WriteLiteralSignatures(t *testing.T) {
	stub := newFactsWriteStoreStub()
	key := api.GraphKey{GraphID: 5, ParentHash: 9}
	stub.graphKeyFor = key
	stub.graphKeyForOK = true
	writer := newInterprocFactWriter(stub)

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	sig := typ.Func().Returns(typ.String).Build()
	sigs := map[*ast.FunctionExpr]*typ.Function{fn: sig}

	writer.writeLiteralSignatures(graph, scope.New(), sigs)

	gotFacts := stub.factsByGraphKeyNext[key]
	if gotFacts.LiteralSigs == nil || gotFacts.LiteralSigs[fn] != sig {
		t.Fatalf("expected literal sig in facts update, got %#v", gotFacts.LiteralSigs)
	}
}

func TestInterprocFactWriter_WriteLiteralSignatures_RequiresGraphKey(t *testing.T) {
	stub := newFactsWriteStoreStub()
	stub.graphKeyForOK = false
	writer := newInterprocFactWriter(stub)

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	sig := typ.Func().Returns(typ.Number).Build()
	writer.writeLiteralSignatures(graph, scope.New(), map[*ast.FunctionExpr]*typ.Function{fn: sig})

	if len(stub.factsByGraphKeyNext) != 0 {
		t.Fatalf("expected no facts writes without graph key, got %#v", stub.factsByGraphKeyNext)
	}
}
