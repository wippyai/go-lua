package relation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type countingCompoundJoinType struct {
	name      string
	hashCalls *int
}

func (t *countingCompoundJoinType) Kind() kind.Kind { return kind.Record }
func (t *countingCompoundJoinType) String() string  { return t.name }
func (t *countingCompoundJoinType) Hash() uint64 {
	if t.hashCalls != nil {
		*t.hashCalls = *t.hashCalls + 1
	}
	return 1
}
func (t *countingCompoundJoinType) Equals(other typ.Type) bool { return t == other }

func TestDedupeJoinInputsCompoundValuesUseIdentityWithoutHashing(t *testing.T) {
	hashCalls := 0
	a := &countingCompoundJoinType{name: "a", hashCalls: &hashCalls}
	b := &countingCompoundJoinType{name: "b", hashCalls: &hashCalls}

	got := DedupeJoinInputs([]typ.Type{a, b, a})
	if len(got) != 2 {
		t.Fatalf("DedupeJoinInputs compound len = %d, want 2", len(got))
	}
	if hashCalls != 0 {
		t.Fatalf("compound join dedupe called Hash %d times, want 0", hashCalls)
	}
}

func TestDedupeJoinInputsDeduplicatesEquivalentRecursiveProductFamilies(t *testing.T) {
	left := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			OptField("next", self).
			Build()
	})
	right := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			OptField("next", self).
			Build()
	})

	got := DedupeJoinInputs([]typ.Type{left, right})
	if len(got) != 1 || got[0] != left {
		t.Fatalf("DedupeJoinInputs(equivalent recursive products) = %v, want first member only", got)
	}
}

func TestCoalesceProductFamiliesWithSlotJoinCoalescesRecursiveFamiliesByBody(t *testing.T) {
	left := typ.NewRecursive("Flow", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Inferred", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("parent", typ.NewOptional(self)).
			Build()
	})

	got := CoalesceProductFamiliesWithSlotJoin([]typ.Type{left, right}, JoinReturnSlot)
	if len(got) != 1 {
		t.Fatalf("CoalesceProductFamiliesWithSlotJoin recursive len = %d, want 1: %v", len(got), got)
	}
	rec, ok := got[0].(*typ.Recursive)
	if !ok {
		t.Fatalf("coalesced recursive family = %T %[1]v, want recursive", got[0])
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	parent := body.GetField("parent")
	if parent == nil || !parent.Optional {
		t.Fatalf("parent field = %v, want optional recursive field", parent)
	}
	children := body.GetField("children")
	if children == nil {
		t.Fatalf("missing children field in %v", body)
	}
	arr, ok := children.Type.(*typ.Array)
	if !ok || !typ.IsRecursiveRef(arr.Element, rec) {
		t.Fatalf("children field = %v, want array of merged recursive self", children.Type)
	}
}

func TestCoalesceProductFamiliesWithSlotJoinPreservesDiscriminatedRecursiveFamilies(t *testing.T) {
	a := typ.NewRecursive("Case", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("a")).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	b := typ.NewRecursive("Case", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("b")).
			Field("next", typ.NewOptional(self)).
			Build()
	})

	got := CoalesceProductFamiliesWithSlotJoin([]typ.Type{a, b}, JoinReturnSlot)
	if len(got) != 2 {
		t.Fatalf("discriminated recursive family len = %d, want 2: %v", len(got), got)
	}
}

func TestCoalesceProductFamiliesWithSlotJoinCoalescesRecordMapComponents(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.String).Build()).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.String).
		Field("handler", typ.Func().Returns(typ.String).Build()).
		MapComponent(typ.String, typ.Boolean).
		Build()

	got := CoalesceProductFamiliesWithSlotJoin([]typ.Type{left, right}, JoinReturnSlot)
	if len(got) != 1 {
		t.Fatalf("record map component family len = %d, want 1: %v", len(got), got)
	}
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("record map component family = %T %[1]v, want record", got[0])
	}
	if !rec.HasMapComponent() {
		t.Fatalf("coalesced record should keep map component: %v", rec)
	}
	if _, ok := rec.MapValue.(*typ.Union); !ok {
		t.Fatalf("merged map value = %T %[1]v, want union", rec.MapValue)
	}
}

func TestCoalesceProductFamiliesWithSlotJoinMakesVariantRecordMapFieldsOptional(t *testing.T) {
	left := typ.NewRecord().
		Field("suite", typ.String).
		Field("started", typ.Boolean).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("suite", typ.String).
		Field("finished", typ.Boolean).
		MapComponent(typ.String, typ.Boolean).
		Build()

	got := CoalesceProductFamiliesWithSlotJoin([]typ.Type{left, right}, JoinReturnSlot)
	if len(got) != 1 {
		t.Fatalf("record map family len = %d, want 1: %v", len(got), got)
	}
	rec, ok := got[0].(*typ.Record)
	if !ok {
		t.Fatalf("record map family = %T %[1]v, want record", got[0])
	}
	for _, name := range []string{"started", "finished"} {
		field := rec.GetField(name)
		if field == nil || !field.Optional {
			t.Fatalf("field %q = %v, want optional after family coalescing", name, field)
		}
	}
	if _, ok := rec.MapValue.(*typ.Union); !ok {
		t.Fatalf("merged map value = %T %[1]v, want union", rec.MapValue)
	}
}

func TestCoalesceProductFamiliesWithSlotJoinPreservesRecordMapDiscriminants(t *testing.T) {
	left := typ.NewRecord().
		Field("kind", typ.LiteralString("start")).
		Field("payload", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	right := typ.NewRecord().
		Field("kind", typ.LiteralString("stop")).
		Field("payload", typ.Integer).
		MapComponent(typ.String, typ.Number).
		Build()

	got := CoalesceProductFamiliesWithSlotJoin([]typ.Type{left, right}, JoinReturnSlot)
	if len(got) != 2 {
		t.Fatalf("discriminated record map family len = %d, want 2: %v", len(got), got)
	}
}
