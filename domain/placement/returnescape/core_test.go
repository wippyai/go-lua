package returnescape

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
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
	boundary    valuedomain.ReturnBoundary
	allocations []heap.Key
}

// The route relation is DECLARED, so the laws below are stated over the three
// authored judgments and the construction the emitter writes from them. Its
// unit is the whole delivered member vector, which is what the rule is handed,
// rather than one fact at a time: a cell is not a value, and which cells the
// owner admits is half of what the set answers.
//
// One thing the authored plan carried is gone rather than restated elsewhere:
// a route class separating a Bottom relation from an ordinary scalar. It had
// no reader outside this file - a consumer of this relation sees rows - so
// what survives of it is what was ever observable, which is how many rows
// there are and in what order.

// returnCell is one present, owner-issued cell of a delivered member vector.
func returnCell(fact valuedomain.Value) execution.MemberCell[valuedomain.Value] {
	return execution.MemberCell[valuedomain.Value]{Value: fact, Present: true}
}

// returnRoutes derives one boundary's whole route set the way the emitted
// family does, over a member vector built from the given cells.
func returnRoutes(t testing.TB, fixture returnPlanFixture, cells ...execution.MemberCell[valuedomain.Value]) (derived2Rows, bool) {
	t.Helper()
	if cells == nil {
		cells = []execution.MemberCell[valuedomain.Value]{}
	}
	vector, vectorOK := execution.NewMemberVector(cells)
	if !vectorOK {
		t.Fatal("member vector")
	}
	return deriveDerived2Rows(fixture.placement, fixture.values, fixture.boundary, fixture.values.Bottom(), vector)
}

// assertReturnRoutesAscend states the order both arms leave through: ascending
// by the coordinate a route's key normalizes to, which is its tag minus one.
func assertReturnRoutesAscend(t testing.TB, plan derived2Rows) {
	t.Helper()
	previous := uint64(0)
	for index := 0; index < derived2Count(plan); index++ {
		route, routeOK := derived2At(plan, index)
		if !routeOK || route.Tag <= previous {
			t.Fatalf("route %d = %#v/%t after tag %d", index, route, routeOK, previous)
		}
		previous = route.Tag
	}
}

func TestAReturnRouteSetAnswersScalarAndExactAllocationCells(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	scalarAtom, scalarOK := fixture.values.OpaqueKind(runtimekind.Number)
	if !scalarOK {
		t.Fatal("numeric scalar atom")
	}
	scalar, scalarOK := fixture.values.Singleton(scalarAtom)
	if !scalarOK {
		t.Fatal("numeric scalar value")
	}
	plan, ok := returnRoutes(t, fixture, returnCell(scalar))
	if !ok || plan.widened || derived2Count(plan) != 0 {
		t.Fatalf("scalar return route set = %#v/%t, want no routes", plan, ok)
	}

	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		t.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation value")
	}
	plan, ok = returnRoutes(t, fixture, returnCell(fact))
	route, routeOK := derived2At(plan, 0)
	if !ok || plan.widened || derived2Count(plan) != 1 || !routeOK || route.Key != fixture.allocations[0] {
		t.Fatalf("exact allocation return route set = %#v/%t", plan, ok)
	}
}

func TestAReturnRouteSetHoldsOneRoutePerRootAcrossCellsAndAlternatives(t *testing.T) {
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
	// Two materializations of one root, in one value, are one route.
	fact, factOK := fixture.values.Alternatives(first, alias, second)
	if !factOK {
		t.Fatal("alternate allocation value")
	}
	plan, ok := returnRoutes(t, fixture, returnCell(fact))
	if !ok || plan.widened || derived2Count(plan) != 2 {
		t.Fatalf("alternate return route set = %#v/%t", plan, ok)
	}
	assertReturnRoutesAscend(t, plan)

	// And two CELLS naming one root are one route too: the set is one member
	// per address however many cells reached it.
	firstOnly, firstOnlyOK := fixture.values.Singleton(first)
	aliasOnly, aliasOnlyOK := fixture.values.Singleton(alias)
	if !firstOnlyOK || !aliasOnlyOK {
		t.Fatal("singleton alternatives")
	}
	across, acrossOK := returnRoutes(t, fixture, returnCell(firstOnly), returnCell(aliasOnly))
	if !acrossOK || across.widened || derived2Count(across) != 1 {
		t.Fatalf("route set across cells = %#v/%t, want one route", across, acrossOK)
	}
}

