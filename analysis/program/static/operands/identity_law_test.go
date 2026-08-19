package operands

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	sectionDomain  = "program/static/operands-law"
	sectionVersion = 1
)

func ledgerCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyCell] = 2
	counts[keyspace.FamilyValues] = 2
	counts[keyspace.FamilyValueClaim] = 4
	counts[keyspace.FamilyTypeValue] = 2
	counts[keyspace.FamilyAnnotation] = 3
	counts[keyspace.FamilyTypePrimitive] = 3
	counts[keyspace.FamilyTypeRef] = 2
	counts[keyspace.FamilyTypeAlias] = 1
	return counts
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

func primitive(ordinal uint32) keyspace.Term { return term(keyspace.FamilyTypePrimitive, ordinal) }

// ledgerTypes and ledgerRefs are the two sealed stage inputs whose published
// reads decide which runtime type targets are admissible.
func ledgerTypes(t *testing.T) statictypes.Table {
	t.Helper()
	table, err := statictypes.Build(statictypes.Input{Primitive: []statictypes.Primitive{
		{Kind: statictypes.PrimitiveNumber},
		{Kind: statictypes.PrimitiveString},
		// Function is a static-only form and is deliberately not loadable.
		{Kind: statictypes.PrimitiveFunction},
	}}, ledgerCounts())
	if err != nil {
		t.Fatalf("types.Build: %v", err)
	}
	return table
}

func ledgerRefs(t *testing.T) staticrefs.Table {
	t.Helper()
	table, err := staticrefs.Build(staticrefs.Input{TypeRef: []staticrefs.TypeRef{
		{Resolution: staticrefs.Declaration, Source: []keyspace.Key{1}, Target: term(keyspace.FamilyTypeAlias, 1)},
		{Resolution: staticrefs.Unresolved, Source: []keyspace.Key{2}},
	}}, ledgerCounts())
	if err != nil {
		t.Fatalf("references.Build: %v", err)
	}
	return table
}

// ledgerInput exercises the sparse relation, both dense sidecars, and an
// annotation target carrying more than one annotation.
func ledgerInput() Input {
	return Input{
		Claim: []ClaimTarget{
			{Claim: term(keyspace.FamilyValueClaim, 1), Target: primitive(1)},
			{Claim: term(keyspace.FamilyValueClaim, 3), Target: primitive(2)},
		},
		TypeValue: []TypeValueTarget{
			{Target: primitive(1)},
			{Target: term(keyspace.FamilyTypeRef, 1)},
		},
		Annotation: []Annotation{
			{Scope: term(keyspace.FamilyCell, 1), Target: primitive(1), Name: 5, Values: term(keyspace.FamilyValues, 1)},
			{Scope: term(keyspace.FamilyCell, 2), Target: primitive(2), Name: 6, Values: term(keyspace.FamilyValues, 2)},
			{Scope: term(keyspace.FamilyCell, 1), Target: primitive(1), Name: 7, Values: term(keyspace.FamilyValues, 1)},
		},
	}
}

func build(t *testing.T, input Input) Table {
	t.Helper()
	table, err := Build(input, ledgerCounts(), ledgerTypes(t), ledgerRefs(t))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return table
}

func sectionBytes(t *testing.T, input Input) []byte {
	t.Helper()
	table := build(t, input)
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	if err := WriteContent(&writer, table); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func sectionReader(t *testing.T, data []byte) *framing.Reader {
	t.Helper()
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}

// TestAuthoredDistinctionsReachTheSection proves the section byte stream, which
// is the same schema the Static ContentID digests, separates every authored
// field and order distinction this vertical retains.
func TestAuthoredDistinctionsReachTheSection(t *testing.T) {
	for _, test := range []struct {
		name    string
		perturb func(*Input)
	}{
		{"claim.claim", func(in *Input) { in.Claim[0].Claim = term(keyspace.FamilyValueClaim, 2) }},
		{"claim.target", func(in *Input) { in.Claim[0].Target = primitive(2) }},
		{"claim.arity", func(in *Input) { in.Claim = in.Claim[:1] }},
		{"type-value.target", func(in *Input) { in.TypeValue[0].Target = primitive(2) }},
		{"type-value.row-order", func(in *Input) {
			in.TypeValue[0], in.TypeValue[1] = in.TypeValue[1], in.TypeValue[0]
		}},
		{"annotation.scope", func(in *Input) { in.Annotation[0].Scope = term(keyspace.FamilyCell, 2) }},
		{"annotation.target", func(in *Input) { in.Annotation[0].Target = primitive(2) }},
		{"annotation.name", func(in *Input) { in.Annotation[0].Name = 77 }},
		{"annotation.values", func(in *Input) { in.Annotation[0].Values = term(keyspace.FamilyValues, 2) }},
		{"annotation.row-order", func(in *Input) {
			in.Annotation[0], in.Annotation[1] = in.Annotation[1], in.Annotation[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := sectionBytes(t, ledgerInput())
			perturbed := ledgerInput()
			test.perturb(&perturbed)
			if bytes.Equal(base, sectionBytes(t, perturbed)) {
				t.Fatal("authored distinction is absent from the section stream")
			}
		})
	}
}

// TestSparseClaimOrderIsCanonicalNotAuthored proves the retained relation is
// ordered by the Flow claim ordinal, so permuting the builder input cannot
// change the sealed stream. This is the one relation whose input order is not
// its authored order.
func TestSparseClaimOrderIsCanonicalNotAuthored(t *testing.T) {
	base := sectionBytes(t, ledgerInput())
	permuted := ledgerInput()
	permuted.Claim[0], permuted.Claim[1] = permuted.Claim[1], permuted.Claim[0]
	if !bytes.Equal(base, sectionBytes(t, permuted)) {
		t.Fatal("sparse claim input order changed the canonical section stream")
	}
}

// TestSectionRoundTripPreservesEveryAuthoredRow proves the section decoder
// recovers exactly the authored input the writer emitted.
func TestSectionRoundTripPreservesEveryAuthoredRow(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput())
	reader := sectionReader(t, encoded)
	decoded, err := Decode(reader)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(encoded, sectionBytes(t, decoded)) {
		t.Fatal("round-tripped input did not reproduce the section stream")
	}
}

// TestScanValidatesWithoutRetainingRows proves the preflight half consumes the
// same stream shape as Decode.
func TestScanValidatesWithoutRetainingRows(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput())
	reader := sectionReader(t, encoded)
	if err := Scan(reader); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Scan left the stream unconsumed: %v", err)
	}
}

