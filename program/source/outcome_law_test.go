package source

import (
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestSourceOutcomeIsDerivedFromCanonicalBodyOrigins(t *testing.T) {
	input, index := sourceFixture(2)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	index.OutcomeOrigins = []keyspace.Term{body}

	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	identity := component.View().Identity()
	if got := identity.FamilyCount(keyspace.FamilyOutcome); got != 1 {
		t.Fatalf("Outcome count = %d, want 1", got)
	}
	if got, want := identity.TermCount(), uint32(12); got != want {
		t.Fatalf("final TermCount = %d, want %d", got, want)
	}
	bodySpan, bodyOK := identity.Span(body)
	outcomeSpan, outcomeOK := identity.Span(outcome)
	if !bodyOK || !outcomeOK || bodySpan != outcomeSpan {
		t.Fatalf("Outcome span = %#v/%v, Body span = %#v/%v", outcomeSpan, outcomeOK, bodySpan, bodyOK)
	}
	if _, _, _, ok := component.View().Index().Position(outcome); ok {
		t.Fatal("Outcome unexpectedly acquired a source position")
	}
}

func TestSourceOutcomeDoesNotEnterAuthoredContentID(t *testing.T) {
	baseInput, baseIndex := sourceFixture(2)
	baseDraft, err := Build(baseInput)
	if err != nil {
		t.Fatalf("base Build: %v", err)
	}
	base, err := commitSource(baseDraft, baseIndex)
	if err != nil {
		t.Fatalf("base Finalize: %v", err)
	}

	input, index := sourceFixture(2)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	index.OutcomeOrigins = []keyspace.Term{body}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("derived Build: %v", err)
	}
	derived, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("derived Finalize: %v", err)
	}
	if got, want := derived.View().Identity().ContentID(), base.View().Identity().ContentID(); got != want {
		t.Fatalf("derived Outcome changed authored ContentID: %x != %x", got, want)
	}
}

func TestSourceRejectsAuthoredOutcomeSpans(t *testing.T) {
	input, _ := sourceFixture(1)
	input.Families[int(keyspace.FamilyOutcome)-1].Spans = []Span{{
		File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1,
	}}
	if _, err := Build(input); err == nil {
		t.Fatal("Build accepted authored Outcome span")
	}
}

func TestSourceRejectsMalformedOutcomeOriginAndPosition(t *testing.T) {
	input, index := sourceFixture(1)
	index.OutcomeOrigins = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 99)}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted foreign Outcome origin Body")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("failed Outcome finalization was retryable")
	}

	input, index = sourceFixture(1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	origin, ok := sourcePositionFor(index.Positions, keyspace.MakeTerm(keyspace.FamilyReturn, 1))
	if !ok {
		t.Fatal("Return position missing from fixture")
	}
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	origin.Term = outcome
	index.OutcomeOrigins = []keyspace.Term{body}
	appendCanonicalFixturePosition(&index, origin)
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build malformed position: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted an Outcome Position.Term")
	}
}

func TestSourceColdContainsOnlyContentID(t *testing.T) {
	if got, want := unsafe.Sizeof(Cold{}), unsafe.Sizeof(keyspace.ContentID{}); got != want {
		t.Fatalf("Cold size = %d, want ContentID size %d", got, want)
	}
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	cold := component.Cold()
	if got, want := cold.ContentID(), component.View().Identity().ContentID(); got != want {
		t.Fatalf("Cold ContentID = %x, want %x", got, want)
	}
	if got := testing.AllocsPerRun(1000, func() { _ = cold.ContentID() }); got != 0 {
		t.Fatalf("Cold.ContentID allocations = %f, want 0", got)
	}
	if got := (Cold{}).ContentID(); got.Available() {
		t.Fatal("zero Cold exposed identity")
	}
}

func TestSourceAllowsOmittedOutcomePositions(t *testing.T) {
	input, index := sourceFixture(1)
	index.OutcomeOrigins = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 2)}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if _, _, _, ok := component.View().Index().Position(outcome); ok {
		t.Fatal("omitted Outcome unexpectedly acquired a source position")
	}
}

func TestSourceAllowsRepeatedOutcomeOriginsInSuppliedOrder(t *testing.T) {
	input, index := sourceFixture(2)
	origins := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyBody, 2),
		keyspace.MakeTerm(keyspace.FamilyBody, 1),
		keyspace.MakeTerm(keyspace.FamilyBody, 2),
	}
	index.OutcomeOrigins = append([]keyspace.Term(nil), origins...)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	view := component.View()
	if got := view.Identity().FamilyCount(keyspace.FamilyOutcome); got != len(origins) {
		t.Fatalf("Outcome count = %d, want %d", got, len(origins))
	}
	for ordinal, body := range origins {
		outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(ordinal+1))
		bodySpan, bodyOK := view.Identity().Span(body)
		outcomeSpan, outcomeOK := view.Identity().Span(outcome)
		if !bodyOK || !outcomeOK || bodySpan != outcomeSpan {
			t.Fatalf("Outcome %v span = %#v/%v, Body %v span = %#v/%v", outcome, outcomeSpan, outcomeOK, body, bodySpan, bodyOK)
		}
		if _, _, _, ok := view.Index().Position(outcome); ok {
			t.Fatalf("Outcome %v unexpectedly acquired a source position", outcome)
		}
	}
}

func TestSourceOutcomeCommitCopiesCallerBatches(t *testing.T) {
	input, index := sourceFixture(2)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	index.OutcomeOrigins = []keyspace.Term{body}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	identity := component.View().Identity()
	wantSpan, wantSpanOK := identity.Span(outcome)
	wantRoot, wantOffset, wantCursor, wantPositionOK := component.View().Index().Position(body)
	wantContent := identity.ContentID()

	index.OutcomeOrigins[0] = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	index.Positions[0].Term = keyspace.MakeTerm(keyspace.FamilyCell, 1)
	index.Positions[0].Root = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	index.Positions[0].Offset++

	gotSpan, gotSpanOK := identity.Span(outcome)
	gotRoot, gotOffset, gotCursor, gotPositionOK := component.View().Index().Position(body)
	if gotSpan != wantSpan || gotSpanOK != wantSpanOK || identity.ContentID() != wantContent ||
		gotRoot != wantRoot || gotOffset != wantOffset || gotCursor != wantCursor || gotPositionOK != wantPositionOK {
		t.Fatalf("published Source changed after caller-batch mutation: span %#v/%v -> %#v/%v, position %v/%d/%d/%v -> %v/%d/%d/%v, ContentID %x -> %x", wantSpan, wantSpanOK, gotSpan, gotSpanOK, wantRoot, wantOffset, wantCursor, wantPositionOK, gotRoot, gotOffset, gotCursor, gotPositionOK, wantContent, identity.ContentID())
	}
}
