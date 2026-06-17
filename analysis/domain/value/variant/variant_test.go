package variant

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
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
	if got, ok := TypeFromOrigin(family, []int{cases[0], 9999}); ok {
		t.Fatalf("origin reconstruction accepted invalid active case: %s", got)
	}
	got, changed := NarrowByOrigin(union, family, duplicateUnsortedCases)
	if changed {
		t.Fatal("duplicate unsorted full case set caused a strict narrow")
	}
	if !typ.TypeEquals(got, union) {
		t.Fatalf("narrow result = %s, want original union %s", got, union)
	}
}

func TestOriginCatalogPoisonsIncompatibleClosedFamilyCollision(t *testing.T) {
	const familyID uint64 = 0x1c0111510c011151
	resetOriginFamilyForTest(t, familyID)
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

	if !storeOriginFamily(originFamily{
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
	if got, ok := TypeFromOrigin(familyID, []int{0}); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("initial synthetic origin type = %v/%v, want %s", got, ok, left)
	}

	if storeOriginFamily(originFamily{
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
	if got, ok := TypeFromOrigin(familyID, []int{0}); ok {
		t.Fatalf("poisoned origin reconstructed %s, want fail-closed", got)
	}
	if got, ok := FullFamilyType(familyID); ok {
		t.Fatalf("poisoned full family reconstructed %s, want fail-closed", got)
	}
	if projectedFamily, projectedCases, ok := ProjectOrigin(familyID, []int{0}, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}); ok {
		t.Fatalf("poisoned origin projected to %d/%v, want fail-closed", projectedFamily, projectedCases)
	}
}

func TestOriginCatalogPoisonsTaggedSignatureCollision(t *testing.T) {
	const familyID uint64 = 0x51c6a7551deca11
	const caseIndex = 1201
	resetOriginFamilyForTest(t, familyID)
	kindTagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()
	statusTagged := typetable.NewRecord().
		Field("status", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()

	if !storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: caseIndex, typ: kindTagged}},
	}) {
		t.Fatal("initial tagged family store failed")
	}
	if got, ok := TypeFromOrigin(familyID, []int{caseIndex}); !ok || !typ.TypeEquals(got, kindTagged) {
		t.Fatalf("initial tagged origin type = %v/%v, want %s", got, ok, kindTagged)
	}

	if storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:6:status;",
		cases:     []originCase{{index: caseIndex, typ: statusTagged}},
	}) {
		t.Fatal("same-id tagged family with different tag-path signature should poison")
	}
	if got, ok := TypeFromOrigin(familyID, []int{caseIndex}); ok {
		t.Fatalf("signature-poisoned tagged origin reconstructed %s, want fail-closed", got)
	}
}

