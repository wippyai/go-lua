package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// The directory is a sealed function from a published content identity to one
// locator. These laws exercise it through the current ConstructProgram result;
// no disposable construction workspace is retained by the test.
func TestSemanticDirectoryResolvesAtMostOneLocatorPerContentID(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	directory := fixture.graph.directory
	if directory == nil || len(directory.entries) == 0 {
		t.Fatal("sealed program published no semantic directory")
	}
	for id, entry := range directory.entries {
		roles := 0
		if _, ok := directory.point(id); ok {
			roles++
		}
		if _, ok := directory.member(id); ok {
			roles++
		}
		if _, ok := directory.query(id); ok {
			roles++
		}
		if _, ok := directory.activation(id); ok {
			roles++
		}
		if roles != 1 || entry.slot >= uint32(len(directory.entries)) {
			t.Fatalf("directory identity %x resolves through %d role planes", id[:4], roles)
		}
	}
	if _, ok := directory.resolve(identity.ContentID{}); ok {
		t.Fatal("unavailable identity resolved")
	}
	if _, ok := directory.resolve(programMatrixID(250)); ok {
		t.Fatal("unknown identity resolved")
	}
	for _, id := range fixture.observationIDs {
		if _, ok := directory.resolve(id); ok {
			t.Fatalf("observation identity entered the construction directory: %x", id[:4])
		}
	}
}

// TestSemanticDirectoryLocatorsRemainOwnedByOneTopologyRevision maps the
// exact/revision/foreign directory law onto CommittedProgram.Seal: every
// locator resolves against both fresh runtime seals of its owner topology and
// against neither an equal-shaped foreign program nor a wrong role plane.
func TestSemanticDirectoryLocatorsRemainOwnedByOneTopologyRevision(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	directory := fixture.graph.directory
	queryID := programMatrixID(110)
	queryLocator, queryOK := directory.query(queryID)
	if !queryOK {
		t.Fatal("query directory locator")
	}
	var pointID, memberID identity.ContentID
	for id, entry := range directory.entries {
		switch entry.kind {
		case bindingSemanticPoint:
			pointID = id
		case bindingSemanticMember:
			memberID = id
		}
	}
	pointLocator, pointOK := directory.point(pointID)
	memberLocator, memberOK := directory.member(memberID)
	if !pointOK || !memberOK || !pointID.Available() || !memberID.Available() {
		t.Fatal("point/member directory locators")
	}
	first, firstFailure, firstOK := fixture.graph.Seal(nil)
	second, secondFailure, secondOK := fixture.graph.Seal(nil)
	if !firstOK || !secondOK || first == nil || second == nil || firstFailure.Available() || secondFailure.Available() {
		t.Fatalf("fresh seals = %v/%v/%v and %v/%v/%v", first, firstFailure, firstOK, second, secondFailure, secondOK)
	}
	if _, ok := pointLocator.Resolve(first.runtime.graph); !ok {
		t.Fatal("point locator crossed its owner revision")
	}
	if _, ok := memberLocator.Resolve(second.runtime.graph); !ok {
		t.Fatal("member locator crossed its owner revision")
	}
	if _, ok := queryLocator.Resolve(first.runtime.graph); !ok {
		t.Fatal("query locator crossed its owner revision")
	}
	foreign := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	if _, ok := pointLocator.Resolve(foreign.graph.graph); ok {
		t.Fatal("point locator resolved against a foreign graph")
	}
	if _, ok := memberLocator.Resolve(foreign.graph.graph); ok {
		t.Fatal("member locator resolved against a foreign graph")
	}
	if _, ok := queryLocator.Resolve(foreign.graph.graph); ok {
		t.Fatal("query locator resolved against a foreign graph")
	}
	if _, ok := directory.point(memberID); ok {
		t.Fatal("member identity entered the point directory")
	}
	if _, ok := directory.member(pointID); ok {
		t.Fatal("point identity entered the member directory")
	}
}

// TestSemanticDirectoryQuerySurfaceIsDetached keeps the query-row clone law
// on the committed Query value: a caller's surface slice edit cannot mutate
// the directory's parent graph or a later revision lookup.
func TestSemanticDirectoryQuerySurfaceIsDetached(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	locator, located := fixture.graph.directory.query(programMatrixID(110))
	query, resolved := locator.Resolve(fixture.graph.graph)
	if !located || !resolved {
		t.Fatal("query directory row")
	}
	surfaces := query.Surfaces()
	if len(surfaces) != 1 {
		t.Fatal("query surface cardinality")
	}
	// The committed coordinate is the one the query's point wrote, which the
	// matrix rule declares at Local 2. What this law holds is that an edit of
	// the caller's copy never becomes the committed row's.
	committed := surfaces[0].Local
	surfaces[0].Local = 99
	again, resolved := locator.Resolve(fixture.graph.graph)
	if !resolved || len(again.Surfaces()) != 1 || again.Surfaces()[0].Local != committed || committed != 2 {
		t.Fatal("query surface edit crossed the committed row")
	}
}

