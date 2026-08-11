package variant

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func caseView(values []int) caseset.View {
	return caseset.New(values).View()
}

func typeFromOriginInts(cache *Cache, family uint64, cases []int) (typ.Type, bool) {
	return cache.TypeFromOrigin(family, caseView(cases))
}

func narrowByOriginInts(t typ.Type, family uint64, cases []int) (typ.Type, bool) {
	return NarrowByOrigin(t, family, caseView(cases))
}

func projectOriginInts(cache *Cache, family uint64, cases []int, suffix []segment.Segment) (uint64, []int, bool) {
	return cache.ProjectOrigin(family, caseView(cases), suffix)
}

func narrowOriginByPathInts(cache *Cache, parentFamily uint64, parentCases []int, suffix []segment.Segment, constraintFamily uint64, constraintCases []int, equal bool) ([]int, bool) {
	return cache.NarrowOriginByPath(parentFamily, caseView(parentCases), suffix, constraintFamily, caseView(constraintCases), equal)
}

func narrowOriginByPathTypeInts(cache *Cache, parentFamily uint64, parentCases []int, suffix []segment.Segment, constraint typ.Type, equal bool) ([]int, bool) {
	return cache.NarrowOriginByPathType(parentFamily, caseView(parentCases), suffix, constraint, equal)
}

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

func TestNarrowByPathTypeRefutesDisjointArm(t *testing.T) {
	chanInt := typetable.NewRecord().Field("tag", typ.LiteralString("int")).Build()
	chanStr := typetable.NewRecord().Field("tag", typ.LiteralString("str")).Build()
	intArm := typetable.NewRecord().Field("channel", chanInt).Field("value", typ.Number).Build()
	strArm := typetable.NewRecord().Field("channel", chanStr).Field("value", typ.String).Build()

	suffix := []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}
	got, ok := NarrowByPathType(typeexpr.Union(intArm, strArm), suffix, chanInt)
	if !ok {
		t.Fatal("expected strict narrowing against a disjoint peer")
	}
	if !typ.TypeEquals(got, intArm) {
		t.Fatalf("narrowed type = %s, want int arm %s", got, intArm)
	}
}

func TestNarrowByPathTypeKeepsBothWhenPeerOverlapsAll(t *testing.T) {
	chanInt := typetable.NewRecord().Field("tag", typ.LiteralString("int")).Build()
	chanStr := typetable.NewRecord().Field("tag", typ.LiteralString("str")).Build()
	intArm := typetable.NewRecord().Field("channel", chanInt).Field("value", typ.Number).Build()
	strArm := typetable.NewRecord().Field("channel", chanStr).Field("value", typ.String).Build()

	suffix := []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}
	if _, ok := NarrowByPathType(typeexpr.Union(intArm, strArm), suffix, typeexpr.Union(chanInt, chanStr)); ok {
		t.Fatal("peer overlapping every arm must not narrow")
	}
}

func TestFieldAtPathUsesProductiveRecursiveMustProof(t *testing.T) {
	path := []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}
	node := typ.NewRecursivePlaceholder("Node")
	record := typetable.NewRecord().Field("value", typ.String).Build()
	node.SetBody(&typ.Union{Members: []typ.Type{node, record}})
	got, ok := FieldAtPath(node, path)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("FieldAtPath(recursive union) = %v/%v, want string", got, ok)
	}

	bad := typ.NewRecursivePlaceholder("Bad")
	bad.SetBody(&typ.Union{Members: []typ.Type{bad, typ.Boolean}})
	if got, ok := FieldAtPath(bad, path); ok || got != nil {
		t.Fatalf("FieldAtPath(productive mismatch) = %v/%v, want failure", got, ok)
	}
}

func TestFieldAtPathTraversesDeepAcyclicGraphExactly(t *testing.T) {
	var value typ.Type = typ.String
	path := make([]segment.Segment, 257)
	for i := len(path) - 1; i >= 0; i-- {
		name := "next"
		path[i] = segment.Segment{Kind: segment.SegmentField, Name: name}
		value = typetable.NewRecord().Field(name, value).Build()
	}
	got, ok := FieldAtPath(value, path)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("deep FieldAtPath = %v/%v, want string", got, ok)
	}
}

func TestNarrowByPathTruthyDoesNotTreatStringLiteralAsBooleanTrue(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()
	animal := typeexpr.Union(dog, cat)

	got, ok := NarrowByPathTruthy(animal, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	})
	if ok {
		t.Fatalf("truthy narrowing = %s, want no strict narrowing for non-empty string discriminants", got)
	}
	if got != nil && !typ.TypeEquals(got, animal) {
		t.Fatalf("truthy narrowing returned %s, want original animal union", got)
	}
}

func TestNarrowByPathTruthyKeepsOnlyTruthyBooleanMember(t *testing.T) {
	on := typetable.NewRecord().
		Field("enabled", typ.True).
		Field("run", typ.Func().Returns().Build()).
		Build()
	off := typetable.NewRecord().
		Field("enabled", typ.False).
		Field("skip", typ.Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathTruthy(typeexpr.Union(on, off), []segment.Segment{
		{Kind: segment.SegmentField, Name: "enabled"},
	})
	if !ok {
		t.Fatal("expected truthy guard to narrow boolean discriminant")
	}
	if !typ.TypeEquals(got, on) {
		t.Fatalf("narrowed type = %s, want on variant %s", got, on)
	}
}

func TestNarrowByPathFalsyInstantiatedGenericResultWithFreeParam(t *testing.T) {
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
	got, ok := NarrowByPathFalsy(typ.Instantiate(result, tp), []segment.Segment{
		{Kind: segment.SegmentField, Name: "ok"},
	})
	if !ok {
		t.Fatal("NarrowByPathFalsy returned no change for generic Result<T>")
	}
	want := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("narrowed type = %s, want error arm %s", got, want)
	}
}

