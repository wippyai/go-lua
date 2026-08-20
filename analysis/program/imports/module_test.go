package imports

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestBuildAndCommitSeparatesAuthoredAndDerivedModuleState(t *testing.T) {
	input := authoredInput()
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	view := finalizer.View()
	if view.Count() != 2 {
		t.Fatalf("authored count = %d, want 2", view.Count())
	}
	row, ok := view.ImportAt(0)
	if !ok || row.Term != input.Imports[0].Term || row.Call != input.Imports[0].Call || row.Alias != input.Imports[0].Alias || row.Request != input.Imports[0].Request || row.Key != 0 {
		t.Fatalf("authored row = %#v/%v", row, ok)
	}
	authoredID := view.ContentID()
	component, err := finalizer.Commit(CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	final := component.View()
	row, ok = final.Import(input.Imports[1].Term)
	if !ok || row.Request != stringTerm(2) || row.Key != 8 {
		t.Fatalf("derived row = %#v/%v", row, ok)
	}
	if got := final.ContentID(); got != authoredID {
		t.Fatalf("ContentID changed across derived commit: %x != %x", got, authoredID)
	}
	if view.Count() != 0 || view.ContentID().Available() {
		t.Fatal("captured View retained authored authority after terminal Commit")
	}
	if row, ok := view.ImportAt(0); ok || row != (Import{}) {
		t.Fatalf("captured View retained row after Commit: %#v/%v", row, ok)
	}
	if component.ContentID() != authoredID {
		t.Fatal("Component did not retain authored identity")
	}
}

func TestDraftCopiesShareOneFinalizerClaim(t *testing.T) {
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	copy := *draft
	first, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copy.Finalizer(); err == nil {
		t.Fatal("copied Draft claimed a second finalizer")
	}
	if !first.Abort() || first.Abort() {
		t.Fatal("Abort was not terminal")
	}
	if _, err := first.Commit(CommitInput{Resolutions: authoredResolutions(7, 8), Entry: emptyEntry()}); err == nil {
		t.Fatal("Commit reopened an aborted finalizer")
	}
}

func TestCapturedFinalizerViewExpiresOnAbort(t *testing.T) {
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	view := finalizer.View()
	if view.Count() != 2 || !view.ContentID().Available() {
		t.Fatal("captured View was not live before Abort")
	}
	if !finalizer.Abort() {
		t.Fatal("Abort rejected active finalizer")
	}
	if view.Count() != 0 || view.ContentID().Available() {
		t.Fatal("captured View retained authored authority after Abort")
	}
	if row, ok := view.ImportAt(0); ok || row != (Import{}) {
		t.Fatalf("captured View retained row after Abort: %#v/%v", row, ok)
	}
}

func TestFinalizerCommitIsTerminalOnValidationFailure(t *testing.T) {
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalizer.Commit(CommitInput{
		Resolutions: []Resolution{{Request: stringTerm(1), Key: 0}, {Request: stringTerm(2), Key: 8}},
		Entry:       emptyEntry(),
	}); err == nil {
		t.Fatal("Commit accepted unpaired resolution")
	}
	if finalizer.Abort() {
		t.Fatal("failed Commit left an abortable finalizer")
	}
}

func TestFinalizerCommitRejectsRequestWitnessMismatch(t *testing.T) {
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalizer.Commit(CommitInput{
		Resolutions: []Resolution{{Request: stringTerm(2), Key: 7}, {Request: stringTerm(2), Key: 8}},
		Entry:       emptyEntry(),
	}); err == nil {
		t.Fatal("Commit accepted a resolution Request different from authored Import")
	}
}

func TestAuthoredIdentityExcludesResolutionAndEntry(t *testing.T) {
	first := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	second := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(8, 9),
		Entry: Entry{
			ReturnIndex:  []uint32{0},
			RootRanges:   []EntryRange{{}},
			MemberRanges: []EntryRange{{}},
		},
	})
	if first.ContentID() != second.ContentID() {
		t.Fatal("derived Module state changed authored ContentID")
	}
	changed := authoredInput()
	changed.Imports[0].Alias = keyspace.MakeTerm(keyspace.FamilyCell, 2)
	draft, err := Build(changed)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, _ := draft.Finalizer()
	third, err := finalizer.Commit(CommitInput{Resolutions: authoredResolutions(7, 8), Entry: emptyEntry()})
	if err != nil {
		t.Fatal(err)
	}
	if third.ContentID() == first.ContentID() {
		t.Fatal("authored Alias change did not change ContentID")
	}
}

