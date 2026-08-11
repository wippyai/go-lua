package engine_test

import (
	"testing"

	engine "github.com/wippyai/go-lua/analysis/engine"
)

// An external caller may declare an activation family and bind it to a
// structural rule, but plan topology is deliberately engine-internal.
func TestExternalActivationFamilyDeclarationBoundary(t *testing.T) {
	composition := engine.NewComposition()
	factor, factorOK := engine.DeclareFactor(composition, engine.FactorSpec[externalSummaryKey, uint64]{
		Semantic: externalSummarySemantic(80), KeyEnd: 1, Lattice: externalSummaryLattice(), Default: 0,
		AdmitAt: func(externalSummaryKey, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(*engine.Factor[externalSummaryKey, uint64]) bool { return true })
	completion, completionOK := engine.DeclareSupportCompletion(composition, externalSummarySemantic(81))
	prune, pruneOK := engine.DeclarePrune(completion, externalSummarySemantic(82))
	support, supportOK := engine.DeclareSupportRule(composition, engine.SupportRuleSpec{
		Semantic:   externalSummarySemantic(83),
		Completion: completion,
		Prune:      prune,
		Inputs:     0,
		Admission:  engine.AdmitSupportByTrustedTheorem(externalSummarySemantic(84)),
		Run:        func(value engine.Support) (engine.Support, bool) { return value, true },
	})
	query, queryOK := engine.DeclareSupportQuery(composition, externalSummarySemantic(85), func(engine.SupportObservation) uint64 { return 0 }, engine.FrozenResult[uint64]{
		Semantic:    externalSummarySemantic(86),
		Freeze:      func(value uint64) uint64 { return value },
		Clone:       func(value uint64) uint64 { return value },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
	})
	family, familyOK := engine.DeclareActivationFamily(composition, externalSummarySemantic(87))
	rule, ruleOK := engine.DeclareActivationRule(composition, engine.ActivationRuleSpec{
		Semantic:  externalSummarySemantic(88),
		Family:    family,
		Inputs:    0,
		Admission: engine.AdmitActivationByTrustedTheorem(externalSummarySemantic(89)),
		Run:       func(engine.Activation) bool { return true },
	})
	if !factorOK || factor == nil || !completionOK || !pruneOK || !supportOK || support == nil || !queryOK || query == nil ||
		!familyOK || !ruleOK || rule == nil || !composition.Seal() {
		t.Fatal("external activation family declaration")
	}
}
