package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/snapshot"
	executionowner "github.com/wippyai/go-lua/domain/execution/owner"
)

// The execution-reachability axis is the analyzer's first engine-published
// coordinate space: the demand pass derives which mounted execution points a
// Link reaches and publishes them itself, so the declaration states the space,
// the writer principal and the published column and carries no factor binding
// at all. The laws below hold that declaration to the same three ends every
// other axis is held to - it seals, it addresses one column, and the column
// answers the read contract - with the one difference the storage declares.

// mountedExecutionPoint is the coordinate the reachability column is keyed by,
// spelled at the read site: the mount a point was reached in, and the point
// itself. The mount qualifies the point because a program mounted twice is two
// mounts, and reaching a point in one of them says nothing about the other.
//
// The claim is what the published value's checked recovery holds a reader to,
// so the pair travels with the publisher and the reader rather than with the
// declaration, and the law below is about the column rather than about the
// spelling of its key.
type mountedExecutionPoint struct {
	Mount identity.MountID
	Point identity.ContentID
}

var (
	reachabilityDenominator = identity.ContentID{0x9a, 0xbc}
	reachabilityMount       = identity.MountID{0x01}
	reachedPoint            = mountedExecutionPoint{Mount: reachabilityMount, Point: identity.ContentID{0x02}}
	unreachedPoint          = mountedExecutionPoint{Mount: reachabilityMount, Point: identity.ContentID{0x03}}
	unmountedPoint          = mountedExecutionPoint{Mount: identity.MountID{0x04}, Point: identity.ContentID{0x05}}
)

// TestExecutionReachabilityIsDeclaredAsAnEnginePublishedAxis states the
// declaration itself. The pass that fills this column is not a factor lane, so
// the axis declares no cold fragment, no factor binding, and no algebra of one,
// and the composition's inventory carries it beside the factor axes rather than
// as one of them.
func TestExecutionReachabilityIsDeclaredAsAnEnginePublishedAxis(t *testing.T) {
	entry, declared := axisForKey(axisKeyExecutionReachability)
	if !declared {
		t.Fatalf("the composition declares no axis %q", axisKeyExecutionReachability)
	}
	if entry.Key() != executionowner.AxisKey {
		t.Fatalf("the registered axis is keyed %q, its owner declares %q", entry.Key(), executionowner.AxisKey)
	}
	storage, storageOK := AxisStorage(axisKeyExecutionReachability)
	if !storageOK || storage != axis.StorageEngine {
		t.Fatalf("axis %q declares storage %d, not the engine-published one", axisKeyExecutionReachability, storage)
	}
	if storage.Bound() {
		t.Fatal("the engine-published axis reports a bound storage")
	}
	if entry.MountDeclared() {
		t.Fatal("the engine-published axis seals a Link authority of its own")
	}
	// A cold half it does not declare is a cold half it cannot be asked for: a
	// pass walks the bound axes, which is what the declared storage tells it.
	if entry.Storage().Bound() {
		t.Fatal("the engine-published axis recorded a cold shape")
	}
	roles, rolesOK := SemanticRoles()
	if !rolesOK {
		t.Fatal("semantic role vocabulary")
	}
	expected, expectedOK := roles.Key("semantic/axis/execution-reachability")
	semantic, semanticOK := AxisSemantic(axisKeyExecutionReachability)
	if !expectedOK || !semanticOK || semantic != expected {
		t.Fatalf("axis %q publishes %x, the vocabulary declares the axis role", axisKeyExecutionReachability, semantic.Digest())
	}
}

// TestExecutionReachabilityPublishesOneEngineWrittenColumn states the published
// half. The axis publishes exactly one column, and the principal admitted to
// write it is the axis itself: an axis is a writer principal, so the pass that
// fills the column is admitted as the axis it is declared as, and the write
// request the composition issues names that same pair.
func TestExecutionReachabilityPublishesOneEngineWrittenColumn(t *testing.T) {
	entry, declared := axisForKey(axisKeyExecutionReachability)
	if !declared {
		t.Fatalf("the composition declares no axis %q", axisKeyExecutionReachability)
	}
	if entry.OutputCount() != 1 {
		t.Fatalf("axis %q publishes %d columns, not the one it declares", axisKeyExecutionReachability, entry.OutputCount())
	}
	output, outputOK := entry.OutputAt(0)
	if !outputOK || output.Key != executionowner.OutputKey || output.Writer != executionowner.AxisKey {
		t.Fatalf("axis %q publishes column %q written by %q", axisKeyExecutionReachability, output.Key, output.Writer)
	}
	if _, resolves := axisForKey(output.Writer); !resolves {
		t.Fatalf("column %q is written by %q, which no declared axis is", output.Key, output.Writer)
	}
	requests, requestsOK := WriteRequests()
	if !requestsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	issued := 0
	for _, request := range requests {
		if request.Output != executionowner.OutputKey {
			continue
		}
		issued++
		if request.Writer != executionowner.AxisKey {
			t.Fatalf("column %q is requested for writer %q", request.Output, request.Writer)
		}
	}
	if issued != 1 {
		t.Fatalf("column %q is requested %d times", executionowner.OutputKey, issued)
	}
}

