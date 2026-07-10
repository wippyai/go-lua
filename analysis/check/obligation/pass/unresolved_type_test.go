package pass_test

import (
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestUnresolvedTypesRefutesKnownOutOfScopeType(t *testing.T) {
	result := testutil.CheckFile(`
if true then
	type LocalPoint = {x: number}
end
local p: LocalPoint = {x = 1}
`, "test.lua").RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.UnresolvedTypes{}).Run(obligationpass.Context{
		FunctionKey: "fixture:types",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want one unresolved type: %#v", len(got), got)
	}
	item := got[0]
	if item.Code != judgment.CodeUnresolvedType || item.Verdict != judgment.VerdictRefuted {
		t.Fatalf("judgment = %#v, want unresolved refuted type", item)
	}
	if item.Subject.Kind != judgment.SubjectPath || item.Subject.Label != "LocalPoint" {
		t.Fatalf("subject = %#v, want path subject labelled LocalPoint", item.Subject)
	}
	if stable := item.Subject.StableKey(); !strings.Contains(stable, "fn:fixture\\ctypes") ||
		!strings.Contains(stable, "kind:path") ||
		!strings.Contains(stable, "bind_kind:type") ||
		!strings.Contains(stable, "bind:type\\cLocalPoint") ||
		!strings.Contains(stable, "name:LocalPoint") ||
		!strings.Contains(stable, "role:type.unresolved") ||
		!strings.Contains(stable, "ord:0") {
		t.Fatalf("stable key = %q", stable)
	}
	if len(item.Spans) != 1 || item.Spans[0].DisplayFile() != "test.lua" || item.Spans[0].StartLine != 5 || item.Spans[0].StartCol != 10 {
		t.Fatalf("spans = %#v, want LocalPoint source span", item.Spans)
	}
	if len(item.Evidence) != 1 || item.Evidence[0].Kind != judgment.EvidenceAbstractFact || item.Evidence[0].Trust != judgment.EvidenceTrustProven {
		t.Fatalf("evidence = %#v, want one proven abstract lookup fact", item.Evidence)
	}
}
