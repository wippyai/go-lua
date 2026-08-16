package program

import (
	"testing"
)

func TestTransformerValuesCatalogAvailabilityDistinguishesValidEmptyAndMissing(t *testing.T) {
	published, err := Publish(rootAssembly(t, "transformer-values-empty.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	input := published.TransformerInput()
	view := input.Values()
	if !view.Available() || view.Count() != 0 {
		t.Fatalf("published empty Values catalog availability/count = %v/%d", view.Available(), view.Count())
	}
	original := published.valuesCatalog
	published.valuesCatalog = nil
	defer func() { published.valuesCatalog = original }()
	missing := input.Values()
	if missing.Available() || missing.Count() != 0 {
		t.Fatalf("missing Values catalog availability/count = %v/%d", missing.Available(), missing.Count())
	}
}

// The catalog is the sole immutable Values authority after publication. This
// law deliberately inspects the in-package seal so a future second directory
// or partial denominator cannot hide behind the public visitor.
func TestTransformerValuesCatalogCanonicalDenominatorAndPermutation(t *testing.T) {
	published := transformerStorageSpliceProgram(t, "transformer-values-catalog.lua")
	catalog := published.valuesCatalog
	if catalog == nil || !catalog.sealed || catalog.owner != published || catalog.owner.valuesCatalog != catalog || !validateValuesCatalog(catalog) {
		t.Fatal("Values catalog did not publish as the canonical sealed authority")
	}
	input := published.TransformerInput()
	view := input.Values()
	if view.Count() != len(catalog.rows) {
		t.Fatalf("Values count = %d, catalog rows = %d", view.Count(), len(catalog.rows))
	}
	for index := 0; index < view.Count(); index++ {
		values, ok := view.At(index)
		if !ok || !values.Available() {
			t.Fatalf("Values.At(%d) unavailable", index)
		}
		byID, ok := view.ForID(values.ID())
		if !ok || byID != values {
			t.Fatalf("Values.ForID did not replay row %d", index)
		}
		span, spanOK := values.Span()
		if spanOK != catalog.rows[index].spanOK {
			t.Fatalf("Values row %d span availability differs from sealed catalog", index)
		}
		if spanOK && (!input.OwnsSpan(span) || span != catalog.rows[index].span) {
			t.Fatalf("Values row %d did not return its exact retained span", index)
		}
		for memberIndex := 0; memberIndex < values.Count(); memberIndex++ {
			member, memberOK := values.At(memberIndex)
			if !memberOK || !member.Available() {
				t.Fatalf("Values.At(%d).At(%d) unavailable", index, memberIndex)
			}
			byMemberID, memberByIDOK := values.ForID(member.ID())
			if !memberByIDOK || byMemberID != member {
				t.Fatalf("Values.ForID did not replay row %d member %d", index, memberIndex)
			}
		}
	}
}

func TestTransformerValuesCatalogIDsAreSequenceSensitive(t *testing.T) {
	published := transformerStorageSpliceProgram(t, "transformer-values-sequence.lua")
	view := published.TransformerInput().Values()
	first, firstOK := view.At(0)
	second, secondOK := view.At(1)
	if !firstOK || !secondOK || first.ID() == second.ID() {
		t.Fatal("distinct Values rows shared a semantic ID")
	}
	firstMember, firstMemberOK := first.At(0)
	secondMember, secondMemberOK := second.At(0)
	if !firstMemberOK || !secondMemberOK || firstMember.ID() == secondMember.ID() {
		t.Fatal("distinct Values member roots shared a semantic ID")
	}
}

func TestTransformerValuesCatalogRejectsForeignReplayAndSplice(t *testing.T) {
	left := transformerStorageSpliceProgram(t, "transformer-values-replay.lua")
	right := transformerStorageSpliceProgram(t, "transformer-values-replay.lua")
	leftInput, rightInput := left.TransformerInput(), right.TransformerInput()
	leftValues, leftOK := leftInput.Values().At(0)
	rightValues, rightOK := rightInput.Values().At(0)
	if !leftOK || !rightOK || leftValues.ID() != rightValues.ID() {
		t.Fatal("equivalent replay did not produce equivalent Values IDs")
	}
	if leftInput.OwnsValuesOccurrence(rightValues) || rightInput.OwnsValuesOccurrence(leftValues) {
		t.Fatal("equivalent replay crossed the canonical catalog owner fence")
	}
	spliced := leftValues
	spliced.catalog = right.valuesCatalog
	if spliced.Available() || leftInput.OwnsValuesOccurrence(spliced) {
		t.Fatal("same-owner Values handle accepted a foreign catalog splice")
	}
	member, memberOK := leftValues.At(0)
	_, foreignMemberOK := rightValues.At(0)
	if !memberOK || !foreignMemberOK {
		t.Fatal("fixture did not publish Values members")
	}
	member.values = rightValues
	if member.Available() || leftInput.OwnsValuesMember(member) {
		t.Fatal("Values member accepted a foreign parent receipt")
	}
}

func TestTransformerValuesCatalogTailOwnership(t *testing.T) {
	published := transformerStorageSpliceProgram(t, "transformer-values-tail.lua")
	view := published.TransformerInput().Values()
	for index := 0; index < view.Count(); index++ {
		values, ok := view.At(index)
		if !ok {
			t.Fatalf("Values.At(%d) unavailable", index)
		}
		producer, open := values.Tail()
		if !open && producer.Available() {
			t.Fatalf("Values row %d returned a producer for a closed tail", index)
		}
		if open && (!producer.Available() || !published.TransformerInput().OwnsTailProducer(producer)) {
			t.Fatalf("Values row %d returned an unowned tail producer", index)
		}
	}
}

func TestTransformerValuesCatalogHotQueriesDoNotAllocate(t *testing.T) {
	published := transformerStorageSpliceProgram(t, "transformer-values-hot.lua")
	view := published.TransformerInput().Values()
	values, ok := view.At(0)
	if !ok {
		t.Fatal("fixture did not publish a Values row")
	}
	id := values.ID()
	allocs := testing.AllocsPerRun(10000, func() {
		got, gotOK := view.ForID(id)
		if !gotOK || !got.Available() {
			t.Fatal("hot Values lookup failed")
		}
		_, _ = got.At(0)
		_, _ = got.Tail()
	})
	if allocs != 0 {
		t.Fatalf("hot Values catalog queries allocated %v times per run", allocs)
	}
}
