package axiscompose

import "testing"

func TestBoundaryMayMustExactRoundTripAndRebind(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.may.ID(), s.must.ID())
	arena := &Arena{}
	input := Put(arena, Bottom(arena, schema), s.may, uint8(3))
	input = Put(arena, input, s.must, uint8(6))
	projection := ProjectBoundary(input, ProjectCtx{Used: AllUsed(schema), Binding: Binding{Symbol: "callee.param0"}})
	if !projection.Fallback.Empty() {
		t.Fatalf("supported lanes requested fallback: %d", projection.Fallback.Count())
	}
	fallbackCalls := 0
	got, usedFallback := InstantiateBoundary(arena, projection, InstantiateCtx{Binding: Binding{Symbol: "caller.arg2"}}, func() State {
		fallbackCalls++
		return Bottom(arena, schema)
	})
	if usedFallback || fallbackCalls != 0 || !Equal(got, input) {
		t.Fatalf("round trip fallback=%v calls=%d equal=%v", usedFallback, fallbackCalls, Equal(got, input))
	}
}

func TestBoundaryUnsupportedUsedFallsBackAllOrNothing(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.may.ID(), s.unsupported.ID())
	arena := &Arena{}
	input := Put(arena, Bottom(arena, schema), s.may, uint8(3))
	input = Put(arena, input, s.unsupported, uint8(4))
	projection := ProjectBoundary(input, ProjectCtx{Used: AllUsed(schema), Binding: Binding{Symbol: "p"}})
	if projection.Fallback.Count() != 1 || len(projection.Lanes) != 1 {
		t.Fatalf("fallback=%d projected=%d", projection.Fallback.Count(), len(projection.Lanes))
	}
	contextualWant := Put(arena, Bottom(arena, schema), s.may, uint8(9))
	calls := 0
	got, fallback := InstantiateBoundary(arena, projection, InstantiateCtx{Binding: Binding{Symbol: "a"}}, func() State {
		calls++
		return contextualWant
	})
	if !fallback || calls != 1 || !Equal(got, contextualWant) {
		t.Fatalf("fallback=%v calls=%d equal contextual=%v", fallback, calls, Equal(got, contextualWant))
	}
}

func TestBoundaryUnsupportedUnusedNeedsCertifiedMask(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.may.ID(), s.unsupported.ID())
	arena := &Arena{}
	input := Put(arena, Bottom(arena, schema), s.may, uint8(5))
	input = Put(arena, input, s.unsupported, uint8(8))
	projection := ProjectBoundary(input, ProjectCtx{
		Used:    UsedAxes(schema, s.may.ID()),
		Binding: Binding{Symbol: "p"},
	})
	if !projection.Fallback.Empty() {
		t.Fatal("certified-unused unsupported lane forced fallback")
	}
	got, fallback := InstantiateBoundary(arena, projection, InstantiateCtx{Binding: Binding{Symbol: "a"}}, nil)
	if fallback {
		t.Fatal("unexpected fallback")
	}
	if may, _ := Get(got, s.may); may != 5 {
		t.Fatalf("may = %d, want 5", may)
	}
	if unsupported, _ := Get(got, s.unsupported); unsupported != 0 {
		t.Fatalf("unused unsupported = %d, want bottom", unsupported)
	}
}

func TestBoundaryMaskFromDifferentSchemaIsConservativelyRejected(t *testing.T) {
	s := newToySetup()
	full := mustSchema(t, &s.catalog, s.may.ID(), s.unsupported.ID())
	other := mustSchema(t, &s.catalog, s.may.ID())
	arena := &Arena{}
	input := Put(arena, Bottom(arena, full), s.unsupported, uint8(1))
	projection := ProjectBoundary(input, ProjectCtx{
		Used:    UsedAxes(other, s.may.ID()),
		Binding: Binding{Symbol: "p"},
	})
	if projection.Fallback.Count() != 1 {
		t.Fatalf("foreign used mask fallback=%d, want 1", projection.Fallback.Count())
	}
}

func TestBoundaryLateInstantiationFailureDiscardsPartialState(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.may.ID(), s.must.ID())
	arena := &Arena{}
	input := Put(arena, Bottom(arena, schema), s.may, uint8(3))
	projection := ProjectBoundary(input, ProjectCtx{Used: AllUsed(schema), Binding: Binding{Symbol: "p"}})
	// Corrupt the second payload so failure occurs after the first lane applied.
	projection.Lanes[1].Payload = struct{}{}
	want := Put(arena, Bottom(arena, schema), s.must, uint8(2))
	calls := 0
	got, fallback := InstantiateBoundary(arena, projection, InstantiateCtx{Binding: Binding{Symbol: "a"}}, func() State {
		calls++
		return want
	})
	if !fallback || calls != 1 || !Equal(got, want) {
		t.Fatal("late failure published partial state or called fallback incorrectly")
	}
}

func TestBoundaryInstantiationPreservesMayAndMustJoin(t *testing.T) {
	s := newToySetup()
	schema := mustSchema(t, &s.catalog, s.may.ID(), s.must.ID())
	arena := &Arena{}
	a := Put(arena, Bottom(arena, schema), s.may, uint8(1))
	a = Put(arena, a, s.must, uint8(0b1110))
	b := Put(arena, Bottom(arena, schema), s.may, uint8(2))
	b = Put(arena, b, s.must, uint8(0b1101))
	instantiate := func(value State) State {
		p := ProjectBoundary(value, ProjectCtx{Used: AllUsed(schema), Binding: Binding{Symbol: "p"}})
		got, fallback := InstantiateBoundary(arena, p, InstantiateCtx{Binding: Binding{Symbol: "a"}}, nil)
		if fallback {
			t.Fatal("exact toy boundary fell back")
		}
		return got
	}
	left := instantiate(Join(arena, a, b))
	right := Join(arena, instantiate(a), instantiate(b))
	if !Equal(left, right) {
		t.Fatal("project/instantiate does not preserve product join")
	}
}
