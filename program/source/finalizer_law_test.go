package source

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestFinalizerClaimsCopiedDraftAndExpiresAuthoredViews(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	copy := *draft
	finalizer, err := copy.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("copied Draft acquired a second Finalizer")
	}
	preimage := finalizer.Preimage()
	if got, want := preimage.Identity().Name(), input.Name; got != want {
		t.Fatalf("Preimage.Identity.Name = %q, want %q", got, want)
	}
	if got, ok := preimage.Order().BodyLen(keyspace.MakeTerm(keyspace.FamilyBody, 1)); !ok || got != 1 {
		t.Fatalf("Preimage.Order.BodyLen = %d/%v, want 1/true", got, ok)
	}
	if got, ok := preimage.Binds().Len(keyspace.MakeTerm(keyspace.FamilyBind, 1)); !ok || got != 1 {
		t.Fatalf("Preimage.Binds.Len = %d/%v, want 1/true", got, ok)
	}
	if got, ok := preimage.Formals().Len(keyspace.MakeTerm(keyspace.FamilyFunction, 1)); !ok || got != 1 {
		t.Fatalf("Preimage.Formals.Len = %d/%v, want 1/true", got, ok)
	}
	if _, ok := preimage.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}); ok {
		t.Fatal("fixture unexpectedly has an exact integer key")
	}
	if got := preimage.Faults().Count(); got != 0 {
		t.Fatalf("Preimage.Faults.Count = %d, want 0", got)
	}
	if got := preimage.Literals().Nils().Count(); got != 1 {
		t.Fatalf("Preimage.Literals.Nils.Count = %d, want 1", got)
	}
	identity := preimage.Identity()
	order := preimage.Order()
	binds := preimage.Binds()
	formals := preimage.Formals()
	keys := preimage.Keys()
	faults := preimage.Faults()
	nils := preimage.Literals().Nils()
	if _, err := finalizer.Commit(ownedIndex(draft, index)); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got := identity.Name(); got != "" {
		t.Fatalf("post-Commit captured Identity.Name = %q, want empty", got)
	}
	if _, ok := order.BodyLen(keyspace.MakeTerm(keyspace.FamilyBody, 1)); ok {
		t.Fatal("post-Commit captured Order remained live")
	}
	if _, ok := binds.Len(keyspace.MakeTerm(keyspace.FamilyBind, 1)); ok {
		t.Fatal("post-Commit captured Binds remained live")
	}
	if _, ok := formals.Len(keyspace.MakeTerm(keyspace.FamilyFunction, 1)); ok {
		t.Fatal("post-Commit captured Formals remained live")
	}
	if _, ok := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}); ok {
		t.Fatal("post-Commit captured Keys remained live")
	}
	if got := faults.Count(); got != 0 {
		t.Fatalf("post-Commit captured Faults.Count = %d, want 0", got)
	}
	if got := nils.Count(); got != 0 {
		t.Fatalf("post-Commit captured Literals remained live: nil count %d", got)
	}
	if _, _, ok := nils.At(0); ok {
		t.Fatal("post-Commit captured literal row remained live")
	}
	if key, ok := draft.FindKey(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 1}); ok || key != 0 {
		t.Fatalf("post-Commit Draft.FindKey = %v/%v, want unavailable", key, ok)
	}
}

func TestCopiedFinalizersConcurrentCommitHaveOneConsumer(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	index = ownedIndex(draft, index)
	const width = 32
	start := make(chan struct{})
	results := make(chan error, width)
	var group sync.WaitGroup
	for at := 0; at < width; at++ {
		copy := finalizer
		group.Add(1)
		go func(candidate Finalizer) {
			defer group.Done()
			<-start
			_, err := candidate.Commit(index)
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
		t.Fatalf("successful copied Finalizer commits = %d, want 1", successes)
	}
	if err := finalizer.Abort(); err == nil {
		t.Fatal("terminal Finalizer accepted Abort after Commit")
	}
}

func TestFinalizerAbortIsTerminal(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if err := finalizer.Abort(); err == nil {
		t.Fatal("Abort was not terminal")
	}
	if _, err := finalizer.Commit(ownedIndex(draft, index)); err == nil {
		t.Fatal("Commit succeeded after Abort")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("Draft acquired Finalizer after Abort")
	}
}

func TestFinalizerRejectsForeignBuildIndex(t *testing.T) {
	input, ownIndex := sourceFixture(1)
	foreignInput, foreignIndex := sourceFixture(1)
	// Keep the foreign build valid while changing its authored Source identity.
	// The same typed terms then expose whether Commit accepts a seal batch
	// issued by another owner.
	foreignInput.Name = "foreign.lua"
	for family := range foreignInput.Families {
		for at := range foreignInput.Families[family].Spans {
			if foreignInput.Families[family].Spans[at].File != "" {
				foreignInput.Families[family].Spans[at].File = foreignInput.Name
			}
		}
	}
	foreignDraft, err := Build(foreignInput)
	if err != nil {
		t.Fatalf("foreign Build: %v", err)
	}
	foreignIndex = ownedIndex(foreignDraft, foreignIndex)
	if foreignFinalizer, finalizerErr := foreignDraft.Finalizer(); finalizerErr == nil {
		_ = foreignFinalizer.Abort()
	}
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalizer.Commit(foreignIndex); err == nil {
		t.Fatal("Commit accepted a foreign-build index")
	}
	if _, err := draft.Finalizer(); err == nil {
		t.Fatal("failed foreign Commit was not terminal")
	}
	// Keep ownIndex referenced so the test documents that the source fixture
	// has a valid batch for the owning Build; Commit above is intentionally
	// terminal before a retry can substitute it.
	if len(ownIndex.Positions) == 0 {
		t.Fatal("own fixture unexpectedly empty")
	}
}

func TestFinalizerRejectsPayloadWithForeignSourceIdentity(t *testing.T) {
	input, ownIndex := sourceFixture(1)
	foreignInput, foreignIndex := sourceFixture(1)
	foreignInput.ExactAtoms = []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "foreign"}}
	foreignDraft, err := Build(foreignInput)
	if err != nil {
		t.Fatalf("foreign Build: %v", err)
	}
	foreignIndex = ownedIndex(foreignDraft, foreignIndex)
	if foreignFinalizer, finalizerErr := foreignDraft.Finalizer(); finalizerErr == nil {
		_ = foreignFinalizer.Abort()
	}
	ownDraft, err := Build(input)
	if err != nil {
		t.Fatalf("own Build: %v", err)
	}
	finalizer, err := ownDraft.Finalizer()
	if err != nil {
		t.Fatalf("own Finalizer: %v", err)
	}
	if _, err := finalizer.Commit(foreignIndex); err == nil {
		t.Fatal("Commit accepted a payload carrying a foreign Source identity")
	}
	if len(ownIndex.Positions) == 0 {
		t.Fatal("own fixture unexpectedly empty")
	}
}
