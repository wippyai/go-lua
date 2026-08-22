package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/identity"
)

// TestCommittedProgramAdmissionIsSealedByConstruction pins that a committed
// program's ownership verdict is reached exactly once, on the finished value.
// The sealed field is the verdict every accessor reads, the zero value carries
// the false verdict, and the self fence keeps a copy out of the verdict.
func TestCommittedProgramAdmissionIsSealedByConstruction(t *testing.T) {
	program, _, constructed := stageTotalityConstruct(t, true)
	if !constructed || program == nil {
		t.Fatal("sealed program construction")
	}
	if !program.admitted || !program.valid() {
		t.Fatalf("sealed verdict admitted=%v valid=%v", program.admitted, program.valid())
	}
	if !program.artifactBacked || !program.contextLayout.Available() || !program.contextIndex.Available() {
		t.Fatal("sealed program does not exercise the compact context plane")
	}

	if (&CommittedProgram{}).valid() {
		t.Fatal("zero committed program admitted")
	}
	if (*CommittedProgram)(nil).valid() {
		t.Fatal("nil committed program admitted")
	}

	copied := *program
	if copied.valid() {
		t.Fatal("a copied committed program passed the self fence")
	}

	unsealed := *program
	unsealed.self = &unsealed
	unsealed.admitted = false
	if unsealed.valid() {
		t.Fatal("an unsealed committed program was admitted")
	}
}

// TestCommittedProgramSealedVerdictEqualsTheDerivation is the equivalence law
// for the ownership proof moved to construction time: the sealed field states
// exactly what the full derivation states, on an admitted program and on a
// program whose context plane no longer belongs to it.
func TestCommittedProgramSealedVerdictEqualsTheDerivation(t *testing.T) {
	program, _, constructed := stageTotalityConstruct(t, true)
	if !constructed || program == nil {
		t.Fatal("sealed program construction")
	}
	if program.admitted != program.deriveAdmission() {
		t.Fatalf("sealed verdict %v disagrees with the derivation %v", program.admitted, program.deriveAdmission())
	}

	// A foreign point-owner vector is exactly what the per-call Layout owner
	// digest used to re-prove. The derivation must still refuse it, so the
	// seal is a settled proof and not an unconditional yes.
	foreign := *program
	foreign.self = &foreign
	foreign.pointOwners = append([]contextfiber.PointOwner(nil), program.pointOwners...)
	foreign.pointOwners[0] = contextfiber.PointOwner{}
	if foreign.deriveAdmission() {
		t.Fatal("the derivation admitted a foreign point-owner vector")
	}
	if foreign.sealProgramAdmission() || foreign.valid() {
		t.Fatal("the seal admitted a foreign point-owner vector")
	}
}

// TestCommittedProgramReadPathAllocatesNothing pins the cost of the sealed
// verdict: a pure read of the committed program - the validity fence and the
// compact executable-state address of a published query - allocates nothing.
// The per-call ownership derivation it replaces allocated one framed digest
// vector over the whole point-owner plane on every accessor.
func TestCommittedProgramReadPathAllocatesNothing(t *testing.T) {
	program, _, constructed := stageTotalityConstruct(t, true)
	if !constructed || program == nil || !program.valid() {
		t.Fatal("sealed program construction")
	}
	if len(program.queries) == 0 {
		t.Fatal("sealed program published no query row")
	}
	id := program.queries[0].id
	query, resolved := program.Query(id)
	if !resolved {
		t.Fatal("published query row")
	}
	if _, ok := query.StateOrdinal(); !ok {
		t.Fatal("published query state ordinal")
	}

	if allocations := testing.AllocsPerRun(200, func() {
		if !program.valid() {
			t.Fatal("sealed verdict")
		}
	}); allocations != 0 {
		t.Fatalf("committed program validity allocated %v objects per call", allocations)
	}
	if allocations := testing.AllocsPerRun(200, func() {
		if _, ok := query.StateOrdinal(); !ok {
			t.Fatal("published query state ordinal")
		}
	}); allocations != 0 {
		t.Fatalf("committed query state read allocated %v objects per call", allocations)
	}
}

// TestContentIdentityDerivationAllocatesNothing pins the framed digest
// construction shared by every owner in the analysis: deriving one content
// identity from a domain tag and its ordered payload allocates nothing. It is
// the single hottest identity operation in a solve, so a per-call frame buffer
// there is paid once per derived identity across the whole engine.
func TestContentIdentityDerivationAllocatesNothing(t *testing.T) {
	left, leftOK := identity.DeriveContentID("analysis/engine/derive-allocation-law/v1", []byte("alpha"))
	right, rightOK := identity.DeriveContentID("analysis/engine/derive-allocation-law/v1", []byte("alpha"))
	if !leftOK || !rightOK || left != right {
		t.Fatal("content identity derivation is not deterministic")
	}
	payload := left
	if allocations := testing.AllocsPerRun(500, func() {
		if _, ok := identity.DeriveContentID("analysis/engine/derive-allocation-law/v1", payload[:]); !ok {
			t.Fatal("content identity derivation")
		}
	}); allocations != 0 {
		t.Fatalf("content identity derivation allocated %v objects per call", allocations)
	}
}
