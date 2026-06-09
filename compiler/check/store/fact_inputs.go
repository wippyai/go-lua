package store

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/types/db"
)

// factInputs are the Salsa-style source inputs for final projection reads. They
// publish canonical Summary-derived FunctionFacts slots so FuncResult queries
// depend on exactly the final projection rows they read.
type factInputs struct {
	database *db.DB

	canonicalFunctionFactMaps *db.Input[api.GraphKey, api.FunctionFacts]
	canonicalFunctionFacts    *db.Input[api.FunctionFactKey, api.FunctionFact]

	canonicalFunctionMapValues  map[api.GraphKey]api.FunctionFacts
	canonicalFunctionFactValues map[api.FunctionFactKey]api.FunctionFact
}

func newFactInputs(database *db.DB) *factInputs {
	if database == nil {
		return nil
	}
	return &factInputs{
		database:                    database,
		canonicalFunctionFactMaps:   db.NewInput[api.GraphKey, api.FunctionFacts](database),
		canonicalFunctionFacts:      db.NewInput[api.FunctionFactKey, api.FunctionFact](database),
		canonicalFunctionMapValues:  make(map[api.GraphKey]api.FunctionFacts),
		canonicalFunctionFactValues: make(map[api.FunctionFactKey]api.FunctionFact),
	}
}

func (in *factInputs) reset() {
	if in == nil || in.database == nil {
		return
	}
	batch := in.database.NewInputBatch()
	for key := range in.canonicalFunctionMapValues {
		in.canonicalFunctionFactMaps.SetInBatch(batch, key, nil)
	}
	for key := range in.canonicalFunctionFactValues {
		in.canonicalFunctionFacts.SetInBatch(batch, key, api.FunctionFact{})
	}
	clear(in.canonicalFunctionMapValues)
	clear(in.canonicalFunctionFactValues)
}

func (in *factInputs) canonicalFunctionFactsFor(ctx *db.QueryContext, key api.GraphKey) (api.FunctionFacts, bool) {
	if in == nil {
		return nil, false
	}
	return functionFactsFromInput(ctx, in.canonicalFunctionFactMaps, key)
}

func (in *factInputs) canonicalFunctionFactFor(ctx *db.QueryContext, key api.FunctionFactKey) (api.FunctionFact, bool) {
	if in == nil {
		return api.FunctionFact{}, false
	}
	return functionFactFromInput(ctx, in.canonicalFunctionFacts, key)
}

func functionFactsFromInput(ctx *db.QueryContext, input *db.Input[api.GraphKey, api.FunctionFacts], key api.GraphKey) (api.FunctionFacts, bool) {
	if input == nil {
		return nil, false
	}
	facts, ok := input.Get(ctx, key)
	if !ok || len(facts) == 0 {
		return nil, false
	}
	return cloneFunctionFacts(facts), true
}

func functionFactFromInput(ctx *db.QueryContext, input *db.Input[api.FunctionFactKey, api.FunctionFact], key api.FunctionFactKey) (api.FunctionFact, bool) {
	if input == nil {
		return api.FunctionFact{}, false
	}
	ff, ok := input.Get(ctx, key)
	if !ok || functionfact.Empty(ff) {
		return api.FunctionFact{}, false
	}
	return cloneFunctionFact(ff), true
}

func setFunctionFactMap(
	batch *db.InputBatch,
	input *db.Input[api.GraphKey, api.FunctionFacts],
	values map[api.GraphKey]api.FunctionFacts,
	key api.GraphKey,
	facts api.FunctionFacts,
) {
	if input == nil || values == nil {
		return
	}
	if len(facts) == 0 {
		if _, ok := values[key]; !ok {
			return
		}
		delete(values, key)
		input.SetInBatch(batch, key, nil)
		return
	}
	next := cloneFunctionFacts(facts)
	if prev, ok := values[key]; ok && functionfact.FactsEqual(prev, next) {
		return
	}
	values[key] = next
	input.SetInBatch(batch, key, next)
}

func setFunctionFacts(
	batch *db.InputBatch,
	input *db.Input[api.FunctionFactKey, api.FunctionFact],
	values map[api.FunctionFactKey]api.FunctionFact,
	key api.GraphKey,
	facts api.FunctionFacts,
) {
	if input == nil || values == nil {
		return
	}
	seen := make(map[api.FunctionFactKey]bool, len(facts))
	for sym, ff := range facts {
		inputKey := api.FunctionFactKey{GraphKey: key, Symbol: sym}
		seen[inputKey] = true
		next := cloneFunctionFact(ff)
		if prev, ok := values[inputKey]; ok && functionfact.Equal(prev, next) {
			continue
		}
		values[inputKey] = next
		input.SetInBatch(batch, inputKey, next)
	}
	for inputKey := range values {
		if inputKey.GraphKey != key || seen[inputKey] {
			continue
		}
		delete(values, inputKey)
		input.SetInBatch(batch, inputKey, api.FunctionFact{})
	}
}

func (s *SessionStore) PushFactReadContext(ctx *db.QueryContext) func() {
	if s == nil || ctx == nil || s.factInputs == nil {
		return func() {}
	}
	prev := s.factCtx
	s.factCtx = ctx
	return func() {
		s.factCtx = prev
	}
}

func (s *SessionStore) canonicalFunctionFactsByKey(key api.GraphKey) api.FunctionFacts {
	if s == nil || s.factInputs == nil {
		return nil
	}
	if facts, ok := s.factInputs.canonicalFunctionFactsFor(s.factCtx, key); ok {
		return facts
	}
	return nil
}

func (s *SessionStore) canonicalFunctionFactByKey(key api.FunctionFactKey) (api.FunctionFact, bool) {
	if s == nil || key.Symbol == 0 || s.factInputs == nil {
		return api.FunctionFact{}, false
	}
	if ff, ok := s.factInputs.canonicalFunctionFactFor(s.factCtx, key); ok {
		return ff, true
	}
	return api.FunctionFact{}, false
}

// SetCanonicalFunctionFactsProjection publishes final Summary-derived
// FunctionFacts without mutating any analysis authority.
func (s *SessionStore) SetCanonicalFunctionFactsProjection(facts map[api.GraphKey]api.FunctionFacts) {
	if s == nil || s.factInputs == nil {
		return
	}
	keys := make(map[api.GraphKey]struct{}, len(facts)+len(s.factInputs.canonicalFunctionMapValues))
	for key := range facts {
		keys[key] = struct{}{}
	}
	for key := range s.factInputs.canonicalFunctionMapValues {
		keys[key] = struct{}{}
	}
	for key := range s.factInputs.canonicalFunctionFactValues {
		keys[key.GraphKey] = struct{}{}
	}
	batch := s.factInputs.database.NewInputBatch()
	for key := range keys {
		functionFacts := facts[key]
		setFunctionFactMap(batch, s.factInputs.canonicalFunctionFactMaps, s.factInputs.canonicalFunctionMapValues, key, functionFacts)
		setFunctionFacts(batch, s.factInputs.canonicalFunctionFacts, s.factInputs.canonicalFunctionFactValues, key, functionFacts)
	}
}
