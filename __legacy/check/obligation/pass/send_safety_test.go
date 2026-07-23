package pass

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestSendSafetyJudgmentClassifiesAdmissionVerdicts(t *testing.T) {
	tests := []struct {
		name        string
		input       readmodel.SendSafetyVerdict
		want        judgment.Verdict
		wantTrust   judgment.EvidenceTrust
		wantMissing bool
	}{
		{name: "isolated", input: readmodel.SendSafetyProvenIsolated, want: judgment.VerdictProven, wantTrust: judgment.EvidenceTrustProven},
		{name: "immutable", input: readmodel.SendSafetyProvenImmutable, want: judgment.VerdictProven, wantTrust: judgment.EvidenceTrustProven},
		{name: "escaped", input: readmodel.SendSafetyRefutedEscaped, want: judgment.VerdictRefuted, wantTrust: judgment.EvidenceTrustProven},
		{name: "unknown", input: readmodel.SendSafetyUnknown, want: judgment.VerdictUnknown, wantTrust: judgment.EvidenceTrustUnknown, wantMissing: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := sendSafetyJudgment(Context{SourceFile: "main.lua"}, "fixture", readmodel.SendSafety{
				Point:   cfg.Point(7),
				Target:  path.Path{Root: "payload"},
				Verdict: test.input,
				Reason:  "solved send-safety fact",
				Argument: readmodel.CallArgument{
					Index: 1,
					Span:  readmodel.SourceSpan{StartLine: 7, StartCol: 24, EndLine: 7, EndCol: 31},
				},
			})

			if item.Code != judgment.CodeSendIsolation || item.Verdict != test.want {
				t.Fatalf("judgment = (%s, %v), want (%s, %v)", item.Code, item.Verdict, judgment.CodeSendIsolation, test.want)
			}
			if item.Actual.Label != test.input.String() {
				t.Fatalf("actual label = %q, want %q", item.Actual.Label, test.input.String())
			}
			if item.Evidence[0].Trust != test.wantTrust {
				t.Fatalf("primary evidence trust = %v, want %v", item.Evidence[0].Trust, test.wantTrust)
			}
			if got := item.Evidence.Has(judgment.EvidenceMissingProof); got != test.wantMissing {
				t.Fatalf("has missing proof = %t, want %t", got, test.wantMissing)
			}
			if !judgment.DefaultRegistry().Validate(item) {
				t.Fatalf("send-safety judgment is not registry-valid: %#v", item)
			}
		})
	}
}
