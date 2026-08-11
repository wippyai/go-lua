package engine_test

import (
	"context"
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

// TestExternalSourceControlBuildsFactorFreeStructuralEdge proves that the
// public source facade can express a non-identity control transport and one
// output-free structural member without exposing equation coordinates or
// manufacturing a domain Factor merely to make the edge schedulable.
func TestExternalSourceControlBuildsFactorFreeStructuralEdge(t *testing.T) {
	composition := engine.NewComposition()
	completion, completionOK := engine.DeclareSupportCompletion(composition, facadeKey(21))
	prune, pruneOK := engine.DeclarePrune(completion, facadeKey(22))
	runs := 0
	rule, ruleOK := engine.DeclareSupportRule(composition, engine.SupportRuleSpec{
		Semantic:   facadeKey(23),
		Completion: completion,
		Prune:      prune,
		Inputs:     1,
		Admission:  engine.AdmitSupportByTrustedTheorem(facadeKey(24)),
		Run: func(value engine.Support) (engine.Support, bool) {
			runs++
			return value, true
		},
	})
	query, queryOK := engine.DeclareSupportQuery(composition, facadeKey(25), func(observation engine.SupportObservation) bool {
		reachable, ok := engine.SupportReachable(observation)
		return ok && reachable
	}, engine.FrozenResult[bool]{
		Semantic: facadeKey(26),
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
		t.Fatal("factor-free control declaration")
	}

	source := engine.NewSourceAssembly(composition)
	raw, rawOK := source.Decision(facadeKey(27))
	fresh, freshOK := source.Decision(facadeKey(28))
	sourceScope, sourceScopeOK := source.Scope(raw)
	targetScope, targetScopeOK := source.Scope(fresh)
	truth, truthOK := source.TrueExpr()
	falsity, falsityOK := source.FalseExpr()
	sourceSite, sourceSiteOK := source.Site(facadeKey(29), sourceScope, truth, true)
	targetSite, targetSiteOK := source.Site(facadeKey(30), targetScope, falsity, false)
	targetOccurrence, occurrenceOK := source.At(targetSite)
	instance, instanceOK := engine.NewSupportInstance(rule, func(*engine.StructuralBinding) bool { return true })
	prepared, preparedOK := source.PrepareStructural(targetOccurrence, facadeKey(31), instance)
	if prepared.Available() {
		t.Fatal("structural capability became available before source sealing")
	}
	pre, preOK := source.DecisionExpr(raw)
	post, postOK := source.DecisionExpr(fresh)
	rename, renameOK := source.RenameMap(raw, fresh)
	reindex, reindexOK := source.Reindex(sourceScope, targetScope, rename)
	boundary, boundaryOK := source.Boundary(sourceSite, targetSite, facadeKey(32), pre, reindex, post)
	sealed := source.Seal()
	if !rawOK || !freshOK || !sourceScopeOK || !targetScopeOK || !truthOK || !falsityOK ||
		!sourceSiteOK || !targetSiteOK || !occurrenceOK || !instanceOK || !preparedOK ||
		!preOK || !postOK || !renameOK || !reindexOK || !sealed || !prepared.Available() || !boundaryOK || !boundary.Available() {
		t.Fatal("opaque source control admission")
	}

	queryInstance, queryInstanceOK := engine.NewQueryInstance(query, func(*engine.QueryBinding[bool]) bool { return true })
	if !queryInstanceOK || queryInstance == nil {
		t.Fatal("support query instance")
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		sourcePoint, sourcePointOK := assembly.Point(sourceSite)
		targetPoint, targetPointOK := assembly.Point(targetSite)
		member, memberOK := assembly.Member(targetPoint, prepared)
		group, groupOK := assembly.Group(targetPoint, member)
		_, queryAttached := assembly.Query(targetPoint, queryInstance)
		return sourcePointOK && targetPointOK && memberOK && groupOK && queryAttached && assembly.Boundary(group, boundary) &&
			sourcePoint.Available()
	})
	if !assembled || solver == nil {
		t.Fatal("factor-free control assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	if status != engine.SolveComplete || state == nil || runs != 1 || !receiptOK {
		t.Fatalf("factor-free control solve state=%v status=%v runs=%d", state, status, runs)
	}
	if reachable, ok := engine.QueryResult(receipt, state); !ok || !reachable {
		t.Fatalf("factor-free control reachable=%t ok=%t", reachable, ok)
	}
}
