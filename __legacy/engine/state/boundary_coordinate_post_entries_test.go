package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func boundaryAliasCoordinateTestPlan(
	t *testing.T,
	domain ProductDomain,
	from, to *keyspace.KeySpace,
	bindings BoundaryRootMap,
) BoundaryFactorTransportPlan {
	t.Helper()
	formal := from.FromPath(pathdom.Path{Root: "formal"})
	selection, err := SealBoundaryFactorSelection(from, []BoundaryFactorRoot{{Path: formal}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(lexicalidentity.RootBody(namespace)), nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, bindings, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	companionLane, ok := domain.BoundaryClosureCompanion()
	if !ok {
		t.Fatal("registered domain has no boundary closure companion")
	}
	companion, err := domain.LaneBottom(companionLane)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := domain.ProjectBoundaryClosureCompanion(selection, &companion)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, projection)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func pathEvidenceCoordinateTestFamily(t *testing.T, domain ProductDomain) CoordinateFamily {
	t.Helper()
	lane, ok := domain.ProductLane(LanePathEvidence)
	if !ok {
		t.Fatal("registered domain has no PathEvidence lane")
	}
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		t.Fatalf("PathEvidence coordinate families = %d, err=%v", len(families), err)
	}
	return families[0]
}

func boundaryLiftContainsCoordinateSlot(t *testing.T, domain ProductDomain, lift CoordinateBoundaryFamilyLift, want CoordinateSlot) bool {
	t.Helper()
	for index := 0; index < lift.OutputCount(); index++ {
		got, ok := lift.OutputSlot(index)
		if !ok {
			t.Fatalf("coordinate output %d has no slot", index)
		}
		equal, err := domain.CoordinateSlotEqual(got, want)
		if err != nil {
			t.Fatal(err)
		}
		if equal {
			return true
		}
	}
	return false
}

func TestBoundaryCoordinatePostEntriesAreCanonicalAndSupportFiltered(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	left := to.FromPath(pathdom.Path{Root: "left"})
	right := to.FromPath(pathdom.Path{Root: "right"})
	bindings := BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: left},
		{FromRoot: 0, ToRoot: 1, To: right},
	}
	first := boundaryAliasCoordinateTestPlan(t, domain, from, to, bindings)
	second := boundaryAliasCoordinateTestPlan(t, domain, from, to, BoundaryRootMap{bindings[1], bindings[0]})
	if len(first.aliases) != 1 || len(second.aliases) != 1 || first.aliases[0] != second.aliases[0] {
		t.Fatalf("canonical aliases differ: first=%v second=%v", first.aliases, second.aliases)
	}

	family := pathEvidenceCoordinateTestFamily(t, domain)
	want, err := domain.PathBranchProofCoordinateSlot(to, pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathEqual, Path: first.aliases[0][0], Other: first.aliases[0][1],
	})
	if err != nil {
		t.Fatal(err)
	}
	destinationSkeleton, err := domain.CoordinateSkeletonTop(family, to)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := domain.SealCoordinateFamilyShape(destinationSkeleton, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		top  bool
		want bool
	}{
		{name: "supported", top: true, want: true},
		{name: "proofs-bottom", top: false, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var sourceSkeleton CoordinateFamilySkeleton
			if test.top {
				sourceSkeleton, err = domain.CoordinateSkeletonTop(family, from)
			} else {
				sourceSkeleton, err = domain.CoordinateSkeletonBottom(family, from)
			}
			if err != nil {
				t.Fatal(err)
			}
			source, shapeErr := domain.SealCoordinateFamilyShape(sourceSkeleton, nil)
			if shapeErr != nil {
				t.Fatal(shapeErr)
			}
			lift, liftErr := first.PrepareCoordinateBoundaryFamilyLift(source, destination, false)
			if liftErr != nil {
				t.Fatal(liftErr)
			}
			if got := boundaryLiftContainsCoordinateSlot(t, domain, lift, want); got != test.want {
				t.Fatalf("alias equality present=%t, want %t", got, test.want)
			}
		})
	}
}

func TestBoundaryCoordinatePostEntriesMatchRuntimePathEvidence(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	aliases := [][2]keyspace.Key{
		{keys.FromPath(pathdom.Path{Root: "a"}), keys.FromPath(pathdom.Path{Root: "b"})},
		{keys.FromPath(pathdom.Path{Root: "c"}), keys.FromPath(pathdom.Path{Root: "d"})},
	}
	entries := pathevidence.BoundaryAliasCoordinateEntries(aliases)
	got, ok := pathevidence.ComposeCoordinates(pathevidence.CoordinateTop(), entries, reg, keys)
	if !ok {
		t.Fatal("static alias candidates did not compose")
	}
	want, ok := postRebasePathEvidenceBoundaryFactor(nil, aliases, pathevidence.Domain(reg).Top())
	if !ok || !pathevidence.Domain(reg).Equal(got, want) {
		t.Fatal("static post-incidence relation differs from runtime PathEvidence semantics")
	}
}
