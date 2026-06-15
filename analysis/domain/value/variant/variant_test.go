package variant

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestNarrowByPathLiteralKeepsMatchingVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteral(typeexpr.Union(dog, cat), []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, typ.LiteralString("dog"))
	if !ok {
		t.Fatal("expected strict discriminant narrowing")
	}
	if !typ.TypeEquals(got, dog) {
		t.Fatalf("narrowed type = %s, want dog variant %s", got, dog)
	}
}

func TestNarrowByPathLiteralNarrowsNilBearingUnion(t *testing.T) {
	// A flattened optional discriminated union (nil | A | B) — as produced when a
	// guarded optional surfaces with nil as a union member rather than an
	// Optional wrapper — must still narrow on its discriminant. nil is not a
	// record variant; it is dropped from the family.
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteral(typeexpr.Union(typ.Nil, dog, cat), []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, typ.LiteralString("dog"))
	if !ok {
		t.Fatal("expected discriminant narrowing of a nil-bearing union")
	}
	if !typ.TypeEquals(got, dog) {
		t.Fatalf("narrowed type = %s, want dog variant %s", got, dog)
	}
}

func TestOriginOfTypeDropsNilAndReconstructsRecordUnion(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()
	union := typeexpr.Union(typ.Nil, dog, cat)

	family, cases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("expected nil-bearing union origin")
	}
	if family == 0 {
		t.Fatal("origin family used sentinel id 0")
	}
	if len(cases) != 2 {
		t.Fatalf("origin cases = %v, want two non-nil record cases", cases)
	}
	got, ok := TypeFromOrigin(family, cases)
	if !ok {
		t.Fatal("origin reconstruction failed")
	}
	want := typeexpr.Union(dog, cat)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("reconstructed origin type = %s, want %s", got, want)
	}
}

func TestOriginCasesTreatDuplicateUnsortedCasesAsSet(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()
	union := typeexpr.Union(dog, cat)

	family, cases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("expected closed union origin")
	}
	if len(cases) != 2 {
		t.Fatalf("origin cases = %v, want two cases", cases)
	}
	duplicateUnsortedCases := []int{cases[1], cases[0], cases[1]}

	got, ok := TypeFromOrigin(family, duplicateUnsortedCases)
	if !ok {
		t.Fatal("origin reconstruction failed")
	}
	if !typ.TypeEquals(got, union) {
		t.Fatalf("reconstructed type = %s, want %s", got, union)
	}
	got, changed := NarrowByOrigin(union, family, duplicateUnsortedCases)
	if changed {
		t.Fatal("duplicate unsorted full case set caused a strict narrow")
	}
	if !typ.TypeEquals(got, union) {
		t.Fatalf("narrow result = %s, want original union %s", got, union)
	}
}

func TestNarrowByPathLiteralReturnsNeverForImpossibleSingleVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteral(dog, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, typ.LiteralString("cat"))
	if !ok || got != typ.Never {
		t.Fatalf("narrowed type = %s/%v, want never/true", got, ok)
	}
}

func TestNarrowByPathLiteralNotKeepsNonMatchingVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteralNot(typeexpr.Union(dog, cat), []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, typ.LiteralString("dog"))
	if !ok {
		t.Fatal("expected strict negative discriminant narrowing")
	}
	if !typ.TypeEquals(got, cat) {
		t.Fatalf("narrowed type = %s, want cat variant %s", got, cat)
	}
}

func TestNarrowByPathLiteralNotReturnsNeverForMatchingSingleVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteralNot(dog, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, typ.LiteralString("dog"))
	if !ok || got != typ.Never {
		t.Fatalf("narrowed type = %s/%v, want never/true", got, ok)
	}
}

func TestNarrowByPathLiteralExpandsInstantiatedResult(t *testing.T) {
	resultProfile, valueCase, errorCase := resultProfileDiscriminantFixture()
	okPath := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}

	got, ok := NarrowByPathLiteral(resultProfile, okPath, typ.LiteralBool(true))
	if !ok {
		t.Fatal("expected instantiated Result<Profile> to narrow on ok = true")
	}
	if !typ.TypeEquals(got, valueCase) {
		t.Fatalf("ok = true narrowed type = %s, want value variant %s", got, valueCase)
	}

	got, ok = NarrowByPathLiteral(resultProfile, okPath, typ.LiteralBool(false))
	if !ok {
		t.Fatal("expected instantiated Result<Profile> to narrow on ok = false")
	}
	if !typ.TypeEquals(got, errorCase) {
		t.Fatalf("ok = false narrowed type = %s, want error variant %s", got, errorCase)
	}
}

func TestOriginOfTypeExpandsInstantiatedResult(t *testing.T) {
	resultProfile, valueCase, errorCase := resultProfileDiscriminantFixture()

	family, cases, ok := OriginOfType(resultProfile)
	if !ok {
		t.Fatal("missing origin for instantiated Result<Profile>")
	}
	got, ok := TypeFromOrigin(family, cases)
	if !ok {
		t.Fatal("missing reconstructed origin type for instantiated Result<Profile>")
	}
	want := typeexpr.Union(valueCase, errorCase)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("origin type = %s, want %s", got, want)
	}
}