func TestAuthoredRequestParticipatesInContentID(t *testing.T) {
	baseline := buildCommitted(t, CommitInput{Resolutions: authoredResolutions(7, 8), Entry: emptyEntry()})
	changed := authoredInput()
	changed.Imports[0].Request = stringTerm(3)
	draft, err := Build(changed)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, err := finalizer.Commit(CommitInput{
		Resolutions: []Resolution{{Request: stringTerm(3), Key: 7}, {Request: stringTerm(2), Key: 8}},
		Entry:       emptyEntry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if component.ContentID() == baseline.ContentID() {
		t.Fatal("authored Request change did not change ContentID")
	}
}

func TestBuildRejectsDerivedFieldsAndInvalidEntry(t *testing.T) {
	input := authoredInput()
	input.Imports[0].Key = 1
	if _, err := Build(input); err == nil {
		t.Fatal("Build accepted derived Key in authored input")
	}
	input = authoredInput()
	input.Imports[0].Request = 0
	if _, err := Build(input); err == nil {
		t.Fatal("Build accepted an Import without its authored Request")
	}
	input = authoredInput()
	input.Imports[0].Call = keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("Build accepted non-Call authored row")
	}
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, _ := draft.Finalizer()
	if _, err := finalizer.Commit(CommitInput{Resolutions: authoredResolutions(7, 8), Entry: Entry{}}); err == nil {
		t.Fatal("Commit accepted zero Entry")
	}
}

func TestCommitRejectsWrongEntryRootFamilies(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Entry)
	}{
		{
			name: "root Function",
			mutate: func(entry *Entry) {
				entry.Roots[0] = keyspace.MakeTerm(keyspace.FamilyCell, 1)
			},
		},
		{
			name: "root Cell",
			mutate: func(entry *Entry) {
				entry.RootCells[0] = keyspace.MakeTerm(keyspace.FamilyFunction, 1)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			draft, err := Build(authoredInput())
			if err != nil {
				t.Fatal(err)
			}
			finalizer, err := draft.Finalizer()
			if err != nil {
				t.Fatal(err)
			}
			entry := entryWithRoot()
			test.mutate(&entry)
			if _, err := finalizer.Commit(CommitInput{
				Resolutions: authoredResolutions(7, 8),
				Entry:       entry,
			}); err == nil {
				t.Fatal("Commit accepted wrong-family Entry root")
			}
		})
	}
}

func TestCommitRejectsMalformedEntryMemberPath(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Entry)
	}{
		{
			name: "parent ordinal zero",
			mutate: func(entry *Entry) {
				entry.Members[0].Parent = keyspace.Term(keyspace.FamilyTable)
			},
		},
		{
			name: "zero suffix",
			mutate: func(entry *Entry) {
				entry.Members[0].Suffix = 0
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			draft, err := Build(authoredInput())
			if err != nil {
				t.Fatal(err)
			}
			finalizer, err := draft.Finalizer()
			if err != nil {
				t.Fatal(err)
			}
			entry := entryWithMember()
			test.mutate(&entry)
			if _, err := finalizer.Commit(CommitInput{
				Resolutions: authoredResolutions(7, 8),
				Entry:       entry,
			}); err == nil {
				t.Fatal("Commit accepted malformed Entry member path")
			}
		})
	}
}

func TestFinalizerCommitIsSingleWinnerUnderContention(t *testing.T) {
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	const contenders = 32
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var group sync.WaitGroup
	group.Add(contenders)
	for range contenders {
		copy := finalizer
		go func() {
			defer group.Done()
			<-start
			component, err := copy.Commit(CommitInput{Resolutions: authoredResolutions(7, 8), Entry: emptyEntry()})
			results <- err == nil && component != nil
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
		t.Fatalf("concurrent Commit successes = %d, want 1", successes)
	}
}

func buildCommitted(t *testing.T, input CommitInput) *Component {
	t.Helper()
	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, err := finalizer.Commit(input)
	if err != nil {
		t.Fatal(err)
	}
	return component
}

func authoredInput() Input {
	return Input{Imports: []Import{
		{Term: keyspace.MakeTerm(keyspace.FamilyImport, 1), Call: keyspace.MakeTerm(keyspace.FamilyCall, 1), Alias: keyspace.MakeTerm(keyspace.FamilyCell, 1), Request: stringTerm(1)},
		{Term: keyspace.MakeTerm(keyspace.FamilyImport, 2), Call: keyspace.MakeTerm(keyspace.FamilyCall, 2), Request: stringTerm(2)},
	}}
}

func authoredResolutions(keys ...keyspace.Key) []Resolution {
	if len(keys) != 2 {
		keys = []keyspace.Key{7, 8}
	}
	return []Resolution{{Request: stringTerm(1), Key: keys[0]}, {Request: stringTerm(2), Key: keys[1]}}
}

func stringTerm(ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(keyspace.FamilyString, ordinal)
}

func emptyEntry() Entry {
	return Entry{
		ReturnIndex:  []uint32{0},
		RootRanges:   []EntryRange{{}},
		MemberRanges: []EntryRange{{}},
	}
}

func entryWithRoot() Entry {
	return Entry{
		ReturnTerms: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyReturn, 1),
		},
		ReturnIndex: []uint32{0, 1},
		RootRanges: []EntryRange{
			{}, {Start: 0, End: 1},
		},
		Roots: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyFunction, 1),
		},
		RootCells: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyCell, 1),
		},
		MemberRanges: []EntryRange{{}, {}},
	}
}

func entryWithMember() Entry {
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	table := keyspace.MakeTerm(keyspace.FamilyTable, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	return Entry{
		ReturnTerms:  []keyspace.Term{returned},
		ReturnIndex:  []uint32{0, 1},
		RootRanges:   []EntryRange{{}, {Start: 0, End: 1}},
		Roots:        []keyspace.Term{0},
		RootCells:    []keyspace.Term{0},
		MemberRanges: []EntryRange{{}, {Start: 0, End: 1}},
		Members: []EntryMember{{
			Field: field, Parent: table, Returned: returned, Table: table, Suffix: 1,
		}},
		MemberIndex: []uint32{0, 1},
	}
}
