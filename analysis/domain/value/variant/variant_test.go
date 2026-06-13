package variant

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

func TestNarrowByPathLiteralKeepsMatchingVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", LiteralString("cat")).
		Field("meow", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteral(NewUnion(dog, cat), []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("dog"))
	if !ok {
		t.Fatal("expected strict discriminant narrowing")
	}
	if !TypeEquals(got, dog) {
		t.Fatalf("narrowed type = %s, want dog variant %s", got, dog)
	}
}

func TestNarrowByPathLiteralReturnsNeverForImpossibleSingleVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteral(dog, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("cat"))
	if !ok || got != Never {
		t.Fatalf("narrowed type = %s/%v, want never/true", got, ok)
	}
}

func TestNarrowByPathLiteralNotKeepsNonMatchingVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", LiteralString("cat")).
		Field("meow", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteralNot(NewUnion(dog, cat), []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("dog"))
	if !ok {
		t.Fatal("expected strict negative discriminant narrowing")
	}
	if !TypeEquals(got, cat) {
		t.Fatalf("narrowed type = %s, want cat variant %s", got, cat)
	}
}

func TestNarrowByPathLiteralNotReturnsNeverForMatchingSingleVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteralNot(dog, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("dog"))
	if !ok || got != Never {
		t.Fatalf("narrowed type = %s/%v, want never/true", got, ok)
	}
}

func TestNarrowByPathLiteralExpandsInstantiatedResult(t *testing.T) {
	resultProfile, valueCase, errorCase := resultProfileDiscriminantFixture()
	okPath := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}

	got, ok := NarrowByPathLiteral(resultProfile, okPath, LiteralBool(true))
	if !ok {
		t.Fatal("expected instantiated Result<Profile> to narrow on ok = true")
	}
	if !TypeEquals(got, valueCase) {
		t.Fatalf("ok = true narrowed type = %s, want value variant %s", got, valueCase)
	}

	got, ok = NarrowByPathLiteral(resultProfile, okPath, LiteralBool(false))
	if !ok {
		t.Fatal("expected instantiated Result<Profile> to narrow on ok = false")
	}
	if !TypeEquals(got, errorCase) {
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
	want := NewUnion(valueCase, errorCase)
	if !TypeEquals(got, want) {
		t.Fatalf("origin type = %s, want %s", got, want)
	}
}

func TestOriginByPathLiteralExpandsInstantiatedResult(t *testing.T) {
	resultProfile, valueCase, errorCase := resultProfileDiscriminantFixture()
	okPath := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}

	family, cases, ok := OriginByPathLiteral(resultProfile, okPath, LiteralBool(true))
	if !ok {
		t.Fatal("missing ok = true origin cases for instantiated Result<Profile>")
	}
	got, ok := NarrowByOrigin(resultProfile, family, cases)
	if !ok || !TypeEquals(got, valueCase) {
		t.Fatalf("ok = true origin narrowed type = %s/%v, want value variant", got, ok)
	}

	family, cases, ok = OriginByPathLiteral(resultProfile, okPath, LiteralBool(false))
	if !ok {
		t.Fatal("missing ok = false origin cases for instantiated Result<Profile>")
	}
	got, ok = NarrowByOrigin(resultProfile, family, cases)
	if !ok || !TypeEquals(got, errorCase) {
		t.Fatalf("ok = false origin narrowed type = %s/%v, want error variant", got, ok)
	}
}

func TestOriginProjectsAndNarrowsClosedRecordUnion(t *testing.T) {
	chanInt := NewAlias("__test_ChanInt", typetable.NewRecord().
		Field("__tag", LiteralString("int")).
		Build())
	chanStr := NewAlias("__test_ChanStr", typetable.NewRecord().
		Field("__tag", LiteralString("str")).
		Build())
	intCase := typetable.NewRecord().
		Field("channel", chanInt).
		Field("value", Number).
		Build()
	strCase := typetable.NewRecord().
		Field("channel", chanStr).
		Field("value", String).
		Build()
	union := NewUnion(intCase, strCase)

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
	if !ok || !TypeEquals(got, intCase) {
		t.Fatalf("positive narrowed type = %s/%v, want int case", got, ok)
	}

	remainingCases, ok := NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "channel"},
	}, intFamily, intCases, false)
	if !ok {
		t.Fatal("negative origin narrowing did not change root cases")
	}
	got, ok = NarrowByOrigin(union, rootFamily, remainingCases)
	if !ok || !TypeEquals(got, strCase) {
		t.Fatalf("negative narrowed type = %s/%v, want str case", got, ok)
	}
}

func TestOriginNarrowByPathIncompatibleConstraintIsNoop(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("value", Number).
		Build()
	cat := typetable.NewRecord().
		Field("kind", LiteralString("cat")).
		Field("value", String).
		Build()
	union := NewUnion(dog, cat)

	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	otherFamily, otherCases, ok := OriginOfType(typetable.NewRecord().
		Field("__tag", LiteralString("other")).
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

func resultProfileDiscriminantFixture() (Type, Type, Type) {
	profile := typetable.NewRecord().
		Field("id", String).
		Field("count", Number).
		Field("label", NewOptional(String)).
		Build()
	tp := NewTypeParam("T", nil)
	result := NewGeneric("Result", []*TypeParam{tp}, NewUnion(
		typetable.NewRecord().
			Field("ok", LiteralBool(true)).
			Field("value", tp).
			Build(),
		typetable.NewRecord().
			Field("ok", LiteralBool(false)).
			Field("error", String).
			Build(),
	))
	valueCase := typetable.NewRecord().
		Field("ok", LiteralBool(true)).
		Field("value", profile).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", LiteralBool(false)).
		Field("error", String).
		Build()
	return Instantiate(result, profile), valueCase, errorCase
}
