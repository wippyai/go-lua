package typ

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/kind"
)

// TestRecursiveBasic tests basic recursive type creation and properties.
func TestRecursiveBasic(t *testing.T) {
	// type Node = { next: Node? }
	// This is a self-referential type
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	if rec.Kind() != kind.Recursive {
		t.Errorf("Kind: got %v, want Recursive", rec.Kind())
	}

	if rec.Name != "Node" {
		t.Errorf("Name: got %q, want %q", rec.Name, "Node")
	}

	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})
}

func assertRecursiveRecord(t *testing.T, rec *Recursive, wantName string, wantFields []Field) {
	t.Helper()
	if got, want := rec.Name, wantName; got != want {
		t.Fatalf("recursive name = %q, want %q", got, want)
	}
	body, ok := rec.Body.(*Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	if got, want := body.String(), recursiveRecordString(wantFields); got != want {
		t.Fatalf("recursive body String() = %q, want %q", got, want)
	}
	if len(body.Fields) != len(wantFields) {
		t.Fatalf("recursive body fields = %#v, want %#v", body.Fields, wantFields)
	}
	for i, want := range wantFields {
		got := body.Fields[i]
		if got.Name != want.Name || got.Type != want.Type || got.Optional != want.Optional || got.Readonly != want.Readonly {
			t.Fatalf("recursive body field[%d] = %#v, want %#v", i, got, want)
		}
	}
}

func recursiveRecordString(fields []Field) string {
	parts := make([]string, len(fields))
	for i, field := range fields {
		optional := ""
		if field.Optional {
			optional = "?"
		}
		readonly := ""
		if field.Readonly {
			readonly = "readonly "
		}
		parts[i] = fmt.Sprintf("%s%s%s: %s", readonly, field.Name, optional, field.Type.String())
	}
	return "{" + joinRecursiveFields(parts) + "}"
}

func joinRecursiveFields(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	joined := parts[0]
	for _, part := range parts[1:] {
		joined += ", " + part
	}
	return joined
}

// TestRecursiveEqualsSelf tests that a recursive type equals itself (no infinite loop).
func TestRecursiveEqualsSelf(t *testing.T) {
	// type Node = { next: Node? }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})

	// Should equal itself without stack overflow
	if !typeEquals(rec, rec) {
		t.Error("recursive type should equal itself")
	}

	if !rec.Equals(rec) {
		t.Error("recursive type Equals should return true for itself")
	}
}

// TestRecursiveEqualsEquivalent tests equality of two equivalent recursive types.
func TestRecursiveEqualsEquivalent(t *testing.T) {
	// Create two structurally identical recursive types
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	assertRecursiveRecord(t, rec1, "Node", []Field{{Name: "next", Type: rec1, Optional: true}})
	assertRecursiveRecord(t, rec2, "Node", []Field{{Name: "next", Type: rec2, Optional: true}})

	// They should be structurally equal
	if !typeEquals(rec1, rec2) {
		t.Error("structurally equivalent recursive types should be equal")
	}
}

// TestRecursiveNotEqualsNonRecursive tests that recursive types don't equal non-recursive.
func TestRecursiveNotEqualsNonRecursive(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})

	// A non-recursive record
	plain := newRecord().OptField("next", Number).Build()

	if typeEquals(rec, plain) {
		t.Error("recursive type should not equal non-recursive type")
	}
}

// TestRecursiveHashConsistency tests that same recursive structure produces same hash.
func TestRecursiveHashConsistency(t *testing.T) {
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	assertRecursiveRecord(t, rec1, "Node", []Field{{Name: "next", Type: rec1, Optional: true}})
	assertRecursiveRecord(t, rec2, "Node", []Field{{Name: "next", Type: rec2, Optional: true}})

	if rec1.Hash() != rec2.Hash() {
		t.Error("structurally equal recursive types should have same hash")
	}
}

// TestRecursiveHashNoPanic tests that hashing a recursive type doesn't cause infinite recursion.
func TestRecursiveHashNoPanic(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})
	first, second := rec.Hash(), rec.Hash()
	const wantHash uint64 = 13282620527444473375
	if first != wantHash || second != wantHash {
		t.Fatalf("recursive hash calls = %d, %d; want %d", first, second, wantHash)
	}
}

func TestRecursiveSetBodyPanicsOnSecondCall(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	rec.SetBody(newRecord().Field("value", String).Build())
	first := rec.Hash()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("second SetBody on a sealed recursive node did not panic")
			}
		}()
		rec.SetBody(newRecord().Field("value", Number).Build())
	}()

	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "value", Type: String}})
	if got := rec.Hash(); got != first {
		t.Fatalf("rejected SetBody mutated the sealed hash: got %d, want %d", got, first)
	}
}

