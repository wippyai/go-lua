package static

import (
	"math"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

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
	component.contentID = identity.ContentID{}
	if got := cold.ContentID(); got != want {
		t.Fatalf("Cold snapshot changed after Component mutation: %x != %x", got, want)
	}
}

func TestReferencesPreserveAuthoredBinderDisposition(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeRef] = 3
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyCell] = 1
	draft, err := Build(referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{
		{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7, 8}},
		{Resolution: TypeRefUnresolved, Source: []keyspace.Key{4, 5}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1)},
	}}))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	references := component.View().References()
	if got := references.Count(); got != 3 {
		t.Fatalf("Count() = %d, want 3", got)
	}
	declaration := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	if kind, target, root, ok := references.Get(declaration); !ok || kind != TypeRefDeclaration ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1) || root != 0 {
		t.Fatalf("declaration Get() = (%v, %v, %v, %v)", kind, target, root, ok)
	}
	canonical := keyspace.MakeTerm(keyspace.FamilyTypeRef, 2)
	if source, ok := references.SourceCount(canonical); !ok || source != 2 {
		t.Fatalf("SourceCount() = (%d, %v), want 2", source, ok)
	}
	if key, ok := references.SourceAt(canonical, 1); !ok || key != 3 {
		t.Fatalf("SourceAt() = (%d, %v), want 3", key, ok)
	}
	if canonicalCount, ok := references.CanonicalCount(canonical); !ok || canonicalCount != 2 {
		t.Fatalf("CanonicalCount() = (%d, %v), want 2", canonicalCount, ok)
	}
	if key, ok := references.CanonicalAt(canonical, 0); !ok || key != 7 {
		t.Fatalf("CanonicalAt() = (%d, %v), want 7", key, ok)
	}
	unresolved := keyspace.MakeTerm(keyspace.FamilyTypeRef, 3)
	if kind, target, root, ok := references.Get(unresolved); !ok || kind != TypeRefUnresolved || target != 0 || root != keyspace.MakeTerm(keyspace.FamilyCell, 1) {
		t.Fatalf("unresolved Get() = (%v, %v, %v, %v)", kind, target, root, ok)
	}
	if length, ok := references.CanonicalCount(unresolved); !ok || length != 0 {
		t.Fatalf("unresolved CanonicalCount() = (%d, %v), want 0", length, ok)
	}
}

func TestReferencesRejectInvalidXORRootAndArity(t *testing.T) {
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	interfaceType := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	cases := []struct {
		name string
		row  TypeRef
		ok   bool
	}{
		{"unqualified unresolved", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}}, true},
		{"qualified unresolved", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1, 2}, Root: cell}, true},
		{"alias target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: alias}, true},
		{"interface target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: interfaceType}, true},
		{"param target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: param}, true},
		{"canonical", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1, 2}, Root: cell, Canonical: []keyspace.Key{3}}, true},
		{"empty source", TypeRef{Resolution: TypeRefUnresolved}, false},
		{"zero source key", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{0}}, false},
		{"unqualified root", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}, Root: cell}, false},
		{"qualified missing root", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1, 2}}, false},
		{"qualified noncell root", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1, 2}, Root: alias}, false},
		{"unresolved target", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}, Target: alias}, false},
		{"unresolved canonical", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}, Canonical: []keyspace.Key{2}}, false},
		{"declaration missing target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}}, false},
		{"declaration canonical", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: alias, Canonical: []keyspace.Key{2}}, false},
		{"declaration type ref target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)}, false},
		{"canonical target", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1}, Target: alias, Canonical: []keyspace.Key{2}}, false},
		{"canonical missing path", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1}}, false},
		{"canonical zero path key", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1}, Canonical: []keyspace.Key{0}}, false},
		{"unknown disposition", TypeRef{Resolution: 99, Source: []keyspace.Key{1}}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			counts := [keyspace.FamilyCount]uint32{}
			counts[keyspace.FamilyTypeRef] = 1
			counts[keyspace.FamilyCell] = 1
			counts[keyspace.FamilyTypeAlias] = 1
			counts[keyspace.FamilyTypeInterface] = 1
			counts[keyspace.FamilyTypeParam] = 1
			_, err := Build(referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{test.row}}))
			if (err == nil) != test.ok {
				t.Fatalf("Build() error = %v, want accepted=%v", err, test.ok)
			}
		})
	}
}

func TestReferencesCopyFenceAndBounds(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeRef] = 1
	counts[keyspace.FamilyCell] = 1
	source := []keyspace.Key{1, 2}
	canonical := []keyspace.Key{3, 4}
	draft, err := Build(referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{{
		Resolution: TypeRefCanonicalPath,
		Source:     source,
		Root:       keyspace.MakeTerm(keyspace.FamilyCell, 1),
		Canonical:  canonical,
	}}}))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	source[0], canonical[0] = 99, 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	references := component.View().References()
	term := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	if got, ok := references.SourceAt(term, 0); !ok || got != 1 {
		t.Fatalf("source copy fence = (%d, %v)", got, ok)
	}
	if got, ok := references.CanonicalAt(term, 0); !ok || got != 3 {
		t.Fatalf("canonical copy fence = (%d, %v)", got, ok)
	}
	for _, index := range []int{-1, 2} {
		if _, ok := references.SourceAt(term, index); ok {
			t.Fatalf("SourceAt(%d) accepted out-of-bounds index", index)
		}
		if _, ok := references.CanonicalAt(term, index); ok {
			t.Fatalf("CanonicalAt(%d) accepted out-of-bounds index", index)
		}
	}
	if _, ok := references.At(1); ok {
		t.Fatal("At() accepted out-of-bounds ordinal")
	}
	if _, _, _, ok := references.Get(keyspace.MakeTerm(keyspace.FamilyTypeRef, 2)); ok {
		t.Fatal("Get() accepted an unknown TypeRef")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		references.Get(term)
		references.SourceCount(term)
		references.SourceAt(term, 1)
		references.CanonicalCount(term)
		references.CanonicalAt(term, 1)
	}); allocations != 0 {
		t.Fatalf("Reference queries allocated %.2f times", allocations)
	}
}

func TestGenericAcceptsReferencesOwnedTypeRef(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeRef] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeGeneric] = 1
	input := referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{{
		Resolution: TypeRefDeclaration,
		Source:     []keyspace.Key{1},
		Target:     keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1),
	}}})
	input.Types.Generic = []Generic{{
		Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
		Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)},
	}}
	_, err := Build(input)
	if err != nil {
		t.Fatalf("Build() rejected Generic TypeRef base: %v", err)
	}
}