// TestExecutionReachabilityColumnIsTotalOverItsMountedPoints is the read law.
// The key universe is the mounted execution points the column is published
// with, so a point inside it that carries no row is unreachable as a published
// fact, and a point the publication never covered is ignorance. A reachable
// point costs one unit row and an unreachable one costs none, which is the whole
// of what the column stores.
func TestExecutionReachabilityColumnIsTotalOverItsMountedPoints(t *testing.T) {
	cardinality, cardinalityOK := AxisCardinality(axisKeyExecutionReachability)
	if !cardinalityOK || cardinality != axis.CardinalityDense {
		t.Fatalf("axis %q declares cardinality %d, so its column proves no absence", axisKeyExecutionReachability, cardinality)
	}
	coverage, coverageOK := PublicationCoverage(executionowner.OutputKey)
	if !coverageOK || coverage != axis.CoverageTotal {
		t.Fatalf("column %q publishes coverage %d, not the total coverage its dense axis declares", executionowner.OutputKey, coverage)
	}

	schemaID, schemaOK := PublicationSchema()
	if !schemaOK || !schemaID.Available() {
		t.Fatal("the sealed table publishes no schema identity")
	}
	column, projected := ProjectAxis[mountedExecutionPoint, executionowner.Reachable](executionowner.OutputKey)
	if !projected || !column.Available() {
		t.Fatalf("the declared column %q projects no address", executionowner.OutputKey)
	}
	if int(column.Slot) >= PublicationColumns() {
		t.Fatalf("column %q projects slot %d outside the %d published columns", executionowner.OutputKey, column.Slot, PublicationColumns())
	}

	requests, requestsOK := WriteRequests()
	if !requestsOK {
		t.Fatal("the sealed table issues no write requests")
	}
	builder := snapshot.NewBuilder(schemaID, pilotStore, pilotGeneration)
	// The publication is dense, so every declared column is filled before the
	// snapshot seals. This axis's column is sealed with the key universe it is
	// total over and one unit row: the row is the whole of what a reachable
	// point stores, and a covered point without one is what an unreachable point
	// stores, which is nothing. The peer columns carry a placeholder, because
	// what this law reads is the one column its axis declares.
	for _, request := range requests {
		if request.Output == executionowner.OutputKey {
			if err := snapshot.PutColumn(&builder, column, snapshot.Content[mountedExecutionPoint, executionowner.Reachable]{
				Denominator: reachabilityDenominator,
				Members:     []mountedExecutionPoint{reachedPoint, unreachedPoint},
			}); err != nil {
				t.Fatalf("seal the reachability column: %v", err)
			}
			if err := snapshot.SetRow(&builder, column, reachedPoint, executionowner.Reachable{}); err != nil {
				t.Fatalf("publish a reachable point: %v", err)
			}
			continue
		}
		peer := snapshot.Axis[uint64, uint64]{SchemaID: schemaID, Slot: request.Slot}
		if err := snapshot.PutColumn(&builder, peer, snapshot.Content[uint64, uint64]{
			Denominator: columnDenominator(request.Slot),
			Members:     []uint64{1},
		}); err != nil {
			t.Fatalf("seal the peer column %q: %v", request.Output, err)
		}
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal the publication: %v", err)
	}

	if _, status := snapshot.Read(&sealed, column, reachedPoint); status != snapshot.ReadHit {
		t.Fatalf("a published point read back as %s, not a hit", status)
	}
	if _, status := snapshot.Read(&sealed, column, unreachedPoint); status != snapshot.ReadProvenAbsent {
		t.Fatalf("a covered point with no row read back as %s, not a proven absence", status)
	}
	if _, status := snapshot.Read(&sealed, column, unmountedPoint); status != snapshot.ReadMiss {
		t.Fatalf("an uncovered point read back as %s, not a miss", status)
	}
	mistyped := snapshot.Axis[mountedExecutionPoint, uint64]{SchemaID: schemaID, Slot: column.Slot}
	if _, status := snapshot.Read(&sealed, mistyped, reachedPoint); status != snapshot.ReadInvalid {
		t.Fatalf("a wrong value claim read back as %s", status)
	}
}
