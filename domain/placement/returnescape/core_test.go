package returnescape

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

type returnPlanFixture struct {
	values      *valuedomain.Schema
	placement   placement.Schema
	module      identity.ContentID
	returnID    identity.ContentID
	allocations []heap.Key
}

func TestReturnRoutePlanScalarAndExactAllocation(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	scalarAtom, scalarOK := fixture.values.OpaqueKind(runtimekind.Number)
	if !scalarOK {
		t.Fatal("numeric scalar atom")
	}
	scalar, scalarOK := fixture.values.Singleton(scalarAtom)
	if !scalarOK {
		t.Fatal("numeric scalar value")
	}
	plan, ok := routePlanFor(fixture.placement, fixture.values, scalar)
	if !ok || plan.class != routeScalar || plan.routeCount() != 0 {
		t.Fatalf("scalar return plan = %#v/%t", plan, ok)
	}

	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation value")
	}
	plan, ok = routePlanFor(fixture.placement, fixture.values, fact)
	firstRoute, firstRouteOK := plan.routeAt(0)
	if !ok || plan.class != routeExact || plan.routeCount() != 1 || !firstRouteOK || firstRoute.key != fixture.allocations[0] {
		t.Fatalf("exact allocation return plan = %#v/%t", plan, ok)
	}
}

func TestReturnRoutePlanAliasesAndAlternateJoinsDeduplicateAndOrderRoots(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	if len(fixture.allocations) < 2 {
		t.Skip("fixture has one allocation root")
	}
	first, firstOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	second, secondOK := fixture.values.Allocation(fixture.allocations[1], materialization.Recent)
	alias, aliasOK := fixture.values.Allocation(fixture.allocations[0], materialization.Summary)
	if !firstOK || !secondOK || !aliasOK {
		t.Fatal("allocation alternatives")
	}
	fact, factOK := fixture.values.Alternatives(first, alias, second)
	if !factOK {
		t.Fatal("alternate allocation value")
	}
	plan, ok := routePlanFor(fixture.placement, fixture.values, fact)
	if !ok || plan.class != routeExact || plan.routeCount() != 2 {
		t.Fatalf("alternate return plan = %#v/%t", plan, ok)
	}
	firstRoute, firstRouteOK := plan.routeAt(0)
	secondRoute, secondRouteOK := plan.routeAt(1)
	firstIndex, firstIndexOK := fixture.placement.Heap().KeyIndex(firstRoute.key)
	secondIndex, secondIndexOK := fixture.placement.Heap().KeyIndex(secondRoute.key)
	if !firstRouteOK || !secondRouteOK || !firstIndexOK || !secondIndexOK || firstIndex >= secondIndex {
		t.Fatalf("alternate routes are not Heap ordered: %d/%d", firstIndex, secondIndex)
	}
}

func TestReturnRoutePlanTopAndOpaqueWidenEveryAllocationRoot(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	plan, ok := routePlanFor(fixture.placement, fixture.values, fixture.values.Top())
	if !ok || plan.class != routeWidened || plan.routeCount() != len(fixture.allocations) {
		t.Fatalf("top return plan = %#v/%t", plan, ok)
	}
	opaqueAtom, opaqueOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	if !opaqueOK {
		t.Fatal("opaque reference atom")
	}
	opaque, opaqueOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueOK {
		t.Fatal("opaque reference value")
	}
	plan, ok = routePlanFor(fixture.placement, fixture.values, opaque)
	if !ok || plan.class != routeWidened || plan.routeCount() != len(fixture.allocations) {
		t.Fatalf("opaque return plan = %#v/%t", plan, ok)
	}
}

