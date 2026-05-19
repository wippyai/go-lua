package store

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/factproduct"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

// snapshotInputs are the Salsa-style source inputs for interprocedural reads.
// They mirror the store's read snapshots, so FuncResult queries depend on the
// exact graph facts, refinement symbols, and constructor symbols they read.
type snapshotInputs struct {
	database *db.DB

	facts      *db.Input[api.GraphKey, api.Facts]
	factValues map[api.GraphKey]api.Facts

	refinements      *db.Input[cfg.SymbolID, *constraint.FunctionRefinement]
	refinementValues map[cfg.SymbolID]*constraint.FunctionRefinement

	constructorFields      *db.Input[cfg.SymbolID, map[string]typ.Type]
	constructorFieldValues map[cfg.SymbolID]map[string]typ.Type
}

func newSnapshotInputs(database *db.DB) *snapshotInputs {
	if database == nil {
		return nil
	}
	return &snapshotInputs{
		database:               database,
		facts:                  db.NewInput[api.GraphKey, api.Facts](database),
		factValues:             make(map[api.GraphKey]api.Facts),
		refinements:            db.NewInput[cfg.SymbolID, *constraint.FunctionRefinement](database),
		refinementValues:       make(map[cfg.SymbolID]*constraint.FunctionRefinement),
		constructorFields:      db.NewInput[cfg.SymbolID, map[string]typ.Type](database),
		constructorFieldValues: make(map[cfg.SymbolID]map[string]typ.Type),
	}
}

func (in *snapshotInputs) reset() {
	if in == nil || in.database == nil {
		return
	}
	for key := range in.factValues {
		in.facts.Set(key, api.Facts{})
	}
	clear(in.factValues)
	for sym := range in.refinementValues {
		in.refinements.Set(sym, nil)
	}
	clear(in.refinementValues)
	for sym := range in.constructorFieldValues {
		in.constructorFields.Set(sym, nil)
	}
	clear(in.constructorFieldValues)
}

func (in *snapshotInputs) factsFor(ctx *db.QueryContext, key api.GraphKey) (api.Facts, bool) {
	if in == nil || in.facts == nil {
		return api.Facts{}, false
	}
	facts, ok := in.facts.Get(ctx, key)
	if !ok {
		return api.Facts{}, false
	}
	return cloneFacts(facts), true
}

func (in *snapshotInputs) setFacts(key api.GraphKey, facts api.Facts) {
	if in == nil || in.facts == nil {
		return
	}
	if factsEmpty(facts) {
		if _, ok := in.factValues[key]; !ok {
			return
		}
		delete(in.factValues, key)
		in.facts.Set(key, api.Facts{})
		return
	}
	next := cloneFacts(facts)
	if prev, ok := in.factValues[key]; ok && factproduct.FactsEqual(prev, next) {
		return
	}
	in.factValues[key] = next
	in.facts.Set(key, next)
}

func (in *snapshotInputs) refinement(ctx *db.QueryContext, sym cfg.SymbolID) (*constraint.FunctionRefinement, bool) {
	if in == nil || in.refinements == nil {
		return nil, false
	}
	return in.refinements.Get(ctx, sym)
}

func (in *snapshotInputs) setRefinement(sym cfg.SymbolID, refinement *constraint.FunctionRefinement) {
	if in == nil || in.refinements == nil || sym == 0 {
		return
	}
	if refinement == nil {
		if _, ok := in.refinementValues[sym]; !ok {
			return
		}
		delete(in.refinementValues, sym)
		in.refinements.Set(sym, nil)
		return
	}
	if prev, ok := in.refinementValues[sym]; ok && effectsEqual(prev, refinement) {
		return
	}
	in.refinementValues[sym] = refinement
	in.refinements.Set(sym, refinement)
}

func (in *snapshotInputs) constructorFieldsFor(
	ctx *db.QueryContext,
	sym cfg.SymbolID,
) (map[string]typ.Type, bool) {
	if in == nil || in.constructorFields == nil {
		return nil, false
	}
	return in.constructorFields.Get(ctx, sym)
}

