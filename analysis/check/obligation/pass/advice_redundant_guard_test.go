package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestAdviceRedundantGuardsUsesDominatingNilAndTypeProofsAndSkipsInvalidatedCheck(t *testing.T) {
	checked := testutil.CheckFile(`
local function redundant(value: string?): string
	if value ~= nil then
		if value ~= nil then
			return value
		end
	end
	return ""
end

local function needed(value: string?): string
	if value ~= nil then
		value = nil
		if value ~= nil then
			return value
		end
	end
	return ""
end

local function typed(value: string | number): string
	if type(value) == "string" then
		if type(value) ~= "number" then
			return value
		end
	end
	return ""
end
`, "test.lua", testutil.WithStdlib())

	var got []judgment.Judgment
	for _, result := range checked.BodyResults() {
		got = append(got, obligationpass.New(obligationpass.AdviceRedundantGuards{}).Run(obligationpass.Context{
			FunctionKey: "fixture:advice-redundant-guard",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	if len(got) != 2 {
		t.Fatalf("redundant-guard judgments = %d, want 2: %#v", len(got), got)
	}
	wantLabels := map[string]bool{"value ~= nil": false, `type(value) is not "number"`: false}
	for _, item := range got {
		if item.Code != judgment.CodeAdviceAlwaysTrueGuard || item.Verdict != judgment.VerdictProven {
			t.Fatalf("judgment = (%s, %v), want (%s, %v)", item.Code, item.Verdict, judgment.CodeAdviceAlwaysTrueGuard, judgment.VerdictProven)
		}
		if _, ok := wantLabels[item.Subject.Label]; !ok {
			t.Fatalf("subject label = %q, want nil/type guard", item.Subject.Label)
		}
		wantLabels[item.Subject.Label] = true
		if len(item.Spans) != 2 || item.Spans[0].File != "test.lua" || item.Spans[1].File != "test.lua" {
			t.Fatalf("spans = %#v, want current and dominating proof spans", item.Spans)
		}
		if !judgmentHasEvidenceDetail(item, judgment.EvidenceDetailAdviceGuardValue) {
			t.Fatalf("evidence = %#v, want solved guard-value proof", item.Evidence)
		}
	}
	for label, found := range wantLabels {
		if !found {
			t.Fatalf("missing solved guard judgment for %q", label)
		}
	}
}
