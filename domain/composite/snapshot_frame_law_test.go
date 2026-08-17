package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
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

// TestPublishedColumnIsAddressedByItsDeclaration is the stitch: a column
// declared on the axis surface, projected into the published value's
// addressing, filled by a publication, and read back through every outcome the
// read contract distinguishes. It is the one law that holds the declaration,
// the projection and the published value to the same column.
func TestPublishedColumnIsAddressedByItsDeclaration(t *testing.T) {
	schemaID, schemaOK := PublicationSchema()
	if !schemaOK || !schemaID.Available() {
		t.Fatal("the sealed table publishes no schema identity")
	}
	if schemaID != sealedTableDigest(t) {
		t.Fatal("a projected address names a schema other than the sealed table")
	}
	column, projected := ProjectAxis[uint64, uint64](pilotOutput)
	if !projected || !column.Available() {
		t.Fatalf("the declared output %q projects no address", pilotOutput)
	}
	if int(column.Slot) >= PublicationColumns() {
		t.Fatalf("output %q projects slot %d outside the %d published columns", pilotOutput, column.Slot, PublicationColumns())
	}
	// A dense axis is total over its key space, so its column is published with
	// the key universe it is total over and an in-universe miss is a fact.
	coverage, coverageOK := PublicationCoverage(pilotOutput)
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("output %q publishes coverage %d, not the total coverage its dense axis declares", pilotOutput, coverage)
	}

	builder := snapshot.NewBuilder(schemaID, pilotStore, pilotGeneration)
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
	sealRegistry()
	if registry.sealed == nil {
		t.Fatal("the declaration table is unavailable")
	}
	for _, entry := range registry.axes {
		if !entry.MountDeclared() {
			continue
		}
		if entry.OutputCount() == 0 {
			t.Fatalf("axis %q mounts its own authority and publishes no column", entry.Key())
		}
	}
}

// TestEverySealedQueryFamilyRequestsAResultColumn is the query half of the
// issuance set. A family the table seals is a family the analyzer answers, so
// the projection requests one result column for each of them, addressed by the
// same schema and the same dense slot discipline the axis columns are addressed
// by, and identified by the identity the table sealed the family under rather
// than by one a publisher minted.
func TestEverySealedQueryFamilyRequestsAResultColumn(t *testing.T) {
	requests, ok := QueryRequests()
	if !ok {
		t.Fatal("the sealed table requests no query result columns")
	}
	if len(requests) != sealedQueryFamilies(t) {
		t.Fatalf("%d result columns requested for %d sealed query families", len(requests), sealedQueryFamilies(t))
	}
	schemaID, _ := PublicationSchema()
	columns, columnsOK := WriteRequests()
	if !columnsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	families := make(map[identity.ContentID]schema.Key, len(requests))
	for index, request := range requests {
		if request.Schema != schemaID {
			t.Fatalf("the request for family %q names a schema other than the sealed table", request.Family)
		}
		if !request.Family.Available() || !request.ID.Available() {
			t.Fatalf("request %d names family %q under identity %v", index, request.Family, request.ID)
		}
		projected, projects := ProjectQuery(request.Family)
		if !projects || projected != request.ID {
			t.Fatalf("family %q is answered under an identity its own projection does not name", request.Family)
		}
		if prior, duplicate := families[request.ID]; duplicate {
			t.Fatalf("families %q and %q are answered under one identity", prior, request.Family)
		}
		families[request.ID] = request.Family
		// The result columns continue the one dense slot range the axis columns
		// opened, so a family's answers are addressed by the sealed table alone.
		if int(request.Slot) >= PublicationColumns() {
			t.Fatalf("family %q answers at slot %d outside the %d published columns", request.Family, request.Slot, PublicationColumns())
		}
		expected := uint32(len(columns) + index)
		if request.Slot != expected {
			t.Fatalf("family %q answers at slot %d, not at slot %d where the axis columns leave off", request.Family, request.Slot, expected)
		}
	}
	if _, projects := ProjectQuery("no-such-family"); projects {
		t.Fatal("a family the table never sealed projects an identity")
	}
}