// TestRecursiveIndependentlyBuiltEquivalentGraphsAgreeOnDerivedAnswers pins
// that deleting the revision machinery did not change any observable answer:
// two structurally identical recursive graphs, built independently, must
// still report identical Hash, EqualityHash, and ContainsAny.
func TestRecursiveIndependentlyBuiltEquivalentGraphsAgreeOnDerivedAnswers(t *testing.T) {
	build := func() *Recursive {
		return NewRecursive("Node", func(self Type) Type {
			return newRecord().Field("value", Any).OptField("next", self).Build()
		})
	}
	a, b := build(), build()

	if a == b {
		t.Fatal("test requires two independently constructed nodes")
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("independently built equivalent recursive graphs disagree on Hash: %d vs %d", a.Hash(), b.Hash())
	}
	if EqualityHash(a) != EqualityHash(b) {
		t.Fatalf("independently built equivalent recursive graphs disagree on EqualityHash: %d vs %d", EqualityHash(a), EqualityHash(b))
	}
	if !ContainsAny(a) || !ContainsAny(b) {
		t.Fatal("test fixture should contain Any")
	}
}

// TestRecursiveString tests string representation of recursive types.
func TestRecursiveString(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	if got, want := rec.String(), "μNode. {next?: Node}"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})
}

// TestRecursiveStringNoPanic tests that String() on recursive type doesn't infinite loop.
func TestRecursiveStringNoPanic(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	if got, want := rec.String(), "μNode. {next?: Node}"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})
}

// TestRecursiveStringGoldenParity pins the public spelling and traversal order
// of every recursive renderer shape that previously used recursive descent.
func TestRecursiveStringGoldenParity(t *testing.T) {
	t.Run("record", func(t *testing.T) {
		rec := NewRecursivePlaceholder("Node")
		rec.SetBody(&Record{
			Fields: []Field{{Name: "next", Type: &Optional{Inner: rec}, Optional: true, Readonly: true}},
			StaticMembers: []StaticMember{{
				Kind: StaticMemberStringIndex, Name: `x"y`, Type: &Tuple{Elements: []Type{rec, nil, Nil}}, Optional: true, Readonly: true,
			}},
			MapKey: String, MapValue: &Array{Element: rec}, Open: true,
		})
		if got, want := rec.String(), "μNode. {readonly next?: Node?, readonly [\"x\\\"y\"]?: (Node, unknown, nil), [string]: Node[], ...}"; got != want {
			t.Fatalf("String() = %q, want %q", got, want)
		}
	})

	t.Run("function", func(t *testing.T) {
		rec := NewRecursivePlaceholder("Fn")
		rec.SetBody(&Function{
			TypeParams: []*TypeParam{{Name: "T", Constraint: String}},
			Params:     []Param{{Name: "node", Type: rec, Optional: true}},
			Variadic:   rec,
			Returns:    []Type{rec, &Map{Key: rec, Value: rec}},
		})
		if got, want := rec.String(), "μFn. fun<T : string>(node: Fn?, ...Fn) -> (Fn, {[Fn]: Fn})"; got != want {
			t.Fatalf("String() = %q, want %q", got, want)
		}
	})

	t.Run("interface_meta_and_instantiation", func(t *testing.T) {
		iface := NewRecursivePlaceholder("Iface")
		iface.SetBody(&Interface{Methods: []Method{{Name: "next", Type: &Function{Returns: []Type{iface}}}}})
		if got, want := iface.String(), "μIface. interface { next: fun() -> Iface }"; got != want {
			t.Fatalf("interface String() = %q, want %q", got, want)
		}

		meta := NewRecursivePlaceholder("Meta")
		meta.SetBody(&Meta{Of: meta})
		if got, want := meta.String(), "μMeta. typeof(Meta)"; got != want {
			t.Fatalf("meta String() = %q, want %q", got, want)
		}

		generic := &Generic{Name: "Box"}
		instantiated := NewRecursivePlaceholder("Inst")
		instantiated.SetBody(&Instantiated{Generic: generic, TypeArgs: []Type{instantiated}})
		if got, want := instantiated.String(), "μInst. Box<Inst>"; got != want {
			t.Fatalf("instantiation String() = %q, want %q", got, want)
		}
	})

	t.Run("joins_annotations_and_generics", func(t *testing.T) {
		join := NewRecursivePlaceholder("Join")
		join.SetBody(&Union{Members: []Type{
			&Intersection{Members: []Type{join, &ReadonlyMap{Key: String, Value: join}}},
			&Optional{Inner: join},
		}})
		if got, want := join.String(), "μJoin. Join & readonly {[string]: Join} | Join?"; got != want {
			t.Fatalf("join String() = %q, want %q", got, want)
		}

		annotated := NewRecursivePlaceholder("Annotated")
		annotated.SetBody(&Tuple{Elements: []Type{
			NewAnnotated(String, []annotation.Annotation{{Name: "min", Arg: annotation.Int64Arg(7)}}),
			&Generic{Name: "Box", TypeParams: []*TypeParam{{Name: "T", Constraint: String}}},
		}})
		if got, want := annotated.String(), "μAnnotated. (string @min(7), Box<T : string>)"; got != want {
			t.Fatalf("annotation/generic String() = %q, want %q", got, want)
		}
	})
}

