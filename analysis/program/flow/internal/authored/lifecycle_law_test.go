package authored

import (
	"sync"
	"testing"
)

func TestFinalizerClaimsCopiedDraftAndCapturedViewExpiresAfterCommit(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	copyDraft := *draft
	finalizer, err := copyDraft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("copied Draft acquired a second Finalizer")
	}
	captured := finalizer.View()
	if captured.Values().Count() == 0 {
		t.Fatal("claimed View did not expose authored rows")
	}
	committed, err := finalizer.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if captured.Values().Count() != 0 || captured.Cold().ContentID().Available() {
		t.Fatal("captured View remained live after Commit")
	}
	if committed.Values().Count() == 0 || !committed.Cold().ContentID().Available() {
		t.Fatal("committed View did not survive terminal transition")
	}
	if _, err := finalizer.Commit(); err == nil {
		t.Fatal("terminal Finalizer accepted a second Commit")
	}
	if err := finalizer.Abort(); err == nil {
		t.Fatal("terminal Finalizer accepted Abort after Commit")
	}
}

func TestCapturedViewExpiresAfterAbort(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	captured := finalizer.View()
	if captured.Tables().Count() == 0 {
		t.Fatal("claimed View did not expose authored tables")
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if captured.Tables().Count() != 0 || captured.Cold().ContentID().Available() {
		t.Fatal("captured View remained live after Abort")
	}
	if _, err := finalizer.Commit(); err == nil {
		t.Fatal("terminal Finalizer accepted Commit after Abort")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("Draft reopened after Abort")
	}
}

func TestCopiedFinalizersConcurrentCommitHaveOneConsumer(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	const callers = 32
	start := make(chan struct{})
	results := make(chan error, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		candidate := finalizer
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, commitErr := candidate.Commit()
			results <- commitErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	for commitErr := range results {
		if commitErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful copied Finalizer commits = %d, want 1", successes)
	}
}

func TestConcurrentReadsAndTerminalCommitDoNotPanic(t *testing.T) {
	input, _ := flowFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	captured := finalizer.View()
	const readers = 32
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(readers + 1)
	for index := 0; index < readers; index++ {
		go func() {
			defer group.Done()
			<-start
			for repeat := 0; repeat < 128; repeat++ {
				captured.Values().Count()
				captured.Tables().At(0)
				captured.Operators().Unaries().Get(0)
			}
		}()
	}
	var committed View
	var commitErr error
	go func() {
		defer group.Done()
		<-start
		committed, commitErr = finalizer.Commit()
	}()
	close(start)
	group.Wait()
	if commitErr != nil {
		t.Fatalf("concurrent Commit: %v", commitErr)
	}
	if committed.Values().Count() == 0 {
		t.Fatal("committed View lost rows after concurrent reads")
	}
	if captured.Values().Count() != 0 {
		t.Fatal("captured View remained live after concurrent Commit")
	}
}
