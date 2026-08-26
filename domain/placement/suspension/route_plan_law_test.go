package suspension

import (
	"fmt"
	"sync"
	"testing"

	reduceroperand "github.com/wippyai/go-lua/analysis/engine/operand"
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
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

type suspensionRoutePlanFixture struct {
	placement placementdomain.Schema
	values    *valuedomain.Schema
	keys      []heap.Key
}

func suspensionRoutePlanFixtureFor(t testing.TB, width int) suspensionRoutePlanFixture {
	t.Helper()
	if width < 1 {
		t.Fatal("suspension route fixture width must be positive")
	}
	source := "local first = {};"
	for index := 1; index < width; index++ {
		source += fmt.Sprintf(" local value%d = {};", index)
	}
	source += " return first"
	program, err := lower.Lower(lower.Source{Name: fmt.Sprintf("placement-suspension-route-%d.lua", width), Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: fmt.Sprintf("placement-suspension-route-%d", width), Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, testfixture.EmptyProgramIssuancePlan(t))
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural := suspensionRoutePlanStructural(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	placementSchema, placementOK := placementdomain.NewSchema(heapSchema)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if !grammarOK || failure.Available() || artifact == nil || !lowered || !shardOK || !moduleOK || !programIDOK || !mountOK || !valueMountOK || heapFailure != heap.SealFailureNone || !placementOK || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("suspension route fixture grammar=%t failure=%v artifact=%t lowered=%t shard=%t module=%t program=%t mount=%t valueMount=%t heap=%v placement=%t value=%v", grammarOK, failure, artifact != nil, lowered, shardOK, moduleOK, programIDOK, mountOK, valueMountOK, heapFailure, placementOK, valueFailure)
	}
	keys := make([]heap.Key, 0, width)
	for index := 0; index < placementSchema.DenseKeyCount(); index++ {
		key, keyOK := placementSchema.KeyAt(index)
		if !keyOK {
			t.Fatal("placement route fixture key")
		}
		if key.Kind() == heap.RootAllocation {
			keys = append(keys, key)
		}
	}
	if len(keys) < width {
		t.Fatalf("placement route fixture allocations=%d want >=%d", len(keys), width)
	}
	return suspensionRoutePlanFixture{placement: placementSchema, values: values, keys: keys}
}

func suspensionRouteFact(t testing.TB, values *valuedomain.Schema, key heap.Key) valuedomain.Value {
	t.Helper()
	atom, atomOK := values.Allocation(key, materialization.Recent)
	if !atomOK {
		t.Fatal("suspension route allocation atom")
	}
	fact, factOK := values.Singleton(atom)
	if !factOK {
		t.Fatal("suspension route allocation fact")
	}
	return fact
}

func suspensionSourceFacts(fact valuedomain.Value) reduceroperand.SummaryVector[valuedomain.Value] {
	return suspensionSourceCells(reduceroperand.MemberCell[valuedomain.Value]{Value: fact, Present: true})
}

// suspensionSourceCells delivers a fixture source vector in the same member
// form the hot rules build from their selected Value read.
func suspensionSourceCells(cells ...reduceroperand.MemberCell[valuedomain.Value]) reduceroperand.SummaryVector[valuedomain.Value] {
	if cells == nil {
		cells = []reduceroperand.MemberCell[valuedomain.Value]{}
	}
	vector, vectorOK := reduceroperand.NewMemberVector(cells)
	if !vectorOK {
		return reduceroperand.SummaryVector[valuedomain.Value]{}
	}
	return vector
}

func suspensionRoutePlanStructural(t testing.TB) structure.Table {
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
			spelling := fmt.Sprintf("suspension-route/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("suspension route structure entries")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("suspension route structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(suspensionRoutePlanEmptySurface{kind: kind}) {
			t.Fatal("suspension route empty surface")
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatal("suspension route structure seal")
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("suspension route structure view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("suspension route structure table")
	}
	return table
}

type suspensionRoutePlanEmptySurface struct{ kind schema.SurfaceKind }

func (surface suspensionRoutePlanEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface suspensionRoutePlanEmptySurface) Entries() []schema.Entry  { return nil }
func (surface suspensionRoutePlanEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func TestSuspensionRoutePlanClassesAndLazyAllRoot(t *testing.T) {
	fixture := suspensionRoutePlanFixtureFor(t, routeInlineWidth+2)
	exactFact := suspensionRouteFact(t, fixture.values, fixture.keys[0])
	exact, exactOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceFacts(exactFact))
	if !exactOK || exact.class != routeExact || exact.count() != 1 || exact.allRoot || len(exact.extra) != 0 {
		t.Fatalf("exact suspension plan=%#v/%t", exact, exactOK)
	}
	dense, denseOK := fixture.placement.Heap().KeyIndex(fixture.keys[0])
	exactRoute, exactRouteOK := exact.at(0)
	if !denseOK || !exactRouteOK || exactRoute.key != fixture.keys[0] || exactRoute.tag != routeTag(uint64(dense)+1) {
		t.Fatalf("exact suspension route=%#v/%t dense=%d/%t", exactRoute, exactRouteOK, dense, denseOK)
	}

	bottom, bottomOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceCells(reduceroperand.MemberCell[valuedomain.Value]{Value: fixture.values.Bottom(), Present: true}))
	if !bottomOK || bottom.class != routeScalar || bottom.count() != 0 {
		t.Fatalf("Bottom suspension plan=%#v/%t, want scalar/no routes", bottom, bottomOK)
	}
	sparseBottom, sparseBottomOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceCells(reduceroperand.MemberCell[valuedomain.Value]{Value: fixture.values.Bottom()}))
	if !sparseBottomOK || sparseBottom.class != routeScalar || sparseBottom.count() != 0 {
		t.Fatalf("sparse Bottom suspension plan=%#v/%t, want scalar/no routes", sparseBottom, sparseBottomOK)
	}

	top, topOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceCells(reduceroperand.MemberCell[valuedomain.Value]{Value: fixture.values.Top(), Present: true}))
	if !topOK || top.class != routeWidened || !top.allRoot || len(top.extra) != 0 || top.count() != len(fixture.keys) {
		t.Fatalf("Top suspension plan=%#v/%t, want lazy all-root view", top, topOK)
	}
	for index := 0; index < top.count(); index++ {
		candidate, candidateOK := top.at(index)
		if !candidateOK || candidate.key.Kind() != heap.RootAllocation || candidate.tag == 0 {
			t.Fatalf("Top suspension route %d=%#v/%t", index, candidate, candidateOK)
		}
		byTag, byTagOK := routeAtTag(top, candidate.tag)
		if !byTagOK || byTag != candidate {
			t.Fatalf("Top suspension routeAtTag(%d)=%#v/%t want %#v/true", candidate.tag, byTag, byTagOK, candidate)
		}
	}
	if _, routeOK := routeAtTag(top, routeTag(0)); routeOK {
		t.Fatal("Top suspension plan accepted zero selection tag")
	}

	opaqueAtom, opaqueAtomOK := fixture.values.OpaqueReference(valuedomain.ReferenceOpaque)
	if !opaqueAtomOK {
		t.Fatal("opaque suspension atom")
	}
	opaqueFact, opaqueFactOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueFactOK {
		t.Fatal("opaque suspension fact")
	}
	opaque, opaqueOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceFacts(opaqueFact))
	if !opaqueOK || opaque.class != routeWidened || !opaque.allRoot || opaque.count() != top.count() || len(opaque.extra) != 0 {
		t.Fatalf("opaque suspension plan=%#v/%t, want lazy all-root view", opaque, opaqueOK)
	}
}

