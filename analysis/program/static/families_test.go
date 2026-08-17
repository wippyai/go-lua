package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"testing"
)

// This law is deliberately about semantic coverage, never Go file layout.
// Every dense authored Static facet has exactly one family entry and exactly
// one typed input-count disposition; the cross-owner sparse anchor has its own
// bounded relation and never pretends to be dense.
func TestStaticInputRelationDenominatorIsCompleteAndUnique(t *testing.T) {
	wantDense := [...]keyspace.Family{
		keyspace.FamilyTypePrimitive, keyspace.FamilyTypeLiteral, keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion, keyspace.FamilyTypeIntersection, keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric, keyspace.FamilyTypeArray, keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord, keyspace.FamilyTypeField, keyspace.FamilyTypeAlias,
		keyspace.FamilyTypeParam, keyspace.FamilyTypeInterface, keyspace.FamilyDeclaredType,
		keyspace.FamilyTypeFunction, keyspace.FamilyTypeAsserts, keyspace.FamilyFunction,
		keyspace.FamilyCall, keyspace.FamilyTypePublication, keyspace.FamilyTypeValue,
		keyspace.FamilyAnnotation, keyspace.FamilyTypeOf, keyspace.FamilyTypeKeyOf,
		keyspace.FamilyTypeIndexAccess, keyspace.FamilyTypeConditional,
	}
	wantSparse := [...]keyspace.Family{keyspace.FamilyValueClaim}
	if len(staticInputRelationFamilies) != 26 || len(staticSparseInputRelationFamilies) != 1 {
		t.Fatalf("Static relation inventory = dense %d + sparse %d, want 26 + 1", len(staticInputRelationFamilies), len(staticSparseInputRelationFamilies))
	}
	if staticInputRelationFamilies != wantDense {
		t.Fatalf("static dense relation denominator = %v, want %v", staticInputRelationFamilies, wantDense)
	}
	if staticSparseInputRelationFamilies != wantSparse {
		t.Fatalf("static sparse relation denominator = %v, want %v", staticSparseInputRelationFamilies, wantSparse)
	}
	var empty Input
	seen := map[keyspace.Family]bool{}
	for _, family := range staticInputRelationFamilies {
		if seen[family] {
			t.Fatalf("static relation denominator duplicates %v", family)
		}
		seen[family] = true
		if count, ok := staticFamilyInputCount(empty, family); !ok || count != 0 {
			t.Fatalf("static dense relation %v has no exact empty input disposition: (%d, %v)", family, count, ok)
		}
		empty.Counts[family] = 1
		if matchingCounts(empty) {
			t.Fatalf("matchingCounts accepted a nonzero %v without its typed input row", family)
		}
		empty.Counts[family] = 0
	}
	for _, family := range staticSparseInputRelationFamilies {
		if seen[family] {
			t.Fatalf("static relation denominator overlaps dense family %v", family)
		}
		seen[family] = true
		if count, ok := staticFamilyInputCount(empty, family); !ok || count != 0 {
			t.Fatalf("static sparse relation %v has no exact empty input disposition: (%d, %v)", family, count, ok)
		}
		// A sparse relation may be absent under a nonzero external census.
		empty.Counts[family] = 1
		if !matchingCounts(empty) {
			t.Fatalf("matchingCounts rejected an empty sparse %v relation under external count one", family)
		}
		empty.Operands.Claim = []ClaimTarget{
			{Claim: keyspace.MakeTerm(family, 1)},
			{Claim: keyspace.MakeTerm(family, 2)},
		}
		if matchingCounts(empty) {
			t.Fatalf("matchingCounts accepted sparse %v rows beyond external count", family)
		}
		empty.Operands.Claim = nil
		empty.Counts[family] = 0
	}
	if len(seen) != 27 {
		t.Fatalf("Static relation inventory unique count = %d, want 27", len(seen))
	}
}