// TestSemanticDirectoryRejectsDuplicateAndZeroRows keeps the directory's
// terminal identity fence on the current sealed rows. The old assembly test
// supplied duplicate/zero identities; this law feeds the same malformed
// identity classes to the sole directory sealer.
func TestSemanticDirectoryRejectsDuplicateAndZeroRows(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	var pointID, memberID identity.ContentID
	rows := &bindingSemanticRows{
		points:      make(map[identity.ContentID]equation.PointRef),
		members:     make(map[identity.ContentID]equation.RuleRef),
		queries:     make(map[identity.ContentID]uint64),
		activations: make(map[identity.ContentID]equation.RuleRef),
	}
	for id, entry := range fixture.graph.directory.entries {
		switch entry.kind {
		case bindingSemanticPoint:
			pointID = id
			rows.points[id] = equation.PointAt(int(entry.slot) + 1)
		case bindingSemanticMember:
			memberID = id
			rows.members[id] = equation.RuleAt(int(entry.slot) + 1)
		case bindingSemanticQuery:
			rows.queries[id] = uint64(entry.slot)
		}
	}
	if pointID == (identity.ContentID{}) || memberID == (identity.ContentID{}) {
		t.Fatal("directory source rows")
	}
	duplicate := *rows
	duplicate.members = make(map[identity.ContentID]equation.RuleRef, len(rows.members)+1)
	for id, ref := range rows.members {
		duplicate.members[id] = ref
	}
	duplicate.members[pointID] = equation.RuleAt(1)
	if directory, ok := sealSemanticDirectory(fixture.graph.topology, fixture.graph.state, fixture.graph.authority, &duplicate); ok || directory != nil {
		t.Fatal("duplicate structural identity entered the directory")
	}
	zero := *rows
	zero.points = make(map[identity.ContentID]equation.PointRef, len(rows.points)+1)
	for id, ref := range rows.points {
		zero.points[id] = ref
	}
	zero.points[identity.ContentID{}] = equation.PointAt(1)
	if directory, ok := sealSemanticDirectory(fixture.graph.topology, fixture.graph.state, fixture.graph.authority, &zero); ok || directory != nil {
		t.Fatal("zero structural identity entered the directory")
	}
}

// TestSemanticDirectoryActivationLocatorIsExactAndForeignSafe maps the
// activation-directory law to the selected overlay's sealed trigger row.
func TestSemanticDirectoryActivationLocatorIsExactAndForeignSafe(t *testing.T) {
	fixture := newSelectedOverlayLawFixtureWithCandidates(t, 2, false)
	locator, located := fixture.graph.directory.activation(fixture.activationID)
	if !located {
		t.Fatal("activation directory locator")
	}
	if _, ok := locator.Resolve(fixture.graph.graph); !ok {
		t.Fatal("activation locator did not resolve its committed trigger")
	}
	foreign := newSelectedOverlayLawFixture(t)
	if _, ok := locator.Resolve(foreign.graph.graph); ok {
		t.Fatal("activation locator resolved against a foreign graph")
	}
}

// TestSemanticDirectoryActivationRejectsConflictingApplications keeps the
// old mixed-application terminal law at the current admission boundary: one
// trigger identity cannot be published twice under different applications.
func TestSemanticDirectoryActivationRejectsConflictingApplications(t *testing.T) {
	conflict := newSelectedOverlayLawFixtureWithCandidates(t, 2, true)
	if conflict.graph != nil {
		t.Fatal("conflicting activation applications published a trigger directory")
	}
}

func TestSemanticDirectoryEntriesAreBoundedByPublishedRows(t *testing.T) {
	for _, width := range []int{1, 4, 16, 32} {
		fixture := newReceiptQueryMatrixFixture(t, width, nil, nil)
		graph := fixture.graph.graph
		roots := graph.PointCount() + graph.GroupCount() + graph.QueryCount()
		if roots == 0 || len(fixture.graph.directory.entries) > roots*2 {
			t.Fatalf("width %d directory entries=%d published roots=%d", width, len(fixture.graph.directory.entries), roots)
		}
		for index := 0; index < width; index++ {
			if _, ok := fixture.graph.Query(programMatrixID(110 + index)); !ok {
				t.Fatalf("width %d query %d lost its directory locator", width, index)
			}
		}
	}
}

func TestSemanticDirectoryIsStableAcrossCommittedProgramSeals(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 8, nil, nil)
	directory := fixture.graph.directory
	entries := len(directory.entries)
	for index := 0; index < 8; index++ {
		if _, ok := fixture.graph.Query(programMatrixID(110 + index)); !ok {
			t.Fatal("baseline query locator")
		}
	}
	for attempt := 0; attempt < 16; attempt++ {
		solver, failure, ok := fixture.graph.Seal(nil)
		if !ok || solver == nil || failure.Available() {
			t.Fatalf("seal %d failed: %v", attempt, failure)
		}
		if fixture.graph.directory != directory || len(directory.entries) != entries {
			t.Fatal("sealing a committed program grew or replaced its directory")
		}
	}
}