func (in *snapshotInputs) setConstructorFields(sym cfg.SymbolID, fields map[string]typ.Type) {
	if in == nil || in.constructorFields == nil || sym == 0 {
		return
	}
	if len(fields) == 0 {
		if _, ok := in.constructorFieldValues[sym]; !ok {
			return
		}
		delete(in.constructorFieldValues, sym)
		in.constructorFields.Set(sym, nil)
		return
	}
	next := cloneFieldTypes(fields)
	if prev, ok := in.constructorFieldValues[sym]; ok && constructorFieldMapsEqual(sym, prev, next) {
		return
	}
	in.constructorFieldValues[sym] = next
	in.constructorFields.Set(sym, next)
}

func cloneFieldTypes(src map[string]typ.Type) map[string]typ.Type {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(src))
	for name, t := range src {
		out[name] = t
	}
	return out
}

func constructorFieldMapsEqual(sym cfg.SymbolID, a, b map[string]typ.Type) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return factproduct.ConstructorFieldsEqual(
		api.ConstructorFields{sym: a},
		api.ConstructorFields{sym: b},
	)
}

func (s *SessionStore) PushSnapshotReadContext(ctx *db.QueryContext) func() {
	if s == nil || ctx == nil || s.snapshotInputs == nil {
		return func() {}
	}
	prev := s.snapshotCtx
	s.snapshotCtx = ctx
	return func() {
		s.snapshotCtx = prev
	}
}

func (s *SessionStore) currentInterprocFacts(key api.GraphKey) api.Facts {
	if s == nil {
		return api.Facts{}
	}
	var prev api.Facts
	if s.InterprocPrev != nil && s.InterprocPrev.Facts != nil {
		prev = s.InterprocPrev.Facts[key]
	}
	if s.InterprocNext != nil && s.InterprocNext.Facts != nil {
		if next, ok := s.InterprocNext.Facts[key]; ok {
			if factsEmpty(prev) {
				return cloneFacts(next)
			}
			if factsEmpty(next) {
				return cloneFacts(prev)
			}
			return cloneFacts(factproduct.JoinFacts(prev, next))
		}
	}
	return cloneFacts(prev)
}

func (s *SessionStore) syncFactsInput(key api.GraphKey) {
	if s == nil || s.snapshotInputs == nil {
		return
	}
	s.snapshotInputs.setFacts(key, s.currentInterprocFacts(key))
}

func (s *SessionStore) syncSnapshotInputs() {
	if s == nil || s.snapshotInputs == nil {
		return
	}

	factKeys := make(map[api.GraphKey]struct{}, len(s.snapshotInputs.factValues))
	for key := range s.snapshotInputs.factValues {
		factKeys[key] = struct{}{}
	}
	if s.InterprocPrev != nil {
		for key := range s.InterprocPrev.Facts {
			factKeys[key] = struct{}{}
		}
	}
	if s.InterprocNext != nil {
		for key := range s.InterprocNext.Facts {
			factKeys[key] = struct{}{}
		}
	}
	for key := range factKeys {
		s.syncFactsInput(key)
	}

	refinementSyms := make(map[cfg.SymbolID]struct{}, len(s.snapshotInputs.refinementValues))
	for sym := range s.snapshotInputs.refinementValues {
		refinementSyms[sym] = struct{}{}
	}
	if s.InterprocPrev != nil {
		for sym := range s.InterprocPrev.Refinements {
			refinementSyms[sym] = struct{}{}
		}
	}
	for sym := range refinementSyms {
		var refinement *constraint.FunctionRefinement
		if s.InterprocPrev != nil {
			refinement = s.InterprocPrev.Refinements[sym]
		}
		s.snapshotInputs.setRefinement(sym, refinement)
	}

	constructorSyms := make(map[cfg.SymbolID]struct{}, len(s.snapshotInputs.constructorFieldValues))
	for sym := range s.snapshotInputs.constructorFieldValues {
		constructorSyms[sym] = struct{}{}
	}
	if s.InterprocPrev != nil {
		for sym := range s.InterprocPrev.ConstructorFields {
			constructorSyms[sym] = struct{}{}
		}
	}
	for sym := range constructorSyms {
		var fields map[string]typ.Type
		if s.InterprocPrev != nil {
			fields = s.InterprocPrev.ConstructorFields[sym]
		}
		s.snapshotInputs.setConstructorFields(sym, fields)
	}
}
