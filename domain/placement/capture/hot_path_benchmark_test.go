package capture

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
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/call/calltest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

type captureRoutePlanBenchmarkFixture struct {
	placement placementdomain.Schema
	values    *valuedomain.Schema
	facts     []SourceFact
}

var (
	captureHotPlanBenchmarkResult RoutePlan
	captureHotPlanBenchmarkOK     bool
)

// BenchmarkCaptureRoutePlanForFacts measures the complete selected-read
// planner, including Value atom authentication and route union. The common
// one-to-eight-root widths stay allocation-free; width nine takes the
// explicit route spill.
func BenchmarkCaptureRoutePlanForFacts(b *testing.B) {
	fixture := newCaptureRoutePlanBenchmarkFixture(b)
	for _, width := range []int{1, 4, captureRouteInlineCapacity, captureRouteInlineCapacity + 1} {
		width := width
		b.Run(fmt.Sprintf("%d", width), func(b *testing.B) {
			facts := fixture.facts[:width]
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				captureHotPlanBenchmarkResult, captureHotPlanBenchmarkOK = DeriveCaptureRoutes(fixture.placement, fixture.values, facts)
			}
			if !captureHotPlanBenchmarkOK || captureHotPlanBenchmarkResult.RouteCount() != width {
				b.Fatalf("capture Route plan width %d = %d/%t", width, captureHotPlanBenchmarkResult.RouteCount(), captureHotPlanBenchmarkOK)
			}
		})
	}
}

// BenchmarkCaptureWidenedRoutePlanForFacts proves that authenticated Top and
// opaque facts retain only the owner schema view. Widening remains
// allocation-free even when the Heap has many allocation roots.
func BenchmarkCaptureWidenedRoutePlanForFacts(b *testing.B) {
	fixture := newCaptureRoutePlanBenchmarkFixture(b)
	opaqueAtom, opaqueAtomOK := fixture.values.OpaqueReference(valuedomain.ReferenceTable)
	opaque, opaqueOK := fixture.values.Singleton(opaqueAtom)
	if !opaqueAtomOK || !opaqueOK {
		b.Fatal("opaque Value fact")
	}
	for _, item := range []struct {
		name string
		fact valuedomain.Value
	}{
		{name: "top", fact: fixture.values.Top()},
		{name: "opaque", fact: opaque},
	} {
		item := item
		b.Run(item.name, func(b *testing.B) {
			facts := []SourceFact{{fact: item.fact, present: true, available: true}}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				captureHotPlanBenchmarkResult, captureHotPlanBenchmarkOK = DeriveCaptureRoutes(fixture.placement, fixture.values, facts)
			}
			if !captureHotPlanBenchmarkOK || !captureHotPlanBenchmarkResult.allRoot || captureHotPlanBenchmarkResult.RouteCount() != len(fixture.facts) {
				b.Fatalf("widened %s plan = all-root %t/count %d, want true/%d", item.name, captureHotPlanBenchmarkResult.allRoot, captureHotPlanBenchmarkResult.RouteCount(), len(fixture.facts))
			}
		})
	}
}

func newCaptureRoutePlanBenchmarkFixture(t testing.TB) captureRoutePlanBenchmarkFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{
		Name: "placement_capture_hot_path.lua",
		Text: []byte("local first = {}; local second = {}; local third = {}; local fourth = {}; local fifth = {}; local sixth = {}; local seventh = {}; local eighth = {}; local ninth = {}; return first"),
	})
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
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "placement-capture-hot-path", Program: program}}})
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
	structural := captureSyntheticStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered {
		t.Fatal("ingress lower")
	}
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !heapMountOK || !valueMountOK {
		t.Fatal("artifact mounts")
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("schema seal heap=%v value=%v", heapFailure, valueFailure)
	}
	placement, placementOK := placementdomain.NewSchema(heapSchema)
	if !placementOK {
		t.Fatal("placement schema")
	}
	var facts []SourceFact
	for index := 0; index < heapSchema.KeyCount(); index++ {
		key, keyOK := heapSchema.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		atom, atomOK := values.Allocation(key, materialization.Recent)
		fact, factOK := values.Singleton(atom)
		if !atomOK || !factOK {
			t.Fatal("allocation Value fact")
		}
		facts = append(facts, SourceFact{fact: fact, present: true, available: true})
	}
	if len(facts) < captureRouteInlineCapacity+1 {
		t.Fatalf("capture fixture allocation roots = %d, want at least %d", len(facts), captureRouteInlineCapacity+1)
	}
	return captureRoutePlanBenchmarkFixture{placement: placement, values: values, facts: facts}
}

func captureSyntheticStructuralVocabulary(t testing.TB) structure.Table {
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
			spelling := fmt.Sprintf("capture/%d/%d", category, ordinal)
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
		if !builder.Register(captureEmptySurface{kind: kind}) {
			t.Fatalf("synthetic surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("synthetic structure: %v", failure)
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

type captureEmptySurface struct{ kind schema.SurfaceKind }

func (surface captureEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface captureEmptySurface) Entries() []schema.Entry  { return nil }
func (surface captureEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
