package store

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// factInputs are the Salsa-style source inputs for interprocedural reads. They
// publish projected lane slots so FuncResult queries depend on the exact
// slots they read; canonical uses them only for final Summary-derived output
// projection, while projection paths may still refresh them at iteration boundaries.
type factInputs struct {
	database *db.DB

	functionFactMaps    *db.Input[api.GraphKey, api.FunctionFacts]
	functionFacts       *db.Input[api.FunctionFactKey, api.FunctionFact]
	capturedTypes       *db.Input[api.CapturedTypeKey, product.AbstractValue]
	capturedFields      *db.Input[api.GraphKey, api.CapturedFieldAssigns]
	constructorFields   *db.Input[api.ConstructorFieldKey, api.FieldValues]
	functionMapValues   map[api.GraphKey]api.FunctionFacts
	functionFactValues  map[api.FunctionFactKey]api.FunctionFact
	capturedTypeValues  map[api.CapturedTypeKey]product.AbstractValue
	capturedFieldValues map[api.GraphKey]api.CapturedFieldAssigns
	constructorValues   map[api.ConstructorFieldKey]api.FieldValues
}

func newFactInputs(database *db.DB) *factInputs {
	if database == nil {
		return nil
	}
	return &factInputs{
		database:            database,
		functionFactMaps:    db.NewInput[api.GraphKey, api.FunctionFacts](database),
		functionFacts:       db.NewInput[api.FunctionFactKey, api.FunctionFact](database),
		capturedTypes:       db.NewInput[api.CapturedTypeKey, product.AbstractValue](database),
		capturedFields:      db.NewInput[api.GraphKey, api.CapturedFieldAssigns](database),
		constructorFields:   db.NewInput[api.ConstructorFieldKey, api.FieldValues](database),
		functionMapValues:   make(map[api.GraphKey]api.FunctionFacts),
		functionFactValues:  make(map[api.FunctionFactKey]api.FunctionFact),
		capturedTypeValues:  make(map[api.CapturedTypeKey]product.AbstractValue),
		capturedFieldValues: make(map[api.GraphKey]api.CapturedFieldAssigns),
		constructorValues:   make(map[api.ConstructorFieldKey]api.FieldValues),
	}
}

func (in *factInputs) reset() {
	if in == nil || in.database == nil {
		return
	}
	batch := in.database.NewInputBatch()
	for key := range in.functionMapValues {
		in.functionFactMaps.SetInBatch(batch, key, nil)
	}
	for key := range in.functionFactValues {
		in.functionFacts.SetInBatch(batch, key, api.FunctionFact{})
	}
	for key := range in.capturedTypeValues {
		in.capturedTypes.SetInBatch(batch, key, product.AbstractValue{})
	}
	for key := range in.capturedFieldValues {
		in.capturedFields.SetInBatch(batch, key, nil)
	}
	for key := range in.constructorValues {
		in.constructorFields.SetInBatch(batch, key, nil)
	}
	clear(in.functionMapValues)
	clear(in.functionFactValues)
	clear(in.capturedTypeValues)
	clear(in.capturedFieldValues)
	clear(in.constructorValues)
}

func (in *factInputs) functionFactsFor(ctx *db.QueryContext, key api.GraphKey) (api.FunctionFacts, bool) {
	if in == nil || in.functionFactMaps == nil {
		return nil, false
	}
	facts, ok := in.functionFactMaps.Get(ctx, key)
	if !ok || len(facts) == 0 {
		return nil, false
	}
	return cloneFunctionFacts(facts), true
}

func (in *factInputs) functionFactFor(ctx *db.QueryContext, key api.FunctionFactKey) (api.FunctionFact, bool) {
	if in == nil || in.functionFacts == nil {
		return api.FunctionFact{}, false
	}
	ff, ok := in.functionFacts.Get(ctx, key)
	if !ok || functionfact.Empty(ff) {
		return api.FunctionFact{}, false
	}
	return cloneFunctionFact(ff), true
}

func (in *factInputs) capturedTypeFor(ctx *db.QueryContext, key api.CapturedTypeKey) (typ.Type, bool) {
	if in == nil || in.capturedTypes == nil {
		return nil, false
	}
	t, ok := in.capturedTypes.Get(ctx, key)
	if !ok || t.IsZero() {
		return nil, false
	}
	return t.ProjectValue(), true
}

