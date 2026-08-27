package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/heap/relation"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// spanInput declares one bounded-span input of a sealed signature. A key
// selection is read as a span because its length is the count the owner's
// enumeration is stated against: a statically keyed candidate delivers no row
// and a dynamically keyed one delivers exactly one.
func spanInput(t testing.TB, relationID model.RelationID, column model.ColumnID, typeID model.TypeID, denominator model.DenominatorRef, bound uint32) signature.Input {
	t.Helper()
	delivery, ok := signature.NewBoundedSpanDelivery(bound, denominator.Key())
	if !ok {
		t.Fatal("bounded span delivery")
	}
	return signature.Input{Relation: relationID, Column: column, Type: typeID, Presence: signature.AllowMissing, Delivery: delivery, Denominator: denominator}
}

// TestThePackExpansionBindsOverTheReadsItsEnumerationMakes drives the read
// side's pack expansion end to end.
//
// The enumeration reads four things: the route it expands, the heap fact that
// route selected, the candidate whose key geometry decides whether a key
// selection is read at all, and the key selection itself. The binding is
// admitted under a signature that delivers exactly those, the last as the span
// its length is counted from, so the frame this test builds is the frame the
// authored plan's four reads project.
//
// The fixture issues no raw-access candidate, so the owner's enumeration
// refuses the operand rather than walking a route. What this states is that
// the whole path is bound and total: the artifacts agree with a signature
// carrying a span slot, and the invocation reaches the owner's own guard.
func TestThePackExpansionBindsOverTheReadsItsEnumerationMakes(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/candidate")

	routeType := place.TypeID(t, "type/heap-route")
	heapType := place.TypeID(t, "type/heap")
	candidateType := place.TypeID(t, "type/read-candidate")
	valueType := place.TypeID(t, "type/value")
	packRouteType := place.TypeID(t, "type/pack-route")

	routeColumn := harness.NewColumn[relation.HeapRouteFact](t, routeType, "store/heap-route", reserve)
	heapColumn := harness.NewColumn[heapdomain.Value](t, heapType, "store/heap", reserve)
	candidateColumn := harness.NewColumn[indexdomain.Index](t, candidateType, "store/read-candidate", reserve)
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	packRouteColumn := harness.NewColumn[relation.PackRouteFact](t, packRouteType, "store/pack-route", reserve)

	columns, ok := relation.NewRawGetPackRoutesColumns(valueColumn, heapColumn, routeColumn, packRouteColumn, candidateColumn)
	if !ok {
		t.Fatal("pack route columns")
	}

	routeAddress := place.Column(t, "column/route")
	heapAddress := place.Column(t, "column/heap")
	candidateAddress := place.Column(t, "column/candidate")
	keyAddress := place.Column(t, "column/key")
	packRouteAddress := place.Column(t, "column/pack-route")

	many, ok := model.NewCardinality(model.BoundedMany, 64)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/raw-get-pack-routes",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, routeAddress, routeType, place.Denominator),
			harness.ScalarInput(t, place.Relation, heapAddress, heapType, place.Denominator),
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			spanInput(t, place.Relation, keyAddress, valueType, place.Denominator, 64),
		},
		[]signature.Output{{Relation: place.Relation, Column: packRouteAddress, Type: packRouteType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		many, outcome.Produced, outcome.NoCandidate, outcome.Refused)

	judgment, ok := relation.NewRawGetPackRoutesOperation(fixture.Topology)
	if !ok {
		t.Fatal("pack route judgment")
	}
	factory, ok := relation.BindRawGetPackRoutes(operation, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind pack routes: the emitted artifacts do not agree with a signature that delivers a key selection as a span")
	}
	worker := place.Worker(t, factory, operation)

	routeToken, ok := routeColumn.Encode(place.Issuer, relation.HeapRouteFact{})
	if !ok {
		t.Fatal("encode route")
	}
	heapToken, ok := heapColumn.Encode(place.Issuer, heapdomain.Value{})
	if !ok {
		t.Fatal("encode heap fact")
	}
	candidateToken, ok := candidateColumn.Encode(place.Issuer, indexdomain.Index{})
	if !ok {
		t.Fatal("encode candidate")
	}
	keyToken, ok := valueColumn.Encode(place.Issuer, fixture.Values.Bottom())
	if !ok {
		t.Fatal("encode key value")
	}

	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, routeAddress, place.Rows[0], routeType, routeToken)),
		harness.ScalarSlot(t, place.Cell(t, heapAddress, place.Rows[0], heapType, heapToken)),
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, candidateToken)),
		harness.SpanSlot(t, []binding.Cell{place.Cell(t, keyAddress, place.Rows[0], valueType, keyToken)}),
	)
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	if result.Code != outcome.Refused {
		t.Fatalf("the pack expansion answered %v for a candidate no topology issued", result.Code)
	}
	if batch, _ := buffer.Seal(result); batch.Len() != 0 {
		t.Fatalf("a refused expansion published %d rows", batch.Len())
	}
}
