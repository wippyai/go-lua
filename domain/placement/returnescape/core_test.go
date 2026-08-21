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
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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
	if !ok || plan.class != routeScalar || len(plan.routes) != 0 {
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
	if !ok || plan.class != routeExact || len(plan.routes) != 1 || plan.routes[0].key != fixture.allocations[0] {
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
	if !ok || plan.class != routeExact || len(plan.routes) != 2 {
		t.Fatalf("alternate return plan = %#v/%t", plan, ok)
	}
	firstIndex, firstIndexOK := fixture.placement.Heap().KeyIndex(plan.routes[0].key)
	secondIndex, secondIndexOK := fixture.placement.Heap().KeyIndex(plan.routes[1].key)
	if !firstIndexOK || !secondIndexOK || firstIndex >= secondIndex {
		t.Fatalf("alternate routes are not Heap ordered: %d/%d", firstIndex, secondIndex)
	}
}

func TestReturnRoutePlanTopAndOpaqueWidenEveryAllocationRoot(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	for name, fact := range map[string]valuedomain.Value{
		"top": fixture.values.Top(),
	} {
		plan, ok := routePlanFor(fixture.placement, fixture.values, fact)
		if !ok || plan.class != routeWidened || len(plan.routes) != len(fixture.allocations) {
			t.Fatalf("%s return plan = %#v/%t", name, plan, ok)
		}
	}
	opaqueAtom, opaqueOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	if !opaqueOK {
		t.Fatal("opaque reference atom")
	}
	opaque, opaqueOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueOK {
		t.Fatal("opaque reference value")
	}
	plan, ok := routePlanFor(fixture.placement, fixture.values, opaque)
	if !ok || plan.class != routeWidened || len(plan.routes) != len(fixture.allocations) {
		t.Fatalf("opaque return plan = %#v/%t", plan, ok)
	}
}

func TestReturnPlacementDemandPreservesMoreConservativeJoinBranches(t *testing.T) {
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
	cases := []struct {
		current placement.Placement
		want    placement.Placement
	}{
		{placement.Stack, placement.OwnedHeap},
		{placement.OwnedHeap, placement.OwnedHeap},
		{placement.SharedHeap, placement.SharedHeap},
		{placement.Unknown, placement.Unknown},
	}
	for _, item := range cases {
		got, ok := returnValue(item.current, true, plan)
		if !ok || got != item.want {
			t.Fatalf("return demand(%s) = %s/%t, want %s", item.current, got, ok, item.want)
		}
	}
	topPlan, topOK := routePlanFor(fixture.placement, fixture.values, fixture.values.Top())
	if !topOK || topPlan.class != routeWidened {
		t.Fatal("top return plan")
	}
	got, ok := returnValue(placement.Stack, true, topPlan)
	if !ok || got != placement.Unknown {
		t.Fatalf("widened return demand = %s/%t, want unknown", got, ok)
	}
}

func TestReturnBoundaryOperandUsesCanonicalValueRow(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	local, localOK := returnOperandForSchema(fixture.values, fixture.module, fixture.returnID)
	if !localOK {
		t.Fatal("canonical Value return-boundary row was not resolved")
	}
	if _, _, contentOK := returnOperandContentForSchema(fixture.values, local); !contentOK {
		t.Fatal("canonical Value return-boundary row crossed its owner fence")
	}
	if _, ok := returnOperandForSchema(fixture.values, identity.ContentID{}, fixture.returnID); ok {
		t.Fatal("unavailable module crossed return-boundary fence")
	}
	foreign := identity.ContentID{}
	foreign[0] = 1
	if _, ok := returnOperandForSchema(fixture.values, foreign, fixture.returnID); ok {
		t.Fatal("foreign module crossed return-boundary fence")
	}
}

func newReturnPlanFixture(t testing.TB) returnPlanFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "placement_returnescape.lua", Text: []byte("local first = {}; local second = {}; return first")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "placement-returnescape", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewGrammarIdentity(identity.ContentID{1}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("artifact grammar")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, artifactcompiler.IssuanceDirectory{})
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	if !shardOK || !moduleOK || !programIDOK {
		t.Fatal("mounted module")
	}
	structural := syntheticStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered {
		t.Fatal("ingress lower")
	}
	mount, mountOK := heap.NewArtifactMount(snapshot, module, programID)
	if !mountOK {
		t.Fatal("heap artifact mount")
	}
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshot, module, programID)
	if !valueMountOK {
		t.Fatal("value artifact mount")
	}
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []heap.ArtifactMount{mount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount}, structural)
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
		case structure.CategoryIssuanceForm:
			return 5
		case structure.CategoryIssuanceInput:
			return 4
		case structure.CategoryIssuanceStage:
			return 5
		case structure.CategoryIssuanceRequirement:
			return 2
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
	builder := schema.NewBuilder()
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
func (surface emptySurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// BenchmarkAllAllocationRoutesDensePass keeps the widened cold enumeration on
// the direct KeyAt path. AllocationAt is intentionally not part of this hot
// benchmark: its allocation-only ordinal API rescans the Heap prefix.
func BenchmarkAllAllocationRoutesDensePass(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if routes, ok := allAllocationRoutes(fixture.placement); !ok || len(routes) != len(fixture.allocations) {
			b.Fatal("dense allocation route enumeration")
		}
	}
}
