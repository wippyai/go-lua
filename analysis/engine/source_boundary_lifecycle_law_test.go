package engine_test

import (
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

func boundaryLifecycleComposition(t *testing.T) *engine.Composition {
	t.Helper()
	composition := engine.NewComposition()
	completion, completionOK := engine.DeclareSupportCompletion(composition, facadeKey(41))
	prune, pruneOK := engine.DeclarePrune(completion, facadeKey(42))
	rule, ruleOK := engine.DeclareSupportRule(composition, engine.SupportRuleSpec{
		Semantic:   facadeKey(43),
		Completion: completion,
		Prune:      prune,
		Inputs:     1,
		Admission:  engine.AdmitSupportByTrustedTheorem(facadeKey(44)),
		Run: func(value engine.Support) (engine.Support, bool) {
			return value, true
		},
	})
	query, queryOK := engine.DeclareSupportQuery(composition, facadeKey(45), func(observation engine.SupportObservation) bool {
		reachable, ok := engine.SupportReachable(observation)
		return ok && reachable
	}, engine.FrozenResult[bool]{
		Semantic: facadeKey(46),
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
		t.Fatal("boundary lifecycle composition")
	}
	return composition
}

func TestSourceAssemblyBoundaryAdmissionPublishesOnlyAtSeal(t *testing.T) {
	composition := boundaryLifecycleComposition(t)
	source := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(facadeKey(47), scope, truth, true)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(site, site, facadeKey(48), truth, reindex, truth)
	if !scopeOK || !truthOK || !siteOK || !reindexOK || !boundaryOK {
		t.Fatal("pre-Seal boundary admission")
	}
	if boundary.Available() {
		t.Fatal("pending boundary published before Seal")
	}
	if !source.Seal() || !boundary.Available() {
		t.Fatal("boundary was not published by successful Seal")
	}
	if _, ok := source.Boundary(site, site, facadeKey(49), truth, reindex, truth); ok {
		t.Fatal("post-Seal boundary admission")
	}
}

func TestSourceAssemblySealPoisonsPendingBoundariesAtomically(t *testing.T) {
	composition := boundaryLifecycleComposition(t)
	source := engine.NewSourceAssembly(composition)
	leftDecision, leftDecisionOK := source.Decision(facadeKey(50))
	rightDecision, rightDecisionOK := source.Decision(facadeKey(51))
	leftScope, leftScopeOK := source.Scope(leftDecision)
	rightScope, rightScopeOK := source.Scope(rightDecision)
	truth, truthOK := source.TrueExpr()
	leftSite, leftSiteOK := source.Site(facadeKey(52), leftScope, truth, true)
	rightSite, rightSiteOK := source.Site(facadeKey(53), rightScope, truth, true)
	leftReindex, leftReindexOK := source.IdentityReindex(leftScope)
	valid, validOK := source.Boundary(leftSite, leftSite, facadeKey(54), truth, leftReindex, truth)
	invalid, invalidOK := source.Boundary(leftSite, rightSite, facadeKey(55), truth, leftReindex, truth)
	if !leftDecisionOK || !rightDecisionOK || !leftScopeOK || !rightScopeOK || !truthOK || !leftSiteOK || !rightSiteOK || !leftReindexOK || !validOK || !invalidOK {
		t.Fatal("pending boundary set admission")
	}
	if valid.Available() || invalid.Available() {
		t.Fatal("pending boundary became available before Seal")
	}
	if source.Seal() {
		t.Fatal("malformed boundary set sealed")
	}
	if valid.Available() || invalid.Available() {
		t.Fatal("partial boundary publication after poisoned Seal")
	}
	if _, ok := source.Scope(); ok {
		t.Fatal("poisoned source issued a new control capability")
	}
	if source.Seal() {
		t.Fatal("poisoned source sealed on retry")
	}
}

func TestSourceAssemblyBoundaryAdmissionRejectsForeignOwner(t *testing.T) {
	composition := boundaryLifecycleComposition(t)
	source := engine.NewSourceAssembly(composition)
	foreign := engine.NewSourceAssembly(composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(facadeKey(56), scope, truth, true)
	reindex, reindexOK := source.IdentityReindex(scope)
	if !scopeOK || !truthOK || !siteOK || !reindexOK {
		t.Fatal("owner-law source setup")
	}
	if _, ok := foreign.Boundary(site, site, facadeKey(57), truth, reindex, truth); ok {
		t.Fatal("foreign source admitted another owner's boundary")
	}
	copyOfSource := *source
	if _, ok := copyOfSource.Boundary(site, site, facadeKey(58), truth, reindex, truth); ok {
		t.Fatal("copied source admitted a boundary")
	}
}
