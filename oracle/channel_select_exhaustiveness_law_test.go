package oracle

import (
	"context"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	typedomain "github.com/wippyai/go-lua/domain/type"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestChannelSelectMissingIfArmCollectsExhaustivenessFromSnapshotFacts(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	const source = `type Event = {kind: string}
type Stop = {reason: string}
type Time = {sec: number}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>, timeout_ch: Channel<Time>): string
    local result = channel.select {
        events_ch:case_receive(),
        { channel = events_ch, value = 1, ok = true, default = nil },
        stop_ch:case_receive(),
        timeout_ch:case_receive(),
    }
    if result.channel == events_ch then
        return result.value.kind
    elseif result.channel == stop_ch then
        return result.value.reason
    end
    return "timeout"
end
return handle
`
	linked, err := testfixture.SealSource(contract, "analysis.lua", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	offResult, offReport, offStatus, _ := plan.SolveWithReport(context.Background(), corpusHarnessSolveOptions(), anadiag.DiagnosticPolicy{})
	if offStatus != analysis.AnalyzeComplete || offResult == nil || offReport != nil {
		t.Fatalf("policy-off solve = %v result=%t report=%t", offStatus, offResult != nil, offReport != nil)
	}
	policy := anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.ChannelSelectExhaustivenessCode}}
	result, report, solveStatus, diagnostics := plan.SolveWithReport(context.Background(), corpusHarnessSolveOptions(), policy)
	if solveStatus != analysis.AnalyzeComplete || result == nil || report == nil || result.ContentID() != offResult.ContentID() {
		t.Fatalf("policy solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", solveStatus, result != nil, report != nil, result.ContentID(), offResult.ContentID(), diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("exhaustiveness report failure=%d findings=%d, want OK/1", report.CollectionFailure(), report.FindingCount())
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !findingOK || !locationOK || finding.Code() != typedomain.ChannelSelectExhaustivenessCode ||
		finding.Severity() != anadiag.FindingSeverityWarning || location.File() != "analysis.lua" ||
		line != 12 || column != 8 {
		t.Fatalf("exhaustiveness finding is not the first if site: code=%q loc=%s:%d:%d", finding.Code(), location.File(), line, column)
	}
	if finding.Message() != "channel select is not exhaustive; missing case: `timeout_ch`" {
		t.Fatalf("message = %q", finding.Message())
	}
	if finding.EvidenceCount() != 4 {
		t.Fatalf("evidence count = %d", finding.EvidenceCount())
	}
	want := []struct {
		kind, trust, detail string
	}{
		{"abstract fact", "proven", "branch chain checks channel `result.channel`"},
		{"abstract fact", "proven", "handled cases: `events_ch`, `stop_ch`"},
		{"missing proof", "unknown", "missing cases: `timeout_ch`"},
		{"missing proof", "unknown", "no default case handles the remaining channel cases"},
	}
	for index, expect := range want {
		evidence, evidenceOK := finding.EvidenceAt(index)
		if !evidenceOK || evidence.Kind() != expect.kind || evidence.Trust() != expect.trust || evidence.Detail() != expect.detail {
			t.Fatalf("evidence %d = kind=%q trust=%q detail=%q", index, evidence.Kind(), evidence.Trust(), evidence.Detail())
		}
	}
	rendered, renderedOK := finding.RenderSource("analysis.lua", source)
	if !renderedOK {
		t.Fatal("render refused the source")
	}
	for _, fragment := range []string{
		"warning[channel.select.exhaustiveness]: channel select is not exhaustive; missing case: `timeout_ch`",
		"if result.channel == events_ch then",
		"proven: handled cases: `events_ch`, `stop_ch`",
		"missing proof: missing cases: `timeout_ch`",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("render missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestChannelSelectDefaultCoversRemainingArms(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "analysis.lua", []byte(`type Event = {kind: string}
type Stop = {reason: string}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>): string
    local result = channel.select {
        events_ch:case_receive(),
        stop_ch:case_receive(),
        default = true,
    }
    if result.channel == events_ch then
        return "e"
    end
    return "d"
end
return handle
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	result, report, solveStatus, diagnostics := plan.SolveWithReport(
		context.Background(),
		corpusHarnessSolveOptions(),
		anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.ChannelSelectExhaustivenessCode}},
	)
	if solveStatus != analysis.AnalyzeComplete || result == nil || report == nil ||
		report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("default-covered report = %v result=%t report=%t failure=%d findings=%d diagnostics=%+v",
			solveStatus, result != nil, report != nil, report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}
