package store

import (
	"fmt"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

type routePlanFixture struct {
	placement placement.Schema
	values    *valuedomain.Schema
	keys      []heap.Key
}

func routePlanTransfer(t testing.TB, values *valuedomain.Schema, persistent bool) valuedomain.StorageTransfer {
	t.Helper()
	for index := 0; index < values.StorageTransferCount(); index++ {
		transfer, ok := values.StorageTransferAt(index)
		if ok && transfer.Persistent() == persistent {
			return transfer
		}
	}
	t.Fatalf("Value fixture has no persistent=%t StorageTransfer", persistent)
	return valuedomain.StorageTransfer{}
}

func routePlanFixtureForStore(t testing.TB, width int) routePlanFixture {
	t.Helper()
	// A global store gives this fixture one persistent transfer; returning it
	// also preserves a frame-local read transfer. Both are needed to exercise
	// the public relation derivation boundary rather than only its private
	// route-set algebra.
	source := "stored = {"
	for index := 0; index < width; index++ {
		if index != 0 {
			source += ","
		}
		source += fmt.Sprintf("[%d] = {}", index+1)
	}
	source += "}; return stored"
	program, err := lower.Lower(lower.Source{Name: fmt.Sprintf("store-route-%d.lua", width), Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: fmt.Sprintf("store-route-%d", width), Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if !grammarOK || failure.Available() || artifact == nil {
		t.Fatalf("artifact grammar=%t failure=%v artifact=%t", grammarOK, failure, artifact != nil)
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	if !shardOK || !moduleOK || !programIDOK {
		t.Fatal("mount")
	}
	structural := routePlanStructural(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	projected, projectedOK := placement.NewSchema(heapSchema)
	if !lowered || !mountOK || !valueMountOK || heapFailure != heap.SealFailureNone || valueFailure != valuedomain.SealFailureNone || !projectedOK || values == nil {
		t.Fatalf("seal lowered=%t mount=%t valueMount=%t heap=%v value=%v placement=%t", lowered, mountOK, valueMountOK, heapFailure, valueFailure, projectedOK)
	}
	keys := make([]heap.Key, 0, projected.DenseKeyCount())
	for index := 0; index < projected.DenseKeyCount(); index++ {
		key, keyOK := projected.KeyAt(index)
		if !keyOK {
			t.Fatal("key")
		}
		if key.Kind() == heap.RootAllocation {
			keys = append(keys, key)
		}
	}
	if len(keys) < width {
		t.Fatalf("allocation roots=%d want >=%d", len(keys), width)
	}
	return routePlanFixture{placement: projected, values: values, keys: keys}
}

func TestDeriveRoutesAuthenticatesCandidateAndKeepsFrameTransfersEmpty(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 2)
	persistent := routePlanTransfer(t, fixture.values, true)
	bottom, bottomOK := DeriveRoutes(fixture.placement, fixture.values, persistent, fixture.values.Bottom())
	if !bottomOK || !bottom.Valid() || !bottom.Bottom() || RouteCount(bottom) != 0 {
		t.Fatalf("persistent Bottom derivation=%#v/%t", bottom, bottomOK)
	}
	if _, ok := DeriveRoutes(fixture.placement, fixture.values, valuedomain.StorageTransfer{}, fixture.values.Bottom()); ok {
		t.Fatal("forged StorageTransfer candidate admitted")
	}
	atom, atomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	source, sourceOK := fixture.values.Singleton(atom)
	if !sourceOK {
		t.Fatal("allocation source fact")
	}
	plan, planOK := DeriveRoutes(fixture.placement, fixture.values, persistent, source)
	route, routeOK := RouteAt(plan, 0)
	if !planOK || !routeOK || RouteCount(plan) != 1 {
		t.Fatalf("authenticated route plan=%#v/%t route=%#v/%t", plan, planOK, route, routeOK)
	}
	got, outcome := StorageFold(persistent, source, route.Tag, placement.DefaultFact())
	if outcome != structure.Concrete || got != (placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("StorageFold=%v/%v, want authenticated SharedHeap/Concrete", got, outcome)
	}

	frame := routePlanTransfer(t, fixture.values, false)
	empty, emptyOK := DeriveRoutes(fixture.placement, fixture.values, frame, valuedomain.Value{})
	if !emptyOK || !empty.Valid() || empty.Bottom() || empty.Widened() || RouteCount(empty) != 0 {
		t.Fatalf("frame-local transfer must derive a valid empty route set: %#v/%t", empty, emptyOK)
	}
}

func TestRouteReducerAdmitsAuthenticatedSparsePlacementDefault(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 1)
	candidate := routePlanTransfer(t, fixture.values, true)
	atom, atomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	source, sourceOK := fixture.values.Singleton(atom)
	if !sourceOK {
		t.Fatal("allocation source fact")
	}
	plan, planOK := DeriveRoutes(fixture.placement, fixture.values, candidate, source)
	route, routeOK := RouteAt(plan, 0)
	if !planOK || !routeOK || route.Tag == 0 {
		t.Fatalf("authenticated route plan=%#v/%t route=%#v/%t", plan, planOK, route, routeOK)
	}

	got, outcome := (familyReducer{candidate: candidate, input0: source}).Reduce(route.Key, execution.SelectedCell[placement.Fact]{
		Value:   placement.DefaultFact(),
		Present: false,
		Tag:     route.Tag,
	})
	want := placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}
	if outcome != structure.Concrete || got != want {
		t.Fatalf("sparse Placement default reduction=%v/%v, want %v/Concrete", got, outcome, want)
	}
}

func routePlanStructural(t testing.TB) structure.Table {
	t.Helper()
	counts := func(category structure.Category) int {
		switch category {
		case structure.CategoryArm:
			return 8
		case structure.CategoryEvent:
			return 3
		case structure.CategoryOutcome:
			return 7
		case structure.CategoryRuntimeKind:
			return int(runtimekind.Count) - 1
		case structure.CategoryOccurrenceKind:
			return 32
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("store-route/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("structure entries")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(routePlanEmptySurface{kind: kind}) {
			t.Fatal("empty surface")
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatal("structure seal")
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("structure view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("structure table")
	}
	return table
}

type routePlanEmptySurface struct{ kind schema.SurfaceKind }

func (surface routePlanEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface routePlanEmptySurface) Entries() []schema.Entry  { return nil }
func (surface routePlanEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func TestPlanClassesAndCanonicalTags(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 8)

	bottom, bottomOK := planRoutes(fixture.placement, fixture.values, fixture.values.Bottom())
	if !bottomOK || !bottom.Valid() || !bottom.Bottom() || bottom.Widened() || RouteCount(bottom) != 0 {
		t.Fatalf("Bottom plan = %#v/%t, want valid Bottom with no routes", bottom, bottomOK)
	}

	var scalarAtom valuedomain.Atom
	scalarFound := false
	if !fixture.values.VisitSupport(fixture.values.Top(), func(atom valuedomain.Atom) {
		if scalarFound {
			return
		}
		classification, classificationOK := placement.ClassifyAtom(fixture.values, atom)
		if classificationOK && classification.Class == placement.AtomClassScalar {
			scalarAtom = atom
			scalarFound = true
		}
	}) || !scalarFound {
		t.Fatal("scalar atom")
	}
	scalar, scalarOK := fixture.values.Singleton(scalarAtom)
	if !scalarOK {
		t.Fatal("scalar fact")
	}
	scalarPlan, scalarPlanOK := planRoutes(fixture.placement, fixture.values, scalar)
	if !scalarPlanOK || !scalarPlan.Valid() || scalarPlan.Bottom() || scalarPlan.Widened() || RouteCount(scalarPlan) != 0 {
		t.Fatalf("scalar plan = %#v/%t, want valid scalar with no routes", scalarPlan, scalarPlanOK)
	}

	atom, atomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation fact")
	}
	exact, exactOK := planRoutes(fixture.placement, fixture.values, fact)
	if !exactOK || !exact.Valid() || exact.Bottom() || exact.Widened() || RouteCount(exact) != 1 {
		t.Fatalf("exact plan = %#v/%t, want one exact route", exact, exactOK)
	}
	route, routeOK := RouteAt(exact, 0)
	dense, denseOK := fixture.placement.Heap().KeyIndex(route.Key)
	if !routeOK || !denseOK || route.Tag != uint64(dense)+1 || route.Key != fixture.keys[0] {
		t.Fatalf("exact route = %#v/%t dense=%d/%t", route, routeOK, dense, denseOK)
	}
	if got, gotOK := exact.routeAtTag(route.Tag); !gotOK || got != route {
		t.Fatalf("exact routeAtTag = %#v/%t, want %#v/true", got, gotOK, route)
	}

	first, firstOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	second, secondOK := fixture.values.Allocation(fixture.keys[1], materialization.Recent)
	third, thirdOK := fixture.values.Allocation(fixture.keys[2], materialization.Recent)
	if !firstOK || !secondOK || !thirdOK {
		t.Fatal("allocation atoms for alternatives")
	}
	joined, joinedOK := fixture.values.Alternatives(third, first, second, first)
	if !joinedOK {
		t.Fatal("joined fact")
	}
	joinedPlan, joinedPlanOK := planRoutes(fixture.placement, fixture.values, joined)
	if !joinedPlanOK || !joinedPlan.Valid() || joinedPlan.Widened() || RouteCount(joinedPlan) != 3 {
		t.Fatalf("joined plan = %#v/%t, want three exact routes", joinedPlan, joinedPlanOK)
	}
	previousTag := uint64(0)
	for index := 0; index < RouteCount(joinedPlan); index++ {
		candidate, candidateOK := RouteAt(joinedPlan, index)
		if !candidateOK || candidate.Tag <= previousTag {
			t.Fatalf("joined route %d = %#v/%t after tag %d", index, candidate, candidateOK, previousTag)
		}
		previousTag = candidate.Tag
		if byTag, byTagOK := joinedPlan.routeAtTag(candidate.Tag); !byTagOK || byTag != candidate {
			t.Fatalf("joined routeAtTag(%d) = %#v/%t, want %#v/true", candidate.Tag, byTag, byTagOK, candidate)
		}
	}

	top, topOK := planRoutes(fixture.placement, fixture.values, fixture.values.Top())
	if !topOK || !top.Valid() || !top.Widened() || top.Bottom() || RouteCount(top) != len(fixture.keys) {
		t.Fatalf("Top plan = %#v/%t, want widened allocation routes", top, topOK)
	}
	for index := 0; index < RouteCount(top); index++ {
		candidate, candidateOK := RouteAt(top, index)
		if !candidateOK || candidate.Key.Kind() != heap.RootAllocation {
			t.Fatalf("Top route %d = %#v/%t", index, candidate, candidateOK)
		}
		if byTag, byTagOK := top.routeAtTag(candidate.Tag); !byTagOK || byTag != candidate {
			t.Fatalf("Top routeAtTag(%d) = %#v/%t, want %#v/true", candidate.Tag, byTag, byTagOK, candidate)
		}
	}

	opaqueAtom, opaqueOK := fixture.values.OpaqueReference(valuedomain.ReferenceOpaque)
	if !opaqueOK {
		t.Fatal("opaque atom")
	}
	opaqueFact, opaqueFactOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueFactOK {
		t.Fatal("opaque fact")
	}
	opaquePlan, opaquePlanOK := planRoutes(fixture.placement, fixture.values, opaqueFact)
	if !opaquePlanOK || !opaquePlan.Widened() || RouteCount(opaquePlan) != RouteCount(top) {
		t.Fatalf("opaque plan = %#v/%t, want widened allocation routes", opaquePlan, opaquePlanOK)
	}
}

func TestPlanRejectsForeignFactsAndIsConcurrent(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 2)
	foreign := routePlanFixtureForStore(t, 2)
	foreignAtom, foreignAtomOK := foreign.values.Allocation(foreign.keys[0], materialization.Recent)
	if !foreignAtomOK {
		t.Fatal("foreign allocation atom")
	}
	foreignFact, foreignFactOK := foreign.values.Singleton(foreignAtom)
	if !foreignFactOK {
		t.Fatal("foreign fact")
	}
	if _, ok := planRoutes(fixture.placement, foreign.values, foreignFact); ok {
		t.Fatal("foreign Value schema crossed placement owner fence")
	}
	localAtom, localAtomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !localAtomOK {
		t.Fatal("local allocation atom")
	}
	localFact, localFactOK := fixture.values.Singleton(localAtom)
	if !localFactOK {
		t.Fatal("local fact")
	}
	if _, ok := planRoutes(fixture.placement, fixture.values, foreignFact); ok {
		t.Fatal("foreign Value fact crossed value owner fence")
	}
	exactPlan, exactPlanOK := planRoutes(fixture.placement, fixture.values, localFact)
	widePlan, widePlanOK := planRoutes(fixture.placement, fixture.values, fixture.values.Top())
	if !exactPlanOK || RouteCount(exactPlan) != 1 || !widePlanOK || !widePlan.Widened() || RouteCount(widePlan) != len(fixture.keys) {
		t.Fatal("plans for concurrency")
	}

	const workers = 8
	const iterations = 100
	errors := make(chan string, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				exact, exactOK := RouteAt(exactPlan, 0)
				wide, wideOK := RouteAt(widePlan, iteration%RouteCount(widePlan))
				_, routeOK := widePlan.routeAtTag(wide.Tag)
				if !exactOK || exact.Tag == 0 || !wideOK || wide.Key.Kind() != heap.RootAllocation || !routeOK {
					errors <- "concurrent Plan result"
					return
				}
			}
		}()
	}
	wait.Wait()
	select {
	case message := <-errors:
		t.Fatal(message)
	default:
	}
}

func TestPlanInlineOverflowRemainsCanonical(t *testing.T) {
	fixture := routePlanFixtureForStore(t, routeInlineWidth+4)
	atoms := make([]valuedomain.Atom, 0, routeInlineWidth+4)
	for _, key := range fixture.keys[:routeInlineWidth+4] {
		atom, atomOK := fixture.values.Allocation(key, materialization.Recent)
		if !atomOK {
			t.Fatal("allocation atom")
		}
		atoms = append(atoms, atom)
	}
	fact, factOK := fixture.values.Alternatives(atoms...)
	if !factOK {
		t.Fatal("overflow fact")
	}
	plan, planOK := planRoutes(fixture.placement, fixture.values, fact)
	if !planOK || !plan.Valid() || plan.Widened() || RouteCount(plan) != len(atoms) {
		t.Fatalf("overflow plan = %#v/%t", plan, planOK)
	}
	for index := 0; index < RouteCount(plan); index++ {
		candidate, candidateOK := RouteAt(plan, index)
		if !candidateOK {
			t.Fatalf("overflow route %d unavailable", index)
		}
		if index > 0 {
			prior, priorOK := RouteAt(plan, index-1)
			if !priorOK || prior.Tag >= candidate.Tag {
				t.Fatalf("overflow route order at %d: prior=%#v current=%#v", index, prior, candidate)
			}
		}
	}
}

func TestPlanAllocations(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 2)
	atom, atomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation fact")
	}
	if got := testing.AllocsPerRun(100, func() {
		plan, ok := planRoutes(fixture.placement, fixture.values, fact)
		if !ok || !plan.Valid() || RouteCount(plan) != 1 {
			t.Fatal("exact plan")
		}
	}); got != 0 {
		t.Fatalf("exact Plan allocations=%v", got)
	}
	if got := testing.AllocsPerRun(100, func() {
		plan, ok := planRoutes(fixture.placement, fixture.values, fixture.values.Top())
		if !ok || !plan.Widened() || RouteCount(plan) != len(fixture.keys) {
			t.Fatal("wide plan")
		}
	}); got != 0 {
		t.Fatalf("wide Plan allocations=%v", got)
	}
}

var (
	storePlanBenchmarkPlan RoutePlan
	storePlanBenchmarkOK   bool
)

func BenchmarkPlanExact(b *testing.B) {
	fixture := routePlanFixtureForStore(b, 2)
	atom, atomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !atomOK {
		b.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		b.Fatal("allocation fact")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		storePlanBenchmarkPlan, storePlanBenchmarkOK = planRoutes(fixture.placement, fixture.values, fact)
		if !storePlanBenchmarkOK || RouteCount(storePlanBenchmarkPlan) != 1 {
			b.Fatal("exact Plan")
		}
	}
}

func BenchmarkPlanTopScaling(b *testing.B) {
	for _, width := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {
			fixture := routePlanFixtureForStore(b, width)
			fact := fixture.values.Top()
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				storePlanBenchmarkPlan, storePlanBenchmarkOK = planRoutes(fixture.placement, fixture.values, fact)
				if !storePlanBenchmarkOK || !storePlanBenchmarkPlan.Widened() || RouteCount(storePlanBenchmarkPlan) != len(fixture.keys) {
					b.Fatal("wide Plan")
				}
			}
		})
	}
}