func TestCacheDoesNotRetainNegativeOriginFamilyEntries(t *testing.T) {
	c := NewCache()
	if _, _, ok := c.OriginOfType(typ.String); ok {
		t.Fatal("primitive type unexpectedly produced a variant origin family")
	}
	if _, ok := c.origins[typ.String]; ok {
		t.Fatal("negative origin-family lookup was retained in cache")
	}

	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Build()
	union := typeexpr.Union(left, right)
	if _, _, ok := c.OriginOfType(union); !ok {
		t.Fatal("discriminated union did not produce a variant origin family")
	}
	if _, ok := c.origins[union]; !ok {
		t.Fatal("positive origin-family lookup was not retained in cache")
	}
}

func TestOriginCasesExposeReadOnlyFiniteFamilyCases(t *testing.T) {
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Build()
	union := typeexpr.Union(left, right)

	family, originCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("OriginOfType returned !ok")
	}
	caseFamily, cases, ok := OriginCasesOfType(union)
	if !ok {
		t.Fatal("OriginCasesOfType returned !ok")
	}
	if caseFamily != family {
		t.Fatalf("OriginCasesOfType family = %d, want %d", caseFamily, family)
	}
	if len(cases) != 2 || len(originCases) != 2 {
		t.Fatalf("cases = %#v origin=%v, want two cases", cases, originCases)
	}
	if cases[0].Index != originCases[0] || cases[1].Index != originCases[1] {
		t.Fatalf("case indices = %d,%d want %v", cases[0].Index, cases[1].Index, originCases)
	}
	if !typ.TypeEquals(cases[0].Type, left) || !typ.TypeEquals(cases[1].Type, right) {
		t.Fatalf("case types = %s / %s, want %s / %s", cases[0].Type, cases[1].Type, left, right)
	}
	cache := NewCache()
	cache.OriginOfType(union)
	loaded, ok := cache.OriginCases(family)
	if !ok {
		t.Fatal("OriginCases returned !ok")
	}
	if len(loaded) != len(cases) || loaded[0].Index != cases[0].Index || loaded[1].Index != cases[1].Index {
		t.Fatalf("OriginCases = %#v, want %#v", loaded, cases)
	}
}

func TestSingleCaseWithField(t *testing.T) {
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.String).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("error", typ.String).
		Build()
	_, cases, ok := OriginCasesOfType(typeexpr.Union(left, right))
	if !ok {
		t.Fatal("OriginCasesOfType returned !ok")
	}
	if got, ok := SingleCaseWithField(cases, "value"); !ok || !originCaseIndexHasType(cases, got, left) {
		t.Fatalf("SingleCaseWithField(value) = %d/%v, want left case", got, ok)
	}
	if _, ok := SingleCaseWithField(cases, "missing"); ok {
		t.Fatal("SingleCaseWithField(missing) returned ok, want false")
	}
	if _, ok := SingleCaseWithField(cases, "kind"); ok {
		t.Fatal("SingleCaseWithField(kind) returned ok for ambiguous field, want false")
	}
}

func TestSingleCaseWithPath(t *testing.T) {
	withPayload := typetable.NewRecord().
		Field("kind", typ.LiteralString("payload")).
		Field("payload", typetable.NewRecord().
			Field("id", typ.String).
			Build()).
		Build()
	withoutPayload := typetable.NewRecord().
		Field("kind", typ.LiteralString("empty")).
		Build()
	_, cases, ok := OriginCasesOfType(typeexpr.Union(withPayload, withoutPayload))
	if !ok {
		t.Fatal("OriginCasesOfType returned !ok")
	}
	suffix := []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentField, Name: "id"},
	}
	if got, ok := SingleCaseWithPath(cases, suffix); !ok || !originCaseIndexHasType(cases, got, withPayload) {
		t.Fatalf("SingleCaseWithPath(payload.id) = %d/%v, want payload case", got, ok)
	}
	if _, ok := SingleCaseWithPath(cases, nil); ok {
		t.Fatal("SingleCaseWithPath(nil) returned ok, want false")
	}
}

