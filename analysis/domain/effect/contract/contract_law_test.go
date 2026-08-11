package contract

import (
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsAreCanonicalDetachedAndFailClosed(t *testing.T) {
	factor := testFactor(1)
	first, firstOK := Contracts(factor)
	second, secondOK := Contracts(factor)
	if !firstOK || !secondOK || !reflect.DeepEqual(first, second) {
		t.Fatal("Effect contracts changed across identical calls")
	}
	canonical, canonicalOK := coverage.SealContracts(first)
	if !canonicalOK || !reflect.DeepEqual(first, canonical) {
		t.Fatal("Effect contracts are not generically canonical")
	}
	first[0] = coverage.CoverageContract{}
	replayed, replayedOK := Contracts(factor)
	if !replayedOK || replayed[0] == (coverage.CoverageContract{}) {
		t.Fatal("Effect contracts retained caller mutation")
	}
	if got, ok := Contracts(engine.SemanticKey{}); ok || got != nil {
		t.Fatal("Effect contracts accepted an unavailable Factor")
	}
}

func TestJointOperandsShareTheirEffectJudgments(t *testing.T) {
	factor := testFactor(2)
	contracts, ok := Contracts(factor)
	if !ok {
		t.Fatal("Effect contracts")
	}
	assertSharedConclusion(t, contracts, factor, bodyCapability,
		[]sourceFacet{{semanticsource.OriginProgramFlowBody, 0}, {semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots}},
	)
	assertSharedConclusion(t, contracts, factor, selectedCallRow,
		[]sourceFacet{{semanticsource.OriginProgramFlowCall, 0}, {semanticsource.OriginTargetOperation, 0}, {semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect}, {semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect}, {semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback}, {semanticsource.OriginLinkProjectBaseApplication, 0}, {semanticsource.OriginLinkBoundary, 0}},
	)
	assertSharedConclusion(t, contracts, factor, outcomeCapability,
		[]sourceFacet{{semanticsource.OriginProgramFlowOutcome, 0}, {semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI}, {semanticsource.OriginLinkBoundary, 0}},
	)
	assertSharedConclusion(t, contracts, factor, opaqueBoundary,
		[]sourceFacet{{semanticsource.OriginProgramFlowCall, 0}, {semanticsource.OriginTargetOperation, 0}, {semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque}, {semanticsource.OriginLinkProjectBaseApplication, 0}, {semanticsource.OriginLinkBoundary, 0}},
	)
	assertSharedConclusion(t, contracts, factor, boundaryTransport,
		[]sourceFacet{{semanticsource.OriginProgramModuleImport, 0}, {semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest}, {semanticsource.OriginLinkBoundary, 0}, {semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport}, {semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget}},
	)
}

type sourceFacet struct {
	origin semanticsource.Origin
	facet  semanticsource.Facet
}

func assertSharedConclusion(t testing.TB, contracts []coverage.CoverageContract, factor engine.SemanticKey, want conclusion, operands []sourceFacet) {
	t.Helper()
	derived, ok := coverage.DeriveConclusion(factor, uint16(want), revision)
	if !ok {
		t.Fatal("derive Effect conclusion")
	}
	for _, operand := range operands {
		definition, found := semanticsource.Definition(operand.origin, operand.facet)
		if !found || !contains(contracts, definition.Token(), derived) {
			t.Fatalf("Effect source %d/%d did not share conclusion %d", operand.origin, operand.facet, want)
		}
	}
}

func TestContractsContainOnlyDeclaredSourceConclusionPairs(t *testing.T) {
	contracts, ok := Contracts(testFactor(3))
	if !ok || len(contracts) != len(obligations) {
		t.Fatal("Effect contract count")
	}
	seen := make(map[coverage.CoverageContract]struct{}, len(contracts))
	for _, contract := range contracts {
		if _, duplicate := seen[contract]; duplicate {
			t.Fatal("duplicate Effect source-conclusion pair")
		}
		seen[contract] = struct{}{}
	}
}

func TestExactEffectRowsIssueNewConclusionIdentities(t *testing.T) {
	factor := testFactor(4)
	current, currentOK := coverage.DeriveConclusion(factor, uint16(selectedCallRow), revision)
	retired, retiredOK := coverage.DeriveConclusion(factor, uint16(selectedCallRow), revision-1)
	if !currentOK || !retiredOK || current == retired {
		t.Fatal("exact effect-row contract did not retire its aggregate conclusion identity")
	}
}

func TestBuildPlanAssignsBodyCapabilityToBodyRule(t *testing.T) {
	factor := testFactor(5)
	keys := []engine.SemanticKey{testFactor(6), testFactor(7), testFactor(8), testFactor(9)}
	plan, ok := BuildPlan(factor, PlanBindings{Selected: keys[0], Opaque: keys[1], Body: keys[2], Query: keys[3]})
	if !ok || len(plan.Rules) != 3 || len(plan.Queries) != 1 {
		t.Fatalf("Effect body coverage plan: ok=%t rules=%d queries=%d", ok, len(plan.Rules), len(plan.Queries))
	}
	body, query := coverage.Requirement{}, coverage.Requirement{}
	var bodyCount, selectedBodyCount, queryBodyCount int
	for _, rule := range plan.Rules {
		for _, requirement := range rule.Covers {
			if requirement.Source.Origin() != semanticsource.OriginProgramFlowBody {
				continue
			}
			switch rule.Semantic {
			case keys[2]:
				body, bodyCount = requirement, bodyCount+1
			case keys[0]:
				selectedBodyCount++
			}
		}
	}
	for _, queryPlan := range plan.Queries {
		for _, requirement := range queryPlan.Covers {
			if requirement.Source.Origin() == semanticsource.OriginProgramFlowBody {
				query, queryBodyCount = requirement, queryBodyCount+1
			}
		}
	}
	if bodyCount != 1 || body.Source.Facet() != semanticsource.FacetProgramFlowBodyRoots ||
		selectedBodyCount != 0 || queryBodyCount != 1 || query.Source.Facet() != 0 {
		t.Fatalf("body capability lanes: body=%d/%d selected=%d query=%d/%d", body.Source.Origin(), body.Source.Facet(), selectedBodyCount, query.Source.Origin(), query.Source.Facet())
	}
}

func TestBuildPlanRequiresDedicatedBodyRuleIdentity(t *testing.T) {
	factor := testFactor(10)
	keys := []engine.SemanticKey{testFactor(11), testFactor(12), testFactor(13)}
	if plan, ok := BuildPlan(factor, PlanBindings{Selected: keys[0], Opaque: keys[1], Query: keys[2]}); ok || plan.Contracts != nil || plan.Rules != nil {
		t.Fatal("Effect coverage silently assigned body obligations to an existing lane")
	}
}

func contains(contracts []coverage.CoverageContract, source semanticsource.Token, conclusion engine.SemanticKey) bool {
	for _, contract := range contracts {
		if contract.Source == source && contract.Conclusion == conclusion {
			return true
		}
	}
	return false
}

func testFactor(n byte) engine.SemanticKey {
	key, ok := engine.NewSemanticKey(sha256.Sum256([]byte{n}), 1)
	if !ok {
		panic("test Factor")
	}
	return key
}
