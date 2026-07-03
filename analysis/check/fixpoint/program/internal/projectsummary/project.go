package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type ResultReader interface {
	Registry() *axis.Registry
	Graph() cfg.Graph
	ExitState() (state.State, bool)
	ReturnPoints() []cfg.Point
	KeySpace() *keyspace.KeySpace
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

type branchSufficientCheckReader interface {
	BranchConditionSufficientChecksOnEdge(semantics.BranchConditionFact, bool) []branchcond.ImpliedCheck
}

type pointDominatorReader interface {
	PointDominates(dominator, point cfg.Point) bool
}

type noNormalReturnReader interface {
	NoNormalReturn(cfg.Point) bool
}

type normalEdgeReachabilityReader interface {
	EdgeCanCompleteNormally(from, to cfg.Point) bool
}

type returnTypeValueReader interface {
	ReturnTypeValues() []product.Value
}

type returnValueSourceReader interface {
	ReturnValueSources(cfg.Point) ([]factflow.ValueSource, bool)
}

type expressionValueReader interface {
	ExpressionValueAtBoundary(cfg.Point, ast.Expr) (product.Value, bool)
}

type expressionValueBeforeReader interface {
	ExpressionValueBeforeBoundary(cfg.Point, ast.Expr) (product.Value, bool)
}

type returnPresenceRelationReader interface {
	ReturnPresenceRelations(cfg.Point) []factflow.ReturnPresenceRelation
}

type expressionConditionReader interface {
	ExpressionCondition(factflow.ExprRef) (factflow.ExpressionCondition, bool)
}

type dynamicIndexWriteReader interface {
	DynamicIndexWrite(cfg.Point) (factflow.DynamicIndexWrite, bool)
}

// FromResult projects one completed check result into a fixed-point summary.
func FromResult(result ResultReader) summary.Summary {
	if result == nil {
		return summary.Summary{}
	}
	reg := result.Registry()
	graph := result.Graph()
	exit, ok := result.ExitState()
	if reg == nil || graph == nil {
		return summary.Summary{}
	}
	if !ok {
		// A body that never returns normally (e.g. a stub `function f(): T
		// error("nyi") end`) has an unreachable exit and so no normal-return facts,
		// but its declared signature is still the contract callers see. Emit the
		// exit-independent projections and the declared return slots.
		return noNormalExitSummary(reg, result)
	}
	heapTables := exit.HeapTableObjectsSnapshot()
	heapTableObjects := heapTables.Objects
	if heapTables.Top {
		heapTableObjects = nil
	}
	var heapKeySpace *keyspace.KeySpace
	if len(heapTableObjects) != 0 {
		heapKeySpace = result.KeySpace()
	}
	paramCache := newParamObligationProjectorCache(graph)

	out := summary.Summary{
		ParamObligations:                projectParamObligations(reg, result, paramCache),
		ParamMemberCallObligations:      projectParamMemberCallObligations(reg, result, paramCache),
		ParamMemberReturnSlots:          projectParamMemberReturnSlots(reg, result, paramCache),
		ReturnParamPathAliases:          projectReturnParamPathAliases(result),
		ParamSinkExposures:              projectParamSinkExposures(reg, result, exit),
		CapturedPathObligations:         projectCapturedPathObligations(reg, result, paramCache),
		NormalReturnParams:              projectNormalReturnParams(reg, result, exit),
		NormalReturnParamConditions:     projectNormalReturnParamConditions(reg, result),
		NormalReturnParamEqualities:     projectNormalReturnParamEqualities(reg, result),
		NormalReturnFacts:               projectNormalReturnFacts(reg, result, exit),
		HeapTableObjects:                heapTableObjects,
		HeapKeySpace:                    heapKeySpace,
		ReturnConditionParamRefinements: projectReturnConditionParamRefinements(result),
		ReturnConditionSlotRefinements:  projectReturnConditionSlotRefinements(reg, result),
		ReturnParamLiteralCases:         projectReturnParamLiteralCases(reg, result),
		ReturnPresenceRelations:         projectReturnPresenceRelations(reg, result),
	}

	var declared []product.Value
	if reader, ok := result.(returnTypeValueReader); ok {
		declared = reader.ReturnTypeValues()
	}
	arity := len(declared)
	for _, point := range result.ReturnPoints() {
		pointArity, ok := resultReturnSourceArity(result, point)
		if ok && pointArity > arity {
			arity = pointArity
		}
	}
	if arity > 0 {
		out.Returns = projectReturnSlots(reg, result, exit, arity, declared)
	}
	return summary.NormalizeOwned(reg, out)
}

// noNormalExitSummary builds the summary for a function whose body never returns
// normally (an unreachable exit). The exit-dependent facts (normal-return facts,
// param-sink exposures, heap objects at return) are correctly empty, but the
// exit-independent param obligations and the declared return signature still hold
// and are the contract callers rely on.
func noNormalExitSummary(reg *axis.Registry, result ResultReader) summary.Summary {
	paramCache := newParamObligationProjectorCache(result.Graph())
	out := summary.Summary{
		ParamObligations:                projectParamObligations(reg, result, paramCache),
		ParamMemberCallObligations:      projectParamMemberCallObligations(reg, result, paramCache),
		ParamMemberReturnSlots:          projectParamMemberReturnSlots(reg, result, paramCache),
		CapturedPathObligations:         projectCapturedPathObligations(reg, result, paramCache),
		ReturnParamPathAliases:          projectReturnParamPathAliases(result),
		NormalReturnParamConditions:     projectNormalReturnParamConditions(reg, result),
		NormalReturnParamEqualities:     projectNormalReturnParamEqualities(reg, result),
		ReturnConditionParamRefinements: projectReturnConditionParamRefinements(result),
	}
	if reader, ok := result.(returnTypeValueReader); ok {
		if declared := reader.ReturnTypeValues(); len(declared) != 0 {
			out.Returns = append([]product.Value(nil), declared...)
		}
	}
	return summary.NormalizeOwned(reg, out)
}

func resultReturnSourceArity(result ResultReader, point cfg.Point) (int, bool) {
	reader, ok := result.(returnValueSourceReader)
	if !ok {
		return 0, false
	}
	sources, ok := reader.ReturnValueSources(point)
	if !ok {
		return 0, false
	}
	return len(sources), true
}
