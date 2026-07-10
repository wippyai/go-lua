package placement

import "testing"

func TestAllocationSiteLicenseJoinLaws(t *testing.T) {
	if got := LicenseProven.Join(LicenseUnknown); got != LicenseUnknown {
		t.Fatalf("proven join unknown = %v, want unknown", got)
	}
	for _, state := range []LicenseState{LicenseUnknown, LicenseRefuted, LicenseProven} {
		if got := LicenseRefuted.Join(state); got != LicenseRefuted {
			t.Fatalf("refuted join %v = %v, want refuted", state, got)
		}
		if got := state.Join(LicenseRefuted); got != LicenseRefuted {
			t.Fatalf("%v join refuted = %v, want refuted", state, got)
		}
	}

	proven := AllocationSiteLicenses{}
	for _, kind := range AllocationSiteLicenseKinds() {
		proven = proven.With(kind, LicenseProven)
	}
	unknown := proven.With(LicenseDecomposable, LicenseUnknown)
	refuted := proven.With(LicenseFrameLocal, LicenseRefuted)
	if got := proven.Join(unknown).State(LicenseDecomposable); got != LicenseUnknown {
		t.Fatalf("record decomposable join = %v, want unknown", got)
	}
	if got := proven.Join(refuted).State(LicenseFrameLocal); got != LicenseRefuted {
		t.Fatalf("record frame-local join = %v, want refuted", got)
	}
}
