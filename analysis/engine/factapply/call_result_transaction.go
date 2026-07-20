package factapply

import (
	"context"
	"fmt"

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

type CallResultStepKind uint8

const (
	CallResultStepInvalid CallResultStepKind = iota
	CallResultStepValue
	CallResultStepPostconditionRefinement
	CallResultStepPostconditionPathRelation
	CallResultStepReturnPresenceRelation
)

// CallResultStep is one immutable member of the ordered N0/N3/N5 call-result
// transaction. Exactly one payload is selected by Kind.
type CallResultStep struct {
	kind           CallResultStepKind
	value          factflow.CallResultValue
	refinement     factflow.PostconditionRefinement
	pathRelation   factflow.PostconditionPathRelation
	returnPresence factflow.ReturnPresenceRelation
}

func (s CallResultStep) Kind() CallResultStepKind { return s.kind }

func (s CallResultStep) ResultValue() (factflow.CallResultValue, bool) {
	return s.value, s.kind == CallResultStepValue
}

func (s CallResultStep) PostconditionRefinement() (factflow.PostconditionRefinement, bool) {
	if s.kind != CallResultStepPostconditionRefinement {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(s.refinement.TargetPath(), s.refinement.Value()), true
}

func (s CallResultStep) PostconditionPathRelation() (factflow.PostconditionPathRelation, bool) {
	if s.kind != CallResultStepPostconditionPathRelation {
		return factflow.PostconditionPathRelation{}, false
	}
	switch s.pathRelation.Kind() {
	case factflow.PostconditionPathRelationEqual:
		return factflow.NewPostconditionPathEquality(s.pathRelation.LeftPath(), s.pathRelation.RightPath()), true
	default:
		return factflow.PostconditionPathRelation{}, false
	}
}

func (s CallResultStep) ReturnPresenceRelation() (factflow.ReturnPresenceRelation, bool) {
	return s.returnPresence, s.kind == CallResultStepReturnPresenceRelation
}

// CallResultTransaction is the complete point-local result boundary syntax.
// It owns semantic ordering but never State, providers, or solve scratch.
type CallResultTransaction struct {
	point cfg.Point
	steps []CallResultStep
}

func PlanCallResultTransaction(facts factflow.Facts, point cfg.Point) CallResultTransaction {
	values := facts.CallResultValues(point)
	refinements := facts.PostconditionRefinements(point)
	paths := facts.PostconditionPathRelations(point)
	presence := facts.ReturnPresenceRelations(point)
	steps := make([]CallResultStep, 0, len(values)+len(refinements)+len(paths)+len(presence))
	for _, value := range values {
		steps = append(steps, CallResultStep{kind: CallResultStepValue, value: value})
	}
	for _, refinement := range refinements {
		steps = append(steps, CallResultStep{kind: CallResultStepPostconditionRefinement, refinement: refinement})
	}
	for _, relation := range paths {
		steps = append(steps, CallResultStep{kind: CallResultStepPostconditionPathRelation, pathRelation: relation})
	}
	for _, relation := range presence {
		steps = append(steps, CallResultStep{kind: CallResultStepReturnPresenceRelation, returnPresence: relation})
	}
	return CallResultTransaction{point: point, steps: steps}
}

func (t CallResultTransaction) Point() cfg.Point { return t.point }
func (t CallResultTransaction) Len() int         { return len(t.steps) }

// Clone returns a deeply detached copy suitable for sealed N0/N3/N5 syntax.
func (t CallResultTransaction) Clone() CallResultTransaction {
	t.steps = append([]CallResultStep(nil), t.steps...)
	for index := range t.steps {
		t.steps[index] = cloneCallResultStep(t.steps[index])
	}
	return t
}

func cloneCallResultStep(step CallResultStep) CallResultStep {
	switch step.kind {
	case CallResultStepPostconditionRefinement:
		step.refinement = factflow.NewPostconditionRefinement(step.refinement.TargetPath(), step.refinement.Value())
	case CallResultStepPostconditionPathRelation:
		if step.pathRelation.Kind() == factflow.PostconditionPathRelationEqual {
			step.pathRelation = factflow.NewPostconditionPathEquality(step.pathRelation.LeftPath(), step.pathRelation.RightPath())
		}
	}
	return step
}

func (t CallResultTransaction) Step(index int) (CallResultStep, bool) {
	if index < 0 || index >= len(t.steps) {
		return CallResultStep{}, false
	}
	return cloneCallResultStep(t.steps[index]), true
}

func (t CallResultTransaction) HasStateSteps() bool {
	return t.HasMaterializeSteps() || t.HasPostconditionSteps()
}

func (t CallResultTransaction) HasMaterializeSteps() bool {
	for _, step := range t.steps {
		if step.kind == CallResultStepValue {
			return true
		}
	}
	return false
}

func (t CallResultTransaction) HasPostconditionSteps() bool {
	for _, step := range t.steps {
		if step.kind == CallResultStepPostconditionRefinement || step.kind == CallResultStepPostconditionPathRelation {
			return true
		}
	}
	return false
}

func (t CallResultTransaction) HasPublicationSteps() bool {
	for _, step := range t.steps {
		if step.kind == CallResultStepReturnPresenceRelation {
			return true
		}
	}
	return false
}

func (t CallResultTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil {
		return false
	}
	for _, step := range t.steps {
		switch step.kind {
		case CallResultStepValue:
			if step.value.Index() < 0 || !product.BelongsToRegistry(reg, step.value.Value()) {
				return false
			}
		case CallResultStepPostconditionRefinement:
			constraint, hasConstraint := step.refinement.Value().Constraint()
			if step.refinement.TargetPathRef().IsEmpty() || hasConstraint && !product.BelongsToRegistry(reg, constraint) {
				return false
			}
		case CallResultStepPostconditionPathRelation:
			if step.pathRelation.Kind() != factflow.PostconditionPathRelationEqual || step.pathRelation.LeftPath().IsEmpty() || step.pathRelation.RightPath().IsEmpty() {
				return false
			}
		case CallResultStepReturnPresenceRelation:
			if step.returnPresence.TriggerIndex() < 0 || step.returnPresence.TargetIndex() < 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type ConcreteCallResultPhase uint8

const (
	ConcreteCallResultPhaseInvalid ConcreteCallResultPhase = iota
	ConcreteCallResultPhaseMaterialize
	ConcreteCallResultPhasePostconditions
)

type ConcreteCallResultRequest struct {
	Context     transfer.NodeContext
	Resolver    *visibility.Resolver
	ProjectPath PathTypeProjector
	TypeValues  *typevalue.Cache
	Transaction CallResultTransaction
	Phase       ConcreteCallResultPhase
	Output      state.State
}

type ConcreteCallResultResult struct {
	Output   state.State
	Canceled bool
	Err      error
}

// ApplyConcreteCallResultTransaction is the sole State executor for this
// semantic family. Return-presence members are publication syntax and are
// deliberately not interpreted as State updates.
func ApplyConcreteCallResultTransaction(req ConcreteCallResultRequest) ConcreteCallResultResult {
	out := req.Output
	if req.Context.Registry == nil || req.Context.Point != req.Transaction.point || !req.Transaction.Valid(req.Context.Registry) {
		return ConcreteCallResultResult{Output: out}
	}
	materialize := req.Phase == ConcreteCallResultPhaseMaterialize
	postconditions := req.Phase == ConcreteCallResultPhasePostconditions
	if !materialize && !postconditions {
		return ConcreteCallResultResult{Output: out}
	}
	if materialize {
		program, err := prepareConcreteCallResultMaterializeFactorProgram(req.Context.Registry, req.Transaction)
		if err != nil {
			return ConcreteCallResultResult{Output: out}
		}
		residual, values := state.DecomposeValueLane(state.Domain(req.Context.Registry), out)
		ctx := req.Context.Context
		if ctx == nil {
			ctx = context.Background()
		}
		next, err := program.Apply(ctx, tokenOf(req.Context.Session), values)
		if err != nil {
			return ConcreteCallResultResult{Output: req.Output, Canceled: err == context.Canceled || ctx.Err() != nil}
		}
		return ConcreteCallResultResult{Output: state.RecomposeValueLane(req.Context.Registry, state.Domain(req.Context.Registry), residual, next)}
	}
	if postconditions {
		return applyConcreteCallResultPostconditionFactors(req)
	}
	return ConcreteCallResultResult{Output: out}
}

func applyConcreteCallResultPostconditionFactors(req ConcreteCallResultRequest) ConcreteCallResultResult {
	if req.Resolver == nil {
		return ConcreteCallResultResult{Output: req.Output, Err: fmt.Errorf("factapply: call-result N3 requires exact path authority")}
	}
	domain := state.RegisteredProductDomain(req.Context.Registry)
	authority := NewPathSemanticAuthority(req.Resolver, req.ProjectPath, req.TypeValues)
	seed, err := authority.CoordinateFactorInventoryFromPreparedState(domain, req.Output)
	if err != nil {
		return ConcreteCallResultResult{Output: req.Output, Err: err}
	}
	inventory, err := authority.CloseCoordinateFactorInventory(domain, seed)
	if err != nil {
		return ConcreteCallResultResult{Output: req.Output, Err: err}
	}
	program, err := PrepareCallResultPostconditionFactorProgram(
		authority, domain, req.Transaction, inventory,
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() },
		req.TypeValues, req.ProjectPath,
	)
	if err != nil {
		return ConcreteCallResultResult{Output: req.Output, Err: err}
	}
	residual, values := state.DecomposeValueLane(domain.Lattice(), req.Output)
	factors, err := domain.DecomposeLanes(residual, program.Lanes())
	if err != nil {
		return ConcreteCallResultResult{Output: req.Output, Err: err}
	}
	ctx := req.Context.Context
	if ctx == nil {
		ctx = context.Background()
	}
	frame, err := program.Apply(ctx, tokenOf(req.Context.Session), CallResultPostconditionFactorFrame[statekey.Value]{
		Values: values, Factors: factors, Reachable: !domain.Lattice().Equal(req.Output, domain.Lattice().Bottom()),
	})
	if err != nil {
		canceled := err == context.Canceled || ctx.Err() != nil || req.Context.Session != nil && req.Context.Session.Token().Canceled()
		return ConcreteCallResultResult{Output: req.Output, Canceled: canceled, Err: err}
	}
	if !frame.Reachable {
		return ConcreteCallResultResult{Output: domain.Lattice().Bottom()}
	}
	delta, err := domain.ComposeSparse(frame.Factors)
	if err != nil {
		return ConcreteCallResultResult{Output: req.Output, Err: err}
	}
	ids := make([]state.LaneID, len(program.Lanes()))
	for index, lane := range program.Lanes() {
		ids[index] = lane.ID()
	}
	residual, err = domain.PatchFactors(residual, delta, state.NewLaneSet(ids...))
	if err != nil {
		return ConcreteCallResultResult{Output: req.Output, Err: err}
	}
	return ConcreteCallResultResult{Output: state.RecomposeValueLane(req.Context.Registry, domain.Lattice(), residual, frame.Values)}
}

func (a *PathSemanticAuthority) ApplyCallResultPhase(ctx context.Context, reg *axis.Registry, transaction CallResultTransaction, phase ConcreteCallResultPhase, input state.State) (state.State, error) {
	if ctx == nil || reg == nil || !a.Valid() || !transaction.Valid(reg) ||
		phase != ConcreteCallResultPhaseMaterialize && phase != ConcreteCallResultPhasePostconditions {
		return state.State{}, fmt.Errorf("factapply: invalid call-result path authority")
	}
	session := cancellation.FromContext(ctx)
	result := ApplyConcreteCallResultTransaction(ConcreteCallResultRequest{
		Context:  transfer.NodeContext{Context: ctx, Session: session, Registry: reg, Point: transaction.point},
		Resolver: a.resolver, ProjectPath: a.projectPath, Transaction: transaction,
		TypeValues: a.typeValues, Phase: phase, Output: input,
	})
	if result.Err != nil {
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