func (in *factInputs) capturedFieldAssignsFor(ctx *db.QueryContext, key api.GraphKey) (api.CapturedFieldAssigns, bool) {
	if in == nil || in.capturedFields == nil {
		return nil, false
	}
	fields, ok := in.capturedFields.Get(ctx, key)
	if !ok || len(fields) == 0 {
		return nil, false
	}
	return cloneCapturedFieldAssigns(fields), true
}

func (in *factInputs) constructorFieldsFor(ctx *db.QueryContext, key api.ConstructorFieldKey) (api.FieldValues, bool) {
	if in == nil || in.constructorFields == nil {
		return nil, false
	}
	fields, ok := in.constructorFields.Get(ctx, key)
	if !ok || len(fields) == 0 {
		return nil, false
	}
	return cloneConstructorFieldMap(fields), true
}

func (in *factInputs) setPostflowProjectionLanes(
	batch *db.InputBatch,
	key api.GraphKey,
	functionFacts api.FunctionFacts,
	capturedTypes api.CapturedTypes,
	capturedFields api.CapturedFieldAssigns,
	constructorFields api.ConstructorFields,
) {
	in.setProjectedFunctionFactMap(batch, key, functionFacts)
	in.setProjectedFunctionFacts(batch, key, functionFacts)
	in.setProjectedCapturedTypes(batch, key, capturedTypes)
	in.setProjectedCapturedFields(batch, key, capturedFields)
	in.setProjectedConstructorFields(batch, key, constructorFields)
}

func (in *factInputs) setProjectedFunctionFactMap(batch *db.InputBatch, key api.GraphKey, facts api.FunctionFacts) {
	if in == nil || in.functionFactMaps == nil {
		return
	}
	if len(facts) == 0 {
		if _, ok := in.functionMapValues[key]; !ok {
			return
		}
		delete(in.functionMapValues, key)
		in.functionFactMaps.SetInBatch(batch, key, nil)
		return
	}
	next := cloneFunctionFacts(facts)
	if prev, ok := in.functionMapValues[key]; ok && interproc.FunctionFactsEqual(prev, next) {
		return
	}
	in.functionMapValues[key] = next
	in.functionFactMaps.SetInBatch(batch, key, next)
}

func (in *factInputs) setProjectedFunctionFacts(batch *db.InputBatch, key api.GraphKey, facts api.FunctionFacts) {
	if in == nil || in.functionFacts == nil {
		return
	}
	seen := make(map[api.FunctionFactKey]bool, len(facts))
	for sym, ff := range facts {
		inputKey := api.FunctionFactKey{GraphKey: key, Symbol: sym}
		seen[inputKey] = true
		next := cloneFunctionFact(ff)
		if prev, ok := in.functionFactValues[inputKey]; ok && interproc.FunctionFactEqual(prev, next) {
			continue
		}
		in.functionFactValues[inputKey] = next
		in.functionFacts.SetInBatch(batch, inputKey, next)
	}
	for inputKey := range in.functionFactValues {
		if inputKey.GraphKey != key || seen[inputKey] {
			continue
		}
		delete(in.functionFactValues, inputKey)
		in.functionFacts.SetInBatch(batch, inputKey, api.FunctionFact{})
	}
}

func (in *factInputs) setProjectedCapturedTypes(batch *db.InputBatch, key api.GraphKey, types api.CapturedTypes) {
	if in == nil || in.capturedTypes == nil {
		return
	}
	seen := make(map[api.CapturedTypeKey]bool, len(types))
	for sym, t := range types {
		inputKey := api.CapturedTypeKey{GraphKey: key, Symbol: sym}
		seen[inputKey] = true
		if prev, ok := in.capturedTypeValues[inputKey]; ok && product.Equal(prev, t) {
			continue
		}
		in.capturedTypeValues[inputKey] = t
		in.capturedTypes.SetInBatch(batch, inputKey, t)
	}
	for inputKey := range in.capturedTypeValues {
		if inputKey.GraphKey != key || seen[inputKey] {
			continue
		}
		delete(in.capturedTypeValues, inputKey)
		in.capturedTypes.SetInBatch(batch, inputKey, product.AbstractValue{})
	}
}

