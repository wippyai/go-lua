package body_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
)

func TestSendSafetyFrozenPayloadUsesSendInputFact(t *testing.T) {
	checked := testutil.CheckFile(`local pid: string = "worker"
local sealed = { id = "sealed" }
table.freeze(sealed)
process.send(pid, "sealed", sealed)`, "test.lua",
		testutil.WithStdlib(),
		testutil.WithManifest("process", testutil.ProcessManifest()),
		testutil.WithGlobals("process"),
	)

	occurrences := sendSafetyOccurrences(checked)
	if len(occurrences) != 1 {
		t.Fatalf("send safety occurrences = %d, want 1: %#v", len(occurrences), occurrences)
	}
	if !occurrences[0].Frozen || occurrences[0].Verdict != body.SendSafetyProvenImmutable {
		t.Fatalf("send safety occurrence = %#v, want frozen immutable proof", occurrences[0])
	}
}

func TestSendSafetyDoesNotUseConditionalFrozenFact(t *testing.T) {
	checked := testutil.CheckFile(`local function send(flag: boolean, pid: string)
  local sealed = { id = "sealed" }
  if flag then
    table.freeze(sealed)
  end
  process.send(pid, "sealed", sealed)
end`, "test.lua",
		testutil.WithStdlib(),
		testutil.WithManifest("process", testutil.ProcessManifest()),
		testutil.WithGlobals("process"),
	)

	occurrences := sendSafetyOccurrences(checked)
	if len(occurrences) != 1 {
		t.Fatalf("send safety occurrences = %d, want 1: %#v", len(occurrences), occurrences)
	}
	if occurrences[0].Frozen || occurrences[0].Verdict != body.SendSafetyUnknown {
		t.Fatalf("send safety occurrence = %#v, want unknown without an all-path frozen proof", occurrences[0])
	}
}

func sendSafetyOccurrences(checked testutil.Result) []body.SendSafetyOccurrence {
	var out []body.SendSafetyOccurrence
	for _, result := range checked.BodyResults() {
		for _, point := range result.Graph().RPO() {
			out = append(out, result.SendSafetyOccurrences(point)...)
		}
	}
	return out
}