func TestSuspensionRoutePlanOverflowAndOwnerFence(t *testing.T) {
	fixture := suspensionRoutePlanFixtureFor(t, routeInlineWidth+4)
	atoms := make([]valuedomain.Atom, 0, routeInlineWidth+4)
	for _, key := range fixture.keys[:routeInlineWidth+4] {
		atom, atomOK := fixture.values.Allocation(key, materialization.Recent)
		if !atomOK {
			t.Fatal("overflow suspension atom")
		}
		atoms = append(atoms, atom)
	}
	fact, factOK := fixture.values.Alternatives(atoms...)
	if !factOK {
		t.Fatal("overflow suspension fact")
	}
	plan, planOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceFacts(fact))
	if !planOK || plan.class != routeExact || plan.count() != len(atoms) || len(plan.extra) != len(atoms)-routeInlineWidth {
		t.Fatalf("overflow suspension plan=%#v/%t", plan, planOK)
	}
	previous := routeTag(0)
	for index := 0; index < plan.count(); index++ {
		candidate, candidateOK := plan.at(index)
		if !candidateOK || candidate.tag <= previous {
			t.Fatalf("overflow suspension route %d=%#v/%t after %d", index, candidate, candidateOK, previous)
		}
		previous = candidate.tag
	}

	foreign := suspensionRoutePlanFixtureFor(t, 1)
	foreignFact := suspensionRouteFact(t, foreign.values, foreign.keys[0])
	if _, foreignOK := routePlanForSources(fixture.placement, foreign.values, suspensionSourceFacts(foreignFact)); foreignOK {
		t.Fatal("suspension route planner accepted foreign Value owner")
	}
	if _, foreignOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceFacts(foreignFact)); foreignOK {
		t.Fatal("suspension route planner accepted foreign Value fact")
	}
}

var (
	suspensionRoutePlanBenchmarkPlan routePlan
	suspensionRoutePlanBenchmarkOK   bool
)