func originCaseIndexHasType(cases []OriginCase, index int, want typ.Type) bool {
	for _, c := range cases {
		if c.Index == index {
			return typ.TypeEquals(c.Type, want)
		}
	}
	return false
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

	cache := NewCache()
	family, cases, ok := cache.OriginOfType(union)
	if !ok {
		t.Fatal("expected nil-bearing union origin")
	}
	if family == 0 {
		t.Fatal("origin family used sentinel id 0")
	}
	if len(cases) != 2 {
		t.Fatalf("origin cases = %v, want two non-nil record cases", cases)
	}
	got, ok := typeFromOriginInts(cache, family, cases)
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

	cache := NewCache()
	family, cases, ok := cache.OriginOfType(union)
	if !ok {
		t.Fatal("expected closed union origin")
	}
	if len(cases) != 2 {
		t.Fatalf("origin cases = %v, want two cases", cases)
	}
	duplicateUnsortedCases := []int{cases[1], cases[0], cases[1]}

	got, ok := typeFromOriginInts(cache, family, duplicateUnsortedCases)
	if !ok {
		t.Fatal("origin reconstruction failed")
	}
	if !typ.TypeEquals(got, union) {
		t.Fatalf("reconstructed type = %s, want %s", got, union)
	}
	if got, ok := typeFromOriginInts(cache, family, []int{cases[0], 9999}); ok {
		t.Fatalf("origin reconstruction accepted invalid active case: %s", got)
	}
	got, changed := narrowByOriginInts(union, family, duplicateUnsortedCases)
	if changed {
		t.Fatal("duplicate unsorted full case set caused a strict narrow")
	}
	if !typ.TypeEquals(got, union) {
		t.Fatalf("narrow result = %s, want original union %s", got, union)
	}
}

func TestOriginCatalogPoisonsIncompatibleClosedFamilyCollision(t *testing.T) {
	const familyID uint64 = 0x1c0111510c011151
	cache := NewCache()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	collidingLeft := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Boolean).
		Build()

	if !cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindClosedRecordUnion,
		signature: "closed-record-union",
		cases: []originCase{
			{index: 0, typ: left},
			{index: 1, typ: right},
		},
	}) {
		t.Fatal("initial synthetic family store failed")
	}
	if got, ok := typeFromOriginInts(cache, familyID, []int{0}); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("initial synthetic origin type = %v/%v, want %s", got, ok, left)
	}

	if cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindClosedRecordUnion,
		signature: "closed-record-union",
		cases: []originCase{
			{index: 0, typ: collidingLeft},
			{index: 1, typ: right},
		},
	}) {
		t.Fatal("incompatible same-id closed family should poison, not merge")
	}
	if got, ok := typeFromOriginInts(cache, familyID, []int{0}); ok {
		t.Fatalf("poisoned origin reconstructed %s, want fail-closed", got)
	}
	if got, ok := cache.FullFamilyType(familyID); ok {
		t.Fatalf("poisoned full family reconstructed %s, want fail-closed", got)
	}
	if projectedFamily, projectedCases, ok := projectOriginInts(cache, familyID, []int{0}, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}); ok {
		t.Fatalf("poisoned origin projected to %d/%v, want fail-closed", projectedFamily, projectedCases)
	}
}

func TestOriginCatalogPoisonsTaggedSignatureCollision(t *testing.T) {
	const familyID uint64 = 0x51c6a7551deca11
	const caseIndex = 1201
	cache := NewCache()
	kindTagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()
	statusTagged := typetable.NewRecord().
		Field("status", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()

	if !cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: caseIndex, typ: kindTagged}},
	}) {
		t.Fatal("initial tagged family store failed")
	}
	if got, ok := typeFromOriginInts(cache, familyID, []int{caseIndex}); !ok || !typ.TypeEquals(got, kindTagged) {
		t.Fatalf("initial tagged origin type = %v/%v, want %s", got, ok, kindTagged)
	}

	if cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:6:status;",
		cases:     []originCase{{index: caseIndex, typ: statusTagged}},
	}) {
		t.Fatal("same-id tagged family with different tag-path signature should poison")
	}
	if got, ok := typeFromOriginInts(cache, familyID, []int{caseIndex}); ok {
		t.Fatalf("signature-poisoned tagged origin reconstructed %s, want fail-closed", got)
	}
}

func TestOriginCatalogKeepsDuplicateTaggedWritesIdempotent(t *testing.T) {
	const familyID uint64 = 0x1ded51deca5e
	cache := NewCache()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.String).
		Build()
	leftFamily := originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: 3101, typ: left}},
	}

	if !cache.storeOriginFamily(leftFamily) {
		t.Fatal("initial tagged family store failed")
	}
	rev1, ok := cache.originFamilyRevision(familyID)
	if !ok {
		t.Fatal("initial family revision missing")
	}
	if !cache.storeOriginFamily(leftFamily) {
		t.Fatal("duplicate tagged family store failed")
	}
	rev2, ok := cache.originFamilyRevision(familyID)
	if !ok {
		t.Fatal("duplicate family revision missing")
	}
	if rev2 != rev1 {
		t.Fatalf("duplicate tagged write changed revision: got %d want %d", rev2, rev1)
	}

	if !cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: 3102, typ: right}},
	}) {
		t.Fatal("new tagged case store failed")
	}
	rev3, ok := cache.originFamilyRevision(familyID)
	if !ok {
		t.Fatal("extended family revision missing")
	}
	if rev3 == rev2 {
		t.Fatalf("new tagged case did not change revision: got %d", rev3)
	}
	if !cache.storeOriginFamily(leftFamily) {
		t.Fatal("covered tagged case rewrite failed")
	}
	rev4, ok := cache.originFamilyRevision(familyID)
	if !ok {
		t.Fatal("covered family revision missing")
	}
	if rev4 != rev3 {
		t.Fatalf("covered tagged write changed revision: got %d want %d", rev4, rev3)
	}
}

