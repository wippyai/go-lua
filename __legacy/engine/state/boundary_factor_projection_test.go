package state

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestExactCoordinateBoundarySelectionDoesNotReadmitSharedEndpoint(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	anchor := keys.FromPath(pathdom.Path{Symbol: symbol.ID(701), Version: 1})
	selectedOther := keys.FromPath(pathdom.Path{Symbol: symbol.ID(702), Version: 1})
	rejectedOther := keys.FromPath(pathdom.Path{Symbol: symbol.ID(703), Version: 1})
	selectedProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: anchor, Other: selectedOther}
	rejectedProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: anchor, Other: rejectedOther}
	source := Domain(reg).Bottom().AddBranchProof(selectedProof).AddBranchProof(rejectedProof)
	selectedSlot, err := domain.PathBranchProofCoordinateSlot(keys, selectedProof)
	if err != nil {
		t.Fatal(err)
	}
	rejectedSlot, err := domain.PathBranchProofCoordinateSlot(keys, rejectedProof)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SealBoundaryFactorSelection(keys, []BoundaryFactorRoot{{Path: anchor}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	selection, err = domain.selectBoundaryFactorCoordinates(selection, []CoordinateSlot{selectedSlot})
	if err != nil {
		t.Fatal(err)
	}
	exact, err := domain.SealCoordinateFactorInventory(keys, []CoordinateSlot{selectedSlot})
	if err != nil {
		t.Fatal(err)
	}
	selection.coordinates, selection.exactCoordinates = exact, true
	lanes := domain.NonValuesLaneInventory()
	factors, err := domain.DecomposeLanes(source, lanes)
	if err != nil {
		t.Fatal(err)
	}
	var projected LaneFactor
	foundLane := false
	for index, lane := range lanes {
		families, familyErr := domain.CoordinateFamilies(lane)
		if familyErr != nil {
			t.Fatal(familyErr)
		}
		for _, family := range families {
			if family == selectedSlot.Family() {
				projected, err = domain.ProjectBoundaryFactor(selection, factors[index])
				if err != nil {
					t.Fatal(err)
				}
				foundLane = true
			}
		}
	}
	if !foundLane {
		t.Fatal("path-evidence lane not found")
	}
	_, scalars, err := domain.DecomposeCoordinateFamily(projected, selectedSlot.Family(), keys)
	if err != nil {
		t.Fatal(err)
	}
	foundSelected, foundRejected := false, false
	for _, scalar := range scalars {
		if equal, _ := domain.CoordinateSlotEqual(scalar.Slot(), selectedSlot); equal {
			foundSelected = true
		}
		if equal, _ := domain.CoordinateSlotEqual(scalar.Slot(), rejectedSlot); equal {
			foundRejected = true
		}
	}
	if !foundSelected || foundRejected {
		t.Fatalf("exact projection selected=%t rejected=%t", foundSelected, foundRejected)
	}
}

func TestProjectBoundaryFactorMatchesCanonicalProjectionForEveryNonValuesLane(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	fixture := stateLawFixtureFor(reg, keys)
	source := domain.Lattice().Bottom()
	for _, sample := range stateLawLaneSamples(reg, keys) {
		source = domain.Lattice().Join(source, sample.state)
	}
	source = domain.Normalize(source)
	rootPath, ok := keys.FromStateKey(fixture.pathKey)
	if !ok {
		t.Fatal("fixture root path is outside keyspace")
	}
	roots := BoundaryRoots{{
		Slot: fixture.valueSlot, Path: rootPath,
		Value: source.ReadValue(reg, fixture.valueSlot),
	}}
	artifact, err := ProjectBoundary(reg, keys, source, roots)
	if err != nil {
		t.Fatal(err)
	}
	selection := BoundaryFactorSelection{
		seal: &boundaryFactorSelectionSeal{}, keys: keys,
		closure: cloneBoundaryFactorClosure(artifact.closure),
		roots:   []BoundaryFactorRoot{{Slot: roots[0].Slot, Path: roots[0].Path}},
	}
	lanes := domain.NonValuesLaneInventory()
	sourceFactors, err := domain.DecomposeLanes(source, lanes)
	if err != nil {
		t.Fatal(err)
	}
	wantFactors, err := domain.DecomposeLanes(artifact.world, lanes)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceFactors) != len(lanes) || len(wantFactors) != len(lanes) {
		t.Fatalf("factor inventory = %d/%d, want %d", len(sourceFactors), len(wantFactors), len(lanes))
	}
	for index, lane := range lanes {
		before := sourceFactors[index]
		got, projectErr := domain.ProjectBoundaryFactor(selection, before)
		if projectErr != nil {
			t.Fatalf("project lane %q: %v", lane.ID(), projectErr)
		}
		if got.Lane() != lane || wantFactors[index].Lane() != lane {
			t.Fatalf("project lane identity %q = %q/%q", lane.ID(), got.Lane().ID(), wantFactors[index].Lane().ID())
		}
		equal, equalErr := domain.LaneEqual(got, wantFactors[index])
		if equalErr != nil || !equal {
			t.Fatalf("project lane %q differs from canonical ProjectBoundary: equal=%v err=%v", lane.ID(), equal, equalErr)
		}
		unchanged, unchangedErr := domain.LaneCanonicalRepresentationEqual(before, sourceFactors[index])
		if unchangedErr != nil || !unchanged {
			t.Fatalf("project lane %q mutated its source: equal=%v err=%v", lane.ID(), unchanged, unchangedErr)
		}
	}
}

func TestBoundaryFactorViewSealsOrderedRootSchema(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	firstPath, ok := keys.InternStateKey(rootAssignmentTestStateKey(t, "sym901@1.first"))
	if !ok {
		t.Fatal("first path not interned")
	}
	secondPath, ok := keys.InternStateKey(rootAssignmentTestStateKey(t, "sym902@1.second"))
	if !ok {
		t.Fatal("second path not interned")
	}
	want := []BoundaryFactorRoot{{Path: firstPath}, {Path: secondPath}}
	selection, err := SealBoundaryFactorSelection(keys, want, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	view, err := domain.PrepareBoundaryFactorView(selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := view.RootSchemas()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root schema = %#v, want %#v", got, want)
	}
	got[0] = BoundaryFactorRoot{}
	if again := view.RootSchemas(); !reflect.DeepEqual(again, want) {
		t.Fatalf("root schema leaked mutable storage: %#v", again)
	}
}
