package heap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

const formalFreezeSource = `
local first = {}
local second = {}
return first, second
`

// formalFreezeRoot returns one owner-issued table allocation root of a sealed
// schema. The freeze judgment is a statement about owner-issued roots and their
// Recent references, so the laws below decide over real ones.
func formalFreezeRoot(t testing.TB, schema heapdomain.Schema) heapdomain.Key {
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

// formalFreezePredecessor builds the live Recent relation a freeze transitions
// from.
func formalFreezePredecessor(t testing.TB, schema heapdomain.Schema, key heapdomain.Key) heapdomain.Value {
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

// TestFormalFreezeFactPublishesTheNormalSuccessorOfItsRoute is the family's
// freeze semantics: at the route it is handed, the fact published is the normal
// branch of the owner's shallow freeze of that route's predecessor. The fold
// neither issues the transition itself nor republishes the predecessor.
func TestFormalFreezeFactPublishesTheNormalSuccessorOfItsRoute(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "formal_freeze_fact", formalFreezeSource, nil)
	key := formalFreezeRoot(t, schema)
	predecessor := formalFreezePredecessor(t, schema, key)

	fact, outcome := heapdomain.FormalFreezeFact(key, predecessor)
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

// TestFormalFreezeFactPublishesBottomWhenTheFreezeIssuesNoNormalBranch states
// the one case a routed write cannot leave unanswered. A route whose
// predecessor is the empty world has no normal branch to take, but the routed
// output must still settle that exact Heap target, so the empty normal image is
// published rather than a fabricated frozen object.
func TestFormalFreezeFactPublishesBottomWhenTheFreezeIssuesNoNormalBranch(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "formal_freeze_bottom", formalFreezeSource, nil)
	key := formalFreezeRoot(t, schema)

	zero, zeroOK := schema.Zero(key)
	unallocated, unallocatedOK := schema.Relation(key, zero)
	if !zeroOK || !unallocatedOK || heapdomain.Equal(unallocated, schema.Bottom()) {
		t.Fatal("a predecessor holding only the zero world")
	}
	for _, predecessor := range []heapdomain.Value{schema.Bottom(), unallocated} {
		fact, outcome := heapdomain.FormalFreezeFact(key, predecessor)
		if outcome != structure.Concrete {
			t.Fatalf("outcome = %d, want Concrete", outcome)
		}
		if !heapdomain.Equal(fact, schema.Bottom()) {
			t.Fatal("a predecessor with no normal branch published something other than the empty normal image")
		}
	}
}

// TestFormalFreezeFactDeclaresAbsenceToBeBottom is the freeze judgment's
// statement about an unwritten route coordinate, and it is why this fold takes
// no presence bit.
//
// The Heap Factor's declared default is Bottom, so a route the Factor has not
// written reaches a fold under the default sparse clause as Bottom, and the
// answer at Bottom is the empty normal image: freezing Bottom is admitted by the
// owner but issues no normal branch. Absence is therefore not a distinction this
// axis publishes, and a fold carrying a presence bit would be carrying evidence
// provenance it has no judgment for.
//
// The law is stated over the judgment, not over a read: a change that makes an
// absent coordinate and a Bottom predecessor disagree is a change to what a
// freeze publishes and has to be argued as one. It cannot be introduced by
// re-declaring the route read's sparse clause, because no parameter remains
// through which that clause could reach this fold.
func TestFormalFreezeFactDeclaresAbsenceToBeBottom(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "formal_freeze_absence", formalFreezeSource, nil)
	key := formalFreezeRoot(t, schema)

	if !heapdomain.Equal(schema.Default(), schema.Bottom()) {
		t.Fatal("the Heap Factor default an unwritten route is delivered as is not Bottom")
	}
	fact, outcome := heapdomain.FormalFreezeFact(key, schema.Default())
	if outcome != structure.Concrete {
		t.Fatalf("default predecessor outcome = %d, want Concrete", outcome)
	}
	if !heapdomain.Equal(fact, schema.Bottom()) {
		t.Fatal("the fact published at an unwritten route is not the empty normal image")
	}
}

// TestFormalFreezeFactSettlesNoSelectionOnTheZeroRoute states where this
// family's NoSelection comes from. A route form makes exactly one invocation
// over an empty route set, with the zero candidate and the Factor's default
// inputs, and that invocation is where the fold says the call is a real
// occurrence justifying no exact Recent root. It must not be reported as a
// refusal, as an absent candidate, or as a freeze of nothing.
func TestFormalFreezeFactSettlesNoSelectionOnTheZeroRoute(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "formal_freeze_no_selection", formalFreezeSource, nil)
	key := formalFreezeRoot(t, schema)
	predecessor := formalFreezePredecessor(t, schema, key)

	for _, input := range []heapdomain.Value{{}, schema.Default(), schema.Bottom(), predecessor} {
		if _, outcome := heapdomain.FormalFreezeFact(heapdomain.Key{}, input); outcome != structure.NoSelection {
			t.Fatalf("zero-route outcome = %d, want NoSelection", outcome)
		}
	}
}

// TestFormalFreezeFactRefusesACoordinateItsPredecessorDoesNotBelongTo states
// that the fold reaches no owner authority of its own. Its candidate is the
// whole proof of which schema it decides in, so a coordinate of another schema,
// or a root that has no Recent reference to freeze, has no transition this fold
// may issue - and a refusal is not the zero route's NoSelection, which is an
// answer about a population rather than about a malformed candidate.
func TestFormalFreezeFactRefusesACoordinateItsPredecessorDoesNotBelongTo(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "formal_freeze_refusal", formalFreezeSource, nil)
	key := formalFreezeRoot(t, schema)
	predecessor := formalFreezePredecessor(t, schema, key)

	_, foreign, _ := compactHeapFixture(t, "formal_freeze_foreign", formalFreezeSource, nil)
	foreignKey := formalFreezeRoot(t, foreign)
	if _, outcome := heapdomain.FormalFreezeFact(foreignKey, predecessor); outcome != structure.Refuse {
		t.Fatal("a foreign coordinate concluded something other than Refuse over a local world")
	}
	for index := 0; index < schema.KeyCount(); index++ {
		candidate, candidateOK := schema.KeyAt(index)
		if !candidateOK || candidate.Kind() == heapdomain.RootAllocation {
			continue
		}
		if _, referenceOK := schema.Reference(candidate, materialization.Recent); referenceOK {
			continue
		}
		if _, outcome := heapdomain.FormalFreezeFact(candidate, predecessor); outcome != structure.Refuse {
			t.Fatal("a root with no Recent reference concluded something other than Refuse")
		}
	}
}
