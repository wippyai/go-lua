package source

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func commitSource(draft *Draft, index IndexInput) (*Component, error) {
	index = ownedIndex(draft, index)
	finalizer, err := draft.Finalizer()
	if err != nil {
		return nil, err
	}
	return finalizer.Commit(index)
}

func ownedIndex(draft *Draft, index IndexInput) IndexInput {
	if draft != nil && draft.state != nil {
		draft.state.mu.Lock()
		if draft.state.authority != nil {
			index.SourceID = draft.state.authority.content
		}
		draft.state.mu.Unlock()
	}
	return index
}

func TestSourceBuildRetainsOwnedRowsAndSealProjection(t *testing.T) {
	input, index := sourceFixture(2)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	view := component.View()

	if got, want := view.Identity().Name(), input.Name; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := view.Identity().TermCount(), uint32(11); got != want {
		t.Fatalf("TermCount = %d, want %d", got, want)
	}
	nilOne := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	if span, ok := view.Identity().Span(nilOne); !ok || span.File != input.Name || span.StartLine != 1 || span.StartCol != 1 {
		t.Fatalf("Span = %#v, %v", span, ok)
	}
	if got, ok := view.Order().BodyLen(keyspace.MakeTerm(keyspace.FamilyBody, 1)); !ok || got != 1 {
		t.Fatalf("BodyLen = %d, %v", got, ok)
	}
	if got, ok := view.Order().BodyAt(keyspace.MakeTerm(keyspace.FamilyBody, 2), 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyReturn, 1) {
		t.Fatalf("BodyAt = %v, %v", got, ok)
	}
	if got, ok := view.Binds().At(keyspace.MakeTerm(keyspace.FamilyBind, 1), 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyCell, 1) {
		t.Fatalf("Bind cell = %v, %v", got, ok)
	}
	if got, ok := view.Formals().At(keyspace.MakeTerm(keyspace.FamilyFunction, 1), 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyCell, 2) {
		t.Fatalf("Formal cell = %v, %v", got, ok)
	}
	if root, body, offset, ok := view.Index().Position(keyspace.MakeTerm(keyspace.FamilyFunction, 1)); !ok ||
		root != keyspace.MakeTerm(keyspace.FamilyBody, 1) || body != 0 || offset != 0 {
		t.Fatalf("Position = %v, %d, %d, %v", root, body, offset, ok)
	}
	if body, cursor, ok := view.Index().Frontier(keyspace.MakeTerm(keyspace.FamilyFunction, 1)); !ok ||
		body != keyspace.MakeTerm(keyspace.FamilyBody, 1) || cursor != 0 {
		t.Fatalf("Frontier = %v, %d, %v", body, cursor, ok)
	}
	if parent, ok := view.Index().BodyParent(keyspace.MakeTerm(keyspace.FamilyBody, 2)); !ok || parent != keyspace.MakeTerm(keyspace.FamilyBody, 1) {
		t.Fatalf("BodyParent = %v, %v", parent, ok)
	}
	if entry, ok := view.Index().Entry(); !ok || entry != keyspace.MakeTerm(keyspace.FamilyBody, 1) {
		t.Fatalf("Entry = %v, %v", entry, ok)
	}
	if term, owner, value, ok := view.Literals().Integers().At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyInteger, 1) ||
		owner != keyspace.MakeTerm(keyspace.FamilyBody, 2) || value != 42 {
		t.Fatalf("Integer = %v, %v, %d, %v", term, owner, value, ok)
	}
	if got := view.Identity().ContentID(); !got.Available() {
		t.Fatal("unavailable authored content identity")
	}
}