func TestRecursiveStringIterativeTwelveThousandDeepMutualCycle(t *testing.T) {
	const depth = 12_000
	left := NewRecursivePlaceholder("Left")
	right := NewRecursivePlaceholder("Right")
	left.SetBody(deepRecursiveStringArrays(right, depth))
	right.SetBody(deepRecursiveStringArrays(left, depth))

	want := "μLeft. μRight. Left" + strings.Repeat("[]", depth*2)
	if got := left.String(); got != want {
		t.Fatalf("deep mutual cycle String() differs: got length %d, want length %d", len(got), len(want))
	}

	if allocs := testing.AllocsPerRun(10, func() {
		if got := left.String(); got != want {
			t.Fatal("deep mutual cycle rendering lost deterministic output")
		}
	}); allocs > 64 {
		t.Fatalf("deep iterative rendering allocated %.1f times/run, want bounded linear-buffer growth", allocs)
	}
}

func deepRecursiveStringArrays(leaf Type, depth int) Type {
	var out Type = leaf
	for range depth {
		out = &Array{Element: out}
	}
	return out
}

// TestRecursiveMutualRecursion tests two mutually recursive types.
func TestRecursiveMutualRecursion(t *testing.T) {
	// type A = { b: B? }
	// type B = { a: A? }
	// Use placeholders for mutual recursion
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")

	// Now set the bodies
	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("a", recA).Build())
	assertRecursiveRecord(t, recA, "A", []Field{{Name: "b", Type: recB, Optional: true}})
	assertRecursiveRecord(t, recB, "B", []Field{{Name: "a", Type: recA, Optional: true}})

	// Neither should cause infinite loops
	if !typeEquals(recA, recA) {
		t.Error("recA should equal itself")
	}

	if !typeEquals(recB, recB) {
		t.Error("recB should equal itself")
	}

	// A and B should not be equal
	if typeEquals(recA, recB) {
		t.Error("A should not equal B")
	}
}

// TestRecursiveMutualHashOrderIndependence tests that mutual recursion hash
// is consistent regardless of SetBody call order.
func TestRecursiveMutualHashOrderIndependence(t *testing.T) {
	// Setup 1: A first, then B
	recA1 := NewRecursivePlaceholder("X")
	recB1 := NewRecursivePlaceholder("Y")
	recA1.SetBody(newRecord().OptField("ref", recB1).Build())
	recB1.SetBody(newRecord().OptField("ref", recA1).Build())

	// Setup 2: B first, then A (reversed order)
	recA2 := NewRecursivePlaceholder("X")
	recB2 := NewRecursivePlaceholder("Y")
	recB2.SetBody(newRecord().OptField("ref", recA2).Build())
	recA2.SetBody(newRecord().OptField("ref", recB2).Build())

	// Hashes should match regardless of setup order
	if recA1.Hash() != recA2.Hash() {
		t.Errorf("X hash order-dependent: %d vs %d", recA1.Hash(), recA2.Hash())
	}
	if recB1.Hash() != recB2.Hash() {
		t.Errorf("Y hash order-dependent: %d vs %d", recB1.Hash(), recB2.Hash())
	}

	// Types should be equal
	if !typeEquals(recA1, recA2) {
		t.Error("X types should be structurally equal")
	}
	if !typeEquals(recB1, recB2) {
		t.Error("Y types should be structurally equal")
	}
}

// TestRecursiveInUnion tests recursive type as union member.
func TestRecursiveInUnion(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	union := MaterializeUnion([]Type{rec, Nil})

	u, ok := union.(*Union)
	if !ok {
		t.Fatalf("MaterializeUnion() = %T %[1]v, want union", union)
	}
	if got, want := u.String(), "nil | "+rec.String(); got != want {
		t.Fatalf("union String() = %q, want %q", got, want)
	}
	if len(u.Members) != 2 || u.Members[0] != Nil || u.Members[1] != rec {
		t.Fatalf("union members = %#v, want [nil, recursive Node]", u.Members)
	}
	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})
}

// TestRecursiveAsAliasTarget tests recursive type wrapped in alias.
func TestRecursiveAsAliasTarget(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	alias := NewAlias("MyNode", rec)

	if !typeEquals(alias, rec) {
		t.Error("alias to recursive type should equal the recursive type")
	}

	if got, want := alias.String(), "MyNode"; got != want {
		t.Fatalf("alias String() = %q, want %q", got, want)
	}
	if got, want := alias.Hash(), rec.Hash(); got != want {
		t.Fatalf("alias Hash() = %d, want recursive target hash %d", got, want)
	}
	assertRecursiveRecord(t, rec, "Node", []Field{{Name: "next", Type: rec, Optional: true}})
}

// TestRecursiveListType tests a classic recursive list type.
func TestRecursiveListType(t *testing.T) {
	// type List<T> = nil | { head: T, tail: List<T> }
	// Simplified: type List = { head: number, tail: List? }
	rec := NewRecursive("List", func(self Type) Type {
		return newRecord().
			Field("head", Number).
			OptField("tail", self).
			Build()
	})

	assertRecursiveRecord(t, rec, "List", []Field{
		{Name: "head", Type: Number},
		{Name: "tail", Type: rec, Optional: true},
	})

	// Should handle equality
	if !typeEquals(rec, rec) {
		t.Error("list type should equal itself")
	}

	// Hash should be stable
	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("hash should be stable")
	}
}

