package source

import (
	"sync"
	"testing"
)

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
	span := Span{File: input.Name, StartLine: 3, StartCol: 2, EndLine: 4, EndCol: 1}
	coordinate, ok := CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	if !ok {
		t.Fatal("valid coordinate rejected")
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
	if got := testing.AllocsPerRun(1000, func() {
		_, _ = CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	}); got != 0 {
		t.Fatalf("CoordinateFromParts allocations = %v, want 0", got)
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
}