func TestSourceBuildRetainsOnlyOwnedRows(t *testing.T) {
	input, index := contentFixture()
	wantName := input.Name
	wantSpan := input.Families[int(keyspace.FamilyNil)-1].Spans[0]
	wantFloatBits := input.Float[0].Bits
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	wantContent := draft.state.authority.content

	// Build must own every caller-backed row before Finalizer or Preimage is
	// issued. Mutate both outer row headers and nested backing arrays to
	// different, valid-looking values; no unsafe aliasing is involved.
	input.Name = "mutated.lua"
	for family := range input.Families {
		input.Families[family].Family = keyspace.FamilyBody
		for span := range input.Families[family].Spans {
			input.Families[family].Spans[span] = Span{
				File: "mutated.lua", StartLine: 101, StartCol: 7, EndLine: 101, EndCol: 9,
			}
		}
	}
	input.Nil[0].Owner = keyspace.MakeTerm(keyspace.FamilyBody, 2)
	input.Bool[0] = BoolLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 2), Value: false}
	input.Integer[0] = IntegerLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: 43}
	input.Float[0] = FloatLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 2), Bits: 0x400d000000000000}
	input.String[0] = StringLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: "mutated"}
	input.Bodies[0].Body = keyspace.MakeTerm(keyspace.FamilyBody, 2)
	input.Bodies[0].Terms[0] = keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	input.Bodies[0].Terms[1] = keyspace.MakeTerm(keyspace.FamilyBind, 1)
	input.Bodies[1].Terms[0] = keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	input.Binds[0].Cells[0] = keyspace.MakeTerm(keyspace.FamilyCell, 3)
	input.Binds[0].Cells[1] = keyspace.MakeTerm(keyspace.FamilyCell, 4)
	input.Functions[0].Formals[0] = keyspace.MakeTerm(keyspace.FamilyCell, 1)
	input.Functions[0].Formals[1] = keyspace.MakeTerm(keyspace.FamilyCell, 2)
	input.Keys[0] = NameKey(keyspace.MakeTerm(keyspace.FamilyBody, 2), "mutated-z")
	input.Keys[1] = ListKey(keyspace.MakeTerm(keyspace.FamilyBody, 2), 2)
	input.Keys[2] = NameKey(keyspace.MakeTerm(keyspace.FamilyBody, 2), "mutated-a")
	input.Faults[0] = ControlFault{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 2), Kind: ControlFaultUndefinedGoto}
	input.ExactAtoms[0] = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 2}
	input.ExactAtoms[1] = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "mutated-a"}
	input.ExactAtoms[2] = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "mutated-z"}

	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer: %v", err)
	}
	preimage := finalizer.Preimage()
	assertOwnedSourceRows(t, "Preimage", preimage.Identity(), preimage.Order(), preimage.Binds(), preimage.Formals(), preimage.Literals(), preimage.Keys(), preimage.Faults(), wantName, wantSpan, wantFloatBits, wantContent)
	component, err := finalizer.Commit(ownedIndex(draft, index))
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	view := component.View()
	assertOwnedSourceRows(t, "Component", view.Identity(), view.Order(), view.Binds(), view.Formals(), view.Literals(), view.Keys(), view.Faults(), wantName, wantSpan, wantFloatBits, wantContent)
}

