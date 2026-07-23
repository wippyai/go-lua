package diagnostics

import (
	"os"
	"strings"
	"testing"
)

func TestLifecycleResourceReportOwnsMessageEvidenceAndHelp(t *testing.T) {
	report := newLifecycleResourceReport("tx", "transaction", "active", "`finished`")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "message",
			got:  report.Message(),
			want: "resource `tx` remains in transaction state `active` at function exit; expected `finished`",
		},
		{
			name: "acquire evidence",
			got:  report.AcquireEvidence("tx", "active"),
			want: "this call acquires `tx` as transaction:`active` and requires `finished` before local ownership ends",
		},
		{
			name: "transition evidence",
			got:  report.TransitionEvidence("tx", "active", "finished"),
			want: "this call transitions `tx` in protocol transaction from `active` to `finished` on a reachable path",
		},
		{
			name: "escape evidence",
			got:  report.EscapeEvidence("tx"),
			want: "this call escapes local ownership of `tx` in protocol transaction on a reachable path",
		},
		{
			name: "exit obligation evidence",
			got:  report.ExitObligationEvidence(),
			want: "exit state still has `tx` in protocol transaction at `active`; no proof reaches `finished` or escapes ownership on every path",
		},
		{
			name: "help",
			got:  report.Help(),
			want: "Transition `tx` to `finished` or escape ownership on every return path.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestLifecycleResourceReportIsTheResourceTextOwner(t *testing.T) {
	for _, file := range []string{"display.go", "display_core_messages.go"} {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{
			"resourceUnreleased",
			"resourceAcquire",
			"resourceTransition",
			"resourceEscape",
			"resourceExitObligation",
			"ResourceUnreleased",
			"ResourceAcquire",
			"ResourceTransition",
			"ResourceEscape",
			"ResourceExitObligation",
		} {
			if strings.Contains(string(content), banned) {
				t.Fatalf("%s contains lifecycle resource text route %q; lifecycleResourceReport owns this family", file, banned)
			}
		}
	}
}
