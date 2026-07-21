package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
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

// CallResultPhase separates materialization from N3 postcondition application.
// It is shared immutable formal syntax, not a State execution mode.
type CallResultPhase uint8

const (
	CallResultPhaseInvalid CallResultPhase = iota
	CallResultPhaseMaterialize
	CallResultPhasePostconditions
)
