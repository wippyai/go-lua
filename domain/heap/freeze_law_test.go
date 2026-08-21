package heap

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/domain/materialization"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestShallowFreezeOneAndManyLaws(t *testing.T) {
	schema := minimalFreezeSchema(t, "freeze-one-many")
	key, keyOK := schema.KeyAt(0)
	wrongKey, wrongKeyOK := schema.KeyAt(1)
	recentReference, recentReferenceOK := schema.Reference(key, materialization.Recent)
	summaryReference, summaryReferenceOK := schema.Reference(key, materialization.Summary)
	none, noneOK := schema.ContainmentNone()
	recent, recentOK := schema.Object(ShapeEligible, FrozenMutable, none)
	summary, summaryOK := schema.Object(ShapeIneligible, FrozenMutable, none)
	if !keyOK || !wrongKeyOK || !recentReferenceOK || !summaryReferenceOK || !noneOK || !recentOK || !summaryOK {
		t.Fatal("minimal freeze constructor")
	}

	oneWorld, oneWorldOK := schema.One(key, recent)
	manyWorld, manyWorldOK := schema.Many(key, recent, summary)
	one, oneOK := schema.Relation(key, oneWorld)
	many, manyOK := schema.Relation(key, manyWorld)
	if !oneWorldOK || !manyWorldOK || !oneOK || !manyOK {
		t.Fatal("minimal freeze relations")
	}

	for _, test := range []struct {
		name        string
		value       Value
		wantMany    bool
		wantRecent  Object
		wantSummary Object
	}{
		{name: "one", value: one, wantRecent: recent},
		{name: "many", value: many, wantMany: true, wantRecent: recent, wantSummary: summary},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, beforeOK := schema.Fingerprint(test.value)
			branches, freezeOK := schema.ShallowFreeze(test.value, recentReference)
			normal, normalOK := branches.Normal(key)
			if !beforeOK || !freezeOK || !normalOK {
				t.Fatal("ShallowFreeze did not issue a normal successor")
			}
			if _, wrongCoordinateOK := branches.Normal(wrongKey); wrongCoordinateOK {
				t.Fatal("normal successor accepted a wrong allocation coordinate")
			}

			world, worldOK := normal.WorldAt(0)
			frozenRecent, frozenRecentOK := world.Recent()
			if !worldOK || !frozenRecentOK {
				t.Fatal("frozen successor lost its Recent world")
			}
			shape, frozen, headerOK := frozenRecent.Header()
			wantShape, wantFrozen, wantHeaderOK := test.wantRecent.Header()
			if !headerOK || !wantHeaderOK || shape != wantShape || wantFrozen != FrozenMutable || frozen != FrozenFrozen {
				t.Fatal("ShallowFreeze did not freeze the Recent object in place")
			}

			if test.wantMany {
				gotSummary, summaryOK := world.Summary()
				if !summaryOK || !reflect.DeepEqual(gotSummary, test.wantSummary) {
					t.Fatal("ShallowFreeze changed the Many Summary object")
				}
			}
			after, afterOK := schema.Fingerprint(test.value)
			if !afterOK || after != before {
				t.Fatal("ShallowFreeze mutated its predecessor")
			}

			again, againOK := schema.ShallowFreeze(normal, recentReference)
			againNormal, againNormalOK := again.Normal(key)
			if !againOK || !againNormalOK || !Same(againNormal, normal) {
				t.Fatal("ShallowFreeze was not idempotent")
			}
			if _, summaryOK := schema.ShallowFreeze(test.value, summaryReference); summaryOK {
				t.Fatal("Summary reference authorized a strong freeze")
			}
		})
	}
}

