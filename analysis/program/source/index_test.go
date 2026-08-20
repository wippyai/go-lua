package source

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

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
	if term, owner, value, ok := view.Literals().Integers().At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyInteger, 1) ||
		owner != keyspace.MakeTerm(keyspace.FamilyBody, 2) || value != 42 {
		t.Fatalf("Integer = %v, %v, %d, %v", term, owner, value, ok)
	}
	if got := view.Identity().ContentID(); !got.Available() {
		t.Fatal("unavailable authored content identity")
	}
}

func TestSourceAllowsTypedChildBodyWithoutDirectSourceOccurrence(t *testing.T) {
	input, index := sourceFixture(2)
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
			slots += component.authority.index.Count(family)
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
		positions := component.authority.index.Count(family)
		if positions == 0 {
			continue
		}
		retained += positions
	}
	if retained != len(index.Positions) {
		t.Fatalf("retained positions = %d, want %d", retained, len(index.Positions))
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
