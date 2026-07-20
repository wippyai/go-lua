package factapply

import (
	"context"
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	typecovariant "github.com/wippyai/go-lua/analysis/type/covariant"
)

// CovariantExposureStep is one immutable N6 exposure. The factflow payload
// owns a detached source path, and Exposure returns another detached value so
// callers cannot mutate transaction storage.
type CovariantExposureStep struct {
	exposure factflow.CovariantExposure
}

func (s CovariantExposureStep) Exposure() factflow.CovariantExposure {
	return factflow.NewCovariantExposure(s.exposure.SourcePath(), s.exposure.WideValue(), s.exposure.Kind())
}

// CovariantExposureTransaction is the complete immutable N6 finalizer program
// for one CFG point. It owns only ordered semantic syntax; State, stable path
// visibility, type-widening authority, cancellation, and solve-local context
// belong to its executor.
type CovariantExposureTransaction struct {
	point cfg.Point
	steps []CovariantExposureStep
}

// PlanCovariantExposureTransaction freezes the exact factflow exposure order
// consumed by the canonical N6 finalizer.
func PlanCovariantExposureTransaction(facts factflow.Facts, point cfg.Point) CovariantExposureTransaction {
	exposures := facts.CovariantExposures(point)
	steps := make([]CovariantExposureStep, len(exposures))
	for index, exposure := range exposures {
		steps[index] = CovariantExposureStep{exposure: exposure}
	}
	return CovariantExposureTransaction{point: point, steps: steps}
}

func (t CovariantExposureTransaction) Point() cfg.Point { return t.point }
func (t CovariantExposureTransaction) Len() int         { return len(t.steps) }
func (t CovariantExposureTransaction) Clone() CovariantExposureTransaction {
	out := CovariantExposureTransaction{point: t.point, steps: make([]CovariantExposureStep, len(t.steps))}
	for index, step := range t.steps {
		out.steps[index] = CovariantExposureStep{exposure: step.Exposure()}
	}
	return out
}

func (t CovariantExposureTransaction) HasStateSteps() bool { return len(t.steps) != 0 }

// Step returns one immutable transaction member without exposing the backing
// slice or the payload's source-path storage.
func (t CovariantExposureTransaction) Step(index int) (CovariantExposureStep, bool) {
	if index < 0 || index >= len(t.steps) {
		return CovariantExposureStep{}, false
	}
	step := t.steps[index]
	return CovariantExposureStep{exposure: step.Exposure()}, true
}

// Valid reports whether every embedded product belongs to reg. Source-path and
// kind edge cases deliberately remain executable: the concrete N6 primitive
// historically treats a zero source as a no-op and every non-array kind as the
// record lane, so rejecting either here would change established behavior.
func (t CovariantExposureTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil {
		return false
	}
	for _, step := range t.steps {
		if !product.BelongsToRegistry(reg, step.exposure.WideValue()) {
			return false
		}
	}
	return true
}

// ConcreteCovariantExposureRequest supplies the execution authority which is
// deliberately absent from CovariantExposureTransaction. Input is the immutable
// point-entry snapshot used for whole-node cancellation rollback; Output is the
// completed N0..N5 state observed and evolved by N6.
type ConcreteCovariantExposureRequest struct {
	Context        transfer.NodeContext
	Resolver       *visibility.Resolver
	CovariantWiden CovariantWiden
	Transaction    CovariantExposureTransaction
	Input          state.State
	Output         state.State
}

type ConcreteCovariantExposureResult struct {
	Output   state.State
	Canceled bool
	Err      error
}

