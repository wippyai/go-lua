package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// PointWriter mutates one PointState with normalized, low-level writes.
//
// It owns only primitive mechanics: write Env, write Cells, clear stale Env on
// cell writes, and optionally record cell effects. The caller must supply the
// storage-class decision; lexical policy stays in canonical transfer.
type PointWriter struct {
	state *PointState
}

// NewPointWriter returns a writer over state.
func NewPointWriter(state *PointState) PointWriter {
	return PointWriter{state: state}
}

func (w PointWriter) detachEnvForMutation() {
	if w.state == nil {
		return
	}
	if w.state.envShared {
		w.state.Env = cloneEnv(w.state.Env)
		w.state.envShared = false
	}
	if w.state.Env == nil {
		w.state.Env = make(map[ValueKey]product.AbstractValue)
	}
}

// DeleteValueKey removes a non-axis Env value by typed key.
func (w PointWriter) DeleteValueKey(key ValueKey) bool {
	if w.state == nil || w.state.Env == nil {
		return false
	}
	if _, had := w.state.Env[key]; !had {
		return false
	}
	w.detachEnvForMutation()
	delete(w.state.Env, key)
	return true
}

// DeleteSymbolEnvValue removes sym's primitive Env entry without touching Cells.
func (w PointWriter) DeleteSymbolEnvValue(sym cfg.SymbolID) bool {
	if sym == 0 {
		return false
	}
	return w.DeleteValueKey(SymbolValueKey(sym))
}

// WriteValueKey writes an abstract value under a typed Env key.
//
// This is the primitive for non-symbol values such as return slots. Symbol
// writes that may target captured Cells should use WriteSymbolValue instead.
func (w PointWriter) WriteValueKey(key ValueKey, val product.AbstractValue, joinExisting bool) bool {
	if w.state == nil || key == "" {
		return false
	}
	if joinExisting {
		if prev, had := w.state.Env[key]; had {
			val = product.Domain.Join(prev, val)
		}
	}
	if val.IsZero() || val.IsBottom() {
		return w.DeleteValueKey(key)
	}
	if prev, had := w.state.Env[key]; had && product.Domain.Equal(prev, val) {
		return false
	}
	w.detachEnvForMutation()
	w.state.Env[key] = val
	return true
}

// WriteSymbolValue writes an abstract value for sym into Env or Cells.
//
// When toCells is true, the write updates Cells and deletes any stale Env
// entry for the same symbol. Otherwise it updates Env under SymbolValueKey. This
// method does not infer whether sym is captured; callers pass toCells and
// emitCellEffect from their own policy layer. When joinExisting is true, the new
// value is joined with the existing value from the relevant target location
// first.
func (w PointWriter) WriteSymbolValue(sym cfg.SymbolID, val product.AbstractValue, toCells bool, joinExisting bool, emitCellEffect bool) {
	if w.state == nil || sym == 0 {
		return
	}

	if toCells {
		if joinExisting {
			if prev, ok := SymbolValue(*w.state, sym); ok {
				val = product.Domain.Join(prev, val)
			}
		}
		w.state.Cells = w.state.Cells.With(sym, val)
		if emitCellEffect {
			w.state.CellEffects = w.state.CellEffects.WithMustWrite(sym, val)
		}
		w.DeleteValueKey(SymbolValueKey(sym))
		return
	}

	key := SymbolValueKey(sym)
	if joinExisting {
		if prev, had := w.state.Env[key]; had {
			val = product.Domain.Join(prev, val)
		}
	}
	if val.IsZero() || val.IsBottom() {
		w.DeleteValueKey(key)
		return
	}
	if prev, had := w.state.Env[key]; had && product.Domain.Equal(prev, val) {
		return
	}
	w.detachEnvForMutation()
	w.state.Env[key] = val
}
