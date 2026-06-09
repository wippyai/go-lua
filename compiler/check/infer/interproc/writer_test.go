package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/postflow"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

type factsWriteStoreStub struct {
	parentKeyBySymbol   map[cfg.SymbolID]api.GraphKey
	functionFactsByKey  map[api.GraphKey]api.FunctionFacts
	capturedFieldsByKey map[api.GraphKey]postflow.CapturedFieldAssigns
}

func newFactsWriteStoreStub() *factsWriteStoreStub {
	return &factsWriteStoreStub{
		parentKeyBySymbol:   make(map[cfg.SymbolID]api.GraphKey),
		functionFactsByKey:  make(map[api.GraphKey]api.FunctionFacts),
		capturedFieldsByKey: make(map[api.GraphKey]postflow.CapturedFieldAssigns),
	}
}

func (s *factsWriteStoreStub) MergePostflowFunctionFactProjection(key api.GraphKey, sym cfg.SymbolID, fact api.FunctionFact) {
	if s.functionFactsByKey[key] == nil {
		s.functionFactsByKey[key] = make(api.FunctionFacts)
	}
	s.functionFactsByKey[key][sym] = fact
}

func (s *factsWriteStoreStub) MergeCapturedFieldProjection(key api.GraphKey, nestedSym cfg.SymbolID, capturedSym cfg.SymbolID, fields postflow.FieldValues) {
	if s.capturedFieldsByKey[key] == nil {
		s.capturedFieldsByKey[key] = make(postflow.CapturedFieldAssigns)
	}
	if s.capturedFieldsByKey[key][nestedSym] == nil {
		s.capturedFieldsByKey[key][nestedSym] = make(map[cfg.SymbolID]postflow.FieldValues)
	}
	s.capturedFieldsByKey[key][nestedSym][capturedSym] = fields
}

func (s *factsWriteStoreStub) ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool) {
	key, ok := s.parentKeyBySymbol[sym]
	return key, ok
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
