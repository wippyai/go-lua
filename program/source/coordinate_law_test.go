package source

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestCoordinateTokenRoundTrip(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	coordinates := draft.Coordinates()
	cases := []Span{
		{File: input.Name, StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 7},
		{File: input.Name, StartLine: 2, StartCol: 3, EndLine: 4, EndCol: 2},
		{File: input.Name, StartLine: 7, StartCol: 9, EndLine: 7, EndCol: 9},
		{File: input.Name, StartLine: 9, StartCol: 1},
	}
	var compact []Coordinate
	for _, want := range cases {
		got, ok := coordinates.Token(want)
		if !ok {
			t.Fatalf("Token(%#v) rejected valid token", want)
		}
		if sl, sc, el, ec := got.Parts(); sl != want.StartLine || sc != want.StartCol || el != want.EndLine || ec != want.EndCol {
			t.Fatalf("Parts() = %d:%d-%d:%d, want %#v", sl, sc, el, ec, want)
		}
		compact = append(compact, got)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatal(err)
	}
	for at, want := range cases {
		got, ok := component.View().Identity().Render(compact[at])
		if !ok || got != want {
			t.Fatalf("Render(%d) = %#v/%v, want %#v", at, got, ok, want)
		}
	}
}

func TestCoordinateRejectsInvalidAndForeignTokens(t *testing.T) {
	input, _ := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	coordinates := draft.Coordinates()
	for _, span := range []Span{
		{},
		{File: "foreign.lua", StartLine: 1, StartCol: 1},
		{File: input.Name},
		{File: input.Name, StartCol: 1},
		{File: input.Name, StartLine: 1},
		{File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1},
		{File: input.Name, StartLine: 2, StartCol: 2, EndLine: 2, EndCol: 1},
		{File: input.Name, StartLine: 2, StartCol: 1, EndLine: 1, EndCol: 9},
	} {
		if got, ok := coordinates.Token(span); ok || got != (Coordinate{}) {
			t.Fatalf("Token(%#v) = %#v/%v, want rejection", span, got, ok)
		}
	}
}

func TestCoordinatePartsValidationAndZeroRender(t *testing.T) {
	valid := []struct {
		parts [4]uint32
		zero  bool
	}{
		{parts: [4]uint32{0, 0, 0, 0}, zero: true},
		{parts: [4]uint32{1, 1, 0, 0}},
		{parts: [4]uint32{1, 2, 1, 2}},
		{parts: [4]uint32{1, 2, 4, 1}},
	}
	for _, test := range valid {
		coordinate, ok := CoordinateFromParts(test.parts[0], test.parts[1], test.parts[2], test.parts[3])
		if !ok {
			t.Fatalf("CoordinateFromParts(%v) rejected valid shape", test.parts)
		}
		sl, sc, el, ec := coordinate.Parts()
		if got := [4]uint32{sl, sc, el, ec}; got != test.parts {
			t.Fatalf("Parts = %v, want %v", got, test.parts)
		}
		if (coordinate == (Coordinate{})) != test.zero {
			t.Fatalf("zero status for %v is wrong", test.parts)
		}
	}
	for _, parts := range [][4]uint32{
		{0, 1, 0, 0}, {1, 0, 0, 0}, {1, 1, 2, 0}, {1, 1, 0, 2}, {2, 2, 1, 9}, {2, 2, 2, 1},
	} {
		if coordinate, ok := CoordinateFromParts(parts[0], parts[1], parts[2], parts[3]); ok || coordinate != (Coordinate{}) {
			t.Fatalf("CoordinateFromParts(%v) = %#v/%v, want rejection", parts, coordinate, ok)
		}
	}
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatal(err)
	}
	if span, ok := component.View().Identity().Render(Coordinate{}); ok || span != (Span{}) {
		t.Fatalf("Render(zero) = %#v/%v, want rejection", span, ok)
	}
}

func TestCoordinateOperationsDoNotAllocate(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	coordinates := draft.Coordinates()
	span := Span{File: input.Name, StartLine: 3, StartCol: 2, EndLine: 4, EndCol: 1}
	coordinate, ok := coordinates.Token(span)
	if !ok {
		t.Fatal("valid token rejected")
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatal(err)
	}
	identity := component.View().Identity()
	if got := testing.AllocsPerRun(1000, func() { _, _, _, _ = coordinate.Parts() }); got != 0 {
		t.Fatalf("Parts allocations = %v, want 0", got)
	}
	if got := testing.AllocsPerRun(1000, func() { _, _ = identity.Render(coordinate) }); got != 0 {
		t.Fatalf("Render allocations = %v, want 0", got)
	}

	input, _ = sourceFixture(1)
	draft, err = Build(input)
	if err != nil {
		t.Fatal(err)
	}
	coordinates = draft.Coordinates()
	if got := testing.AllocsPerRun(1000, func() { _, _ = coordinates.Token(span) }); got != 0 {
		t.Fatalf("Token allocations = %v, want 0", got)
	}
}

func TestCopiedDraftConcurrentFinalizeHasOneConsumer(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	const width = 32
	start := make(chan struct{})
	results := make(chan error, width)
	var group sync.WaitGroup
	for at := 0; at < width; at++ {
		copy := *draft
		group.Add(1)
		go func(candidate Draft) {
			defer group.Done()
			<-start
			_, err := commitSource(&candidate, index)
			results <- err
		}(copy)
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful copied finalizations = %d, want 1", successes)
	}
	if _, ok := draft.FindKey(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}); ok {
		t.Fatal("consumed Draft retained key authority")
	}
}

func TestCoordinateSynchronizesWithFinalizeAndIsConsumed(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	coordinates := draft.Coordinates()
	span := Span{File: input.Name, StartLine: 3, StartCol: 2}
	start := make(chan struct{})
	var group sync.WaitGroup
	for at := 0; at < 32; at++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, _ = coordinates.Token(span)
		}()
	}
	finalized := make(chan error, 1)
	copy := *draft
	group.Add(1)
	go func() {
		defer group.Done()
		<-start
		_, err := commitSource(&copy, index)
		finalized <- err
	}()
	close(start)
	group.Wait()
	if err := <-finalized; err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if coordinate, ok := coordinates.Token(span); ok || coordinate != (Coordinate{}) {
		t.Fatalf("post-consume Token = %#v/%v, want rejection", coordinate, ok)
	}
}
