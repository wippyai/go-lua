// Package testlaw supplies test-only execution of already-declared canonical
// Rules through the production SourceAssembly facade.
package testlaw

import (
	"context"

	"github.com/wippyai/go-lua/analysis/engine"
)

type RuleFixture[V, O, R any] struct {
	Composition        *engine.Composition
	Instance           *engine.RuleInstance[V, O]
	Query              *engine.Query[R]
	BindQuery          func(*engine.QueryBinding[R]) bool
	SiteSemantic       engine.SemanticKey
	OccurrenceSemantic engine.SemanticKey
}

type RuleResult[R any] struct {
	Status         engine.SolveStatus
	Value          R
	ValueAvailable bool
}

func solve[R any](ctx context.Context, solver *engine.Solver, query *engine.QueryInstance[R]) RuleResult[R] {
	var result RuleResult[R]
	if ctx == nil || solver == nil || query == nil {
		return result
	}
	state, status := solver.Solve(ctx)
	result.Status = status
	if status != engine.SolveComplete || state == nil {
		return result
	}
	receipt, receiptOK := query.Receipt()
	if !receiptOK {
		return result
	}
	result.Value, result.ValueAvailable = engine.QueryResult(receipt, state)
	return result
}

func Run[V, O, R any](ctx context.Context, fixture RuleFixture[V, O, R]) RuleResult[R] {
	var result RuleResult[R]
	if ctx == nil || fixture.Composition == nil || fixture.Instance == nil || fixture.Query == nil || fixture.BindQuery == nil || !fixture.SiteSemantic.Available() || !fixture.OccurrenceSemantic.Available() {
		return result
	}
	source := engine.NewSourceAssembly(fixture.Composition)
	if source == nil {
		return result
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	if !scopeOK || !truthOK {
		return result
	}
	site, siteOK := source.Site(fixture.SiteSemantic, scope, truth, true)
	occurrence, occurrenceOK := source.Relation(site, fixture.OccurrenceSemantic)
	prepared, preparedOK := source.PrepareInstance(occurrence, fixture.Instance)
	if !siteOK || !occurrenceOK || !preparedOK || !source.Seal() {
		return result
	}
	var queryInstance *engine.QueryInstance[R]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		point, pointOK := assembly.Point(site)
		member, memberOK := assembly.Member(point, prepared)
		_, groupOK := assembly.Group(point, member)
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.Query, fixture.BindQuery)
		if queryOK {
			_, observationOK = assembly.Query(point, queryInstance)
		}
		return pointOK && memberOK && groupOK && queryOK && observationOK
	})
	if !assembled {
		return result
	}
	return solve(ctx, solver, queryInstance)
}

type SelfFixture[V, O, R any] struct {
	Composition *engine.Composition
	Instance    *engine.RuleInstance[V, O]
	Query       *engine.Query[R]
	BindQuery   func(*engine.QueryBinding[R]) bool

	SiteSemantic       engine.SemanticKey
	OccurrenceSemantic engine.SemanticKey
	BoundarySemantic   engine.SemanticKey
}

func RunSelf[V, O, R any](ctx context.Context, fixture SelfFixture[V, O, R]) RuleResult[R] {
	var result RuleResult[R]
	if ctx == nil || fixture.Composition == nil || fixture.Instance == nil || fixture.Query == nil || fixture.BindQuery == nil || !fixture.SiteSemantic.Available() || !fixture.OccurrenceSemantic.Available() || !fixture.BoundarySemantic.Available() {
		return result
	}
	source := engine.NewSourceAssembly(fixture.Composition)
	if source == nil {
		return result
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	if !scopeOK || !truthOK {
		return result
	}
	site, siteOK := source.Site(fixture.SiteSemantic, scope, truth, true)
	occurrence, occurrenceOK := source.Relation(site, fixture.OccurrenceSemantic)
	prepared, preparedOK := source.PrepareInstance(occurrence, fixture.Instance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(site, site, fixture.BoundarySemantic, truth, reindex, truth)
	if !siteOK || !occurrenceOK || !preparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		return result
	}
	var queryInstance *engine.QueryInstance[R]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		point, pointOK := assembly.Point(site)
		member, memberOK := assembly.Member(point, prepared)
		group, groupOK := assembly.Group(point, member)
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.Query, fixture.BindQuery)
		if queryOK {
			_, observationOK = assembly.Query(point, queryInstance)
		}
		return pointOK && memberOK && groupOK && queryOK && observationOK && assembly.Boundary(group, boundary)
	})
	if !assembled {
		return result
	}
	return solve(ctx, solver, queryInstance)
}

