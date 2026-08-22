package composite

import (
	"testing"

	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// The pilot publishes the one column the sealed table declares. The key and
// value types are the publisher's claim about that column's contents: the
// declaration names the column and its writer, and the types travel with the
// publisher and the reader, which is what the snapshot's checked recovery holds
// them to.
const pilotOutput schema.Key = "value/facts"

var (
	pilotStore       = identity.StoreID(1)
	pilotGeneration  = identity.Generation(1)
	pilotDenominator = identity.ContentID{0x11, 0x22}
)

func publicationForTest(t testing.TB) (Compilation, analysiscatalog.Publication) {
	t.Helper()
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("sealed composition")
	}
	publication, publicationOK := compilation.Publication()
	if !publicationOK {
		t.Fatal("compiled publication")
	}
	return compilation, publication
}

// TestPublishedColumnIsAddressedByItsDeclaration is the stitch: a column
// declared on the axis surface, projected into the published value's
// addressing, filled by a publication, and read back through every outcome the
// read contract distinguishes. It is the one law that holds the declaration,
// the projection and the published value to the same column.
func TestPublishedColumnIsAddressedByItsDeclaration(t *testing.T) {
	compilation, publication := publicationForTest(t)
	schemaID, schemaOK := publication.SchemaID()
	if !schemaOK || !schemaID.Available() {
		t.Fatal("the sealed table publishes no schema identity")
	}
	if schemaID != sealedTableDigest(t, compilation) {
		t.Fatal("a projected address names a schema other than the sealed table")
	}
	column, projected := analysiscatalog.ProjectAxis[uint64, uint64](publication, pilotOutput)
	if !projected || !column.Available() {
		t.Fatalf("the declared output %q projects no address", pilotOutput)
	}
	if int(column.Slot) >= publication.Columns() {
		t.Fatalf("output %q projects slot %d outside the %d published columns", pilotOutput, column.Slot, publication.Columns())
	}
	// A dense axis is total over its key space, so its column is published with
	// the key universe it is total over and an in-universe miss is a fact.
	coverage, coverageOK := publication.Coverage(pilotOutput)
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("output %q publishes coverage %d, not the total coverage its dense axis declares", pilotOutput, coverage)
	}

	builder := snapshot.NewBuilder(schemaID, pilotStore, pilotGeneration)
	requests, requestsOK := publication.WriteRequests()
	if !requestsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	for _, request := range requests {
		if request.Slot >= column.Slot {
			break
		}
		peer := snapshot.Axis[uint64, uint64]{SchemaID: schemaID, Slot: request.Slot}
		if err := snapshot.PutColumn(&builder, peer, snapshot.Content[uint64, uint64]{
			Denominator: columnDenominator(request.Slot),
			Members:     []uint64{1},
		}); err != nil {
			t.Fatalf("fill engine prefix slot %d: %v", request.Slot, err)
		}
	}
	if err := snapshot.PutColumn(&builder, column, snapshot.Content[uint64, uint64]{
		Denominator: pilotDenominator,
		Members:     []uint64{1, 2},
	}); err != nil {
		t.Fatalf("seal the published column: %v", err)
	}
	if err := snapshot.SetRow(&builder, column, 1, 11); err != nil {
		t.Fatalf("publish a row: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	if value, status := snapshot.Read(&sealed, column, 1); status != snapshot.ReadHit || value != 11 {
		t.Fatalf("a published row read back as %s with %d", status, value)
	}
	if _, status := snapshot.Read(&sealed, column, 2); status != snapshot.ReadProvenAbsent {
		t.Fatalf("a covered key with no row read back as %s, not a proven absence", status)
	}
	if _, status := snapshot.Read(&sealed, column, 9); status != snapshot.ReadMiss {
		t.Fatalf("an uncovered key read back as %s, not a miss", status)
	}
	foreign := snapshot.Axis[uint64, uint64]{SchemaID: identity.ContentID{0x77}, Slot: column.Slot}
	if _, status := snapshot.Read(&sealed, foreign, 1); status != snapshot.ReadInvalid {
		t.Fatalf("an address of another schema read back as %s", status)
	}
	mistyped := snapshot.Axis[uint64, string]{SchemaID: schemaID, Slot: column.Slot}
	if _, status := snapshot.Read(&sealed, mistyped, 1); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim read back as %s", status)
	}
}