func (in *factInputs) setProjectedCapturedFields(batch *db.InputBatch, key api.GraphKey, fields api.CapturedFieldAssigns) {
	if in == nil || in.capturedFields == nil {
		return
	}
	if len(fields) == 0 {
		if _, ok := in.capturedFieldValues[key]; !ok {
			return
		}
		delete(in.capturedFieldValues, key)
		in.capturedFields.SetInBatch(batch, key, nil)
		return
	}
	next := cloneCapturedFieldAssigns(fields)
	if prev, ok := in.capturedFieldValues[key]; ok && interproc.CapturedFieldAssignsEqual(prev, next) {
		return
	}
	in.capturedFieldValues[key] = next
	in.capturedFields.SetInBatch(batch, key, next)
}

func (in *factInputs) setProjectedConstructorFields(batch *db.InputBatch, key api.GraphKey, fields api.ConstructorFields) {
	if in == nil || in.constructorFields == nil {
		return
	}
	seen := make(map[api.ConstructorFieldKey]bool, len(fields))
	for sym, fieldMap := range fields {
		inputKey := api.ConstructorFieldKey{GraphKey: key, Symbol: sym}
		seen[inputKey] = true
		next := cloneConstructorFieldMap(fieldMap)
		if prev, ok := in.constructorValues[inputKey]; ok && interproc.ConstructorFieldMapEqual(sym, prev, next) {
			continue
		}
		in.constructorValues[inputKey] = next
		in.constructorFields.SetInBatch(batch, inputKey, next)
	}
	for inputKey := range in.constructorValues {
		if inputKey.GraphKey != key || seen[inputKey] {
			continue
		}
		delete(in.constructorValues, inputKey)
		in.constructorFields.SetInBatch(batch, inputKey, nil)
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

func (s *SessionStore) visibleFunctionFact(key api.GraphKey, sym cfg.SymbolID) (api.FunctionFact, bool) {
	if s == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	if s.postflowPrev != nil && s.postflowNext != nil {
		prev := functionFactFromMap(s.postflowPrev.functionFacts[key], sym)
		next := functionFactFromMap(s.postflowNext.functionFacts[key], sym)
		switch {
		case functionfact.Empty(prev) && functionfact.Empty(next):
			return api.FunctionFact{}, false
		case functionfact.Empty(prev):
			return cloneFunctionFact(next), true
		case functionfact.Empty(next):
			return cloneFunctionFact(prev), true
		default:
			merged := interproc.OverlayFunctionFacts(
				api.FunctionFacts{sym: prev},
				api.FunctionFacts{sym: next},
			)
			ff := functionFactFromMap(merged, sym)
			return cloneFunctionFact(ff), !functionfact.Empty(ff)
		}
	}
	return api.FunctionFact{}, false
}

func functionFactFromMap(facts api.FunctionFacts, sym cfg.SymbolID) api.FunctionFact {
	if sym == 0 || len(facts) == 0 {
		return api.FunctionFact{}
	}
	return facts[sym]
}

func (s *SessionStore) visibleFunctionFacts(key api.GraphKey) api.FunctionFacts {
	if s == nil {
		return nil
	}
	var prev api.FunctionFacts
	if s.postflowPrev != nil {
		prev = s.postflowPrev.functionFacts[key]
	}
	var next api.FunctionFacts
	if s.postflowNext != nil {
		next = s.postflowNext.functionFacts[key]
	}
	return cloneFunctionFacts(interproc.OverlayFunctionFacts(prev, next))
}

func (s *SessionStore) visibleCapturedType(key api.GraphKey, sym cfg.SymbolID) (typ.Type, bool) {
	if s == nil || sym == 0 {
		return nil, false
	}
	if s.postflowPrev != nil && s.postflowNext != nil {
		prev := capturedTypeFromMap(s.postflowPrev.capturedTypes[key], sym)
		next := capturedTypeFromMap(s.postflowNext.capturedTypes[key], sym)
		switch {
		case prev.IsZero() && next.IsZero():
			return nil, false
		case prev.IsZero():
			return next.ProjectValue(), true
		case next.IsZero():
			return prev.ProjectValue(), true
		default:
			merged := interproc.WidenCapturedTypes(
				api.CapturedTypes{sym: prev},
				api.CapturedTypes{sym: next},
			)
			t := merged[sym]
			if t.IsZero() {
				return nil, false
			}
			return t.ProjectValue(), true
		}
	}
	return nil, false
}

func capturedTypeFromMap(types api.CapturedTypes, sym cfg.SymbolID) product.AbstractValue {
	if sym == 0 || len(types) == 0 {
		return product.AbstractValue{}
	}
	return types[sym]
}

func (s *SessionStore) visibleCapturedFieldAssigns(key api.GraphKey) api.CapturedFieldAssigns {
	if s == nil {
		return nil
	}
	var prev api.CapturedFieldAssigns
	if s.postflowPrev != nil {
		prev = s.postflowPrev.capturedFields[key]
	}
	var next api.CapturedFieldAssigns
	if s.postflowNext != nil {
		next = s.postflowNext.capturedFields[key]
	}
	return cloneCapturedFieldAssigns(interproc.WidenCapturedFieldAssigns(prev, next))
}

func (s *SessionStore) visibleConstructorFields(key api.GraphKey, sym cfg.SymbolID) (api.FieldValues, bool) {
	if s == nil || sym == 0 {
		return nil, false
	}
	if s.postflowPrev != nil && s.postflowNext != nil {
		prev := constructorFieldsFromMap(s.postflowPrev.constructorFields[key], sym)
		next := constructorFieldsFromMap(s.postflowNext.constructorFields[key], sym)
		switch {
		case len(prev) == 0 && len(next) == 0:
			return nil, false
		case len(prev) == 0:
			return cloneConstructorFieldMap(next), true
		case len(next) == 0:
			return cloneConstructorFieldMap(prev), true
		default:
			merged := interproc.WidenConstructorFields(
				api.ConstructorFields{sym: prev},
				api.ConstructorFields{sym: next},
			)
			fields := merged[sym]
			return cloneConstructorFieldMap(fields), len(fields) > 0
		}
	}
	return nil, false
}

func constructorFieldsFromMap(fields api.ConstructorFields, sym cfg.SymbolID) api.FieldValues {
	if sym == 0 || len(fields) == 0 {
		return nil
	}
	return fields[sym]
}

func (s *SessionStore) functionFactsByKey(key api.GraphKey) api.FunctionFacts {
	if s == nil {
		return nil
	}
	if s.factInputs != nil {
		if facts, ok := s.factInputs.functionFactsFor(s.factCtx, key); ok {
			return facts
		}
		return nil
	}
	return s.visibleFunctionFacts(key)
}

func (s *SessionStore) functionFactByKey(key api.FunctionFactKey) (api.FunctionFact, bool) {
	if s == nil || key.Symbol == 0 {
		return api.FunctionFact{}, false
	}
	if s.factInputs != nil {
		if ff, ok := s.factInputs.functionFactFor(s.factCtx, key); ok {
			return ff, true
		}
		return api.FunctionFact{}, false
	}
	return s.visibleFunctionFact(key.GraphKey, key.Symbol)
}

func (s *SessionStore) capturedTypeByKey(key api.CapturedTypeKey) (typ.Type, bool) {
	if s == nil || key.Symbol == 0 {
		return nil, false
	}
	if s.factInputs != nil {
		return s.factInputs.capturedTypeFor(s.factCtx, key)
	}
	return s.visibleCapturedType(key.GraphKey, key.Symbol)
}

func (s *SessionStore) capturedFieldAssignsByKey(key api.GraphKey) api.CapturedFieldAssigns {
	if s == nil {
		return nil
	}
	if s.factInputs != nil {
		if fields, ok := s.factInputs.capturedFieldAssignsFor(s.factCtx, key); ok {
			return fields
		}
		return nil
	}
	return s.visibleCapturedFieldAssigns(key)
}

func (s *SessionStore) constructorFieldsByKey(key api.ConstructorFieldKey) (api.FieldValues, bool) {
	if s == nil || key.Symbol == 0 {
		return nil, false
	}
	if s.factInputs != nil {
		return s.factInputs.constructorFieldsFor(s.factCtx, key)
	}
	return s.visibleConstructorFields(key.GraphKey, key.Symbol)
}

func (s *SessionStore) syncPostflowProjectionInputs(batch *db.InputBatch, key api.GraphKey) {
	if s == nil || s.factInputs == nil {
		return
	}
	s.factInputs.setPostflowProjectionLanes(
		batch,
		key,
		s.visibleFunctionFacts(key),
		s.visibleCapturedTypes(key),
		s.visibleCapturedFieldAssigns(key),
		s.visibleConstructorFieldMap(key),
	)
}

// SetCanonicalFunctionFactsProjection publishes final Summary-derived FunctionFacts
// without mutating postflow prev/next lanes or advancing postflow projection
// iteration.
func (s *SessionStore) SetCanonicalFunctionFactsProjection(facts map[api.GraphKey]api.FunctionFacts) {
	if s == nil || s.factInputs == nil {
		return
	}
	keys := make(map[api.GraphKey]struct{}, len(facts)+len(s.factInputs.functionMapValues))
	for key := range facts {
		keys[key] = struct{}{}
	}
	for key := range s.factInputs.functionMapValues {
		keys[key] = struct{}{}
	}
	for key := range s.factInputs.functionFactValues {
		keys[key.GraphKey] = struct{}{}
	}
	batch := s.factInputs.database.NewInputBatch()
	for key := range keys {
		functionFacts := facts[key]
		s.factInputs.setProjectedFunctionFactMap(batch, key, functionFacts)
		s.factInputs.setProjectedFunctionFacts(batch, key, functionFacts)
	}
}

func (s *SessionStore) visibleCapturedTypes(key api.GraphKey) api.CapturedTypes {
	if s == nil {
		return nil
	}
	var prev api.CapturedTypes
	if s.postflowPrev != nil {
		prev = s.postflowPrev.capturedTypes[key]
	}
	var next api.CapturedTypes
	if s.postflowNext != nil {
		next = s.postflowNext.capturedTypes[key]
	}
	return interproc.WidenCapturedTypes(prev, next)
}

func (s *SessionStore) visibleConstructorFieldMap(key api.GraphKey) api.ConstructorFields {
	if s == nil {
		return nil
	}
	var prev api.ConstructorFields
	if s.postflowPrev != nil {
		prev = s.postflowPrev.constructorFields[key]
	}
	var next api.ConstructorFields
	if s.postflowNext != nil {
		next = s.postflowNext.constructorFields[key]
	}
	return interproc.WidenConstructorFields(prev, next)
}

func (s *SessionStore) syncFactInputs() {
	if s == nil || s.factInputs == nil {
		return
	}

	factKeys := make(map[api.GraphKey]struct{}, len(s.factInputs.functionMapValues))
	for key := range s.factInputs.functionMapValues {
		factKeys[key] = struct{}{}
	}
	for key := range s.factInputs.functionFactValues {
		factKeys[key.GraphKey] = struct{}{}
	}
	for key := range s.factInputs.capturedTypeValues {
		factKeys[key.GraphKey] = struct{}{}
	}
	for key := range s.factInputs.capturedFieldValues {
		factKeys[key] = struct{}{}
	}
	for key := range s.factInputs.constructorValues {
		factKeys[key.GraphKey] = struct{}{}
	}
	if s.postflowPrev != nil {
		for key := range s.postflowPrev.functionFacts {
			factKeys[key] = struct{}{}
		}
		for key := range s.postflowPrev.capturedTypes {
			factKeys[key] = struct{}{}
		}
		for key := range s.postflowPrev.capturedFields {
			factKeys[key] = struct{}{}
		}
		for key := range s.postflowPrev.constructorFields {
			factKeys[key] = struct{}{}
		}
	}
	if s.postflowNext != nil {
		for key := range s.postflowNext.functionFacts {
			factKeys[key] = struct{}{}
		}
		for key := range s.postflowNext.capturedTypes {
			factKeys[key] = struct{}{}
		}
		for key := range s.postflowNext.capturedFields {
			factKeys[key] = struct{}{}
		}
		for key := range s.postflowNext.constructorFields {
			factKeys[key] = struct{}{}
		}
	}
	batch := s.factInputs.database.NewInputBatch()
	for key := range factKeys {
		s.syncPostflowProjectionInputs(batch, key)
	}
}
