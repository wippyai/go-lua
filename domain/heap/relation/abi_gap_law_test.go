package relation_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	calldomain "github.com/wippyai/go-lua/domain/call"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

// standing is what one authored raw-access operation has: a binding, or a
// named reason it has none.
type standing uint8

const (
	// bound means one declared family carries this operation.
	bound standing = iota
	// planIncomplete means the owner mathematics is published and reachable,
	// and the authored plan does not deliver everything the enumeration reads.
	// It is a declaration-surface finding against the plan, not an ABI gap,
	// and the remedy is a join the plan does not yet state.
	planIncomplete
)

// operation is one semantic operation the two authored indexed raw-access
// plans state, the owner entry point that answers it, and where it stands.
//
// This table is the hostile specimen for raw access. It is written to be
// uncomfortable: it names every operation both plans declare and refuses to
// let one disappear. Half a surface is worse than a named hole, so what is
// missing is named and what is missing about it is named too.
type operation struct {
	plan  string
	stem  string
	entry string
	state standing
}

// rawAccess is the eleven operations the two indexed raw-access plans author.
//
// Every enumeration is now the owner's own published judgment. Nothing here
// reaches an unexported symbol, and nothing reaches the operand type of the
// protocol this engine replaces: what was once a gap in what the owner said is
// closed. What remains is a gap in what the plan delivers. The pack expansion
// reads the payloads one selected route carries under a key selector, and the
// authored step joins the route relation onto the heap facts alone, so the
// selector the key route determined never reaches the frame.
func rawAccess() []operation {
	return []operation{
		{plan: "raw-get/key-routes", stem: "RawGetKeyRoutes", entry: "index.Index.DynamicKey with (*index.Topology).CoordinateName", state: bound},
		{plan: "raw-get/call-routes", stem: "RawGetCallRoutes", entry: "(*index.Topology).VisitReceiverCallDemand", state: bound},
		{plan: "raw-get/heap-routes", stem: "HeapReceiverRoutes", entry: "(*index.Topology).VisitReceiver", state: bound},
		{plan: "raw-get/pack-routes", stem: "RawGetPackRoutes", entry: "(*index.Topology).VisitRoutePayloads", state: planIncomplete},
		{plan: "raw-get/source-routes", stem: "RawGetSourceRoutes", entry: "(*index.Topology).VisitPayloadSources", state: bound},
		{plan: "raw-get/result", stem: "RawGetResult", entry: "(*index.Topology).RawGetResult", state: bound},
		{plan: "raw-set/key-routes", stem: "RawSetKeyRoutes", entry: "index.Index.DynamicKey with (*index.Topology).CoordinateName", state: bound},
		{plan: "raw-set/heap-routes", stem: "HeapReceiverRoutes", entry: "(*index.Topology).VisitReceiver", state: bound},
		{plan: "raw-set/pack-routes", stem: "RawSetPackRoutes", entry: "(*index.Topology).VisitRoutePayloads", state: planIncomplete},
		{plan: "raw-set/source-routes", stem: "RawSetSourceRoutes", entry: "(*index.Topology).VisitPayloadSources", state: bound},
		{plan: "raw-set/commit", stem: "RawSetCommit", entry: "(*index.Topology).RawSetCommit", state: bound},
	}
}

// TestEveryRawAccessEnumerationIsAPublishedOwnerJudgment states the closure of
// the original gap. Every operation of both plans names an exported owner
// entry point, so no binding is waiting on the owner to say something, and the
// hot rule and any standing plan reach the same statement of each enumeration.
func TestEveryRawAccessEnumerationIsAPublishedOwnerJudgment(t *testing.T) {
	for _, entry := range rawAccess() {
		if entry.entry == "" {
			t.Errorf("%s names no owner entry point", entry.plan)
			continue
		}
		if strings.Contains(entry.entry, "index.") && strings.Contains(entry.entry, ".index") {
			t.Errorf("%s reaches an unexported symbol", entry.plan)
		}
		for _, unexported := range []string{"visitSelectedPayloads", "catalog.", "rawSourceTag", "RawGetRule", "RawSetRule", "mutateRoute"} {
			if strings.Contains(entry.entry, unexported) {
				t.Errorf("%s reaches %s, which the owner does not publish", entry.plan, unexported)
			}
		}
	}
}