// TestRecursiveTreeType tests a recursive tree structure.
func TestRecursiveTreeType(t *testing.T) {
	// type Tree = { value: number, left: Tree?, right: Tree? }
	rec := NewRecursive("Tree", func(self Type) Type {
		return newRecord().
			Field("value", Number).
			OptField("left", self).
			OptField("right", self).
			Build()
	})
	assertRecursiveRecord(t, rec, "Tree", []Field{
		{Name: "left", Type: rec, Optional: true},
		{Name: "right", Type: rec, Optional: true},
		{Name: "value", Type: Number},
	})

	if !typeEquals(rec, rec) {
		t.Error("tree type should equal itself")
	}
}

// TestRecursiveDifferentStructuresNotEqual tests that different recursive structures are not equal.
func TestRecursiveDifferentStructuresNotEqual(t *testing.T) {
	// type A = { next: A? }
	recA := NewRecursive("A", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// type B = { child: B?, value: number }
	recB := NewRecursive("B", func(self Type) Type {
		return newRecord().
			OptField("child", self).
			Field("value", Number).
			Build()
	})

	if typeEquals(recA, recB) {
		t.Error("different recursive structures should not be equal")
	}
}

// TestIsRecursiveRef tests the IsRecursiveRef utility function.
func TestIsRecursiveRef(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Same pointer should match
	if !IsRecursiveRef(rec, rec) {
		t.Error("IsRecursiveRef should return true for same pointer")
	}

	// Different recursive type with same ID should match
	rec2 := &Recursive{ID: rec.ID, Name: "Node"}
	if !IsRecursiveRef(rec2, rec) {
		t.Error("IsRecursiveRef should return true for same ID")
	}

	// Different ID should not match
	rec3 := NewRecursive("Other", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	if IsRecursiveRef(rec3, rec) {
		t.Error("IsRecursiveRef should return false for different IDs")
	}

	// Non-recursive type should not match
	if IsRecursiveRef(Number, rec) {
		t.Error("IsRecursiveRef should return false for non-recursive type")
	}

	// Nil should not match
	if IsRecursiveRef(nil, rec) {
		t.Error("IsRecursiveRef should return false for nil")
	}
}

// TestRecursiveInArray tests recursive type as array element.
func TestRecursiveInArray(t *testing.T) {
	// type Node = { children: Node[] }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("children", NewArray(self)).Build()
	})

	// Hash should be stable
	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in array should have stable hash")
	}

	// Should equal itself
	if !typeEquals(rec, rec) {
		t.Error("recursive type in array should equal itself")
	}

	// Equivalent structure should be equal
	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("children", NewArray(self)).Build()
	})
	if !typeEquals(rec, rec2) {
		t.Error("equivalent recursive types in arrays should be equal")
	}
}

// TestRecursiveInMap tests recursive type in map key and value.
func TestRecursiveInMap(t *testing.T) {
	// type Node = { lookup: Map<string, Node> }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("lookup", NewMap(String, self)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in map value should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive type in map should equal itself")
	}
}

// TestRecursiveInTuple tests recursive type in tuple elements.
func TestRecursiveInTuple(t *testing.T) {
	// type Node = { pair: (Node, number) }
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("pair", NewTuple(self, Number)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in tuple should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive type in tuple should equal itself")
	}
}

