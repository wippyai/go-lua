package typeauthority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/kind"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestRuntimeFieldProjectionExactAndMetadata(t *testing.T) {
	record := typetable.NewRecord().
		Field("required", typ.String).
		OptReadonlyField("maybe", typ.Number).
		Build()
	runtime, inners := sealRuntimeFieldFixture(t, record)

	required, requiredOK := runtime.Field(inners[0], "required")
	if !requiredOK || !runtime.Equal(required.Inner, innersForKind(t, runtime, kind.String)) {
		t.Fatalf("required field = %#v/%v", required, requiredOK)
	}
	if required.Optional || required.Readonly {
		t.Fatalf("required metadata = optional=%v readonly=%v", required.Optional, required.Readonly)
	}

	maybe, maybeOK := runtime.Field(inners[0], "maybe")
	if !maybeOK {
		t.Fatal("optional readonly field is absent")
	}
	if runtimeKind(t, runtime, maybe.Inner) != kind.Number {
		t.Fatalf("maybe field kind = %v, want number", runtimeKind(t, runtime, maybe.Inner))
	}
	if !maybe.Optional || !maybe.Readonly {
		t.Fatalf("maybe metadata = optional=%v readonly=%v", maybe.Optional, maybe.Readonly)
	}
	if maybe.Child() != maybe.Inner || maybe.Type() != maybe.Inner {
		t.Fatal("field child accessors disagree with Inner")
	}
}

func TestRuntimeFieldProjectionMissingAndForeignAreAbsent(t *testing.T) {
	record := typetable.NewRecord().Field("name", typ.String).Build()
	runtime, inners := sealRuntimeFieldFixture(t, record)
	foreign, foreignInners := sealRuntimeFieldFixture(t, record)

	if _, ok := runtime.Field(inners[0], "missing"); ok {
		t.Fatal("missing field was projected")
	}
	if _, ok := runtime.Field(foreignInners[0], "name"); ok {
		t.Fatal("foreign parent inner crossed Runtime owner fence")
	}
	if _, ok := foreign.Field(inners[0], "name"); ok {
		t.Fatal("reverse foreign parent inner crossed Runtime owner fence")
	}
	if _, ok := runtime.Field(RuntimeInner{}, "name"); ok {
		t.Fatal("zero parent inner was projected")
	}
	if !foreign.Equal(foreignInners[0], foreignInners[0]) {
		t.Fatal("foreign fixture sanity check failed")
	}
}

func TestRuntimeFieldProjectionLookupIsAllocationFree(t *testing.T) {
	record := typetable.NewRecord().Field("name", typ.String).Build()
	runtime, inners := sealRuntimeFieldFixture(t, record)
	if allocations := testing.AllocsPerRun(200, func() {
		_, _ = runtime.Field(inners[0], "name")
	}); allocations != 0 {
		t.Fatalf("Field lookup allocated %.2f objects/run", allocations)
	}
}

func TestRuntimeFieldProjectionContributesDeterministicIdentity(t *testing.T) {
	withField := typetable.NewRecord().Field("name", typ.String).Build()
	withoutField := typetable.NewRecord().Build()

	withRuntime, _ := sealRuntimeFieldFixture(t, withField)
	withoutRuntime, _ := sealRuntimeFieldFixture(t, withoutField)
	withID, withOK := withRuntime.ContentID(), withRuntime.ContentID().Available()
	withoutID, withoutOK := withoutRuntime.ContentID(), withoutRuntime.ContentID().Available()
	if !withOK || !withoutOK {
		t.Fatal("Runtime identity unavailable")
	}
	if withID == withoutID {
		t.Fatal("field projection did not change Runtime identity")
	}

	replayedRuntime, _ := sealRuntimeFieldFixture(t, withField)
	if replayedRuntime.ContentID() != withID {
		t.Fatal("same field projection produced a nondeterministic Runtime identity")
	}
}

func sealRuntimeFieldFixture(t *testing.T, value typ.Type) (*Runtime, []RuntimeInner) {
	t.Helper()
	authority := &Authority{linkID: identity.ContentID{91}, artifact: &artifactAuthority{}}
	input, inputOK := authority.RuntimeInputForType(value)
	if !inputOK {
		t.Fatal("mint RuntimeInput")
	}
	runtime, inners, err := SealRuntime(authority, []RuntimeInput{input})
	if err != nil {
		t.Fatalf("SealRuntime: %v", err)
	}
	return runtime, inners
}

func innersForKind(t *testing.T, runtime *Runtime, want kind.Kind) RuntimeInner {
	t.Helper()
	for index := 1; index <= runtime.Count(); index++ {
		inner, ok := runtime.InnerAtIndex(uint32(index))
		if ok && runtimeKind(t, runtime, inner) == want {
			return inner
		}
	}
	t.Fatalf("Runtime has no %v row", want)
	return RuntimeInner{}
}

func runtimeKind(t *testing.T, runtime *Runtime, inner RuntimeInner) kind.Kind {
	t.Helper()
	got, ok := runtime.Kind(inner)
	if !ok {
		t.Fatal("read Runtime row kind")
	}
	return got
}
