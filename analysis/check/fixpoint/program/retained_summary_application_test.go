package program

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type retainedSummaryTestResource struct{ releases int }

func (r *retainedSummaryTestResource) Release() { r.releases++ }

func retainedSummaryTestRead(value product.Value) trackedSummaryRead {
	return trackedSummaryRead{present: true, sum: summary.Summary{Returns: []product.Value{value}}}
}

func retainedSummaryTestSnapshot(reg *axis.Registry, value product.Value, key summary.SummaryKey) summary.Snapshot {
	return summary.NewSnapshot(reg, summary.EntrySummary{
		Key: key, Summary: summary.Summary{Returns: []product.Value{value}},
	})
}

func TestRetainedSummaryApplicationPolicyLifecycle(t *testing.T) {
	reg := standard.Registry()
	key := retainedSummaryApplicationKey{body: 1, input: 2, profile: "typed", resolution: 3}
	dep := dependencyTestKey(201)
	stringValue := typevalue.FromType(reg, typ.String)
	numberValue := typevalue.FromType(reg, typ.Number)
	stringReader := retainedSummaryTestSnapshot(reg, stringValue, dep)
	numberReader := retainedSummaryTestSnapshot(reg, numberValue, dep)
	owner := newRetainedSummaryApplicationOwner(reg)

	ordinary := owner.begin(key, stringReader)
	if got := ordinary.Decision(); got.kind != retainedSummaryApplyOrdinary || got.dropped {
		t.Fatalf("first decision = %+v, want ordinary", got)
	}
	if !ordinary.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(stringValue)}, pointSummaryDependencies{}, nil) {
		t.Fatal("ordinary publication rejected")
	}

	build := owner.begin(key, numberReader)
	if got := build.Decision(); got.kind != retainedSummaryApplyBuild || !slices.Equal(got.changed, []summary.SummaryKey{dep}) {
		t.Fatalf("first changed decision = %+v, want retained build for dep", got)
	}
	resource := &retainedSummaryTestResource{}
	buildPoints := pointSummaryDependencies{
		byPoint: map[cfg.Point]map[summary.SummaryKey]pointSummaryRead{
			7: {dep: {present: true, digest: 1}},
		},
	}
	if !build.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(numberValue)}, buildPoints, resource) {
		t.Fatal("retained publication rejected")
	}

	regional := owner.begin(key, stringReader)
	if got := regional.Decision(); got.kind != retainedSummaryApplyRegional || !slices.Equal(got.points, []cfg.Point{7}) || got.forceFull {
		t.Fatalf("later changed decision = %+v, want regional point 7", got)
	}
	update := pointSummaryDependencies{
		byPoint: map[cfg.Point]map[summary.SummaryKey]pointSummaryRead{
			9: {dep: {present: true, digest: 2}},
		},
		visited: map[cfg.Point]struct{}{7: {}, 9: {}},
	}
	if !regional.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(stringValue)}, update, nil) {
		t.Fatal("regional publication rejected")
	}
	if _, ok := owner.published.points.byPoint[7]; ok {
		t.Fatal("regional tombstone retained old point")
	}
	if _, ok := owner.published.points.byPoint[9]; !ok {
		t.Fatal("regional update did not publish new point")
	}

	reuse := owner.begin(key, stringReader).Decision()
	if reuse.kind != retainedSummaryApplyReuse || len(reuse.changed) != 0 {
		t.Fatalf("unchanged decision = %+v, want reuse", reuse)
	}
	owner.Release()
	owner.Release()
	if resource.releases != 1 {
		t.Fatalf("resource releases = %d, want exactly 1", resource.releases)
	}
}

func TestRetainedSummaryApplicationPreFlowForcesFull(t *testing.T) {
	reg := standard.Registry()
	key := retainedSummaryApplicationKey{body: 1}
	dep := dependencyTestKey(202)
	stringValue := typevalue.FromType(reg, typ.String)
	numberValue := typevalue.FromType(reg, typ.Number)
	owner := newRetainedSummaryApplicationOwner(reg)

	first := owner.begin(key, retainedSummaryTestSnapshot(reg, stringValue, dep))
	_ = first.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(stringValue)}, pointSummaryDependencies{}, nil)
	build := owner.begin(key, retainedSummaryTestSnapshot(reg, numberValue, dep))
	_ = build.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(numberValue)}, pointSummaryDependencies{
		preFlow: map[summary.SummaryKey]pointSummaryRead{dep: {present: true, digest: 1}},
	}, &retainedSummaryTestResource{})

	got := owner.begin(key, retainedSummaryTestSnapshot(reg, stringValue, dep)).Decision()
	if got.kind != retainedSummaryApplyRegional || !got.forceFull || len(got.points) != 0 {
		t.Fatalf("pre-flow decision = %+v, want full retained fallback", got)
	}
	owner.Release()
}