// TestRecursiveInFunction tests recursive type in function parameters and returns.
func TestRecursiveInFunction(t *testing.T) {
	// type Handler = (self: Handler, input: number) -> Handler
	rec := NewRecursive("Handler", func(self Type) Type {
		return Func().Param("self", self).Param("input", Number).Returns(self).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive type in function should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive type in function should equal itself")
	}
}

// TestRecursiveNestedRecords tests deeply nested recursive structures.
func TestRecursiveNestedRecords(t *testing.T) {
	// type Node = { a: { b: { c: Node? } } }
	rec := NewRecursive("Node", func(self Type) Type {
		inner := newRecord().OptField("c", self).Build()
		middle := newRecord().Field("b", inner).Build()
		return newRecord().Field("a", middle).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("deeply nested recursive type should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("deeply nested recursive type should equal itself")
	}

	// Equivalent structure
	rec2 := NewRecursive("Node", func(self Type) Type {
		inner := newRecord().OptField("c", self).Build()
		middle := newRecord().Field("b", inner).Build()
		return newRecord().Field("a", middle).Build()
	})
	if !typeEquals(rec, rec2) {
		t.Error("equivalent deeply nested recursive types should be equal")
	}
}

// TestRecursiveTripleMutual tests three mutually recursive types.
func TestRecursiveTripleMutual(t *testing.T) {
	// type A = { b: B? }
	// type B = { c: C? }
	// type C = { a: A? }
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recC := NewRecursivePlaceholder("C")

	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("c", recC).Build())
	recC.SetBody(newRecord().OptField("a", recA).Build())

	// All should equal themselves
	if !typeEquals(recA, recA) {
		t.Error("recA should equal itself")
	}
	if !typeEquals(recB, recB) {
		t.Error("recB should equal itself")
	}
	if !typeEquals(recC, recC) {
		t.Error("recC should equal itself")
	}

	// Hash should be stable
	hA1, hA2 := recA.Hash(), recA.Hash()
	if hA1 != hA2 {
		t.Error("triple mutual recursion A hash should be stable")
	}

	// None should equal each other
	if typeEquals(recA, recB) || typeEquals(recB, recC) || typeEquals(recA, recC) {
		t.Error("different mutually recursive types should not be equal")
	}
}

// TestRecursivePlaceholderNilBody tests placeholder with nil body.
func TestRecursivePlaceholderNilBody(t *testing.T) {
	rec := NewRecursivePlaceholder("Empty")

	if rec.Body != nil {
		t.Fatalf("placeholder body = %T %[1]v, want nil", rec.Body)
	}
	if got, want := rec.String(), "μEmpty"; got != want {
		t.Fatalf("placeholder String() = %q, want %q", got, want)
	}
	if got, want := rec.Hash(), rec.Hash(); got != want {
		t.Fatalf("placeholder hash = %d, want stable %d", got, want)
	}
}

// TestRecursiveHashDeterminism tests that hash is deterministic across calls.
func TestRecursiveHashDeterminism(t *testing.T) {
	for i := 0; i < 100; i++ {
		rec := NewRecursive("Node", func(self Type) Type {
			return newRecord().OptField("next", self).Build()
		})
		h1 := rec.Hash()
		h2 := rec.Hash()
		h3 := rec.Hash()
		if h1 != h2 || h2 != h3 {
			t.Fatalf("hash not deterministic on iteration %d: %d, %d, %d", i, h1, h2, h3)
		}
	}
}

// TestRecursiveInOptional tests recursive type wrapped in optional.
func TestRecursiveInOptional(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("next", MaterializeOptional(self)).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive in optional should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive in optional should equal itself")
	}
}

// TestRecursiveInUnionMultiple tests recursive type in union with multiple members.
func TestRecursiveInUnionMultiple(t *testing.T) {
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("value", MaterializeUnion([]Type{self, Number, String})).Build()
	})

	h1 := rec.Hash()
	h2 := rec.Hash()
	if h1 != h2 {
		t.Error("recursive in multi-member union should have stable hash")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive in multi-member union should equal itself")
	}
}

// TestRecursiveEqualsIgnoresBinderName holds that a recursive binder is a
// bound variable. Two declarations that reach one fixed point spell its binder
// after themselves, so a name-bearing identity would make one type present as
// two; the body still decides, and two bodies remain two types.
func TestRecursiveEqualsIgnoresBinderName(t *testing.T) {
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	rec2 := NewRecursive("Item", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	if !typeEquals(rec1, rec2) {
		t.Error("one fixed point spelled with two binder names should be one type")
	}

	if rec1.Hash() != rec2.Hash() {
		t.Error("one fixed point spelled with two binder names should have one hash")
	}

	other := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("previous", self).Build()
	})

	if typeEquals(rec1, other) {
		t.Error("two fixed points should stay two types under one binder name")
	}
}

// TestUnionKeepsMutualRecursiveFamiliesByIdentity tests that generic union
// construction does not prove recursive product-family equality. Semantic
// coalescing belongs to explicit product-family join policy.
func TestUnionKeepsMutualRecursiveFamiliesByIdentity(t *testing.T) {
	// Build mutual recursion: A <-> B, first order
	recA1 := NewRecursivePlaceholder("A")
	recB1 := NewRecursivePlaceholder("B")
	recA1.SetBody(newRecord().OptField("b", recB1).Build())
	recB1.SetBody(newRecord().OptField("a", recA1).Build())

	// Build equivalent mutual recursion, second order
	recA2 := NewRecursivePlaceholder("A")
	recB2 := NewRecursivePlaceholder("B")
	recB2.SetBody(newRecord().OptField("a", recA2).Build())
	recA2.SetBody(newRecord().OptField("b", recB2).Build())

	union := MaterializeUnion([]Type{recA1, recA2, Number})
	u, ok := union.(*Union)
	if !ok {
		t.Fatalf("expected union, got %T", union)
	}

	if len(u.Members) != 3 {
		t.Errorf("expected union to preserve distinct recursive identities, got %d members: %v", len(u.Members), u.Members)
	}
}

func TestRecursiveIdentityGraphUsesInlineStorageForSmallGraphs(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("a", recA).Build())
	left := NewArray(recA)
	right := newRecord().Field("root", recA).Build()

	if !sameRecursiveIdentityGraph(left, right) {
		t.Fatal("test setup should compare the same mutual-recursive identity graph")
	}

	allocs := testing.AllocsPerRun(100, func() {
		if !sameRecursiveIdentityGraph(left, right) {
			t.Fatal("recursive identity graph mismatch")
		}
	})
	if allocs > 1 {
		t.Fatalf("sameRecursiveIdentityGraph allocations/run = %.1f, want inline storage", allocs)
	}
}