func TestShallowFreezeZeroBottomAndTopLaws(t *testing.T) {
	schema := minimalFreezeSchema(t, "freeze-endpoints")
	key, keyOK := schema.KeyAt(0)
	recentReference, recentReferenceOK := schema.Reference(key, materialization.Recent)
	none, noneOK := schema.ContainmentNone()
	recent, recentOK := schema.Object(ShapeEligible, FrozenMutable, none)
	zeroWorld, zeroWorldOK := schema.Zero(key)
	oneWorld, oneWorldOK := schema.One(key, recent)
	zero, zeroOK := schema.Relation(key, zeroWorld)
	if !keyOK || !recentReferenceOK || !noneOK || !recentOK || !zeroWorldOK || !oneWorldOK || !zeroOK {
		t.Fatal("endpoint freeze constructor")
	}

	zeroBranches, zeroBranchesOK := schema.ShallowFreeze(zero, recentReference)
	if !zeroBranchesOK {
		t.Fatal("Zero-only freeze was rejected")
	}
	if _, normalOK := zeroBranches.Normal(key); normalOK {
		t.Fatal("Zero-only freeze fabricated a normal successor")
	}

	// Relation normally removes a dominated Zero world.  Construct this
	// deliberately unnormalized white-box carrier to prove that ShallowFreeze
	// drops Zero while retaining the concrete One branch.
	mixed := Value{owner: schema.owner, worlds: []World{zeroWorld, oneWorld}}
	if !mixed.valid() {
		t.Fatal("mixed Zero+One predecessor")
	}
	mixedBranches, mixedOK := schema.ShallowFreeze(mixed, recentReference)
	mixedNormal, mixedNormalOK := mixedBranches.Normal(key)
	if !mixedOK || !mixedNormalOK || mixedNormal.WorldCount() != 1 {
		t.Fatal("Zero+One freeze did not drop the Zero branch")
	}
	mixedWorld, mixedWorldOK := mixedNormal.WorldAt(0)
	if !mixedWorldOK || mixedWorld.Kind() != WorldOne {
		t.Fatal("Zero+One freeze changed the normal control family")
	}

	bottomBranches, bottomOK := schema.ShallowFreeze(schema.Bottom(), recentReference)
	if !bottomOK {
		t.Fatal("Bottom freeze was rejected")
	}
	if _, normalOK := bottomBranches.Normal(key); normalOK {
		t.Fatal("Bottom freeze fabricated a normal successor")
	}

	top := schema.Top()
	topBranches, topOK := schema.ShallowFreeze(top, recentReference)
	topNormal, topNormalOK := topBranches.Normal(key)
	if !topOK || !topNormalOK || !topNormal.IsTop() || !Same(topNormal, top) {
		t.Fatal("Top was not retained as Top")
	}
}

func TestShallowFreezeRejectsForeignInputs(t *testing.T) {
	local := minimalFreezeSchema(t, "freeze-local")
	foreign := minimalFreezeSchema(t, "freeze-foreign")
	localKey, localKeyOK := local.KeyAt(0)
	foreignKey, foreignKeyOK := foreign.KeyAt(0)
	localNone, localNoneOK := local.ContainmentNone()
	foreignNone, foreignNoneOK := foreign.ContainmentNone()
	localObject, localObjectOK := local.Object(ShapeEligible, FrozenMutable, localNone)
	foreignObject, foreignObjectOK := foreign.Object(ShapeEligible, FrozenMutable, foreignNone)
	localWorld, localWorldOK := local.One(localKey, localObject)
	foreignWorld, foreignWorldOK := foreign.One(foreignKey, foreignObject)
	localValue, localValueOK := local.Relation(localKey, localWorld)
	foreignValue, foreignValueOK := foreign.Relation(foreignKey, foreignWorld)
	localReference, localReferenceOK := local.Reference(localKey, materialization.Recent)
	foreignReference, foreignReferenceOK := foreign.Reference(foreignKey, materialization.Recent)
	if !localKeyOK || !foreignKeyOK || !localNoneOK || !foreignNoneOK || !localObjectOK || !foreignObjectOK || !localWorldOK || !foreignWorldOK || !localValueOK || !foreignValueOK || !localReferenceOK || !foreignReferenceOK {
		t.Fatal("foreign freeze constructor")
	}
	if _, ok := local.ShallowFreeze(localValue, foreignReference); ok {
		t.Fatal("ShallowFreeze accepted a foreign reference")
	}
	if _, ok := local.ShallowFreeze(foreignValue, localReference); ok {
		t.Fatal("ShallowFreeze accepted a foreign value")
	}
}

func minimalFreezeSchema(t testing.TB, name string) Schema {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("return {}")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: name, Program: program}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linkOwner := linked.OwnerCapability()
	owner := &schema{
		linkOwner:            linkOwner,
		id:                   heapContentID(linkOwner),
		roots:                []rootRow{{kind: RootAllocation}, {kind: RootAllocation}},
		programRootCount:     2,
		referenceCount:       1,
		presentPotential:     1,
		fixedObjectRankBound: 1,
		maxObjectRankSum:     1,
	}
	owner.bottom = Value{owner: owner}
	owner.top = Value{owner: owner, top: true}
	schema := Schema{owner: owner}
	if !schema.Valid() {
		t.Fatal("minimal Heap schema is invalid")
	}
	return schema
}
