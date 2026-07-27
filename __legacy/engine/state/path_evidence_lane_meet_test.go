package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func TestPathEvidenceLaneRegistersExactMeet(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LanePathEvidence)
	if !ok {
		t.Fatal("path-evidence lane is not registered")
	}
	mustKey := func(path pathdom.PathKey) keyspace.Key {
		t.Helper()
		key, ok := keys.FromPathKey(path)
		if !ok {
			t.Fatalf("invalid path %q", path)
		}
		return key
	}
	shared := mustKey("sym1@1.shared")
	leftOnly := mustKey("sym2@1.left")
	rightOnly := mustKey("sym3@1.right")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	leftProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: shared, Other: leftOnly}
	rightProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: shared, Other: rightOnly}
	leftImplication := pathevidence.NewPathPresenceImplication(shared, presence.Present(), leftOnly, presence.Present())
	rightImplication := pathevidence.NewPathPresenceImplication(shared, presence.Present(), rightOnly, presence.Present())

	leftState := domain.Lattice().Bottom().
		WriteLocalPathKey(reg, shared, present).
		WriteLocalPathKey(reg, leftOnly, present).
		WriteLocalPathStaticMember(shared, present).
		AddBranchProof(leftProof).
		AddPathPresenceImplication(leftImplication)
	rightState := domain.Lattice().Bottom().
		WriteLocalPathKey(reg, shared, product.Top()).
		WriteLocalPathKey(reg, rightOnly, absent).
		WriteLocalPathStaticMember(shared, product.Top()).
		AddBranchProof(rightProof).
		AddPathPresenceImplication(rightImplication)
	left := mustOnlyLaneFactor(t, domain, leftState)
	right := mustOnlyLaneFactor(t, domain, rightState)

	met, err := domain.LaneMeet(left, right)
	if err != nil {
		t.Fatalf("registered Meet: %v", err)
	}
	got, err := domain.Compose([]LaneFactor{met})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[keyspace.Key]product.Value{
		shared:    product.Meet(reg, present, product.Top()),
		leftOnly:  present,
		rightOnly: absent,
	} {
		if value := got.ReadLocalPathKey(reg, path); !product.Equal(reg, value, want) {
			t.Fatalf("path %s Meet value is not exact", keys.Format(path))
		}
	}
	if member, ok := got.ReadLocalPathStaticMember(shared); !ok || !product.Equal(reg, member, present) {
		t.Fatal("static-member Meet is not exact")
	}
	for _, proof := range []pathevidence.BranchProof{leftProof, rightProof} {
		if !got.HasBranchProof(proof) {
			t.Fatalf("must proof missing after registered Meet: %+v", proof)
		}
	}
	for _, implication := range []pathevidence.PathPresenceImplication{leftImplication, rightImplication} {
		if !got.HasPathPresenceImplication(implication) {
			t.Fatalf("must implication missing after registered Meet: %+v", implication)
		}
	}

	top, err := domain.LaneTop(lane)
	if err != nil {
		t.Fatal(err)
	}
	topIdentity, err := domain.LaneMeet(top, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, topIdentity, left, "path-evidence top identity")
	bottom, err := domain.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	bottomAbsorbed, err := domain.LaneMeet(bottom, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, bottomAbsorbed, bottom, "path-evidence bottom absorption")
}