type SelfLinearFixture[V, SO, TO, R any] struct {
	Composition *engine.Composition
	Source      *engine.RuleInstance[V, SO]
	Steps       []*engine.RuleInstance[V, TO]
	Query       *engine.Query[R]
	BindQuery   func(*engine.QueryBinding[R]) bool

	SourceSite       engine.SemanticKey
	SourceOccurrence engine.SemanticKey
	SourceBoundary   engine.SemanticKey
	StepSites        []engine.SemanticKey
	StepOccurrences  []engine.SemanticKey
	BoundaryKeys     []engine.SemanticKey
}

func RunSelfLinear[V, SO, TO, R any](ctx context.Context, fixture SelfLinearFixture[V, SO, TO, R]) RuleResult[R] {
	var result RuleResult[R]
	if ctx == nil || fixture.Composition == nil || fixture.Source == nil || len(fixture.Steps) == 0 || fixture.Query == nil || fixture.BindQuery == nil ||
		!fixture.SourceSite.Available() || !fixture.SourceOccurrence.Available() || !fixture.SourceBoundary.Available() ||
		len(fixture.StepSites) != len(fixture.Steps) || len(fixture.StepOccurrences) != len(fixture.Steps) || len(fixture.BoundaryKeys) != len(fixture.Steps) {
		return result
	}
	for index, step := range fixture.Steps {
		if step == nil || !fixture.StepSites[index].Available() || !fixture.StepOccurrences[index].Available() || !fixture.BoundaryKeys[index].Available() {
			return result
		}
	}
	source := engine.NewSourceAssembly(fixture.Composition)
	if source == nil {
		return result
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	if !scopeOK || !truthOK || !falseOK {
		return result
	}
	sourceSite, sourceSiteOK := source.Site(fixture.SourceSite, scope, truth, true)
	sourceOccurrence, sourceOccurrenceOK := source.Relation(sourceSite, fixture.SourceOccurrence)
	sourcePrepared, sourcePreparedOK := source.PrepareInstance(sourceOccurrence, fixture.Source)
	stepSites := make([]engine.SourceSite, len(fixture.Steps))
	stepOccurrences := make([]engine.SourceOccurrence, len(fixture.Steps))
	stepPrepared := make([]engine.SourceInstance, len(fixture.Steps))
	if !sourceSiteOK || !sourceOccurrenceOK || !sourcePreparedOK {
		return result
	}
	for index, step := range fixture.Steps {
		var stepSiteOK, stepOccurrenceOK, stepPreparedOK bool
		stepSites[index], stepSiteOK = source.Site(fixture.StepSites[index], scope, falsity, false)
		stepOccurrences[index], stepOccurrenceOK = source.Relation(stepSites[index], fixture.StepOccurrences[index])
		stepPrepared[index], stepPreparedOK = source.PrepareInstance(stepOccurrences[index], step)
		if !stepSiteOK || !stepOccurrenceOK || !stepPreparedOK {
			return result
		}
	}
	reindex, reindexOK := source.IdentityReindex(scope)
	if !reindexOK {
		return result
	}
	selfBoundary, selfBoundaryOK := source.Boundary(sourceSite, sourceSite, fixture.SourceBoundary, truth, reindex, truth)
	if !selfBoundaryOK {
		return result
	}
	boundaries := make([]engine.SourceBoundary, len(fixture.Steps))
	previous := sourceSite
	for index, target := range stepSites {
		boundary, boundaryOK := source.Boundary(previous, target, fixture.BoundaryKeys[index], truth, reindex, truth)
		if !boundaryOK {
			return result
		}
		boundaries[index], previous = boundary, target
	}
	if !source.Seal() {
		return result
	}

	var queryInstance *engine.QueryInstance[R]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		sourceMember, sourceMemberOK := assembly.Member(sourcePoint, sourcePrepared)
		sourceGroup, sourceGroupOK := assembly.Group(sourcePoint, sourceMember)
		if !sourcePointOK || !sourceMemberOK || !sourceGroupOK || !assembly.Boundary(sourceGroup, selfBoundary) {
			return false
		}
		for index := range fixture.Steps {
			point, pointOK := assembly.Point(stepSites[index])
			member, memberOK := assembly.Member(point, stepPrepared[index])
			group, groupOK := assembly.Group(point, member)
			if !pointOK || !memberOK || !groupOK || !assembly.Boundary(group, boundaries[index]) {
				return false
			}
			if index+1 == len(fixture.Steps) {
				var queryOK, observationOK bool
				queryInstance, queryOK = engine.NewQueryInstance(fixture.Query, fixture.BindQuery)
				if queryOK {
					_, observationOK = assembly.Query(point, queryInstance)
				}
				if !queryOK || !observationOK {
					return false
				}
			}
		}
		return true
	})
	if !assembled {
		return result
	}
	return solve(ctx, solver, queryInstance)
}

