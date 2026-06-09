package store

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// factInputs are the Salsa-style source inputs for interprocedural reads. They
// publish projected product slots so FuncResult queries depend on the exact
// slots they read; canonical uses them only for final Summary-derived output
// projection, while legacy paths may still refresh them at iteration boundaries.
type factInputs struct {
	database *db.DB

	functionFactMaps    *db.Input[api.GraphKey, api.FunctionFacts]
	functionFacts       *db.Input[api.FunctionFactKey, api.FunctionFact]
	literalSigs         *db.Input[api.LiteralSigKey, *typ.Function]
	capturedTypes       *db.Input[api.CapturedTypeKey, product.AbstractValue]
	capturedFields      *db.Input[api.GraphKey, api.CapturedFieldAssigns]
	constructorFields   *db.Input[api.ConstructorFieldKey, api.FieldValues]
	functionMapValues   map[api.GraphKey]api.FunctionFacts
	functionFactValues  map[api.FunctionFactKey]api.FunctionFact
	literalSigValues    map[api.LiteralSigKey]*typ.Function
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
		literalSigs:         db.NewInput[api.LiteralSigKey, *typ.Function](database),
		capturedTypes:       db.NewInput[api.CapturedTypeKey, product.AbstractValue](database),
		capturedFields:      db.NewInput[api.GraphKey, api.CapturedFieldAssigns](database),
		constructorFields:   db.NewInput[api.ConstructorFieldKey, api.FieldValues](database),
		functionMapValues:   make(map[api.GraphKey]api.FunctionFacts),
		functionFactValues:  make(map[api.FunctionFactKey]api.FunctionFact),
		literalSigValues:    make(map[api.LiteralSigKey]*typ.Function),
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
	for key := range in.literalSigValues {
		in.literalSigs.SetInBatch(batch, key, nil)
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
	clear(in.literalSigValues)
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

func (in *factInputs) literalSigFor(ctx *db.QueryContext, key api.LiteralSigKey) (*typ.Function, bool) {
	if in == nil || in.literalSigs == nil {
		return nil, false
	}
	sig, ok := in.literalSigs.Get(ctx, key)
	if !ok || sig == nil {
		return nil, false
	}
	return sig, true
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

func (in *factInputs) setProjectedFacts(batch *db.InputBatch, key api.GraphKey, facts api.Facts) {
	in.setProjectedFunctionFactMap(batch, key, facts.FunctionFacts)
	in.setProjectedFunctionFacts(batch, key, facts.FunctionFacts)
	in.setProjectedLiteralSigs(batch, key, facts.LiteralSigs)
	in.setProjectedCapturedTypes(batch, key, facts.CapturedTypes)
	in.setProjectedCapturedFields(batch, key, facts.CapturedFields)
	in.setProjectedConstructorFields(batch, key, facts.ConstructorFields)
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

func (in *factInputs) setProjectedLiteralSigs(batch *db.InputBatch, key api.GraphKey, sigs api.LiteralSigs) {
	if in == nil || in.literalSigs == nil {
		return
	}
	seen := make(map[api.LiteralSigKey]bool, len(sigs))
	for fn, sig := range sigs {
		inputKey := api.LiteralSigKey{GraphKey: key, Func: fn}
		seen[inputKey] = true
		if prev, ok := in.literalSigValues[inputKey]; ok && value.FactTypeEqual(prev, sig) {
			continue
		}
		in.literalSigValues[inputKey] = sig
		in.literalSigs.SetInBatch(batch, inputKey, sig)
	}
	for inputKey := range in.literalSigValues {
		if inputKey.GraphKey != key || seen[inputKey] {
			continue
		}
		delete(in.literalSigValues, inputKey)
		in.literalSigs.SetInBatch(batch, inputKey, nil)
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
	if s.legacyInterprocPrev != nil && s.legacyInterprocNext != nil {
		prev := functionFactFromFacts(s.legacyInterprocPrev.facts[key], sym)
		next := functionFactFromFacts(s.legacyInterprocNext.facts[key], sym)
		switch {
		case functionfact.Empty(prev) && functionfact.Empty(next):
			return api.FunctionFact{}, false
		case functionfact.Empty(prev):
			return cloneFunctionFact(next), true
		case functionfact.Empty(next):
			return cloneFunctionFact(prev), true
		default:
			merged := interproc.OverlayFacts(
				api.Facts{FunctionFacts: api.FunctionFacts{sym: prev}},
				api.Facts{FunctionFacts: api.FunctionFacts{sym: next}},
			)
			ff := functionFactFromFacts(merged, sym)
			return cloneFunctionFact(ff), !functionfact.Empty(ff)
		}
	}
	return api.FunctionFact{}, false
}

func functionFactFromFacts(facts api.Facts, sym cfg.SymbolID) api.FunctionFact {
	if sym == 0 || len(facts.FunctionFacts) == 0 {
		return api.FunctionFact{}
	}
	return facts.FunctionFacts[sym]
}

func (s *SessionStore) visibleFunctionFacts(key api.GraphKey) api.FunctionFacts {
	if s == nil {
		return nil
	}
	prev := api.Facts{}
	if s.legacyInterprocPrev != nil {
		prev.FunctionFacts = s.legacyInterprocPrev.facts[key].FunctionFacts
	}
	next := api.Facts{}
	if s.legacyInterprocNext != nil {
		next.FunctionFacts = s.legacyInterprocNext.facts[key].FunctionFacts
	}
	return cloneFunctionFacts(interproc.OverlayFacts(prev, next).FunctionFacts)
}

func (s *SessionStore) visibleLiteralSig(key api.GraphKey, fn *ast.FunctionExpr) (*typ.Function, bool) {
	if s == nil || fn == nil {
		return nil, false
	}
	if s.legacyInterprocNext != nil {
		if sig := s.legacyInterprocNext.facts[key].LiteralSigs[fn]; sig != nil {
			return sig, true
		}
	}
	if s.legacyInterprocPrev != nil {
		if sig := s.legacyInterprocPrev.facts[key].LiteralSigs[fn]; sig != nil {
			return sig, true
		}
	}
	return nil, false
}

func (s *SessionStore) visibleCapturedType(key api.GraphKey, sym cfg.SymbolID) (typ.Type, bool) {
	if s == nil || sym == 0 {
		return nil, false
	}
	if s.legacyInterprocPrev != nil && s.legacyInterprocNext != nil {
		prev := capturedTypeFromFacts(s.legacyInterprocPrev.facts[key], sym)
		next := capturedTypeFromFacts(s.legacyInterprocNext.facts[key], sym)
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

func capturedTypeFromFacts(facts api.Facts, sym cfg.SymbolID) product.AbstractValue {
	if sym == 0 || len(facts.CapturedTypes) == 0 {
		return product.AbstractValue{}
	}
	return facts.CapturedTypes[sym]
}

func (s *SessionStore) visibleCapturedFieldAssigns(key api.GraphKey) api.CapturedFieldAssigns {
	if s == nil {
		return nil
	}
	prev := api.Facts{}
	if s.legacyInterprocPrev != nil {
		prev.CapturedFields = s.legacyInterprocPrev.facts[key].CapturedFields
	}
	next := api.Facts{}
	if s.legacyInterprocNext != nil {
		next.CapturedFields = s.legacyInterprocNext.facts[key].CapturedFields
	}
	return cloneCapturedFieldAssigns(interproc.OverlayFacts(prev, next).CapturedFields)
}

func (s *SessionStore) visibleConstructorFields(key api.GraphKey, sym cfg.SymbolID) (api.FieldValues, bool) {
	if s == nil || sym == 0 {
		return nil, false
	}
	if s.legacyInterprocPrev != nil && s.legacyInterprocNext != nil {
		prev := constructorFieldsFromFacts(s.legacyInterprocPrev.facts[key], sym)
		next := constructorFieldsFromFacts(s.legacyInterprocNext.facts[key], sym)
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

func constructorFieldsFromFacts(facts api.Facts, sym cfg.SymbolID) api.FieldValues {
	if sym == 0 || len(facts.ConstructorFields) == 0 {
		return nil
	}
	return facts.ConstructorFields[sym]
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

func (s *SessionStore) literalSigByKey(key api.LiteralSigKey) (*typ.Function, bool) {
	if s == nil || key.Func == nil {
		return nil, false
	}
	if s.factInputs != nil {
		return s.factInputs.literalSigFor(s.factCtx, key)
	}
	return s.visibleLiteralSig(key.GraphKey, key.Func)
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

func (s *SessionStore) syncProjectedFactInputs(batch *db.InputBatch, key api.GraphKey) {
	if s == nil || s.factInputs == nil {
		return
	}
	s.factInputs.setProjectedFacts(batch, key, s.visibleProjectedLegacyFacts(key))
}

// SetCanonicalFunctionFactsProjection publishes final Summary-derived FunctionFacts
// without mutating the legacy prev/next product or advancing the legacy fixpoint
// product.
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

func (s *SessionStore) visibleProjectedLegacyFacts(key api.GraphKey) api.Facts {
	if s == nil {
		return api.Facts{}
	}
	prev := api.Facts{}
	if s.legacyInterprocPrev != nil {
		facts := s.legacyInterprocPrev.facts[key]
		prev = api.Facts{
			FunctionFacts:     facts.FunctionFacts,
			LiteralSigs:       facts.LiteralSigs,
			CapturedTypes:     facts.CapturedTypes,
			CapturedFields:    facts.CapturedFields,
			ConstructorFields: facts.ConstructorFields,
		}
	}
	next := api.Facts{}
	if s.legacyInterprocNext != nil {
		facts := s.legacyInterprocNext.facts[key]
		next = api.Facts{
			FunctionFacts:     facts.FunctionFacts,
			LiteralSigs:       facts.LiteralSigs,
			CapturedTypes:     facts.CapturedTypes,
			CapturedFields:    facts.CapturedFields,
			ConstructorFields: facts.ConstructorFields,
		}
	}
	return interproc.OverlayFacts(prev, next)
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
	for key := range s.factInputs.literalSigValues {
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
	if s.legacyInterprocPrev != nil {
		for key := range s.legacyInterprocPrev.facts {
			factKeys[key] = struct{}{}
		}
	}
	if s.legacyInterprocNext != nil {
		for key := range s.legacyInterprocNext.facts {
			factKeys[key] = struct{}{}
		}
	}
	batch := s.factInputs.database.NewInputBatch()
	for key := range factKeys {
		s.syncProjectedFactInputs(batch, key)
	}
}
