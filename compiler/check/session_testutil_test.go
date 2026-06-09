package check

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/typ"
)

func liftConstructorFieldsForTest(fields map[cfg.SymbolID]map[string]typ.Type) api.ConstructorFields {
	out := make(api.ConstructorFields, len(fields))
	for sym, byName := range fields {
		out[sym] = interprocdomain.LiftTypeFieldMap(byName)
	}
	return out
}

func setConstructorFieldsNextForTest(s *store.SessionStore, fields map[cfg.SymbolID]map[string]typ.Type) {
	if s == nil {
		return
	}
	if s.LegacyInterprocNext == nil {
		s.LegacyInterprocNext = store.NewLegacyInterprocState()
	}
	s.LegacyInterprocNext.Facts[api.ModuleFactsKey()] = api.Facts{ConstructorFields: liftConstructorFieldsForTest(fields)}
}

func setConstructorFieldsPrevForTest(s *store.SessionStore, fields map[cfg.SymbolID]map[string]typ.Type) {
	if s == nil {
		return
	}
	if s.LegacyInterprocPrev == nil {
		s.LegacyInterprocPrev = store.NewLegacyInterprocState()
	}
	s.LegacyInterprocPrev.Facts[api.ModuleFactsKey()] = api.Facts{ConstructorFields: liftConstructorFieldsForTest(fields)}
}
