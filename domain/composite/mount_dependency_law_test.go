package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The mount phase seals one axis's authority over another's, so the order it
// walks is the order the declared edges admit rather than the catalog's. The
// laws below state that order and the input scoping that makes an undeclared
// edge unusable rather than merely undocumented.

// TestMountPhaseWalksAxesInDependencyOrder states that every axis is mounted
// after the axes it declared an edge to, so a mount that seals over a peer's
// authority finds that authority already sealed.
func TestMountPhaseWalksAxesInDependencyOrder(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		t.Fatal("declaration table did not seal")
	}
	order, ok := axis.DependencyOrder(state.axes)
	if !ok || len(order) != len(state.axes) {
		t.Fatalf("mount order rejected the sealed inventory: ok=%v placed=%d", ok, len(order))
	}
	positions := make(map[schema.Key]int, len(order))
	for index, entry := range order {
		positions[entry.Key()] = index
	}
	edges := 0
	for _, entry := range state.axes {
		for index := 0; index < entry.DependencyCount(); index++ {
			dependency, dependencyOK := entry.DependencyAt(index)
			if !dependencyOK {
				t.Fatalf("axis %q publishes an unreadable dependency edge", entry.Key())
			}
			position, declared := positions[dependency]
			if !declared {
				t.Fatalf("axis %q depends on %q, which the mount order does not place", entry.Key(), dependency)
			}
			if position >= positions[entry.Key()] {
				t.Fatalf("axis %q is mounted before its dependency %q", entry.Key(), dependency)
			}
			edges++
		}
	}
	if edges == 0 {
		t.Fatalf("no axis declares a dependency edge; the order law measures nothing")
	}
}

// TestMountScopeCarriesNoUndeclaredAuthority states the input scoping every
// mount receives: the phase's neutral half is the Link, its artifact view, and
// the static inventory, and nothing else. An axis therefore reads a peer's
// authority only through a declared edge, and one it did not declare reads as
// absent and is rejected by that axis's own seal.
func TestMountScopeCarriesNoUndeclaredAuthority(t *testing.T) {
	inputs := LinkInputs{
		Artifacts:       []programmount.MountedArtifact{{}},
		StaticAuthority: &staticdomain.Authority{},
		ValueSchema:     &valuedomain.Schema{},
		PackSchema:      &packdomain.Schema{},
		CallAlgebra:     &calldomain.Algebra{},
		EffectAlgebra:   &effectfactor.Algebra{},
		topology:        &heapindex.Topology{},
		activation:      &callactivation.TargetBatchCatalog{},
	}
	neutral := inputs.neutral()
	if neutral.ValueSchema != nil || neutral.PlacementSchema.Valid() || neutral.PackSchema != nil || neutral.CallAlgebra != nil || neutral.EffectAlgebra != nil {
		t.Fatalf("the phase's neutral input half carried a mounted factor authority")
	}
	if neutral.HeapSchema.Valid() || neutral.topology != nil || neutral.activation != nil {
		t.Fatalf("the phase's neutral input half carried a derived authority")
	}
	if neutral.StaticAuthority == nil || len(neutral.Artifacts) != len(inputs.Artifacts) {
		t.Fatalf("the phase's neutral input half dropped an input every mount reads")
	}
}

// TestMountScopeCarriesDeclaredDependencyClosure states that a mount receives
// the sealed transitive prerequisites of each dependency it declares. The
// evidence axis names Placement only, yet Placement's own Heap prerequisite is
// available for the Placement adopter's owner-fence check. An unrelated axis
// remains absent from the scoped record.
func TestMountScopeCarriesDeclaredDependencyClosure(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		t.Fatal("declaration table did not seal")
	}
	evidence, evidenceOK := axisForKey(state, axisKeyPlacementEvidence)
	if !evidenceOK || evidence == nil {
		t.Fatal("evidence axis was not declared")
	}
	sealed := newAxisCells(state.axes)
	for _, key := range []schema.Key{axisKeyHeap, axisKeyPlacement, axisKeyValue} {
		slot, slotOK := axisSlotForKey(state, key)
		if !slotOK {
			t.Fatalf("axis %q was not assigned a slot", key)
		}
		sealed[slot] = axis.NewCell(key)
	}
	scoped := dependencyCells(state, evidence, sealed)
	heapSlot, heapOK := axisSlotForKey(state, axisKeyHeap)
	placementSlot, placementOK := axisSlotForKey(state, axisKeyPlacement)
	valueSlot, valueOK := axisSlotForKey(state, axisKeyValue)
	if !heapOK || !placementOK || !valueOK {
		t.Fatal("dependency closure fixture axes were not assigned slots")
	}
	if !scoped[heapSlot].Available() || !scoped[placementSlot].Available() {
		t.Fatal("Placement dependency did not carry its Heap prerequisite")
	}
	if scoped[valueSlot].Available() {
		t.Fatal("undeclared unrelated axis crossed the mount scope")
	}
}

// TestMountPhaseRejectsAnAbsentStaticInventory states that the static authority
// is part of the phase's own admission: it is owned by no axis, so no axis can
// seal it, and a record without it is rejected before any mount opens.
func TestMountPhaseRejectsAnAbsentStaticInventory(t *testing.T) {
	inputs := LinkInputs{Artifacts: []programmount.MountedArtifact{{}}}
	if inputs.mountable() {
		t.Fatalf("the mount phase admitted a record with no static inventory")
	}
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	mounted, failure := MountLink(compilation, inputs)
	if !failure.Available() || failure.Stage != MountStageInput {
		t.Fatalf("mount phase admitted an incomplete neutral half: %v", failure)
	}
	if mounted.Source != nil {
		t.Fatalf("rejected mount phase published a partially mounted record")
	}
}