func TestContractsPreserveDenseTypedSidecars(t *testing.T) {
	draft, err := Build(contractsFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	contracts := component.View().Contracts()
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if contracts.Functions().Count() != 1 || contracts.Calls().Count() != 1 {
		t.Fatalf("dense counts = (%d, %d)", contracts.Functions().Count(), contracts.Calls().Count())
	}
	if known, ok := contracts.Functions().Get(function); !ok || !known {
		t.Fatalf("function header = (%v, %v)", known, ok)
	}
	if count, ok := contracts.Functions().TypeParamCount(function); !ok || count != 1 {
		t.Fatalf("function type parameter count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Functions().TypeParamAt(function, 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeParam, 1) {
		t.Fatalf("function type parameter = (%v, %v)", got, ok)
	}
	if count, ok := contracts.Functions().ReturnCount(function); !ok || count != 1 {
		t.Fatalf("function return count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Functions().ReturnAt(function, 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1) {
		t.Fatalf("function return = (%v, %v)", got, ok)
	}
	if count, ok := contracts.Calls().TypeArgumentCount(call); !ok || count != 2 {
		t.Fatalf("call argument count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Calls().TypeArgumentAt(call, 1); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) {
		t.Fatalf("call argument = (%v, %v)", got, ok)
	}
	id, idOK := contracts.Calls().TypeArgumentID(call)
	if !idOK || !id.Available() {
		t.Fatal("call type-argument identity unavailable")
	}
	if replay, replayOK := contracts.Calls().TypeArgumentID(call); !replayOK || replay != id {
		t.Fatal("call type-argument identity was not stable")
	}
}

func TestContractsPreserveOmittedAndKnownEmptyReturns(t *testing.T) {
	for _, known := range []bool{false, true} {
		input := contractsFixture(t)
		input.Contracts.Function[0].ReturnsKnown = known
		input.Contracts.Function[0].Returns = nil
		input.Contracts.Call[0].TypeArguments = nil
		input.Signatures.TypeAsserts[0].Bound = false
		input.Signatures.TypeAsserts[0].Narrow = 0
		draft, err := Build(input)
		if err != nil {
			t.Fatalf("Build(known=%v) error = %v", known, err)
		}
		component, err := commitStaticDraft(t, draft)
		if err != nil {
			t.Fatalf("take() error = %v", err)
		}
		got, ok := component.View().Contracts().Functions().Get(keyspace.MakeTerm(keyspace.FamilyFunction, 1))
		if !ok || got != known {
			t.Fatalf("ReturnsKnown = (%v, %v), want (%v, true)", got, ok, known)
		}
		if count, ok := component.View().Contracts().Calls().TypeArgumentCount(keyspace.MakeTerm(keyspace.FamilyCall, 1)); !ok || count != 0 {
			t.Fatalf("empty call type arguments = (%d, %v)", count, ok)
		}
	}
}

func TestContractsRejectCoverageOwnershipAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing function contract", func(input *Input) { input.Contracts.Function = nil }},
		{"extra call contract", func(input *Input) { input.Contracts.Call = append(input.Contracts.Call, CallContract{}) }},
		{"omitted returns with child", func(input *Input) { input.Contracts.Function[0].ReturnsKnown = false }},
		{"orphan function type parameter", func(input *Input) { input.Contracts.Function[0].TypeParams = nil }},
		{"duplicate function type parameter", func(input *Input) {
			input.Counts[keyspace.FamilyTypeParam] = 2
			input.Declarations.TypeParam = append(input.Declarations.TypeParam, TypeParam{Owner: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Name: 4})
			input.Contracts.Function[0].TypeParams = []keyspace.Term{
				keyspace.MakeTerm(keyspace.FamilyTypeParam, 1), keyspace.MakeTerm(keyspace.FamilyTypeParam, 1),
			}
		}},
		{"wrong function type parameter owner", func(input *Input) {
			input.Declarations.TypeParam[0].Owner = keyspace.MakeTerm(keyspace.FamilyFunction, 0)
		}},
		{"shared return type argument child", func(input *Input) {
			input.Contracts.Call[0].TypeArguments[0] = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
		}},
		{"bound assertion as call argument", func(input *Input) {
			input.Contracts.Function[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
			input.Contracts.Call[0].TypeArguments[0] = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Build(func() Input { input := contractsFixture(t); test.edit(&input); return input }()); err == nil {
				t.Fatal("Build() accepted invalid contract relation")
			}
		})
	}
}

func TestContractsRejectNestedAssertionAndCrossOwnerDuplicate(t *testing.T) {
	input := contractsFixture(t)
	input.Counts[keyspace.FamilyTypeGeneric] = 1
	input.Types.Generic = []Generic{{
		Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1), Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)},
	}}
	input.Counts[keyspace.FamilyTypeRef] = 1
	input.References.TypeRef = []TypeRef{{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}}}
	input.Contracts.Function[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1)}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted bound assertion nested in a function return")
	}

	input = contractsFixture(t)
	input.Counts[keyspace.FamilyTypeAlias] = 1
	input.Counts[keyspace.FamilyBody] = 1
	input.Declarations.Alias = []TypeAlias{{
		Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
		Name: 11, NameCoordinate: signatureCoordinate(t), Params: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
	}}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a type parameter claimed by Alias and Function")
	}

	input = contractsFixture(t)
	input.Counts[keyspace.FamilyCell] = 1
	input.Counts[keyspace.FamilyTypeAsserts] = 0
	input.Signatures.TypeAsserts = nil
	input.Counts[keyspace.FamilyTypeFunction] = 1
	input.Counts[keyspace.FamilyTypeOptional] = 1
	input.Types.Optional = []Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)}}
	input.Signatures.TypeFunction = []TypeFunction{{
		Scope:        keyspace.MakeTerm(keyspace.FamilyCell, 1),
		ReturnsKnown: true, Returns: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)},
	}}
	input.Contracts.Function[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
	input.Contracts.Call[0].TypeArguments = nil
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a cycle through an existing TypeFunction")
	}

	input = contractsFixture(t)
	input.Counts[keyspace.FamilyTypeKeyOf] = 1
	input.Operators.KeyOf = []KeyOf{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)}}
	input.Contracts.Call[0].TypeArguments = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a cycle through an existing static operator")
	}
}

func TestContractsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := contractsFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Contracts.Function[0].TypeParams[0] = 0
	input.Contracts.Function[0].Returns[0] = 0
	input.Contracts.Call[0].TypeArguments[0] = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	contracts := component.View().Contracts()
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := contracts.Functions().TypeParamAt(function, 0); !ok || got == 0 {
		t.Fatalf("type parameter copy fence = (%v, %v)", got, ok)
	}
	if got, ok := contracts.Functions().ReturnAt(function, 0); !ok || got == 0 {
		t.Fatalf("return copy fence = (%v, %v)", got, ok)
	}
	if got, ok := contracts.Calls().TypeArgumentAt(call, 0); !ok || got == 0 {
		t.Fatalf("type argument copy fence = (%v, %v)", got, ok)
	}
	if _, ok := contracts.Functions().ReturnAt(function, -1); ok {
		t.Fatal("ReturnAt accepted negative index")
	}
	if _, ok := contracts.Calls().TypeArgumentAt(call, 2); ok {
		t.Fatal("TypeArgumentAt accepted out-of-range index")
	}
	if _, ok := contracts.Functions().Get(keyspace.MakeTerm(keyspace.FamilyFunction, 2)); ok {
		t.Fatal("Functions.Get accepted unknown term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		contracts.Functions().Get(function)
		contracts.Functions().TypeParamAt(function, 0)
		contracts.Functions().ReturnAt(function, 0)
		contracts.Calls().TypeArgumentAt(call, 0)
	}); allocations != 0 {
		t.Fatalf("contract queries allocated %.2f times", allocations)
	}
}

func TestDeclarationsPreserveTypedOwnershipAndOrder(t *testing.T) {
	draft, err := Build(declarationFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	declarations := component.View().Declarations()
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	if owner, target, name, coordinate, ok := declarations.Aliases().Get(alias); !ok ||
		owner != keyspace.MakeTerm(keyspace.FamilyBody, 1) || target != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) ||
		name != 1 || coordinate == (source.Coordinate{}) {
		t.Fatalf("alias relation = (%v, %v, %v, %v, %v)", owner, target, name, coordinate, ok)
	}
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	if count, ok := declarations.Aliases().ParamCount(alias); !ok || count != 1 {
		t.Fatalf("alias parameter count = (%d, %v)", count, ok)
	}
	if got, ok := declarations.Aliases().ParamAt(alias, 0); !ok || got != param {
		t.Fatalf("alias parameter = (%v, %v)", got, ok)
	}
	if owner, name, constraint, ok := declarations.TypeParams().Get(param); !ok || owner != alias || name != 2 ||
		constraint != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) {
		t.Fatalf("type parameter = (%v, %v, %v, %v)", owner, name, constraint, ok)
	}
	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	if count, ok := declarations.Interfaces().ExtendCount(iface); !ok || count != 1 {
		t.Fatalf("interface extends = (%d, %v)", count, ok)
	}
	if member, ok := declarations.Interfaces().MemberAt(iface, 1); !ok || member.Kind != InterfaceMethod ||
		member.Field != 0 || member.Name != 6 || member.Signature != keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1) {
		t.Fatalf("method member = (%+v, %v)", member, ok)
	}
}

func TestDeclarationsRejectTotalityXORCoordinatesAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing alias parameter", func(input *Input) { input.Declarations.Alias[0].Params = nil }},
		{"orphan field", func(input *Input) {
			input.Declarations.Interface[0].Members = input.Declarations.Interface[0].Members[1:]
		}},
		{"field method xor", func(input *Input) { input.Declarations.Interface[0].Members[0].Name = 9 }},
		{"method missing coordinate", func(input *Input) { input.Declarations.Interface[0].Members[1].NameCoordinate = source.Coordinate{} }},
		{"alias absent coordinate", func(input *Input) { input.Declarations.Alias[0].NameCoordinate = source.Coordinate{} }},
		{"interface non-reference extends", func(input *Input) {
			input.Declarations.Interface[0].Extends[0] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"shared concrete child", func(input *Input) {
			input.Declarations.TypeParam[0].Constraint = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := declarationFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid declaration relation")
			}
		})
	}
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeOptional] = 1
	if _, err := Build(Input{Counts: counts, Types: TypesInput{
		Optional: []Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)}},
	}}); err == nil {
		t.Fatal("Build() accepted cyclic static type forest")
	}

	// Record membership is not a generic type-child edge, but it still closes
	// the concrete containment walk. A field cannot point back to its record.
	input := declarationFixture(t)
	input.Counts[keyspace.FamilyTypeRecord] = 1
	input.Types.Record = []Record{{Fields: []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTypeField, 1),
	}}}
	input.Types.Field[0].Type = keyspace.MakeTerm(keyspace.FamilyTypeRecord, 1)
	input.Declarations.Interface[0].Members = input.Declarations.Interface[0].Members[1:]
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a field type cycle through its record owner")
	}
}

func TestDeclarationsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := declarationFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Declarations.Alias[0].Params[0] = 0
	input.Declarations.Interface[0].Extends[0] = 0
	input.Declarations.Interface[0].Members[1].Name = 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	declarations := component.View().Declarations()
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	if got, ok := declarations.Aliases().ParamAt(alias, 0); !ok || got == 0 {
		t.Fatalf("alias copy fence = (%v, %v)", got, ok)
	}
	if got, ok := declarations.Interfaces().ExtendAt(iface, 0); !ok || got == 0 {
		t.Fatalf("interface extension copy fence = (%v, %v)", got, ok)
	}
	if got, ok := declarations.Interfaces().MemberAt(iface, 1); !ok || got.Name != 6 {
		t.Fatalf("interface member copy fence = (%+v, %v)", got, ok)
	}
	if _, ok := declarations.Aliases().ParamAt(alias, -1); ok {
		t.Fatal("ParamAt accepted negative index")
	}
	if _, ok := declarations.Interfaces().MemberAt(iface, 2); ok {
		t.Fatal("MemberAt accepted out-of-range index")
	}
	if _, _, _, _, ok := declarations.Aliases().Get(keyspace.MakeTerm(keyspace.FamilyTypeAlias, 2)); ok {
		t.Fatal("Aliases.Get accepted unknown term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		declarations.Aliases().Get(alias)
		declarations.Aliases().ParamCount(alias)
		declarations.Aliases().ParamAt(alias, 0)
		declarations.TypeParams().Get(keyspace.MakeTerm(keyspace.FamilyTypeParam, 1))
		declarations.Interfaces().Get(iface)
		declarations.Interfaces().ExtendAt(iface, 0)
		declarations.Interfaces().MemberAt(iface, 1)
	}); allocations != 0 {
		t.Fatalf("declaration queries allocated %.2f times", allocations)
	}
}

func compositeLocalContainmentInput(t *testing.T) Input {
	t.Helper()
	input := declarationFixture(t)
	coordinate := input.Declarations.Alias[0].NameCoordinate
	term := func(family keyspace.Family, ordinal uint32) keyspace.Term { return keyspace.MakeTerm(family, ordinal) }
	primitive := func(ordinal uint32) keyspace.Term { return term(keyspace.FamilyTypePrimitive, ordinal) }
	input.Counts[keyspace.FamilyCell] = 1
	input.Counts[keyspace.FamilyRead] = 1
	input.Counts[keyspace.FamilyAssign] = 1
	input.Counts[keyspace.FamilyValueClaim] = 1
	input.Counts[keyspace.FamilyTypeValue] = 1
	input.Counts[keyspace.FamilyFunction] = 1
	input.Counts[keyspace.FamilyCall] = 1
	input.Counts[keyspace.FamilyTypePublication] = 1
	input.Counts[keyspace.FamilyTypePrimitive] = 27
	input.Counts[keyspace.FamilyTypeOptional] = 1
	input.Counts[keyspace.FamilyTypeUnion] = 1
	input.Counts[keyspace.FamilyTypeIntersection] = 1
	input.Counts[keyspace.FamilyTypeRef] = 3
	input.Counts[keyspace.FamilyTypeGeneric] = 1
	input.Counts[keyspace.FamilyTypeArray] = 1
	input.Counts[keyspace.FamilyTypeMap] = 1
	input.Counts[keyspace.FamilyTypeRecord] = 1
	input.Counts[keyspace.FamilyTypeField] = 2
	input.Counts[keyspace.FamilyTypeAsserts] = 1
	input.Counts[keyspace.FamilyTypeOf] = 1
	input.Counts[keyspace.FamilyTypeKeyOf] = 1
	input.Counts[keyspace.FamilyTypeIndexAccess] = 1
	input.Counts[keyspace.FamilyTypeConditional] = 1
	input.Types.Primitive = make([]Primitive, 27)
	for index := range input.Types.Primitive {
		input.Types.Primitive[index] = Primitive{Kind: PrimitiveAny}
	}
	input.Types.Optional = []Optional{{Inner: primitive(8)}}
	input.Types.Union = []Union{{Members: []keyspace.Term{primitive(9), primitive(10)}}}
	input.Types.Intersection = []Intersection{{Members: []keyspace.Term{primitive(11), primitive(12)}}}
	input.Types.Generic = []Generic{{Base: term(keyspace.FamilyTypeRef, 3), Args: []keyspace.Term{primitive(13)}}}
	input.Types.Array = []Array{{Element: primitive(14)}}
	input.Types.Map = []Map{{Key: primitive(15), Value: primitive(16)}}
	input.Types.Field = []Field{{Key: 4, Type: primitive(3), Optional: true}, {Key: 5, Type: primitive(17)}}
	input.Types.Record = []Record{{Fields: []keyspace.Term{term(keyspace.FamilyTypeField, 2)}}}
	input.References.TypeRef = append(input.References.TypeRef,
		TypeRef{Resolution: TypeRefDeclaration, Target: term(keyspace.FamilyTypeAlias, 1), Source: []keyspace.Key{6}},
		TypeRef{Resolution: TypeRefDeclaration, Target: term(keyspace.FamilyTypeInterface, 1), Source: []keyspace.Key{7}},
	)
	input.Signatures.TypeFunction[0] = TypeFunction{
		Scope:      term(keyspace.FamilyTypeInterface, 1),
		Parameters: []Parameter{{Name: 20, NameCoordinate: coordinate, Type: primitive(4)}},
		Variadic:   primitive(5), VariadicCoordinate: coordinate,
		ReturnsKnown: true, Returns: []keyspace.Term{term(keyspace.FamilyTypeAsserts, 1), primitive(6)},
	}
	input.Counts[keyspace.FamilyTypeAsserts] = 1
	input.Signatures.TypeAsserts = []TypeAsserts{{Name: 20, ParamCoordinate: coordinate, Bound: true, Param: 0, Narrow: primitive(7)}}
	input.Contracts = ContractsInput{
		Function: []FunctionContract{{ReturnsKnown: true, Returns: []keyspace.Term{primitive(18)}}},
		Call:     []CallContract{{TypeArguments: []keyspace.Term{primitive(19)}}},
	}
	input.Operators = OperatorsInput{
		TypeOf:      []TypeOf{{Scope: term(keyspace.FamilyCell, 1), Operand: term(keyspace.FamilyRead, 1)}},
		KeyOf:       []KeyOf{{Inner: term(keyspace.FamilyTypeOf, 1)}},
		IndexAccess: []IndexAccess{{Object: primitive(20), Index: primitive(21)}},
		Conditional: []Conditional{{Check: primitive(22), Extends: primitive(23), Then: primitive(24), Else: primitive(25)}},
	}
	input.Operands = OperandsInput{
		Claim:     []ClaimTarget{{Claim: term(keyspace.FamilyValueClaim, 1), Target: primitive(26)}},
		TypeValue: []TypeValueTarget{{Target: primitive(27)}},
	}
	input.Publications = PublicationsInput{Type: []Publication{{Assign: term(keyspace.FamilyAssign, 1), Target: term(keyspace.FamilyTypeRef, 2)}}}
	return input
}