func TestRecursiveContentFlagsDoNotForceGraphClosure(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	rec.SetBody(newRecord().Field("value", String).Build())

	if rec.containsMemo.Load() != nil || rec.closedMemo.Load() != nil {
		t.Fatal("fresh recursive body should have no derived content or graph-closure memo")
	}
	if knownContainsAny(rec) {
		t.Fatal("record without any should not contain any")
	}
	if rec.containsMemo.Load() == nil {
		t.Fatal("content flag query should publish content memo")
	}
	if rec.closedMemo.Load() != nil {
		t.Fatal("content flag query must not force graph-closure proof")
	}
	if knownContainsOpenRecursive(rec) {
		t.Fatal("closed recursive body should not be open-recursive")
	}
	if rec.closedMemo.Load() == nil {
		t.Fatal("open-recursive query should publish graph-closure memo")
	}

	direct := NewRecursivePlaceholder("Direct")
	direct.SetBody(newRecord().Field("value", String).Build())
	if knownContainsAny(direct) {
		t.Fatal("direct recursive record without any should not contain any")
	}
	if direct.closedMemo.Load() != nil {
		t.Fatal("direct content predicate must not force graph-closure proof")
	}
}

func TestNilRecursiveFlagRefreshIsNoop(t *testing.T) {
	var rec *Recursive
	if got := rec.containsFlags(); got.containsAny || got.containsNever || got.containsTypeParam || got.containsInstantiated || got.containsGeneric {
		t.Fatalf("nil recursive content flags = %#v", got)
	}
	if rec.containsClosedFlag() {
		t.Fatal("nil recursive node reported closed")
	}
}

func TestOpenRecursiveWrapperHashRefreshesForEquality(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	staleWrapper := newRecord().OptField("next", rec).Build()

	rec.SetBody(newRecord().Field("value", Number).OptField("next", rec).Build())
	freshWrapper := newRecord().OptField("next", rec).Build()

	if !typeEquals(staleWrapper, freshWrapper) {
		t.Fatal("wrapper built before recursive SetBody should remain structurally equal to a fresh wrapper")
	}
	if EqualityHash(staleWrapper) != EqualityHash(freshWrapper) {
		t.Fatalf("equality hash should refresh open recursive wrapper: %d vs %d", EqualityHash(staleWrapper), EqualityHash(freshWrapper))
	}
}

func TestEqualityHashNamedGenericIncludesBodyInOpenRecursivePath(t *testing.T) {
	rec := NewRecursivePlaceholder("Node")
	left := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)},
		newRecord().Field("value", String).OptField("next", rec).Build())
	right := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)},
		newRecord().Field("value", Number).OptField("next", rec).Build())

	rec.SetBody(newRecord().OptField("next", rec).Build())

	if !knownContainsRecursive(left) || !knownContainsRecursive(right) {
		t.Fatal("test requires recursive-containing generics")
	}
	if knownContainsOpenRecursive(left) || knownContainsOpenRecursive(right) {
		t.Fatal("closed recursive generics should not retain stale open-recursive state")
	}
	if typeEquals(left, right) {
		t.Fatal("same-named generics with different bodies should not be equal")
	}
	if EqualityHash(left) == EqualityHash(right) {
		t.Fatalf("same-named generics with different bodies must not share EqualityHash: %d", EqualityHash(left))
	}
}

// TestRecursiveMutualHashConsistency tests that mutual recursion produces
// consistent hashes when accessed multiple times.
func TestRecursiveMutualHashConsistency(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("a", recA).Build())

	// Access hashes multiple times
	hashes := make([]uint64, 10)
	for i := 0; i < 10; i++ {
		hashes[i] = recA.Hash()
	}

	// All hashes should be identical
	for i := 1; i < 10; i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("hash inconsistent at iteration %d: %d vs %d", i, hashes[i], hashes[0])
		}
	}

	// Same for B
	hashesB := make([]uint64, 10)
	for i := 0; i < 10; i++ {
		hashesB[i] = recB.Hash()
	}
	for i := 1; i < 10; i++ {
		if hashesB[i] != hashesB[0] {
			t.Errorf("hash B inconsistent at iteration %d: %d vs %d", i, hashesB[i], hashesB[0])
		}
	}
}

// TestRecursiveHashOptionalReadonlyFlags tests that optional/readonly flags affect recursive hash.
func TestRecursiveHashOptionalReadonlyFlags(t *testing.T) {
	// Required field
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("next", self).Build()
	})

	// Optional field (different)
	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Readonly field (different)
	rec3 := NewRecursive("Node", func(self Type) Type {
		return newRecord().ReadonlyField("next", self).Build()
	})

	// All should have different hashes
	h1 := rec1.Hash()
	h2 := rec2.Hash()
	h3 := rec3.Hash()

	if h1 == h2 {
		t.Error("required and optional field should have different hashes")
	}
	if h1 == h3 {
		t.Error("required and readonly field should have different hashes")
	}
	if h2 == h3 {
		t.Error("optional and readonly field should have different hashes")
	}
}

// TestRecursiveHashFunctionVariadic tests that variadic changes recursive hash.
func TestRecursiveHashFunctionVariadic(t *testing.T) {
	// Function with param only
	rec1 := NewRecursive("Handler", func(self Type) Type {
		return newRecord().
			Field("process", Func().Param("x", Number).Returns(self).Build()).
			Build()
	})

	// Function with param AND variadic (different - more args)
	rec2 := NewRecursive("Handler", func(self Type) Type {
		return newRecord().
			Field("process", Func().Param("x", Number).Variadic(String).Returns(self).Build()).
			Build()
	})

	h1 := rec1.Hash()
	h2 := rec2.Hash()

	if h1 == h2 {
		t.Error("function with variadic should have different hash than without")
	}
}

