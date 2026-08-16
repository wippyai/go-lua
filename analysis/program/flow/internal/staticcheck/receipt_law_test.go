package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestStaticCheckCanonicalEmptyReceipt(t *testing.T) {
	counts := checkCounts(checkCount(keyspace.FamilyBody, 1))
	fixture := newCheckFixture(t, checkSpec{counts: counts})
	receipt, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.direct,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(receipt.TypeOf) != 0 || len(receipt.Annotations) != 0 || len(receipt.Publications) != 0 {
		t.Fatalf("empty receipt = %#v", receipt)
	}
}

func TestStaticCheckCanonicalDenseReceiptStreams(t *testing.T) {
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1),
		checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyRead, 1),
		checkCount(keyspace.FamilyValues, 1),
		checkCount(keyspace.FamilyAssign, 1),
		checkCount(keyspace.FamilyKey, 1),
		checkCount(keyspace.FamilyTypePrimitive, 1),
		checkCount(keyspace.FamilyTypeRef, 1),
		checkCount(keyspace.FamilyTypeOf, 1),
		checkCount(keyspace.FamilyAnnotation, 1),
		checkCount(keyspace.FamilyTypePublication, 1),
	)
	input := static.Input{
		Counts: counts,
		Types:  static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveNumber}}},
		References: static.ReferencesInput{TypeRef: []static.TypeRef{{
			Resolution: static.TypeRefCanonicalPath, Source: []keyspace.Key{1}, Canonical: []keyspace.Key{1},
		}}},
		Operators: static.OperatorsInput{TypeOf: []static.TypeOf{{
			Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1),
		}}},
		Operands: static.OperandsInput{Annotation: []static.Annotation{{
			Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
			Name: 1, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1),
		}}},
		Publications: static.PublicationsInput{Type: []static.Publication{{
			Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: 0, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
		}}},
	}
	draft, err := static.Build(input)
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	receipt, err := canonicalReceipt(finalizer.View())
	if err != nil {
		t.Fatalf("canonicalReceipt: %v", err)
	}
	wantTypeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	wantAnnotation := keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)
	wantPublication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	if len(receipt.TypeOf) != 1 || receipt.TypeOf[0] != wantTypeOf ||
		len(receipt.Annotations) != 1 || receipt.Annotations[0] != wantAnnotation ||
		len(receipt.Publications) != 1 || receipt.Publications[0] != wantPublication {
		t.Fatalf("receipt = %#v", receipt)
	}
	receipt.TypeOf[0] = 0
	receipt.Annotations[0] = 0
	receipt.Publications[0] = 0
	fresh, err := canonicalReceipt(finalizer.View())
	if err != nil || fresh.TypeOf[0] != wantTypeOf || fresh.Annotations[0] != wantAnnotation || fresh.Publications[0] != wantPublication {
		t.Fatalf("receipt mutation leaked into Static view: %#v/%v", fresh, err)
	}
	// Rebuild the exact dense receipt for the terminal Commit lifecycle law.
	receipt = fresh
	if _, err := finalizer.Commit(receipt); err != nil {
		t.Fatalf("Static Commit(receipt): %v", err)
	}
}

func TestStaticCheckReceiptCommitRejectsAndClosesInvalidStaticFinalizer(t *testing.T) {
	draft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	if _, err := finalizer.Commit(static.CommitInput{TypeOf: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)}}); err == nil {
		t.Fatal("invalid Static receipt Commit unexpectedly succeeded")
	}
	if _, err := finalizer.Commit(static.CommitInput{}); err == nil {
		t.Fatal("Static finalizer reopened after invalid Commit")
	}
}