// TestSealedQueryFamiliesAreAnswerableOnAPublishedSnapshot is the stitch on the
// query side: a snapshot built from the sealed catalog fills every declared
// column and materializes every sealed family's answers at the slot the
// projection requested, and each family then opens on that snapshot and answers
// the same four outcomes a column read reports. A materialized absence stays
// distinguishable from ignorance on the way out of the analyzer as well as
// inside it.
func TestSealedQueryFamiliesAreAnswerableOnAPublishedSnapshot(t *testing.T) {
	schemaID, schemaOK := PublicationSchema()
	if !schemaOK {
		t.Fatal("the sealed table publishes no schema identity")
	}
	columns, columnsOK := WriteRequests()
	queries, queriesOK := QueryRequests()
	if !columnsOK || !queriesOK || len(queries) == 0 {
		t.Fatal("the sealed table publishes no column plan")
	}

	builder := snapshot.NewBuilder(schemaID, pilotStore, pilotGeneration)
	// The publication is dense: a family's result column is addressed above
	// every axis column, so every one of them is filled before it.
	for _, column := range columns {
		if err := snapshot.PutColumn(&builder, snapshot.Axis[uint64, uint64]{SchemaID: schemaID, Slot: column.Slot}, snapshot.Content[uint64, uint64]{
			Rows:        map[uint64]uint64{1: 11},
			Denominator: columnDenominator(column.Slot),
			Members:     []uint64{1, 2},
		}); err != nil {
			t.Fatalf("seal the published column %q: %v", column.Output, err)
		}
	}
	plans := make([]snapshot.QueryPlan[uint64, uint64], 0, len(queries))
	for _, query := range queries {
		plan, err := snapshot.DeclareQuery(&builder, query.ID, query.Slot, snapshot.Content[uint64, uint64]{
			Rows:        map[uint64]uint64{4: 44},
			Denominator: columnDenominator(query.Slot),
			Members:     []uint64{4, 5},
		})
		if err != nil {
			t.Fatalf("materialize the query family %q: %v", query.Family, err)
		}
		plans = append(plans, plan)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	for index, query := range queries {
		opened, opens := snapshot.OpenQuery[uint64, uint64](&sealed, query.ID)
		if !opens || opened != plans[index] {
			t.Fatalf("family %q opens a plan other than the one it declared", query.Family)
		}
		if answer, status := snapshot.Query(&sealed, opened, 4); status != snapshot.ReadHit || answer != 44 {
			t.Fatalf("a materialized answer of %q read back as %s with %d", query.Family, status, answer)
		}
		if _, status := snapshot.Query(&sealed, opened, 5); status != snapshot.ReadProvenAbsent {
			t.Fatalf("a covered key with no answer of %q read back as %s, not a proven absence", query.Family, status)
		}
		if _, status := snapshot.Query(&sealed, opened, 9); status != snapshot.ReadMiss {
			t.Fatalf("an uncovered key of %q read back as %s, not a miss", query.Family, status)
		}
		if _, opens := snapshot.OpenQuery[uint64, string](&sealed, query.ID); opens {
			t.Fatalf("a wrong answer claim opened family %q", query.Family)
		}
		foreign := snapshot.QueryPlan[uint64, uint64]{SchemaID: identity.ContentID{0x77}, Slot: opened.Slot}
		if _, status := snapshot.Query(&sealed, foreign, 4); status != snapshot.ReadInvalid {
			t.Fatalf("a plan of another schema answered %q with %s", query.Family, status)
		}
	}
}

// columnDenominator is one column's own key universe identity. A denominator
// identity carries its members exactly once, so each column of this publication
// proves its coverage against a set of its own.
func columnDenominator(slot uint32) identity.ContentID {
	return identity.ContentID{0x40, byte(slot)}
}

// sealedQueryFamilies is the number of families the declaration table sealed.
// Reading it from the table rather than through the projection is what makes
// the request set a claim this law can check.
func sealedQueryFamilies(t *testing.T) int {
	t.Helper()
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration table is unavailable: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindQuery)
	if !viewOK {
		t.Fatal("the sealed table registers no query surface")
	}
	return view.Count()
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
	requests, ok := WriteRequests()
	if !ok {
		t.Fatal("the sealed table issues no write requests")
	}
	answers, answersOK := QueryRequests()
	if !answersOK {
		t.Fatal("the sealed table requests no query result columns")
	}
	if len(requests)+len(answers) != PublicationColumns() {
		t.Fatalf("%d write requests and %d result columns for %d published columns", len(requests), len(answers), PublicationColumns())
	}
	schemaID, _ := PublicationSchema()
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
func sealedTableDigest(t *testing.T) identity.ContentID {
	t.Helper()
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration table is unavailable: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	return sealed.Digest()
}