func TestAReturnRouteSetWidensToEveryAllocationRootAtItsEndpoint(t *testing.T) {
	fixture := newReturnPlanFixture(t)
	plan, ok := returnRoutes(t, fixture, returnCell(fixture.values.Top()))
	if !ok || !plan.widened || derived2Count(plan) != len(fixture.allocations) {
		t.Fatalf("top return route set = %#v/%t", plan, ok)
	}
	assertReturnRoutesAscend(t, plan)
	for index := 0; index < derived2Count(plan); index++ {
		route, routeOK := derived2At(plan, index)
		if !routeOK || route.Key.Kind() != heap.RootAllocation {
			t.Fatalf("widened route %d = %#v/%t", index, route, routeOK)
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
	plan, ok = returnRoutes(t, fixture, returnCell(opaque))
	if !ok || !plan.widened || derived2Count(plan) != len(fixture.allocations) {
		t.Fatalf("opaque return route set = %#v/%t", plan, ok)
	}
}

// TestAReturnRouteSetAdmitsOnlyTheCellsItsOwnerIssued is the cell law, and it
// is why the delivery's source level names a judgment at all.
//
// A cell is not a value. The only absent cell an owner admits is its own
// sparse Bottom default; presence metadata never manufactures one, and a
// foreign, zero, or non-Bottom sparse cell is refused rather than enumerated
// as a fact. Because the endpoint runs whether or not the vector yields
// anything, that refusal reaches an empty delivery too.
func TestAReturnRouteSetAdmitsOnlyTheCellsItsOwnerIssued(t *testing.T) {
	fixture := newReturnPlanFixture(t)

	if plan, ok := returnRoutes(t, fixture, execution.MemberCell[valuedomain.Value]{}); ok {
		t.Fatalf("absent selected return cell fabricated a route set: %#v", plan)
	}
	if plan, ok := returnRoutes(t, fixture, execution.MemberCell[valuedomain.Value]{Present: true}); ok {
		t.Fatalf("malformed Value cell fabricated a route set: %#v", plan)
	}
	sparseBottom, sparseOK := returnRoutes(t, fixture, execution.MemberCell[valuedomain.Value]{Value: fixture.values.Bottom()})
	if !sparseOK || sparseBottom.widened || derived2Count(sparseBottom) != 0 {
		t.Fatalf("owner-authenticated sparse Bottom cell = %#v/%t, want a valid empty route set", sparseBottom, sparseOK)
	}

	foreign := newReturnPlanFixture(t)
	if plan, ok := returnRoutes(t, fixture, returnCell(foreign.values.Bottom())); ok {
		t.Fatalf("foreign Bottom Value fabricated a route set: %#v", plan)
	}
	if plan, ok := returnRoutes(t, fixture, returnCell(foreign.values.Top())); ok {
		t.Fatalf("foreign Top Value fabricated a route set: %#v", plan)
	}
	if _, ok := deriveDerived2Rows(fixture.placement, fixture.values, valuedomain.ReturnBoundary{}, fixture.values.Bottom(), mustMemberVector(t)); ok {
		t.Fatal("a forged ReturnBoundary candidate derived a route set")
	}
	empty, emptyOK := returnRoutes(t, fixture)
	if !emptyOK || empty.widened || derived2Count(empty) != 0 {
		t.Fatalf("empty delivery = %#v/%t, want a valid empty route set", empty, emptyOK)
	}
}

// mustMemberVector builds an empty delivery, which is the one a forged
// candidate must still be refused against.
func mustMemberVector(t testing.TB) execution.SummaryVector[valuedomain.Value] {
	t.Helper()
	vector, vectorOK := execution.NewMemberVector([]execution.MemberCell[valuedomain.Value]{})
	if !vectorOK {
		t.Fatal("member vector")
	}
	return vector
}

func TestReturnPlacementDemandPreservesMonotoneDisplacement(t *testing.T) {
	for _, item := range []struct {
		current placement.Fact
		want    placement.Fact
	}{
		{placement.DefaultFact(), placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted}, placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}},
		{placement.UnknownFact(), placement.Fact{Class: placement.Unknown, RetainEscape: placement.EvidenceProven}},
	} {
		got, outcome := ReturnEscapeFold(1, item.current)
		if outcome != structure.Concrete || got != item.want {
			t.Fatalf("return demand(%v) = %v/%v, want %v", item.current, got, outcome, item.want)
		}
	}
	// A missing predecessor refuses rather than fabricating Unknown, and a
	// zero tag is not a route row whatever the predecessor holds.
	if got, outcome := ReturnEscapeFold(1, placement.BottomFact()); outcome == structure.Concrete || got == placement.UnknownFact() {
		t.Fatalf("missing Bottom placement seed = %v/%v, must refuse without Unknown", got, outcome)
	}
	if got, outcome := ReturnEscapeFold(0, placement.DefaultFact()); outcome == structure.Concrete || got == placement.UnknownFact() {
		t.Fatalf("zero route tag = %v/%v, must refuse without Unknown", got, outcome)
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
	boundary, boundaryOK := values.ReturnBoundary(module, returnID)
	if !boundaryOK {
		t.Fatal("return boundary")
	}
	return returnPlanFixture{
		values: values, placement: projected, module: module, returnID: returnID,
		boundary: boundary, allocations: allocations,
	}
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
// BenchmarkReturnRouteSetWidened measures the widened arm, which reads the
// owner's directory where it lies and copies nothing.
func BenchmarkReturnRouteSetWidened(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	cells := []execution.MemberCell[valuedomain.Value]{returnCell(fixture.values.Top())}
	vector, vectorOK := execution.NewMemberVector(cells)
	if !vectorOK {
		b.Fatal("member vector")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := deriveDerived2Rows(fixture.placement, fixture.values, fixture.boundary, fixture.values.Bottom(), vector)
		if !planOK || !plan.widened || derived2Count(plan) != len(fixture.allocations) {
			b.Fatal("widened return route set")
		}
	}
}

// BenchmarkReturnRouteSetExact measures the ordinary arm at one route, which
// is the answer a return usually has and the one held entirely by value.
func BenchmarkReturnRouteSetExact(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		b.Fatal("allocation atom")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		b.Fatal("allocation value")
	}
	vector, vectorOK := execution.NewMemberVector([]execution.MemberCell[valuedomain.Value]{returnCell(fact)})
	if !vectorOK {
		b.Fatal("member vector")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := deriveDerived2Rows(fixture.placement, fixture.values, fixture.boundary, fixture.values.Bottom(), vector)
		if !planOK || derived2Count(plan) != 1 {
			b.Fatal("exact return route set")
		}
	}
}

// BenchmarkReturnRouteSetExactScaling walks the inline prefix into the spill,
// which is where a wide exact answer stops being free.
func BenchmarkReturnRouteSetExactScaling(b *testing.B) {
	fixture := newReturnPlanFixture(b)
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
		b.Fatal("alternate allocation value")
	}
	vector, vectorOK := execution.NewMemberVector([]execution.MemberCell[valuedomain.Value]{returnCell(fact)})
	if !vectorOK {
		b.Fatal("member vector")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		plan, planOK := deriveDerived2Rows(fixture.placement, fixture.values, fixture.boundary, fixture.values.Bottom(), vector)
		if !planOK || derived2Count(plan) != len(fixture.allocations) {
			b.Fatal("wide exact return route set")
		}
	}
}

// BenchmarkReturnRouteSetWidenedSelectionScaling reads every member of a
// widened set. Nothing was copied when the set was built, so this is where the
// owner's directory is actually walked.
func BenchmarkReturnRouteSetWidenedSelectionScaling(b *testing.B) {
	fixture := newReturnPlanFixture(b)
	vector, vectorOK := execution.NewMemberVector([]execution.MemberCell[valuedomain.Value]{returnCell(fixture.values.Top())})
	if !vectorOK {
		b.Fatal("member vector")
	}
	plan, planOK := deriveDerived2Rows(fixture.placement, fixture.values, fixture.boundary, fixture.values.Bottom(), vector)
	if !planOK || !plan.widened {
		b.Fatal("widened return route set")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		for routeIndex := 0; routeIndex < derived2Count(plan); routeIndex++ {
			route, routeOK := derived2At(plan, routeIndex)
			if !routeOK || route.Tag == 0 {
				b.Fatal("widened route member")
			}
		}
	}
}
