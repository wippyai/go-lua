package testlaw

import (
	"context"

	"github.com/wippyai/go-lua/analysis/engine"
)

// LinearFixture binds one zero-input production source and an ordered
// sequence of production one-input Rules. All topology is issued by the
// owning SourceAssembly; callers retain ownership of the typed Rules.
type LinearFixture[SourceValue, SourceOperand, StepValue, StepOperand, Result any] struct {
	Composition *engine.Composition
	Source      *engine.RuleInstance[SourceValue, SourceOperand]
	Steps       []*engine.RuleInstance[StepValue, StepOperand]
	Query       *engine.Query[Result]
	BindQuery   func(*engine.QueryBinding[Result]) bool

	SourceSite        engine.SemanticKey
	SourceOccurrence  engine.SemanticKey
	StepSites         []engine.SemanticKey
	StepOccurrences   []engine.SemanticKey
	BoundarySemantics []engine.SemanticKey
}

// RunLinear executes the supplied source followed by every supplied
// production step over ordinary identity boundaries.
func RunLinear[SourceValue, SourceOperand, StepValue, StepOperand, Result any](ctx context.Context, fixture LinearFixture[SourceValue, SourceOperand, StepValue, StepOperand, Result]) RuleResult[Result] {
	var result RuleResult[Result]
	if ctx == nil || fixture.Composition == nil || fixture.Source == nil || len(fixture.Steps) == 0 || fixture.Query == nil || fixture.BindQuery == nil ||
		!fixture.SourceSite.Available() || !fixture.SourceOccurrence.Available() ||
		len(fixture.StepSites) != len(fixture.Steps) || len(fixture.StepOccurrences) != len(fixture.Steps) || len(fixture.BoundarySemantics) != len(fixture.Steps) {
		return result
	}
	for index, step := range fixture.Steps {
		if step == nil || !fixture.StepSites[index].Available() || !fixture.StepOccurrences[index].Available() || !fixture.BoundarySemantics[index].Available() {
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
	if !sourceSiteOK || !sourceOccurrenceOK || !sourcePreparedOK {
		return result
	}
	stepSites := make([]engine.SourceSite, len(fixture.Steps))
	stepOccurrences := make([]engine.SourceOccurrence, len(fixture.Steps))
	stepPrepared := make([]engine.SourceInstance, len(fixture.Steps))
	for index, step := range fixture.Steps {
		var stepSitesOK, stepOccurrencesOK, stepPreparedOK bool
		stepSites[index], stepSitesOK = source.Site(fixture.StepSites[index], scope, falsity, false)
		stepOccurrences[index], stepOccurrencesOK = source.Relation(stepSites[index], fixture.StepOccurrences[index])
		stepPrepared[index], stepPreparedOK = source.PrepareInstance(stepOccurrences[index], step)
		if !stepSitesOK || !stepOccurrencesOK || !stepPreparedOK {
			return result
		}
	}
	reindex, reindexOK := source.IdentityReindex(scope)
	if !reindexOK {
		return result
	}
	boundaries := make([]engine.SourceBoundary, len(fixture.Steps))
	previous := sourceSite
	for index, stepSite := range stepSites {
		boundary, boundaryOK := source.Boundary(previous, stepSite, fixture.BoundarySemantics[index], truth, reindex, truth)
		if !boundaryOK {
			return result
		}
		boundaries[index], previous = boundary, stepSite
	}
	if !source.Seal() {
		return result
	}

	var queryInstance *engine.QueryInstance[Result]
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		sourceMember, sourceMemberOK := assembly.Member(sourcePoint, sourcePrepared)
		_, sourceGroupOK := assembly.Group(sourcePoint, sourceMember)
		if !sourcePointOK || !sourceMemberOK || !sourceGroupOK {
			return false
		}
		lastPoint := sourcePoint
		for index := range fixture.Steps {
			point, pointOK := assembly.Point(stepSites[index])
			member, memberOK := assembly.Member(point, stepPrepared[index])
			group, groupOK := assembly.Group(point, member)
			if !pointOK || !memberOK || !groupOK || !assembly.Boundary(group, boundaries[index]) {
				return false
			}
			lastPoint = point
		}
		var queryOK bool
		queryInstance, queryOK = engine.NewQueryInstance(fixture.Query, fixture.BindQuery)
		_, observationOK := assembly.Query(lastPoint, queryInstance)
		return queryOK && observationOK
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