func TestTaggedOriginDistinguishesSameTagDifferentPayload(t *testing.T) {
	cache := NewCache()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.String).
		Build()

	leftFamily, leftCases, ok := cache.OriginOfType(left)
	if !ok || len(leftCases) != 1 {
		t.Fatalf("left tagged origin = %d/%v/%v, want one case", leftFamily, leftCases, ok)
	}
	rightFamily, rightCases, ok := cache.OriginOfType(right)
	if !ok || len(rightCases) != 1 {
		t.Fatalf("right tagged origin = %d/%v/%v, want one case", rightFamily, rightCases, ok)
	}
	if rightFamily != leftFamily {
		t.Fatalf("same tag path used families %d and %d, want shared family", leftFamily, rightFamily)
	}
	if rightCases[0] == leftCases[0] {
		t.Fatalf("same tag with different payload reused case %d", leftCases[0])
	}
	if got, ok := typeFromOriginInts(cache, leftFamily, leftCases); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("left origin reconstructed %v/%v, want %s", got, ok, left)
	}
	if got, ok := typeFromOriginInts(cache, rightFamily, rightCases); !ok || !typ.TypeEquals(got, right) {
		t.Fatalf("right origin reconstructed %v/%v, want %s", got, ok, right)
	}
	wantBoth := typeexpr.Union(left, right)
	bothCases := append(append([]int(nil), leftCases...), rightCases...)
	if got, ok := typeFromOriginInts(cache, leftFamily, bothCases); !ok || !typ.TypeEquals(got, wantBoth) {
		t.Fatalf("combined same-tag origin reconstructed %v/%v, want %s", got, ok, wantBoth)
	}
	joined := variantorigin.Join(variantorigin.Of(leftFamily, leftCases), variantorigin.Of(rightFamily, rightCases))
	if joined.IsTop() || joined.IsBottom() || joined.Family() != leftFamily || !sameIntSet(joined.Cases(), bothCases) {
		t.Fatalf("joined same-tag origins = %v, want concrete family %d cases %v", joined, leftFamily, bothCases)
	}

	cache = NewCache()
	reverseRightFamily, reverseRightCases, ok := cache.OriginOfType(right)
	if !ok || reverseRightFamily != leftFamily || len(reverseRightCases) != 1 {
		t.Fatalf("reverse right tagged origin = %d/%v/%v, want family %d one case", reverseRightFamily, reverseRightCases, ok, leftFamily)
	}
	reverseLeftFamily, reverseLeftCases, ok := cache.OriginOfType(left)
	if !ok || reverseLeftFamily != leftFamily || len(reverseLeftCases) != 1 {
		t.Fatalf("reverse left tagged origin = %d/%v/%v, want family %d one case", reverseLeftFamily, reverseLeftCases, ok, leftFamily)
	}
	if reverseLeftCases[0] != leftCases[0] || reverseRightCases[0] != rightCases[0] {
		t.Fatalf("reverse registration cases = left %v right %v, want left %v right %v", reverseLeftCases, reverseRightCases, leftCases, rightCases)
	}
	if got, ok := typeFromOriginInts(cache, leftFamily, leftCases); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("reverse left origin reconstructed %v/%v, want %s", got, ok, left)
	}
	if got, ok := typeFromOriginInts(cache, leftFamily, rightCases); !ok || !typ.TypeEquals(got, right) {
		t.Fatalf("reverse right origin reconstructed %v/%v, want %s", got, ok, right)
	}
}

func TestOriginCatalogPoisonsTaggedSameCaseHashCollision(t *testing.T) {
	const familyID uint64 = 0x71c6ed5aceca5e
	const caseIndex = 1701
	cache := NewCache()
	kindTagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()
	collidingKindTagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Boolean).
		Build()

	if !cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: caseIndex, typ: kindTagged}},
	}) {
		t.Fatal("initial tagged family store failed")
	}
	if got, ok := typeFromOriginInts(cache, familyID, []int{caseIndex}); !ok || !typ.TypeEquals(got, kindTagged) {
		t.Fatalf("initial tagged origin type = %v/%v, want %s", got, ok, kindTagged)
	}

	if cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: caseIndex, typ: collidingKindTagged}},
	}) {
		t.Fatal("same-index tagged family collision should poison")
	}
	if got, ok := typeFromOriginInts(cache, familyID, []int{caseIndex}); ok {
		t.Fatalf("same-index-poisoned tagged origin reconstructed %s, want fail-closed", got)
	}
}

func TestOriginCatalogCopiesCasesOnStoreAndLoad(t *testing.T) {
	const familyID uint64 = 0x5afe1ceca5ec0de
	cache := NewCache()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	collidingLeft := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Boolean).
		Build()

	cases := []originCase{
		{index: 0, typ: left},
		{index: 1, typ: right},
	}
	if !cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindClosedRecordUnion,
		signature: "closed-record-union",
		cases:     cases,
	}) {
		t.Fatal("initial synthetic family store failed")
	}
	cases[0].typ = collidingLeft
	if got, ok := typeFromOriginInts(cache, familyID, []int{0}); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("stored family reflected caller case mutation: %v/%v, want %s", got, ok, left)
	}

	loaded, ok := cache.loadOriginFamily(familyID)
	if !ok || len(loaded.cases) != 2 {
		t.Fatalf("loadOriginFamily = %v/%v, want two cases", loaded, ok)
	}
	loaded.cases[0].typ = collidingLeft
	loaded.cases = append(loaded.cases[:1], loaded.cases[2:]...)
	if got, ok := typeFromOriginInts(cache, familyID, []int{0}); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("stored family reflected loaded case mutation: %v/%v, want %s", got, ok, left)
	}
	if got, ok := cache.FullFamilyType(familyID); !ok || !typ.TypeEquals(got, typeexpr.Union(left, right)) {
		t.Fatalf("full family after loaded mutation = %v/%v, want original union", got, ok)
	}
}

