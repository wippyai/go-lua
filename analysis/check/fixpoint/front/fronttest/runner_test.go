package fronttest

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestStarterCorpusIsSortedAndCoversLanguageFamilies(t *testing.T) {
	cases := StarterCorpus()
	if len(cases) != 144 {
		t.Fatalf("starter corpus has %d cases, want 144", len(cases))
	}
	families := map[string]int{}
	for index, test := range cases {
		if index > 0 && cases[index-1].Name >= test.Name {
			t.Fatalf("corpus is not sorted at %q", test.Name)
		}
		family, _, found := strings.Cut(test.Name, "/")
		if !found {
			t.Fatalf("case %q has no family", test.Name)
		}
		families[family]++
	}
	for _, family := range []string{"allocation", "assignment", "branch", "call", "channel-select", "claim", "diagnostic", "expression", "loop", "outcome", "pathstore", "provider"} {
		if families[family] == 0 {
			t.Fatalf("starter corpus has no %s case", family)
		}
	}
	for _, family := range []string{"outcome", "provider"} {
		if families[family] < 8 {
			t.Fatalf("%s harness cases = %d, want at least 8", family, families[family])
		}
	}
}

func TestProductionRunnerExecutesAdversarialLuaSemanticsCases(t *testing.T) {
	var adversarial []Case
	for _, test := range StarterCorpus() {
		if strings.HasPrefix(test.Name, "adversarial/") {
			adversarial = append(adversarial, test)
		}
	}
	if len(adversarial) < 16 {
		t.Fatalf("adversarial Lua semantics cases = %d, want at least 16", len(adversarial))
	}
	assertProductionCaseResults(t, adversarial)
}

func TestProductionRunnerExecutesExpressionCases(t *testing.T) {
	var expressions []Case
	for _, test := range StarterCorpus() {
		if strings.HasPrefix(test.Name, "expression/") {
			expressions = append(expressions, test)
		}
	}
	if len(expressions) < 4 {
		t.Fatalf("expression harness cases = %d, want at least 4", len(expressions))
	}
	assertProductionCaseResults(t, expressions)
}

func TestProductionRunnerExecutesClaimCases(t *testing.T) {
	var claims []Case
	for _, test := range StarterCorpus() {
		if strings.HasPrefix(test.Name, "claim/") {
			claims = append(claims, test)
		}
	}
	if len(claims) < 4 {
		t.Fatalf("claim harness cases = %d, want at least 4", len(claims))
	}
	assertProductionCaseResults(t, claims)
}

func TestProductionRunnerExecutesAdjustedReturnAndMemberWriteCases(t *testing.T) {
	wanted := map[string]bool{
		"outcome/open-return-tail-publishes-head-result":          true,
		"outcome/open-return-tail-keeps-preceding-slot":           true,
		"outcome/parenthesized-return-call-is-one-adjusted-slot":  true,
		"pathstore/member-write-from-first-temporary-is-admitted": true,
	}
	var selected []Case
	for _, test := range StarterCorpus() {
		if wanted[test.Name] {
			selected = append(selected, test)
		}
	}
	if len(selected) != len(wanted) {
		t.Fatalf("adjusted return/member-write harness cases = %d, want %d", len(selected), len(wanted))
	}
	assertProductionCaseResults(t, selected)
}

func TestProductionRunnerExecutesAllocationCompletedWriteCases(t *testing.T) {
	var allocations []Case
	for _, test := range StarterCorpus() {
		if strings.HasPrefix(test.Name, "allocation/constructor-") {
			allocations = append(allocations, test)
		}
	}
	if len(allocations) != 4 {
		t.Fatalf("allocation completed-write harness cases = %d, want 4", len(allocations))
	}
	assertProductionCaseResults(t, allocations)
}

func TestProductionRunnerExecutesAssignmentCases(t *testing.T) {
	var assignments []Case
	for _, test := range StarterCorpus() {
		if strings.HasPrefix(test.Name, "assignment/") {
			assignments = append(assignments, test)
		}
	}
	if len(assignments) < 8 {
		t.Fatalf("assignment harness cases = %d, want at least 8", len(assignments))
	}
	assertProductionCaseResults(t, assignments)
}

