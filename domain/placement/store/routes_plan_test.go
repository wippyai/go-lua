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

// The route relation is DECLARED, so the laws below are stated over the three
// authored judgments and over the construction the emitter writes from them.
// Two things the authored plan carried are gone rather than restated
// elsewhere: a route class that separated Bottom from an ordinary scalar, and
// a lookup by tag. Neither had a reader outside this file - a consumer of this
// relation sees rows, and the engine reaches a member through the plane's own
// selected-member table - so what survives of both is what was ever
// observable, which is how many rows there are and in what order.

// routes derives one transfer's whole route set the way the emitted family
// does, so every law below reads the construction the rule actually runs.
func routes(t testing.TB, fixture routePlanFixture, transfer valuedomain.StorageTransfer, fact valuedomain.Value) (derived1Rows, bool) {
	t.Helper()
	return deriveDerived1Rows(fixture.placement, fixture.values, transfer, fact)
}

func TestARouteSetAuthenticatesItsCandidateAndKeepsFrameTransfersEmpty(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 2)
	persistent := routePlanTransfer(t, fixture.values, true)
	bottom, bottomOK := routes(t, fixture, persistent, fixture.values.Bottom())
	if !bottomOK || derived1Count(bottom) != 0 {
		t.Fatalf("persistent Bottom route set=%#v/%t, want no routes", bottom, bottomOK)
	}
	if _, ok := routes(t, fixture, valuedomain.StorageTransfer{}, fixture.values.Bottom()); ok {
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
	plan, planOK := routes(t, fixture, persistent, source)
	route, routeOK := derived1At(plan, 0)
	if !planOK || !routeOK || derived1Count(plan) != 1 {
		t.Fatalf("authenticated route set=%#v/%t route=%#v/%t", plan, planOK, route, routeOK)
	}
	got, outcome := StorageFold(persistent, source, route.Tag, placement.DefaultFact())
	if outcome != structure.Concrete || got != (placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("StorageFold=%v/%v, want authenticated SharedHeap/Concrete", got, outcome)
	}

	// A frame-local transfer reaches no store. It has no routes and it is not
	// beyond enumeration either: widening a whole directory to discover that
	// every row of it declines is the answer the endpoint exists to avoid.
	frame := routePlanTransfer(t, fixture.values, false)
	empty, emptyOK := routes(t, fixture, frame, source)
	if !emptyOK || derived1Count(empty) != 0 || empty.widened {
		t.Fatalf("frame-local transfer must derive a valid empty route set: %#v/%t", empty, emptyOK)
	}
	// A relation this schema never issued is refused rather than answered as
	// an empty one. It is the endpoint that says so: it is the only judgment
	// that runs when the value yields no atom for ResolveRoute to fence.
	if _, ok := routes(t, fixture, persistent, valuedomain.Value{}); ok {
		t.Fatal("a Value this schema never issued derived a route set")
	}
	if beyond, admissible := BeyondAllocations(fixture.placement, fixture.values, frame, fixture.values.Top()); beyond || !admissible {
		t.Fatalf("a frame-local transfer widened to a directory it reaches nothing in: beyond=%t admissible=%t", beyond, admissible)
	}
	wide, wideOK := routes(t, fixture, frame, fixture.values.Top())
	if !wideOK || derived1Count(wide) != 0 {
		t.Fatalf("frame-local Top route set=%#v/%t, want no routes", wide, wideOK)
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
	plan, planOK := routes(t, fixture, candidate, source)
	route, routeOK := derived1At(plan, 0)
	if !planOK || !routeOK || route.Tag == 0 {
		t.Fatalf("authenticated route set=%#v/%t route=%#v/%t", plan, planOK, route, routeOK)
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

// TestARouteSetAnswersEachValueShapeInCanonicalTagOrder is the whole route
// algebra stated over the shapes a Value takes: nothing, a scalar, one
// allocation, several, and the two ways of naming no closed list at all.
func TestARouteSetAnswersEachValueShapeInCanonicalTagOrder(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 8)
	persistent := routePlanTransfer(t, fixture.values, true)

	bottom, bottomOK := routes(t, fixture, persistent, fixture.values.Bottom())
	if !bottomOK || bottom.widened || derived1Count(bottom) != 0 {
		t.Fatalf("Bottom route set = %#v/%t, want no routes", bottom, bottomOK)
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
	scalarPlan, scalarPlanOK := routes(t, fixture, persistent, scalar)
	if !scalarPlanOK || scalarPlan.widened || derived1Count(scalarPlan) != 0 {
		t.Fatalf("scalar route set = %#v/%t, want no routes", scalarPlan, scalarPlanOK)
	}

	atom, atomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation fact")
	}
	exact, exactOK := routes(t, fixture, persistent, fact)
	if !exactOK || exact.widened || derived1Count(exact) != 1 {
		t.Fatalf("exact route set = %#v/%t, want one exact route", exact, exactOK)
	}
	route, routeOK := derived1At(exact, 0)
	dense, denseOK := fixture.placement.Heap().KeyIndex(route.Key)
	if !routeOK || !denseOK || route.Tag != uint64(dense)+1 || route.Key != fixture.keys[0] {
		t.Fatalf("exact route = %#v/%t dense=%d/%t", route, routeOK, dense, denseOK)
	}

	first, firstOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	second, secondOK := fixture.values.Allocation(fixture.keys[1], materialization.Recent)
	third, thirdOK := fixture.values.Allocation(fixture.keys[2], materialization.Recent)
	if !firstOK || !secondOK || !thirdOK {
		t.Fatal("allocation atoms for alternatives")
	}
	// The first allocation is named twice. One member reached at one address
	// is one member, so the alias adds no ordinal.
	joined, joinedOK := fixture.values.Alternatives(third, first, second, first)
	if !joinedOK {
		t.Fatal("joined fact")
	}
	joinedPlan, joinedPlanOK := routes(t, fixture, persistent, joined)
	if !joinedPlanOK || joinedPlan.widened || derived1Count(joinedPlan) != 3 {
		t.Fatalf("joined route set = %#v/%t, want three exact routes", joinedPlan, joinedPlanOK)
	}
	assertAscendingTags(t, joinedPlan)

	top, topOK := routes(t, fixture, persistent, fixture.values.Top())
	if !topOK || !top.widened || derived1Count(top) != len(fixture.keys) {
		t.Fatalf("Top route set = %#v/%t, want widened allocation routes", top, topOK)
	}
	for index := 0; index < derived1Count(top); index++ {
		candidate, candidateOK := derived1At(top, index)
		if !candidateOK || candidate.Key.Kind() != heap.RootAllocation {
			t.Fatalf("Top route %d = %#v/%t", index, candidate, candidateOK)
		}
	}
	assertAscendingTags(t, top)

	opaqueAtom, opaqueOK := fixture.values.OpaqueReference(valuedomain.ReferenceOpaque)
	if !opaqueOK {
		t.Fatal("opaque atom")
	}
	opaqueFact, opaqueFactOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueFactOK {
		t.Fatal("opaque fact")
	}
	opaquePlan, opaquePlanOK := routes(t, fixture, persistent, opaqueFact)
	if !opaquePlanOK || !opaquePlan.widened || derived1Count(opaquePlan) != derived1Count(top) {
		t.Fatalf("opaque route set = %#v/%t, want widened allocation routes", opaquePlan, opaquePlanOK)
	}
}

// assertAscendingTags states the order both arms leave through. The engine
// canonicalizes a selection by the coordinate its cells are read at, and a
// route's tag is that coordinate plus one.
func assertAscendingTags(t testing.TB, plan derived1Rows) {
	t.Helper()
	previous := uint64(0)
	for index := 0; index < derived1Count(plan); index++ {
		candidate, candidateOK := derived1At(plan, index)
		if !candidateOK || candidate.Tag <= previous {
			t.Fatalf("route %d = %#v/%t after tag %d", index, candidate, candidateOK, previous)
		}
		previous = candidate.Tag
	}
}

func TestARouteSetRejectsForeignFactsAndIsConcurrent(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 2)
	foreign := routePlanFixtureForStore(t, 2)
	persistent := routePlanTransfer(t, fixture.values, true)
	foreignPersistent := routePlanTransfer(t, foreign.values, true)
	foreignAtom, foreignAtomOK := foreign.values.Allocation(foreign.keys[0], materialization.Recent)
	if !foreignAtomOK {
		t.Fatal("foreign allocation atom")
	}
	foreignFact, foreignFactOK := foreign.values.Singleton(foreignAtom)
	if !foreignFactOK {
		t.Fatal("foreign fact")
	}
	if _, ok := deriveDerived1Rows(fixture.placement, foreign.values, foreignPersistent, foreignFact); ok {
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
	if _, ok := routes(t, fixture, persistent, foreignFact); ok {
		t.Fatal("foreign Value fact crossed value owner fence")
	}
	exactPlan, exactPlanOK := routes(t, fixture, persistent, localFact)
	widePlan, widePlanOK := routes(t, fixture, persistent, fixture.values.Top())
	if !exactPlanOK || derived1Count(exactPlan) != 1 || !widePlanOK || !widePlan.widened || derived1Count(widePlan) != len(fixture.keys) {
		t.Fatal("route sets for concurrency")
	}

	// A widened set holds the owner's schema and answers each member on
	// demand, so reading one concurrently reads that schema concurrently.
	const workers = 8
	const iterations = 100
	errors := make(chan string, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				exact, exactOK := derived1At(exactPlan, 0)
				wide, wideOK := derived1At(widePlan, iteration%derived1Count(widePlan))
				if !exactOK || exact.Tag == 0 || !wideOK || wide.Key.Kind() != heap.RootAllocation {
					errors <- "concurrent route set result"
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

func TestARouteSetPastItsInlineWidthRemainsCanonical(t *testing.T) {
	fixture := routePlanFixtureForStore(t, derived1InlineWidth+4)
	persistent := routePlanTransfer(t, fixture.values, true)
	atoms := make([]valuedomain.Atom, 0, derived1InlineWidth+4)
	for _, key := range fixture.keys[:derived1InlineWidth+4] {
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
	plan, planOK := routes(t, fixture, persistent, fact)
	if !planOK || plan.widened || derived1Count(plan) != len(atoms) {
		t.Fatalf("overflow route set = %#v/%t", plan, planOK)
	}
	assertAscendingTags(t, plan)
}

// TestARouteSetAllocatesNothing is the migration bar. The authored derivation
// this construction replaces was allocation-free in both arms and said why in
// its own comments; the generated one holds its ordinary answer by value and
// reads its widened answer where it lies, so it is allocation-free for the
// same two reasons.
func TestARouteSetAllocatesNothing(t *testing.T) {
	fixture := routePlanFixtureForStore(t, 2)
	persistent := routePlanTransfer(t, fixture.values, true)
	atom, atomOK := fixture.values.Allocation(fixture.keys[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation fact")
	}
	if got := testing.AllocsPerRun(100, func() {
		plan, ok := routes(t, fixture, persistent, fact)
		if !ok || derived1Count(plan) != 1 {
			t.Fatal("exact route set")
		}
	}); got != 0 {
		t.Fatalf("exact route set allocations=%v", got)
	}
	if got := testing.AllocsPerRun(100, func() {
		plan, ok := routes(t, fixture, persistent, fixture.values.Top())
		if !ok || !plan.widened || derived1Count(plan) != len(fixture.keys) {
			t.Fatal("wide route set")
		}
	}); got != 0 {
		t.Fatalf("wide route set allocations=%v", got)
	}
}

var (
	storePlanBenchmarkPlan derived1Rows
	storePlanBenchmarkOK   bool
)

func BenchmarkRouteSetExact(b *testing.B) {
	fixture := routePlanFixtureForStore(b, 2)
	persistent := routePlanTransfer(b, fixture.values, true)
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
		storePlanBenchmarkPlan, storePlanBenchmarkOK = deriveDerived1Rows(fixture.placement, fixture.values, persistent, fact)
		if !storePlanBenchmarkOK || derived1Count(storePlanBenchmarkPlan) != 1 {
			b.Fatal("exact route set")
		}
	}
}

func BenchmarkRouteSetTopScaling(b *testing.B) {
	for _, width := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {
			fixture := routePlanFixtureForStore(b, width)
			persistent := routePlanTransfer(b, fixture.values, true)
			fact := fixture.values.Top()
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				storePlanBenchmarkPlan, storePlanBenchmarkOK = deriveDerived1Rows(fixture.placement, fixture.values, persistent, fact)
				if !storePlanBenchmarkOK || !storePlanBenchmarkPlan.widened || derived1Count(storePlanBenchmarkPlan) != len(fixture.keys) {
					b.Fatal("wide route set")
				}
			}
		})
	}
}
