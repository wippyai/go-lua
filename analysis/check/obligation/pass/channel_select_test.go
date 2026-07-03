package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
)

func TestChannelSelectsRefutesMissingReceiveCase(t *testing.T) {
	checked := testutil.CheckFile(`
type Event = { id: string }
type Timer = { elapsed: number }
type Stop = { reason: string }
function consume(primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>)
	local selected = channel.select {
		primary:case_receive(),
		timers:case_receive(),
		stops:case_receive(),
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == timers then
		return tostring(selected.value.elapsed)
	end
	return ""
end
`, "test.lua", testutil.WithStdlib(), testutil.WithManifest("channel", testutil.ChannelManifest()), testutil.WithGlobals("channel"))

	got := channelSelectJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("channel-select judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeChannelSelect {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeChannelSelect)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if !judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailChannelSelectHandled) ||
		!judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailChannelSelectMissing) ||
		!judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailChannelSelectNoDefault) {
		t.Fatalf("evidence = %#v, want handled, missing, and no-default details", got[0].Evidence)
	}
}

func TestChannelSelectsSkipsExhaustiveAndDefaultSelects(t *testing.T) {
	for _, src := range []string{
		`
type Event = { id: string }
type Timer = { elapsed: number }
function consume(primary: Channel<Event>, timers: Channel<Timer>)
	local selected = channel.select {
		primary:case_receive(),
		timers:case_receive(),
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == timers then
		return tostring(selected.value.elapsed)
	end
	return ""
end
`,
		`
type Event = { id: string }
type Timer = { elapsed: number }
type Stop = { reason: string }
function consume(primary: Channel<Event>, timers: Channel<Timer>, stops: Channel<Stop>)
	local selected = channel.select {
		primary:case_receive(),
		timers:case_receive(),
		stops:case_receive(),
		default = true,
	}
	if selected.channel == primary then
		return selected.value.id
	elseif selected.channel == timers then
		return tostring(selected.value.elapsed)
	end
	return ""
end
`,
	} {
		checked := testutil.CheckFile(src, "test.lua", testutil.WithStdlib(), testutil.WithManifest("channel", testutil.ChannelManifest()), testutil.WithGlobals("channel"))
		if got := channelSelectJudgmentsForAllBodies(checked); len(got) != 0 {
			t.Fatalf("channel-select judgments = %#v, want none", got)
		}
	}
}

func channelSelectJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.ChannelSelects{}).Run(obligationpass.Context{
			FunctionKey: "fixture:channel-select",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}
