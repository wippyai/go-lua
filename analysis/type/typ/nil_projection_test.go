package typ

import "testing"

func TestWithoutNilDirectNil(t *testing.T) {
	got, nilable := WithoutNil(Nil, NilProjectionStructural)
	if !nilable {
		t.Fatal("expected nilable")
	}
	if got != Never {
		t.Fatalf("WithoutNil(nil) = %v, want never", got)
	}
}

func TestWithoutNilLeavesAbsentTypeAlone(t *testing.T) {
	got, nilable := WithoutNil(nil, NilProjectionStructural)
	if nilable {
		t.Fatal("did not expect nilable")
	}
	if got != nil {
		t.Fatalf("WithoutNil(nil Type) = %v, want nil", got)
	}
}

func TestWithoutNilOptionalAndUnion(t *testing.T) {
	got, nilable := WithoutNil(NewOptional(String), NilProjectionStructural)
	if !nilable {
		t.Fatal("expected optional string to be nilable")
	}
	if !TypeEquals(got, String) {
		t.Fatalf("WithoutNil(string?) = %v, want string", got)
	}

	union := NewUnion(String, Boolean, Nil)
	got, nilable = WithoutNil(union, NilProjectionStructural)
	if !nilable {
		t.Fatal("expected union with nil to be nilable")
	}
	want := NewUnion(String, Boolean)
	if !TypeEquals(got, want) {
		t.Fatalf("WithoutNil(string | boolean | nil) = %v, want %v", got, want)
	}
}

func TestWithoutNilStructuralUnwrapsNilableAlias(t *testing.T) {
	maybeString := NewAlias("MaybeString", NewOptional(String))
	got, nilable := WithoutNil(maybeString, NilProjectionStructural)
	if !nilable {
		t.Fatal("expected alias to optional to be nilable")
	}
	if !TypeEquals(got, String) {
		t.Fatalf("WithoutNil(MaybeString) = %v, want string", got)
	}
	if _, ok := got.(*Alias); ok {
		t.Fatalf("structural projection returned alias %v, want payload", got)
	}
}

func TestWithoutNilPreserveAliasesRebuildsNilableAlias(t *testing.T) {
	maybeString := NewAlias("MaybeString", NewOptional(String))
	got, nilable := WithoutNil(maybeString, NilProjectionPreserveAliases)
	if !nilable {
		t.Fatal("expected alias to optional to be nilable")
	}
	alias, ok := got.(*Alias)
	if !ok {
		t.Fatalf("WithoutNil(MaybeString) = %T, want alias", got)
	}
	if alias.Name != "MaybeString" {
		t.Fatalf("alias name = %q, want MaybeString", alias.Name)
	}
	if !TypeEquals(alias.Target, String) {
		t.Fatalf("alias target = %v, want string", alias.Target)
	}
}

func TestWithoutNilAnnotatedOptional(t *testing.T) {
	ann := []Annotation{{Name: "tag"}}
	got, nilable := WithoutNil(NewAnnotated(NewOptional(String), ann), NilProjectionStructural)
	if !nilable {
		t.Fatal("expected annotated optional to be nilable")
	}
	annotated, ok := got.(*Annotated)
	if !ok {
		t.Fatalf("WithoutNil(annotated optional) = %T, want annotated", got)
	}
	if !TypeEquals(annotated.Inner, String) {
		t.Fatalf("annotated inner = %v, want string", annotated.Inner)
	}
}
