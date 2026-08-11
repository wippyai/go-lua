package engine_test

import (
	"context"
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

// EnvironmentEdge has no Group or Rule at its target. The active target must
// still receive the source support, while an undemanded structural consumer
// must be ignored when the source publishes.
func TestEnvironmentEdgeLifecycleSkipsInactiveStructuralConsumer(t *testing.T) {
	composition := engine.NewComposition()
	completion, completionOK := engine.DeclareSupportCompletion(composition, facadeKey(191))
	prune, pruneOK := engine.DeclarePrune(completion, facadeKey(192))
	runs := 0
	rule, ruleOK := engine.DeclareSupportRule(composition, engine.SupportRuleSpec{
		Semantic:   facadeKey(193),
		Completion: completion,
		Prune:      prune,
		Inputs:     0,
		Admission:  engine.AdmitSupportByTrustedTheorem(facadeKey(194)),
		Run: func(value engine.Support) (engine.Support, bool) {
			runs++
			return value, true
		},
	})
	query, queryOK := engine.DeclareSupportQuery(composition, facadeKey(195), func(observation engine.SupportObservation) bool {
		reachable, ok := engine.SupportReachable(observation)
		return ok && reachable
	}, engine.FrozenResult[bool]{
		Semantic: facadeKey(196),
		Freeze:   func(value bool) bool { return value },
		Clone:    func(value bool) bool { return value },
		Equal:    func(left, right bool) bool { return left == right },
		Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		},
	})
	if !completionOK || !pruneOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("cold")
	}

	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(facadeKey(197), scope, truth, true)
	targetSite, targetSiteOK := source.Site(facadeKey(198), scope, falsity, false)
	dormantSite, dormantSiteOK := source.Site(facadeKey(199), scope, falsity, false)
	sourceOccurrence, occurrenceOK := source.At(sourceSite)
	instance, instanceOK := engine.NewSupportInstance(rule, func(*engine.StructuralBinding) bool { return true })
	prepared, preparedOK := source.PrepareStructural(sourceOccurrence, facadeKey(200), instance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(sourceSite, targetSite, facadeKey(201), truth, reindex, truth)
	dormantBoundary, dormantBoundaryOK := source.Boundary(sourceSite, dormantSite, facadeKey(202), truth, reindex, truth)
	if !scopeOK || !truthOK || !falseOK || !sourceSiteOK || !targetSiteOK || !dormantSiteOK || !occurrenceOK || !instanceOK || !preparedOK || !reindexOK || !boundaryOK || !dormantBoundaryOK || !source.Seal() {
		t.Fatal("source")
	}
	queryInstance, queryInstanceOK := engine.NewQueryInstance(query, func(*engine.QueryBinding[bool]) bool { return true })
	if !queryInstanceOK {
		t.Fatal("query instance")
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		dormantPoint, dormantPointOK := assembly.Point(dormantSite)
		member, memberOK := assembly.Member(sourcePoint, prepared)
		_, groupOK := assembly.Group(sourcePoint, member)
		activeEdgeOK := assembly.EnvironmentEdge(targetPoint, boundary)
		inactiveEdgeOK := assembly.EnvironmentEdge(dormantPoint, dormantBoundary)
		_, observationOK := assembly.Query(targetPoint, queryInstance)
		return sourcePointOK && targetPointOK && dormantPointOK && memberOK && groupOK && activeEdgeOK && inactiveEdgeOK && observationOK
	})
	if !assembled || solver == nil {
		t.Fatal("assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	reachable, readable := engine.QueryResult(receipt, state)
	if status != engine.SolveComplete || !receiptOK || !readable || !reachable || runs != 1 {
		t.Fatalf("environment edge state=%v status=%v receipt=%t readable=%t reachable=%t runs=%d", state, status, receiptOK, readable, reachable, runs)
	}
}