func TestReturnRoutePlanFactsRequiresEvidenceAndOnlyAuthenticatedWidening(t *testing.T) {
	fixture := newReturnPlanFixture(t)

	var unavailable returnFacts
	unavailable.append(returnFact{available: false})
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, unavailable, false); ok {
		t.Fatalf("missing selected return cell fabricated plan: %#v", plan)
	}
	var absent returnFacts
	absent.append(returnFact{available: true})
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, absent, false); ok {
		t.Fatalf("absent selected return fact fabricated plan: %#v", plan)
	}
	var sparseBottom returnFacts
	sparseBottom.append(returnFact{fact: fixture.values.Bottom(), available: true})
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, sparseBottom, false); !ok || plan.class != routeScalar || plan.routeCount() != 0 {
		t.Fatalf("owner-authenticated sparse Bottom return fact = %#v/%t, want scalar no-route plan", plan, ok)
	}

	var malformed returnFacts
	malformed.append(returnFact{present: true, available: true})
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, malformed, false); ok {
		t.Fatalf("malformed Value fact fabricated plan: %#v", plan)
	}
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, malformed, true); ok {
		t.Fatalf("malformed Value fact bypassed authenticated open-tail widening: %#v", plan)
	}

	var top returnFacts
	top.append(returnFact{fact: fixture.values.Top(), present: true, available: true})
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, top, false); !ok || plan.class != routeWidened || plan.routeCount() != len(fixture.allocations) {
		t.Fatalf("authenticated Top return fact = %#v/%t", plan, ok)
	}

	opaqueAtom, opaqueOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	if !opaqueOK {
		t.Fatal("opaque reference atom")
	}
	opaque, opaqueOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueOK {
		t.Fatal("opaque reference value")
	}
	var opaqueFacts returnFacts
	opaqueFacts.append(returnFact{fact: opaque, present: true, available: true})
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, opaqueFacts, false); !ok || plan.class != routeWidened || plan.routeCount() != len(fixture.allocations) {
		t.Fatalf("authenticated opaque return fact = %#v/%t", plan, ok)
	}

	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, returnFacts{}, true); !ok || plan.class != routeWidened || plan.routeCount() != len(fixture.allocations) {
		t.Fatalf("authenticated open-tail return fact = %#v/%t", plan, ok)
	}

	foreign := newReturnPlanFixture(t)
	if plan, ok := routePlanFor(fixture.placement, fixture.values, foreign.values.Bottom()); ok {
		t.Fatalf("foreign Bottom Value fabricated plan: %#v", plan)
	}
	if plan, ok := routePlanFor(fixture.placement, fixture.values, foreign.values.Top()); ok {
		t.Fatalf("foreign Top Value fabricated plan: %#v", plan)
	}
	var foreignFacts returnFacts
	foreignFacts.append(returnFact{fact: foreign.values.Top(), present: true, available: true})
	if plan, ok := routePlanForFacts(fixture.placement, fixture.values, foreignFacts, false); ok {
		t.Fatalf("foreign Top selected Value fabricated plan: %#v", plan)
	}
}

func TestReturnPlacementDemandPreservesMonotoneDisplacement(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation value")
	}
	plan, planOK := routePlanFor(fixture.placement, fixture.values, fact)
	if !planOK || plan.class != routeExact {
		t.Fatal("exact return plan")
	}
	for _, item := range []struct {
		current placement.Fact
		want    placement.Fact
	}{
		{placement.DefaultFact(), placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.UnknownFact(), placement.Fact{Class: placement.Unknown, RetainEscape: placement.EvidenceProven}},
	} {
		got, ok := returnValue(item.current, true, plan)
		if !ok || got != item.want {
			t.Fatalf("return demand(%v) = %v/%t, want %v", item.current, got, ok, item.want)
		}
	}
	if got, ok := returnValue(placement.BottomFact(), true, plan); ok || got == placement.UnknownFact() {
		t.Fatalf("missing Bottom placement seed = %v/%t, must refuse without Unknown", got, ok)
	}
	if got, ok := returnValue(placement.DefaultFact(), false, plan); !ok || got != (placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("sparse Placement default = %v/%t, want owned-heap/proven/true", got, ok)
	}
	topPlan, topOK := routePlanFor(fixture.placement, fixture.values, fixture.values.Top())
	if !topOK || topPlan.class != routeWidened {
		t.Fatal("top return plan")
	}
	got, ok := returnValue(placement.DefaultFact(), true, topPlan)
	if !ok || got != (placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}) {
		t.Fatalf("widened return demand = %v/%t, want OwnedHeap/Proven", got, ok)
	}
	if got, ok := returnValue(placement.BottomFact(), true, topPlan); ok || got == placement.UnknownFact() {
		t.Fatalf("missing Bottom seed on widened return = %v/%t, must refuse without Unknown", got, ok)
	}
}

