package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func assertPendingExact(t *testing.T, pending *Pending, subject keyspace.Term, want ...keyspace.Term) {
	t.Helper()
	count, ok := pending.Count(subject)
	if !ok || count != len(want) {
		t.Fatalf("Pending.Count(%08x) = %d/%v, want %d/true", uint32(subject), count, ok, len(want))
	}
	for index, expected := range want {
		got, gotOK := pending.At(subject, index)
		if !gotOK || got != expected {
			t.Fatalf("Pending.At(%08x, %d) = %08x/%v, want %08x/true", uint32(subject), index, uint32(got), gotOK, uint32(expected))
		}
		if index != 0 && want[index-1] >= expected {
			t.Fatalf("expected Pending sequence for %08x is not in canonical strict order at %d", uint32(subject), index)
		}
	}
	if _, ok := pending.At(subject, -1); ok {
		t.Fatalf("Pending.At(%08x, -1) accepted a negative index", uint32(subject))
	}
	if _, ok := pending.At(subject, len(want)); ok {
		t.Fatalf("Pending.At(%08x, Count) accepted an out-of-range index", uint32(subject))
	}
}

func assertPendingAbsent(t *testing.T, pending *Pending, subject keyspace.Term) {
	t.Helper()
	if count, ok := pending.Count(subject); ok {
		t.Fatalf("Pending.Count(%08x) = %d/true, want absent", uint32(subject), count)
	}
	if _, ok := pending.At(subject, 0); ok {
		t.Fatalf("Pending.At(%08x, 0) accepted an absent subject", uint32(subject))
	}
}

type exactCandidateView struct {
	count func() int
	at    func(int) (keyspace.Term, bool)
}

func assertCandidateExact(t *testing.T, name string, view exactCandidateView, want ...keyspace.Term) {
	t.Helper()
	if got := view.count(); got != len(want) {
		t.Fatalf("%s.Count() = %d, want %d", name, got, len(want))
	}
	for index, expected := range want {
		got, ok := view.at(index)
		if !ok || got != expected {
			t.Fatalf("%s.At(%d) = %08x/%v, want %08x/true", name, index, uint32(got), ok, uint32(expected))
		}
	}
	if _, ok := view.at(-1); ok {
		t.Fatalf("%s.At(-1) accepted", name)
	}
	if _, ok := view.at(len(want)); ok {
		t.Fatalf("%s.At(Count()) accepted", name)
	}
}

func pendingExactMatrixFixture(t *testing.T, name string) *pendingFixture {
	t.Helper()
	body := pendingTerm(keyspace.FamilyBody, 1)
	return openPendingFixture(t, name, pendingRuntimeMatrixCounts(),
		pendingRuntimeMatrixRows(), pendingRuntimeMatrixFlow(), []source.BindCells{
			{Bind: pendingTerm(keyspace.FamilyBind, 1), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 2)}},
			{Bind: pendingTerm(keyspace.FamilyBind, 2), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 3)}},
			{Bind: pendingTerm(keyspace.FamilyBind, 3), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 4)}},
		}, nil, nil, pendingSourceExtras{
			keys: []source.KeyInput{source.NameKey(body, "field-list"), source.NameKey(body, "field-name"), source.NameKey(body, "method")},
			exactAtoms: []keyspace.LiteralValue{
				{Kind: keyspace.LiteralString, String: "field-list"},
				{Kind: keyspace.LiteralString, String: "field-name"},
				{Kind: keyspace.LiteralString, String: "method"},
			},
		})
}