// TestDerivedIndexesAreExcludedFromTheSection proves the dense claim lookup
// and the annotation query index are derivatives: widening the external
// ValueClaim denominator they are sized by must not move a single byte.
func TestDerivedIndexesAreExcludedFromTheSection(t *testing.T) {
	base := sectionBytes(t, ledgerInput())
	counts := ledgerCounts()
	counts[keyspace.FamilyValueClaim]++
	table, err := Build(ledgerInput(), counts, ledgerTypes(t), ledgerRefs(t))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	if err := WriteContent(&writer, table); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(base, data.Bytes()) {
		t.Fatal("a derived index reached the section stream")
	}
}

// TestSparseClaimPresenceIsNotFlowMembership proves the sparse relation's
// presence bit answers only "has an authored static target", and that the
// dense lookup covers the whole external ValueClaim denominator.
func TestSparseClaimPresenceIsNotFlowMembership(t *testing.T) {
	view := build(t, ledgerInput()).NewView(ledgerCounts())
	claims := view.Claims()
	if claims.Count() != 2 {
		t.Fatalf("sparse claim count = %d, want 2", claims.Count())
	}
	for _, present := range []uint32{1, 3} {
		if target, ok := claims.Target(term(keyspace.FamilyValueClaim, present)); !ok || target == 0 {
			t.Fatalf("claim %d lost its authored target", present)
		}
	}
	for _, absent := range []uint32{2, 4} {
		if target, ok := claims.Target(term(keyspace.FamilyValueClaim, absent)); ok || target != 0 {
			t.Fatalf("claim %d invented a target: %d/%v", absent, target, ok)
		}
	}
	if target, ok := claims.Target(term(keyspace.FamilyValueClaim, 5)); ok || target != 0 {
		t.Fatalf("a claim past the external denominator resolved: %d/%v", target, ok)
	}
}

// TestAnnotationIndexGroupsByTargetInAuthoredOrder proves the query index
// returns exactly the annotations naming a target, in authored ordinal order,
// and distinguishes a valid target with none from a target it cannot admit.
func TestAnnotationIndexGroupsByTargetInAuthoredOrder(t *testing.T) {
	annotations := build(t, ledgerInput()).NewView(ledgerCounts()).Annotations()

	count, ok := annotations.ForCount(primitive(1))
	if !ok || count != 2 {
		t.Fatalf("ForCount(primitive 1) = %d/%v, want 2", count, ok)
	}
	for index, want := range []uint32{1, 3} {
		got, ok := annotations.ForAt(primitive(1), index)
		if !ok || got != term(keyspace.FamilyAnnotation, want) {
			t.Fatalf("ForAt(primitive 1, %d) = %d/%v, want annotation %d", index, got, ok, want)
		}
	}
	if got, ok := annotations.ForAt(primitive(1), 2); ok || got != 0 {
		t.Fatalf("ForAt past the group = %d/%v, want fail closed", got, ok)
	}

	// A valid target with no annotations is (0, true); an inadmissible target
	// is (0, false). Collapsing the two would hide a missing relation.
	if count, ok := annotations.ForCount(primitive(3)); !ok || count != 0 {
		t.Fatalf("ForCount(unannotated target) = %d/%v, want 0/true", count, ok)
	}
	if count, ok := annotations.ForCount(term(keyspace.FamilyValues, 1)); ok || count != 0 {
		t.Fatalf("ForCount(inadmissible target) = %d/%v, want 0/false", count, ok)
	}
}

// TestRuntimeTypeTargetsComeFromThePublishedColumns proves this vertical
// admits a runtime type target only on the strength of what the Types and
// References owners publish, and never re-derives it.
func TestRuntimeTypeTargetsComeFromThePublishedColumns(t *testing.T) {
	for _, test := range []struct {
		name   string
		target keyspace.Term
	}{
		{name: "static-only primitive", target: primitive(3)},
		{name: "unresolved reference", target: term(keyspace.FamilyTypeRef, 2)},
		{name: "absent primitive", target: primitive(9)},
		{name: "foreign family", target: term(keyspace.FamilyCell, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledgerInput()
			input.TypeValue[0].Target = test.target
			if _, err := Build(input, ledgerCounts(), ledgerTypes(t), ledgerRefs(t)); err == nil {
				t.Fatal("Build admitted a target no owner published as loadable")
			}
		})
	}
}

// TestBuildRefusesADuplicateClaimTarget proves one Flow claim carries at most
// one authored static target, which is what makes the dense lookup total.
func TestBuildRefusesADuplicateClaimTarget(t *testing.T) {
	input := ledgerInput()
	input.Claim[1].Claim = input.Claim[0].Claim
	if _, err := Build(input, ledgerCounts(), ledgerTypes(t), ledgerRefs(t)); err == nil {
		t.Fatal("Build admitted two static targets for one Flow claim")
	}
}
