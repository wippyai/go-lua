package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// EffectFacts provides function effect lookup.
type EffectFacts interface {
	LookupBySym(sym cfg.SymbolID) *constraint.FunctionEffect
}

// EffectStore provides methods for storing and retrieving function effects.
type EffectStore interface {
	LookupEffectBySym(sym cfg.SymbolID) *constraint.FunctionEffect
}

// storeEffectFacts implements EffectFacts backed by an EffectStore.
type storeEffectFacts struct {
	store EffectStore
}

// NewEffectFacts creates an EffectFacts backed by an EffectStore.
func NewEffectFacts(store EffectStore) EffectFacts {
	if store == nil {
		return nilEffectFacts{}
	}
	return &storeEffectFacts{store: store}
}

func (f *storeEffectFacts) LookupBySym(sym cfg.SymbolID) *constraint.FunctionEffect {
	if f.store == nil || sym == 0 {
		return nil
	}
	return f.store.LookupEffectBySym(sym)
}

// nilEffectFacts is a no-op EffectFacts implementation.
type nilEffectFacts struct{}

func (nilEffectFacts) LookupBySym(cfg.SymbolID) *constraint.FunctionEffect { return nil }