func newReturnPlanFixture(t testing.TB) returnPlanFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "placement_returnescape.lua", Text: []byte("local first = {}; local second = {}; local third = {}; local fourth = {}; local fifth = {}; local sixth = {}; local seventh = {}; local eighth = {}; local ninth = {}; return first")})
	if err != nil {
		t.Fatal(err)
	}
	requireOperation, requireErr := testfixture.ScopedRequireOperation()
	if requireErr != nil {
		t.Fatal(requireErr)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{requireOperation}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "placement-returnescape", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("artifact grammar")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	if !shardOK || !moduleOK || !programIDOK {
		t.Fatal("mounted module")
	}
	structural := syntheticStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered {
		t.Fatal("ingress lower")
	}
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !mountOK {
		t.Fatal("heap artifact mount")
	}
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !valueMountOK {
		t.Fatal("value artifact mount")
	}
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if heapFailure != heap.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("schema seal heap=%v value=%v", heapFailure, valueFailure)
	}
	projected, projectedOK := placement.NewSchema(heapSchema)
	if !projectedOK {
		t.Fatal("placement schema")
	}
	var returnID identity.ContentID
	programSchema := artifact.Program()
	occurrenceCount, occurrenceOK := programSchema.OccurrenceCount()
	if !occurrenceOK {
		t.Fatal("occurrence count")
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := programSchema.OccurrenceAt(index)
		if rowOK && row.Kind() == programschema.OccurrenceReturnBoundary {
			returnID = row.ID()
			break
		}
	}
	if !returnID.Available() {
		t.Fatal("return-boundary occurrence")
	}
	allocations := make([]heap.Key, 0)
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heap.RootAllocation {
			allocations = append(allocations, key)
		}
	}
	if len(allocations) == 0 {
		t.Fatal("allocation roots")
	}
	return returnPlanFixture{values: values, placement: projected, module: module, returnID: returnID, allocations: allocations}
}

