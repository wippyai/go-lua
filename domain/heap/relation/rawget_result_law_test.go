package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestTheRawGetDecoderResolvesEveryTagToTheRowThatCarriesIt is the decoder law.
//
// The owner's reduction looks its facts up by the owner tags its own
// enumeration issues, and the frame delivers those facts as rows addressed by
// the identity each row's own owner issued. The decoder is that correspondence
// and nothing else, so what has to be proven is that every tag it can be asked
// for lands on the row that actually carries the fact - checked against rows
// this law addressed itself, so a wrong correspondence has somewhere to show.
func TestTheRawGetDecoderResolvesEveryTagToTheRowThatCarriesIt(t *testing.T) {
	fixture := relationfixture.New(t)

	// Source tags: the owner names each source's coordinate, and the value
	// schema names that coordinate. A row addressed by that name is the row
	// carrying the fact the tag asks for.
	sources := 0
	for payload := 0; payload < fixture.Values.CoordinateCount(); payload++ {
		coordinate, coordinateOK := fixture.Values.CoordinateAt(payload)
		if !coordinateOK {
			t.Fatalf("coordinate %d is not issued", payload)
		}
		name, named := fixture.Topology.CoordinateName(coordinate)
		if !named {
			t.Fatalf("coordinate %d carries no owner-issued name", payload)
		}
		back, resolved := fixture.Values.CoordinateForID(name)
		if !resolved || back != coordinate {
			t.Fatalf("the name of coordinate %d does not resolve back to it", payload)
		}
		sources++
	}
	if sources == 0 {
		t.Fatal("the fixture sealed no coordinate for a source tag to name")
	}

	// Route tags: the owner names each rooted route by its heap key, and the
	// heap key names its own row.
	routes := 0
	if !fixture.Topology.VisitReceiver(fixture.Receiver, nil, func(route indexdomain.Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			return true
		}
		routes++
		tag, tagged := fixture.Topology.RawRouteTag(key, role)
		name, named := key.ContentID()
		if !tagged || tag == 0 || !named || !name.Available() {
			t.Fatal("a rooted route carries no tag or no owner-issued row name")
		}
		return true
	}) {
		t.Fatal("the sealed topology refused the receiver its own program allocates")
	}
	if routes == 0 {
		t.Fatal("the fixture sealed no rooted route for a route tag to name")
	}

	// Demand tags: the owner enumerates each demanded call with its tag, and
	// the call names its own row. Two tags never name one row and one tag
	// never names two, or the correspondence would not be one.
	demand := map[uint64]identity.ContentID{}
	rows := map[identity.ContentID]uint64{}
	if !fixture.Topology.VisitReceiverCallDemand(fixture.Receiver, func(key calldomain.Key, tag uint64) bool {
		name, named := key.ContentID()
		if !named {
			t.Fatal("a demanded call carries no owner-issued row name")
		}
		if previous, seen := demand[tag]; seen && previous != name {
			t.Fatalf("demand tag %d names two rows", tag)
		}
		if previous, seen := rows[name]; seen && previous != tag {
			t.Fatalf("one call row carries demand tags %d and %d", previous, tag)
		}
		demand[tag], rows[name] = name, tag
		return true
	}) {
		t.Fatal("the sealed topology refused the call demand of its own receiver")
	}
}

// TestTheRawGetDecoderReadsTheRowItWasDelivered drives the correspondence
// through a real delivery: a span this law addressed row by row, read back by
// the identity the owner names each coordinate with.
func TestTheRawGetDecoderReadsTheRowItWasDelivered(t *testing.T) {
	fixture := relationfixture.New(t)
	coordinates := fixture.Values.CoordinateCount()
	names := make([]identity.ContentID, 0, coordinates)
	for index := 0; index < coordinates; index++ {
		coordinate, ok := fixture.Values.CoordinateAt(index)
		if !ok {
			t.Fatalf("coordinate %d is not issued", index)
		}
		name, named := fixture.Topology.CoordinateName(coordinate)
		if !named {
			t.Fatalf("coordinate %d carries no owner-issued name", index)
		}
		names = append(names, name)
	}
	place := harness.NewKeyed(t, names)
	valueType := place.TypeID(t, "type/value")
	column := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	cellAddress := place.Column(t, "column/value")

	// Each row carries a distinguishable fact: the bottom of the schema at one
	// coordinate and its top at every other, so reading the wrong row is
	// visible rather than merely possible.
	marked := 1
	if coordinates < 2 {
		t.Fatalf("the fixture exposes %d coordinates", coordinates)
	}
	cells := make([]binding.Cell, 0, coordinates)
	for index, row := range place.Rows {
		fact := fixture.Values.Top()
		if index == marked {
			fact = fixture.Values.Bottom()
		}
		token, encodeOK := column.Encode(place.Issuer, fact)
		if !encodeOK {
			t.Fatal("encode coordinate")
		}
		cells = append(cells, place.Cell(t, cellAddress, row, valueType, token))
	}
	frame := place.Frame(t, harness.SpanSlot(t, cells))
	span, ok := relbindgen.SpanAtFrame(frame, 0, column)
	if !ok {
		t.Fatal("borrow span")
	}

	for index := 0; index < span.Len(); index++ {
		delivered, ok := span.RowKeyAt(index)
		if !ok || delivered != names[index] {
			t.Fatalf("delivered row %d does not carry the identity it was addressed by", index)
		}
		value, present, available := span.At(index)
		if !available || !present {
			t.Fatalf("delivered row %d carries no fact", index)
		}
		wantBottom := index == marked
		if fixture.Values.LessOrEq(value, fixture.Values.Bottom()) != wantBottom {
			t.Fatalf("delivered row %d carries the fact of another row", index)
		}
	}
}

// TestTheRawSetDecoderNamesEveryRouteByItsOwnKey is the write decoder's law.
//
// A write publishes one row per route it ascends, at the heap key that route
// is rooted at. The correspondence it rests on is the read's in reverse: a
// delivered heap row is addressed by its key's own identity, and the tag that
// key answers under is the one the owner issues for it. Both directions have
// to agree, or a write would ascend one route's cell and publish it at
// another's row.
func TestTheRawSetDecoderNamesEveryRouteByItsOwnKey(t *testing.T) {
	fixture := relationfixture.New(t)
	rooted := 0
	seen := map[identity.ContentID]heapdomain.RawRouteTag{}
	if !fixture.Topology.VisitReceiver(fixture.Receiver, nil, func(route indexdomain.Route) bool {
		key, role, isRoot := route.Root()
		if !isRoot {
			return true
		}
		rooted++
		tag, tagged := fixture.Topology.RawRouteTag(key, role)
		row, named := key.ContentID()
		if !tagged || !named {
			t.Fatal("a rooted route carries no tag or no owner-issued row name")
		}
		if previous, repeat := seen[row]; repeat && previous != tag {
			t.Fatalf("one route row answers under tags %d and %d", previous, tag)
		}
		seen[row] = tag

		// The owner resolves the tag back to the same key and role, so the row
		// a write publishes at is the row whose cell it ascended.
		back, backRole, resolved := fixture.Heap.RouteForTag(tag)
		if !resolved || back != key || backRole != role {
			t.Fatalf("route tag %d does not resolve back to the route it names", tag)
		}
		return true
	}) {
		t.Fatal("the sealed topology refused the receiver its own program allocates")
	}
	if rooted == 0 {
		t.Fatal("the fixture sealed no rooted route for a write to publish at")
	}
}
