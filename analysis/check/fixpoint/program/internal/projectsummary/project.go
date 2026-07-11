package projectsummary

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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

type rootAssignmentReader interface {
	RootAssignment(cfg.Point) (factflow.RootAssignment, bool)
}

type pathAssignmentReader interface {
	PathAssignment(cfg.Point) (factflow.PathAssignment, bool)
}

type pathDescendantInvalidationReader interface {
	PathDescendantInvalidation(cfg.Point) (factflow.PathDescendantInvalidation, bool)
}

type stateAtReader interface {
	StateAt(cfg.Point) (state.State, bool)
}

type branchPathEvidenceReader interface {
	BranchPathEvidence(cfg.Point) []factflow.BranchPathEvidence
}

type branchPathRelationReader interface {
	BranchPathRelations(cfg.Point) []factflow.BranchPathRelation
}

type branchSufficientLiteralCaseReader interface {
	BranchSufficientLiteralCases(cfg.Point) []factflow.BranchSufficientLiteralCase
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

type returnPresenceRelationReader interface {
	ReturnPresenceRelations(cfg.Point) []factflow.ReturnPresenceRelation
}

type expressionConditionReader interface {
	ExpressionCondition(factflow.ExprRef) (factflow.ExpressionCondition, bool)
}

type dynamicIndexWriteReader interface {
	DynamicIndexWrite(cfg.Point) (factflow.DynamicIndexWrite, bool)
}

type stableShapeSourceReader interface {
	SourceHasStableShapeBeforeBoundary(cfg.Point, factflow.ValueSource) bool
}

// FromResult projects one completed check result into a fixed-point summary.
func FromResult(result ResultReader) summary.Summary {
	projected, _ := FromResultContext(context.Background(), result)
	return projected
}

// FromResultContext projects a completed check result into a summary while
// observing cancellation during graph-sized projection passes. A canceled
// projection never publishes its partially accumulated summary.
func FromResultContext(ctx context.Context, result ResultReader) (summary.Summary, error) {
	if err := projectionContextErr(ctx); err != nil {
		return summary.Summary{}, err
	}
	if result == nil {
		return summary.Summary{}, nil
	}
	reg := result.Registry()
	graph := result.Graph()
	exit, ok := result.ExitState()
	if reg == nil || graph == nil {
		return summary.Summary{}, nil
	}
	if !ok {
		// A body that never returns normally (e.g. a stub `function f(): T
		// error("nyi") end`) has an unreachable exit and so no normal-return facts,
		// but its declared signature is still the contract callers see. Emit the
		// exit-independent projections and the declared return slots.
		return noNormalExitSummaryContext(ctx, reg, result)
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

	paramObligations, err := projectParamObligationsContext(ctx, reg, result, paramCache)
	if err != nil {
		return summary.Summary{}, err
	}
	out := summary.Summary{
		ParamObligations:                paramObligations,
		ParamMemberCallObligations:      projectParamMemberCallObligations(reg, result, paramCache),
		ParamMemberReturnSlots:          projectParamMemberReturnSlots(reg, result, paramCache),
		ReturnParamPathAliases:          projectReturnParamPathAliases(result),
		ReturnFlows:                     projectReturnFlows(result),
		ParamSinkExposures:              projectParamSinkExposures(reg, result, exit),
		CapturedPathObligations:         projectCapturedPathObligations(reg, result, paramCache),
		NormalReturnParams:              projectNormalReturnParams(reg, result, exit),
		NormalReturnParamConditions:     projectNormalReturnParamConditions(reg, result),
		NormalReturnParamEqualities:     projectNormalReturnParamEqualities(reg, result),
		NormalReturnFacts:               projectNormalReturnFacts(reg, result, exit),
		ProtectedCallTypestate:          projectProtectedCallTypestate(result, exit, true),
		HeapTableObjects:                heapTableObjects,
		HeapKeySpace:                    heapKeySpace,
		ReturnConditionParamRefinements: projectReturnConditionParamRefinements(result),
		ReturnConditionSlotRefinements:  projectReturnConditionSlotRefinements(reg, result),
		ReturnParamLiteralCases:         projectReturnParamLiteralCases(reg, result),
		ReturnPresenceRelations:         projectReturnPresenceRelations(reg, result),
		MaySuspend:                      projectMaySuspend(result),
	}

	var declared []product.Value
	if reader, ok := result.(returnTypeValueReader); ok {
		declared = reader.ReturnTypeValues()
	}
	arity := len(declared)
	for _, point := range result.ReturnPoints() {
		if err := projectionContextErr(ctx); err != nil {
			return summary.Summary{}, err
		}
		pointArity, ok := resultReturnSourceArity(result, point)
		if ok && pointArity > arity {
			arity = pointArity
		}
	}
	if arity > 0 {
		out.Returns = projectReturnSlots(reg, result, exit, arity, declared)
	}
	out.HeapTableObjects = markStableReturnHeapObjects(reg, result, out.HeapTableObjects, out.Returns)
	if err := projectionContextErr(ctx); err != nil {
		return summary.Summary{}, err
	}
	return summary.NormalizeOwned(reg, out), nil
}

// noNormalExitSummary builds the summary for a function whose body never returns
// normally (an unreachable exit). The exit-dependent facts (normal-return facts,
// param-sink exposures, heap objects at return) are correctly empty, but the
// exit-independent param obligations and the declared return signature still hold
// and are the contract callers rely on.
func noNormalExitSummary(reg *axis.Registry, result ResultReader) summary.Summary {
	projected, _ := noNormalExitSummaryContext(context.Background(), reg, result)
	return projected
}

func noNormalExitSummaryContext(ctx context.Context, reg *axis.Registry, result ResultReader) (summary.Summary, error) {
	paramCache := newParamObligationProjectorCache(result.Graph())
	paramObligations, err := projectParamObligationsContext(ctx, reg, result, paramCache)
	if err != nil {
		return summary.Summary{}, err
	}
	out := summary.Summary{
		ParamObligations:                paramObligations,
		ParamMemberCallObligations:      projectParamMemberCallObligations(reg, result, paramCache),
		ParamMemberReturnSlots:          projectParamMemberReturnSlots(reg, result, paramCache),
		CapturedPathObligations:         projectCapturedPathObligations(reg, result, paramCache),
		ReturnParamPathAliases:          projectReturnParamPathAliases(result),
		ReturnFlows:                     projectReturnFlows(result),
		NormalReturnParamConditions:     projectNormalReturnParamConditions(reg, result),
		NormalReturnParamEqualities:     projectNormalReturnParamEqualities(reg, result),
		ProtectedCallTypestate:          projectProtectedCallTypestate(result, state.State{}, false),
		ReturnConditionParamRefinements: projectReturnConditionParamRefinements(result),
		MaySuspend:                      projectMaySuspend(result),
	}
	if reader, ok := result.(returnTypeValueReader); ok {
		if declared := reader.ReturnTypeValues(); len(declared) != 0 {
			out.Returns = append([]product.Value(nil), declared...)
		}
	}
	if err := projectionContextErr(ctx); err != nil {
		return summary.Summary{}, err
	}
	return summary.NormalizeOwned(reg, out), nil
}

func projectionContextErr(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return errors.Join(solve.ErrCanceled, ctx.Err())
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