type OneFixture[PV, PO, TV, TO, R any] struct {
	Composition *engine.Composition
	Predecessor *engine.RuleInstance[PV, PO]
	Target      *engine.RuleInstance[TV, TO]
	Query       *engine.Query[R]
	BindQuery   func(*engine.QueryBinding[R]) bool

	PredecessorSite       engine.SemanticKey
	PredecessorOccurrence engine.SemanticKey
	TargetSite            engine.SemanticKey
	TargetOccurrence      engine.SemanticKey
	BoundarySemantic      engine.SemanticKey
}

func RunOne[PV, PO, TV, TO, R any](ctx context.Context, fixture OneFixture[PV, PO, TV, TO, R]) RuleResult[R] {
	var result RuleResult[R]
	if ctx == nil || fixture.Composition == nil || fixture.Predecessor == nil || fixture.Target == nil || fixture.Query == nil || fixture.BindQuery == nil ||
		!fixture.PredecessorSite.Available() || !fixture.PredecessorOccurrence.Available() || !fixture.TargetSite.Available() || !fixture.TargetOccurrence.Available() || !fixture.BoundarySemantic.Available() {
		return result
	}
	source := engine.NewSourceAssembly(fixture.Composition)
	if source == nil {
		return result
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	if !scopeOK || !truthOK || !falseOK {
		return result
	}
	predecessorSite, predecessorSiteOK := source.Site(fixture.PredecessorSite, scope, truth, true)
	targetSite, targetSiteOK := source.Site(fixture.TargetSite, scope, falsity, false)
	predecessorOccurrence, predecessorOccurrenceOK := source.Relation(predecessorSite, fixture.PredecessorOccurrence)
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, fixture.TargetOccurrence)
	predecessorPrepared, predecessorPreparedOK := source.PrepareInstance(predecessorOccurrence, fixture.Predecessor)
	targetPrepared, targetPreparedOK := source.PrepareInstance(targetOccurrence, fixture.Target)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(predecessorSite, targetSite, fixture.BoundarySemantic, truth, reindex, truth)
	if !predecessorSiteOK || !targetSiteOK || !predecessorOccurrenceOK || !targetOccurrenceOK || !predecessorPreparedOK || !targetPreparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		return result
	}
	var queryInstance *engine.QueryInstance[R]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		predecessorPoint, predecessorPointOK := assembly.Point(predecessorSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		predecessorMember, predecessorMemberOK := assembly.Member(predecessorPoint, predecessorPrepared)
		targetMember, targetMemberOK := assembly.Member(targetPoint, targetPrepared)
		_, predecessorGroupOK := assembly.Group(predecessorPoint, predecessorMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, targetMember)
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.Query, fixture.BindQuery)
		if queryOK {
			_, observationOK = assembly.Query(targetPoint, queryInstance)
		}
		return predecessorPointOK && targetPointOK && predecessorMemberOK && targetMemberOK && predecessorGroupOK && targetGroupOK && queryOK && observationOK && assembly.Boundary(targetGroup, boundary)
	})
	if !assembled {
		return result
	}
	return solve(ctx, solver, queryInstance)
}

