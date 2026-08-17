package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
)

// The mount phase rejects one axis's own authority. Which axis that is belongs
// to the declaration table, so this boundary carries one verdict for the whole
// phase and the axis travels beside it. The laws below state that: the verdict
// does not grow a member per coordinate space, and the axis it is about is
// never lost.

// TestAxisAuthorityIsOneVerdictCarryingTheAxis states the collapse. Every
// declared axis projects onto the same verdict, and the axis that raised it is
// recoverable from the phase's own record, named as its owner declared it.
func TestAxisAuthorityIsOneVerdictCarryingTheAxis(t *testing.T) {
	if composite.AxisCount() == 0 {
		t.Fatal("the sealed table declares no axis; the law measures nothing")
	}
	for position := 0; position < composite.AxisCount(); position++ {
		key, keyOK := composite.AxisKeyAt(position)
		if !keyOK {
			t.Fatalf("axis position %d publishes no key", position)
		}
		classification := composite.DiagnosticAxisForKey(key)
		if classification == composite.DiagnosticAxisUnknown {
			t.Fatalf("axis %q classifies as unknown", key)
		}
		failure := composite.MountFailure{Stage: composite.MountStageAxis, Axis: classification}
		if verdict := programMountFailure(failure); verdict != ProgramBindingFailureAxisAuthority {
			t.Fatalf("axis %q projects onto verdict %q, not the one axis-authority verdict", key, verdict)
		}
		if failure.Axis.String() != string(key) {
			t.Fatalf("the verdict for axis %q carries %q", key, failure.Axis.String())
		}
	}
}

// TestAxisAuthorityVerdictRendersItsOwnName states that the collapsed member is
// spelled, so a caller reading the boundary is not left with an ordinal the
// name table has no row for.
func TestAxisAuthorityVerdictRendersItsOwnName(t *testing.T) {
	if got := ProgramBindingFailureAxisAuthority.String(); got != "axis-authority" {
		t.Fatalf("the axis-authority verdict renders as %q", got)
	}
	if got := ProgramBindingFailureRuntimeContexts.String(); got != "runtime-contexts" {
		t.Fatalf("the runtime-contexts verdict renders as %q", got)
	}
}

// TestMountPhaseWithoutAnAxisNamesNoAxisVerdict states the phase half: a
// rejection that is not an axis's own seal is not reported as one, and a
// rejection that names no axis carries no axis identity.
func TestMountPhaseWithoutAnAxisNamesNoAxisVerdict(t *testing.T) {
	if verdict := programMountFailure(composite.MountFailure{}); verdict != ProgramBindingFailureNone {
		t.Fatalf("an unavailable mount verdict projected onto %q", verdict)
	}
	for _, stage := range []composite.MountStage{composite.MountStageTable, composite.MountStageInput, composite.MountStageAdopt} {
		failure := composite.MountFailure{Stage: stage}
		if verdict := programMountFailure(failure); verdict != ProgramBindingFailureInput {
			t.Fatalf("mount stage %q projected onto %q", stage, verdict)
		}
	}
	unnamed := composite.MountFailure{Stage: composite.MountStageAxis}
	if verdict := programMountFailure(unnamed); verdict != ProgramBindingFailureInput {
		t.Fatalf("an axis phase naming no axis projected onto %q", verdict)
	}
}

// TestPostMountDerivationKeepsItsOwnVerdict states that moving the two
// post-mount derivations into the mount phase kept this boundary's evidence:
// a topology that did not seal and an activation catalog that did not seal are
// each still their own verdict, distinct from every axis authority and from the
// phase's input rejection.
func TestPostMountDerivationKeepsItsOwnVerdict(t *testing.T) {
	verdicts := map[composite.MountStage]ProgramBindingFailure{
		composite.MountStageTopology:   ProgramBindingFailureHeapIndex,
		composite.MountStageActivation: ProgramBindingFailureTargetCatalog,
	}
	for stage, want := range verdicts {
		if verdict := programMountFailure(composite.MountFailure{Stage: stage}); verdict != want {
			t.Fatalf("derivation stage %q projected onto %q, not %q", stage, verdict, want)
		}
	}
	if ProgramBindingFailureHeapIndex.String() != "heap-index" || ProgramBindingFailureTargetCatalog.String() != "target-catalog" {
		t.Fatalf("a derivation verdict lost its own name: %q %q", ProgramBindingFailureHeapIndex, ProgramBindingFailureTargetCatalog)
	}
}
