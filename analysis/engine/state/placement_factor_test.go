package state

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPlacementFactorExactInverseAndAuthority(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	first := identity.ID{Kind: "table", Site: "placement-factor", Index: 1}
	second := identity.ID{Kind: "table", Site: "placement-factor", Index: 2}
	value := domain.Lattice().Bottom().
		WritePlacement(first, placement.Stack).
		WritePlacement(second, placement.SharedHeap)
	factor := onlyPlacementLaneFactor(t, domain, value)
	skeleton, coordinates, err := domain.DecomposePlacement(factor)
	if err != nil {
		t.Fatal(err)
	}
	if len(coordinates) != 2 || coordinates[0].Identity() != first || coordinates[1].Identity() != second {
		t.Fatalf("placement coordinates = %#v", coordinates)
	}
	recomposed, err := domain.ComposePlacement(skeleton, coordinates)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.LaneEqual(factor, recomposed)
	if err != nil || !equal {
		t.Fatalf("placement inverse equal=%t err=%v", equal, err)
	}
	if _, err := domain.ComposePlacement(skeleton, append(coordinates, coordinates[0])); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("duplicate coordinate error = %v", err)
	}
	if _, err := domain.BindPlacementValue(coordinates[0].Slot(), placement.Bottom); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("explicit Bottom error = %v", err)
	}
	peer := RegisteredProductDomain(reg)
	importedSkeleton, err := peer.ImportPlacementSkeleton(skeleton)
	if err != nil {
		t.Fatal(err)
	}
	imported := make([]PlacementFactor, 0, len(coordinates))
	for _, coordinate := range coordinates {
		slot, importErr := peer.ImportPlacementSlot(coordinate.Slot())
		if importErr != nil {
			t.Fatal(importErr)
		}
		factor, bindErr := peer.BindPlacementValue(slot, coordinate.Value())
		if bindErr != nil {
			t.Fatal(bindErr)
		}
		imported = append(imported, factor)
	}
	if _, err := peer.ComposePlacement(importedSkeleton, imported); err != nil {
		t.Fatal(err)
	}
}

func TestPlacementFactorPreservesFormalVocabularyCoordinates(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 0x51
	schema := identity.NewFormalSchemaID(owner, 7)
	input := identity.FormalTerm(identity.NewFormalVar(schema, formal.Input))
	output := identity.FormalTerm(identity.NewFormalVar(schema, formal.Output))

	skeleton, err := domain.PlacementSkeletonBottom()
	if err != nil {
		t.Fatal(err)
	}
	factors := make([]PlacementFactor, 0, 2)
	for _, term := range []identity.Term{input, output} {
		slot, slotErr := domain.placementTermSlot(term)
		if slotErr != nil {
			t.Fatal(slotErr)
		}
		factor, bindErr := domain.BindPlacementValue(slot, placement.Stack)
		if bindErr != nil {
			t.Fatal(bindErr)
		}
		factors = append(factors, factor)
	}
	composed, err := domain.ComposePlacement(skeleton, factors)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := domain.DecomposePlacement(composed)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].IdentityTerm() == got[1].IdentityTerm() {
		t.Fatalf("formal placement coordinates collapsed: %#v", got)
	}
	seen := map[identity.Term]bool{}
	for _, factor := range got {
		seen[factor.IdentityTerm()] = true
		if factor.Identity() != (identity.ID{}) {
			t.Fatalf("formal coordinate escaped as concrete identity: %#v", factor.Identity())
		}
	}
	if !seen[input] || !seen[output] {
		t.Fatalf("formal vocabulary coordinates not preserved: %#v", seen)
	}
	recomposed, err := domain.ComposePlacement(skeleton, got)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.LaneEqual(composed, recomposed)
	if err != nil || !equal {
		t.Fatalf("formal placement inverse equal=%t err=%v", equal, err)
	}
}

