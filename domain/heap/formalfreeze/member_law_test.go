package formalfreeze_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/formalfreeze"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// freezeSchema seals one real Heap schema over a module that allocates, which
// is what these laws need: the freeze judgment is a statement about owner-issued
// allocation roots and their Recent references, and a hand-built schema would
// let the laws agree with a fiction.
func freezeSchema(t testing.TB) heapdomain.Schema {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "formal_freeze.lua", Text: []byte("local first = {}; local second = {}; return first, second")})
	if err != nil {
		t.Fatal(err)
	}
	targetContract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: targetContract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK || linked.Project() == nil {
		t.Fatal("freeze artifact issuance")
	}
	projectMounts := linked.Project().Mounts()
	mounts := make([]programmount.MountedArtifact, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		mountedProgram, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !programOK || mountedProgram == nil || !moduleOK {
			t.Fatal("freeze artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(mountedProgram, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("freeze artifact compile: %v", failure)
		}
		var mountOK bool
		mounts[index], mountOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		if !mountOK {
			t.Fatal("freeze artifact mount receipt")
		}
	}
	schema, failure := heapdomain.SealWithArtifacts(linked, mounts)
	if failure != heapdomain.SealFailureNone || !schema.Valid() {
		t.Fatalf("freeze heap schema: %v", failure)
	}
	return schema
}

// freezeAllocationRoot returns one owner-issued table allocation root.
func freezeAllocationRoot(t testing.TB, schema heapdomain.Schema) heapdomain.Key {
	t.Helper()
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		_, _, _, kind, _, originOK := schema.AllocationOriginForKey(root)
		if rootOK && originOK && kind == heapdomain.AllocationTable && root.Kind() == heapdomain.RootAllocation {
			return root
		}
	}
	t.Fatal("no table allocation root")
	return heapdomain.Key{}
}

// freezeRoutePlan is the one-route plan a call justifying exactly one exact
// Recent root produces.
func freezeRoutePlan(t testing.TB, schema heapdomain.Schema, key heapdomain.Key) (recentplan.Plan, heapdomain.RawRouteTag) {
	t.Helper()
	tag, tagOK := schema.RouteTag(key, materialization.Recent)
	if !tagOK || tag == 0 {
		t.Fatal("route tag")
	}
	var plan recentplan.Plan
	if !plan.Add(recentplan.Route{Key: key, Tag: tag}) {
		t.Fatal("route plan")
	}
	return plan, tag
}

// freezePredecessor builds the live Recent relation a freeze transitions from.
func freezePredecessor(t testing.TB, schema heapdomain.Schema, key heapdomain.Key) heapdomain.Value {
	t.Helper()
	none, noneOK := schema.ContainmentNone()
	object, objectOK := schema.Object(heapdomain.ShapeEligible, heapdomain.FrozenMutable, none)
	if !noneOK || !objectOK {
		t.Fatal("freeze object")
	}
	world, worldOK := schema.One(key, object)
	relation, relationOK := schema.Relation(key, world)
	if !worldOK || !relationOK {
		t.Fatal("freeze predecessor")
	}
	return relation
}

// TestFreezeFoldPublishesTheNormalSuccessorOfASelectedRoute is the family's
// freeze semantics: at a route the call justifies, the fact published is the
// normal branch of the owner's shallow freeze of that route's predecessor. The
// fold neither issues the transition itself nor republishes the predecessor.
func TestFreezeFoldPublishesTheNormalSuccessorOfASelectedRoute(t *testing.T) {
	schema := freezeSchema(t)
	key := freezeAllocationRoot(t, schema)
	plan, tag := freezeRoutePlan(t, schema, key)
	predecessor := freezePredecessor(t, schema, key)

	fact, outcome := formalfreeze.FreezeFold(schema, plan, tag, predecessor, true)
	if outcome != structure.Concrete {
		t.Fatalf("outcome = %d, want Concrete", outcome)
	}
	reference, referenceOK := schema.Reference(key, materialization.Recent)
	if !referenceOK {
		t.Fatal("recent reference")
	}
	branches, freezeOK := schema.ShallowFreeze(predecessor, reference)
	normal, normalOK := branches.Normal(key)
	if !freezeOK || !normalOK {
		t.Fatal("owner shallow freeze")
	}
	if !heapdomain.Equal(fact, normal) {
		t.Fatal("the published fact is not the owner's normal freeze successor")
	}
	if heapdomain.Equal(fact, predecessor) {
		t.Fatal("the fold republished the predecessor instead of freezing it")
	}
}

