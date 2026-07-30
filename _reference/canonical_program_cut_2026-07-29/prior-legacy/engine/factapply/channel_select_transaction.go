package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ChannelSelectStep is one immutable N3 channel-select publication. Its path
// payload remains encapsulated by factflow.ChannelSelect's detached accessors.
type ChannelSelectStep struct {
	event factflow.ChannelSelect
}

// Event returns the immutable event value. Its path accessors return detached
// paths and therefore cannot mutate transaction storage.
func (s ChannelSelectStep) Event() factflow.ChannelSelect { return s.event }

// ChannelSelectTransaction is the complete ordered N3 channel-select
// publication program for one CFG point. It owns syntax only: State,
// resolution authority, cancellation, and solve-local scratch are supplied by
// its executor.
type ChannelSelectTransaction struct {
	point cfg.Point
	steps []ChannelSelectStep
}

// PlanChannelSelectTransaction freezes the exact factflow publication order
// used by the canonical point transaction.
func PlanChannelSelectTransaction(facts factflow.Facts, point cfg.Point) ChannelSelectTransaction {
	events := facts.ChannelSelects(point)
	steps := make([]ChannelSelectStep, len(events))
	for index, event := range events {
		steps[index] = ChannelSelectStep{event: event}
	}
	return ChannelSelectTransaction{point: point, steps: steps}
}

func (t ChannelSelectTransaction) Point() cfg.Point { return t.point }
func (t ChannelSelectTransaction) Len() int         { return len(t.steps) }
func (t ChannelSelectTransaction) Clone() ChannelSelectTransaction {
	out := ChannelSelectTransaction{point: t.point, steps: make([]ChannelSelectStep, len(t.steps))}
	copy(out.steps, t.steps)
	return out
}

func (t ChannelSelectTransaction) HasPublicationSteps() bool { return len(t.steps) != 0 }

// Step returns one immutable publication without exposing the backing slice.
func (t ChannelSelectTransaction) Step(index int) (ChannelSelectStep, bool) {
	if index < 0 || index >= len(t.steps) {
		return ChannelSelectStep{}, false
	}
	return t.steps[index], true
}

// Valid reports whether every publication has a supported event tag and any
// embedded payload belongs to the transaction's value registry.
func (t ChannelSelectTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil {
		return false
	}
	for _, step := range t.steps {
		event := step.event
		switch event.Kind() {
		case factflow.ChannelSelectSelect, factflow.ChannelSelectReceive, factflow.ChannelSelectCase:
		default:
			return false
		}
		if payload, ok := event.PayloadValue(); ok && !product.BelongsToRegistry(reg, payload) {
			return false
		}
	}
	return true
}