// syntheticStructuralVocabulary supplies the neutral structural projection
// needed by the Value/ingress fixture without importing the composition root.
// The production test path intentionally uses a direct artifact compiler
// grammar, so returnescape's package tests cannot form a cycle when composite
// wires this package into its rule inventory.
func syntheticStructuralVocabulary(t testing.TB) structure.Table {
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
			spelling := fmt.Sprintf("returnescape/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal),
				Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("synthetic structural declarations")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("synthetic structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(emptySurface{kind: kind}) {
			t.Fatalf("synthetic surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("synthetic schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("synthetic structure view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("synthetic structure table")
	}
	return table
}

type emptySurface struct{ kind schema.SurfaceKind }

func (surface emptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface emptySurface) Entries() []schema.Entry  { return nil }
func (surface emptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// BenchmarkAllAllocationRoutesDensePass measures the one-time owner-schema
// validation/counting pass. Route materialization remains lazy after this
// setup, so the widened plan does not retain a copied root catalogue.
func BenchmarkReturnAllRootPlanDensePass(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if plan, ok := allAllocationPlan(fixture.placement); !ok || !plan.allRoot || plan.routeCount() != len(fixture.allocations) {
			b.Fatal("dense allocation route plan")
		}
	}
}

// BenchmarkReturnRoutePlanExact isolates the selected-read planner. The
// exact relation exercises direct Value atom pulls, route ordering, and alias
// deduplication without rebuilding the mounted fixture in the loop.
func BenchmarkReturnRoutePlanExact(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		b.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		b.Fatal("allocation value")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := routePlanFor(fixture.placement, fixture.values, fact)
		if !planOK || plan.class != routeExact || plan.routeCount() != 1 {
			b.Fatal("exact route plan")
		}
	}
}

// BenchmarkReturnRoutePlanExactTwo covers the bounded two-alternative route
// case. It keeps canonical insertion and Value atom pulls on the caller's
// stack just like the one-route benchmark above.
func BenchmarkReturnRoutePlanExactTwo(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	if len(fixture.allocations) < 2 {
		b.Skip("fixture has one allocation root")
	}
	first, firstOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	second, secondOK := fixture.values.Allocation(fixture.allocations[1], materialization.Recent)
	if !firstOK || !secondOK {
		b.Fatal("allocation atoms")
	}
	fact, factOK := fixture.values.Alternatives(first, second)
	if !factOK {
		b.Fatal("two-route value")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := routePlanFor(fixture.placement, fixture.values, fact)
		if !planOK || plan.class != routeExact || plan.routeCount() != 2 {
			b.Fatal("two exact routes")
		}
	}
}

// BenchmarkReturnRoutePlanExactScaling exercises an exact Value with enough
// allocation alternatives to cross the bounded inline route prefix. Only the
// suffix is heap-backed; the four-route common prefix remains in the plan.
func BenchmarkReturnRoutePlanExactScaling(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	if len(fixture.allocations) <= len((routePlan{}).inline) {
		b.Skip("fixture has no exact spill width")
	}
	atoms := make([]valuedomain.Atom, 0, len(fixture.allocations))
	for _, key := range fixture.allocations {
		atom, atomOK := fixture.values.Allocation(key, materialization.Recent)
		if !atomOK {
			b.Fatal("allocation atom")
		}
		atoms = append(atoms, atom)
	}
	fact, factOK := fixture.values.Alternatives(atoms...)
	if !factOK {
		b.Fatal("wide exact value")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := routePlanFor(fixture.placement, fixture.values, fact)
		if !planOK || plan.class != routeExact || plan.routeCount() != len(fixture.allocations) || len(plan.spill) != len(fixture.allocations)-len(plan.inline) {
			b.Fatal("wide exact route plan")
		}
	}
}

// BenchmarkReturnRoutePlanTop measures the widened return planner without
// charging a copied route catalogue. The returned plan retains only the
// immutable owner schema and dense root count.
func BenchmarkReturnRoutePlanTop(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	top := fixture.values.Top()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := routePlanFor(fixture.placement, fixture.values, top)
		if !planOK || plan.class != routeWidened || !plan.allRoot || plan.spill != nil || plan.routeCount() != len(fixture.allocations) {
			b.Fatal("top widened route plan")
		}
	}
}

// BenchmarkReturnRoutePlanOpaque measures the same lazy widened path for an
// authenticated opaque reference alternative.
func BenchmarkReturnRoutePlanOpaque(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	atom, atomOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	if !atomOK {
		b.Fatal("opaque reference atom")
	}
	opaque, opaqueOK := fixture.values.Singleton(atom)
	if !opaqueOK {
		b.Fatal("opaque reference value")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := routePlanFor(fixture.placement, fixture.values, opaque)
		if !planOK || plan.class != routeWidened || !plan.allRoot || plan.spill != nil || plan.routeCount() != len(fixture.allocations) {
			b.Fatal("opaque widened route plan")
		}
	}
}

// BenchmarkReturnRoutePlanWidenedSelectionScaling walks every route in one
// lazy widened plan. It keeps the selection side of widening visible as the
// mounted allocation denominator grows, rather than measuring only setup.
func BenchmarkReturnRoutePlanWidenedSelectionScaling(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	plan, planOK := routePlanFor(fixture.placement, fixture.values, fixture.values.Top())
	if !planOK || plan.class != routeWidened || !plan.allRoot {
		b.Fatal("top widened route plan")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for routeIndex := 0; routeIndex < plan.routeCount(); routeIndex++ {
			candidate, candidateOK := plan.routeAt(routeIndex)
			if !candidateOK {
				b.Fatal("lazy widened route")
			}
			byTag, byTagOK := routeAtTag(plan, candidate.tag)
			if !byTagOK || byTag != candidate {
				b.Fatal("lazy widened tag lookup")
			}
		}
	}
}
