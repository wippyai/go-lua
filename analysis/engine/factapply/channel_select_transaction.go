package factapply

import (
	"context"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
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

// ConcreteChannelSelectRequest supplies the execution authority deliberately
// absent from ChannelSelectTransaction.
type ConcreteChannelSelectRequest struct {
	Context     transfer.NodeContext
	Resolver    *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
	Transaction ChannelSelectTransaction
	Output      state.State
}

type ConcreteChannelSelectResult struct {
	Output   state.State
	Canceled bool
}

// ApplyConcreteChannelSelectTransaction is the sole State executor for the N3
// publication family. Cancellation rolls back this transaction to Output; the
// enclosing node transaction may additionally roll the whole node back.
func ApplyConcreteChannelSelectTransaction(req ConcreteChannelSelectRequest) ConcreteChannelSelectResult {
	if req.Context.Registry == nil || req.Context.Point != req.Transaction.point || !req.Transaction.Valid(req.Context.Registry) {
		return ConcreteChannelSelectResult{Output: req.Output}
	}
	token := tokenOf(req.Context.Session)
	if token != nil && token.Canceled() {
		return ConcreteChannelSelectResult{Output: req.Output, Canceled: true}
	}
	prepared, err := PrepareChannelSelectTransaction(req.Context.Registry, req.Transaction,
		func(path pathdom.Path) (pathaddr.StateKey, bool) {
			return visibility.AddressAt(req.Resolver, req.Context.Point, path).VisibleStateKey()
		},
		func(point cfg.Point, index int) (statekey.Value, bool) {
			if index < 0 {
				return 0, false
			}
			return statekey.CallResult(uint32(point), uint32(index)), true
		},
	)
	if err != nil {
		return ConcreteChannelSelectResult{Output: req.Output}
	}
	ctx := req.Context.Context
	if ctx == nil {
		ctx = context.Background()
	}
	evaluated, err := EvaluatePreparedChannelSelect(ctx, req.Context.Registry, req.TypeValues, prepared,
		func(path PreparedChannelSelectPath) (product.Value, bool) {
			payload, ok := channelSelectCasePathPayloadType(req.Context, req.TypeValues, req.Resolver, req.ProjectPath, req.Output, path.SourcePath())
			if !ok {
				return product.Value{}, false
			}
			return req.TypeValues.FromTypeWithWitness(req.Context.Registry, payload), true
		},
	)
	if err != nil {
		if token != nil && token.Canceled() {
			return ConcreteChannelSelectResult{Output: req.Output, Canceled: true}
		}
		return ConcreteChannelSelectResult{Output: req.Output}
	}
	out := req.Output
	for _, fact := range evaluated.Facts() {
		out = out.AddChannelSelectFact(fact)
	}
	writes := evaluated.ResultWrites()
	if len(writes) != 0 {
		edit := out.EditValues(req.Context.Registry)
		for _, write := range writes {
			edit.Write(write.Target, write.Value)
		}
		out = edit.Done()
	}
	return ConcreteChannelSelectResult{Output: out}
}

// ApplyChannelSelect executes one frozen N3 publication transaction with the
// prepared body's stable path-resolution authority.
func (a *PathSemanticAuthority) ApplyChannelSelect(ctx context.Context, reg *axis.Registry, transaction ChannelSelectTransaction, input state.State) (state.State, error) {
	if ctx == nil || reg == nil || !a.Valid() || !transaction.Valid(reg) {
		return state.State{}, fmt.Errorf("factapply: invalid channel-select path authority")
	}
	session := cancellation.FromContext(ctx)
	result := ApplyConcreteChannelSelectTransaction(ConcreteChannelSelectRequest{
		Context: transfer.NodeContext{
			Context: ctx, Session: session, Registry: reg, Point: transaction.point,
		},
		Resolver: a.resolver, ProjectPath: a.projectPath, TypeValues: a.typeValues,
		Transaction: transaction, Output: input,
	})
	if result.Canceled {
		if err := ctx.Err(); err != nil {
			return input, err
		}
		return input, context.Canceled
	}
	return result.Output, nil
}
