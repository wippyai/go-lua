package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"sync"
	"testing"
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
	component.contentID = identity.ContentID{}
	if got := cold.ContentID(); got != want {
		t.Fatalf("Cold snapshot changed after Component mutation: %x != %x", got, want)
	}
}

// referenceInput supplies complete declaration rows whenever a TypeRef test
// reserves a declaration family. The static boundary rejects counted-but-
// absent rows; tests that exercise a target must therefore carry its owner.
func referenceInput(counts [keyspace.FamilyCount]uint32, refs ReferencesInput) Input {
	input := Input{Counts: counts, References: refs}
	if counts[keyspace.FamilyTypeAlias] != 0 {
		input.Counts[keyspace.FamilyBody] = 1
		// Keep the declaration target distinct from any relation under test so
		// the structural forest test does not hide the Reference assertion.
		input.Counts[keyspace.FamilyTypePrimitive] = 2
		input.Types.Primitive = []Primitive{{Kind: PrimitiveAny}, {Kind: PrimitiveNever}}
		coordinate, _ := source.CoordinateFromParts(1, 1, 1, 2)
		params := []keyspace.Term(nil)
		if counts[keyspace.FamilyTypeParam] != 0 {
			params = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)}
			input.Declarations.TypeParam = []TypeParam{{
				Owner: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), Name: 1,
			}}
		}
		input.Declarations.Alias = []TypeAlias{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			Name: 1, NameCoordinate: coordinate, Params: params,
		}}
	}
	if counts[keyspace.FamilyTypeInterface] != 0 {
		input.Counts[keyspace.FamilyBody] = 1
		coordinate, _ := source.CoordinateFromParts(1, 1, 1, 2)
		input.Declarations.Interface = []Interface{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Name: 2, NameCoordinate: coordinate,
		}}
	}
	return input
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