func TestStaticLocalContainmentCompositeEmitterRows(t *testing.T) {
	input := compositeLocalContainmentInput(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	wantID := draft.state.component.ContentID()
	if !wantID.Available() {
		t.Fatal("composite fixture produced unavailable authored identity")
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	term := func(family keyspace.Family, ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(family, ordinal)
	}
	parents := make(map[keyspace.Term]keyspace.Term)
	for _, family := range staticFamilyInventory[:staticTypeFamilyCount] {
		for ordinal := uint32(1); ordinal <= input.Counts[family]; ordinal++ {
			parents[term(family, ordinal)] = 0
		}
	}
	setParent := func(child, parent keyspace.Term) { parents[child] = parent }
	setParent(primitive(1), term(keyspace.FamilyTypeAlias, 1))
	setParent(primitive(2), term(keyspace.FamilyTypeParam, 1))
	setParent(primitive(3), term(keyspace.FamilyTypeField, 1))
	setParent(primitive(4), term(keyspace.FamilyTypeFunction, 1))
	setParent(primitive(5), term(keyspace.FamilyTypeFunction, 1))
	setParent(term(keyspace.FamilyTypeAsserts, 1), term(keyspace.FamilyTypeFunction, 1))
	setParent(primitive(6), term(keyspace.FamilyTypeFunction, 1))
	setParent(primitive(7), term(keyspace.FamilyTypeAsserts, 1))
	setParent(primitive(8), term(keyspace.FamilyTypeOptional, 1))
	setParent(primitive(9), term(keyspace.FamilyTypeUnion, 1))
	setParent(primitive(10), term(keyspace.FamilyTypeUnion, 1))
	setParent(primitive(11), term(keyspace.FamilyTypeIntersection, 1))
	setParent(primitive(12), term(keyspace.FamilyTypeIntersection, 1))
	setParent(term(keyspace.FamilyTypeRef, 3), term(keyspace.FamilyTypeGeneric, 1))
	setParent(primitive(13), term(keyspace.FamilyTypeGeneric, 1))
	setParent(primitive(14), term(keyspace.FamilyTypeArray, 1))
	setParent(primitive(15), term(keyspace.FamilyTypeMap, 1))
	setParent(primitive(16), term(keyspace.FamilyTypeMap, 1))
	setParent(primitive(17), term(keyspace.FamilyTypeField, 2))
	setParent(term(keyspace.FamilyTypeRef, 1), term(keyspace.FamilyTypeInterface, 1))
	setParent(term(keyspace.FamilyTypeFunction, 1), term(keyspace.FamilyTypeInterface, 1))
	setParent(primitive(18), term(keyspace.FamilyFunction, 1))
	setParent(primitive(19), term(keyspace.FamilyCall, 1))
	setParent(term(keyspace.FamilyTypeRef, 2), term(keyspace.FamilyTypePublication, 1))
	setParent(term(keyspace.FamilyTypeOf, 1), term(keyspace.FamilyTypeKeyOf, 1))
	setParent(primitive(20), term(keyspace.FamilyTypeIndexAccess, 1))
	setParent(primitive(21), term(keyspace.FamilyTypeIndexAccess, 1))
	setParent(primitive(22), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(23), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(24), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(25), term(keyspace.FamilyTypeConditional, 1))
	setParent(primitive(26), term(keyspace.FamilyValueClaim, 1))
	setParent(primitive(27), term(keyspace.FamilyTypeValue, 1))
	fieldOwners := map[keyspace.Term]keyspace.Term{
		term(keyspace.FamilyTypeField, 1): term(keyspace.FamilyTypeInterface, 1),
		term(keyspace.FamilyTypeField, 2): term(keyspace.FamilyTypeRecord, 1),
	}
	for child, wantParent := range parents {
		gotParent, ok := proof.Parent(child)
		if wantParent == 0 {
			if ok || gotParent != 0 {
				t.Fatalf("root Parent(%v) = %v/%v, want 0/false", child, gotParent, ok)
			}
			continue
		}
		if !ok || gotParent != wantParent {
			t.Fatalf("Parent(%v) = %v/%v, want %v/true", child, gotParent, ok, wantParent)
		}
	}
	for field, wantOwner := range fieldOwners {
		if gotOwner, ok := proof.FieldOwner(field); !ok || gotOwner != wantOwner {
			t.Fatalf("FieldOwner(%v) = %v/%v, want %v/true", field, gotOwner, ok, wantOwner)
		}
	}
	seen := make(map[keyspace.Term]struct{}, proof.Count())
	for index := 0; index < proof.Count(); index++ {
		at, ok := proof.At(index)
		if !ok {
			t.Fatalf("At(%d) failed within Count=%d", index, proof.Count())
		}
		if _, duplicate := seen[at]; duplicate {
			t.Fatalf("At(%d) duplicated %v", index, at)
		}
		seen[at] = struct{}{}
		if keyspace.TermFamily(at) == keyspace.FamilyTypeField {
			t.Fatalf("At(%d) exposed Field %v outside FieldOwner", index, at)
		}
	}
	if len(seen) != len(parents) {
		t.Fatalf("At set size = %d, closed parent denominator = %d", len(seen), len(parents))
	}
	for at := range parents {
		if _, ok := seen[at]; !ok {
			t.Fatalf("At enumeration omitted parent-domain term %v", at)
		}
	}
	if _, ok := proof.At(-1); ok {
		t.Fatal("At accepted negative index")
	}
	if _, ok := proof.At(proof.Count()); ok {
		t.Fatal("At accepted Count index")
	}
	if _, ok := proof.Parent(term(keyspace.FamilyRead, 1)); ok {
		t.Fatal("Parent accepted foreign family")
	}
	if _, ok := proof.Parent(keyspace.Term(uint32(keyspace.FamilyTypePrimitive))); ok {
		t.Fatal("Parent accepted ordinal-zero term")
	}
	if _, ok := proof.Parent(primitive(input.Counts[keyspace.FamilyTypePrimitive] + 1)); ok {
		t.Fatal("Parent accepted out-of-range ordinal")
	}
	coldID := draft.state.component.ContentID()
	if coldID != wantID {
		t.Fatalf("repeated Build identity = %x, want %x", coldID, wantID)
	}
	component, err := finalizer.Commit(commitInputForDraft(draft))
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if got := component.ContentID(); got != coldID {
		t.Fatalf("published identity = %x, before proof = %x", got, coldID)
	}
	if got := component.Cold().ContentID(); got != coldID {
		t.Fatalf("Cold identity = %x, before proof = %x", got, coldID)
	}
}

func TestStaticLocalContainmentRowsAndBounds(t *testing.T) {
	draft, err := Build(declarationFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	ref := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	if got, ok := proof.Parent(primitive(1)); !ok || got != alias {
		t.Fatalf("alias target parent = %v/%v, want %v/true", got, ok, alias)
	}
	if got, ok := proof.Parent(primitive(2)); !ok || got != param {
		t.Fatalf("constraint parent = %v/%v, want %v/true", got, ok, param)
	}
	if got, ok := proof.Parent(ref); !ok || got != iface {
		t.Fatalf("interface extension parent = %v/%v, want %v/true", got, ok, iface)
	}
	if got, ok := proof.Parent(function); !ok || got != iface {
		t.Fatalf("interface method parent = %v/%v, want %v/true", got, ok, iface)
	}
	if got, ok := proof.Parent(primitive(3)); !ok || got != field {
		t.Fatalf("field value parent = %v/%v, want %v/true", got, ok, field)
	}
	if got, ok := proof.Parent(alias); ok || got != 0 {
		t.Fatalf("root alias parent = %v/%v, want 0/false", got, ok)
	}
	if got, ok := proof.FieldOwner(field); !ok || got != iface {
		t.Fatalf("field owner = %v/%v, want %v/true", got, ok, iface)
	}
	if _, ok := proof.Parent(keyspace.MakeTerm(keyspace.FamilyRead, 1)); ok {
		t.Fatal("Parent accepted foreign family")
	}
	if _, ok := proof.Parent(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4)); ok {
		t.Fatal("Parent accepted out-of-range ordinal")
	}
	if _, ok := proof.Parent(0); ok {
		t.Fatal("Parent accepted zero term")
	}
	if _, ok := proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("FieldOwner accepted non-field family")
	}
	if _, ok := proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypeField, 2)); ok {
		t.Fatal("FieldOwner accepted out-of-range field")
	}
	if proof.Count() == 0 {
		t.Fatal("LocalContainment omitted closed static denominator")
	}
	if _, ok := proof.At(-1); ok {
		t.Fatal("LocalContainment.At accepted negative index")
	}
	if _, ok := proof.At(proof.Count()); ok {
		t.Fatal("LocalContainment.At accepted out-of-range index")
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
}