// ApplyConcreteCovariantExposureTransaction is the sole State executor for the
// ordered N6 covariant-exposure family. Every exposure observes the evolving
// completed point. Cancellation never publishes an N6 prefix and rolls the
// enclosing node back to its immutable point Input.
func ApplyConcreteCovariantExposureTransaction(req ConcreteCovariantExposureRequest) ConcreteCovariantExposureResult {
	if req.Context.Registry == nil || req.Context.Point != req.Transaction.point || !req.Transaction.Valid(req.Context.Registry) {
		return ConcreteCovariantExposureResult{Output: req.Output, Err: fmt.Errorf("factapply: invalid concrete covariant transaction")}
	}
	ctx := req.Context.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ConcreteCovariantExposureResult{Output: req.Input, Canceled: true, Err: err}
	}
	domain := state.RegisteredProductDomain(req.Context.Registry)
	topology, err := domain.SealCovariantFactorTopology()
	if err != nil {
		return ConcreteCovariantExposureResult{Output: req.Output, Err: err}
	}
	lanes := topology.Lanes()
	factors, err := domain.DecomposeLanes(req.Output, lanes)
	if err != nil {
		return ConcreteCovariantExposureResult{Output: req.Output, Err: err}
	}
	_, values := state.DecomposeValueLane(state.Domain(req.Context.Registry), req.Output)
	bindings := make([]CovariantFactorBinding[statekey.Value], req.Transaction.Len())
	var keys *keyspace.KeySpace
	if req.Resolver != nil {
		keys = req.Resolver.KeySpace()
	}
	for index := range bindings {
		step, _ := req.Transaction.Step(index)
		path := step.exposure.SourcePath()
		if path.Symbol == 0 {
			bindings[index] = CovariantFactorBinding[statekey.Value]{Kind: CovariantFactorBindingNoop}
			continue
		}
		bindings[index] = CovariantFactorBinding[statekey.Value]{
			Kind: CovariantFactorBindingValues, Source: statekey.SymbolValue(path.Symbol),
		}
		if req.Resolver == nil {
			continue
		}
		root, ok := visibility.AddressAt(req.Resolver, req.Context.Point, pathdom.NewPath(path.Symbol, "")).VisibleKeyspaceKey()
		if !ok {
			return ConcreteCovariantExposureResult{Output: req.Output, Err: fmt.Errorf("factapply: covariant source has no visible root")}
		}
		bindings[index] = CovariantFactorBinding[statekey.Value]{
			Kind: CovariantFactorBindingStructural, Source: statekey.SymbolValue(path.Symbol), Root: root,
		}
	}
	factored, err := ApplyCovariantExposureFactors(ctx, req.CovariantWiden, CovariantFactorTransaction[statekey.Value]{
		Transaction: req.Transaction, Bindings: bindings, Values: values, Factors: factors,
		Domain: domain, Keys: keys, Topology: topology, Token: tokenOf(req.Context.Session),
	})
	if err != nil {
		canceled := ctx.Err() != nil || tokenOf(req.Context.Session) != nil && tokenOf(req.Context.Session).Canceled()
		if canceled {
			return ConcreteCovariantExposureResult{Output: req.Input, Canceled: true, Err: err}
		}
		return ConcreteCovariantExposureResult{Output: req.Output, Err: err}
	}
	delta, err := domain.ComposeSparse(factored.Factors)
	if err != nil {
		return ConcreteCovariantExposureResult{Output: req.Output, Err: err}
	}
	ids := make([]state.LaneID, len(lanes))
	for index, lane := range lanes {
		ids[index] = lane.ID()
	}
	out, err := domain.PatchFactors(req.Output, delta, state.NewLaneSet(ids...))
	if err != nil {
		return ConcreteCovariantExposureResult{Output: req.Output, Err: err}
	}
	out = state.RecomposeValueLane(req.Context.Registry, state.Domain(req.Context.Registry), out, factored.Values)
	return ConcreteCovariantExposureResult{Output: out}
}

// ApplyCovariantExposure executes one frozen N6 finalizer through the shared
// callback-free record-widening authority. Input is the immutable point-entry
// rollback state; Output is the completed N0..N5 state observed by N6.
func (a *PathSemanticAuthority) ApplyCovariantExposure(ctx context.Context, reg *axis.Registry, transaction CovariantExposureTransaction, input, output state.State) (state.State, error) {
	if ctx == nil || reg == nil || !a.Valid() || !transaction.Valid(reg) {
		return state.State{}, fmt.Errorf("factapply: invalid covariant-exposure path authority")
	}
	result := ApplyConcreteCovariantExposureTransaction(ConcreteCovariantExposureRequest{
		Context: transfer.NodeContext{
			Context: ctx, Session: cancellation.FromContext(ctx), Registry: reg, Point: transaction.point,
		},
		Resolver: a.resolver, CovariantWiden: typecovariant.WidenRecord,
		Transaction: transaction, Input: input, Output: output,
	})
	if result.Err != nil && !result.Canceled {
		return input, result.Err
	}
	if result.Canceled {
		if err := ctx.Err(); err != nil {
			return input, err
		}
		return input, context.Canceled
	}
	return result.Output, nil
}
