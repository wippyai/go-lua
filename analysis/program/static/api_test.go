package static

import "testing"

func TestCommitInputCanonicalStreamsAreConsumedAtPublicationBoundary(t *testing.T) {
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
	component, err := finalizer.Commit(input)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	input.TypeOf[0] = 0
	input.Annotations[0] = 0
	input.Publications[0] = 0
	if component == nil || component.ContentID() != wantID {
		t.Fatal("CommitInput mutation changed the published authored identity")
	}
	if got := component.View().Publications().Count(); got != 1 {
		t.Fatalf("published view lost publication relation after caller mutation: count=%d", got)
	}
}
