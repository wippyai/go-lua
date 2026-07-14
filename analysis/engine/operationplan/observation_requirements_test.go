package operationplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestObservationRequirementsRejectUnknownAndWholeStateSelectors(t *testing.T) {
	for _, id := range []ProjectionID{"future.unknown.v1", "body.state.v1"} {
		if _, ok := makeObservationRequirement(id, 1, 0, observation.Occurrence{}, false); ok {
			t.Fatalf("unknown/whole-State selector %q admitted", id)
		}
	}
	if _, ok := makeObservationRequirement(ProjectionEdgeNormal, 1, 1, observation.Occurrence{}, false); ok {
		t.Fatal("degenerate edge selector admitted")
	}
}

func TestObservationRequirementsAreCanonicalImmutableAndSchemaBound(t *testing.T) {
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	lowered := wir.NewBody("requirements")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 1
	input := factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: {}}}
	first := New(graph, input).WithObservationIdentity(owner, lowered, graph)
	second := New(graph, input).WithObservationIdentity(owner, lowered, graph)
	a, aOK := first.ObservationRequirements()
	b, bOK := second.ObservationRequirements()
	if !aOK || !bOK || !a.Sealed() || a.SchemaID() == (ObservationSchemaID{}) || a.SchemaID() != b.SchemaID() || a.ConsumerInventoryID() != b.ConsumerInventoryID() {
		t.Fatal("requirement identity is unsealed, zero, or nondeterministic")
	}
	entries := a.Entries(true)
	if len(entries) == 0 {
		t.Fatal("sealed requirement inventory is empty")
	}
	oracle, oracleCount := observationRequirementInventoryID(a)
	if a.ConsumerInventoryID() != oracle || oracleCount != len(entries) {
		t.Fatalf("canonical inventory=%x/%d, cursor oracle=%x/%d", a.ConsumerInventoryID(), len(entries), oracle, oracleCount)
	}
	entries[0] = ObservationRequirement{}
	again := a.Entries(true)
	if len(again) == 0 || again[0].Projection() == "" {
		t.Fatal("caller mutated sealed requirement storage")
	}
	seen := make(map[observation.Occurrence]struct{})
	for _, requirement := range again {
		anchor, ok := requirement.Anchor()
		if !ok {
			continue
		}
		if _, duplicate := seen[anchor]; duplicate {
			t.Fatalf("duplicate anchor %v", anchor)
		}
		seen[anchor] = struct{}{}
	}
}

func TestObservationRequirementInventoryIgnoresFactInsertionPermutation(t *testing.T) {
	graph := cfg.New()
	first := graph.AddNode(cfg.NodeAssign)
	second := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), first, false)
	graph.AddEdge(first, second, false)
	graph.AddEdge(second, graph.Exit(), false)
	lowered := wir.NewBody("permuted-requirements")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 3
	leftFacts := make(map[cfg.Point]factflow.RootAssignment)
	leftFacts[first], leftFacts[second] = factflow.RootAssignment{}, factflow.RootAssignment{}
	rightFacts := make(map[cfg.Point]factflow.RootAssignment)
	rightFacts[second], rightFacts[first] = factflow.RootAssignment{}, factflow.RootAssignment{}
	left := New(graph, factflow.FactsInput{RootAssignments: leftFacts}).WithObservationIdentity(owner, lowered, graph)
	right := New(graph, factflow.FactsInput{RootAssignments: rightFacts}).WithObservationIdentity(owner, lowered, graph)
	a, aOK := left.ObservationRequirements()
	b, bOK := right.ObservationRequirements()
	if !aOK || !bOK || a.ConsumerInventoryID() != b.ConsumerInventoryID() || len(a.Entries(true)) != len(b.Entries(true)) {
		t.Fatalf("permuted inventory differs: %v/%v %x/%x", aOK, bOK, a.ConsumerInventoryID(), b.ConsumerInventoryID())
	}
}

func TestObservationRequirementCompactInventoryIgnoresTraversalEnumeration(t *testing.T) {
	reachable := []uint64{0b1111}
	selectors := []uint64{
		0,
		uint64(1) << selectorBoundaryRootAssignment,
		uint64(1) << selectorBoundaryCallOutcome,
		0,
	}
	argument := knownObservationRequirementSelector(
		selectorObservationCallArgument,
		2,
		0,
		observation.Occurrence{Point: wir.DebugPointID{Ordinal: 7, Phase: wir.DebugPhaseCall}, Kind: observation.CallArgument},
		false,
	)
	route := knownObservationRequirementSelector(
		selectorObservationCallInvocation,
		2,
		0,
		observation.Occurrence{Point: wir.DebugPointID{Ordinal: 7, Phase: wir.DebugPhaseCall}, Kind: observation.CallInvocation},
		false,
	)
	leftEdges := []uint64{uint64(2)<<32 | 3, uint64(0)<<32 | 1, uint64(1)<<32 | 2}
	rightEdges := []uint64{uint64(1)<<32 | 2, uint64(2)<<32 | 3, uint64(0)<<32 | 1}
	leftRecords := []observationRequirementKey{encodeObservationRequirementKey(route), encodeObservationRequirementKey(argument)}
	rightRecords := []observationRequirementKey{encodeObservationRequirementKey(argument), encodeObservationRequirementKey(route)}

	left, leftCount, leftOK := observationRequirementCompactInventoryID(
		defaultObservationProjectionSchemaID, 4, append([]uint64(nil), reachable...), append([]uint64(nil), selectors...), leftEdges, leftRecords,
	)
	right, rightCount, rightOK := observationRequirementCompactInventoryID(
		defaultObservationProjectionSchemaID, 4, append([]uint64(nil), reachable...), append([]uint64(nil), selectors...), rightEdges, rightRecords,
	)
	if !leftOK || !rightOK || left != right || leftCount != rightCount {
		t.Fatalf("enumeration changed compact identity: %v/%v %x/%x %d/%d", leftOK, rightOK, left, right, leftCount, rightCount)
	}

	trailing, trailingCount, trailingOK := observationRequirementCompactInventoryID(
		defaultObservationProjectionSchemaID,
		5,
		[]uint64{0b1111},
		append(append([]uint64(nil), selectors...), 0),
		[]uint64{uint64(0)<<32 | 1, uint64(1)<<32 | 2, uint64(2)<<32 | 3},
		[]observationRequirementKey{encodeObservationRequirementKey(argument), encodeObservationRequirementKey(route)},
	)
	if !trailingOK || trailing != left || trailingCount != leftCount {
		t.Fatalf("trailing unreachable storage changed inventory: %v %x/%x %d/%d", trailingOK, trailing, left, trailingCount, leftCount)
	}

	referenced, referencedCount, referencedOK := observationRequirementCompactInventoryID(
		defaultObservationProjectionSchemaID,
		5,
		[]uint64{0b11111},
		append(append([]uint64(nil), selectors...), 0),
		[]uint64{uint64(0)<<32 | 1, uint64(1)<<32 | 2, uint64(2)<<32 | 3, uint64(3)<<32 | 4},
		[]observationRequirementKey{encodeObservationRequirementKey(argument), encodeObservationRequirementKey(route)},
	)
	if !referencedOK || referenced == left || referencedCount == leftCount {
		t.Fatalf("new referenced point did not change inventory: %v %x/%x %d/%d", referencedOK, referenced, left, referencedCount, leftCount)
	}
}