func assertOwnedSourceRows(t *testing.T, label string, identity Identity, order Order, binds BindOrder, formals FormalOrder, literals Literals, keys Keys, faults Faults, wantName string, wantSpan Span, wantFloatBits uint64, wantContent keyspace.ContentID) {
	t.Helper()
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	if got := identity.Name(); got != wantName {
		t.Errorf("%s Name = %q, want %q", label, got, wantName)
	}
	if got := identity.TermCount(); got != 19 {
		t.Errorf("%s TermCount = %d, want 19", label, got)
	}
	if got, ok := identity.Span(keyspace.MakeTerm(keyspace.FamilyNil, 1)); !ok || got != wantSpan {
		t.Errorf("%s Nil span = %#v/%v, want %#v/true", label, got, ok, wantSpan)
	}
	if got := identity.ContentID(); got != wantContent {
		t.Errorf("%s ContentID = %x, want %x", label, got, wantContent)
	}
	if got, ok := order.BodyLen(body1); !ok || got != 2 {
		t.Errorf("%s Body1 len = %d/%v, want 2/true", label, got, ok)
	}
	if got, ok := order.BodyAt(body1, 0); !ok || got != bind {
		t.Errorf("%s Body1[0] = %v/%v, want Bind1/true", label, got, ok)
	}
	if got, ok := order.BodyAt(body1, 1); !ok || got != fault {
		t.Errorf("%s Body1[1] = %v/%v, want ControlFault1/true", label, got, ok)
	}
	if got, ok := order.BodyAt(body2, 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyReturn, 1) {
		t.Errorf("%s Body2[0] = %v/%v, want Return1/true", label, got, ok)
	}
	if got, ok := binds.Len(bind); !ok || got != 2 {
		t.Errorf("%s Bind len = %d/%v, want 2/true", label, got, ok)
	}
	for at, want := range []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1), keyspace.MakeTerm(keyspace.FamilyCell, 2)} {
		if got, ok := binds.At(bind, at); !ok || got != want {
			t.Errorf("%s Bind[%d] = %v/%v, want %v/true", label, at, got, ok, want)
		}
	}
	if got, ok := formals.Len(function); !ok || got != 2 {
		t.Errorf("%s Formals len = %d/%v, want 2/true", label, got, ok)
	}
	for at, want := range []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 3), keyspace.MakeTerm(keyspace.FamilyCell, 4)} {
		if got, ok := formals.At(function, at); !ok || got != want {
			t.Errorf("%s Formal[%d] = %v/%v, want %v/true", label, at, got, ok, want)
		}
	}
	if term, owner, ok := literals.Nils().At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyNil, 1) || owner != body1 {
		t.Errorf("%s Nil = %v/%v/%v, want Nil1/Body1/true", label, term, owner, ok)
	}
	if term, owner, value, ok := literals.Bools().At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyBool, 1) || owner != body1 || !value {
		t.Errorf("%s Bool = %v/%v/%v/%v, want Bool1/Body1/true/true", label, term, owner, value, ok)
	}
	if term, owner, value, ok := literals.Integers().At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyInteger, 1) || owner != body2 || value != 42 {
		t.Errorf("%s Integer = %v/%v/%d/%v, want Integer1/Body2/42/true", label, term, owner, value, ok)
	}
	if term, owner, bits, ok := literals.Floats().At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyFloat, 1) || owner != body1 || bits != wantFloatBits {
		t.Errorf("%s Float = %v/%v/%x/%v, want Float1/Body1/%x/true", label, term, owner, bits, ok, wantFloatBits)
	}
	if term, owner, value, ok := literals.Strings().At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyString, 1) || owner != body2 || value != "original" {
		t.Errorf("%s String = %v/%v/%q/%v, want String1/Body2/original/true", label, term, owner, value, ok)
	}
	key1 := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	key2 := keyspace.MakeTerm(keyspace.FamilyKey, 2)
	key3 := keyspace.MakeTerm(keyspace.FamilyKey, 3)
	if owner, text, exact, ok := keys.Name(key1); !ok || owner != body1 || text != "z" || exact != 3 {
		t.Errorf("%s NameKey1 = %v/%q/%v/%v, want Body1/z/3/true", label, owner, text, exact, ok)
	}
	if owner, ordinal, exact, ok := keys.List(key2); !ok || owner != body1 || ordinal != 1 || exact != 1 {
		t.Errorf("%s ListKey2 = %v/%d/%v/%v, want Body1/1/1/true", label, owner, ordinal, exact, ok)
	}
	if owner, text, exact, ok := keys.Name(key3); !ok || owner != body1 || text != "a" || exact != 2 {
		t.Errorf("%s NameKey3 = %v/%q/%v/%v, want Body1/a/2/true", label, owner, text, exact, ok)
	}
	for at, want := range []keyspace.LiteralValue{{Kind: keyspace.LiteralInteger, Integer: 1}, {Kind: keyspace.LiteralString, String: "a"}, {Kind: keyspace.LiteralString, String: "z"}} {
		key, got, ok := keys.ExactAt(at)
		if !ok || key != keyspace.Key(at+1) || got != want {
			t.Errorf("%s ExactAt(%d) = %v/%#v/%v, want %v/%#v/true", label, at, key, got, ok, at+1, want)
		}
	}
	if got, ok := faults.At(fault); !ok || got != (ControlFault{Owner: body1, Kind: ControlFaultDuplicateLabel, Label: keyspace.MakeTerm(keyspace.FamilyLabel, 1)}) {
		t.Errorf("%s Fault = %#v/%v, want original duplicate-label row", label, got, ok)
	}
}

func TestSourceContentExcludesSealProjection(t *testing.T) {
	firstInput, firstIndex := keyFaultFixture()
	secondInput, secondIndex := keyFaultFixture()
	// Move one non-direct literal beneath the other direct root in the same
	// Body. Authored rows are unchanged; only the Flow-supplied position
	// projection differs.
	fault := keyspace.MakeTerm(keyspace.FamilyControlFault, 1)
	boolTerm := keyspace.MakeTerm(keyspace.FamilyBool, 1)
	faultPosition, ok := sourcePositionFor(secondIndex.Positions, fault)
	if !ok {
		t.Fatal("fixture lost ControlFault position")
	}
	faultPosition.Term = boolTerm
	for at := range secondIndex.Positions {
		if secondIndex.Positions[at].Term == boolTerm {
			secondIndex.Positions[at] = faultPosition
		}
	}

	first, err := Build(firstInput)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	firstComponent, err := commitSource(first, firstIndex)
	if err != nil {
		t.Fatalf("first Finalize: %v", err)
	}
	second, err := Build(secondInput)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	secondComponent, err := commitSource(second, secondIndex)
	if err != nil {
		t.Fatalf("second Finalize: %v", err)
	}
	if got, want := secondComponent.View().Identity().ContentID(), firstComponent.View().Identity().ContentID(); got != want {
		t.Fatalf("content identity included Seal-only source projection: %x != %x", got, want)
	}
}

