package operands

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/internal/framing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// TestOperandsPreserveExactSparseAndDenseRelations proves the sparse relation
// keeps only its authored members, the dense sidecars resolve by canonical
// term, and the annotation index groups by target.
func TestOperandsPreserveExactSparseAndDenseRelations(t *testing.T) {
	operands := build(t, ledgerInput()).NewView(ledgerCounts())

	claims := operands.Claims()
	first := term(keyspace.FamilyValueClaim, 1)
	absent := term(keyspace.FamilyValueClaim, 2)
	if claims.Count() != 2 {
		t.Fatalf("semantic ClaimTarget count = %d, want 2 (not the whole ValueClaim census)", claims.Count())
	}
	if got, ok := claims.At(0); !ok || got != first {
		t.Fatalf("canonical claim target term = %v/%v, want claim 1", got, ok)
	}
	if target, ok := claims.Target(first); !ok || target != primitive(1) {
		t.Fatalf("claim target = %v/%v", target, ok)
	}
	if target, ok := claims.Target(absent); ok || target != 0 {
		t.Fatalf("missing sparse target = %v/%v, want zero/false", target, ok)
	}

	typeValues := operands.TypeValues()
	typeValue := term(keyspace.FamilyTypeValue, 2)
	if typeValues.Count() != 2 {
		t.Fatalf("TypeValue target count = %d, want 2", typeValues.Count())
	}
	if target, ok := typeValues.Target(typeValue); !ok || target != term(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("TypeValue target = %v/%v", target, ok)
	}

	annotations := operands.Annotations()
	if count, ok := annotations.ForCount(primitive(1)); !ok || count != 2 {
		t.Fatalf("annotation index count = %d/%v, want 2", count, ok)
	}
	for index, want := range []uint32{1, 3} {
		got, ok := annotations.ForAt(primitive(1), index)
		if !ok || got != term(keyspace.FamilyAnnotation, want) {
			t.Fatalf("annotation index term[%d] = %v/%v", index, got, ok)
		}
	}
	if count, ok := annotations.ForCount(primitive(3)); !ok || count != 0 {
		t.Fatalf("valid unannotated target count = %d/%v, want 0/true", count, ok)
	}
	if _, ok := annotations.ForCount(term(keyspace.FamilyTypeAlias, 1)); ok {
		t.Fatal("annotation query accepted a non-static anchor")
	}
}

// TestOperandsRejectInvalidTargets proves the admissions this vertical owns.
// Sharing one concrete target between two external parents is a combined
// containment defect and belongs to the enclosing owner's forest seal.
func TestOperandsRejectInvalidTargets(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{"duplicate claim", func(in *Input) { in.Claim = append(in.Claim, in.Claim[0]) }},
		{"invalid claim family", func(in *Input) {
			in.Claim[0].Claim = term(keyspace.FamilyTypeValue, 1)
		}},
		{"claim past the external denominator", func(in *Input) {
			in.Claim[0].Claim = term(keyspace.FamilyValueClaim, 9)
		}},
		{"invalid claim static target", func(in *Input) {
			in.Claim[0].Target = term(keyspace.FamilyValues, 1)
		}},
		{"annotation invalid values", func(in *Input) { in.Annotation[0].Values = primitive(1) }},
		{"annotation missing name", func(in *Input) { in.Annotation[0].Name = 0 }},
		{"annotation invalid anchor", func(in *Input) {
			in.Annotation[0].Target = term(keyspace.FamilyTypeAlias, 1)
		}},
		{"annotation invalid scope", func(in *Input) {
			in.Annotation[0].Scope = term(keyspace.FamilyValues, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledgerInput()
			test.edit(&input)
			if _, err := Build(input, ledgerCounts(), ledgerTypes(t), ledgerRefs(t)); err == nil {
				t.Fatal("Build() accepted an invalid operand relation")
			}
		})
	}
}

// TestOperandsCanonicalizeSparseClaimOrder proves the retained sparse relation
// is ordered by the Flow claim ordinal, not by the order the builder saw.
func TestOperandsCanonicalizeSparseClaimOrder(t *testing.T) {
	input := ledgerInput()
	input.Claim[0], input.Claim[1] = input.Claim[1], input.Claim[0]
	claims := build(t, input).NewView(ledgerCounts()).Claims()
	for index, want := range []keyspace.Term{
		term(keyspace.FamilyValueClaim, 1), term(keyspace.FamilyValueClaim, 3),
	} {
		got, ok := claims.At(index)
		if !ok || got != want {
			t.Fatalf("canonical Claims.At(%d) = %v/%v, want %v", index, got, ok, want)
		}
	}
}