func TestTaggedOriginDistinguishesSameTagDifferentPayload(t *testing.T) {
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.String).
		Build()

	leftFamily, leftCases, ok := OriginOfType(left)
	if !ok || len(leftCases) != 1 {
		t.Fatalf("left tagged origin = %d/%v/%v, want one case", leftFamily, leftCases, ok)
	}
	t.Cleanup(func() { clearOriginFamilyForTest(leftFamily) })
	rightFamily, rightCases, ok := OriginOfType(right)
	if !ok || len(rightCases) != 1 {
		t.Fatalf("right tagged origin = %d/%v/%v, want one case", rightFamily, rightCases, ok)
	}
	if rightFamily != leftFamily {
		t.Fatalf("same tag path used families %d and %d, want shared family", leftFamily, rightFamily)
	}
	if rightCases[0] == leftCases[0] {
		t.Fatalf("same tag with different payload reused case %d", leftCases[0])
	}
	if got, ok := TypeFromOrigin(leftFamily, leftCases); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("left origin reconstructed %v/%v, want %s", got, ok, left)
	}
	if got, ok := TypeFromOrigin(rightFamily, rightCases); !ok || !typ.TypeEquals(got, right) {
		t.Fatalf("right origin reconstructed %v/%v, want %s", got, ok, right)
	}
	wantBoth := typeexpr.Union(left, right)
	bothCases := append(append([]int(nil), leftCases...), rightCases...)
	if got, ok := TypeFromOrigin(leftFamily, bothCases); !ok || !typ.TypeEquals(got, wantBoth) {
		t.Fatalf("combined same-tag origin reconstructed %v/%v, want %s", got, ok, wantBoth)
	}
	joined := variantorigin.Join(variantorigin.Of(leftFamily, leftCases), variantorigin.Of(rightFamily, rightCases))
	if joined.IsTop() || joined.IsBottom() || joined.Family() != leftFamily || !sameIntSet(joined.Cases(), bothCases) {
		t.Fatalf("joined same-tag origins = %v, want concrete family %d cases %v", joined, leftFamily, bothCases)
	}

	clearOriginFamilyForTest(leftFamily)
	reverseRightFamily, reverseRightCases, ok := OriginOfType(right)
	if !ok || reverseRightFamily != leftFamily || len(reverseRightCases) != 1 {
		t.Fatalf("reverse right tagged origin = %d/%v/%v, want family %d one case", reverseRightFamily, reverseRightCases, ok, leftFamily)
	}
	reverseLeftFamily, reverseLeftCases, ok := OriginOfType(left)
	if !ok || reverseLeftFamily != leftFamily || len(reverseLeftCases) != 1 {
		t.Fatalf("reverse left tagged origin = %d/%v/%v, want family %d one case", reverseLeftFamily, reverseLeftCases, ok, leftFamily)
	}
	if reverseLeftCases[0] != leftCases[0] || reverseRightCases[0] != rightCases[0] {
		t.Fatalf("reverse registration cases = left %v right %v, want left %v right %v", reverseLeftCases, reverseRightCases, leftCases, rightCases)
	}
	if got, ok := TypeFromOrigin(leftFamily, leftCases); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("reverse left origin reconstructed %v/%v, want %s", got, ok, left)
	}
	if got, ok := TypeFromOrigin(leftFamily, rightCases); !ok || !typ.TypeEquals(got, right) {
		t.Fatalf("reverse right origin reconstructed %v/%v, want %s", got, ok, right)
	}
}

func TestOriginCatalogPoisonsTaggedSameCaseHashCollision(t *testing.T) {
	const familyID uint64 = 0x71c6ed5aceca5e
	const caseIndex = 1701
	resetOriginFamilyForTest(t, familyID)
	kindTagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Number).
		Build()
	collidingKindTagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("value", typ.Boolean).
		Build()

	if !storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: caseIndex, typ: kindTagged}},
	}) {
		t.Fatal("initial tagged family store failed")
	}
	if got, ok := TypeFromOrigin(familyID, []int{caseIndex}); !ok || !typ.TypeEquals(got, kindTagged) {
		t.Fatalf("initial tagged origin type = %v/%v, want %s", got, ok, kindTagged)
	}

	if storeOriginFamily(originFamily{
		id:        familyID,
		kind:      originFamilyKindTaggedRecord,
		signature: "tagged-record:4:kind;",
		cases:     []originCase{{index: caseIndex, typ: collidingKindTagged}},
	}) {
		t.Fatal("same-index tagged family collision should poison")
	}
	if got, ok := TypeFromOrigin(familyID, []int{caseIndex}); ok {
		t.Fatalf("same-index-poisoned tagged origin reconstructed %s, want fail-closed", got)
	}
}

