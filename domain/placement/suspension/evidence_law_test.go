package suspension

import (
	"testing"

	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func TestEvidenceAllRoutesLaw(t *testing.T) {
	tests := []struct {
		name  string
		left  Evidence
		right Evidence
		want  Evidence
	}{
		{name: "proven", left: EvidenceProven, right: EvidenceProven, want: EvidenceProven},
		{name: "refuted", left: EvidenceRefuted, right: EvidenceRefuted, want: EvidenceRefuted},
		{name: "mixed", left: EvidenceProven, right: EvidenceRefuted, want: EvidenceUnknown},
		{name: "missing", left: EvidenceMissing, right: EvidenceProven, want: EvidenceProven},
		{name: "opaque", left: EvidenceUnknown, right: EvidenceRefuted, want: EvidenceUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.left.Join(test.right); got != test.want {
				t.Fatalf("Join(%v,%v) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestEvidencePublicProjectionKeepsMissingUnknown(t *testing.T) {
	for _, state := range []Evidence{EvidenceMissing, EvidenceUnknown} {
		if got := state.Public(); got != placementdomain.EvidenceUnknown {
			t.Fatalf("Public(%v) = %v, want Unknown", state, got)
		}
	}
	if EvidenceProven.Public() != placementdomain.EvidenceProven || EvidenceRefuted.Public() != placementdomain.EvidenceRefuted {
		t.Fatal("explicit suspension evidence did not retain its public polarity")
	}
}