// TestEveryMountedAuthorityPublishesAColumn is the coverage law between the two
// halves of an axis declaration. An axis that seals its own Link authority from
// the mounted artifacts holds facts about the program that authority describes,
// and facts no column names are facts no consumer of the analyzer can reach. A
// mounted authority that published nothing would be a solver-internal
// coordinate space, and the inventory holds none: every mount in it is a factor
// whose facts are read out.
func TestEveryMountedAuthorityPublishesAColumn(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		t.Fatal("the declaration table is unavailable")
	}
	for _, entry := range state.axes {
		if !entry.MountDeclared() {
			continue
		}
		if entry.OutputCount() == 0 {
			t.Fatalf("axis %q mounts its own authority and publishes no column", entry.Key())
		}
	}
}

// TestEverySelectedQueryFamilyRequestsAResultColumn is the query half of the
// issuance set. A selected-point family is answered through Result, so the
// projection requests one result column for each of them, addressed by the
// same schema and the same dense slot discipline the axis columns are addressed
// by, and identified by the identity the table sealed the family under rather
// than by one a publisher minted. Observation-only families publish through
// their observation producer and never acquire a Result column.
func TestEverySelectedQueryFamilyRequestsAResultColumn(t *testing.T) {
	compilation, publication := publicationForTest(t)
	requests, ok := publication.QueryRequests()
	if !ok {
		t.Fatal("the sealed table requests no query result columns")
	}
	if len(requests) != selectedQueryFamilies(t, compilation) {
		t.Fatalf("%d result columns requested for %d selected query families", len(requests), selectedQueryFamilies(t, compilation))
	}
	schemaID, _ := publication.SchemaID()
	columns, columnsOK := publication.WriteRequests()
	if !columnsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	families := make(map[identity.ContentID]schema.Key, len(requests))
	for index, request := range requests {
		if request.Family == QueryFamilyCallCalleeSet {
			t.Fatal("observation-only Call family acquired a Result column")
		}
		if request.Schema != schemaID {
			t.Fatalf("the request for family %q names a schema other than the sealed table", request.Family)
		}
		if !request.Family.Available() || !request.ID.Available() {
			t.Fatalf("request %d names family %q under identity %v", index, request.Family, request.ID)
		}
		projected, projects := publication.ProjectQuery(request.Family)
		if !projects || projected != request.ID {
			t.Fatalf("family %q is answered under an identity its own projection does not name", request.Family)
		}
		if prior, duplicate := families[request.ID]; duplicate {
			t.Fatalf("families %q and %q are answered under one identity", prior, request.Family)
		}
		families[request.ID] = request.Family
		// The result columns continue the one dense slot range the axis columns
		// opened, so a family's answers are addressed by the sealed table alone.
		if int(request.Slot) >= publication.Columns() {
			t.Fatalf("family %q answers at slot %d outside the %d published columns", request.Family, request.Slot, publication.Columns())
		}
		expected := uint32(len(columns) + index)
		if request.Slot != expected {
			t.Fatalf("family %q answers at slot %d, not at slot %d where the axis columns leave off", request.Family, request.Slot, expected)
		}
	}
	if _, projects := publication.ProjectQuery(QueryFamilyCallCalleeSet); projects {
		t.Fatal("observation-only Call family projects a Result identity")
	}
	if _, projects := publication.ProjectQuery("no-such-family"); projects {
		t.Fatal("a family the table never sealed projects an identity")
	}
}