func TestSourceRejectsBadFrontierAndBodyContainment(t *testing.T) {
	input, index := sourceFixture(2)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	for at := range index.Positions {
		if index.Positions[at].Term == returned {
			index.Positions[at].FrontierCursor = 2
		}
	}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("accepted unbounded ordinary frontier")
	}

	input, index = repeatFixture()
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	// Body 2 is a direct source child of Body 1, but the sealed forest claims
	// that it belongs below Body 3. The projection must preserve that witness.
	index.Bodies[1].Parent = body3
	index.Bodies[2].Parent = body1
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("accepted direct Body with mismatched parent")
	}
}

func TestSourceAllowsTypedChildBodyWithoutDirectSourceOccurrence(t *testing.T) {
	input, index := sourceFixture(2)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	// The fixture already models Body 2 as a typed Function/Branch/Loop child,
	// without a duplicate direct Body term in Body 1's authored sequence.

	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize typed child Body: %v", err)
	}
	view := component.View()
	if parent, ok := view.Index().BodyParent(body2); !ok || parent != body1 {
		t.Fatalf("BodyParent = %v, %v; want %v", parent, ok, body1)
	}
	if _, _, _, ok := view.Index().Position(body2); ok {
		t.Fatal("typed child Body unexpectedly acquired a source Position")
	}
	if _, ok := view.Index().Root(body2); ok {
		t.Fatal("typed child Body unexpectedly acquired a source Root")
	}
	if _, _, ok := view.Index().Frontier(body2); ok {
		t.Fatal("typed child Body unexpectedly acquired a source Frontier")
	}
}

func TestSourceRejectsEntryDirectSourceOccurrence(t *testing.T) {
	input, index := sourceFixture(1)
	entry := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	// Make the Entry itself a well-shaped direct source/root occurrence and
	// provide the corresponding position. The parentless Entry is the sole
	// forest root and must never also have a direct Source witness.
	input.Bodies[0].Terms = append(input.Bodies[0].Terms, entry)
	index.Bodies[0].Roots = append(index.Bodies[0].Roots, entry)
	appendCanonicalFixturePosition(&index, Position{
		Term: entry, Root: entry, Body: entry, Offset: 1, Cursor: 1,
		FrontierBody: entry, FrontierCursor: 1,
	})

	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct source occurrence for the Entry Body")
	}
}

func TestSourceAllowsMissingNonDirectPositionFamily(t *testing.T) {
	input, index := sourceFixture(2)
	missing := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	index.Positions = removeSourcePosition(index.Positions, missing)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if _, _, _, ok := component.View().Index().Position(missing); ok {
		t.Fatal("non-direct Cell unexpectedly acquired a source position")
	}
}

func TestSourceSparsePositionQueriesFailClosed(t *testing.T) {
	input, index := sourceFixture(1)
	index.OutcomeOrigins = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyBody, 1)}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	view := component.View()
	missing := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyBody, 1), // Entry Body
		keyspace.MakeTerm(keyspace.FamilyCell, 1), // Global/local Cell identity
		keyspace.MakeTerm(keyspace.FamilyCell, 2), // Chunk/function Cell identity
		keyspace.MakeTerm(keyspace.FamilyOutcome, 1),
	}
	for _, term := range missing {
		if _, _, _, ok := view.Index().Position(term); ok {
			t.Fatalf("Position(%v) unexpectedly succeeded", term)
		}
		if _, ok := view.Index().Root(term); ok {
			t.Fatalf("Root(%v) unexpectedly succeeded", term)
		}
		if _, _, ok := view.Index().Frontier(term); ok {
			t.Fatalf("Frontier(%v) unexpectedly succeeded", term)
		}
	}
}

func TestSourceRejectsImportInDirectBodyOrder(t *testing.T) {
	input, _ := exactDirectBodyFixture()
	for index := range input.Families {
		if input.Families[index].Family == keyspace.FamilyImport {
			input.Families[index].Spans = []Span{{File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}}
		}
	}
	input.Bodies[0].Terms = append(input.Bodies[0].Terms, keyspace.MakeTerm(keyspace.FamilyImport, 1))
	if _, err := Build(input); err == nil {
		t.Fatal("Build accepted a Module Import in direct Body order")
	}
}

