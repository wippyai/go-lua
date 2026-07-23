package fronttest

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// Front is the only front-end capability the runner needs.
type Front interface {
	CompileBody(source string) (equation.Artifact, error)
}

// Engine is the only checking capability the runner needs.  Results are
// already normalized to this package's public corpus vocabulary, keeping the
// corpus independent of engine-internal fact keys and renderers.
type Engine interface {
	Check(source string) (Results, error)
}

// Results are the normalized channels published by the checking engine.
type Results struct {
	Published   []PublishedOutcome
	Diagnostics []DiagnosticCandidate
}

// FrontFunc and EngineFunc make the production functions and test doubles fit
// the runner without a wrapper type at every call site.
type FrontFunc func(string) (equation.Artifact, error)

func (fn FrontFunc) CompileBody(source string) (equation.Artifact, error) { return fn(source) }

type EngineFunc func(string) (Results, error)

func (fn EngineFunc) Check(source string) (Results, error) { return fn(source) }

// ProductionFront invokes the front's walking-skeleton entrypoint.
var ProductionFront Front = FrontFunc(front.CompileBody)

// ProductionEngine adapts engine.Check's current fact channels to the corpus
// vocabulary.  The adapter is deliberately small: semantic naming belongs in
// the engine, not in fixture files.
var ProductionEngine Engine = EngineFunc(checkProduction)

func checkProduction(source string) (Results, error) {
	result, err := engine.Check(source)
	if err != nil {
		return Results{}, err
	}
	published := make([]PublishedOutcome, 0, len(result.Values)+len(result.Outcomes))
	for _, fact := range result.Values {
		published = append(published, PublishedOutcome{Channel: "value", Subject: fact.Key, Value: string(fact.Value)})
	}
	for _, fact := range result.Outcomes {
		// Branch selectors and narrowing markers are intermediate equation
		// evidence. The corpus observes Lua-visible values and returns, not
		// internal control-flow coordinates whose operation names change as the
		// front gains the writes needed to close a value chain.
		if strings.HasPrefix(fact.Key, "branch/") || strings.HasPrefix(fact.Key, "narrowing/") {
			continue
		}
		published = append(published, PublishedOutcome{Channel: "outcome", Subject: fact.Key, Value: string(fact.Value)})
	}
	diagnostics := make([]DiagnosticCandidate, 0, len(result.Diagnostics))
	for _, fact := range result.Diagnostics {
		diagnostics = append(diagnostics, DiagnosticCandidate{Code: fact.Key, Detail: string(fact.Value)})
	}
	return Results{Published: canonicalPublished(published), Diagnostics: canonicalDiagnostics(diagnostics)}, nil
}

// Runner compiles every case through Front and checks it through Engine.
// It canonicalizes every report before comparing it, so report order never
// depends on map iteration or scheduler timing.
type Runner struct {
	Front  Front
	Engine Engine
}

// Report is the canonical observation produced by a successful run.
type Report struct {
	Case     string
	Artifact equation.Artifact
	Actual   Results
}

// Run executes one case and requires exact equality on every expected channel.
func (r Runner) Run(test Case) (Report, error) {
	if err := validateCase(test); err != nil {
		return Report{}, &Failure{Case: test.Name, Stage: "definition", Cause: err}
	}
	if r.Front == nil {
		return Report{}, &Failure{Case: test.Name, Stage: "compile", Cause: errors.New("fronttest: nil Front")}
	}
	if r.Engine == nil {
		return Report{}, &Failure{Case: test.Name, Stage: "check", Cause: errors.New("fronttest: nil Engine")}
	}
	artifact, err := r.Front.CompileBody(test.Source)
	if err != nil {
		return Report{}, &Failure{Case: test.Name, Stage: "compile", Cause: err}
	}
	actual, err := r.Engine.Check(test.Source)
	if err != nil {
		return Report{}, &Failure{Case: test.Name, Stage: "check", Cause: err}
	}
	actual.Published = canonicalPublished(actual.Published)
	actual.Diagnostics = canonicalDiagnostics(actual.Diagnostics)
	want := Expectation{
		Published:   canonicalPublished(test.Expect.Published),
		Diagnostics: canonicalDiagnostics(test.Expect.Diagnostics),
	}
	if !samePublished(want.Published, actual.Published) || !sameDiagnostics(want.Diagnostics, actual.Diagnostics) {
		return Report{Case: test.Name, Artifact: artifact, Actual: actual}, &Failure{
			Case: test.Name, Stage: "expectation", Want: want, Got: actual,
		}
	}
	return Report{Case: test.Name, Artifact: artifact, Actual: actual}, nil
}

