package diagnostic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
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
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	axisCount := composite.AxisCount(compilation)
	if axisCount == 0 {
		t.Fatal("the sealed table declares no axis; the law measures nothing")
	}
	for position := 0; position < axisCount; position++ {
		key, keyOK := composite.AxisKeyAt(compilation, position)
		if !keyOK {
			t.Fatalf("axis position %d publishes no key", position)
		}
		classification := composite.DiagnosticAxisForKey(compilation, key)
		if classification == composite.DiagnosticAxisUnknown {
			t.Fatalf("axis %q classifies as unknown", key)
		}
		failure := composite.MountFailure{Stage: composite.MountStageAxis, Axis: classification}
		if verdict := ProgramBindingFailureFromMount(failure); verdict != ProgramBindingFailureAxisAuthority {
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
}

// TestMountPhaseWithoutAnAxisNamesNoAxisVerdict states the phase half: a
// rejection that is not an axis's own seal is not reported as one, and a
// rejection that names no axis carries no axis identity.
func TestMountPhaseWithoutAnAxisNamesNoAxisVerdict(t *testing.T) {
	if verdict := ProgramBindingFailureFromMount(composite.MountFailure{}); verdict != ProgramBindingFailureNone {
		t.Fatalf("an unavailable mount verdict projected onto %q", verdict)
	}
	for _, stage := range []composite.MountStage{composite.MountStageTable, composite.MountStageInput, composite.MountStageAdopt} {
		failure := composite.MountFailure{Stage: stage}
		if verdict := ProgramBindingFailureFromMount(failure); verdict != ProgramBindingFailureInput {
			t.Fatalf("mount stage %q projected onto %q", stage, verdict)
		}
	}
	unnamed := composite.MountFailure{Stage: composite.MountStageAxis}
	if verdict := ProgramBindingFailureFromMount(unnamed); verdict != ProgramBindingFailureInput {
		t.Fatalf("an axis phase naming no axis projected onto %q", verdict)
	}
}

// TestPostMountDerivationKeepsItsOwnVerdict states that each surviving
// post-mount derivation remains its own boundary evidence: topology and formal
// refusal are distinct from every axis authority and from the phase's input
// rejection.
func TestPostMountDerivationKeepsItsOwnVerdict(t *testing.T) {
	verdicts := map[composite.MountStage]ProgramBindingFailure{
		composite.MountStageTopology: ProgramBindingFailureHeapIndex,
		composite.MountStageFormal:   ProgramBindingFailureTarget,
	}
	for stage, want := range verdicts {
		if verdict := ProgramBindingFailureFromMount(composite.MountFailure{Stage: stage}); verdict != want {
			t.Fatalf("derivation stage %q projected onto %q, not %q", stage, verdict, want)
		}
	}
	if ProgramBindingFailureHeapIndex.String() != "heap-index" || ProgramBindingFailureTarget.String() != "target" {
		t.Fatalf("a derivation verdict lost its own name: %q %q", ProgramBindingFailureHeapIndex, ProgramBindingFailureTarget)
	}
}

// TestConformanceObservationRequiresProducerGeometry states that a
// type-conformance row is admitted only with the execution geometry its
// measured value is read at. The value a conformance subject judges is
// published on the observation column of the rule occurrence that produces it,
// so a row that reached the collector without producers would name no readable
// address and could only abstain.
func TestConformanceObservationRequiresProducerGeometry(t *testing.T) {
	point := identity.ContentID{1}
	payload := Conformance{
		Site:        schemadiag.SiteCallArgument,
		Owner:       identity.ContentID{2},
		Measured:    identity.ContentID{3},
		Declared:    identity.ContentID{4},
		Span:        identity.ContentID{5},
		ValueID:     identity.ContentID{11},
		DeclaredMay: runtimekind.Bit(runtimekind.String),
		Target:      "string",
		Callee:      "takes_string",
		Evidence:    []identity.ContentID{point},
	}
	if payload.Available() {
		t.Fatal("a conformance row without producer geometry was admitted")
	}
	payload.Producers = []Producer{{
		Key:        schema.Key("value-transfer"),
		Occurrence: identity.ContentID{6},
		Point:      identity.ContentID{7},
		Anchor:     point,
	}}
	if !payload.Available() {
		t.Fatal("a conformance row with one anchored producer was refused")
	}
	payload.Producers[0].Anchor = identity.ContentID{8}
	if payload.Available() {
		t.Fatal("a producer anchored outside the row's evidence points was admitted")
	}
	// One measured value established on two paths keeps both producers: the
	// collector joins the value over all of them, and dropping either would
	// abstain on a violation the program can carry.
	payload.Producers = []Producer{
		{Key: schema.Key("value-transfer"), Occurrence: identity.ContentID{6}, Point: identity.ContentID{7}, Anchor: point},
		{Key: schema.Key("value-transfer"), Occurrence: identity.ContentID{9}, Point: identity.ContentID{10}, Anchor: point},
	}
	if !payload.Available() {
		t.Fatal("a value produced on two paths at one evidence point was refused")
	}
	payload.Producers[1].Point = payload.Producers[0].Point
	if payload.Available() {
		t.Fatal("two producers claiming one execution point were admitted")
	}
}

// TestConformanceSubjectsAddressTheirOwnProducer states the addressing law a
// statement with several measured values depends on. Two conformance subjects
// of one statement share a base evidence point and differ only in the
// occurrence that produces their value, so an address derived from the shared
// point would give both the same column. The address is the producer's.
func TestConformanceSubjectsAddressTheirOwnProducer(t *testing.T) {
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	mount := identity.ContentID{20}
	context, contextOK := executioncontext.NewContext(identity.ContentID{30}, mount, identity.ContentID{31}, identity.ContentID{32})
	if !contextOK {
		t.Fatal("observation context")
	}
	first, firstOK := ValueObservationAddress(compilation, structure.DiagnosticObservationTypeConformance, mount, identity.ContentID{21}, context)
	second, secondOK := ValueObservationAddress(compilation, structure.DiagnosticObservationTypeConformance, mount, identity.ContentID{22}, context)
	if !firstOK || !secondOK || !first.Available() || !second.Available() {
		t.Fatal("the type-conformance population issues no observation address")
	}
	if first == second {
		t.Fatal("two producing occurrences of one mount share an observation address")
	}
	branch, branchOK := ValueObservationAddress(compilation, structure.DiagnosticObservationBranchCondition, mount, identity.ContentID{21}, context)
	if !branchOK || branch != first {
		t.Fatal("one produced value at one occurrence is published on two columns")
	}
}