func TestStaticLocalContainmentExpiresCopiesAndPreservesIdentity(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	copied := proof
	if _, ok := proof.At(0); !ok {
		t.Fatal("claimed LocalContainment unavailable")
	}
	want := finalizer.View().Types().Primitives().Count()
	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if got := component.View().Types().Primitives().Count(); got != want {
		t.Fatalf("published Component count = %d, want %d", got, want)
	}
	if _, ok := proof.At(0); ok {
		t.Fatal("LocalContainment survived Commit")
	}
	if _, ok := copied.At(0); ok {
		t.Fatal("copied LocalContainment survived Commit")
	}
	if draft.state.localContainment != nil {
		t.Fatal("Draft retained local proof after Commit")
	}
	cold := component.Cold()
	if got := cold.ContentID(); got != component.ContentID() {
		t.Fatalf("Cold identity = %x, Component identity = %x", got, component.ContentID())
	}
	componentProof := component.View().LocalContainment()
	if componentProof.Count() != 0 {
		t.Fatalf("published Component View exposed LocalContainment count %d", componentProof.Count())
	}
	if _, ok := componentProof.At(0); ok {
		t.Fatal("published Component View exposed LocalContainment rows")
	}

	abortDraft := primitiveDraft(t)
	abortFinalizer, err := abortDraft.Finalizer()
	if err != nil {
		t.Fatalf("Abort Finalizer() error = %v", err)
	}
	abortProof := abortFinalizer.View().LocalContainment()
	if err := abortFinalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if _, ok := abortProof.At(0); ok {
		t.Fatal("LocalContainment survived Abort")
	}
	if abortDraft.state.localContainment != nil {
		t.Fatal("Draft retained local proof after Abort")
	}
}

func TestStaticLocalContainmentReadsRaceTerminal(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		for range 1000 {
			proof.Parent(primitive)
			proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypeField, 1))
			proof.Count()
			proof.At(0)
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

func TestStaticLocalContainmentQueriesDoNotAllocate(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	proof := finalizer.View().LocalContainment()
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if allocations := testing.AllocsPerRun(100, func() {
		proof.Parent(primitive)
		proof.FieldOwner(keyspace.MakeTerm(keyspace.FamilyTypeField, 1))
		proof.Count()
		proof.At(0)
	}); allocations != 0 {
		t.Fatalf("LocalContainment queries allocated %.2f times", allocations)
	}
	if err := finalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
}

func TestOperandsPreserveExactSparseAndDenseRelations(t *testing.T) {
	draft, err := Build(operandsFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operands := component.View().Operands()
	claims := operands.Claims()
	claim1 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	claim2 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 2)
	if claims.Count() != 1 {
		t.Fatalf("semantic ClaimTarget count = %d, want 1 (not all 2 ValueClaims)", claims.Count())
	}
	if got, ok := claims.At(0); !ok || got != claim1 {
		t.Fatalf("canonical claim target term = %v/%v, want claim 1", got, ok)
	}
	if target, ok := claims.Target(claim1); !ok || target != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("claim target = %v/%v", target, ok)
	}
	if target, ok := claims.Target(claim2); ok || target != 0 {
		t.Fatalf("missing sparse target = %v/%v, want zero/false", target, ok)
	}

	typeValues := operands.TypeValues()
	typeValue := keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
	if typeValues.Count() != 1 {
		t.Fatalf("TypeValue target count = %d, want 1", typeValues.Count())
	}
	if target, ok := typeValues.Target(typeValue); !ok || target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("TypeValue target = %v/%v", target, ok)
	}

	annotations := operands.Annotations()
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if count, ok := annotations.ForCount(primitive); !ok || count != 2 {
		t.Fatalf("annotation CSR count = %d/%v, want 2", count, ok)
	}
	for index := range 2 {
		term, ok := annotations.ForAt(primitive, index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyAnnotation, uint32(index+1)) {
			t.Fatalf("annotation CSR term[%d] = %v/%v", index, term, ok)
		}
	}
	if count, ok := annotations.ForCount(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)); !ok || count != 0 {
		t.Fatalf("valid unannotated target count = %d/%v, want 0/true", count, ok)
	}
	if _, ok := annotations.ForCount(keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)); ok {
		t.Fatal("annotation query accepted non-static anchor")
	}
}

func TestOperandsRejectInvalidTargetsAndDenominators(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"duplicate claim", func(input *Input) {
			input.Operands.Claim = append(input.Operands.Claim, input.Operands.Claim[0])
		}},
		{"invalid claim family", func(input *Input) {
			input.Operands.Claim[0].Claim = keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
		}},
		{"invalid claim static target", func(input *Input) {
			input.Operands.Claim[0].Target = keyspace.MakeTerm(keyspace.FamilyValues, 1)
		}},
		{"type value static-only primitive", func(input *Input) {
			input.Operands.TypeValue[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)
		}},
		{"type value unresolved reference", func(input *Input) {
			input.References.TypeRef[0].Resolution = TypeRefUnresolved
			input.References.TypeRef[0].Target = 0
		}},
		{"type value wrong dense count", func(input *Input) {
			input.Operands.TypeValue = nil
		}},
		{"annotation wrong dense count", func(input *Input) {
			input.Operands.Annotation = input.Operands.Annotation[:1]
		}},
		{"annotation invalid values", func(input *Input) {
			input.Operands.Annotation[0].Values = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"annotation invalid anchor", func(input *Input) {
			input.Operands.Annotation[0].Target = keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := operandsFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid operand relation")
			}
		})
	}
}

func TestOperandsCanonicalizeSparseClaimOrder(t *testing.T) {
	input := operandsFixture(t)
	input.Counts[keyspace.FamilyValueClaim] = 3
	input.Counts[keyspace.FamilyTypePrimitive] = 4
	input.Types.Primitive = append(input.Types.Primitive, Primitive{Kind: PrimitiveBoolean})
	claim1 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	claim3 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 3)
	input.Operands.Claim = []ClaimTarget{
		{Claim: claim3, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4)},
		{Claim: claim1, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)},
	}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	claims := component.View().Operands().Claims()
	for index, want := range []keyspace.Term{claim1, claim3} {
		got, ok := claims.At(index)
		if !ok || got != want {
			t.Fatalf("canonical Claims.At(%d) = %v/%v, want %v", index, got, ok, want)
		}
	}
}

func TestOperandsRejectCrossOwnerConcreteTargetSharing(t *testing.T) {
	input := operandsFixture(t)
	// The same concrete type cannot have two external canonical parents.
	input.Operands.TypeValue[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a static target shared by Claim and TypeValue")
	}
}

func TestOperandsCopyFenceBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := operandsFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operands.Claim[0].Target = 0
	input.Operands.TypeValue[0].Target = 0
	input.Operands.Annotation[0].Name = 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operands := component.View().Operands()
	claim := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	typeValue := keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
	annotationTarget := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if target, ok := operands.Claims().Target(claim); !ok || target == 0 {
		t.Fatalf("claim copy fence = %v/%v", target, ok)
	}
	if target, ok := operands.TypeValues().Target(typeValue); !ok || target == 0 {
		t.Fatalf("TypeValue copy fence = %v/%v", target, ok)
	}
	if row, ok := operands.Annotations().Get(keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)); !ok || row.Name != 2 {
		t.Fatalf("annotation copy fence = %+v/%v", row, ok)
	}
	if _, ok := operands.Claims().At(1); ok {
		t.Fatal("Claims.At accepted sparse out-of-bounds index")
	}
	if _, ok := operands.TypeValues().Target(keyspace.MakeTerm(keyspace.FamilyTypeValue, 2)); ok {
		t.Fatal("TypeValues.Target accepted unknown term")
	}
	if _, ok := operands.Annotations().ForAt(annotationTarget, 2); ok {
		t.Fatal("Annotations.ForAt accepted out-of-bounds index")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		operands.Claims().Count()
		operands.Claims().At(0)
		operands.Claims().Target(claim)
		operands.TypeValues().Target(typeValue)
		operands.Annotations().Get(keyspace.MakeTerm(keyspace.FamilyAnnotation, 1))
		operands.Annotations().ForCount(annotationTarget)
		operands.Annotations().ForAt(annotationTarget, 1)
	}); allocations != 0 {
		t.Fatalf("operand queries allocated %.2f times", allocations)
	}
}

// The closed authored-static family set and its immutable owner stores must
// move together. This is intentionally a local query law: it prevents a new
// type form from being accepted by Build yet becoming unqueryable as an
// annotation anchor after compaction.
func TestAnnotationAnchorCoversEveryStaticTypeFamily(t *testing.T) {
	component := &Component{}
	for _, family := range staticFamilyInventory[staticNodeFamilyOffset:staticTypeFamilyCount] {
		component.census[family] = 1
		if !annotationTargetPresent(component, keyspace.MakeTerm(family, 1)) {
			t.Fatalf("annotation anchor rejected static family %v", family)
		}
		if annotationTargetPresent(component, keyspace.MakeTerm(family, 2)) {
			t.Fatalf("annotation anchor accepted %v past its census", family)
		}
	}
	for _, family := range staticFamilyInventory[:staticNodeFamilyOffset] {
		component.census[family] = 1
		if annotationTargetPresent(component, keyspace.MakeTerm(family, 1)) {
			t.Fatalf("annotation anchor accepted declaration root %v", family)
		}
	}
}