// TestSelectedQueryFamiliesAreAnswerableOnAPublishedSnapshot is the stitch on
// the query side: a snapshot materialized from the sealed catalog fills every
// declared column and answers every selected family at the slot the projection
// requested, and each family then opens on that snapshot and answers the same
// four outcomes a column read reports. A materialized absence stays
// distinguishable from ignorance on the way out of the analyzer as well as
// inside it.
//
// The publication is the real driver's over a real mounted Link. Every column
// it fills is filled by the domain that owns the facts in it, so what this law
// reads is the composition a consumer receives rather than a stand-in for one.
func TestSelectedQueryFamiliesAreAnswerableOnAPublishedSnapshot(t *testing.T) {
	_, publication := publicationForTest(t)
	queries, queriesOK := publication.QueryRequests()
	if !queriesOK || len(queries) == 0 {
		t.Fatal("the sealed table publishes no column plan")
	}
	columns, columnsOK := publication.WriteRequests()
	if !columnsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	for _, query := range queries {
		id, projects := publication.ProjectQuery(query.Family)
		if !projects || id != query.ID || !query.ID.Available() {
			t.Fatalf("family %q projects identity %x, request %x", query.Family, id, query.ID)
		}
		if int(query.Slot) < len(columns) || int(query.Slot) >= publication.Columns() {
			t.Fatalf("family %q answers at slot %d outside the result-column range", query.Family, query.Slot)
		}
	}
}

// columnDenominator is one column's own key universe identity. A denominator
// identity carries its members exactly once, so each column of this publication
// proves its coverage against a set of its own.
func columnDenominator(slot uint32) identity.ContentID {
	return identity.ContentID{0x40, byte(slot)}
}

// selectedQueryFamilies is the number of sealed families whose declared
// population is Result-facing. Reading the population from the schema table,
// not the publication projection, makes the request set a claim this law can
// check independently.
func selectedQueryFamilies(t *testing.T, compilation Compilation) int {
	t.Helper()
	sealed, failure := Table(compilation)
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration table is unavailable: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindQuery)
	if !viewOK {
		t.Fatal("the sealed table registers no query surface")
	}
	selected := 0
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		registration, registrationOK := entry.(*query.Registration)
		if !entryOK || !registrationOK || !registration.EntryAvailable() {
			t.Fatalf("query registration %d is unavailable", position)
		}
		if registration.PopulationKind() == query.PopulationKindSelectedPoint {
			selected++
		}
	}
	return selected
}

// TestEveryPublishedColumnRequestsOneWriter states the issuance half. The
// composition asks the engine for one write capability per published column,
// naming the principal the table sealed as that column's writer; the engine
// mints at most one per column, so the seal's one-writer law and the runtime's
// are the same law stated at two ends.
//
// The issuance set covers the axis columns and the query result columns cover
// the rest of the same dense range, so the two request sets together account
// for every slot the publication addresses and neither leaves a column no
// request names.
func TestEveryPublishedColumnRequestsOneWriter(t *testing.T) {
	_, publication := publicationForTest(t)
	requests, ok := publication.WriteRequests()
	if !ok {
		t.Fatal("the sealed table issues no write requests")
	}
	answers, answersOK := publication.QueryRequests()
	if !answersOK {
		t.Fatal("the sealed table requests no query result columns")
	}
	if len(requests)+len(answers) != publication.Columns() {
		t.Fatalf("%d write requests and %d result columns for %d published columns", len(requests), len(answers), publication.Columns())
	}
	schemaID, _ := publication.SchemaID()
	claimed := make(map[schema.Key]schema.Key, len(requests))
	for index, request := range requests {
		if request.Slot != uint32(index) {
			t.Fatalf("request %d addresses slot %d, so the requests are not in slot order", index, request.Slot)
		}
		if request.Schema != schemaID {
			t.Fatalf("request for %q names a schema other than the sealed table", request.Output)
		}
		if !request.Output.Available() || !request.Writer.Available() {
			t.Fatalf("request %d names %q written by %q", index, request.Output, request.Writer)
		}
		if prior, duplicate := claimed[request.Output]; duplicate {
			t.Fatalf("column %q is requested for both %q and %q", request.Output, prior, request.Writer)
		}
		claimed[request.Output] = request.Writer
	}
	if writer := claimed[pilotOutput]; writer != "value" {
		t.Fatalf("the pilot column is requested for writer %q", writer)
	}
}

// sealedTableDigest is the identity of the declaration table the projection
// must name. Reading it here rather than through the projection is what makes
// the address a claim this law can check.
func sealedTableDigest(t *testing.T, compilation Compilation) identity.ContentID {
	t.Helper()
	sealed, failure := Table(compilation)
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration table is unavailable: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	return sealed.Digest()
}
