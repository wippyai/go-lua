package pass_test

import (
	"strings"
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestUnresolvedValuesRefutesImplicitGlobalRead(t *testing.T) {
	result := testutil.CheckFile(`local x = missing + known`, "test.lua", testutil.WithGlobals("known")).RootResult()
	if result == nil {
		t.Fatal("RootResult nil")
	}

	got := obligationpass.New(obligationpass.UnresolvedValues{}).Run(obligationpass.Context{
		FunctionKey: "fixture:unresolved",
		SourceFile:  "test.lua",
		Reader:      readmodel.New(result),
	})
	if len(got) != 1 {
		t.Fatalf("judgments = %d, want one unresolved value: %#v", len(got), got)
	}
	item := got[0]
	if item.Code != judgment.CodeUnresolvedValue || item.Verdict != judgment.VerdictRefuted {
		t.Fatalf("judgment = %#v, want unresolved refuted value", item)
	}
	if item.Subject.Kind != judgment.SubjectPath || item.Subject.Label != "missing" {
		t.Fatalf("subject = %#v, want path subject labelled missing", item.Subject)
	}
	if stable := item.Subject.StableKey(); !strings.Contains(stable, "fixture:unresolved|path|unresolved-value:") {
		t.Fatalf("stable key = %q", stable)
	}
	if len(item.Spans) != 1 || item.Spans[0].File != "test.lua" || item.Spans[0].StartLine != 1 || item.Spans[0].StartCol != 11 {
		t.Fatalf("spans = %#v, want missing identifier source span", item.Spans)
	}
	if len(item.Evidence) != 1 || item.Evidence[0].Kind != judgment.EvidenceAbstractFact || item.Evidence[0].Trust != judgment.EvidenceTrustProven {
		t.Fatalf("evidence = %#v, want one proven abstract lookup fact", item.Evidence)
	}
}