func TestSourceDirectBodyFamilyMatrix(t *testing.T) {
	admitted := map[keyspace.Family]struct{}{
		keyspace.FamilyBody:          {},
		keyspace.FamilyBind:          {},
		keyspace.FamilyAssign:        {},
		keyspace.FamilyCall:          {},
		keyspace.FamilyBranch:        {},
		keyspace.FamilyLoop:          {},
		keyspace.FamilyReturn:        {},
		keyspace.FamilyBreak:         {},
		keyspace.FamilyGoto:          {},
		keyspace.FamilyLabel:         {},
		keyspace.FamilyControlFault:  {},
		keyspace.FamilyTypeAlias:     {},
		keyspace.FamilyTypeInterface: {},
	}
	for family := keyspace.FamilyInvalid; family <= keyspace.FamilyCount; family++ {
		_, want := admitted[family]
		if got := sourceDirectFamily(family); got != want {
			t.Fatalf("sourceDirectFamily(%d) = %v, want %v", family, got, want)
		}
	}
}

func TestSourceRejectsNonDirectBodyFamilies(t *testing.T) {
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if sourceDirectFamily(family) || family == keyspace.FamilyOutcome {
			continue
		}
		t.Run("family-"+strconv.Itoa(int(family)), func(t *testing.T) {
			input, _ := exactDirectBodyFixture()
			for index := range input.Families {
				if input.Families[index].Family == family && len(input.Families[index].Spans) == 0 {
					input.Families[index].Spans = []Span{{File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}}
				}
			}
			input.Bodies[0].Terms = append(input.Bodies[0].Terms, keyspace.MakeTerm(family, 1))
			if _, err := Build(input); err == nil {
				t.Fatalf("Build accepted non-direct Body family %d", family)
			}
		})
	}
}

func exactDirectBodyFixture() (Input, IndexInput) {
	input, index := sourceFixture(1)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	for index := range input.Families {
		if input.Families[index].Family == keyspace.FamilyFloat || input.Families[index].Family == keyspace.FamilyString || input.Families[index].Family == keyspace.FamilyKey {
			input.Families[index].Spans = []Span{{File: input.Name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1}}
		}
	}
	input.Float = []FloatLiteral{{Owner: body1, Bits: 0x3ff0000000000000}}
	input.String = []StringLiteral{{Owner: body2, Value: "direct"}}
	input.ExactAtoms = []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "d"}}
	input.Keys = []KeyInput{NameKey(body1, "d")}
	input.Bodies[0].Terms = []keyspace.Term{bind, body2}
	input.Bodies[1].Terms = nil
	index.Bodies[0].Roots = append([]keyspace.Term(nil), input.Bodies[0].Terms...)
	index.Bodies[1].Roots = nil
	index.Positions = nil
	appendCanonicalFixturePosition(&index, Position{
		Term: bind, Root: bind, Body: body1, FrontierBody: body1,
	})
	appendCanonicalFixturePosition(&index, Position{
		Term: body2, Root: body2, Body: body1, Offset: 1, Cursor: 1,
		FrontierBody: body1, FrontierCursor: 1,
	})
	return input, index
}

func TestSourceRejectsDirectPositionSubstitution(t *testing.T) {
	input, index := sourceFixture(2)
	missingReturn := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	index.Positions = removeSourcePosition(index.Positions, missingReturn)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build missing direct Return position: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct Return source Term omission")
	}

	input, index = sourceFixture(2)
	missing := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	index.Positions = removeSourcePosition(index.Positions, missing)
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct source Term omission")
	}

	input, index = sourceFixture(2)
	if len(index.Positions) == 0 {
		t.Fatal("fixture unexpectedly empty")
	}
	index.Positions[0] = index.Positions[1]
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build duplicate: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a duplicate Position.Term")
	}

	input, index = sourceFixture(2)
	index.Positions[0].Term = keyspace.MakeTerm(keyspace.FamilyCell, 99)
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build invalid: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted an invalid Position.Term")
	}

	input, index = sourceFixture(2)
	index.Positions[0], index.Positions[1] = index.Positions[1], index.Positions[0]
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build noncanonical order: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted noncanonical Position order")
	}

	input, index = sourceFixture(2)
	term := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	row, ok := sourcePositionFor(index.Positions, term)
	if !ok {
		t.Fatal("Bind position missing from fixture")
	}
	other, ok := sourcePositionFor(index.Positions, keyspace.MakeTerm(keyspace.FamilyReturn, 1))
	if !ok {
		t.Fatal("Return position missing from fixture")
	}
	row.Root, row.Body, row.Offset, row.Cursor = other.Root, other.Body, other.Offset, other.Cursor
	row.FrontierBody, row.FrontierCursor = other.FrontierBody, other.FrontierCursor
	for at := range index.Positions {
		if index.Positions[at].Term == term {
			index.Positions[at] = row
		}
	}
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build root mismatch: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted a direct source Term under another root")
	}
}