// assertProductionCaseResults verifies every case's language-semantic
// contract, rather than merely that the batch produced one report per input.
func assertProductionCaseResults(t *testing.T, cases []Case) {
	t.Helper()
	runner := Runner{Front: ProductionFront, Engine: ProductionEngine}
	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			report, err := runner.Run(test)
			if err != nil {
				t.Fatal(err)
			}
			want := Expectation{
				Published:   canonicalPublished(test.Expect.Published),
				Diagnostics: canonicalDiagnostics(test.Expect.Diagnostics),
			}
			if !samePublished(want.Published, report.Actual.Published) || !sameDiagnostics(want.Diagnostics, report.Actual.Diagnostics) {
				t.Fatalf("production output = %#v, want published=%#v diagnostics=%#v", report.Actual, want.Published, want.Diagnostics)
			}
		})
	}
}

func TestStarterCorpusContainsLuaSource(t *testing.T) {
	for _, test := range StarterCorpus() {
		if _, err := parse.ParseString(test.Source, test.Name); err != nil {
			t.Fatalf("parse %s: %v", test.Name, err)
		}
	}
}

func TestRunnerCanonicalizesPublishedOrderBeforeExactComparison(t *testing.T) {
	test := Case{
		Name:   "law/canonical-order",
		Source: "local a, b = 1, 2",
		Expect: Expectation{Published: []PublishedOutcome{value("a", "1"), value("b", "2")}},
	}
	runner := Runner{
		Front: FrontFunc(func(string) (equation.Artifact, error) { return equation.Artifact{}, nil }),
		Engine: EngineFunc(func(string) (Results, error) {
			return Results{Published: []PublishedOutcome{value("b", "2"), value("a", "1")}}, nil
		}),
	}
	report, err := runner.Run(test)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := report.Actual.Published; got[0] != value("a", "1") || got[1] != value("b", "2") {
		t.Fatalf("canonical published order = %#v", got)
	}
}

func TestRunnerFailureShowsExactChannelDiff(t *testing.T) {
	runner := Runner{
		Front: FrontFunc(func(string) (equation.Artifact, error) { return equation.Artifact{}, nil }),
		Engine: EngineFunc(func(string) (Results, error) {
			return Results{
				Published:   []PublishedOutcome{value("actual", "2")},
				Diagnostics: []DiagnosticCandidate{{Code: "unexpected", Subject: "x", Detail: "bad"}},
			}, nil
		}),
	}
	_, err := runner.Run(Case{
		Name:   "law/exact-diff",
		Source: "local expected = 1",
		Expect: Expectation{
			Published:   []PublishedOutcome{value("expected", "1")},
			Diagnostics: []DiagnosticCandidate{{Code: "missing", Subject: "x", Detail: "needed"}},
		},
	})
	if err == nil {
		t.Fatal("Run succeeded with different results")
	}
	message := err.Error()
	for _, fragment := range []string{
		"published (-want +got):",
		`- value[expected] = "1"`,
		`+ value[actual] = "2"`,
		"diagnostics (-want +got):",
		`- missing[x] = "needed"`,
		`+ unexpected[x] = "bad"`,
	} {
		if !strings.Contains(message, fragment) {
			t.Errorf("failure report missing %q:\n%s", fragment, message)
		}
	}
}

func TestRunAllOrdersFailuresByCaseName(t *testing.T) {
	runner := Runner{
		Front: FrontFunc(func(source string) (equation.Artifact, error) {
			return equation.Artifact{}, errors.New(source)
		}),
		Engine: EngineFunc(func(string) (Results, error) { t.Fatal("Engine called after compile failure"); return Results{}, nil }),
	}
	_, err := runner.RunAll([]Case{
		{Name: "z-last", Source: "z"},
		{Name: "a-first", Source: "a"},
	})
	if err == nil {
		t.Fatal("RunAll succeeded")
	}
	message := err.Error()
	if strings.Index(message, "a-first") > strings.Index(message, "z-last") {
		t.Fatalf("failure order is not deterministic:\n%s", message)
	}
}
