package pathevidence

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestPathEvidenceExactMeetLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	left, right, _ := coordinateTestLanes(t, reg, keys)
	domain := Domain(reg)

	latticelaws.LawSuite[Lane]{
		Name:   "state.pathevidence",
		Domain: domain,
		Sample: []Lane{
			domain.Bottom(),
			domain.Top(),
			left,
			right,
			domain.Join(left, right),
			domain.Meet(left, right),
		},
		WideningBound: 8,
	}.Run(t)
}

func TestPathEvidenceMeetUnionsMustFactsAndRebuildsEqualityIndex(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	shared := mustStructKey(t, keys, pathdom.PathKey("sym1@1.shared"))
	leftPeer := mustStructKey(t, keys, pathdom.PathKey("sym2@1.left"))
	rightPeer := mustStructKey(t, keys, pathdom.PathKey("sym3@1.right"))
	leftValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	rightValue := product.Top()
	leftProof := BranchProof{Kind: BranchProofPathEqual, Path: shared, Other: leftPeer}
	rightProof := BranchProof{Kind: BranchProofPathEqual, Path: shared, Other: rightPeer}
	leftImplication := NewPathPresenceImplication(shared, presence.Present(), leftPeer, presence.Present())
	rightImplication := NewPathPresenceImplication(shared, presence.Present(), rightPeer, presence.Present())

	left, _ := (Lane{}).WritePathKey(reg, shared, leftValue)
	left, _ = left.WritePathStaticMember(shared, leftValue)
	left, _ = left.AddBranchProof(leftProof)
	left, _ = left.AddPathPresenceImplication(leftImplication)
	right, _ := (Lane{}).WritePathKey(reg, shared, rightValue)
	right, _ = right.WritePathStaticMember(shared, rightValue)
	right, _ = right.AddBranchProof(rightProof)
	right, _ = right.AddPathPresenceImplication(rightImplication)

	met := Domain(reg).Meet(left, right)
	wantValue := product.Meet(reg, leftValue, rightValue)
	if got := met.ReadPathKey(reg, shared); !product.Equal(reg, got, wantValue) {
		t.Fatal("refinement Meet did not use the exact product GLB")
	}
	if got, ok := met.ReadPathStaticMember(shared); !ok || !product.Equal(reg, got, wantValue) {
		t.Fatal("static-member Meet did not use the exact product GLB")
	}
	for _, proof := range []BranchProof{leftProof, rightProof} {
		if !met.HasBranchProof(proof) {
			t.Fatalf("must proof missing after Meet: %+v", proof)
		}
	}
	for _, implication := range []PathPresenceImplication{leftImplication, rightImplication} {
		if !met.HasPathPresenceImplication(implication) {
			t.Fatalf("must implication missing after Meet: %+v", implication)
		}
	}
	for _, peer := range []keyspace.Key{leftPeer, rightPeer} {
		if !met.HasEquivalentKeyspaceKey(keys, shared, peer) {
			t.Fatalf("rebuilt equality index omitted met proof peer %s", keys.Format(peer))
		}
	}
}