func TestRetainedSummaryApplicationProjectionOnlyReprojects(t *testing.T) {
	reg := standard.Registry()
	key := retainedSummaryApplicationKey{body: 1}
	dep := dependencyTestKey(203)
	stringValue := typevalue.FromType(reg, typ.String)
	numberValue := typevalue.FromType(reg, typ.Number)
	owner := newRetainedSummaryApplicationOwner(reg)

	ordinary := owner.begin(key, retainedSummaryTestSnapshot(reg, stringValue, dep))
	_ = ordinary.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(stringValue)}, pointSummaryDependencies{}, nil)
	build := owner.begin(key, retainedSummaryTestSnapshot(reg, numberValue, dep))
	_ = build.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(numberValue)}, pointSummaryDependencies{
		projection: map[summary.SummaryKey]pointSummaryRead{dep: {present: true, digest: 2}},
	}, &retainedSummaryTestResource{})

	got := owner.begin(key, retainedSummaryTestSnapshot(reg, stringValue, dep)).Decision()
	if got.kind != retainedSummaryApplyReproject || got.forceFull || !got.reproject || len(got.points) != 0 {
		t.Fatalf("projection-only decision = %+v, want reproject", got)
	}
	owner.Release()
}

func TestRetainedSummaryApplicationStructuralMismatchDropsGeneration(t *testing.T) {
	reg := standard.Registry()
	dep := dependencyTestKey(204)
	value := typevalue.FromType(reg, typ.String)
	reader := retainedSummaryTestSnapshot(reg, value, dep)
	owner := newRetainedSummaryApplicationOwner(reg)
	firstKey := retainedSummaryApplicationKey{body: 1, input: 2, profile: "a", resolution: 3}
	ordinary := owner.begin(firstKey, reader)
	_ = ordinary.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(value)}, pointSummaryDependencies{}, nil)
	build := owner.begin(firstKey, summary.NewSnapshot(reg))
	resource := &retainedSummaryTestResource{}
	_ = build.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: {present: false}}, pointSummaryDependencies{}, resource)

	got := owner.begin(retainedSummaryApplicationKey{body: 1, input: 2, profile: "b", resolution: 3}, reader).Decision()
	if got.kind != retainedSummaryApplyOrdinary || !got.dropped {
		t.Fatalf("structural mismatch decision = %+v, want dropped ordinary", got)
	}
	if owner.published != nil || resource.releases != 1 {
		t.Fatalf("mismatch retained publication/resource = %v/%d, want nil/1", owner.published, resource.releases)
	}
}

func TestRetainedSummaryApplicationCanceledAttemptPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	dep := dependencyTestKey(205)
	stringValue := typevalue.FromType(reg, typ.String)
	numberValue := typevalue.FromType(reg, typ.Number)
	key := retainedSummaryApplicationKey{body: 1}
	owner := newRetainedSummaryApplicationOwner(reg)
	ordinary := owner.begin(key, retainedSummaryTestSnapshot(reg, stringValue, dep))
	_ = ordinary.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(stringValue)}, pointSummaryDependencies{}, nil)

	canceled := owner.begin(key, retainedSummaryTestSnapshot(reg, numberValue, dep))
	if canceled.Decision().kind != retainedSummaryApplyBuild {
		t.Fatalf("canceled attempt kind = %v, want build", canceled.Decision().kind)
	}
	canceled.Abort()
	if canceled.Publish(map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(numberValue)}, pointSummaryDependencies{}, &retainedSummaryTestResource{}) {
		t.Fatal("aborted attempt published")
	}
	got := owner.begin(key, retainedSummaryTestSnapshot(reg, numberValue, dep)).Decision()
	if got.kind != retainedSummaryApplyBuild {
		t.Fatalf("state advanced after cancellation: %+v", got)
	}
}

func TestRetainedSummaryApplicationPublishesOwnedNormalizedDependencies(t *testing.T) {
	reg := standard.Registry()
	dep := dependencyTestKey(206)
	stringValue := typevalue.FromType(reg, typ.String)
	numberValue := typevalue.FromType(reg, typ.Number)
	key := retainedSummaryApplicationKey{body: 1}
	owner := newRetainedSummaryApplicationOwner(reg)
	deps := map[summary.SummaryKey]trackedSummaryRead{dep: retainedSummaryTestRead(stringValue)}
	attempt := owner.begin(key, retainedSummaryTestSnapshot(reg, stringValue, dep))
	if !attempt.Publish(deps, pointSummaryDependencies{}, nil) {
		t.Fatal("ordinary publication rejected")
	}

	// Mutating the caller-owned observation after publication cannot mutate the
	// owner's normalized snapshot.
	mutated := deps[dep]
	mutated.sum.Returns[0] = numberValue
	deps[dep] = mutated
	if got := owner.begin(key, retainedSummaryTestSnapshot(reg, stringValue, dep)).Decision(); got.kind != retainedSummaryApplyReuse {
		t.Fatalf("caller mutation changed publication: %+v", got)
	}
	if got := owner.begin(key, summary.NewSnapshot(reg)).Decision(); got.kind != retainedSummaryApplyBuild || !slices.Equal(got.changed, []summary.SummaryKey{dep}) {
		t.Fatalf("present-to-missing decision = %+v, want changed build", got)
	}
}
