package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/semanticvocabulary"
)

func TestPlanDiagnosticsRejectInvalidOptionsAtSetup(t *testing.T) {
	plan, status := Compile(directFieldHostileLink(t, `return 1`))
	if status != CompileComplete || plan == nil {
		t.Fatal("diagnostic Plan fixture")
	}
	result, analyzeStatus, diagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{MaxWork: 1})
	if result != nil || analyzeStatus != AnalyzeInvalid || diagnostics.Phase != AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != AnalyzeDiagnosticReasonInvalidOptions {
		t.Fatalf("invalid diagnostic options = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
	result, analyzeStatus, diagnostics = plan.SolveWithDiagnostics(nil, engine.SolveDiagnosticOptions{})
	if result != nil || analyzeStatus != AnalyzeInvalid || diagnostics.Phase != AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != AnalyzeDiagnosticReasonInvalidPlan {
		t.Fatalf("invalid diagnostic context = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
	var zero Plan
	result, analyzeStatus, diagnostics = zero.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
	if result != nil || analyzeStatus != AnalyzeInvalid || diagnostics.Phase != AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != AnalyzeDiagnosticReasonInvalidPlan {
		t.Fatalf("invalid diagnostic plan = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
}

func TestAnalyzeDiagnosticRuleClassifiesEverySemanticBundleRule(t *testing.T) {
	bundle, sealed := semanticvocabulary.New()
	if !sealed {
		t.Fatal("semantic bundle")
	}
	cases := []struct {
		key  engine.SemanticKey
		rule AnalyzeDiagnosticRule
	}{
		{bundle.ValueSourceRule.Rule, AnalyzeDiagnosticRuleValueSource},
		{bundle.PackSourceRule.Rule, AnalyzeDiagnosticRulePackSource},
		{bundle.HeapIngressRule.Rule, AnalyzeDiagnosticRuleHeapIngress},
		{bundle.ValueAllocationRule.Rule, AnalyzeDiagnosticRuleValueAllocation},
		{bundle.HeapEmptyRule.Rule, AnalyzeDiagnosticRuleHeapEmpty},
		{bundle.HeapClosedRule.Rule, AnalyzeDiagnosticRuleHeapClosed},
		{bundle.RawGetRule.Rule, AnalyzeDiagnosticRuleRawGet},
		{bundle.RawSetRule.Rule, AnalyzeDiagnosticRuleRawSet},
		{bundle.CallDispatchRule.Rule, AnalyzeDiagnosticRuleCallDispatch},
		{bundle.EffectSelectedRule.Rule, AnalyzeDiagnosticRuleEffectSelected},
		{bundle.EffectOpaqueRule.Rule, AnalyzeDiagnosticRuleEffectOpaque},
		{bundle.EffectBodyRule.Rule, AnalyzeDiagnosticRuleEffectBody},
		{bundle.CallActivation, AnalyzeDiagnosticRuleCallActivation},
		{bundle.ValueBootstrapRule.Rule, AnalyzeDiagnosticRuleValueBootstrap},
		{bundle.HeapBootstrapRule.Rule, AnalyzeDiagnosticRuleHeapBootstrap},
		{bundle.ValueTransferRule.Rule, AnalyzeDiagnosticRuleValueTransfer},
		{bundle.ValueBinaryArithmeticRule.Rule, AnalyzeDiagnosticRuleValueBinaryArithmetic},
		{bundle.ValueBinaryEqualityRule.Rule, AnalyzeDiagnosticRuleValueBinaryEquality},
		{bundle.ValueBinaryOrderRule.Rule, AnalyzeDiagnosticRuleValueBinaryOrder},
		{bundle.ValuePresenceRefinementRule.Rule, AnalyzeDiagnosticRuleValuePresenceRefinement},
	}
	for _, test := range cases {
		if got := classifyDiagnosticRule(bundle, test.key); got != test.rule || got.String() == "unknown" {
			t.Fatalf("semantic rule = %s, want %s", got, test.rule)
		}
	}
	unknown, unknownOK := analysisSemanticKey("rule/unknown")
	if !unknownOK || classifyDiagnosticRule(bundle, engine.SemanticKey{}) != AnalyzeDiagnosticRuleUnknown || classifyDiagnosticRule(bundle, unknown) != AnalyzeDiagnosticRuleUnknown || AnalyzeDiagnosticRuleUnknown.String() != "unknown" {
		t.Fatal("unknown diagnostic Rule classification")
	}
}
