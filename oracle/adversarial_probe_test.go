package oracle

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/composite"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// The adversarial probe lane is an attack surface, not a feature suite. Each
// probe is a program written to make the analyzer admit something that is
// false at run time, or to publish evidence it never derived. A probe that
// fails to break the analyzer is a fence and stays; a probe that succeeds is a
// standing finding and stays red until the hole is closed at its cause.
//
// A probe therefore always states the CORRECT judgment. It is never reworded
// to match what the analyzer does today: the red is the deliverable.
//
// Every probe is one synthetic module sealed through the same Link path the
// frozen corpus uses, so a probe cannot pass because it took a different road
// into the analyzer than a fixture does.
//
// A probe has three verdicts, not two. GREEN means the analyzer refused the
// attack. RED means the attack succeeded. BLOCKED means the analyzer could not
// carry the program far enough to have a verdict at all - the probe is neither
// satisfied nor broken, and reporting it as either would be a fabrication of
// exactly the kind this lane exists to catch.

// adversarialProbeEvidence is one evidence row of a published finding. Trust is
// the column the laundering probes read: it separates what the analyzer derived
// from what the program's author merely asserted.
type adversarialProbeEvidence struct {
	kind   string
	trust  string
	detail string
}

// adversarialProbeFinding is one published report row reduced to what a probe
// judges it by.
type adversarialProbeFinding struct {
	code     string
	message  string
	line     uint32
	column   uint32
	severity anadiag.FindingSeverity
	evidence []adversarialProbeEvidence
}

func (finding adversarialProbeFinding) String() string {
	rendered := fmt.Sprintf("%s at %d:%d: %s", finding.code, finding.line, finding.column, finding.message)
	for _, evidence := range finding.evidence {
		rendered += fmt.Sprintf("\n      evidence[%s/%s] %s", evidence.kind, evidence.trust, evidence.detail)
	}
	return rendered
}

// adversarialProbeOutcome is one probe program's complete published answer: the
// diagnostic rows the report carries and the detached Result the placement
// plane is read from.
type adversarialProbeOutcome struct {
	findings  []adversarialProbeFinding
	result    *result.Result
	schema    placementdomain.Schema
	placement corpusPlacementObservation
}

// matching selects the published rows whose code contains every supplied
// fragment. A probe names the judgment it demands by its code family rather
// than by a rendered message, so a message rewording cannot retire a fence.
func (outcome adversarialProbeOutcome) matching(fragments ...string) []adversarialProbeFinding {
	matched := make([]adversarialProbeFinding, 0, len(outcome.findings))
	for _, finding := range outcome.findings {
		hit := true
		for _, fragment := range fragments {
			if !strings.Contains(finding.code, fragment) {
				hit = false
				break
			}
		}
		if hit {
			matched = append(matched, finding)
		}
	}
	return matched
}

// mentioning selects rows whose code or rendered message names the fragment.
// It is for probes whose judgment is about a specific property of a value
// rather than a specific collector: reading any diagnostic as satisfaction
// would let an unrelated defect in the probe source stand in for the judgment
// the probe demands.
func (outcome adversarialProbeOutcome) mentioning(fragment string) []adversarialProbeFinding {
	matched := make([]adversarialProbeFinding, 0, len(outcome.findings))
	for _, finding := range outcome.findings {
		if strings.Contains(finding.code, fragment) || strings.Contains(strings.ToLower(finding.message), fragment) {
			matched = append(matched, finding)
		}
	}
	return matched
}

