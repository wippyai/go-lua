package heap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/carrier"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// TestEveryHeapCarryTransformCandidateIsARelationSubject is the binding
// precondition Heap owes every rule that carries with one of its transforms.
// A rule draws its candidates from one relation, and the plan admits its carry
// only when the transform's candidate carrier is that relation's subject. A
// transform whose candidate is a descriptor that lives beside the fold - a
// type no relation publishes rows of - therefore cannot be bound by any rule,
// however correct the transform itself is.
func TestEveryHeapCarryTransformCandidateIsARelationSubject(t *testing.T) {
	catalog := heapdomain.AxisMemberCatalog()
	if len(catalog.CarryTransforms) == 0 {
		t.Fatal("heap declares no carry transform")
	}
	subjects := make(map[carrier.Key]struct{}, len(catalog.Relations))
	for _, relation := range catalog.Relations {
		subjects[relation.Subject] = struct{}{}
	}
	for _, transform := range catalog.CarryTransforms {
		if _, ok := subjects[transform.Candidate]; !ok {
			t.Fatalf("carry transform %q carries candidate %q, which is the subject of no heap relation", transform.Key, transform.Candidate)
		}
	}
}

// TestKeyAgeIsTheSchemaTransitionOnItsOwnCoordinate states that publishing the
// carry transition on the coordinate introduces no second transition: for
// every allocation root, the coordinate-side transform and the schema-side one
// agree on every Value the schema owns, and the coordinate refuses what the
// schema refuses.
func TestKeyAgeIsTheSchemaTransitionOnItsOwnCoordinate(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "carry_candidate", compactHeapSource, nil)
	allocations := schema.AllocationRootCount()
	if allocations == 0 {
		t.Fatal("fixture sealed no allocation root")
	}
	subjects := []heapdomain.Value{schema.Top(), schema.Bottom()}
	for index := 0; index < allocations; index++ {
		key, keyOK := schema.AllocationRootAt(index)
		if !keyOK {
			t.Fatalf("AllocationRootAt(%d)", index)
		}
		object, objectOK := schema.EmptyObject(key)
		if !objectOK {
			t.Fatalf("EmptyObject at ordinal %d", index)
		}
		subjects = append(subjects, object)
	}
	for index := 0; index < allocations; index++ {
		key, keyOK := schema.AllocationRootAt(index)
		if !keyOK {
			t.Fatalf("AllocationRootAt(%d)", index)
		}
		for subjectIndex, subject := range subjects {
			want, wantOK := schema.Age(subject, key)
			got, gotOK := key.Age(subject)
			if wantOK != gotOK {
				t.Fatalf("ordinal %d subject %d: coordinate transition availability=%t, schema=%t", index, subjectIndex, gotOK, wantOK)
			}
			if wantOK && !schema.Domain().Equal(got, want) {
				t.Fatalf("ordinal %d subject %d: coordinate transition differs from the schema transition", index, subjectIndex)
			}
		}
	}

	// A bootstrap root is not an allocation coordinate, and an unowned key
	// belongs to no schema; both are refused exactly as the schema refuses
	// them.
	for keyIndex := 0; keyIndex < schema.KeyCount(); keyIndex++ {
		key, keyOK := schema.KeyAt(keyIndex)
		if !keyOK || key.Kind() == heapdomain.RootAllocation {
			continue
		}
		if _, ok := key.Age(schema.Top()); ok {
			t.Fatalf("non-allocation root at %d admitted the allocation carry transition", keyIndex)
		}
	}
	var unowned heapdomain.Key
	if _, ok := unowned.Age(schema.Top()); ok {
		t.Fatal("an unowned coordinate admitted the allocation carry transition")
	}
}