func TestSourceRejectsNoncanonicalPositionOrder(t *testing.T) {
	input, index := sourceFixture(2)
	if len(index.Positions) < 2 {
		t.Fatal("fixture unexpectedly empty")
	}
	// The encoded Term value is not the ordering key: the batch is ordered by
	// explicit (TermFamily, TermOrdinal), so swapping these first rows must be
	// rejected even though every row remains individually well formed.
	index.Positions[0], index.Positions[1] = index.Positions[1], index.Positions[0]
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("Finalize accepted noncanonical Position order")
	}
}

func TestSourceRejectsUnorderedAuthoredAndSealRows(t *testing.T) {
	input, index := sourceFixture(2)
	input.Families[0], input.Families[1] = input.Families[1], input.Families[0]
	if _, err := Build(input); err == nil {
		t.Fatal("accepted reordered canonical family rows")
	}

	input, index = keyFaultFixture()
	index.Bodies[0].Roots[0], index.Bodies[0].Roots[1] = index.Bodies[0].Roots[1], index.Bodies[0].Roots[0]
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("accepted unordered statement roots")
	}

	input, index = sourceFixture(2)
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := commitSource(draft, index); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if _, err := commitSource(draft, index); err == nil {
		t.Fatal("accepted a second finalization")
	}
}

func TestSourceRejectsNonBodyLiteralOwner(t *testing.T) {
	input, _ := sourceFixture(2)
	input.Nil[0].Owner = keyspace.MakeTerm(keyspace.FamilyBind, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("accepted literal with a non-Body owner")
	}
}

func TestSourceProjectionScalesWithAuthoredTerms(t *testing.T) {
	for _, width := range []int{1, 17, 1024} {
		input, index := sourceFixture(width)
		draft, err := Build(input)
		if err != nil {
			t.Fatalf("Build(%d): %v", width, err)
		}
		component, err := commitSource(draft, index)
		if err != nil {
			t.Fatalf("Finalize(%d): %v", width, err)
		}
		last := keyspace.MakeTerm(keyspace.FamilyNil, uint32(width))
		if _, _, _, ok := component.View().Index().Position(last); !ok {
			t.Fatalf("Position(%d) missing", width)
		}
	}
}

func TestSourceSparseProjectionScalesWithPositions(t *testing.T) {
	var retained []int
	var identityTerms []uint32
	for _, unusedLoops := range []int{0, 100000} {
		input, index := sparsePositionFixture(unusedLoops)
		draft, err := Build(input)
		if err != nil {
			t.Fatalf("Build(%d): %v", unusedLoops, err)
		}
		component, err := commitSource(draft, index)
		if err != nil {
			t.Fatalf("Finalize(%d): %v", unusedLoops, err)
		}

		slots := 0
		for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
			slots += len(component.authority.index.positions[family])
		}
		if got, want := slots, len(index.Positions); got != want {
			t.Fatalf("retained position slots(%d) = %d, want Positions count %d", unusedLoops, got, want)
		}
		retained = append(retained, slots)
		identityTerms = append(identityTerms, component.View().Identity().TermCount())

		term := keyspace.MakeTerm(keyspace.FamilyNil, 1)
		if body, offset, cursor, ok := component.View().Index().Position(term); !ok ||
			body != keyspace.MakeTerm(keyspace.FamilyBody, 1) || offset != 0 || cursor != 0 {
			t.Fatalf("Position(%d) = %v/%d/%d/%v", unusedLoops, body, offset, cursor, ok)
		}
	}
	if retained[0] != retained[1] || retained[0] != 2 {
		t.Fatalf("retained sparse position slots changed with unused family cardinality: %v", retained)
	}
	if identityTerms[1] <= identityTerms[0] {
		t.Fatalf("large sparse identity did not increase final family cardinality: %v", identityTerms)
	}
}

func TestSourcePositionSlicesRetainExactBatchWithoutCapacitySlack(t *testing.T) {
	input, index := sourceFixture(17)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	retained := 0
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		positions := component.authority.index.positions[family]
		if len(positions) == 0 {
			continue
		}
		if cap(positions) != len(positions) {
			t.Fatalf("family %d position capacity = %d, want exact length %d", family, cap(positions), len(positions))
		}
		retained += len(positions)
	}
	if retained != len(index.Positions) {
		t.Fatalf("retained positions = %d, want %d", retained, len(index.Positions))
	}
}