// RunAll runs cases in lexical-name order, independent of the caller's slice
// order.  It reports every failure in that same order for reproducible diffs.
func (r Runner) RunAll(cases []Case) ([]Report, error) {
	ordered := append([]Case(nil), cases...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	for index := 1; index < len(ordered); index++ {
		if ordered[index-1].Name == ordered[index].Name {
			return nil, &Failure{
				Case:  ordered[index].Name,
				Stage: "definition",
				Cause: fmt.Errorf("duplicate case name %q", ordered[index].Name),
			}
		}
	}
	reports := make([]Report, 0, len(ordered))
	failures := make([]error, 0)
	for _, test := range ordered {
		report, err := r.Run(test)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		reports = append(reports, report)
	}
	return reports, errors.Join(failures...)
}

// Failure includes a compact, channel-by-channel exact diff.  It is useful in
// normal go test output without requiring a golden file or a legacy renderer.
type Failure struct {
	Case  string
	Stage string
	Cause error
	Want  Expectation
	Got   Results
}

func (f *Failure) Unwrap() error { return f.Cause }

func (f *Failure) Error() string {
	prefix := fmt.Sprintf("fronttest: %s: %s", f.Case, f.Stage)
	if f.Cause != nil {
		return prefix + ": " + f.Cause.Error()
	}
	var lines []string
	lines = append(lines, prefix+" mismatch")
	lines = append(lines, renderPublishedDiff(f.Want.Published, f.Got.Published)...)
	lines = append(lines, renderDiagnosticDiff(f.Want.Diagnostics, f.Got.Diagnostics)...)
	return strings.Join(lines, "\n")
}

func validateCase(test Case) error {
	if test.Name == "" {
		return errors.New("case name is empty")
	}
	if strings.TrimSpace(test.Source) == "" {
		return errors.New("source is empty")
	}
	published := make(map[string]struct{}, len(test.Expect.Published))
	for _, outcome := range test.Expect.Published {
		if outcome.Channel == "" || outcome.Subject == "" {
			return fmt.Errorf("malformed published outcome %#v", outcome)
		}
		key := publishedKey(outcome)
		if _, duplicate := published[key]; duplicate {
			return fmt.Errorf("duplicate published outcome %#v", outcome)
		}
		published[key] = struct{}{}
	}
	diagnostics := make(map[string]struct{}, len(test.Expect.Diagnostics))
	for _, diagnostic := range test.Expect.Diagnostics {
		if diagnostic.Code == "" {
			return fmt.Errorf("malformed diagnostic %#v", diagnostic)
		}
		key := diagnosticKey(diagnostic)
		if _, duplicate := diagnostics[key]; duplicate {
			return fmt.Errorf("duplicate diagnostic %#v", diagnostic)
		}
		diagnostics[key] = struct{}{}
	}
	return nil
}

func canonicalPublished(in []PublishedOutcome) []PublishedOutcome {
	out := append([]PublishedOutcome(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		return publishedKey(out[i]) < publishedKey(out[j])
	})
	return out
}

func canonicalDiagnostics(in []DiagnosticCandidate) []DiagnosticCandidate {
	out := append([]DiagnosticCandidate(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		return diagnosticKey(out[i]) < diagnosticKey(out[j])
	})
	return out
}

func publishedKey(outcome PublishedOutcome) string {
	return outcome.Channel + "\x00" + outcome.Subject + "\x00" + outcome.Value
}

func diagnosticKey(diagnostic DiagnosticCandidate) string {
	return diagnostic.Code + "\x00" + diagnostic.Subject + "\x00" + diagnostic.Detail
}

func samePublished(want, got []PublishedOutcome) bool {
	if len(want) != len(got) {
		return false
	}
	for index := range want {
		if want[index] != got[index] {
			return false
		}
	}
	return true
}

func sameDiagnostics(want, got []DiagnosticCandidate) bool {
	if len(want) != len(got) {
		return false
	}
	for index := range want {
		if want[index] != got[index] {
			return false
		}
	}
	return true
}

func renderPublishedDiff(want, got []PublishedOutcome) []string {
	return renderDiff("published", stringifyPublished(want), stringifyPublished(got))
}

func renderDiagnosticDiff(want, got []DiagnosticCandidate) []string {
	return renderDiff("diagnostics", stringifyDiagnostics(want), stringifyDiagnostics(got))
}

func renderDiff(channel string, want, got []string) []string {
	missing, unexpected := subtract(want, got), subtract(got, want)
	if len(missing) == 0 && len(unexpected) == 0 {
		return nil
	}
	lines := []string{"  " + channel + " (-want +got):"}
	for _, item := range missing {
		lines = append(lines, "  - "+item)
	}
	for _, item := range unexpected {
		lines = append(lines, "  + "+item)
	}
	return lines
}

func subtract(left, right []string) []string {
	counts := make(map[string]int, len(right))
	for _, item := range right {
		counts[item]++
	}
	var result []string
	for _, item := range left {
		if counts[item] == 0 {
			result = append(result, item)
			continue
		}
		counts[item]--
	}
	return result
}

func stringifyPublished(items []PublishedOutcome) []string {
	result := make([]string, len(items))
	for index, item := range items {
		result[index] = fmt.Sprintf("%s[%s] = %q", item.Channel, item.Subject, item.Value)
	}
	return result
}

func stringifyDiagnostics(items []DiagnosticCandidate) []string {
	result := make([]string, len(items))
	for index, item := range items {
		result[index] = fmt.Sprintf("%s[%s] = %q", item.Code, item.Subject, item.Detail)
	}
	return result
}