func TestCacheRejectsPoisonedOriginFamily(t *testing.T) {
	const familyID uint64 = 0x5ca1ecac4e5afe11
	resetOriginFamilyForTest(t, familyID)
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

	if !storeOriginFamily(originFamily{
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
	cache := NewCache()
	if got, ok := cache.TypeFromOrigin(familyID, []int{0}); !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("cached origin before poison = %v/%v, want %s", got, ok, left)
	}

	if storeOriginFamily(originFamily{
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
	if got, ok := cache.TypeFromOrigin(familyID, []int{0}); ok {
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
	t.Cleanup(func() { clearOriginFamilyForTest(family) })
	pathFamily, pathCases, ok := cache.OriginByPathLiteral(union, kindPath, typ.LiteralString("dog"))
	if !ok || pathFamily != family || len(pathCases) != 1 {
		t.Fatalf("cached path origin = %d/%v/%v, want one dog case in family %d", pathFamily, pathCases, ok, family)
	}
	dogCase := pathCases[0]
	if got, changed := cache.NarrowByOrigin(union, family, []int{dogCase}); !changed || !typ.TypeEquals(got, dog) {
		t.Fatalf("cached narrow before poison = %v changed=%v, want dog", got, changed)
	}

	collidingDog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.Number).
		Build()
	if storeOriginFamily(originFamily{
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
	if got, changed := cache.NarrowByOrigin(union, family, []int{dogCase}); changed || !typ.TypeEquals(got, union) {
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

	family, cases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("expected closed union origin")
	}
	cache := NewCache()
	cachedFamily, cachedCases, ok := cache.OriginOfType(union)
	if !ok {
		t.Fatal("cached origin failed")
	}
	if cachedFamily != family || !sameIntSet(cachedCases, cases) {
		t.Fatalf("cached origin = %d/%v, want %d/%v", cachedFamily, cachedCases, family, cases)
	}
	duplicateUnsortedCases := []int{cases[1], cases[0], cases[1]}
	cachedType, ok := cache.TypeFromOrigin(family, duplicateUnsortedCases)
	if !ok || !typ.TypeEquals(cachedType, union) {
		t.Fatalf("cached reconstructed type = %v/%v, want %v", cachedType, ok, union)
	}
	wantNarrow, wantChanged := NarrowByOrigin(union, family, []int{cases[0]})
	cachedNarrow, changed := cache.NarrowByOrigin(union, family, []int{cases[0]})
	if changed != wantChanged || !typ.TypeEquals(cachedNarrow, wantNarrow) {
		t.Fatalf("cached narrow = %v changed=%v, want %v changed=%v", cachedNarrow, changed, wantNarrow, wantChanged)
	}
}

func resetOriginFamilyForTest(t *testing.T, id uint64) {
	t.Helper()
	clearOriginFamilyForTest(id)
	t.Cleanup(func() { clearOriginFamilyForTest(id) })
}

func clearOriginFamilyForTest(id uint64) {
	originCatalogMu.Lock()
	defer originCatalogMu.Unlock()
	delete(originCatalog, id)
	delete(originCatalogRevision, id)
	delete(originCatalogPoisoned, id)
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
	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	payloadPath := []segment.Segment{{Kind: segment.SegmentField, Name: "payload"}}

	if projectedFamily, projectedCases, ok := ProjectOrigin(rootFamily, rootCases, payloadPath); ok {
		t.Fatalf("partial parent projection produced child origin %d/%v, want fail-closed", projectedFamily, projectedCases)
	}
	withFamily, withCases, ok := OriginByPathLiteral(union, []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}, typ.LiteralString("with-payload"))
	if !ok || withFamily != rootFamily || len(withCases) != 1 {
		t.Fatalf("with-payload origin cases = %d/%v/%v, want one case in family %d", withFamily, withCases, ok, rootFamily)
	}
	childFamily, childCases, ok := ProjectOrigin(rootFamily, withCases, payloadPath)
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
	rootFamily, rootCases, ok := OriginOfType(union)
	if !ok {
		t.Fatal("closed record union origin missing")
	}
	channelFamily, channelCases, ok := OriginOfType(chanInt)
	if !ok {
		t.Fatal("channel origin missing")
	}
	channelPath := []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}

	equalCases, ok := NarrowOriginByPath(rootFamily, rootCases, channelPath, channelFamily, channelCases, true)
	if !ok {
		t.Fatal("positive origin narrowing did not drop missing-field case")
	}
	got, ok := NarrowByOrigin(union, rootFamily, equalCases)
	if !ok || !typ.TypeEquals(got, withChannel) {
		t.Fatalf("positive narrowed type = %s/%v, want with-channel case", got, ok)
	}

	notEqualCases, ok := NarrowOriginByPath(rootFamily, rootCases, channelPath, channelFamily, channelCases, false)
	if !ok {
		t.Fatal("negative origin narrowing did not keep missing-field case")
	}
	got, ok = NarrowByOrigin(union, rootFamily, notEqualCases)
	if !ok || !typ.TypeEquals(got, withoutChannel) {
		t.Fatalf("negative narrowed type = %s/%v, want without-channel case", got, ok)
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