func TestSourceDirectLocationScratchScalesWithDirectRows(t *testing.T) {
	var retained []int
	for _, unusedLoops := range []int{0, 100000} {
		input, index := sparsePositionFixture(unusedLoops)
		draft, err := Build(input)
		if err != nil {
			t.Fatalf("Build(%d): %v", unusedLoops, err)
		}
		a := draft.state.authority
		var next indexStore
		next.rootRanges = make([]termRange, a.count(keyspace.FamilyBody))
		next.parents = make([]keyspace.Term, a.count(keyspace.FamilyBody))
		if err := installBodyRoots(a, &next, index.Bodies); err != nil {
			t.Fatalf("installBodyRoots(%d): %v", unusedLoops, err)
		}
		locations, err := buildDirectLocations(a, &next)
		if err != nil {
			t.Fatalf("buildDirectLocations(%d): %v", unusedLoops, err)
		}
		rows := 0
		for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
			rows += len(locations[family].rows)
		}
		if rows != len(a.order.sourceTerms) {
			t.Fatalf("direct rows(%d) = %d, want authored direct rows %d", unusedLoops, rows, len(a.order.sourceTerms))
		}
		retained = append(retained, rows)
	}
	if retained[0] != retained[1] || retained[0] != 1 {
		t.Fatalf("direct-location scratch grew with non-direct family cardinality: %v", retained)
	}
}

func TestSourceIndexQueriesAllocateNothing(t *testing.T) {
	input, index := sparsePositionFixture(0)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	component, err := commitSource(draft, index)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	view := component.View().Index()
	term := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	var root, body, frontierBody keyspace.Term
	var offset, cursor, frontierCursor int
	var rootOK, positionOK, frontierOK bool
	allocs := testing.AllocsPerRun(1000, func() {
		root, rootOK = view.Root(term)
		body, offset, cursor, positionOK = view.Position(term)
		frontierBody, frontierCursor, frontierOK = view.Frontier(term)
	})
	if !rootOK || !positionOK || !frontierOK || root != keyspace.MakeTerm(keyspace.FamilyReturn, 1) || body == 0 || frontierBody == 0 || offset != 0 || cursor != 0 || frontierCursor != 0 {
		t.Fatalf("index query sink root=%v/%v position=%v/%d/%d/%v frontier=%v/%d/%v", root, rootOK, body, offset, cursor, positionOK, frontierBody, frontierCursor, frontierOK)
	}
	if allocs != 0 {
		t.Fatalf("Index queries allocated %v times/run", allocs)
	}
}

func sparsePositionFixture(unusedLoops int) (Input, IndexInput) {
	if unusedLoops < 0 {
		unusedLoops = 0
	}
	const name = "sparse.lua"
	input := Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := 0
		if family == keyspace.FamilyNil || family == keyspace.FamilyBody || family == keyspace.FamilyReturn {
			count = 1
		}
		if family == keyspace.FamilyLoop {
			count = unusedLoops
		}
		spans := make([]Span, count)
		for ordinal := range spans {
			spans[ordinal] = Span{File: name, StartLine: uint32(ordinal + 1), StartCol: 1, EndLine: uint32(ordinal + 1), EndCol: 1}
		}
		input.Families = append(input.Families, FamilySpans{Family: family, Spans: spans})
	}
	input.Nil = []NilLiteral{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1)}}
	input.Bodies = []BodySource{{
		Body:  keyspace.MakeTerm(keyspace.FamilyBody, 1),
		Terms: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyReturn, 1)},
	}}
	term := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	root := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	return input, IndexInput{
		Positions: []Position{{
			Term: term, Root: root, Body: body,
			FrontierBody: body,
		}, {
			Term: root, Root: root, Body: body,
			FrontierBody: body,
		}},
		Bodies: []BodyRoots{{Body: body, Roots: []keyspace.Term{root}}},
		Entry:  body,
	}
}

