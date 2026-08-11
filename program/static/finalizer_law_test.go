package static

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

// commitStaticDraft is the test-only spelling of the complete owner
// lifecycle. Production callers must retain the Finalizer while validating
// its View and then choose exactly one terminal operation.
func commitStaticDraft(t *testing.T, draft *Draft) (*Component, error) {
	t.Helper()
	finalizer, err := draft.Finalizer()
	if err != nil {
		return nil, err
	}
	return finalizer.Commit(commitInputForDraft(draft))
}

func commitInputForDraft(draft *Draft) CommitInput {
	component := draft.state.component
	return CommitInput{
		TypeOf:       canonicalCommitTerms(keyspace.FamilyTypeOf, len(component.operators.typeOf)),
		Annotations:  canonicalCommitTerms(keyspace.FamilyAnnotation, len(component.operands.annotations)),
		Publications: canonicalCommitTerms(keyspace.FamilyTypePublication, len(component.publications)),
	}
}

func canonicalCommitTerms(family keyspace.Family, count int) []keyspace.Term {
	terms := make([]keyspace.Term, count)
	for index := range terms {
		terms[index] = keyspace.MakeTerm(family, uint32(index+1))
	}
	return terms
}

func primitiveDraft(t *testing.T) *Draft {
	t.Helper()
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	draft, err := Build(Input{
		Counts: counts,
		Types:  TypesInput{Primitive: []Primitive{{Kind: PrimitiveAny}}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return draft
}

func emptyDraft(t *testing.T) *Draft {
	t.Helper()
	draft, err := Build(Input{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return draft
}

func TestStaticViewAvailabilityLaws(t *testing.T) {
	if (View{}).Available() {
		t.Fatal("zero View is available")
	}
	var nilComponent *Component
	if nilComponent.View().Available() {
		t.Fatal("nil Component View is available")
	}

	draft := emptyDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	if !view.Available() {
		t.Fatal("live empty Finalizer View is unavailable")
	}
	if view.Types().Primitives().Count() != 0 {
		t.Fatal("live empty Finalizer View gained primitive rows")
	}

	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if view.Available() {
		t.Fatal("committed Finalizer View remained available")
	}
	if !component.View().Available() {
		t.Fatal("published Component View is unavailable")
	}

	abortDraft := emptyDraft(t)
	aborted, err := abortDraft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	abortedView := aborted.View()
	if !abortedView.Available() {
		t.Fatal("second live empty Finalizer View is unavailable")
	}
	if err := aborted.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if abortedView.Available() {
		t.Fatal("aborted Finalizer View remained available")
	}
}

func TestStaticFinalizerClaimsViewAndCommitsOnce(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	copiedView := view
	if !view.Available() || !copiedView.Available() {
		t.Fatal("live Finalizer View copy is unavailable")
	}
	if got := view.Types().Primitives().Count(); got != 1 {
		t.Fatalf("finalizer View primitive count = %d, want 1", got)
	}
	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if component == nil || !component.ContentID().Available() {
		t.Fatal("Commit() did not publish the sealed component")
	}
	if got := view.Types().Primitives().Count(); got != 0 {
		t.Fatalf("committed Finalizer View retained %d primitive rows", got)
	}
	if view.Available() || copiedView.Available() {
		t.Fatal("committed Finalizer View copy remained available")
	}
	if got := copiedView.Types().Primitives().Count(); got != 0 {
		t.Fatalf("copied committed Finalizer View retained %d primitive rows", got)
	}
	if got := component.View().Types().Primitives().Count(); got != 1 {
		t.Fatalf("ordinary sealed Component View lost %d primitive rows", got)
	}
	if _, err := finalizer.Commit(CommitInput{}); err == nil {
		t.Fatal("copied Finalizer committed twice")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("Draft was claimable after Commit")
	}
}

func TestStaticFinalizerAbortIsTerminal(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	copiedView := view
	if !view.Available() || !copiedView.Available() {
		t.Fatal("live Finalizer View copy is unavailable")
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if got := view.Types().Primitives().Count(); got != 0 {
		t.Fatalf("aborted Finalizer View retained %d primitive rows", got)
	}
	if view.Available() || copiedView.Available() {
		t.Fatal("aborted Finalizer View copy remained available")
	}
	if got := copiedView.Types().Primitives().Count(); got != 0 {
		t.Fatalf("copied aborted Finalizer View retained %d primitive rows", got)
	}
	if _, err := finalizer.Commit(CommitInput{}); err == nil {
		t.Fatal("aborted Finalizer committed")
	}
	if err := finalizer.Abort(); err == nil {
		t.Fatal("aborted Finalizer aborted twice")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("Draft was claimable after Abort")
	}
}

func TestStaticFinalizerDraftCopiesClaimExactlyOnceUnderContention(t *testing.T) {
	draft := primitiveDraft(t)
	const contenders = 32
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var group sync.WaitGroup
	group.Add(contenders)
	for range contenders {
		copy := *draft
		go func() {
			defer group.Done()
			<-start
			finalizer, err := copy.Finalizer()
			if err != nil {
				results <- false
				return
			}
			_, err = finalizer.Commit(CommitInput{})
			results <- err == nil
		}()
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("contended Finalizer commits = %d, want exactly 1", successes)
	}
}

func TestStaticFinalizerCopiesTerminalActionExactlyOnceUnderContention(t *testing.T) {
	draft := primitiveDraft(t)
	claimed, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	const contenders = 32
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var group sync.WaitGroup
	group.Add(contenders)
	for range contenders {
		copy := claimed
		go func() {
			defer group.Done()
			<-start
			_, err := copy.Commit(CommitInput{})
			results <- err == nil
		}()
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("contended copied Finalizer commits = %d, want exactly 1", successes)
	}
}

func TestStaticFinalizerViewReadConcurrentCommit(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		for range 1000 {
			_ = view.Available()
			_ = view.Types().Primitives().Count()
			_, _ = view.Types().Primitives().Get(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))
		}
	}()
	go func() {
		defer group.Done()
		<-start
		_, _ = finalizer.Commit(CommitInput{})
	}()
	close(start)
	group.Wait()
}

func TestStaticFinalizerViewReadConcurrentAbort(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		for range 1000 {
			_ = view.Available()
			_ = view.Types().Primitives().Count()
			_, _ = view.Types().Primitives().Get(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))
		}
	}()
	go func() {
		defer group.Done()
		<-start
		_ = finalizer.Abort()
	}()
	close(start)
	group.Wait()
}

func TestStaticColdRetainsOnlyContentIDSnapshot(t *testing.T) {
	component := staticContentComponent(t, Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyTypePrimitive: 1},
		Types:  TypesInput{Primitive: []Primitive{{Kind: PrimitiveAny}}},
	})
	cold := component.Cold()
	want := cold.ContentID()
	component.contentID = keyspace.ContentID{}
	if got := cold.ContentID(); got != want {
		t.Fatalf("Cold snapshot changed after Component mutation: %x != %x", got, want)
	}
}
