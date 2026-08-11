package testlaw

import (
	"context"

	"github.com/wippyai/go-lua/analysis/engine"
)

// TwoStageTopology names the three sites and ordinary identity boundaries of
// one source -> middle -> target execution.
type TwoStageTopology struct {
	SourceSite             engine.SemanticKey
	FirstSourceOccurrence  engine.SemanticKey
	SecondSourceOccurrence engine.SemanticKey
	ThirdSourceOccurrence  engine.SemanticKey

	MiddleSite             engine.SemanticKey
	FirstMiddleOccurrence  engine.SemanticKey
	SecondMiddleOccurrence engine.SemanticKey
	FirstMiddleBoundary    engine.SemanticKey
	SecondMiddleBoundary   engine.SemanticKey

	TargetSite       engine.SemanticKey
	TargetOccurrence engine.SemanticKey
	TargetBoundaries []engine.SemanticKey
}

// RunTwoStage executes three zero-input sources, two independent one-input
// middle Rules, and one target Rule through one SourceAssembly.
func RunTwoStage[
	FirstSourceValue, FirstSourceOperand,
	SecondSourceValue, SecondSourceOperand,
	ThirdSourceValue, ThirdSourceOperand,
	FirstMiddleValue, FirstMiddleOperand,
	SecondMiddleValue, SecondMiddleOperand,
	TargetValue, TargetOperand,
	Result any,
](
	ctx context.Context,
	composition *engine.Composition,
	firstSource *engine.RuleInstance[FirstSourceValue, FirstSourceOperand],
	secondSource *engine.RuleInstance[SecondSourceValue, SecondSourceOperand],
	thirdSource *engine.RuleInstance[ThirdSourceValue, ThirdSourceOperand],
	firstMiddle *engine.RuleInstance[FirstMiddleValue, FirstMiddleOperand],
	secondMiddle *engine.RuleInstance[SecondMiddleValue, SecondMiddleOperand],
	target *engine.RuleInstance[TargetValue, TargetOperand],
	query *engine.Query[Result],
	bindQuery func(*engine.QueryBinding[Result]) bool,
	topology TwoStageTopology,
) RuleResult[Result] {
	var result RuleResult[Result]
	if ctx == nil || composition == nil || firstSource == nil || secondSource == nil || thirdSource == nil || firstMiddle == nil || secondMiddle == nil || target == nil || query == nil || bindQuery == nil || !validTwoStageTopology(topology) {
		return result
	}
	source := engine.NewSourceAssembly(composition)
	if source == nil {
		return result
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	if !scopeOK || !truthOK || !falseOK {
		return result
	}
	sourceSite, sourceSiteOK := source.Site(topology.SourceSite, scope, truth, true)
	middleSite, middleSiteOK := source.Site(topology.MiddleSite, scope, falsity, false)
	targetSite, targetSiteOK := source.Site(topology.TargetSite, scope, falsity, false)
	firstSourceOccurrence, firstSourceOccurrenceOK := source.Relation(sourceSite, topology.FirstSourceOccurrence)
	secondSourceOccurrence, secondSourceOccurrenceOK := source.Relation(sourceSite, topology.SecondSourceOccurrence)
	thirdSourceOccurrence, thirdSourceOccurrenceOK := source.Relation(sourceSite, topology.ThirdSourceOccurrence)
	firstMiddleOccurrence, firstMiddleOccurrenceOK := source.Relation(middleSite, topology.FirstMiddleOccurrence)
	secondMiddleOccurrence, secondMiddleOccurrenceOK := source.Relation(middleSite, topology.SecondMiddleOccurrence)
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, topology.TargetOccurrence)
	firstSourcePrepared, firstSourcePreparedOK := source.PrepareInstance(firstSourceOccurrence, firstSource)
	secondSourcePrepared, secondSourcePreparedOK := source.PrepareInstance(secondSourceOccurrence, secondSource)
	thirdSourcePrepared, thirdSourcePreparedOK := source.PrepareInstance(thirdSourceOccurrence, thirdSource)
	firstMiddlePrepared, firstMiddlePreparedOK := source.PrepareInstance(firstMiddleOccurrence, firstMiddle)
	secondMiddlePrepared, secondMiddlePreparedOK := source.PrepareInstance(secondMiddleOccurrence, secondMiddle)
	targetPrepared, targetPreparedOK := source.PrepareInstance(targetOccurrence, target)
	if !sourceSiteOK || !middleSiteOK || !targetSiteOK || !firstSourceOccurrenceOK || !secondSourceOccurrenceOK || !thirdSourceOccurrenceOK || !firstMiddleOccurrenceOK || !secondMiddleOccurrenceOK || !targetOccurrenceOK ||
		!firstSourcePreparedOK || !secondSourcePreparedOK || !thirdSourcePreparedOK || !firstMiddlePreparedOK || !secondMiddlePreparedOK || !targetPreparedOK {
		return result
	}
	reindex, reindexOK := source.IdentityReindex(scope)
	if !reindexOK {
		return result
	}
	firstMiddleBoundary, firstMiddleBoundaryOK := source.Boundary(sourceSite, middleSite, topology.FirstMiddleBoundary, truth, reindex, truth)
	secondMiddleBoundary, secondMiddleBoundaryOK := source.Boundary(sourceSite, middleSite, topology.SecondMiddleBoundary, truth, reindex, truth)
	targetBoundaries := make([]engine.SourceBoundary, len(topology.TargetBoundaries))
	for index, semantic := range topology.TargetBoundaries {
		boundary, boundaryOK := source.Boundary(middleSite, targetSite, semantic, truth, reindex, truth)
		if !boundaryOK {
			return result
		}
		targetBoundaries[index] = boundary
	}
	if !firstMiddleBoundaryOK || !secondMiddleBoundaryOK || !source.Seal() {
		return result
	}

	var queryInstance *engine.QueryInstance[Result]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		middlePoint, middlePointOK := assembly.Point(middleSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		firstSourceMember, firstSourceMemberOK := assembly.Member(sourcePoint, firstSourcePrepared)
		secondSourceMember, secondSourceMemberOK := assembly.Member(sourcePoint, secondSourcePrepared)
		thirdSourceMember, thirdSourceMemberOK := assembly.Member(sourcePoint, thirdSourcePrepared)
		firstMiddleMember, firstMiddleMemberOK := assembly.Member(middlePoint, firstMiddlePrepared)
		secondMiddleMember, secondMiddleMemberOK := assembly.Member(middlePoint, secondMiddlePrepared)
		targetMember, targetMemberOK := assembly.Member(targetPoint, targetPrepared)
		_, firstSourceGroupOK := assembly.Group(sourcePoint, firstSourceMember)
		_, secondSourceGroupOK := assembly.Group(sourcePoint, secondSourceMember)
		_, thirdSourceGroupOK := assembly.Group(sourcePoint, thirdSourceMember)
		firstMiddleGroup, firstMiddleGroupOK := assembly.Group(middlePoint, firstMiddleMember)
		secondMiddleGroup, secondMiddleGroupOK := assembly.Group(middlePoint, secondMiddleMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, targetMember)
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(query, bindQuery)
		if queryOK {
			_, observationOK = assembly.Query(targetPoint, queryInstance)
		}
		if !sourcePointOK || !middlePointOK || !targetPointOK || !firstSourceMemberOK || !secondSourceMemberOK || !thirdSourceMemberOK || !firstMiddleMemberOK || !secondMiddleMemberOK || !targetMemberOK ||
			!firstSourceGroupOK || !secondSourceGroupOK || !thirdSourceGroupOK || !firstMiddleGroupOK || !secondMiddleGroupOK || !targetGroupOK || !queryOK || !observationOK ||
			!assembly.Boundary(firstMiddleGroup, firstMiddleBoundary) || !assembly.Boundary(secondMiddleGroup, secondMiddleBoundary) {
			return false
		}
		for _, boundary := range targetBoundaries {
			if !assembly.Boundary(targetGroup, boundary) {
				return false
			}
		}
		return true
	})
	if !assembled || solver == nil {
		return result
	}
	state, status := solver.Solve(ctx)
	result.Status = status
	if status != engine.SolveComplete || state == nil {
		return result
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		return result
	}
	result.Value, result.ValueAvailable = engine.QueryResult(receipt, state)
	return result
}

func validTwoStageTopology(topology TwoStageTopology) bool {
	if !topology.SourceSite.Available() || !topology.FirstSourceOccurrence.Available() || !topology.SecondSourceOccurrence.Available() || !topology.ThirdSourceOccurrence.Available() ||
		!topology.MiddleSite.Available() || !topology.FirstMiddleOccurrence.Available() || !topology.SecondMiddleOccurrence.Available() || !topology.FirstMiddleBoundary.Available() || !topology.SecondMiddleBoundary.Available() ||
		!topology.TargetSite.Available() || !topology.TargetOccurrence.Available() || len(topology.TargetBoundaries) == 0 {
		return false
	}
	for _, semantic := range topology.TargetBoundaries {
		if !semantic.Available() {
			return false
		}
	}
	return true
}