func TestCacheRejectsPoisonedOriginFamily(t *testing.T) {
	const familyID uint64 = 0x5ca1ecac4e5afe11
	cache := NewCache()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	collidingLeft := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Boolean).
		Build()

	if !cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindClosedRecordUnion,
		signature: "closed-record-union",
		cases: []originCase{
			{index: 0, typ: left},
			{index: 1, typ: right},
		},
	}) {
		t.Fatal("initial synthetic family store failed")
	}
	if got, ok := cache.TypeFromOrigin(familyID, caseView([]int{0})); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("cached origin before poison = %v/%v, want %s", got, ok, left)
	}

	if cache.storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindClosedRecordUnion,
		signature: "closed-record-union",
		cases: []originCase{
			{index: 0, typ: collidingLeft},
			{index: 1, typ: right},
		},
	}) {
		t.Fatal("incompatible same-id closed family should poison, not merge")
	}
	if got, ok := cache.TypeFromOrigin(familyID, caseView([]int{0})); ok {
		t.Fatalf("cached poisoned origin reconstructed %s, want fail-closed", got)
	}
}

func TestCacheRejectsCachedOriginEvidenceAfterPoison(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()
	union := typeexpr.Union(dog, cat)
	kindPath := []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}
	cache := NewCache()

	family, cases, ok := cache.OriginOfType(union)
	if !ok || len(cases) != 2 {
		t.Fatalf("cached origin = %d/%v/%v, want two cases", family, cases, ok)
	}
	pathFamily, pathCases, ok := cache.OriginByPathLiteral(union, kindPath, typ.LiteralString("dog"))
	if !ok || pathFamily != family || len(pathCases) != 1 {
		t.Fatalf("cached path origin = %d/%v/%v, want one dog case in family %d", pathFamily, pathCases, ok, family)
	}
	dogCase := pathCases[0]
	if got, changed := cache.NarrowByOrigin(union, family, caseView([]int{dogCase})); !changed || !typ.TypeEquals(got, dog) {
		t.Fatalf("cached narrow before poison = %v changed=%v, want dog", got, changed)
	}

	collidingDog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Number).
		Build()
	if cache.storeOriginFamily(originFamily{
		id:        family,
		kind:      originFamilyKindClosedRecordUnion,
		signature: "closed-record-union",
		cases: []originCase{
			{index: dogCase, typ: collidingDog},
			{index: otherOriginCase(cases, dogCase), typ: cat},
		},
	}) {
		t.Fatal("incompatible same-id closed family should poison, not merge")
	}
	if gotFamily, gotCases, ok := cache.OriginOfType(union); ok {
		t.Fatalf("cached poisoned OriginOfType returned %d/%v, want fail-closed", gotFamily, gotCases)
	}
	if gotFamily, gotCases, ok := cache.OriginByPathLiteral(union, kindPath, typ.LiteralString("dog")); ok {
		t.Fatalf("cached poisoned OriginByPathLiteral returned %d/%v, want fail-closed", gotFamily, gotCases)
	}
	if got, changed := cache.NarrowByOrigin(union, family, caseView([]int{dogCase})); changed || !typ.TypeEquals(got, union) {
		t.Fatalf("cached poisoned NarrowByOrigin = %v changed=%v, want original/no-change", got, changed)
	}
}

func TestCacheMatchesOriginNarrowAndReconstruct(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()
	union := typeexpr.Union(dog, cat)

	cache := NewCache()
	family, cases, ok := cache.OriginOfType(union)
	if !ok {
		t.Fatal("expected closed union origin")
	}
	cachedFamily, cachedCases, ok := cache.OriginOfType(union)
	if !ok {
		t.Fatal("cached origin failed")
	}
	if cachedFamily != family || !sameIntSet(cachedCases, cases) {
		t.Fatalf("cached origin = %d/%v, want %d/%v", cachedFamily, cachedCases, family, cases)
	}
	duplicateUnsortedCases := []int{cases[1], cases[0], cases[1]}
	cachedType, ok := cache.TypeFromOrigin(family, caseView(duplicateUnsortedCases))
	if !ok || !typ.TypeEquals(cachedType, union) {
		t.Fatalf("cached reconstructed type = %v/%v, want %v", cachedType, ok, union)
	}
	wantNarrow, wantChanged := narrowByOriginInts(union, family, []int{cases[0]})
	cachedNarrow, changed := cache.NarrowByOrigin(union, family, caseView([]int{cases[0]}))
	if changed != wantChanged || !typ.TypeEquals(cachedNarrow, wantNarrow) {
		t.Fatalf("cached narrow = %v changed=%v, want %v changed=%v", cachedNarrow, changed, wantNarrow, wantChanged)
	}
}

