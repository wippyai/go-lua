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
	pilotStore        = identity.StoreID(1)
	pilotGeneration   = identity.Generation(1)
	pilotDenominator  = identity.ContentID{0x11, 0x22}
	pilotQueryFamily  = identity.ContentID{0x33, 0x44}
	pilotQueryMembers = identity.ContentID{0x55, 0x66}
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

// TestPublishedQueryAnswersTheSameFourOutcomes states that a query family
// materialized against the published plan reports the same outcomes a column
// read reports, so a materialized absence stays distinguishable from ignorance
// on the way out of the analyzer as well as inside it.
func TestPublishedQueryAnswersTheSameFourOutcomes(t *testing.T) {
	schemaID, schemaOK := PublicationSchema()
	if !schemaOK {
		t.Fatal("the sealed table publishes no schema identity")
	}
	column, projected := ProjectAxis[uint64, uint64](pilotOutput)
	if !projected {
		t.Fatalf("the declared output %q projects no address", pilotOutput)
	}
	builder := snapshot.NewBuilder(schemaID, pilotStore, pilotGeneration)
	if err := snapshot.PutColumn(&builder, column, snapshot.Content[uint64, uint64]{
		Rows:        map[uint64]uint64{1: 11},
		Denominator: pilotDenominator,
		Members:     []uint64{1, 2},
	}); err != nil {
		t.Fatalf("seal the published column: %v", err)
	}
	// A query family is answered by a result column of its own, addressed at
	// the next dense slot the publication reaches.
	plan, err := snapshot.DeclareQuery(&builder, pilotQueryFamily, uint32(PublicationColumns()), snapshot.Content[uint64, uint64]{
		Rows:        map[uint64]uint64{4: 44},
		Denominator: pilotQueryMembers,
		Members:     []uint64{4, 5},
	})
	if err != nil {
		t.Fatalf("declare the query family: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	opened, opens := snapshot.OpenQuery[uint64, uint64](&sealed, pilotQueryFamily)
	if !opens || opened != plan {
		t.Fatal("the published family opens a plan other than the one it declared")
	}
	if answer, status := snapshot.Query(&sealed, opened, 4); status != snapshot.ReadHit || answer != 44 {
		t.Fatalf("a materialized answer read back as %s with %d", status, answer)
	}
	if _, status := snapshot.Query(&sealed, opened, 5); status != snapshot.ReadProvenAbsent {
		t.Fatalf("a covered key with no answer read back as %s, not a proven absence", status)
	}
	if _, status := snapshot.Query(&sealed, opened, 9); status != snapshot.ReadMiss {
		t.Fatalf("an uncovered key read back as %s, not a miss", status)
	}
	if _, opens := snapshot.OpenQuery[uint64, string](&sealed, pilotQueryFamily); opens {
		t.Fatal("a wrong answer claim opened the published family")
	}
	unregistered := snapshot.QueryPlan[uint64, uint64]{SchemaID: identity.ContentID{0x77}, Slot: plan.Slot}
	if _, status := snapshot.Query(&sealed, unregistered, 4); status != snapshot.ReadInvalid {
		t.Fatalf("a plan of another schema answered %s", status)
	}
}

// TestEveryPublishedColumnRequestsOneWriter states the issuance half. The
// composition asks the engine for one write capability per published column,
// naming the principal the table sealed as that column's writer; the engine
// mints at most one per column, so the seal's one-writer law and the runtime's
// are the same law stated at two ends.
func TestEveryPublishedColumnRequestsOneWriter(t *testing.T) {
	requests, ok := WriteRequests()
	if !ok {
		t.Fatal("the sealed table issues no write requests")
	}
	if len(requests) != PublicationColumns() {
		t.Fatalf("%d write requests for %d published columns", len(requests), PublicationColumns())
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