func TestOriginByPathLiteralExpandsInstantiatedResult(t *testing.T) {
	resultProfile, valueCase, errorCase := resultProfileDiscriminantFixture()
	okPath := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}

	family, cases, ok := OriginByPathLiteral(resultProfile, okPath, typ.LiteralBool(true))
	if !ok {
		t.Fatal("missing ok = true origin cases for instantiated Result<Profile>")
	}
	got, ok := NarrowByOrigin(resultProfile, family, cases)
	if !ok || !typ.TypeEquals(got, valueCase) {
		t.Fatalf("ok = true origin narrowed type = %s/%v, want value variant", got, ok)
	}

	family, cases, ok = OriginByPathLiteral(resultProfile, okPath, typ.LiteralBool(false))
	if !ok {
		t.Fatal("missing ok = false origin cases for instantiated Result<Profile>")
	}
	got, ok = NarrowByOrigin(resultProfile, family, cases)
	if !ok || !typ.TypeEquals(got, errorCase) {
		t.Fatalf("ok = false origin narrowed type = %s/%v, want error variant", got, ok)
	}
}

func TestRecursiveDiscriminatedUnionNarrowsByLiteralPath(t *testing.T) {
	treeNode, textCase, _ := recursiveTreeNodeFixture()
	kindPath := []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}

	got, ok := NarrowByPathLiteral(treeNode, kindPath, typ.LiteralString("text"))
	if !ok {
		t.Fatal("expected recursive TreeNode to narrow on kind = text")
	}
	if !typ.TypeEquals(got, textCase) {
		t.Fatalf("narrowed type = %s, want text variant %s", got, textCase)
	}

	optionalTree := typeexpr.Optional(treeNode)
	got, ok = NarrowByPathLiteral(optionalTree, kindPath, typ.LiteralString("text"))
	if !ok {
		t.Fatal("expected optional recursive TreeNode to narrow on kind = text")
	}
	if !typ.TypeEquals(got, textCase) {
		t.Fatalf("optional narrowed type = %s, want text variant %s", got, textCase)
	}
}

func TestRecursiveDiscriminatedUnionOriginNarrowsByLiteralPath(t *testing.T) {
	treeNode, textCase, _ := recursiveTreeNodeFixture()
	kindPath := []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}

	family, cases, ok := OriginByPathLiteral(treeNode, kindPath, typ.LiteralString("text"))
	if !ok {
		t.Fatal("missing origin cases for recursive TreeNode kind = text")
	}
	got, ok := NarrowByOrigin(treeNode, family, cases)
	if !ok {
		t.Fatal("origin did not narrow recursive TreeNode")
	}
	if !typ.TypeEquals(got, textCase) {
		t.Fatalf("origin narrowed type = %s, want text variant %s", got, textCase)
	}
}

func TestOriginProjectsAndNarrowsClosedRecordUnion(t *testing.T) {
	chanInt := typ.NewAlias("__test_ChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	chanStr := typ.NewAlias("__test_ChanStr", typetable.NewRecord().
		Field("__tag", typ.LiteralString("str")).
		Build())
	intCase := typetable.NewRecord().
		Field("channel", chanInt).
		Field("value", typ.Number).
		Build()
	strCase := typetable.NewRecord().
		Field("channel", chanStr).
		Field("value", typ.String).
		Build()
	union := typeexpr.Union(intCase, strCase)

	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	channelFamily, channelCases, ok := ProjectOrigin(rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "channel"},
	})
	if !ok {
		t.Fatal("channel origin projection missing")
	}
	intFamily, intCases, ok := OriginOfType(chanInt)
	if !ok {
		t.Fatal("chanInt origin missing")
	}
	if channelFamily != intFamily || len(channelCases) != 2 {
		t.Fatalf("projected channel origin = family %d cases %v, want family %d with two cases", channelFamily, channelCases, intFamily)
	}

	narrowCases, ok := NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "channel"},
	}, intFamily, intCases, true)
	if !ok {
		t.Fatal("positive origin narrowing did not change root cases")
	}
	got, ok := NarrowByOrigin(union, rootFamily, narrowCases)
	if !ok || !typ.TypeEquals(got, intCase) {
		t.Fatalf("positive narrowed type = %s/%v, want int case", got, ok)
	}

	remainingCases, ok := NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "channel"},
	}, intFamily, intCases, false)
	if !ok {
		t.Fatal("negative origin narrowing did not change root cases")
	}
	got, ok = NarrowByOrigin(union, rootFamily, remainingCases)
	if !ok || !typ.TypeEquals(got, strCase) {
		t.Fatalf("negative narrowed type = %s/%v, want str case", got, ok)
	}
}

func TestOriginNarrowByPathIncompatibleConstraintIsNoop(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("value", typ.Number).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("value", typ.String).
		Build()
	union := typeexpr.Union(dog, cat)

	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	otherFamily, otherCases, ok := OriginOfType(typetable.NewRecord().
		Field("__tag", typ.LiteralString("other")).
		Build())
	if !ok {
		t.Fatal("other origin missing")
	}
	if _, ok := NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, otherFamily, otherCases, false); ok {
		t.Fatal("incompatible constraint narrowed root cases")
	}
}

func resultProfileDiscriminantFixture() (typ.Type, typ.Type, typ.Type) {
	profile := typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Field("label", typeexpr.Optional(typ.String)).
		Build()
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", tp).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", typ.String).
			Build(),
	))
	valueCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", profile).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	return typ.Instantiate(result, profile), valueCase, errorCase
}

func recursiveTreeNodeFixture() (typ.Type, typ.Type, typ.Type) {
	textCase := typetable.NewRecord().
		Field("kind", typ.LiteralString("text")).
		Field("value", typ.String).
		Build()
	groupCase := typetable.NewRecord().
		Field("kind", typ.LiteralString("group")).
		Build()
	tree := typ.NewRecursive("TreeNode", func(self typ.Type) typ.Type {
		group := typetable.NewRecord().
			Field("kind", typ.LiteralString("group")).
			Field("children", typ.NewArray(self)).
			Build()
		return typeexpr.Union(textCase, group)
	})
	return tree, textCase, groupCase
}
