package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func TestQualifiedTypeIndexUsesExactNamesAndOwnerHandles(t *testing.T) {
	contract := mustSeal(t, declaration.Spec{
		Types: []vocabulary.QualifiedTypeSpec{
			{Name: "stream.Stream", Declaration: testString},
			{Name: "stream.StreamID", Declaration: testString},
		},
	})
	stream, ok := contract.Types().Lookup("stream.Stream")
	if !ok || stream == 0 {
		t.Fatalf("stream.Stream lookup = %d/%v", stream, ok)
	}
	if _, ok := contract.Types().Lookup("stream.Streamer"); ok {
		t.Fatal("missing stream.Streamer unexpectedly resolved")
	}
	if _, ok := contract.Types().Lookup("other.Stream"); ok {
		t.Fatal("foreign other.Stream unexpectedly resolved")
	}
	name, enumerated, ok := contract.Types().At(0)
	if !ok || name != "stream.Stream" || enumerated != stream {
		t.Fatalf("first qualified type row = %q/%d/%v", name, enumerated, ok)
	}
	declaration, ok := contract.Types().Declaration(stream)
	if !ok || !declaration.Equal(testString) {
		t.Fatal("qualified type declaration was not retained")
	}
}

func TestQualifiedTypeIndexRejectsDuplicateAmbiguousAndCaseVariantNames(t *testing.T) {
	cases := [][]vocabulary.QualifiedTypeSpec{
		{{Name: "stream.Stream", Declaration: testString}, {Name: "stream.Stream", Declaration: testString}},
		{{Name: "stream.Stream", Declaration: testString}, {Name: "stream.Stream", Declaration: testAny}},
		{{Name: "stream.Stream", Declaration: testString}, {Name: "Stream.stream", Declaration: testString}},
	}
	for index, types := range cases {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			if _, err := testSeal(&declaration.Spec{Types: types}); err == nil {
				t.Fatal("ambiguous qualified type declarations sealed")
			}
		})
	}
}

func TestQualifiedTypeIndexKeepsEqualStructureDistinctByName(t *testing.T) {
	contract := mustSeal(t, declaration.Spec{Types: []vocabulary.QualifiedTypeSpec{
		{Name: "stream.Stream", Declaration: testString},
		{Name: "other.Stream", Declaration: testString},
	}})
	left, leftOK := contract.Types().Lookup("stream.Stream")
	right, rightOK := contract.Types().Lookup("other.Stream")
	if !leftOK || !rightOK || left == 0 || right == 0 || left == right {
		t.Fatalf("equal declarations collapsed by name mapping: %d/%v and %d/%v", left, leftOK, right, rightOK)
	}
}

func TestQualifiedTypeIndexChangesTargetContentIdentity(t *testing.T) {
	left := mustSeal(t, declaration.Spec{Types: []vocabulary.QualifiedTypeSpec{
		{Name: "stream.Stream", Declaration: testString},
	}})
	right := mustSeal(t, declaration.Spec{Types: []vocabulary.QualifiedTypeSpec{
		{Name: "other.Stream", Declaration: testString},
	}})
	if left.ContentID() == right.ContentID() {
		t.Fatal("qualified type name change was omitted from Target content identity")
	}
}

func TestQualifiedTypeIndexContentIdentityIsIndependentOfAuthoringOrder(t *testing.T) {
	left := mustSeal(t, declaration.Spec{Types: []vocabulary.QualifiedTypeSpec{
		{Name: "stream.Stream", Declaration: testString},
		{Name: "other.Stream", Declaration: testAny},
	}})
	right := mustSeal(t, declaration.Spec{Types: []vocabulary.QualifiedTypeSpec{
		{Name: "other.Stream", Declaration: testAny},
		{Name: "stream.Stream", Declaration: testString},
	}})
	if left.ContentID() != right.ContentID() {
		t.Fatal("qualified type declaration order changed Target content identity")
	}
}