func TestSealPendingProductionExactCandidateDenominatorAndPrefixes(t *testing.T) {
	fixture := pendingExactMatrixFixture(t, "pending-exact-denominator.lua")
	term := pendingTerm

	unaryNumeric := fixture.candidates.UnaryNumeric()
	length := fixture.candidates.Length()
	arithmetic := fixture.candidates.Arithmetic()
	bitwise := fixture.candidates.Bitwise()
	concat := fixture.candidates.Concat()
	equality := fixture.candidates.Equality()
	order := fixture.candidates.Order()
	indexGet := fixture.candidates.IndexGet()
	indexSet := fixture.candidates.IndexSet()
	assertCandidateExact(t, "UnaryNumeric", exactCandidateView{unaryNumeric.Count, unaryNumeric.At}, term(keyspace.FamilyUnary, 1), term(keyspace.FamilyUnary, 3))
	assertCandidateExact(t, "Length", exactCandidateView{length.Count, length.At}, term(keyspace.FamilyUnary, 2))
	assertCandidateExact(t, "Arithmetic", exactCandidateView{arithmetic.Count, arithmetic.At}, term(keyspace.FamilyBinary, 1))
	assertCandidateExact(t, "Bitwise", exactCandidateView{bitwise.Count, bitwise.At}, term(keyspace.FamilyBinary, 2))
	assertCandidateExact(t, "Concat", exactCandidateView{concat.Count, concat.At}, term(keyspace.FamilyBinary, 3))
	assertCandidateExact(t, "Equality", exactCandidateView{equality.Count, equality.At}, term(keyspace.FamilyBinary, 4))
	assertCandidateExact(t, "Order", exactCandidateView{order.Count, order.At}, term(keyspace.FamilyBinary, 5))
	assertCandidateExact(t, "IndexGet", exactCandidateView{indexGet.Count, indexGet.At}, term(keyspace.FamilyRead, 1), term(keyspace.FamilyRead, 2))
	assertCandidateExact(t, "IndexSet", exactCandidateView{indexSet.Count, indexSet.At}, term(keyspace.FamilyWrite, 2), term(keyspace.FamilyWrite, 3), term(keyspace.FamilyWrite, 4))

	// These are the exact canonical sequences at every admitted subject in the
	// five non-loop planes. They prove nested operand privacy, guarded Select
	// exclusion, method callee/receiver order without duplication, table
	// allocation retention, and assignment target-address ordering.
	expected := map[keyspace.Term][]keyspace.Term{
		term(keyspace.FamilyUnary, 1):  {term(keyspace.FamilyTable, 1)},
		term(keyspace.FamilyUnary, 2):  {term(keyspace.FamilyUnary, 1), term(keyspace.FamilyTable, 1)},
		term(keyspace.FamilyUnary, 3):  {term(keyspace.FamilyLensExact, 2), term(keyspace.FamilyLensKey, 2), term(keyspace.FamilyNil, 7)},
		term(keyspace.FamilyBinary, 1): {term(keyspace.FamilyUnary, 1), term(keyspace.FamilyTable, 1)},
		term(keyspace.FamilyBinary, 2): {term(keyspace.FamilyUnary, 1), term(keyspace.FamilyTable, 1)},
		term(keyspace.FamilyBinary, 3): {term(keyspace.FamilyUnary, 1), term(keyspace.FamilyTable, 1), term(keyspace.FamilyBinary, 2)},
		term(keyspace.FamilyBinary, 4): {term(keyspace.FamilyUnary, 1), term(keyspace.FamilyTable, 1), term(keyspace.FamilyUnary, 2)},
		term(keyspace.FamilyBinary, 5): {term(keyspace.FamilyInteger, 1), term(keyspace.FamilyUnary, 1), term(keyspace.FamilyTable, 1), term(keyspace.FamilyBinary, 2)},
		term(keyspace.FamilyRead, 1):   {},
		term(keyspace.FamilyRead, 2):   {term(keyspace.FamilyRead, 1), term(keyspace.FamilyRead, 6)},
		term(keyspace.FamilyWrite, 2):  {},
		term(keyspace.FamilyWrite, 3):  {term(keyspace.FamilyLensExact, 2)},
		term(keyspace.FamilyWrite, 4):  {term(keyspace.FamilyLensExact, 2), term(keyspace.FamilyLensKey, 2)},
		term(keyspace.FamilyCall, 1):   {},
		term(keyspace.FamilyCall, 2): {
			term(keyspace.FamilyFloat, 1), term(keyspace.FamilyUnary, 1), term(keyspace.FamilyBinary, 1),
			term(keyspace.FamilyTable, 1), term(keyspace.FamilyTypeValue, 1), term(keyspace.FamilyValueClaim, 1), term(keyspace.FamilyRead, 3),
		},
		term(keyspace.FamilyCall, 3): {term(keyspace.FamilyInteger, 1), term(keyspace.FamilyUnary, 1), term(keyspace.FamilyTable, 1), term(keyspace.FamilyBinary, 2)},
	}

	admitted := 0
	counts := pendingRuntimeMatrixCounts()
	for _, family := range []keyspace.Family{
		keyspace.FamilyUnary, keyspace.FamilyBinary, keyspace.FamilyRead,
		keyspace.FamilyWrite, keyspace.FamilyCall,
	} {
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			subject := term(family, ordinal)
			want, exists := expected[subject]
			shouldExist := pendingSubject(fixture.flowView, fixture.executable, fixture.candidates, subject)
			if exists != shouldExist {
				t.Fatalf("exact expected denominator for %08x = %v, executable∩candidate = %v", uint32(subject), exists, shouldExist)
			}
			if !exists {
				assertPendingAbsent(t, fixture.pending, subject)
				continue
			}
			admitted++
			assertPendingExact(t, fixture.pending, subject, want...)
		}
	}
	if admitted != len(expected) {
		t.Fatalf("admitted exact subject rows = %d, want %d", admitted, len(expected))
	}

	// Nested Binary1 contributes its own opaque identity but never flattens
	// Binary2/Binary3 operands into Call2. Select1's guarded leaves are absent.
	for _, forbidden := range []keyspace.Term{
		term(keyspace.FamilyBinary, 2), term(keyspace.FamilyBinary, 3),
		term(keyspace.FamilyNil, 17), term(keyspace.FamilyNil, 18),
	} {
		count, _ := fixture.pending.Count(term(keyspace.FamilyCall, 2))
		for index := 0; index < count; index++ {
			got, _ := fixture.pending.At(term(keyspace.FamilyCall, 2), index)
			if got == forbidden {
				t.Fatalf("private/guarded operand %08x leaked into Call2", uint32(forbidden))
			}
		}
	}
}