type NInputFixture[PV, PO, TV, TO, R any] struct {
	Composition *engine.Composition
	Predecessor *engine.RuleInstance[PV, PO]
	Target      *engine.RuleInstance[TV, TO]
	Query       *engine.Query[R]
	BindQuery   func(*engine.QueryBinding[R]) bool

	PredecessorSite       engine.SemanticKey
	PredecessorOccurrence engine.SemanticKey
	TargetSite            engine.SemanticKey
	TargetOccurrence      engine.SemanticKey
	BoundarySemantics     []engine.SemanticKey
}

func RunNInputs[PV, PO, TV, TO, R any](ctx context.Context, fixture NInputFixture[PV, PO, TV, TO, R]) RuleResult[R] {
	var result RuleResult[R]
	if ctx == nil || fixture.Composition == nil || fixture.Predecessor == nil || fixture.Target == nil || fixture.Query == nil || fixture.BindQuery == nil ||
		!fixture.PredecessorSite.Available() || !fixture.PredecessorOccurrence.Available() || !fixture.TargetSite.Available() || !fixture.TargetOccurrence.Available() || len(fixture.BoundarySemantics) == 0 {
		return result
	}
	for _, semantic := range fixture.BoundarySemantics {
		if !semantic.Available() {
			return result
		}
	}
	source := engine.NewSourceAssembly(fixture.Composition)
	if source == nil {
		return result
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	if !scopeOK || !truthOK || !falseOK {
		return result
	}
	predecessorSite, predecessorSiteOK := source.Site(fixture.PredecessorSite, scope, truth, true)
	targetSite, targetSiteOK := source.Site(fixture.TargetSite, scope, falsity, false)
	predecessorOccurrence, predecessorOccurrenceOK := source.Relation(predecessorSite, fixture.PredecessorOccurrence)
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, fixture.TargetOccurrence)
	predecessorPrepared, predecessorPreparedOK := source.PrepareInstance(predecessorOccurrence, fixture.Predecessor)
	targetPrepared, targetPreparedOK := source.PrepareInstance(targetOccurrence, fixture.Target)
	reindex, reindexOK := source.IdentityReindex(scope)
	if !predecessorSiteOK || !targetSiteOK || !predecessorOccurrenceOK || !targetOccurrenceOK || !predecessorPreparedOK || !targetPreparedOK || !reindexOK {
		return result
	}
	boundaries := make([]engine.SourceBoundary, len(fixture.BoundarySemantics))
	for index, semantic := range fixture.BoundarySemantics {
		boundary, boundaryOK := source.Boundary(predecessorSite, targetSite, semantic, truth, reindex, truth)
		if !boundaryOK {
			return result
		}
		boundaries[index] = boundary
	}
	if !source.Seal() {
		return result
	}
	var queryInstance *engine.QueryInstance[R]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		predecessorPoint, predecessorPointOK := assembly.Point(predecessorSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		predecessorMember, predecessorMemberOK := assembly.Member(predecessorPoint, predecessorPrepared)
		targetMember, targetMemberOK := assembly.Member(targetPoint, targetPrepared)
		_, predecessorGroupOK := assembly.Group(predecessorPoint, predecessorMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, targetMember)
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.Query, fixture.BindQuery)
		if queryOK {
			_, observationOK = assembly.Query(targetPoint, queryInstance)
		}
		if !predecessorPointOK || !targetPointOK || !predecessorMemberOK || !targetMemberOK || !predecessorGroupOK || !targetGroupOK || !queryOK || !observationOK {
			return false
		}
		for _, boundary := range boundaries {
			if !assembly.Boundary(targetGroup, boundary) {
				return false
			}
		}
		return true
	})
	if !assembled {
		return result
	}
	return solve(ctx, solver, queryInstance)
}

