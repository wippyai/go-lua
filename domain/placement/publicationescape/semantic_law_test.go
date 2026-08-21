package publicationescape

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	targetvocabulary "github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	schemavocabulary "github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	"github.com/wippyai/go-lua/domain/runtimekind"
	typecontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// TestPublicationEscapeOperationGateDoesNotCrossApplyCandidates proves that
// an operation-selected receipt only contributes rows for that operation.
// This is intentionally a source-gate law, before any Heap walk occurs.
func TestPublicationEscapeOperationGateDoesNotCrossApplyCandidates(t *testing.T) {
	const (
		send     targetvocabulary.Operation = 1
		callback targetvocabulary.Operation = 2
	)
	prepared := &preparedBatch{
		sources: []sourceSpec{
			{tag: 11, rowID: contentID(1), operation: send},
			{tag: 12, rowID: contentID(2), operation: callback},
		},
	}
	sources := prepared.sourcesForGate(operationGateForTest(send))
	first, firstOK := sources.at(0)
	if sources.len() != 1 || !firstOK || first.operation != send {
		t.Fatalf("selected send gate admitted cross-candidate rows: sources=%#v", sources)
	}
	if _, crossed := sources.find(sourceTag(12)); crossed {
		t.Fatal("callback candidate crossed a send-only operation gate")
	}
}

// TestPublicationEscapeOpaqueCallWidensEveryAllocationRoot proves the Top /
// open Call widening boundary. It must not guess which publication candidate ran: each
// allocation root is emitted with Unknown.
func TestPublicationEscapeOpaqueCallWidensEveryAllocationRoot(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	rule := fixture.rule()
	prepared := &preparedBatch{
		rows: []publicationRow{{id: contentID(3), requirement: placementdomain.SharedHeap, operation: 1}},
	}
	gate := operationGateForTest()
	gate.opaque = true
	routes, ok := rule.routeSet(fixture.placement, prepared, gate, factBuffer{})
	if !ok {
		t.Fatal("opaque Call route set was rejected")
	}
	allocationCount := 0
	for dense := 0; dense < fixture.placement.DenseKeyCount(); dense++ {
		key, keyOK := fixture.placement.KeyAt(dense)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			allocationCount++
		}
	}
	if routes.len() != allocationCount || allocationCount == 0 {
		t.Fatalf("opaque Call routes=%d, allocation roots=%d", routes.len(), allocationCount)
	}
	for index := 0; index < routes.len(); index++ {
		route, routeOK := routes.at(index)
		if !routeOK {
			t.Fatal("opaque Call route buffer entry missing")
		}
		if !route.unknown || route.required != placementdomain.Unknown {
			t.Fatalf("opaque Call route=%#v, want Unknown", route)
		}
	}
}

// TestPublicationEscapeExactBootHandleHasNoLocalAllocationRoute proves that
// an exact rooted handle is not automatically an allocation. Boot roots are
// existing actor-local roots and therefore contribute no local Placement
// allocation route (and no uncertainty) to this rule.
func TestPublicationEscapeExactBootHandleHasNoLocalAllocationRoute(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	if fixture.values == nil || fixture.heap.BootCount() == 0 {
		t.Skip("fixture target has no detached boot root")
	}
	rootID, rootOK := fixture.heap.BootIDAt(0)
	if !rootOK {
		t.Fatal("boot root directory omitted its first root")
	}
	atom, atomOK := fixture.values.BootID(rootID)
	if !atomOK {
		t.Fatal("Value did not issue exact boot handle")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("boot handle singleton")
	}
	roots, unknown, ok := rootsForValue(fixture.placement, fixture.values, fact)
	if !ok || unknown || roots.len() != 0 {
		t.Fatalf("exact boot handle roots=%v unknown=%t ok=%t, want no local route", roots, unknown, ok)
	}
}

func TestPublicationEscapeExactAllocationDispositionRoutes(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	if len(fixture.allocations) == 0 {
		t.Fatal("fixture has no allocation root")
	}
	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		t.Fatal("Value did not issue exact allocation handle")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation handle singleton")
	}
	rule := fixture.rule()
	for _, test := range []struct {
		name        string
		operation   targetvocabulary.Operation
		requirement placementdomain.Placement
	}{
		{name: "send", operation: 1, requirement: placementdomain.SharedHeap},
		{name: "return", operation: 2, requirement: placementdomain.OwnedHeap},
		{name: "callback", operation: 3, requirement: placementdomain.OwnedHeap},
	} {
		t.Run(test.name, func(t *testing.T) {
			rowID := contentID(byte(test.operation + 10))
			prepared := &preparedBatch{rows: []publicationRow{{id: rowID, requirement: test.requirement, operation: test.operation}}}
			routes, ok := rule.routeSet(fixture.placement, prepared, operationGateForTest(test.operation), factBufferForTest(
				map[identity.ContentID]valuedomain.Value{rowID: fact}, map[identity.ContentID]bool{rowID: true},
			))
			if !ok || routes.len() != 1 {
				t.Fatalf("exact %s routes=%#v/%t, want one route", test.name, routes, ok)
			}
			route, routeOK := routes.at(0)
			if !routeOK || route.key != fixture.allocations[0] || route.required != test.requirement || route.unknown {
				t.Fatalf("exact %s route=%#v, want root with %v", test.name, route, test.requirement)
			}
		})
	}
}