func TestPlacementFactorwiseLatticeDifferential(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	first := identity.ID{Kind: "table", Site: "placement-factor-law", Index: 1}
	second := identity.ID{Kind: "table", Site: "placement-factor-law", Index: 2}
	bottom := domain.Lattice().Bottom()
	samples := []State{
		bottom,
		bottom.WritePlacement(first, placement.Stack),
		bottom.WritePlacement(first, placement.SharedHeap).WritePlacement(second, placement.OwnedHeap),
		domain.Lattice().Top(),
	}
	type operation struct {
		name     string
		lane     func(LaneFactor, LaneFactor) (LaneFactor, error)
		skeleton func(PlacementSkeletonFactor, PlacementSkeletonFactor) (PlacementSkeletonFactor, error)
		scalar   func(placement.Value, placement.Value) placement.Value
	}
	operations := []operation{
		{name: "join", lane: domain.LaneJoin, skeleton: domain.PlacementSkeletonJoin, scalar: placement.Join},
		{name: "meet", lane: domain.LaneMeet, skeleton: domain.PlacementSkeletonMeet, scalar: placement.Meet},
		{name: "widen", lane: domain.LaneWiden, skeleton: domain.PlacementSkeletonWiden, scalar: placement.Widen},
	}
	for leftIndex, leftState := range samples {
		for rightIndex, rightState := range samples {
			left := onlyPlacementLaneFactor(t, domain, leftState)
			right := onlyPlacementLaneFactor(t, domain, rightState)
			leftSkeleton, leftCoordinates, err := domain.DecomposePlacement(left)
			if err != nil {
				t.Fatal(err)
			}
			rightSkeleton, rightCoordinates, err := domain.DecomposePlacement(right)
			if err != nil {
				t.Fatal(err)
			}
			for _, operation := range operations {
				want, err := operation.lane(left, right)
				if err != nil {
					t.Fatal(err)
				}
				gotSkeleton, err := operation.skeleton(leftSkeleton, rightSkeleton)
				if err != nil {
					t.Fatal(err)
				}
				got, err := factorwisePlacementOperation(domain, gotSkeleton, leftSkeleton, leftCoordinates, rightSkeleton, rightCoordinates, operation.scalar)
				if err != nil {
					t.Fatal(err)
				}
				equal, err := domain.LaneEqual(got, want)
				if err != nil || !equal {
					t.Fatalf("%s sample %d/%d equal=%t err=%v", operation.name, leftIndex, rightIndex, equal, err)
				}
			}
		}
	}
}

func factorwisePlacementOperation(
	domain ProductDomain,
	output PlacementSkeletonFactor,
	leftSkeleton PlacementSkeletonFactor,
	left []PlacementFactor,
	rightSkeleton PlacementSkeletonFactor,
	right []PlacementFactor,
	combine func(placement.Value, placement.Value) placement.Value,
) (LaneFactor, error) {
	outputDefault, err := domain.PlacementDefault(output)
	if err != nil {
		return LaneFactor{}, err
	}
	if outputDefault == placement.Unknown {
		return domain.ComposePlacement(output, nil)
	}
	leftDefault, err := domain.PlacementDefault(leftSkeleton)
	if err != nil {
		return LaneFactor{}, err
	}
	rightDefault, err := domain.PlacementDefault(rightSkeleton)
	if err != nil {
		return LaneFactor{}, err
	}
	leftValues := make(map[identity.ID]placement.Value, len(left))
	rightValues := make(map[identity.ID]placement.Value, len(right))
	ids := make(map[identity.ID]struct{}, len(left)+len(right))
	for _, factor := range left {
		leftValues[factor.Identity()] = factor.Value()
		ids[factor.Identity()] = struct{}{}
	}
	for _, factor := range right {
		rightValues[factor.Identity()] = factor.Value()
		ids[factor.Identity()] = struct{}{}
	}
	coordinates := make([]PlacementFactor, 0, len(ids))
	for id := range ids {
		leftValue, present := leftValues[id]
		if !present {
			leftValue = leftDefault
		}
		rightValue, present := rightValues[id]
		if !present {
			rightValue = rightDefault
		}
		value := combine(leftValue, rightValue)
		if value == outputDefault {
			continue
		}
		factor, bindErr := domain.BindPlacementValue(PlacementSlot{seal: domain.seal, lane: output.lane, id: identity.ConcreteTerm(id)}, value)
		if bindErr != nil {
			return LaneFactor{}, bindErr
		}
		coordinates = append(coordinates, factor)
	}
	return domain.ComposePlacement(output, coordinates)
}

func onlyPlacementLaneFactor(t *testing.T, domain ProductDomain, value State) LaneFactor {
	t.Helper()
	lane, ok := domain.ProductLane(LanePlacement)
	if !ok {
		t.Fatal("product has no placement lane")
	}
	factors, err := domain.DecomposeLanes(value, []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	if len(factors) != 1 {
		t.Fatalf("placement factor count = %d", len(factors))
	}
	return factors[0]
}