// TestRecursiveHashMetatable tests that metatable changes recursive hash.
func TestRecursiveHashMetatable(t *testing.T) {
	metaType := newRecord().
		Field("__index", Func().Param("key", String).Returns(Any).Build()).
		Build()

	// Without metatable
	rec1 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("value", Number).OptField("next", self).Build()
	})

	// With metatable (different)
	rec2 := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("value", Number).OptField("next", self).Metatable(metaType).Build()
	})

	h1 := rec1.Hash()
	h2 := rec2.Hash()

	if h1 == h2 {
		t.Error("records with and without metatable should have different hashes")
	}
}

// TestRecursiveHashIntersection tests recursive types with intersections.
func TestRecursiveHashIntersection(t *testing.T) {
	// Recursive type using intersection
	rec := NewRecursive("Combined", func(self Type) Type {
		part1 := newRecord().Field("a", Number).Build()
		part2 := newRecord().Field("b", String).OptField("next", self).Build()
		return MaterializeIntersection([]Type{part1, part2})
	})

	h1 := rec.Hash()
	h2 := rec.Hash()

	if h1 != h2 {
		t.Error("recursive intersection hash should be stable")
	}

	if !typeEquals(rec, rec) {
		t.Error("recursive intersection should equal itself")
	}
}

func TestRecursiveContainsGraphClosedHandlesDeepAcyclicProducts(t *testing.T) {
	var body Type = String
	for i := 0; i < 80; i++ {
		body = NewArray(body)
	}

	if !recursiveContainsGraphClosed(body, nil) {
		t.Fatal("deep acyclic products should be recognized as closed without a depth cap")
	}
}

func TestRecursiveContainsGraphClosedAcceptsNilSeenForRecursiveNodes(t *testing.T) {
	closed := NewRecursivePlaceholder("Closed")
	closed.SetBody(newRecord().OptField("next", closed).Build())
	if !recursiveContainsGraphClosed(closed, nil) {
		t.Fatal("closed recursive node should be graph-closed when caller provides nil seen map")
	}

	dangling := NewRecursivePlaceholder("Dangling")
	if recursiveContainsGraphClosed(dangling, nil) {
		t.Fatal("dangling recursive node should not be graph-closed")
	}
}

func TestKnownContainsOpenRecursiveReflectsCurrentChildGraphState(t *testing.T) {
	child := NewRecursivePlaceholder("Child")
	wrapper := NewArray(child)
	if !knownContainsOpenRecursive(wrapper) {
		t.Fatal("unresolved child should make wrapper open-recursive")
	}

	child.SetBody(newRecord().Field("value", String).Build())
	if knownContainsOpenRecursive(wrapper) {
		t.Fatal("wrapper should reflect child becoming closed after construction")
	}
}

func TestKnownContainsOpenRecursiveCachesCompositeClosureOnceSealed(t *testing.T) {
	child := NewRecursivePlaceholder("Child")
	wrapper := NewArray(child)

	if wrapper.loadOpenRecursiveMemo() != nil {
		t.Fatal("construction must not prove recursive graph closure")
	}
	if !knownContainsOpenRecursive(wrapper) {
		t.Fatal("unresolved child should make wrapper open-recursive")
	}
	if wrapper.loadOpenRecursiveMemo() != nil {
		t.Fatal("an open result must not be cached: the child may still be sealed")
	}

	child.SetBody(newRecord().Field("value", String).Build())
	if knownContainsOpenRecursive(wrapper) {
		t.Fatal("closed child should not make wrapper open-recursive")
	}
	memo := wrapper.loadOpenRecursiveMemo()
	if memo == nil || memo.contains {
		t.Fatal("open-recursive query must memoize the closure proof once the child is sealed")
	}
}

func TestExportedContainsPredicatesSeeOpenRecursiveBodyMutation(t *testing.T) {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, newRecord().Field("value", tp).Build())

	cases := []struct {
		name     string
		contains func(Type) bool
		marker   Type
	}{
		{name: "any", contains: ContainsAny, marker: Any},
		{name: "never", contains: ContainsNever, marker: Never},
		{name: "type-param", contains: ContainsTypeParam, marker: NewTypeParam("Free", nil)},
		{name: "instantiated", contains: ContainsInstantiated, marker: Instantiate(box, String)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			child := NewRecursivePlaceholder("Child")
			root := newRecord().Field("child", child).Build()

			if tc.contains(root) {
				t.Fatal("predicate reported marker before recursive placeholder body existed")
			}

			child.SetBody(newRecord().Field("value", tc.marker).Build())
			if !tc.contains(root) {
				t.Fatal("predicate missed marker introduced by later recursive body")
			}
		})
	}
}