func TestPublicationEscapeSameRootSendDominatesRetain(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	if len(fixture.allocations) == 0 {
		t.Fatal("fixture has no allocation root")
	}
	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		t.Fatal("Value did not issue exact allocation handle")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		t.Fatal("allocation handle singleton")
	}
	first, second := contentID(21), contentID(22)
	rule := fixture.rule()
	routes, ok := rule.routeSet(fixture.placement, &preparedBatch{rows: []publicationRow{
		{id: first, requirement: placementdomain.OwnedHeap, operation: 2},
		{id: second, requirement: placementdomain.SharedHeap, operation: 1},
	}}, operationGateForTest(1, 2), factBufferForTest(map[identity.ContentID]valuedomain.Value{
		first: fact, second: fact,
	}, map[identity.ContentID]bool{first: true, second: true}))
	if !ok || routes.len() != 1 {
		t.Fatalf("same-root merge routes=%#v/%t, want one route", routes, ok)
	}
	route, routeOK := routes.at(0)
	if !routeOK || route.required != placementdomain.SharedHeap || route.unknown {
		t.Fatalf("same-root merge=%#v, want SharedHeap", route)
	}
}

func factBufferForTest(values map[identity.ContentID]valuedomain.Value, present map[identity.ContentID]bool) factBuffer {
	var facts factBuffer
	for rowID, value := range values {
		facts.set(factEntry{rowID: rowID, value: value, present: present[rowID]})
	}
	return facts
}

func contentID(first byte) identity.ContentID {
	var id identity.ContentID
	id[0] = first
	return id
}

type publicationEscapeFixture struct {
	heap        heapdomain.Schema
	values      *valuedomain.Schema
	placement   placementdomain.Schema
	valueOwner  *valueowner.HotOwner
	allocations []heapdomain.Key
}

func (fixture publicationEscapeFixture) rule() *HotRule {
	return &HotRule{values: fixture.valueOwner}
}

func newPublicationEscapeFixture(t testing.TB) publicationEscapeFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{
		Name: "placement-publication-escape.lua",
		Text: []byte("local first = {}; local second = {}; return first"),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics: typecontract.NewSemantics(),
		InitialRoots: []targetvocabulary.InitialRootSpec{{
			Identity: "GlobalEnvRoot",
			Shape: targetvocabulary.BootShapeSpec{
				Aggregate: targetvocabulary.BootAggregateTable,
				Value:     targetvocabulary.InitialValueSpec{Kind: targetvocabulary.InitialValueRoot, Root: "GlobalEnvRoot"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "placement-publication-escape", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("artifact grammar")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance.Directory{})
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	if !shardOK || !moduleOK || !programIDOK {
		t.Fatal("mounted module")
	}
	structural := publicationEscapeStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := heapdomain.NewArtifactMount(snapshot, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshot, module, programID)
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{mount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount}, structural)
	projected, projectedOK := placementdomain.NewSchema(heapSchema)
	if !lowered || !mountOK || !valueMountOK || heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil || !projectedOK {
		t.Fatalf("fixture lowered=%t mount=%t valueMount=%t heap=%v value=%v placement=%t", lowered, mountOK, valueMountOK, heapFailure, valueFailure, projectedOK)
	}
	allocationKeys := make([]heapdomain.Key, 0)
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			allocationKeys = append(allocationKeys, key)
		}
	}
	valueOwner := publicationEscapeValueOwner(t, values, projected)
	return publicationEscapeFixture{heap: heapSchema, values: values, placement: projected, valueOwner: valueOwner, allocations: allocationKeys}
}

func publicationEscapeValueOwner(t testing.TB, values *valuedomain.Schema, projected placementdomain.Schema) *valueowner.HotOwner {
	t.Helper()
	builder := engine.NewSchema()
	factor, factorOK := schemavocabulary.Key("factor/value")
	summary, summaryOK := schemavocabulary.Key("factor/value/summary-identity")
	fold, foldOK := schemavocabulary.Key("factor/value/summary-coordinatewise")
	valueFragment, valueFragmentOK := valueowner.DeclareSchema(builder, factor, summary, fold)
	placementSemantic, placementSemanticOK := schemavocabulary.Key("factor/placement")
	placementFold, placementFoldOK := schemavocabulary.Key("factor/placement/summary-coordinatewise")
	placementFragment, placementFragmentOK := placementowner.DeclareSchema(builder, placementSemantic, placementFold)
	sealed, sealedOK := builder.Seal()
	if !factorOK || !summaryOK || !foldOK || !valueFragmentOK || !placementSemanticOK || !placementFoldOK || !placementFragmentOK || !sealedOK {
		t.Fatal("publication escape owner schema")
	}
	binding := engine.NewSchemaBinding(sealed)
	if binding == nil {
		t.Fatal("publication escape owner binding")
	}
	placementHot, placementHotOK := placementowner.BindHot(binding, placementFragment, projected)
	valueHot, valueHotOK := valueowner.BindHot(binding, valueFragment, values)
	if !placementHotOK || !valueHotOK || placementHot.Schema().Heap().ContentID() != valueHot.Schema().Heap().ContentID() {
		t.Fatal("publication escape owner binding")
	}
	return valueHot
}

func publicationEscapeStructuralVocabulary(t testing.TB) structure.Table {
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
	entries := make([]structure.Spec, 0)
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("publication-escape/%d/%d", category, ordinal)
			entries = append(entries, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true,
			})
		}
	}
	collected, collectedOK := structure.Collect(entries)
	if !collectedOK {
		t.Fatal("publication escape structural entries")
	}
	builder := schema.NewBuilder()
	if !builder.Register(structure.NewSurface(collected)) {
		t.Fatal("publication escape structural surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(publicationEscapeEmptySurface{kind: kind}) {
			t.Fatalf("publication escape surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("publication escape structural schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("publication escape structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("publication escape structural table")
	}
	return table
}

type publicationEscapeEmptySurface struct{ kind schema.SurfaceKind }

func (surface publicationEscapeEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface publicationEscapeEmptySurface) Entries() []schema.Entry  { return nil }
func (surface publicationEscapeEmptySurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
