package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

type ResultReader interface {
	Registry() *axis.Registry
	Graph() cfg.Graph
	ExitState() (state.State, bool)
	ReturnPoints() []cfg.Point
	ReturnArity(cfg.Point) (int, bool)
}

type entryStateReader interface {
	EntryState() (state.State, bool)
}

type parameterValueSlotReader interface {
	ParameterValueSlots() []key.Value
}

type reassignedParameterValueSlotReader interface {
	ReassignedParameterValueSlots() map[key.Value]struct{}
}

type stateAtReader interface {
	StateAt(cfg.Point) (state.State, bool)
}

type branchConditionReader interface {
	BranchCondition(cfg.Point) (semantics.BranchConditionFact, bool)
}

type noNormalReturnReader interface {
	NoNormalReturn(cfg.Point) bool
}

type returnTypeValueReader interface {
	ReturnTypeValues() []product.Value
}

type returnValueSourceReader interface {
	ReturnValueSources(cfg.Point) ([]factflow.ValueSource, bool)
}

type returnPresenceRelationReader interface {
	ReturnPresenceRelations(cfg.Point) []factflow.ReturnPresenceRelation
}

type expressionConditionReader interface {
	ExpressionCondition(factflow.ExprRef) (factflow.ExpressionCondition, bool)
}

// FromResult projects one completed check result into a fixed-point summary.
func FromResult(result ResultReader) summary.Summary {
	if result == nil {
		return summary.Summary{}
	}
	reg := result.Registry()
	graph := result.Graph()
	exit, ok := result.ExitState()
	if reg == nil || graph == nil || !ok {
		return summary.Summary{}
	}

	out := summary.Summary{
		NormalReturnParams:              projectNormalReturnParams(reg, result, exit),
		NormalReturnParamConditions:     projectNormalReturnParamConditions(reg, result),
		NormalReturnParamEqualities:     projectNormalReturnParamEqualities(reg, result),
		NormalReturnFacts:               projectNormalReturnFacts(reg, result, exit),
		ReturnConditionParamRefinements: projectReturnConditionParamRefinements(reg, result),
		ReturnPresenceRelations:         projectReturnPresenceRelations(result),
	}

	var declared []product.Value
	if reader, ok := result.(returnTypeValueReader); ok {
		declared = reader.ReturnTypeValues()
	}
	arity := len(declared)
	for _, point := range result.ReturnPoints() {
		pointArity, ok := result.ReturnArity(point)
		if ok && pointArity > arity {
			arity = pointArity
		}
	}
	if arity > 0 {
		out.Returns = projectReturnSlots(reg, result, exit, arity, declared)
	}
	return summary.Normalize(reg, out)
}
