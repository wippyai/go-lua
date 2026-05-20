package store

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/types/db"
)

// factInputs are the Salsa-style source inputs for interprocedural reads.
// They snapshot the store's visible fact product so FuncResult queries depend
// on the exact graph fact products they read.
type factInputs struct {
	database *db.DB

	facts      *db.Input[api.GraphKey, api.Facts]
	factValues map[api.GraphKey]api.Facts
}

func newFactInputs(database *db.DB) *factInputs {
	if database == nil {
		return nil
	}
	return &factInputs{
		database:   database,
		facts:      db.NewInput[api.GraphKey, api.Facts](database),
		factValues: make(map[api.GraphKey]api.Facts),
	}
}

func (in *factInputs) reset() {
	if in == nil || in.database == nil {
		return
	}
	for key := range in.factValues {
		in.facts.Set(key, api.Facts{})
	}
	clear(in.factValues)
}

func (in *factInputs) factsFor(ctx *db.QueryContext, key api.GraphKey) (api.Facts, bool) {
	if in == nil || in.facts == nil {
		return api.Facts{}, false
	}
	facts, ok := in.facts.Get(ctx, key)
	if !ok {
		return api.Facts{}, false
	}
	return cloneFacts(facts), true
}

func (in *factInputs) setFacts(key api.GraphKey, facts api.Facts) {
	if in == nil || in.facts == nil {
		return
	}
	if interproc.Empty(facts) {
		if _, ok := in.factValues[key]; !ok {
			return
		}
		delete(in.factValues, key)
		in.facts.Set(key, api.Facts{})
		return
	}
	next := cloneFacts(facts)
	if prev, ok := in.factValues[key]; ok && interproc.FactsEqual(prev, next) {
		return
	}
	in.factValues[key] = next
	in.facts.Set(key, next)
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

func (s *SessionStore) visibleInterprocFacts(key api.GraphKey) api.Facts {
	if s == nil {
		return api.Facts{}
	}
	var prev api.Facts
	if s.InterprocPrev != nil && s.InterprocPrev.Facts != nil {
		prev = s.InterprocPrev.Facts[key]
	}
	if s.InterprocNext != nil && s.InterprocNext.Facts != nil {
		if next, ok := s.InterprocNext.Facts[key]; ok {
			if interproc.Empty(prev) {
				return cloneFacts(next)
			}
			if interproc.Empty(next) {
				return cloneFacts(prev)
			}
			return cloneFacts(interproc.OverlayFacts(prev, next))
		}
	}
	return cloneFacts(prev)
}

func (s *SessionStore) interprocFactsByKey(key api.GraphKey) api.Facts {
	if s == nil {
		return api.Facts{}
	}
	if s.factInputs != nil {
		if facts, ok := s.factInputs.factsFor(s.factCtx, key); ok {
			return facts
		}
		return api.Facts{}
	}
	return s.visibleInterprocFacts(key)
}

func (s *SessionStore) syncFactsInput(key api.GraphKey) {
	if s == nil || s.factInputs == nil {
		return
	}
	s.factInputs.setFacts(key, s.visibleInterprocFacts(key))
}

func (s *SessionStore) syncFactInputs() {
	if s == nil || s.factInputs == nil {
		return
	}

	factKeys := make(map[api.GraphKey]struct{}, len(s.factInputs.factValues))
	for key := range s.factInputs.factValues {
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

}