func (outcome adversarialProbeOutcome) render() string {
	if len(outcome.findings) == 0 {
		return "      <no diagnostics>"
	}
	rows := make([]string, 0, len(outcome.findings))
	for _, finding := range outcome.findings {
		rows = append(rows, "      "+finding.String())
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// adversarialProbePolicy enables every collectable diagnostic the sealed
// declaration table publishes. A probe must not be able to pass because the
// judgment it attacks was left out of the policy it ran under.
func adversarialProbePolicy(t *testing.T, compilation composite.Compilation) (anadiag.DiagnosticPolicy, string) {
	t.Helper()
	table, tableOK := composite.Diagnostics(compilation)
	if !tableOK {
		return anadiag.DiagnosticPolicy{}, "sealed diagnostic declaration table unavailable"
	}
	enabled := make([]anadiag.DiagnosticCode, 0, table.Count())
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK {
			return anadiag.DiagnosticPolicy{}, fmt.Sprintf("sealed diagnostic declaration row %d unavailable", position)
		}
		if entry.Collectable() {
			enabled = append(enabled, entry.Code())
		}
	}
	if len(enabled) == 0 {
		return anadiag.DiagnosticPolicy{}, "sealed diagnostic declaration table publishes no collectable code"
	}
	return anadiag.DiagnosticPolicy{Enabled: enabled}, ""
}

// adversarialProbeTry seals one synthetic module and runs it through the whole
// published path: compile, solve under the full diagnostic policy, and project
// the detached Result's placement family.
//
// It returns the stage that stopped it rather than failing the test, because a
// probe that never reached a judgment has no verdict, and silently reading that
// as "no diagnostic" would let the lane report an unreachable analyzer as a
// clean one.
func adversarialProbeTry(t *testing.T, source string) (adversarialProbeOutcome, string) {
	t.Helper()
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		return adversarialProbeOutcome{}, "composition: sealed composition unavailable"
	}
	linked, err := testfixture.SealSource(corpusHarnessContract(t), "main.lua", []byte(source))
	if err != nil {
		return adversarialProbeOutcome{}, fmt.Sprintf("link: %v", err)
	}
	plan, compileStatus, compileDiagnostics := analysis.CompileWithDiagnostics(linked)
	if compileStatus != analysis.CompileComplete || plan == nil {
		return adversarialProbeOutcome{}, fmt.Sprintf("compile: status=%v stage=%v binding=%v rule=%v",
			compileStatus, compileDiagnostics.AssembleStage, compileDiagnostics.Binding, compileDiagnostics.Rule)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close probe plan")
		}
	})
	schema, schemaOK := plan.PlacementSchema()
	if !schemaOK {
		return adversarialProbeOutcome{}, "compile: compiled plan has no Placement schema authority"
	}
	policy, policyDefect := adversarialProbePolicy(t, compilation)
	if policyDefect != "" {
		return adversarialProbeOutcome{}, "policy: " + policyDefect
	}
	analysisResult, report, status, solveDiagnostics := plan.SolveWithReport(context.Background(), corpusHarnessSolveOptions(), policy)
	if status != analysis.AnalyzeComplete || analysisResult == nil {
		return adversarialProbeOutcome{}, fmt.Sprintf("solve: status=%v reason=%v engine=%s",
			status, solveDiagnostics.Reason, corpusHarnessEngineFailure(solveDiagnostics))
	}
	if report == nil || !report.Available() {
		return adversarialProbeOutcome{}, "report: solve published no diagnostic report"
	}
	if failure := report.CollectionFailure(); failure != anadiag.DiagnosticCollectionOK {
		return adversarialProbeOutcome{}, fmt.Sprintf("report: diagnostic collection failure=%d", failure)
	}
	outcome := adversarialProbeOutcome{result: analysisResult, schema: schema}
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		if !findingOK {
			return adversarialProbeOutcome{}, fmt.Sprintf("report: row %d unavailable", index)
		}
		row := adversarialProbeFinding{
			code: finding.Code().String(), message: finding.Message(), severity: finding.Severity(),
		}
		if location, locationOK := finding.Location(); locationOK {
			row.line, row.column = location.Start()
		}
		for position := 0; position < finding.EvidenceCount(); position++ {
			evidence, evidenceOK := finding.EvidenceAt(position)
			if !evidenceOK {
				continue
			}
			row.evidence = append(row.evidence, adversarialProbeEvidence{
				kind: evidence.Kind(), trust: evidence.Trust(), detail: evidence.Detail(),
			})
		}
		outcome.findings = append(outcome.findings, row)
	}
	outcome.placement = corpusPlacementProjection(analysisResult, schema)
	return outcome, ""
}

