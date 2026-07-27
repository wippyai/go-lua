package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestEveryDecomposedCoordinateHasRegisteredScalarSupport(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	left := pathaddr.StateKey("sym9801@1.left")
	right := pathaddr.StateKey("sym9802@1.right")
	leftPath := keys.FromPath(pathdom.NewPath(symbol.ID(9801), "left"))
	id := identity.ID{Kind: "table", Site: t.Name(), Index: 1}

	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	state := Reachable(domain.Lattice().Bottom()).
		WriteLocalPathKey(reg, leftPath, value).
		WriteLenFloor(keys, left, 2).
		WriteNumFloor(keys, left, 1).
		WriteNumCeil(keys, left, 8).
		WriteDiffConstraint(RelValueOperand(left), RelValueOperand(right), 3).
		WritePlacement(id, placement.Stack).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: value}))

	lanes := domain.LaneInventory()
	factors, err := domain.DecomposeLanes(state, lanes)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[CoordinateFamilyID]int)
	for laneIndex, lane := range lanes {
		families, familyErr := domain.CoordinateFamilies(lane)
		if familyErr != nil {
			t.Fatal(familyErr)
		}
		for _, family := range families {
			skeleton, scalars, decomposeErr := domain.DecomposeCoordinateFamily(factors[laneIndex], family, keys)
			if decomposeErr != nil {
				t.Fatalf("decompose %q: %v", family.ID(), decomposeErr)
			}
			coordinate, coordinateErr := domain.validateCoordinateSkeleton(skeleton)
			if coordinateErr != nil {
				t.Fatalf("runtime %q: %v", family.ID(), coordinateErr)
			}
			contains := func(inventory []coordinateKeyPayload, key coordinateKeyPayload) bool {
				for _, candidate := range inventory {
					if coordinate.ops.keyEqual(candidate, key) {
						return true
					}
				}
				return false
			}
			explicit := make([]coordinateKeyPayload, len(scalars))
			for index := range scalars {
				explicit[index] = scalars[index].slot.key
			}
			required := coordinate.ops.requiredScalarKeys(skeleton.payload)
			for _, key := range required {
				if coordinate.ops.scalarSupport(skeleton.payload, key) != CoordinateScalarRequired || !contains(explicit, key) {
					t.Fatalf("family %q required inventory is not explicit", family.ID())
				}
			}
			for _, scalar := range scalars {
				if coordinate.ops.scalarSupport(skeleton.payload, scalar.slot.key) == CoordinateScalarRequired && !contains(required, scalar.slot.key) {
					t.Fatalf("family %q omitted required key from inventory", family.ID())
				}
			}
			sealed, post, ok := coordinate.ops.sealSkeletonInventory(skeleton.payload, explicit, keys)
			if !ok || len(post) != 0 || !coordinate.ops.skeletonEqual(skeleton.payload, sealed) {
				t.Fatalf("family %q complete inventory is not an identity", family.ID())
			}
			reversed := append([]coordinateKeyPayload(nil), explicit...)
			for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
				reversed[left], reversed[right] = reversed[right], reversed[left]
			}
			reordered, reorderedPost, ok := coordinate.ops.sealSkeletonInventory(skeleton.payload, reversed, keys)
			if !ok || len(reorderedPost) != 0 || !coordinate.ops.skeletonEqual(sealed, reordered) {
				t.Fatalf("family %q inventory seal depends on input order", family.ID())
			}
			sealedAgain, postAgain, ok := coordinate.ops.sealSkeletonInventory(sealed, explicit, keys)
			if !ok || len(postAgain) != 0 || !coordinate.ops.skeletonEqual(sealed, sealedAgain) {
				t.Fatalf("family %q inventory seal is not idempotent", family.ID())
			}
			empty, emptyPost, ok := coordinate.ops.sealSkeletonInventory(skeleton.payload, nil, keys)
			if !ok {
				t.Fatalf("family %q rejected empty admitted inventory", family.ID())
			}
			emptyRequired := coordinate.ops.requiredScalarKeys(empty)
			for _, key := range emptyRequired {
				foundPost := false
				for _, entry := range emptyPost {
					if coordinate.ops.keyEqual(entry.key, key) {
						foundPost = true
						break
					}
				}
				if !foundPost {
					t.Fatalf("family %q retained Required key outside admitted inventory without a conservative witness", family.ID())
				}
			}
			for _, scalar := range scalars {
				support, supportErr := domain.CoordinateScalarSupport(skeleton, scalar.Slot())
				if supportErr != nil || support == CoordinateScalarForbidden {
					t.Fatalf("decomposed %q scalar support=%v err=%v", family.ID(), support, supportErr)
				}
				if (family.ID() == heapCoordinateFamilyID || family.ID() == lenFloorCoordinateFamilyID) != (support == CoordinateScalarRequired) {
					t.Fatalf("decomposed %q scalar support=%v", family.ID(), support)
				}
				seen[family.ID()]++
			}
		}
	}
	for _, family := range []CoordinateFamilyID{
		heapCoordinateFamilyID, lenFloorCoordinateFamilyID, placementCoordinateFamilyID,
		numFloorCoordinateFamilyID, numCeilCoordinateFamilyID,
		diffRelationCoordinateFamilyID, pathEvidenceCoordinateFamilyID,
	} {
		if seen[family] == 0 {
			t.Fatalf("registered family %q produced no representative coordinate", family)
		}
	}
}