func sourceFixture(width int) (Input, IndexInput) {
	if width < 1 {
		width = 1
	}
	const name = "fixture.lua"
	input := Input{Name: name}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := 0
		switch family {
		case keyspace.FamilyNil:
			count = width
		case keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyBind, keyspace.FamilyFunction, keyspace.FamilyReturn:
			count = 1
		case keyspace.FamilyBody:
			count = 2
		case keyspace.FamilyCell:
			count = 2
		}
		spans := make([]Span, count)
		for ordinal := range spans {
			spans[ordinal] = Span{File: name, StartLine: uint32(ordinal + 1), StartCol: 1, EndLine: uint32(ordinal + 1), EndCol: 1}
		}
		input.Families = append(input.Families, FamilySpans{Family: family, Spans: spans})
	}
	input.Nil = make([]NilLiteral, width)
	for ordinal := range input.Nil {
		input.Nil[ordinal] = NilLiteral{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1)}
	}
	input.Bool = []BoolLiteral{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Value: true}}
	input.Integer = []IntegerLiteral{{Owner: keyspace.MakeTerm(keyspace.FamilyBody, 2), Value: 42}}
	input.Bodies = []BodySource{
		{Body: keyspace.MakeTerm(keyspace.FamilyBody, 1), Terms: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyBind, 1),
		}},
		{Body: keyspace.MakeTerm(keyspace.FamilyBody, 2), Terms: []keyspace.Term{
			keyspace.MakeTerm(keyspace.FamilyReturn, 1),
		}},
	}
	input.Binds = []BindCells{{Bind: keyspace.MakeTerm(keyspace.FamilyBind, 1), Cells: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 1)}}}
	input.Functions = []FunctionFormals{{Function: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Formals: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyCell, 2)}}}

	index := IndexInput{Entry: keyspace.MakeTerm(keyspace.FamilyBody, 1), Bodies: []BodyRoots{
		{Body: keyspace.MakeTerm(keyspace.FamilyBody, 1), Roots: append([]keyspace.Term(nil), input.Bodies[0].Terms...)},
		{Body: keyspace.MakeTerm(keyspace.FamilyBody, 2), Parent: keyspace.MakeTerm(keyspace.FamilyBody, 1), Roots: append([]keyspace.Term(nil), input.Bodies[1].Terms...)},
	}}
	for bodyOrdinal, body := range input.Bodies {
		for offset, term := range body.Terms {
			frontierBody, frontierCursor := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1)), uint32(offset)
			appendCanonicalFixturePosition(&index, Position{
				Term: term, Root: term, Body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1)),
				Offset: uint32(offset), Cursor: uint32(offset), FrontierBody: frontierBody, FrontierCursor: frontierCursor,
			})
		}
	}
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	returned := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	for ordinal := 1; ordinal <= width; ordinal++ {
		appendCanonicalFixturePosition(&index, Position{
			Term: keyspace.MakeTerm(keyspace.FamilyNil, uint32(ordinal)), Root: bind,
			Body: keyspace.MakeTerm(keyspace.FamilyBody, 1), FrontierBody: keyspace.MakeTerm(keyspace.FamilyBody, 1),
		})
	}
	appendCanonicalFixturePosition(&index, Position{
		Term: keyspace.MakeTerm(keyspace.FamilyBool, 1), Root: bind,
		Body: keyspace.MakeTerm(keyspace.FamilyBody, 1), FrontierBody: keyspace.MakeTerm(keyspace.FamilyBody, 1),
	})
	appendCanonicalFixturePosition(&index, Position{
		Term: keyspace.MakeTerm(keyspace.FamilyInteger, 1), Root: returned,
		Body: keyspace.MakeTerm(keyspace.FamilyBody, 2), FrontierBody: keyspace.MakeTerm(keyspace.FamilyBody, 2),
	})
	appendCanonicalFixturePosition(&index, Position{
		Term: function, Root: bind,
		Body: keyspace.MakeTerm(keyspace.FamilyBody, 1), FrontierBody: keyspace.MakeTerm(keyspace.FamilyBody, 1),
	})
	return input, index
}

func removeSourcePosition(rows []Position, omit keyspace.Term) []Position {
	result := make([]Position, 0, len(rows))
	for _, row := range rows {
		if row.Term != omit {
			result = append(result, row)
		}
	}
	return result
}

func sourcePositionFor(rows []Position, term keyspace.Term) (Position, bool) {
	for _, row := range rows {
		if row.Term == term {
			return row, true
		}
	}
	return Position{}, false
}

// appendFixturePositionTerm constructs a local sparse fixture row. It does
// not prove or model Flow containment; production callers supply this batch
// only after their own Layout proof.
func appendFixturePositionTerm(index *IndexInput, term, root keyspace.Term) bool {
	if index == nil {
		return false
	}
	row, ok := sourcePositionFor(index.Positions, root)
	if !ok {
		return false
	}
	row.Term = term
	appendCanonicalFixturePosition(index, row)
	return true
}

func appendCanonicalFixturePosition(index *IndexInput, row Position) {
	if index == nil {
		return
	}
	family, ordinal := keyspace.TermFamily(row.Term), keyspace.TermOrdinal(row.Term)
	at := len(index.Positions)
	for candidate, existing := range index.Positions {
		existingFamily, existingOrdinal := keyspace.TermFamily(existing.Term), keyspace.TermOrdinal(existing.Term)
		if family < existingFamily || family == existingFamily && ordinal < existingOrdinal {
			at = candidate
			break
		}
	}
	index.Positions = append(index.Positions, Position{})
	copy(index.Positions[at+1:], index.Positions[at:])
	index.Positions[at] = row
}