type TwoFixture[AV, AO, BV, BO, TV, TO, R any] struct {
	Composition *engine.Composition
	First       *engine.RuleInstance[AV, AO]
	Second      *engine.RuleInstance[BV, BO]
	Target      *engine.RuleInstance[TV, TO]
	Query       *engine.Query[R]
	BindQuery   func(*engine.QueryBinding[R]) bool

	SourceSite       engine.SemanticKey
	FirstOccurrence  engine.SemanticKey
	SecondOccurrence engine.SemanticKey
	TargetSite       engine.SemanticKey
	TargetOccurrence engine.SemanticKey
	BoundarySemantic engine.SemanticKey
}

func RunTwo[AV, AO, BV, BO, TV, TO, R any](ctx context.Context, fixture TwoFixture[AV, AO, BV, BO, TV, TO, R]) RuleResult[R] {
	var result RuleResult[R]
	if ctx == nil || fixture.Composition == nil || fixture.First == nil || fixture.Second == nil || fixture.Target == nil || fixture.Query == nil || fixture.BindQuery == nil ||
		!fixture.SourceSite.Available() || !fixture.FirstOccurrence.Available() || !fixture.SecondOccurrence.Available() || !fixture.TargetSite.Available() || !fixture.TargetOccurrence.Available() || !fixture.BoundarySemantic.Available() {
		return result
	}
	source := engine.NewSourceAssembly(fixture.Composition)
	if source == nil {
		return result
	}
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	if !scopeOK || !truthOK || !falseOK {
		return result
	}
	sourceSite, sourceSiteOK := source.Site(fixture.SourceSite, scope, truth, true)
	targetSite, targetSiteOK := source.Site(fixture.TargetSite, scope, falsity, false)
	firstOccurrence, firstOccurrenceOK := source.Relation(sourceSite, fixture.FirstOccurrence)
	secondOccurrence, secondOccurrenceOK := source.Relation(sourceSite, fixture.SecondOccurrence)
	targetOccurrence, targetOccurrenceOK := source.Relation(targetSite, fixture.TargetOccurrence)
	firstPrepared, firstPreparedOK := source.PrepareInstance(firstOccurrence, fixture.First)
	secondPrepared, secondPreparedOK := source.PrepareInstance(secondOccurrence, fixture.Second)
	targetPrepared, targetPreparedOK := source.PrepareInstance(targetOccurrence, fixture.Target)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(sourceSite, targetSite, fixture.BoundarySemantic, truth, reindex, truth)
	if !sourceSiteOK || !targetSiteOK || !firstOccurrenceOK || !secondOccurrenceOK || !targetOccurrenceOK || !firstPreparedOK || !secondPreparedOK || !targetPreparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		return result
	}
	var queryInstance *engine.QueryInstance[R]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		firstMember, firstMemberOK := assembly.Member(sourcePoint, firstPrepared)
		secondMember, secondMemberOK := assembly.Member(sourcePoint, secondPrepared)
		targetMember, targetMemberOK := assembly.Member(targetPoint, targetPrepared)
		_, sourceGroupOK := assembly.Group(sourcePoint, firstMember, secondMember)
		targetGroup, targetGroupOK := assembly.Group(targetPoint, targetMember)
		var queryOK, observationOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.Query, fixture.BindQuery)
		if queryOK {
			_, observationOK = assembly.Query(targetPoint, queryInstance)
		}
		return sourcePointOK && targetPointOK && firstMemberOK && secondMemberOK && targetMemberOK && sourceGroupOK && targetGroupOK && queryOK && observationOK && assembly.Boundary(targetGroup, boundary)
	})
	if !assembled {
		return result
	}
	return solve(ctx, solver, queryInstance)
}
