package formal

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestRoutePlanSealIsInvariantToUnorderedDemands(t *testing.T) {
	schema := routePlanFixtureSchema(t, 16)
	keys := routePlanAllocationKeys(t, schema)
	if len(keys) < 4 {
		t.Fatalf("fixture allocation roots = %d, want at least four", len(keys))
	}

	// Feed the same demand multiset in opposite orders. Repeated roots must
	// join by the stronger escape and unknown must remain a monotone widening
	// bit; neither result may depend on map insertion or iteration order.
	inputs := []struct {
		keyIndex int
		escape   placement.Escape
		unknown  bool
	}{
		{keyIndex: 0, escape: placement.Retain},
		{keyIndex: 2, escape: placement.Send},
		{keyIndex: 0, escape: placement.Store},
		{keyIndex: 3, escape: placement.Opaque, unknown: true},
		{keyIndex: 2, escape: placement.Export},
		{keyIndex: 3, escape: placement.Opaque, unknown: true},
	}
	left := make(map[heap.Key]routeDemand, len(inputs))
	right := make(map[heap.Key]routeDemand, len(inputs))
	for _, input := range inputs {
		if !planAddDemand(schema, keys[input.keyIndex], input.escape, input.unknown, left) {
			t.Fatal("left demand admission")
		}
	}
	for index := len(inputs) - 1; index >= 0; index-- {
		input := inputs[index]
		if !planAddDemand(schema, keys[input.keyIndex], input.escape, input.unknown, right) {
			t.Fatal("right demand admission")
		}
	}
	leftPlan, leftOK := (&routePlan{}).seal(schema, left)
	rightPlan, rightOK := (&routePlan{}).seal(schema, right)
	if !leftOK || !rightOK {
		t.Fatalf("sealed unordered plans = %t/%t", leftOK, rightOK)
	}
	if !reflect.DeepEqual(leftPlan.routes, rightPlan.routes) {
		t.Fatalf("unordered demand plans differ:\nleft=%#v\nright=%#v", leftPlan.routes, rightPlan.routes)
	}

	if len(leftPlan.routes) != 3 {
		t.Fatalf("deduplicated route count = %d, want 3", len(leftPlan.routes))
	}
	wantEscapes := map[int]placement.Escape{0: placement.Store, 2: placement.Export}
	previousDense := -1
	for _, route := range leftPlan.routes {
		dense, denseOK := schema.Heap().KeyIndex(route.key)
		if !denseOK || dense <= previousDense {
			t.Fatalf("routes are not in dense Heap order: dense=%d previous=%d", dense, previousDense)
		}
		previousDense = dense
		if route.unknown {
			if route.key != keys[3] {
				t.Fatalf("unknown route key = %v, want dense root %v", route.key, keys[3])
			}
			continue
		}
		keyIndex := -1
		for index, key := range keys {
			if key == route.key {
				keyIndex = index
				break
			}
		}
		if want := wantEscapes[keyIndex]; route.escape != want {
			t.Fatalf("route root %d escape = %v, want %v", keyIndex, route.escape, want)
		}
	}
}

func TestRoutePlanSealRejectsForeignDemandKey(t *testing.T) {
	local := routePlanFixtureSchema(t, 1)
	foreign := routePlanFixtureSchema(t, 1)
	localKeys := routePlanAllocationKeys(t, local)
	foreignKeys := routePlanAllocationKeys(t, foreign)
	if len(localKeys) == 0 || len(foreignKeys) == 0 {
		t.Fatal("route-plan fixture omitted allocation roots")
	}
	demands := map[heap.Key]routeDemand{
		localKeys[0]:   {escape: placement.Retain},
		foreignKeys[0]: {escape: placement.Send},
	}
	if _, ok := (&routePlan{}).seal(local, demands); ok {
		t.Fatal("sealed a demand key owned by a foreign Heap schema")
	}
}

func TestRoutePlanSealSentinelDoesNotBecomeARoute(t *testing.T) {
	schema := routePlanFixtureSchema(t, 2)
	keys := routePlanAllocationKeys(t, schema)
	if len(keys) < 2 {
		t.Fatal("route-plan fixture omitted allocation roots")
	}
	withoutSentinel := map[heap.Key]routeDemand{
		keys[0]: {escape: placement.Retain},
		keys[1]: {escape: placement.Send},
	}
	withSentinel := map[heap.Key]routeDemand{
		keys[0]:    {escape: placement.Retain},
		keys[1]:    {escape: placement.Send},
		heap.Key{}: {unknown: true},
	}
	left, leftOK := (&routePlan{}).seal(schema, withoutSentinel)
	right, rightOK := (&routePlan{}).seal(schema, withSentinel)
	if !leftOK || !rightOK || !reflect.DeepEqual(left.routes, right.routes) {
		t.Fatalf("all-root sentinel changed sealed routes: %t/%t left=%#v right=%#v", leftOK, rightOK, left.routes, right.routes)
	}
}

func routePlanAllocationKeys(t testing.TB, schema placement.Schema) []heap.Key {
	t.Helper()
	keys := make([]heap.Key, 0, schema.DenseKeyCount())
	for dense := 0; dense < schema.DenseKeyCount(); dense++ {
		key, keyOK := schema.KeyAt(dense)
		if !keyOK {
			t.Fatalf("allocation root dense coordinate %d", dense)
		}
		if key.Kind() == heap.RootAllocation {
			keys = append(keys, key)
		}
	}
	return keys
}

// routePlanFixtureSchema builds a real owner-fenced Placement/Heap schema
// with one outer table and width nested allocation roots. The fixture is
// intentionally made before benchmark timing so sealing measures only the
// ephemeral demand reduction and dense route emission.
func routePlanFixtureSchema(t testing.TB, width int) placement.Schema {
	t.Helper()
	var source strings.Builder
	source.Grow(width * 12)
	source.WriteString("return {")
	for index := 0; index < width; index++ {
		if index > 0 {
			source.WriteByte(',')
		}
		fmt.Fprintf(&source, "[%d] = {}", index+1)
	}
	source.WriteString("}")
	program, err := lower.Lower(lower.Source{Name: fmt.Sprintf("route-plan-%d.lua", width), Text: []byte(source.String())})
	if err != nil {
		t.Fatal(err)
	}
	target, err := compiler.Seal(&declaration.Spec{
		Semantics: typecontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: fmt.Sprintf("route-plan-%d", width), Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance.Directory{})
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural := formalSoundnessStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := heap.NewArtifactMount(snapshot, module, programID)
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []heap.ArtifactMount{mount})
	placementSchema, placementOK := placement.NewSchema(heapSchema)
	if !grammarOK || failure.Available() || artifact == nil || !lowered || !shardOK || !moduleOK || !programIDOK || !mountOK || heapFailure != heap.SealFailureNone || !placementOK {
		t.Fatalf("fixture grammar=%t artifact=%t lowered=%t shard=%t module=%t program=%t mount=%t heap=%v placement=%t", grammarOK, artifact != nil, lowered, shardOK, moduleOK, programIDOK, mountOK, heapFailure, placementOK)
	}
	return placementSchema
}
