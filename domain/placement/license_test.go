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

// TestAllocationSiteLicenseKindSetIsClosed states the law the closed kind
// catalog exists for: the stable order enumerates every declared kind exactly
// once, and every kind carries its own storage in the record. A kind added to
// the catalog and missed by the enumeration, by With, or by State is a license
// that silently reads Unknown forever, so the sentinel is the denominator here
// rather than the enumeration itself.
func TestAllocationSiteLicenseKindSetIsClosed(t *testing.T) {
	kinds := AllocationSiteLicenseKinds()
	if len(kinds) != int(licenseKindLimit) {
		t.Fatalf("kind order covers %d of %d declared kinds", len(kinds), licenseKindLimit)
	}
	seen := make(map[LicenseKind]bool, len(kinds))
	for _, kind := range kinds {
		if kind >= licenseKindLimit {
			t.Fatalf("kind order carries undeclared kind %d", kind)
		}
		if seen[kind] {
			t.Fatalf("kind order repeats kind %d", kind)
		}
		seen[kind] = true
	}

	// Each kind owns a distinct field: setting one kind proven leaves every
	// other kind unknown, so two kinds cannot share one cell.
	for _, kind := range kinds {
		record := AllocationSiteLicenses{}.With(kind, LicenseProven)
		if got := record.State(kind); got != LicenseProven {
			t.Fatalf("kind %d does not round-trip: state = %v", kind, got)
		}
		for _, other := range kinds {
			if other == kind {
				continue
			}
			if got := record.State(other); got != LicenseUnknown {
				t.Fatalf("kind %d writes kind %d: state = %v", kind, other, got)
			}
		}
	}
}

// TestAllocationSiteLicenseProjectionIsTotal states that the wire projection
// reads every declared kind. A kind the projection drops is evidence the
// analysis established and the plan never sees.
func TestAllocationSiteLicenseProjectionIsTotal(t *testing.T) {
	proven := AllocationSiteLicenses{}
	for _, kind := range AllocationSiteLicenseKinds() {
		proven = proven.With(kind, LicenseProven)
	}
	projection := proven.Projection()
	fields := map[LicenseKind]bool{
		LicenseAllocationSite:       projection.AllocationSite,
		LicenseDecomposable:         projection.Decomposable,
		LicenseFrameLocalUse:        projection.FrameLocalUseProof,
		LicenseFrameLocal:           projection.FrameLocal,
		LicenseDiesBeforeSuspension: projection.DiesBeforeSuspension,
	}
	if len(fields) != int(licenseKindLimit) {
		t.Fatalf("projection reads %d of %d declared kinds", len(fields), licenseKindLimit)
	}
	for kind, value := range fields {
		if !value {
			t.Fatalf("projection drops proven kind %d", kind)
		}
	}
	if !projection.HasDiesBeforeSuspension {
		t.Fatal("projection drops the lifetime presence bit")
	}

	// Unknown projects conservatively rather than as a proof.
	empty := AllocationSiteLicenses{}.Projection()
	if empty.AllocationSite || empty.Decomposable || empty.FrameLocalUseProof || empty.FrameLocal ||
		empty.DiesBeforeSuspension || empty.HasDiesBeforeSuspension {
		t.Fatalf("unknown record projects as evidence: %+v", empty)
	}
}