func TestCacheOwnsConcurrentOriginQueriesAndRejectsForeignPayload(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()
	union := typeexpr.Union(dog, cat)
	cache := NewCache()
	family, cases, ok := cache.OriginOfType(union)
	if !ok {
		t.Fatal("expected closed union origin")
	}

	const workers = 8
	var group sync.WaitGroup
	group.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer group.Done()
			for pass := 0; pass < 100; pass++ {
				if gotFamily, gotCases, ok := cache.OriginOfType(union); !ok || gotFamily != family || !sameIntSet(gotCases, cases) {
					t.Errorf("concurrent origin = %d/%v/%v, want %d/%v", gotFamily, gotCases, ok, family, cases)
					return
				}
				if got, ok := cache.TypeFromOrigin(family, caseView(cases)); !ok || !typ.TypeEquals(got, union) {
					t.Errorf("concurrent reconstruction = %v/%v, want %s", got, ok, union)
					return
				}
			}
		}()
	}
	group.Wait()

	foreign := NewCache()
	if got, ok := foreign.TypeFromOrigin(family, caseView(cases)); ok || got != nil {
		t.Fatalf("foreign cache reconstructed owner-local family = %v/%v, want nil/false", got, ok)
	}
}

func otherOriginCase(cases []int, exclude int) int {
	for _, c := range cases {
		if c != exclude {
			return c
		}
	}
	return exclude
}

func TestCacheOriginByPathLiteralReturnsCopiedCases(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.Func().Returns().Build()).
		Build()
	union := typeexpr.Union(dog, cat)
	kindPath := []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}
	cache := NewCache()

	family, cases, ok := cache.OriginByPathLiteral(union, kindPath, typ.LiteralString("dog"))
	if !ok || len(cases) != 1 {
		t.Fatalf("cached path origin = %d/%v/%v, want one dog case", family, cases, ok)
	}
	cases[0] = 999

	againFamily, againCases, ok := cache.OriginByPathLiteral(union, kindPath, typ.LiteralString("dog"))
	if !ok || againFamily != family || len(againCases) != 1 || againCases[0] == 999 {
		t.Fatalf("cached path origin after mutation = %d/%v/%v, want unmutated cached cases for family %d", againFamily, againCases, ok, family)
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

func TestNarrowByPathLiteralNotKeepsBroadStringFieldVariant(t *testing.T) {
	template := typetable.NewRecord().
		Field("kind", typ.LiteralString("template")).
		Field("data_func", typ.String).
		Build()
	component := typetable.NewRecord().
		Field("kind", typ.LiteralString("component")).
		Build()
	page := typeexpr.Union(template, component)

	got, ok := NarrowByPathLiteralNot(page, []segment.Segment{
		{Kind: segment.SegmentField, Name: "data_func"},
	}, typ.LiteralString(""))
	if ok {
		t.Fatalf("negative broad string-field narrowing = %s, want no strict narrowing", got)
	}
	if got != nil && !typ.TypeEquals(got, page) {
		t.Fatalf("negative broad string-field narrowing returned %s, want original page union %s", got, page)
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
	cache := NewCache()

	family, cases, ok := OriginOfType(resultProfile)
	if !ok {
		t.Fatal("missing origin for instantiated Result<Profile>")
	}
	cache.OriginOfType(resultProfile)
	got, ok := typeFromOriginInts(cache, family, cases)
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
	got, ok := narrowByOriginInts(resultProfile, family, cases)
	if !ok || !typ.TypeEquals(got, valueCase) {
		t.Fatalf("ok = true origin narrowed type = %s/%v, want value variant", got, ok)
	}

	family, cases, ok = OriginByPathLiteral(resultProfile, okPath, typ.LiteralBool(false))
	if !ok {
		t.Fatal("missing ok = false origin cases for instantiated Result<Profile>")
	}
	got, ok = narrowByOriginInts(resultProfile, family, cases)
	if !ok || !typ.TypeEquals(got, errorCase) {
		t.Fatalf("ok = false origin narrowed type = %s/%v, want error variant", got, ok)
	}
}

func TestOriginByPathLiteralNotDoesNotEliminateBroadStringFieldVariant(t *testing.T) {
	template := typetable.NewRecord().
		Field("kind", typ.LiteralString("template")).
		Field("data_func", typ.String).
		Build()
	component := typetable.NewRecord().
		Field("kind", typ.LiteralString("component")).
		Build()
	page := typeexpr.Union(template, component)

	family, cases, ok := OriginByPathLiteralNot(page, []segment.Segment{
		{Kind: segment.SegmentField, Name: "data_func"},
	}, typ.LiteralString(""))
	if ok {
		t.Fatalf("negative broad string-field origin = %d/%v, want no strict origin", family, cases)
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
	got, ok := narrowByOriginInts(treeNode, family, cases)
	if !ok {
		t.Fatal("origin did not narrow recursive TreeNode")
	}
	if !typ.TypeEquals(got, textCase) {
		t.Fatalf("origin narrowed type = %s, want text variant %s", got, textCase)
	}
}

func TestProjectOriginFailsClosedWhenAnySelectedParentCaseDoesNotProject(t *testing.T) {
	childA := typetable.NewRecord().
		Field("__tag", typ.LiteralString("a")).
		Build()
	childB := typetable.NewRecord().
		Field("__tag", typ.LiteralString("b")).
		Build()
	childUnion := typeexpr.Union(childA, childB)
	withPayload := typetable.NewRecord().
		Field("kind", typ.LiteralString("with-payload")).
		Field("payload", childUnion).
		Build()
	withoutPayload := typetable.NewRecord().
		Field("kind", typ.LiteralString("without-payload")).
		Build()
	union := typeexpr.Union(withPayload, withoutPayload)
	cache := NewCache()
	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	cache.OriginOfType(union)
	cache.OriginOfType(childUnion)
	payloadPath := []segment.Segment{{Kind: segment.SegmentField, Name: "payload"}}

	if projectedFamily, projectedCases, ok := projectOriginInts(cache, rootFamily, rootCases, payloadPath); ok {
		t.Fatalf("partial parent projection produced child origin %d/%v, want fail-closed", projectedFamily, projectedCases)
	}
	withFamily, withCases, ok := OriginByPathLiteral(union, []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}, typ.LiteralString("with-payload"))
	if !ok || withFamily != rootFamily || len(withCases) != 1 {
		t.Fatalf("with-payload origin cases = %d/%v/%v, want one case in family %d", withFamily, withCases, ok, rootFamily)
	}
	childFamily, childCases, ok := projectOriginInts(cache, rootFamily, withCases, payloadPath)
	if !ok {
		t.Fatal("single parent case with payload did not project")
	}
	wantChildFamily, wantChildCases, ok := OriginOfType(childUnion)
	if !ok {
		t.Fatal("child union origin missing")
	}
	if childFamily != wantChildFamily || !sameIntSet(childCases, wantChildCases) {
		t.Fatalf("projected child origin = %d/%v, want %d/%v", childFamily, childCases, wantChildFamily, wantChildCases)
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
	cache := NewCache()

	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	cache.OriginOfType(union)
	channelFamily, channelCases, ok := projectOriginInts(cache, rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "channel"},
	})
	if !ok {
		t.Fatal("channel origin projection missing")
	}
	intFamily, intCases, ok := cache.OriginOfType(chanInt)
	if !ok {
		t.Fatal("chanInt origin missing")
	}
	if channelFamily != intFamily || len(channelCases) != 2 {
		t.Fatalf("projected channel origin = family %d cases %v, want family %d with two cases", channelFamily, channelCases, intFamily)
	}

	narrowCases, ok := narrowOriginByPathInts(cache, rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "channel"},
	}, intFamily, intCases, true)
	if !ok {
		t.Fatal("positive origin narrowing did not change root cases")
	}
	got, ok := narrowByOriginInts(union, rootFamily, narrowCases)
	if !ok || !typ.TypeEquals(got, intCase) {
		t.Fatalf("positive narrowed type = %s/%v, want int case", got, ok)
	}

	remainingCases, ok := narrowOriginByPathInts(cache, rootFamily, rootCases, []segment.Segment{
		{Kind: segment.SegmentField, Name: "channel"},
	}, intFamily, intCases, false)
	if !ok {
		t.Fatal("negative origin narrowing did not change root cases")
	}
	got, ok = narrowByOriginInts(union, rootFamily, remainingCases)
	if !ok || !typ.TypeEquals(got, strCase) {
		t.Fatalf("negative narrowed type = %s/%v, want str case", got, ok)
	}
}