// adversarialSourceProbe is one attack. judge states the correct judgment and
// returns the reason the analyzer failed to make it, or the empty string when
// the attack was refused.
type adversarialSourceProbe struct {
	name    string
	surface string
	source  string
	judge   func(adversarialProbeOutcome) string
}

// adversarialSourceProbes is the scoreboard. Each entry documents the false
// statement the analyzer would have to make for the attack to succeed.
func adversarialSourceProbes() []adversarialSourceProbe {
	return []adversarialSourceProbe{
		{
			// Two things are wrong with this call: it supplies one argument
			// where two are required, and the argument it does supply is a
			// string where a number is required. The count is the cheaper
			// defect to detect, and detecting it must not stand in for
			// checking the argument that was actually written. A caller who
			// fixes only the count is left with a call the analyzer already
			// had the evidence to reject and did not.
			name:    "arity-mismatch-does-not-silence-the-argument",
			surface: "conformance walk",
			source: `
local function g(x: number, y: number): number return x + y end
local function f(): number
    return g("wrong")
end
return f
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.findings) == 0 {
					return "a call that is wrong in both arity and argument type published no diagnostic at all"
				}
				if len(outcome.matching("argument")) == 0 {
					return "the wrong-typed argument was not named; the arity refusal stood in for judging it:\n" + outcome.render()
				}
				return ""
			},
		},
		{
			// The same call with the right number of arguments. This is the
			// control for the probe above: if the argument type is not judged
			// even here, the probe above is measuring the wrong thing.
			name:    "wrong-argument-type-alone-is-refused",
			surface: "conformance walk",
			source: `
local function g(x: number, y: number): number return x + y end
local function f(): number
    return g("wrong", 2)
end
return f
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.matching("argument")) == 0 {
					return "a string argument in a number parameter published no argument diagnostic:\n" + outcome.render()
				}
				return ""
			},
		},
		{
			// forward returns a string followed by an open tail. The first
			// assignment target is therefore provably a string, whatever the
			// tail turns out to hold. An open tail makes the LENGTH of the
			// result list unknown; it says nothing about the values in the
			// fixed prefix, so the prefix stays decidable and a mismatch in it
			// stays a mismatch.
			name:    "open-tail-does-not-hide-a-fixed-prefix-mismatch",
			surface: "conformance walk",
			source: `
local function forward(...)
    return "tag", ...
end
local function f(): number
    local first: number, rest: number = forward(1)
    return first + rest
end
return f
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.findings) == 0 {
					return "a provably string first result bound to a number local published no diagnostic; the open tail silenced the fixed prefix"
				}
				return ""
			},
		},
		{
			// The receiver is an argument. Passing a record of the wrong shape
			// where a method's self parameter is declared is the same defect as
			// passing it in any other position, and a boundary that judges
			// every parameter except the first is not judging the call.
			name:    "method-receiver-is-judged-against-its-self-parameter",
			surface: "conformance walk",
			source: `
type Store = { cache: {[string]: string}, get: (self: Store, key: string) -> string }
type Counter = { count: number }

local function build(): Store
    local store: Store = { cache = {}, get = function(self: Store, key: string): string return "x" end }
    return store
end

local function f(): string
    local store: Store = build()
    local other: Counter = { count = 1 }
    return store.get(other, "k")
end
return f
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.findings) == 0 {
					return "a Counter passed where a Store self parameter is declared published no diagnostic; the receiver was judged against nothing"
				}
				return ""
			},
		},
		{
			// A non-nil claim is the author asserting what the analyzer could
			// not derive. It is allowed to silence the diagnostic. It is not
			// allowed to become the analyzer's own evidence: a finding that
			// rests on a claim must say so, because every consumer that elides
			// a runtime guard is entitled to do so only on derived proof.
			name:    "a-non-nil-claim-is-never-published-as-proven",
			surface: "trust laundering",
			source: `
local function lookup(cache: {[string]: string}, key: string): string
    local value = cache[key]
    local name: string = value!
    return name
end
return lookup
`,
			judge: func(outcome adversarialProbeOutcome) string {
				defects := make([]string, 0)
				for _, finding := range outcome.findings {
					for _, evidence := range finding.evidence {
						if evidence.trust != "proven" {
							continue
						}
						if strings.Contains(evidence.kind, "assertion") {
							defects = append(defects, fmt.Sprintf("%s: %s evidence %q is published as proven", finding.code, evidence.kind, evidence.detail))
						}
					}
				}
				if len(defects) != 0 {
					return "an authored assertion was published as derived proof:\n      " + strings.Join(defects, "\n      ")
				}
				return ""
			},
		},
		{
			// A cast states what a value is to be treated as. It cannot make a
			// later provable mismatch go away: after the cast the local is a
			// string, and a string is not a number, and that is a mismatch the
			// analyzer derived rather than one the author asserted.
			name:    "a-cast-does-not-silence-a-later-provable-mismatch",
			surface: "trust laundering",
			source: `
local function f(value: any): number
    local text = value :: string
    local count: number = text
    return count
end
return f
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.findings) == 0 {
					return "a string-cast local assigned to a number local published no diagnostic; the cast laundered the mismatch"
				}
				return ""
			},
		},
		{
			// The module's only export is a function that is never called
			// inside it. Its allocation still happens - the caller is whoever
			// requires this module - so the allocation has a placement, and
			// "this body was never called here" is not a reason to publish no
			// judgment for it. An unjudged allocation is one the runtime must
			// treat as escaping, which is the answer this analyzer exists to
			// improve on.
			name:    "an-uncalled-returned-body-still-places-its-allocations",
			surface: "declared entry",
			source: `
type Upload = { id: string, size: number }
type View = { id: string, human_size: string }

local function materialize(u: Upload): View
    local kb: number = u.size / 1024
    local view: View = { id = u.id, human_size = tostring(kb) }
    return view
end

return materialize
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.placement.operational) != 0 {
					return "placement projection is operationally unavailable: " + strings.Join(outcome.placement.operational, "; ")
				}
				if outcome.placement.noFact != 0 {
					return fmt.Sprintf("%d allocation(s) in the returned-but-uncalled body carry no placement fact", outcome.placement.noFact)
				}
				if outcome.placement.classCounts[placementdomain.OwnedHeap] == 0 {
					return fmt.Sprintf("the record allocated and returned inside the actor is not owned-heap; classes=%v", outcome.placement.classCounts)
				}
				return ""
			},
		},
		{
			// The regression fence for the conformance projection, at the level
			// a user meets it. An annotated local whose initializer cannot
			// carry the declared type is a mismatch; if the declaration cannot
			// be resolved, the site must be refused rather than proved. Either
			// way the one thing that must not happen is silence.
			name:    "an-annotated-local-is-measured-against-its-declaration",
			surface: "fabrication fence",
			source: `
type Handle = { id: string }
local function f(): number
    local handle: Handle = 42
    return 1
end
return f
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.findings) == 0 {
					return "an integer initializer for a record-declared local published no diagnostic"
				}
				return ""
			},
		},
		{
			// The wire seam's user-level witness. A host function declared to
			// return an optional is consumed with no nil guard; the declaration
			// is the only evidence about that call, so the guard is owed.
			name:    "an-optional-host-return-is-not-consumed-unguarded",
			surface: "fabrication fence",
			source: `
local function f(): string
    local info = debug.getinfo(1)
    return info.source
end
return f
`,
			judge: func(outcome adversarialProbeOutcome) string {
				if len(outcome.mentioning("nil")) == 0 {
					return "a debug.getinfo result declared optional was indexed with no nil guard and no diagnostic named nil:\n" + outcome.render()
				}
				return ""
			},
		},
	}
}

// TestAdversarialSourceProbeScoreboard runs every source-level attack and
// publishes one table of verdicts.
//
// The verdicts are kept apart on purpose. A RED is a hole in the analyzer and
// is owed a fix at its cause. A BLOCKED entry is not a hole and not a pass: it
// is a probe the analyzer could not carry far enough to answer, and collapsing
// those two into one number is how a lane starts reporting an unreachable
// analyzer as a clean one.
func TestAdversarialSourceProbeScoreboard(t *testing.T) {
	probes := adversarialSourceProbes()
	green := make([]string, 0, len(probes))
	red := make([]string, 0)
	blocked := make([]string, 0)
	for _, probe := range probes {
		outcome, blocker := adversarialProbeTry(t, probe.source)
		switch {
		case blocker != "":
			blocked = append(blocked, fmt.Sprintf("  BLOCKED %-52s [%s]\n      %s", probe.name, probe.surface, blocker))
		default:
			if defect := probe.judge(outcome); defect != "" {
				red = append(red, fmt.Sprintf("  RED     %-52s [%s]\n      %s", probe.name, probe.surface, defect))
				continue
			}
			green = append(green, fmt.Sprintf("  GREEN   %-52s [%s]", probe.name, probe.surface))
		}
	}
	t.Logf("adversarial source scoreboard: %d green, %d red, %d blocked\n%s",
		len(green), len(red), len(blocked), strings.Join(green, "\n"))
	if len(blocked) != 0 {
		t.Errorf("%d probe(s) could not be carried to a verdict by the current analyzer:\n%s",
			len(blocked), strings.Join(blocked, "\n"))
	}
	if len(red) != 0 {
		t.Errorf("%d probe(s) broke the analyzer:\n%s", len(red), strings.Join(red, "\n"))
	}
}

// TestAdversarialProbeHarnessSeesTheJudgmentsItAttacks is the lane's own fence.
// Every probe above reads "no diagnostic" as evidence, so the harness must be
// able to publish one at all, must stay quiet on a clean program, and must be
// able to read the placement plane. A lane that lost any of those would report
// the entire attack surface as green.
func TestAdversarialProbeHarnessSeesTheJudgmentsItAttacks(t *testing.T) {
	t.Run("a refused program publishes a row", func(t *testing.T) {
		outcome, blocker := adversarialProbeTry(t, `
local function g(x: number, y: number): number return x + y end
local function f(): number
    return g(1)
end
return f
`)
		if blocker != "" {
			t.Fatalf("the harness could not analyze a call that omits a required argument, so no probe can read absence as evidence: %s", blocker)
		}
		if len(outcome.findings) == 0 {
			t.Fatal("the harness published no row for a call that omits a required argument")
		}
	})
	t.Run("a clean program publishes no row", func(t *testing.T) {
		outcome, blocker := adversarialProbeTry(t, `
local function g(x: number, y: number): number return x + y end
local function f(): number
    return g(1, 2)
end
return f
`)
		if blocker != "" {
			t.Fatalf("the harness could not analyze a clean program: %s", blocker)
		}
		if len(outcome.findings) != 0 {
			t.Fatalf("the harness published rows for a clean program, so every probe reads noise as a judgment:\n%s", outcome.render())
		}
	})
	t.Run("the placement plane is readable", func(t *testing.T) {
		outcome, blocker := adversarialProbeTry(t, `
type View = { id: string }
local view: View = { id = "x" }
return view
`)
		if blocker != "" {
			t.Fatalf("the harness could not analyze a single record allocation: %s", blocker)
		}
		if len(outcome.placement.operational) != 0 {
			t.Fatalf("placement projection is operationally unavailable, so no placement probe has a verdict: %v", outcome.placement.operational)
		}
	})
}
