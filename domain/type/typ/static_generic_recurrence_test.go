package typ

import (
	"context"
	"errors"
	"testing"
)

func TestValidateStaticGenericRecurrenceAdmitsProductiveSelfAndGenerativeForms(t *testing.T) {
	nodeParam := NewTypeParam("T", nil)
	node := NewGeneric("Node", []*TypeParam{nodeParam}, nil)
	node.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(node, nodeParam)}}}))
	if err := ValidateStaticGenericRecurrence(node); err != nil {
		t.Fatalf("productive Node<T> rejected: %v", err)
	}

	growParam := NewTypeParam("T", nil)
	grow := NewGeneric("Grow", []*TypeParam{growParam}, nil)
	grow.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(grow, NewArray(growParam))}}}))
	if err := ValidateStaticGenericRecurrence(grow); err != nil {
		t.Fatalf("productive generative Grow<T> rejected: %v", err)
	}
}

func TestValidateStaticGenericRecurrenceAdmitsOpaqueDeclarationAndScopedExternal(t *testing.T) {
	formal := NewTypeParam("T", nil)
	opaque := NewGeneric("Opaque", []*TypeParam{formal}, nil)
	if err := ValidateStaticGenericRecurrence(opaque); err != nil {
		t.Fatalf("opaque generic declaration rejected: %v", err)
	}

	external := NewTypeParam("E", String)
	if err := ValidateStaticGenericRecurrenceWithFormals(NewTuple(external, String), []*TypeParam{external}); err != nil {
		t.Fatalf("scoped external rejected: %v", err)
	}
	if err := ValidateStaticGenericRecurrence(NewTuple(external, String)); !errors.Is(err, ErrInvalidStaticGenericRecurrence) {
		t.Fatalf("closed admission accepted free formal: %v", err)
	}
}

func TestValidateStaticGenericRecurrenceAdmitsClosedBoxRoundTrip(t *testing.T) {
	formal := NewTypeParam("T", String)
	box := NewGeneric("Box", []*TypeParam{formal}, RebuildRecord(RecordParts{Fields: []Field{{Name: "value", Type: formal}}}))
	if err := ValidateStaticGenericRecurrence(box); err != nil {
		t.Fatalf("source Box rejected: %v", err)
	}
	receipt, err := EncodeCanonicalFormals(context.Background(), box, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), receipt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateStaticGenericRecurrence(decoded); err != nil {
		t.Fatalf("decoded Box rejected: %T %v", decoded, err)
	}
	if err := ValidateStaticGenericRecurrence(NewAlias("Open", box)); err != nil {
		t.Fatalf("alias Box rejected: %v", err)
	}

	methodParam := NewTypeParam("M", String)
	method := Func().TypeParamRef(methodParam).Param("value", methodParam).Returns(methodParam).Build()
	shape := MaterializeIntersection([]Type{
		RebuildRecord(RecordParts{Fields: []Field{{Name: "id", Type: Number}}}),
		NewInterface("Shape", []Method{{Name: "map", Type: method}}),
	})
	if err := ValidateStaticGenericRecurrence(shape); err != nil {
		t.Fatalf("generic interface projection rejected: %v", err)
	}
}

func TestValidateStaticGenericRecurrenceRejectsUnsupportedGenericGroups(t *testing.T) {
	firstParam := NewTypeParam("T", nil)
	secondParam := NewTypeParam("U", nil)
	first := NewGeneric("First", []*TypeParam{firstParam}, nil)
	second := NewGeneric("Second", []*TypeParam{secondParam}, nil)
	first.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(second, firstParam)}}}))
	second.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(first, secondParam)}}}))
	if err := ValidateStaticGenericRecurrence(first); !errors.Is(err, ErrInvalidStaticGenericRecurrence) {
		t.Fatalf("mutual generic group=%v, want recurrence error", err)
	}

	loopParam := NewTypeParam("T", nil)
	loop := NewGeneric("Loop", []*TypeParam{loopParam}, nil)
	loop.SetBody(Instantiate(loop, loopParam))
	if err := ValidateStaticGenericRecurrence(loop); !errors.Is(err, ErrInvalidStaticGenericRecurrence) {
		t.Fatalf("unproductive Loop<T>=%v, want recurrence error", err)
	}
}

func TestValidateStaticGenericRecurrenceRejectsArityAndForeignFormal(t *testing.T) {
	param := NewTypeParam("T", nil)
	generic := NewGeneric("One", []*TypeParam{param}, nil)
	generic.SetBody(NewArray(param))
	if err := ValidateStaticGenericRecurrence(Instantiate(generic)); !errors.Is(err, ErrInvalidStaticGenericRecurrence) {
		t.Fatalf("wrong generic arity=%v, want recurrence error", err)
	}

	external := NewTypeParam("External", nil)
	sibling := NewGeneric("Sibling", []*TypeParam{NewTypeParam("U", nil)}, NewArray(external))
	if err := ValidateStaticGenericRecurrence(sibling); !errors.Is(err, ErrInvalidStaticGenericRecurrence) {
		t.Fatalf("foreign formal=%v, want recurrence error", err)
	}
}

func TestValidateStaticGenericRecurrenceRejectsRecurrenceHiddenInUnusedConstraint(t *testing.T) {
	param := NewTypeParam("T", nil)
	generic := NewGeneric("Constrained", []*TypeParam{param}, String)
	// The body deliberately does not mention T. The declaration constraint is
	// nevertheless part of the type graph, so this is a bare G -> G equation
	// without an explicit Recursive/Mu boundary.
	param.Constraint = Instantiate(generic, param)
	if err := ValidateStaticGenericRecurrence(generic); !errors.Is(err, ErrInvalidStaticGenericRecurrence) {
		t.Fatalf("hidden constraint recurrence = %v, want recurrence error", err)
	}
}

func TestValidateStaticGenericRecurrenceExplicitMuDelimitsGenericRecurrence(t *testing.T) {
	firstParam := NewTypeParam("T", nil)
	secondParam := NewTypeParam("U", nil)
	first := NewGeneric("First", []*TypeParam{firstParam}, nil)
	second := NewGeneric("Second", []*TypeParam{secondParam}, nil)
	first.SetBody(NewRecursive("FirstBoundary", func(Type) Type {
		return Instantiate(second, firstParam)
	}))
	second.SetBody(Instantiate(first, secondParam))
	if err := ValidateStaticGenericRecurrence(first); err != nil {
		t.Fatalf("explicit Mu-delimited generic relation rejected: %v", err)
	}
}