func TestSuspensionRoutePlanAllocations(t *testing.T) {
	fixture := suspensionRoutePlanFixtureFor(t, 2)
	exactFact := suspensionRouteFact(t, fixture.values, fixture.keys[0])
	exactFacts := suspensionSourceFacts(exactFact)
	topFacts := suspensionSourceCells(reduceroperand.MemberCell[valuedomain.Value]{Value: fixture.values.Top(), Present: true})
	opaqueAtom, opaqueAtomOK := fixture.values.OpaqueReference(valuedomain.ReferenceOpaque)
	if !opaqueAtomOK {
		t.Fatal("opaque suspension atom")
	}
	opaqueFact, opaqueFactOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueFactOK {
		t.Fatal("opaque suspension fact")
	}
	opaqueFacts := suspensionSourceFacts(opaqueFact)
	if got := testing.AllocsPerRun(100, func() {
		suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK = routePlanForSources(fixture.placement, fixture.values, exactFacts)
	}); got != 0 || !suspensionRoutePlanBenchmarkOK || suspensionRoutePlanBenchmarkPlan.count() != 1 {
		t.Fatalf("exact suspension Plan allocations=%v plan=%#v/%t", got, suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK)
	}
	if got := testing.AllocsPerRun(100, func() {
		suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK = routePlanForSources(fixture.placement, fixture.values, topFacts)
	}); got != 0 || !suspensionRoutePlanBenchmarkOK || !suspensionRoutePlanBenchmarkPlan.widened() || !suspensionRoutePlanBenchmarkPlan.allRoot {
		t.Fatalf("Top suspension Plan allocations=%v plan=%#v/%t", got, suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK)
	}
	if got := testing.AllocsPerRun(100, func() {
		suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK = routePlanForSources(fixture.placement, fixture.values, opaqueFacts)
	}); got != 0 || !suspensionRoutePlanBenchmarkOK || !suspensionRoutePlanBenchmarkPlan.widened() || !suspensionRoutePlanBenchmarkPlan.allRoot {
		t.Fatalf("opaque suspension Plan allocations=%v plan=%#v/%t", got, suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK)
	}
}

func TestSuspensionRoutePlanWideViewIsConcurrent(t *testing.T) {
	fixture := suspensionRoutePlanFixtureFor(t, routeInlineWidth+4)
	top, topOK := routePlanForSources(fixture.placement, fixture.values, suspensionSourceCells(reduceroperand.MemberCell[valuedomain.Value]{Value: fixture.values.Top(), Present: true}))
	if !topOK || !top.allRoot || top.count() == 0 {
		t.Fatal("wide suspension plan")
	}
	const workers = 8
	const iterations = 100
	failed := make(chan struct{}, 1)
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				candidate, candidateOK := top.at(iteration % top.count())
				byTag, byTagOK := routeAtTag(top, candidate.tag)
				if !candidateOK || !byTagOK || byTag != candidate {
					select {
					case failed <- struct{}{}:
					default:
					}
					return
				}
			}
		}()
	}
	wait.Wait()
	select {
	case <-failed:
		t.Fatal("concurrent suspension wide plan changed")
	default:
	}
}

func BenchmarkSuspensionRoutePlanExact(b *testing.B) {
	fixture := suspensionRoutePlanFixtureFor(b, 2)
	fact := suspensionRouteFact(b, fixture.values, fixture.keys[0])
	facts := suspensionSourceFacts(fact)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK = routePlanForSources(fixture.placement, fixture.values, facts)
		if !suspensionRoutePlanBenchmarkOK || suspensionRoutePlanBenchmarkPlan.count() != 1 {
			b.Fatal("exact suspension Plan")
		}
	}
}

func BenchmarkSuspensionRoutePlanTopScaling(b *testing.B) {
	for _, width := range []int{1, routeInlineWidth, 32} {
		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {
			fixture := suspensionRoutePlanFixtureFor(b, width)
			facts := suspensionSourceCells(reduceroperand.MemberCell[valuedomain.Value]{Value: fixture.values.Top(), Present: true})
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK = routePlanForSources(fixture.placement, fixture.values, facts)
				if !suspensionRoutePlanBenchmarkOK || !suspensionRoutePlanBenchmarkPlan.widened() || !suspensionRoutePlanBenchmarkPlan.allRoot {
					b.Fatal("Top suspension Plan")
				}
			}
		})
	}
}

func BenchmarkSuspensionRoutePlanOpaqueScaling(b *testing.B) {
	for _, width := range []int{1, routeInlineWidth, 32} {
		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {
			fixture := suspensionRoutePlanFixtureFor(b, width)
			atom, atomOK := fixture.values.OpaqueReference(valuedomain.ReferenceOpaque)
			if !atomOK {
				b.Fatal("opaque suspension atom")
			}
			fact, factOK := fixture.values.Singleton(atom)
			if !factOK {
				b.Fatal("opaque suspension fact")
			}
			facts := suspensionSourceFacts(fact)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				suspensionRoutePlanBenchmarkPlan, suspensionRoutePlanBenchmarkOK = routePlanForSources(fixture.placement, fixture.values, facts)
				if !suspensionRoutePlanBenchmarkOK || !suspensionRoutePlanBenchmarkPlan.widened() || !suspensionRoutePlanBenchmarkPlan.allRoot {
					b.Fatal("opaque suspension Plan")
				}
			}
		})
	}
}
