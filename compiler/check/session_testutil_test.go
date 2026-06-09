package check

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/domain/postflow"
	"github.com/wippyai/go-lua/compiler/check/store"
	"github.com/wippyai/go-lua/types/typ"
)

func liftConstructorFieldsForTest(fields map[cfg.SymbolID]map[string]typ.Type) postflow.ConstructorFields {
	out := make(postflow.ConstructorFields, len(fields))
	for sym, byName := range fields {
		out[sym] = interprocdomain.LiftTypeFieldMap(byName)
	}
	return out
}

func setConstructorFieldsNextForTest(s *store.SessionStore, fields map[cfg.SymbolID]map[string]typ.Type) {
	if s == nil {
		return
	}
	for sym, byField := range liftConstructorFieldsForTest(fields) {
		s.MergeConstructorFieldProjection(sym, byField)
	}
}

func setConstructorFieldsPrevForTest(s *store.SessionStore, fields map[cfg.SymbolID]map[string]typ.Type) {
	if s == nil {
		return
	}
	for sym, byField := range liftConstructorFieldsForTest(fields) {
		s.MergeConstructorFieldProjection(sym, byField)
	}
	s.AdvancePostflowProjections()
}
