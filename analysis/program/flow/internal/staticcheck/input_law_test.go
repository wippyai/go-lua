package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestStaticCheckCanonicalEmptyInput(t *testing.T) {
	counts := checkCounts(checkCount(keyspace.FamilyBody, 1))
	fixture := newCheckFixture(t, checkSpec{counts: counts})
	input, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(input.TypeOf) != 0 || len(input.Annotations) != 0 || len(input.Publications) != 0 {
		t.Fatalf("empty input = %#v", input)
	}
}

func TestStaticCheckCanonicalDenseInputStreams(t *testing.T) {
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
	commitInput, err := canonicalInput(finalizer.View())
	if err != nil {
		t.Fatalf("canonicalInput: %v", err)
	}
	wantTypeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	wantAnnotation := keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)
	wantPublication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	if len(commitInput.TypeOf) != 1 || commitInput.TypeOf[0] != wantTypeOf ||
		len(commitInput.Annotations) != 1 || commitInput.Annotations[0] != wantAnnotation ||
		len(commitInput.Publications) != 1 || commitInput.Publications[0] != wantPublication {
		t.Fatalf("input = %#v", commitInput)
	}
	commitInput.TypeOf[0] = 0
	commitInput.Annotations[0] = 0
	commitInput.Publications[0] = 0
	fresh, err := canonicalInput(finalizer.View())
	if err != nil || fresh.TypeOf[0] != wantTypeOf || fresh.Annotations[0] != wantAnnotation || fresh.Publications[0] != wantPublication {
		t.Fatalf("input mutation leaked into Static view: %#v/%v", fresh, err)
	}
	// Rebuild the exact dense input for the terminal Commit lifecycle law.
	commitInput = fresh
	if _, err := finalizer.Commit(commitInput); err != nil {
		t.Fatalf("Static Commit(input): %v", err)
	}
}

func TestStaticCheckCommitInputRejectsAndClosesInvalidStaticFinalizer(t *testing.T) {
	draft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	if _, err := finalizer.Commit(static.CommitInput{TypeOf: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)}}); err == nil {
		t.Fatal("invalid Static input Commit unexpectedly succeeded")
	}
	if _, err := finalizer.Commit(static.CommitInput{}); err == nil {
		t.Fatal("Static finalizer reopened after invalid Commit")
	}
}
