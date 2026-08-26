package suspensionevidence_test

import (
	"testing"

	evidencefixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/suspensionevidence"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// TestTargetRuntimeParity runs SuspensionEvidence through its real
// declaration, compile/check/mount/bootstrap, solve, and canonical generic
// snapshot. The family codec then decodes the owner-issued Evidence value.
func TestTargetRuntimeParity(t *testing.T) {
	fixture := evidencefixture.New(t)
	result, ok := fixture.Solve()
	if !ok || !result.Available() {
		t.Fatal("suspension-evidence target solve")
	}
	if result.Evaluations() != 1 || result.Publications() != 1 {
		t.Fatalf("suspension-evidence target solve = evaluations:%d publications:%d, want 1/1", result.Evaluations(), result.Publications())
	}
	cell, evidence, ok := fixture.Evidence(result)
	if !ok || !cell.Available() || !cell.Lineage.Available() || !cell.Presence.Is(model.Present) {
		t.Fatal("suspension-evidence target snapshot metadata")
	}
	if evidence != fixture.Expected() {
		t.Fatalf("suspension-evidence target snapshot evidence = %v, want %v", evidence, fixture.Expected())
	}
	if key, keyOK := fixture.EvidenceKey(result); !keyOK || !key.Available() {
		t.Fatal("suspension-evidence target snapshot key")
	}
}
