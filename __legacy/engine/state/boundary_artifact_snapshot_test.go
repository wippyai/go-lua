package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBoundaryArtifactProjectedWorldRetainsEveryEnabledLane(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := Domain(reg)
	samples := stateLawLaneSamples(reg, keys)
	if len(samples) != len(DefaultLanes()) {
		t.Fatalf("state-law inventory = %d, enabled lanes = %d", len(samples), len(DefaultLanes()))
	}
	roots := boundaryArtifactCompleteLaneRoots(t, keys)
	for _, sample := range samples {
		artifact, err := ProjectBoundary(reg, keys, sample.state, roots)
		if err != nil {
			t.Fatalf("project lane %q: %v", sample.lane, err)
		}
		projected, _, err := artifact.ProjectedWorld(reg, keys)
		if err != nil {
			t.Fatalf("read lane %q: %v", sample.lane, err)
		}
		laneDomain := DomainWithLaneSet(reg, NewLaneSet(sample.lane))
		if !laneDomain.Equal(projected, NormalizeForDomain(laneDomain, sample.state)) {
			t.Fatalf("final boundary projection changed lane %q", sample.lane)
		}
	}

	joined := Reachable(State{})
	for _, sample := range samples {
		joined = domain.Join(joined, sample.state)
	}
	artifact, err := ProjectBoundary(reg, keys, joined, roots)
	if err != nil {
		t.Fatal(err)
	}
	projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	if projected.laneMask != joined.laneMask {
		t.Fatalf("projected lane mask = %v, want %v", projected.laneMask, joined.laneMask)
	}
	for _, spec := range defaultLaneCatalog.specs {
		if !projected.laneMask.allows(spec.bit) {
			t.Fatalf("enabled lane %q missing from final projection", spec.id)
		}
		if !spec.boundary.equal(reg, projected, artifact.world) {
			t.Fatalf("enabled lane %q changed while exposing final projection", spec.id)
		}
	}
	if len(projectedRoots) != len(roots) {
		t.Fatalf("projected roots = %d, want %d", len(projectedRoots), len(roots))
	}
}

func TestBoundaryArtifactProjectedWorldIsDetachedAndAuthorityChecked(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	slot := key.SymbolValue(991)
	value := product.Top()
	artifact, err := ProjectBoundary(reg, keys, Domain(reg).Bottom().WriteValue(reg, slot, value), BoundaryRoots{{Slot: slot, Value: value}})
	if err != nil {
		t.Fatal(err)
	}

	world, roots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	world = world.WriteValue(reg, slot, product.Bottom(reg))
	roots[0] = BoundaryRoot{}

	again, againRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	if !product.Equal(reg, again.ReadValue(reg, slot), value) {
		t.Fatal("caller State update mutated boundary artifact")
	}
	if len(againRoots) != 1 || againRoots[0].Slot != slot || !product.Equal(reg, againRoots[0].Value, value) {
		t.Fatal("caller root update mutated boundary artifact")
	}
	if !product.Equal(reg, world.ReadValue(reg, slot), product.Bottom(reg)) {
		t.Fatal("detached State update was not observable by its owner")
	}

	foreignReg, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	if got, gotRoots, err := artifact.ProjectedWorld(foreignReg, keys); err == nil || got.laneMask != (laneMask{}) || gotRoots != nil {
		t.Fatalf("foreign registry published projection: %#v/%#v/%v", got, gotRoots, err)
	}
	if got, gotRoots, err := artifact.ProjectedWorld(reg, keyspace.New()); err == nil || got.laneMask != (laneMask{}) || gotRoots != nil {
		t.Fatalf("foreign keyspace published projection: %#v/%#v/%v", got, gotRoots, err)
	}
}

func boundaryArtifactCompleteLaneRoots(t *testing.T, keys *keyspace.KeySpace) BoundaryRoots {
	t.Helper()
	paths := []pathdom.PathKey{
		"sym201@1.field",
		"sym201@1.shared",
		"sym201@1.table",
		"state-law-source",
		"state-law-target",
	}
	roots := BoundaryRoots{
		{Slot: key.SymbolValue(201), Value: product.Top()},
		{Slot: key.ReturnSlot(3), Value: product.Top()},
		{Value: product.Top()},
	}
	for _, path := range paths {
		projected, ok := keys.FromStateKey(path)
		if !ok {
			t.Fatalf("invalid boundary path %q", path)
		}
		roots = append(roots, BoundaryRoot{Path: projected, Value: product.Top()})
	}
	return roots
}