func TestNarrowOriginByPathTreatsMissingFieldAsNil(t *testing.T) {
	chanInt := typ.NewAlias("__test_MissingChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	withChannel := typetable.NewRecord().
		Field("kind", typ.LiteralString("with-channel")).
		Field("channel", chanInt).
		Build()
	withoutChannel := typetable.NewRecord().
		Field("kind", typ.LiteralString("without-channel")).
		Build()
	union := typeexpr.Union(withChannel, withoutChannel)
	cache := NewCache()
	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	cache.OriginOfType(union)
	channelFamily, channelCases, ok := cache.OriginOfType(chanInt)
	if !ok {
		t.Fatal("channel origin missing")
	}
	channelPath := []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}

	equalCases, ok := narrowOriginByPathInts(cache, rootFamily, rootCases, channelPath, channelFamily, channelCases, true)
	if !ok {
		t.Fatal("positive origin narrowing did not drop missing-field case")
	}
	got, ok := narrowByOriginInts(union, rootFamily, equalCases)
	if !ok || !typ.TypeEquals(got, withChannel) {
		t.Fatalf("positive narrowed type = %s/%v, want with-channel case", got, ok)
	}

	notEqualCases, ok := narrowOriginByPathInts(cache, rootFamily, rootCases, channelPath, channelFamily, channelCases, false)
	if !ok {
		t.Fatal("negative origin narrowing did not keep missing-field case")
	}
	got, ok = narrowByOriginInts(union, rootFamily, notEqualCases)
	if !ok || !typ.TypeEquals(got, withoutChannel) {
		t.Fatalf("negative narrowed type = %s/%v, want without-channel case", got, ok)
	}
}

