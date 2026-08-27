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
	invalid := Evidence(99)
	if got, ok := invalid.JoinChecked(EvidenceProven); ok || got.Valid() || got == EvidenceUnknown {
		t.Fatalf("invalid checked join = %v/%t, want refusal", got, ok)
	}
	if got := invalid.Join(EvidenceProven); got.Valid() || got == EvidenceUnknown {
		t.Fatalf("invalid lattice join = %v, want inadmissible sentinel", got)
	}
	lattice := Lattice()
	if lattice.Equal(invalid, invalid) {
		t.Fatal("invalid evidence became lattice-equivalent to itself")
	}
	if got := lattice.Meet(invalid, EvidenceProven); got.Valid() || got == EvidenceMissing {
		t.Fatalf("invalid lattice meet = %v, want inadmissible sentinel", got)
	}
}

// TestEvidencePublicProjectionSeparatesMissingFromUnknown is the publication
// law for this producer's private vocabulary. Missing is the sparse Factor
// default: no route published a row for the coordinate. Unknown is an
// authenticated all-routes verdict that no polarity survives. The projection
// into the public Placement plane must carry both distinctions across; it may
// not collapse the producer's own join identity into a semantic verdict.
func TestEvidencePublicProjectionSeparatesMissingFromUnknown(t *testing.T) {
	if EvidenceMissing.Public() == EvidenceUnknown.Public() {
		t.Fatal("the publication boundary erased the missing/unknown distinction")
	}
	if got := EvidenceMissing.Public(); got != placementdomain.EvidenceAbsent {
		t.Fatalf("Public(missing) = %v, want Absent", got)
	}
	if got := EvidenceUnknown.Public(); got != placementdomain.EvidenceUnknown {
		t.Fatalf("Public(unknown) = %v, want Unknown", got)
	}
	if EvidenceProven.Public() != placementdomain.EvidenceProven || EvidenceRefuted.Public() != placementdomain.EvidenceRefuted {
		t.Fatal("explicit suspension evidence did not retain its public polarity")
	}
	// A private state outside this producer's vocabulary has no public
	// projection. It must not be published as any admissible public state.
	if got := Evidence(99).Public(); got.Valid() {
		t.Fatalf("Public(invalid) = %v, want an inadmissible public state", got)
	}
}

func TestEvidenceCellAuthenticationOwnsSparseBottom(t *testing.T) {
	tests := []struct {
		name      string
		state     Evidence
		present   bool
		available bool
		want      bool
	}{
		{name: "sparse bottom", state: EvidenceMissing, available: true, want: true},
		{name: "present verdict", state: EvidenceProven, present: true, available: true, want: true},
		{name: "present bottom", state: EvidenceMissing, present: true, available: true},
		{name: "sparse verdict", state: EvidenceRefuted, available: true},
		{name: "unavailable", state: EvidenceMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := AuthenticateEvidenceCell(test.state, test.present, test.available)
			if ok != test.want || ok && got != test.state {
				t.Fatalf("authenticate %v/%t/%t = %v/%t, want %t", test.state, test.present, test.available, got, ok, test.want)
			}
		})
	}
}
