package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func staticFixture(t *testing.T) Input {
	t.Helper()
	input := publicationFixture(t)
	input.Counts[keyspace.FamilyCell] = 1
	input.Counts[keyspace.FamilyRead] = 1
	input.Counts[keyspace.FamilyTypeOf] = 2
	input.Counts[keyspace.FamilyValues] = 1
	input.Counts[keyspace.FamilyValueClaim] = 2
	input.Counts[keyspace.FamilyAnnotation] = 2
	input.Operators.TypeOf = []TypeOf{
		{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
		{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
	}
	input.Operands.Annotation = []Annotation{
		{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 1, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
		{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 2, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
	}
	return input
}

func validCommitInputForFixture() CommitInput {
	return CommitInput{
		TypeOf:       canonicalCommitTerms(keyspace.FamilyTypeOf, 2),
		Annotations:  canonicalCommitTerms(keyspace.FamilyAnnotation, 2),
		Publications: canonicalCommitTerms(keyspace.FamilyTypePublication, 1),
	}
}

func TestStaticCommitAcceptsExactCanonicalInputAndRetainsNone(t *testing.T) {
	draft, err := Build(staticFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	wantID := draft.state.component.ContentID()
	if got := view.ContentID(); got != wantID {
		t.Fatalf("claimed View.ContentID() = %x, want %x", got, wantID)
	}
	copiedView := view
	if got := copiedView.ContentID(); got != wantID {
		t.Fatalf("copied claimed View.ContentID() = %x, want %x", got, wantID)
	}
	input := validCommitInputForFixture()
	component, err := finalizer.Commit(input)
	if err != nil {
		t.Fatalf("Commit(exact input) error = %v", err)
	}
	input.TypeOf[0] = 0
	input.Annotations[0] = 0
	input.Publications[0] = 0
	if component == nil {
		t.Fatal("Commit returned nil Component for exact input")
	}
	if component.ContentID() != wantID {
		t.Fatalf("Commit changed authored identity: got %x want %x", component.ContentID(), wantID)
	}
	if view.Available() {
		t.Fatal("construction View remained available after Commit")
	}
	if got := view.ContentID(); got.Available() {
		t.Fatalf("committed construction View retained ContentID %x", got)
	}
	if got := copiedView.ContentID(); got.Available() {
		t.Fatalf("copied committed construction View retained ContentID %x", got)
	}
	if !component.View().Available() {
		t.Fatal("published Component View unavailable")
	}
	if got := component.View().ContentID(); got != wantID {
		t.Fatalf("published View.ContentID() = %x, want %x", got, wantID)
	}
}

func TestStaticCommitRejectsNonCanonicalInputsAndClosesTerminalState(t *testing.T) {
	cases := []struct {
		name string
		edit func(*CommitInput)
	}{
		{"missing TypeOf", func(input *CommitInput) { input.TypeOf = input.TypeOf[:1] }},
		{"permuted Annotations", func(input *CommitInput) {
			input.Annotations[0], input.Annotations[1] = input.Annotations[1], input.Annotations[0]
		}},
		{"foreign Publication family", func(input *CommitInput) { input.Publications[0] = keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) }},
		{"duplicate TypeOf", func(input *CommitInput) { input.TypeOf[1] = input.TypeOf[0] }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			draft, err := Build(staticFixture(t))
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			finalizer, err := draft.Finalizer()
			if err != nil {
				t.Fatalf("Finalizer() error = %v", err)
			}
			view := finalizer.View()
			copied := finalizer
			wantID := view.ContentID()
			if !wantID.Available() {
				t.Fatal("claimed View did not expose ContentID")
			}
			input := validCommitInputForFixture()
			test.edit(&input)
			if _, err := finalizer.Commit(input); err == nil {
				t.Fatal("Commit accepted invalid canonical input")
			}
			if view.Available() {
				t.Fatal("invalid Commit left construction View available")
			}
			if got := view.ContentID(); got.Available() {
				t.Fatalf("invalid Commit left construction View ContentID %x", got)
			}
			if _, err := copied.Commit(validCommitInputForFixture()); err == nil {
				t.Fatal("copied Finalizer retried after invalid terminal Commit")
			}
			if _, err := draft.Finalizer(); err == nil {
				t.Fatal("Draft reopened after invalid terminal Commit")
			}
		})
	}
}

func TestStaticCommitRejectsForeignInputDenominatorWithoutChangingContent(t *testing.T) {
	draft, err := Build(staticFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	wantID := draft.state.component.ContentID()
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	input := validCommitInputForFixture()
	input.Publications = append(input.Publications, keyspace.MakeTerm(keyspace.FamilyTypePublication, 2))
	component, err := finalizer.Commit(input)
	if err == nil || component != nil {
		t.Fatalf("Commit accepted extra input row: component=%v err=%v", component, err)
	}
	if got := draft.state.component; got != nil {
		t.Fatal("invalid Commit retained construction Component")
	}
	if got := wantID; !got.Available() {
		t.Fatal("invalid input test lost pre-commit identity")
	}
}

func TestStaticViewContentIDExpiresOnAbortCopies(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	copied := view
	want := view.ContentID()
	if !want.Available() {
		t.Fatal("claimed View did not expose ContentID")
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if got := view.ContentID(); got.Available() {
		t.Fatalf("aborted View retained ContentID %x", got)
	}
	if got := copied.ContentID(); got.Available() {
		t.Fatalf("copied aborted View retained ContentID %x", got)
	}
}