// TestFreezeFoldPublishesBottomAtARouteWithNoPredecessor states the one case a
// routed write cannot leave unanswered. A selected route whose coordinate holds
// no predecessor fact has no normal branch to take, but the routed output must
// still settle that exact Heap target, so the empty normal image is published
// rather than a fabricated frozen object.
func TestFreezeFoldPublishesBottomAtARouteWithNoPredecessor(t *testing.T) {
	schema := freezeSchema(t)
	key := freezeAllocationRoot(t, schema)
	plan, tag := freezeRoutePlan(t, schema, key)

	fact, outcome := formalfreeze.FreezeFold(schema, plan, tag, heapdomain.Value{}, false)
	if outcome != structure.Concrete {
		t.Fatalf("outcome = %d, want Concrete", outcome)
	}
	if !heapdomain.Equal(fact, schema.Bottom()) {
		t.Fatal("an absent predecessor published something other than the empty normal image")
	}
}

// TestFreezeFoldSettlesNoSelectionOnAnEmptyPlan states where this family's
// NoSelection comes from. A call whose evidence is unresolved, open, opaque or
// ambiguous justifies no exact Recent root, so the plan is empty and valid. That
// is an explicitly empty selection over a population that exists - the call is a
// real occurrence - and it must not be reported as a refusal, as an absent
// candidate, or as a freeze of nothing.
func TestFreezeFoldSettlesNoSelectionOnAnEmptyPlan(t *testing.T) {
	schema := freezeSchema(t)
	key := freezeAllocationRoot(t, schema)
	predecessor := freezePredecessor(t, schema, key)

	for _, present := range []bool{false, true} {
		_, outcome := formalfreeze.FreezeFold(schema, recentplan.Plan{}, 0, predecessor, present)
		if outcome != structure.NoSelection {
			t.Fatalf("empty-plan outcome (present=%v) = %d, want NoSelection", present, outcome)
		}
	}
}

// TestFreezeFoldRefusesATagItsPlanDoesNotName is the plan-content law that
// survives the protocol: the plan is the whole authority over which routes this
// call justifies. A tag outside it names no route, so the fold refuses rather
// than freezing a root the formal rows never justified.
func TestFreezeFoldRefusesATagItsPlanDoesNotName(t *testing.T) {
	schema := freezeSchema(t)
	key := freezeAllocationRoot(t, schema)
	plan, tag := freezeRoutePlan(t, schema, key)
	predecessor := freezePredecessor(t, schema, key)

	if _, outcome := formalfreeze.FreezeFold(schema, plan, tag+1, predecessor, true); outcome != structure.Refuse {
		t.Fatalf("unjustified tag outcome = %d, want Refuse", outcome)
	}
	if _, outcome := formalfreeze.FreezeFold(schema, plan, 0, predecessor, true); outcome != structure.Refuse {
		t.Fatalf("absent tag outcome = %d, want Refuse", outcome)
	}
}

// TestFreezeFoldRefusesAnUnsealedSchema states that the fold reaches no owner
// authority of its own: with no sealed Heap schema there is no freeze to issue
// and no Bottom to publish, so it declines rather than answering from a zero
// value.
func TestFreezeFoldRefusesAnUnsealedSchema(t *testing.T) {
	if _, outcome := formalfreeze.FreezeFold(heapdomain.Schema{}, recentplan.Plan{}, 0, heapdomain.Value{}, false); outcome != structure.Refuse {
		t.Fatalf("unsealed schema outcome = %d, want Refuse", outcome)
	}
}