func TestSealPendingProductionExactGenericLoopPlane(t *testing.T) {
	fixture := openPendingLoopFixture(t, "pending-exact-generic-loop.lua")
	generic := fixture.candidates.GenericLoop()
	assertCandidateExact(t, "GenericLoop", exactCandidateView{generic.Count, generic.At}, pendingTerm(keyspace.FamilyLoop, 4))
	for ordinal := uint32(1); ordinal <= 4; ordinal++ {
		subject := pendingTerm(keyspace.FamilyLoop, ordinal)
		shouldExist := pendingSubject(fixture.flowView, fixture.executable, fixture.candidates, subject)
		if ordinal == 4 {
			if !shouldExist {
				t.Fatal("GenericLoop4 was not executable∩candidate")
			}
			assertPendingExact(t, fixture.pending, subject)
			continue
		}
		if shouldExist {
			t.Fatalf("non-generic Loop%d entered executable∩candidate", ordinal)
		}
		assertPendingAbsent(t, fixture.pending, subject)
	}
}

func TestSealPendingProductionSharedCellKeyAndBodyReferences(t *testing.T) {
	fixture := pendingExactMatrixFixture(t, "pending-shared-references.lua")
	term := pendingTerm
	body := term(keyspace.FamilyBody, 1)
	sharedCell := term(keyspace.FamilyCell, 5)
	for ordinal := uint32(3); ordinal <= 6; ordinal++ {
		owner, sourceTerm, _, ok := fixture.flowView.Storage().Reads().Get(term(keyspace.FamilyRead, ordinal))
		if !ok || owner != body || sourceTerm != sharedCell {
			t.Fatalf("Read%d did not retain the shared Body/Cell reference", ordinal)
		}
	}
	cellKind, cellBody, cellKey, ok := fixture.flowView.Storage().Cells().Get(sharedCell)
	if !ok || cellKind != authored.CellGlobal || cellBody != 0 || cellKey == 0 {
		t.Fatal("shared global Cell row was unavailable")
	}
	lensOwner, _, sourceKey, fieldKind, ok := fixture.flowView.Access().Exact().Get(term(keyspace.FamilyLensExact, 1))
	keyOwner, _, exactKey, keyOK := fixture.sourceView.Keys().Name(sourceKey)
	if !ok || !keyOK || lensOwner != body || keyOwner != body || fieldKind != kind.FieldName || exactKey != cellKey {
		t.Fatal("shared Source Key did not agree across Lens and global Cell references")
	}
	if !MatchesPending(fixture.pending, fixture.sourceView.Identity().ContentID(), fixture.flowView.Cold().ContentID(), fixture.staticID, fixture.moduleID) {
		t.Fatal("shared Cell/Key/Body references prevented genuine SealPending publication")
	}
}