// TestTheOwnerEnumerationsAreReallyReachable drives the published judgments
// against production authorities sealed from a real program, so the table
// above states what the code does rather than what it intends.
func TestTheOwnerEnumerationsAreReallyReachable(t *testing.T) {
	fixture := relationfixture.New(t)
	routes := 0
	if !fixture.Topology.VisitReceiver(fixture.Receiver, nil, func(route indexdomain.Route) bool {
		routes++
		return true
	}) {
		t.Fatal("the sealed topology refused the receiver its own program allocates")
	}
	if routes == 0 {
		t.Fatal("the sealed topology observed no route for a receiver that denotes a table root")
	}
	if !fixture.Topology.VisitReceiverCallDemand(fixture.Receiver, func(calldomain.Key, uint64) bool { return true }) {
		t.Fatal("the sealed topology refused the call demand of its own receiver")
	}
	rootKey, ok := fixture.Root.ContentID()
	if !ok || !rootKey.Available() {
		t.Fatal("the fixture root carries no owner-issued name")
	}
	named := 0
	for index := 0; index < fixture.Values.CoordinateCount(); index++ {
		coordinate, coordinateOK := fixture.Values.CoordinateAt(index)
		if !coordinateOK {
			t.Fatalf("coordinate %d is not issued", index)
		}
		if _, ok := fixture.Topology.CoordinateName(coordinate); ok {
			named++
		}
	}
	if named != fixture.Values.CoordinateCount() {
		t.Fatalf("the topology names %d of %d coordinates; a route destination must carry its owner's name", named, fixture.Values.CoordinateCount())
	}
}

// TestEveryUnboundRawAccessOperationIsCarriedAsANamedDebt states that what is
// still missing is carried rather than routed around. An operation the corpus
// cannot bind is named here with the reason, and the census rows it belongs to
// stay declared unbound with that same reason, so nothing is approximated and
// nothing is quietly dropped.
func TestEveryUnboundRawAccessOperationIsCarriedAsANamedDebt(t *testing.T) {
	unbound := make([]string, 0, 2)
	for _, entry := range rawAccess() {
		if entry.state == planIncomplete {
			unbound = append(unbound, entry.plan)
		}
	}

	declared := 0
	pending := 0
	for _, family := range relbind.Declared().Families {
		if family.Census != "heap/index" {
			continue
		}
		declared++
		if family.Emitted() {
			continue
		}
		pending++
		if !strings.Contains(family.Pending, "w0-plan-incomplete") {
			t.Errorf("family %s carries a debt that is not tagged with what blocks it", family.Stem)
		}
		if !strings.Contains(family.Pending, "key selector") {
			t.Errorf("family %s carries a debt that does not name what the plan fails to deliver", family.Stem)
		}
	}
	if declared == 0 {
		t.Fatal("the corpus declares no indexed raw-access family")
	}

	// An operation this specimen calls bound must be a family that is really
	// declared and really emitted. Without this the table could award itself a
	// binding that does not exist, which is the one thing a specimen must
	// never be able to do.
	emitted := map[string]bool{}
	for _, family := range relbind.Declared().Families {
		if family.Emitted() {
			emitted[family.Stem] = true
		}
	}
	for _, entry := range rawAccess() {
		if entry.state != bound {
			continue
		}
		if entry.stem == "" {
			t.Errorf("%s is called bound and names no family", entry.plan)
			continue
		}
		if !emitted[entry.stem] {
			t.Errorf("%s is called bound and the corpus emits no family %s", entry.plan, entry.stem)
		}
	}
	if len(unbound) != pending {
		t.Fatalf("the specimen names %d unbound operations and the corpus carries %d unbound rows", len(unbound), pending)
	}
	t.Logf("raw-access operations authored: %d, bound: %d, blocked on the plan: %v", len(rawAccess()), len(rawAccess())-len(unbound), unbound)
}