func TestNarrowOriginByPathTypeKeepsFieldCompatibleCases(t *testing.T) {
	chanInt := typ.NewAlias("__test_TypeChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	chanStr := typ.NewAlias("__test_TypeChanStr", typetable.NewRecord().
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
	cache := NewCache()
	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	cache.OriginOfType(union)
	channelPath := []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}

	equalCases, ok := narrowOriginByPathTypeInts(cache, rootFamily, rootCases, channelPath, chanInt, true)
	if !ok {
		t.Fatal("positive type narrowing did not keep channel-compatible case")
	}
	got, ok := narrowByOriginInts(union, rootFamily, equalCases)
	if !ok || !typ.TypeEquals(got, intCase) {
		t.Fatalf("positive narrowed type = %s/%v, want int case", got, ok)
	}

	notEqualCases, ok := narrowOriginByPathTypeInts(cache, rootFamily, rootCases, channelPath, chanInt, false)
	if !ok {
		t.Fatal("negative type narrowing did not keep incompatible case")
	}
	got, ok = narrowByOriginInts(union, rootFamily, notEqualCases)
	if !ok || !typ.TypeEquals(got, strCase) {
		t.Fatalf("negative narrowed type = %s/%v, want str case", got, ok)
	}
}

func TestNarrowOriginByPathTypeTreatsAliasAndExpandedTargetAsCompatible(t *testing.T) {
	chanIntTarget := typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build()
	chanInt := typ.NewAlias("__test_TypeExpandedChanInt", chanIntTarget)
	chanStr := typ.NewAlias("__test_TypeExpandedChanStr", typetable.NewRecord().
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
	cache := NewCache()
	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	cache.OriginOfType(union)
	channelPath := []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}

	equalCases, ok := narrowOriginByPathTypeInts(cache, rootFamily, rootCases, channelPath, chanIntTarget, true)
	if !ok {
		t.Fatal("positive type narrowing treated alias and expanded target as incompatible")
	}
	got, ok := narrowByOriginInts(union, rootFamily, equalCases)
	if !ok || !typ.TypeEquals(got, intCase) {
		t.Fatalf("positive narrowed type = %s/%v, want int case", got, ok)
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
	cache := NewCache()

	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	cache.OriginOfType(union)
	otherFamily, otherCases, ok := cache.OriginOfType(typetable.NewRecord().
		Field("__tag", typ.LiteralString("other")).
		Build())
	if !ok {
		t.Fatal("other origin missing")
	}
	if _, ok := narrowOriginByPathInts(cache, rootFamily, rootCases, []segment.Segment{
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

func TestNarrowByLiteralKeepsOnlyInhabitedArm(t *testing.T) {
	got, ok := NarrowByLiteral(typeexpr.Union(typ.String, typ.False), typ.False)
	if !ok {
		t.Fatal("expected a strict narrowing of string | false by false")
	}
	if !typ.TypeEquals(got, typ.False) {
		t.Fatalf("got %v, want false", got)
	}
}

func TestNarrowByLiteralDecomposesBoolean(t *testing.T) {
	got, ok := NarrowByLiteral(typ.Boolean, typ.True)
	if !ok {
		t.Fatal("expected a strict narrowing of boolean by true")
	}
	if !typ.TypeEquals(got, typ.True) {
		t.Fatalf("got %v, want true", got)
	}
}

func TestNarrowByLiteralLeavesOpenScalarWhole(t *testing.T) {
	got, ok := NarrowByLiteral(typ.String, typ.LiteralString("a"))
	if ok {
		t.Fatal("an open scalar must not collapse onto a single literal")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("got %v, want string", got)
	}
}

func TestNarrowByLiteralAndNotPartitionTheSameUnion(t *testing.T) {
	union := typeexpr.Union(typ.String, typ.False)
	positive, positiveOK := NarrowByLiteral(union, typ.False)
	negative, negativeOK := NarrowByLiteralNot(union, typ.False)
	if !positiveOK || !negativeOK {
		t.Fatal("both edges of x == false must narrow string | false")
	}
	if !typ.TypeEquals(positive, typ.False) || !typ.TypeEquals(negative, typ.String) {
		t.Fatalf("got %v / %v, want false / string", positive, negative)
	}
}

func TestNarrowByRuntimeTypeSelectsTaggedArm(t *testing.T) {
	union := typeexpr.Union(typ.String, typ.Number)
	holds, holdsOK := NarrowByRuntimeType(union, "string", true)
	fails, failsOK := NarrowByRuntimeType(union, "string", false)
	if !holdsOK || !failsOK {
		t.Fatal("both edges of type(x) == \"string\" must narrow string | number")
	}
	if !typ.TypeEquals(holds, typ.String) || !typ.TypeEquals(fails, typ.Number) {
		t.Fatalf("got %v / %v, want string / number", holds, fails)
	}
}

func TestNarrowByRuntimeTypeKeepsUndecidableArm(t *testing.T) {
	got, ok := NarrowByRuntimeType(typeexpr.Union(typ.String, typ.Any), "string", true)
	if ok {
		t.Fatal("an arm whose runtime tag is undecidable must survive the guard")
	}
	if got == nil {
		t.Fatal("expected the original union back")
	}
}

func TestNarrowByRuntimeTypeGroupsEveryTableSpelling(t *testing.T) {
	record := typetable.NewRecord().Field("a", typ.Number).Build()
	got, ok := NarrowByRuntimeType(typeexpr.Union(record, typ.String), "table", true)
	if !ok {
		t.Fatal("expected a record arm to answer the table tag")
	}
	if !typ.TypeEquals(got, record) {
		t.Fatalf("got %v, want the record arm", got)
	}
}