func TestRecursiveHashDepsHandlesDeepAcyclicProducts(t *testing.T) {
	rec := NewRecursive("Deep", func(self Type) Type {
		var body Type = self
		for i := 0; i < 80; i++ {
			body = NewArray(body)
		}
		return body
	})

	if !recursiveGraphClosureForRecursive(rec) {
		t.Fatal("deep recursive graph closure should be proven without a depth cap")
	}
	first := rec.Hash()
	second := rec.Hash()
	if first != second {
		t.Fatalf("recursive hash not stable after dependency caching: %d vs %d", first, second)
	}
}

func TestRecursiveHashClosureAndContainmentTraverseTwelveThousandNodeGraph(t *testing.T) {
	const depth = 12_000

	node := NewRecursivePlaceholder("Deep")
	var backedge Type = node
	for range depth {
		backedge = NewArray(backedge)
	}
	node.SetBody(NewTuple(Any, backedge))

	if knownContainsOpenRecursive(node) {
		t.Fatal("closed deep recursive graph reported open")
	}
	if !recursiveGraphClosureForRecursive(node) {
		t.Fatal("deep recursive graph closure should be proven without a depth cap")
	}
	if !ContainsAny(node) {
		t.Fatal("deep recursive graph lost reachable any containment")
	}
	first, second := node.Hash(), node.Hash()
	if first != second {
		t.Fatalf("deep recursive graph hash changed across cached reads: %d vs %d", first, second)
	}
}

func TestEqualityHashReadonlyMapRefreshesOpenRecursiveKeyAndValue(t *testing.T) {
	cases := []struct {
		name string
		wrap func(*Recursive) Type
	}{
		{name: "key", wrap: func(node *Recursive) Type { return NewReadonlyMap(node, String) }},
		{name: "value", wrap: func(node *Recursive) Type { return NewReadonlyMap(String, node) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := NewRecursivePlaceholder("Node")
			staleWrapper := tc.wrap(node)

			node.SetBody(newRecord().Field("value", Number).Build())
			freshWrapper := tc.wrap(node)

			if !typeEquals(staleWrapper, freshWrapper) {
				t.Fatal("ReadonlyMap built before SetBody should remain structurally equal to a fresh wrapper")
			}
			if EqualityHash(staleWrapper) != EqualityHash(freshWrapper) {
				t.Fatalf("equality hash should refresh ReadonlyMap wrapper: %d vs %d", EqualityHash(staleWrapper), EqualityHash(freshWrapper))
			}
		})
	}
}

func TestEqualityHashStaticMemberIncludesTypeInOpenRecursiveWrapper(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	direct := newRecord().
		StaticStringIndex("node", node).
		Build()
	nested := newRecord().
		StaticStringIndex("node", NewArray(node)).
		Build()

	if EqualityHash(direct) == EqualityHash(nested) {
		t.Fatal("EqualityHash() ignored static member type in open recursive wrapper")
	}
}

func TestRecursiveHashReadonlyMapTraversesKeyAndValue(t *testing.T) {
	cases := []struct {
		name string
		wrap func(Type) Type
	}{
		{name: "key", wrap: func(node Type) Type { return NewReadonlyMap(node, String) }},
		{name: "value", wrap: func(node Type) Type { return NewReadonlyMap(String, node) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			direct := NewRecursive("Box", func(self Type) Type {
				return tc.wrap(self)
			})
			nested := NewRecursive("Box", func(self Type) Type {
				return tc.wrap(NewArray(self))
			})

			if direct.Hash() != direct.Hash() {
				t.Fatal("recursive ReadonlyMap hash should be stable")
			}
			if direct.Hash() == nested.Hash() {
				t.Fatal("recursive hash ignored ReadonlyMap component type")
			}
		})
	}
}

func TestRecursiveGraphClosureStaticMemberSeesUnsealedPlaceholder(t *testing.T) {
	root := NewRecursivePlaceholder("Root")
	dangling := NewRecursivePlaceholder("Dangling")
	root.SetBody(newRecord().
		StaticStringIndex("dangling", dangling).
		Build())

	if !knownContainsOpenRecursive(root) {
		t.Fatal("graph-closure traversal missed unsealed recursive placeholder through static member")
	}
}

func TestRecursiveHashStaticMemberTraversesType(t *testing.T) {
	direct := NewRecursive("Box", func(self Type) Type {
		return newRecord().
			StaticStringIndex("node", self).
			Build()
	})
	nested := NewRecursive("Box", func(self Type) Type {
		return newRecord().
			StaticStringIndex("node", NewArray(self)).
			Build()
	})

	if direct.Hash() != direct.Hash() {
		t.Fatal("recursive static-member hash should be stable")
	}
	if direct.Hash() == nested.Hash() {
		t.Fatal("recursive hash ignored static member type")
	}
}

func TestRecursiveGraphClosureFunctionTypeParamConstraintSeesUnsealedPlaceholder(t *testing.T) {
	root := NewRecursivePlaceholder("Root")
	dangling := NewRecursivePlaceholder("Dangling")
	root.SetBody(Func().
		TypeParam("T", dangling).
		Returns(Number).
		Build())

	if !knownContainsOpenRecursive(root) {
		t.Fatal("graph-closure traversal missed unsealed recursive placeholder through function type-param constraint")
	}
}
