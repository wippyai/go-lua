package state

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func TestValueRefinementPlanSealsExactDescendantPreservationCone(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	rootState := rootAssignmentTestStateKey(t, "sym901@1")
	childState := rootAssignmentTestStateKey(t, "sym901@1.child")
	grandState := rootAssignmentTestStateKey(t, "sym901@1.child.grand")
	unrelatedState := rootAssignmentTestStateKey(t, "sym902@1.other")
	root, _ := keys.InternStateKey(rootState)
	child, _ := keys.InternStateKey(childState)
	grand, _ := keys.InternStateKey(grandState)
	unrelated, _ := keys.InternStateKey(unrelatedState)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: child, Other: unrelated}
	implication := pathevidence.NewPathPresenceImplication(child, presence.Present(), unrelated, presence.Present())
	input := Reachable(State{}).
		WriteLocalPathKey(reg, root, present).
		WriteLocalPathKey(reg, child, present).
		WriteLocalPathKey(reg, grand, present).
		WriteLocalPathKey(reg, unrelated, present).
		WriteLocalPathStaticMember(child, present).
		WriteLocalPathStaticMember(grand, present).
		WriteLocalPathStaticMember(unrelated, present).
		AddBranchProof(proof).
		AddPathPresenceImplication(implication)
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		t.Fatal("path coordinate family")
	}
	factors, err := domain.DecomposeLanes(input, []ProductLane{family.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	_, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	union := make([]CoordinateSlot, len(scalars))
	for index := range scalars {
		union[index] = scalars[index].Slot()
	}
	inventory, err := domain.SealCoordinateFactorInventory(keys, union)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealValueRefinementPlan(keys, root, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ValidFor(domain) || plan.Target() != root || plan.Root() != root {
		t.Fatalf("value-refinement plan ownership: valid=%t target=%s want=%s root=%s want=%s",
			plan.ValidFor(domain), keys.FormatReadOnly(plan.Target()), keys.FormatReadOnly(root),
			keys.FormatReadOnly(plan.Root()), keys.FormatReadOnly(root))
	}
	rootValue, concrete := plan.RootValue().Concrete()
	if !concrete || rootValue != statekey.SymbolValue(901) {
		t.Fatalf("root Values binding=%d/%t", rootValue, concrete)
	}
	got := plan.Descendants()
	if len(got) != 4 {
		t.Fatalf("descendant preservation coordinates=%d, want 4", len(got))
	}
	kinds := map[ValueRefinementCoordinateKind]int{}
	for _, coordinate := range got {
		if coordinate.Path() == unrelated || coordinate.Path() == root {
			t.Fatalf("non-descendant coordinate admitted: %s", keys.FormatReadOnly(coordinate.Path()))
		}
		kinds[coordinate.Kind()]++
	}
	if kinds[ValueRefinementCoordinatePath] != 2 || kinds[ValueRefinementCoordinateStaticMember] != 2 {
		t.Fatalf("descendant kinds=%v", kinds)
	}
	proofSlot, err := domain.PathBranchProofCoordinateSlot(keys, proof)
	if err != nil {
		t.Fatal(err)
	}
	implicationSlot, err := domain.PresenceImplicationCoordinateSlot(keys, implication)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]CoordinateSlot{"proof": proofSlot, "implication": implicationSlot} {
		found := false
		for _, got := range plan.CoordinateWrites() {
			equal, equalErr := domain.CoordinateSlotEqual(got, want)
			found = found || equalErr == nil && equal
		}
		if !found {
			t.Fatalf("descendant mutation omitted %s coordinate", name)
		}
	}
}
