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
	if len(cases) != 29 {
		t.Fatalf("starter corpus has %d cases, want 29", len(cases))
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
	for _, family := range []string{"assignment", "branch", "call", "diagnostic", "loop"} {
		if families[family] == 0 {
			t.Fatalf("starter corpus has no %s case", family)
		}
	}
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
	reports, err := (Runner{Front: ProductionFront, Engine: ProductionEngine}).RunAll(assignments)
	if err != nil {
		t.Fatalf("assignment production harness: %v", err)
	}
	if len(reports) != len(assignments) {
		t.Fatalf("assignment reports = %d, want %d", len(reports), len(assignments))
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