func TestOperatorsPreserveTypedRelationsAndCrossOwnerLeaves(t *testing.T) {
	draft, err := Build(operatorFixture())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operators := component.View().Operators()
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	if scope, operand, ok := operators.TypeOfs().Get(typeOf); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) || operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("typeof relation = (%v, %v, %v)", scope, operand, ok)
	}
	// The same Source/Flow operand can occur in several TypeOf rows.  It is
	// not a concrete type child and therefore must not be rejected as shared.
	if _, operand, ok := operators.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 2)); !ok ||
		operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("second typeof cross-owner operand = (%v, %v)", operand, ok)
	}
	if inner, ok := operators.KeyOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)); !ok || inner != typeOf {
		t.Fatalf("keyof typed child = (%v, %v)", inner, ok)
	}
	if object, index, ok := operators.IndexAccesses().Get(keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)); !ok ||
		object != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) || index != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) {
		t.Fatalf("index-access relation = (%v, %v, %v)", object, index, ok)
	}
	if check, extends, then, otherwise, ok := operators.Conditionals().Get(keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)); !ok ||
		check != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) || extends != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4) ||
		then != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5) || otherwise != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 6) {
		t.Fatalf("conditional relation = (%v, %v, %v, %v, %v)", check, extends, then, otherwise, ok)
	}
}

func TestOperatorsRejectCoverageCrossOwnerAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing typeof row", func(input *Input) { input.Operators.TypeOf = input.Operators.TypeOf[:1] }},
		{"missing keyof row", func(input *Input) { input.Operators.KeyOf = nil }},
		{"missing indexed access row", func(input *Input) { input.Operators.IndexAccess = nil }},
		{"missing conditional row", func(input *Input) { input.Operators.Conditional = nil }},
		{"extra typeof row", func(input *Input) { input.Counts[keyspace.FamilyTypeOf] = 1 }},
		{"invalid typeof scope family", func(input *Input) {
			input.Counts[keyspace.FamilyBody] = 1
			input.Operators.TypeOf[0].Scope = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"foreign typeof scope", func(input *Input) { input.Operators.TypeOf[0].Scope = keyspace.MakeTerm(keyspace.FamilyCell, 2) }},
		{"zero typeof operand", func(input *Input) { input.Operators.TypeOf[0].Operand = 0 }},
		{"foreign typeof operand", func(input *Input) { input.Operators.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyRead, 2) }},
		{"Import typeof operand", func(input *Input) {
			input.Counts[keyspace.FamilyImport] = 1
			input.Operators.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyImport, 1)
		}},
		{"nonstatic keyof child", func(input *Input) { input.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyRead, 1) }},
		{"same indexed child twice", func(input *Input) {
			input.Operators.IndexAccess[0].Index = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"shared operator child", func(input *Input) {
			input.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"keyof cycle", func(input *Input) { input.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1) }},
		{"indexed-access cycle", func(input *Input) {
			input.Operators.IndexAccess[0].Object = keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)
		}},
		{"conditional cycle", func(input *Input) {
			input.Operators.Conditional[0].Check = keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := operatorFixture()
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid operator relation")
			}
		})
	}

	// TypeOf's operand is an explicit cross-owner Flow value occurrence. Static
	// rejects a Body handle locally, before containment can be assembled.
	input := operatorFixture()
	input.Counts[keyspace.FamilyBody] = 1
	input.Operators.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted non-Flow TypeOf operand")
	}

	// A concrete type edge from the Types vertical and an operator edge form
	// one local forest; neither owner gets a private cycle exception.
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeKeyOf] = 1
	if _, err := Build(Input{Counts: counts,
		Types:     TypesInput{Optional: []Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)}}},
		Operators: OperatorsInput{KeyOf: []KeyOf{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)}}},
	}); err == nil {
		t.Fatal("Build() accepted a cross-vertical static cycle")
	}
}

func TestOperatorsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := operatorFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operators.TypeOf[0].Operand = 0
	input.Operators.KeyOf[0].Inner = 0
	input.Operators.IndexAccess[0].Object = 0
	input.Operators.Conditional[0].Check = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operators := component.View().Operators()
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	keyOf := keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)
	indexAccess := keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)
	conditional := keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)
	if _, operand, ok := operators.TypeOfs().Get(typeOf); !ok || operand == 0 {
		t.Fatalf("typeof copy fence = (%v, %v)", operand, ok)
	}
	if inner, ok := operators.KeyOfs().Get(keyOf); !ok || inner == 0 {
		t.Fatalf("keyof copy fence = (%v, %v)", inner, ok)
	}
	if object, _, ok := operators.IndexAccesses().Get(indexAccess); !ok || object == 0 {
		t.Fatalf("indexed access copy fence = (%v, %v)", object, ok)
	}
	if check, _, _, _, ok := operators.Conditionals().Get(conditional); !ok || check == 0 {
		t.Fatalf("conditional copy fence = (%v, %v)", check, ok)
	}
	if _, ok := operators.KeyOfs().At(-1); ok {
		t.Fatal("KeyOfs.At accepted negative index")
	}
	if _, _, ok := operators.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 3)); ok {
		t.Fatal("TypeOfs.Get accepted out-of-range term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		operators.TypeOfs().Get(typeOf)
		operators.KeyOfs().Get(keyOf)
		operators.IndexAccesses().Get(indexAccess)
		operators.Conditionals().Get(conditional)
	}); allocations != 0 {
		t.Fatalf("operator queries allocated %.2f times", allocations)
	}
}

func TestPublicationsPreserveExactDenseRelation(t *testing.T) {
	draft, err := Build(publicationFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	publications := component.View().Publications()
	term, ok := publications.At(0)
	if !ok || term != keyspace.MakeTerm(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publications.At(0) = %v/%v, want publication 1", term, ok)
	}
	assign, pair, target, ok := publications.Get(term)
	if !ok || assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || pair != 0 || target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("Publications.Get() = (%v, %d, %v, %v), want exact authored row", assign, pair, target, ok)
	}
}

func TestPublicationsAcceptResolvedTargetsAndDistinctPairs(t *testing.T) {
	input := publicationFixture(t)
	input.Counts[keyspace.FamilyTypePublication] = 2
	input.Publications.Type = append(input.Publications.Type, Publication{
		Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: math.MaxUint32, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2),
	})
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() rejected Declaration/CanonicalPath targets or maximum uint32 pair: %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	publications := component.View().Publications()
	if publications.Count() != 2 {
		t.Fatalf("Publications.Count() = %d, want 2", publications.Count())
	}
	_, pair, target, ok := publications.Get(keyspace.MakeTerm(keyspace.FamilyTypePublication, 2))
	if !ok || pair != math.MaxUint32 || target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 2) {
		t.Fatalf("second publication = (%d, %v, %v), want maximum pair/canonical target", pair, target, ok)
	}
}

func TestPublicationsRejectInvalidRowsAndLocalForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing publication count", func(input *Input) { input.Counts[keyspace.FamilyTypePublication] = 0 }},
		{"extra publication count", func(input *Input) { input.Counts[keyspace.FamilyTypePublication] = 2 }},
		{"foreign assign", func(input *Input) { input.Publications.Type[0].Assign = keyspace.MakeTerm(keyspace.FamilyAssign, 2) }},
		{"wrong assign family", func(input *Input) { input.Publications.Type[0].Assign = keyspace.MakeTerm(keyspace.FamilyBody, 1) }},
		{"wrong target family", func(input *Input) {
			input.Publications.Type[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"foreign target", func(input *Input) { input.Publications.Type[0].Target = keyspace.MakeTerm(keyspace.FamilyTypeRef, 3) }},
		{"unresolved target", func(input *Input) {
			input.References.TypeRef[0] = TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}}
		}},
		{"duplicate assign pair", func(input *Input) {
			input.Counts[keyspace.FamilyTypePublication] = 2
			input.Publications.Type = append(input.Publications.Type, Publication{
				Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: 0, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2),
			})
		}},
		{"target shared between publications", func(input *Input) {
			input.Counts[keyspace.FamilyAssign] = 2
			input.Counts[keyspace.FamilyTypePublication] = 2
			input.Publications.Type = append(input.Publications.Type, Publication{
				Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 2), Pair: 0, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
			})
		}},
		{"target shared with type value", func(input *Input) {
			input.Counts[keyspace.FamilyTypeValue] = 1
			input.Operands.TypeValue = []TypeValueTarget{{Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)}}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := publicationFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid publication relation")
			}
		})
	}
}

func TestPublicationsCopyFenceBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := publicationFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Publications.Type[0] = Publication{}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	publications := component.View().Publications()
	term := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	assign, pair, target, ok := publications.Get(term)
	if !ok || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("publication copy fence = (%v, %d, %v, %v)", assign, pair, target, ok)
	}
	if _, ok := publications.At(1); ok {
		t.Fatal("Publications.At accepted out-of-bounds index")
	}
	if _, _, _, ok := publications.Get(0); ok {
		t.Fatal("Publications.Get accepted zero term")
	}
	if _, _, _, ok := publications.Get(keyspace.MakeTerm(keyspace.FamilyTypePublication, 2)); ok {
		t.Fatal("Publications.Get accepted foreign ordinal")
	}
	if _, _, _, ok := publications.Get(keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)); ok {
		t.Fatal("Publications.Get accepted foreign family")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		publications.Count()
		publications.At(0)
		publications.Get(term)
	}); allocations != 0 {
		t.Fatalf("publication queries allocated %.2f times", allocations)
	}
}

func TestSignaturesPreserveTypedRelations(t *testing.T) {
	draft, err := Build(signatureFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
	signatures := component.View().Signatures()
	if scope, variadic, coordinate, known, ok := signatures.TypeFunctions().Get(function); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) ||
		variadic != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) || coordinate == (source.Coordinate{}) || !known {
		t.Fatalf("function header = (%v, %v, %v, %v, %v)", scope, variadic, coordinate, known, ok)
	}
	if count, ok := signatures.TypeFunctions().TypeParamCount(function); !ok || count != 1 {
		t.Fatalf("type parameter count = (%d, %v)", count, ok)
	}
	if param, ok := signatures.TypeFunctions().TypeParamAt(function, 0); !ok || param != keyspace.MakeTerm(keyspace.FamilyTypeParam, 1) {
		t.Fatalf("type parameter = (%v, %v)", param, ok)
	}
	if parameter, ok := signatures.TypeFunctions().ParameterAt(function, 0); !ok || parameter.Name != 9 ||
		parameter.NameCoordinate == (source.Coordinate{}) || parameter.Type != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("fixed parameter = (%+v, %v)", parameter, ok)
	}
	if result, ok := signatures.TypeFunctions().ReturnAt(function, 0); !ok || result != assertion {
		t.Fatalf("return = (%v, %v)", result, ok)
	}
	if name, coordinate, bound, ordinal, narrow, ok := signatures.Assertions().Get(assertion); !ok || name != 9 ||
		coordinate == (source.Coordinate{}) || !bound || ordinal != 0 || narrow != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) {
		t.Fatalf("assertion = (%v, %v, %v, %d, %v, %v)", name, coordinate, bound, ordinal, narrow, ok)
	}
}

func TestSignaturesReturnsAndAssertionEncoding(t *testing.T) {
	input := signatureFixture(t)
	input.Signatures.TypeFunction[0].Parameters = nil
	input.Signatures.TypeFunction[0].Variadic = 0
	input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
	input.Signatures.TypeFunction[0].Returns = nil
	input.Signatures.TypeFunction[0].ReturnsKnown = false
	input.Signatures.TypeAsserts[0].Bound = false
	input.Signatures.TypeAsserts[0].Param = 0
	input.Signatures.TypeAsserts[0].Narrow = 0
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() omitted return/error assertion = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	if _, _, _, known, ok := component.View().Signatures().TypeFunctions().Get(function); !ok || known {
		t.Fatalf("omitted returns = (%v, %v)", known, ok)
	}

	input = signatureFixture(t)
	input.Signatures.TypeFunction[0].Parameters = nil
	input.Signatures.TypeFunction[0].Variadic = 0
	input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
	input.Signatures.TypeFunction[0].Returns = nil
	input.Signatures.TypeFunction[0].ReturnsKnown = true
	input.Signatures.TypeAsserts = nil
	input.Counts[keyspace.FamilyTypeAsserts] = 0
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build() explicit empty returns = %v", err)
	}
	component, err = commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	if _, _, _, known, ok := component.View().Signatures().TypeFunctions().Get(function); !ok || !known {
		t.Fatalf("explicit empty returns = (%v, %v)", known, ok)
	}

	input = signatureFixture(t)
	input.Signatures.TypeAsserts[0].Bound = false
	input.Signatures.TypeAsserts[0].Param = 1
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted unbound assertion ordinal")
	}
}

func TestSignaturesRejectCoverageOwnershipScopeAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing signature row", func(input *Input) { input.Signatures.TypeFunction = nil }},
		{"missing assertion row", func(input *Input) { input.Signatures.TypeAsserts = nil }},
		{"anonymous parameter coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].Parameters[0].Name = 0
		}},
		{"named parameter missing coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].Parameters[0].NameCoordinate = source.Coordinate{}
		}},
		{"variadic missing coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
		}},
		{"absent variadic coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].Variadic = 0
		}},
		{"invalid static scope", func(input *Input) {
			input.Counts[keyspace.FamilyBody] = 1
			input.Signatures.TypeFunction[0].Scope = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"orphan type parameter", func(input *Input) {
			input.Signatures.TypeFunction[0].TypeParams = nil
		}},
		{"wrong type parameter owner", func(input *Input) {
			input.Counts[keyspace.FamilyFunction] = 1
			input.Contracts.Function = []FunctionContract{{}}
			input.Declarations.TypeParam[0].Owner = keyspace.MakeTerm(keyspace.FamilyFunction, 1)
		}},
		{"bound assertion wrong name", func(input *Input) {
			input.Signatures.TypeAsserts[0].Name = 10
		}},
		{"bound assertion wrong ordinal", func(input *Input) {
			input.Signatures.TypeAsserts[0].Param = 1
		}},
		{"bound assertion fixed parameter", func(input *Input) {
			input.Signatures.TypeFunction[0].Parameters[0].Type = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
			input.Signatures.TypeFunction[0].Variadic = 0
			input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
			input.Signatures.TypeFunction[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
		}},
		{"bound assertion variadic", func(input *Input) {
			input.Signatures.TypeFunction[0].Variadic = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
			input.Signatures.TypeFunction[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
		}},
		{"bound assertion earlier duplicate name", func(input *Input) {
			coordinate := signatureCoordinate(t)
			input.Signatures.TypeFunction[0].Parameters = append(input.Signatures.TypeFunction[0].Parameters, Parameter{
				Name: 9, NameCoordinate: coordinate, Type: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			})
			input.Signatures.TypeFunction[0].Variadic = 0
			input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
		}},
		{"signature assertion cycle", func(input *Input) {
			input.Signatures.TypeAsserts[0].Narrow = keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
		}},
		{"shared signature child", func(input *Input) {
			input.Signatures.TypeFunction[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}
			input.Counts[keyspace.FamilyTypeAsserts] = 0
			input.Signatures.TypeAsserts = nil
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := signatureFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid signature relation")
			}
		})
	}

	input := declarationFixture(t)
	input.Signatures.TypeFunction[0].Scope = keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted interface method with non-interface signature scope")
	}

	input = signatureFixture(t)
	input.Counts[keyspace.FamilyTypeParam] = 2
	input.Declarations.TypeParam = append(input.Declarations.TypeParam, TypeParam{
		Owner: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1), Name: 11,
	})
	input.Signatures.TypeFunction[0].TypeParams = []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTypeParam, 1), keyspace.MakeTerm(keyspace.FamilyTypeParam, 1),
	}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted duplicate function type parameter")
	}
}

func TestSignatureCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := signatureFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Signatures.TypeFunction[0].TypeParams[0] = 0
	input.Signatures.TypeFunction[0].Parameters[0].Name = 99
	input.Signatures.TypeFunction[0].Returns[0] = 0
	input.Signatures.TypeAsserts[0].Name = 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
	signatures := component.View().Signatures()
	if got, ok := signatures.TypeFunctions().TypeParamAt(function, 0); !ok || got == 0 {
		t.Fatalf("type parameter copy fence = (%v, %v)", got, ok)
	}
	if got, ok := signatures.TypeFunctions().ParameterAt(function, 0); !ok || got.Name != 9 {
		t.Fatalf("parameter copy fence = (%+v, %v)", got, ok)
	}
	if got, ok := signatures.TypeFunctions().ReturnAt(function, 0); !ok || got != assertion {
		t.Fatalf("return copy fence = (%v, %v)", got, ok)
	}
	if name, _, _, _, _, ok := signatures.Assertions().Get(assertion); !ok || name != 9 {
		t.Fatalf("assertion copy fence = (%v, %v)", name, ok)
	}
	if _, ok := signatures.TypeFunctions().ParameterAt(function, -1); ok {
		t.Fatal("ParameterAt accepted negative index")
	}
	if _, ok := signatures.TypeFunctions().ReturnAt(function, 1); ok {
		t.Fatal("ReturnAt accepted out-of-range index")
	}
	if _, _, _, _, ok := signatures.TypeFunctions().Get(keyspace.MakeTerm(keyspace.FamilyTypeFunction, 2)); ok {
		t.Fatal("Functions.Get accepted unknown term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		signatures.TypeFunctions().Get(function)
		signatures.TypeFunctions().TypeParamAt(function, 0)
		signatures.TypeFunctions().ParameterAt(function, 0)
		signatures.TypeFunctions().ReturnAt(function, 0)
		signatures.Assertions().Get(assertion)
	}); allocations != 0 {
		t.Fatalf("signature queries allocated %.2f times", allocations)
	}
}

func TestTypesPreserveExactTypedRelations(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 5
	counts[keyspace.FamilyTypeLiteral] = 1
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeUnion] = 1
	counts[keyspace.FamilyTypeIntersection] = 1
	counts[keyspace.FamilyTypeRef] = 2 // References owns the rows below.
	counts[keyspace.FamilyTypeGeneric] = 1
	counts[keyspace.FamilyTypeArray] = 1
	counts[keyspace.FamilyTypeMap] = 1
	counts[keyspace.FamilyTypeRecord] = 1
	counts[keyspace.FamilyTypeField] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCell] = 1

	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	optional := keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)
	union := keyspace.MakeTerm(keyspace.FamilyTypeUnion, 1)
	generic := keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1)
	array := keyspace.MakeTerm(keyspace.FamilyTypeArray, 1)
	mapType := keyspace.MakeTerm(keyspace.FamilyTypeMap, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	ref := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypeRef, ordinal)
	}

	draft, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{
			{Kind: PrimitiveNil}, {Kind: PrimitiveNumber},
			{Kind: PrimitiveString}, {Kind: PrimitiveBoolean}, {Kind: PrimitiveNever},
		},
		Literal:      []Literal{{Kind: keyspace.LiteralString, Exact: 7}},
		Optional:     []Optional{{Inner: primitive(1)}},
		Union:        []Union{{Members: []keyspace.Term{optional, primitive(2)}}},
		Intersection: []Intersection{{Members: []keyspace.Term{primitive(3), primitive(4)}}},
		Generic:      []Generic{{Base: ref(1), Args: []keyspace.Term{union}}},
		Array:        []Array{{Element: generic, ReadOnly: true}},
		Map:          []Map{{Key: ref(2), Value: array}},
		Field:        []Field{{Key: 9, Type: mapType, Optional: true}},
		Record:       []Record{{Fields: []keyspace.Term{field}, ReadOnly: true}},
	}, References: ReferencesInput{TypeRef: []TypeRef{
		{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{4}},
	}}, Declarations: DeclarationsInput{Alias: []TypeAlias{{
		Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: primitive(5), Name: 10,
		NameCoordinate: func() source.Coordinate { value, _ := source.CoordinateFromParts(1, 1, 1, 2); return value }(),
	}}}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	types := component.View().Types()
	if got := types.Primitives().Count(); got != 5 {
		t.Fatalf("primitive count = %d, want 5", got)
	}
	if kind, ok := types.Primitives().Get(primitive(2)); !ok || kind != PrimitiveNumber {
		t.Fatalf("primitive relation = (%v, %v), want number", kind, ok)
	}
	if kind, exact, bits, ok := types.Literals().Get(keyspace.MakeTerm(keyspace.FamilyTypeLiteral, 1)); !ok ||
		kind != keyspace.LiteralString || exact != 7 || bits != 0 {
		t.Fatalf("literal relation = (%v, %v, %d, %v)", kind, exact, bits, ok)
	}
	if got, ok := types.Unions().MemberCount(union); !ok || got != 2 {
		t.Fatalf("union length = (%d, %v)", got, ok)
	}
	if got, ok := types.Unions().MemberAt(union, 1); !ok || got != primitive(2) {
		t.Fatalf("union member = (%v, %v)", got, ok)
	}
	if base, arity, ok := types.Generics().Get(generic); !ok || base != ref(1) || arity != 1 {
		t.Fatalf("generic relation = (%v, %d, %v)", base, arity, ok)
	}
	if key, typ, optionalField, ok := types.Fields().Get(field); !ok || key != 9 || typ != mapType || !optionalField {
		t.Fatalf("field relation = (%v, %v, %v, %v)", key, typ, optionalField, ok)
	}
	if readOnly, fields, ok := types.Records().Get(keyspace.MakeTerm(keyspace.FamilyTypeRecord, 1)); !ok || !readOnly || fields != 1 {
		t.Fatalf("record relation = (%v, %d, %v)", readOnly, fields, ok)
	}
}

func TestTypesRejectIncompleteOrAmbiguousLocalShape(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeUnion] = 1
	if _, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveNumber}},
		Union:     []Union{{Members: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}}},
	}}); err == nil {
		t.Fatal("Build() accepted one-member union")
	}

	counts = [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeLiteral] = 1
	if _, err := Build(Input{Counts: counts, Types: TypesInput{
		Literal: []Literal{{Kind: keyspace.LiteralString}},
	}}); err == nil {
		t.Fatal("Build() accepted string literal without Source exact key")
	}
}

// This chain is deliberately large enough that the former walk-from-every-
// child algorithm would repeatedly rediscover every suffix. The law asserts
// structure, not a machine-dependent duration: acceptance of the chain and
// rejection after one back-edge exercise the same linear containment seal.
func TestTypesContainmentLongChainAndCycle(t *testing.T) {
	const length = 8192
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeOptional] = length
	rows := make([]Optional, length)
	rows[0] = Optional{Inner: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}
	for index := 1; index < len(rows); index++ {
		rows[index] = Optional{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, uint32(index))}
	}
	input := Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveAny}},
		Optional:  rows,
	}}
	if _, err := Build(input); err != nil {
		t.Fatalf("Build() rejected acyclic containment chain: %v", err)
	}
	input.Types.Optional[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypeOptional, length)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a containment cycle through a long chain")
	}
}

func TestTypesDraftIsOneShot(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	draft, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveAny}},
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	copy := *draft
	if _, err := commitStaticDraft(t, draft); err != nil {
		t.Fatalf("first take() error = %v", err)
	}
	if _, err := commitStaticDraft(t, &copy); err == nil {
		t.Fatal("copied Draft acquired a second component")
	}
}

func TestTypesDraftCopiesConsumeOnceUnderContention(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	draft, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveAny}},
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

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
			_, err := commitStaticDraft(t, &copy)
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
		t.Fatalf("contended Draft takes = %d successes, want exactly 1", successes)
	}
}

func TestContainmentTracksTypedParentsFieldsAndDirectReturns(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyTypePrimitive: 1,
		keyspace.FamilyTypeOptional:  1,
		keyspace.FamilyTypeField:     1,
		keyspace.FamilyTypeAsserts:   1,
	}
	check := newContainment(counts, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	optional := keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	opaqueOwner := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if !check.attach(opaqueOwner, primitive) || !check.attach(primitive, optional) {
		t.Fatal("containment rejected a valid typed parent chain")
	}
	if check.attach(opaqueOwner, primitive) {
		t.Fatal("containment accepted a duplicate concrete child")
	}
	if !check.claimField(opaqueOwner, field) {
		t.Fatal("containment rejected the first Field owner")
	}
	if check.claimField(opaqueOwner, field) {
		t.Fatal("containment accepted duplicate Field ownership")
	}
	assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
	if !check.markDirectReturn(opaqueOwner, assertion) || check.markDirectReturn(opaqueOwner, assertion) {
		t.Fatal("direct assertion-return evidence did not enforce one claim")
	}
	if check.parentOf(optional) != primitive || check.parentOf(field) != opaqueOwner {
		t.Fatal("containment parent lookup lost typed ownership")
	}
	if !check.valid() {
		t.Fatal("valid typed containment chain was rejected")
	}

	cycle := newContainment(counts, 1)
	if !cycle.attach(primitive, optional) || !cycle.attach(optional, primitive) {
		t.Fatal("cycle fixture could not be constructed")
	}
	if cycle.valid() {
		t.Fatal("containment accepted a concrete cycle")
	}
}