// TestOperandsCopyFenceBoundsAndQueriesDoNotAllocate proves the seal takes a
// copy, every read is total, and the hot queries allocate nothing.
func TestOperandsCopyFenceBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := ledgerInput()
	table := build(t, input)
	input.Claim[0].Target = 0
	input.TypeValue[0].Target = 0
	input.Annotation[0].Name = 99

	operands := table.NewView(ledgerCounts())
	claim := term(keyspace.FamilyValueClaim, 1)
	typeValue := term(keyspace.FamilyTypeValue, 1)
	annotation := term(keyspace.FamilyAnnotation, 1)
	if target, ok := operands.Claims().Target(claim); !ok || target == 0 {
		t.Fatalf("claim copy fence = %v/%v", target, ok)
	}
	if target, ok := operands.TypeValues().Target(typeValue); !ok || target == 0 {
		t.Fatalf("TypeValue copy fence = %v/%v", target, ok)
	}
	if row, ok := operands.Annotations().Get(annotation); !ok || row.Name != 5 {
		t.Fatalf("annotation copy fence = %+v/%v", row, ok)
	}
	if _, ok := operands.Claims().At(2); ok {
		t.Fatal("Claims.At accepted sparse out-of-bounds index")
	}
	if _, ok := operands.TypeValues().Target(term(keyspace.FamilyTypeValue, 9)); ok {
		t.Fatal("TypeValues.Target accepted unknown term")
	}
	if _, ok := operands.TypeValues().Target(term(keyspace.FamilyValueClaim, 1)); ok {
		t.Fatal("TypeValues.Target accepted foreign family")
	}
	if _, ok := operands.Annotations().ForAt(primitive(1), 2); ok {
		t.Fatal("Annotations.ForAt accepted out-of-bounds index")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		operands.Claims().Count()
		operands.Claims().At(0)
		operands.Claims().Target(claim)
		operands.TypeValues().Target(typeValue)
		operands.Annotations().Get(annotation)
		operands.Annotations().ForCount(primitive(1))
		operands.Annotations().ForAt(primitive(1), 1)
	}); allocations != 0 {
		t.Fatalf("operand queries allocated %.2f times", allocations)
	}
}

// TestDecoderRetainsSparseAndDenseRelations proves the decoded rows map each
// wire field back to the relation it names.
func TestDecoderRetainsSparseAndDenseRelations(t *testing.T) {
	decoded, err := Decode(sectionReader(t, sectionBytes(t, ledgerInput())))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded.Claim) != 2 || len(decoded.TypeValue) != 2 || len(decoded.Annotation) != 3 {
		t.Fatalf("decoded counts = (%d, %d, %d)", len(decoded.Claim), len(decoded.TypeValue), len(decoded.Annotation))
	}
	if decoded.Claim[1].Claim != term(keyspace.FamilyValueClaim, 3) || decoded.Claim[1].Target != primitive(2) {
		t.Fatalf("decoded claim = %+v", decoded.Claim[1])
	}
	if decoded.TypeValue[1].Target != term(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("decoded type value = %+v", decoded.TypeValue[1])
	}
	row := decoded.Annotation[1]
	if row.Scope != term(keyspace.FamilyCell, 2) || row.Target != primitive(2) ||
		row.Name != 6 || row.Values != term(keyspace.FamilyValues, 2) {
		t.Fatalf("decoded annotation = %+v", row)
	}
}

// TestDecoderRejectsNonCanonicalSparseClaimOrder proves the wire cannot
// smuggle a duplicate or descending claim past the canonical order the writer
// emits. The stream is hand-rolled: Build canonicalizes, so a stream produced
// through it could never carry the defect under test.
func TestDecoderRejectsNonCanonicalSparseClaimOrder(t *testing.T) {
	for _, test := range []struct {
		name   string
		claims []keyspace.Term
	}{
		{name: "descending", claims: []keyspace.Term{term(keyspace.FamilyValueClaim, 3), term(keyspace.FamilyValueClaim, 1)}},
		{name: "duplicate", claims: []keyspace.Term{term(keyspace.FamilyValueClaim, 1), term(keyspace.FamilyValueClaim, 1)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var data bytes.Buffer
			var writer framing.Writer
			if err := writer.Reset(&data, sectionDomain, sectionVersion); err != nil {
				t.Fatal(err)
			}
			if err := writer.Count(uint64(len(test.claims))); err != nil {
				t.Fatal(err)
			}
			for _, claim := range test.claims {
				if err := writer.Uint(uint64(claim)); err != nil {
					t.Fatal(err)
				}
				if err := writer.Uint(uint64(primitive(1))); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := writer.Count(0); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Finish(); err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(sectionReader(t, data.Bytes())); err == nil {
				t.Fatal("Decode accepted a non-canonical sparse claim sequence")
			}
		})
	}
}

// TestZeroViewFailsClosed proves an unavailable view answers nothing.
func TestZeroViewFailsClosed(t *testing.T) {
	var view View
	if view.Available() || view.Claims().Count() != 0 ||
		view.TypeValues().Count() != 0 || view.Annotations().Count() != 0 {
		t.Fatal("zero View reported availability or rows")
	}
	if _, ok := view.Claims().Target(term(keyspace.FamilyValueClaim, 1)); ok {
		t.Fatal("zero View resolved a claim target")
	}
	if _, ok := view.TypeValues().Target(term(keyspace.FamilyTypeValue, 1)); ok {
		t.Fatal("zero View resolved a runtime type target")
	}
	if _, ok := view.Annotations().Get(term(keyspace.FamilyAnnotation, 1)); ok {
		t.Fatal("zero View returned an annotation")
	}
	if _, ok := view.Annotations().ForCount(primitive(1)); ok {
		t.Fatal("zero View counted annotations for a target")
	}
}
