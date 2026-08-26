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
	// abiIncomplete means the authored plan delivers every read the
	// enumeration makes, and the owner entry point takes an operand no caller
	// outside domain/heap/index can spell. It is an ABI finding against the
	// owner's publication, and the remedy is a statement the owner makes.
	abiIncomplete
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
// Every one of them is now the owner's own published judgment. Nothing here
// reaches an unexported symbol, and nothing reaches the operand type of the
// protocol this engine replaces: what was once a gap in what the owner said is
// closed.
//
// What remains is one gap, and it is in what the owner publishes. Four
// operations name an exported entry point that takes an operand only their own
// package can spell. The pack expansion takes a heapdomain.KeySelector, and the
// topology projects one only through its unexported selectors and heap schema.
// The two reductions take RawGetFrame and RawSetFrame, whose every selection
// field is typed by the unexported rawSelected, so the struct is exported and
// unconstructible. Neither is answerable by this layer: a binding that reached
// past an owner's own visibility would be answering for a publication nobody
// made.
func rawAccess() []operation {
	return []operation{
		{plan: "raw-get/key-routes", stem: "RawGetKeyRoutes", entry: "index.Index.DynamicKey with (*index.Topology).CoordinateName", state: bound},
		{plan: "raw-get/call-routes", stem: "RawGetCallRoutes", entry: "(*index.Topology).VisitReceiverCallDemand", state: bound},
		{plan: "raw-get/heap-routes", stem: "HeapReceiverRoutes", entry: "(*index.Topology).VisitReceiver", state: bound},
		{plan: "raw-get/pack-routes", stem: "RawGetPackRoutes", entry: "(*index.Topology).VisitRoutePayloads", state: abiIncomplete},
		{plan: "raw-get/source-routes", stem: "RawGetSourceRoutes", entry: "(*index.Topology).VisitPayloadSources", state: bound},
		{plan: "raw-get/result", stem: "RawGetResult", entry: "(*index.Topology).RawGetReduce", state: abiIncomplete},
		{plan: "raw-set/key-routes", stem: "RawSetKeyRoutes", entry: "index.Index.DynamicKey with (*index.Topology).CoordinateName", state: bound},
		{plan: "raw-set/heap-routes", stem: "HeapReceiverRoutes", entry: "(*index.Topology).VisitReceiver", state: bound},
		{plan: "raw-set/pack-routes", stem: "RawSetPackRoutes", entry: "(*index.Topology).VisitRoutePayloads", state: abiIncomplete},
		{plan: "raw-set/source-routes", stem: "RawSetSourceRoutes", entry: "(*index.Topology).VisitPayloadSources", state: bound},
		{plan: "raw-set/commit", stem: "RawSetCommit", entry: "(*index.Topology).RawSetMutateRoute", state: abiIncomplete},
	}
}

// TestEveryRawAccessEnumerationIsAPublishedOwnerJudgment states what the owner
// names. Every operation of both plans names an exported owner entry point, so
// the hot rule and any standing plan reach the same statement of each
// enumeration. Naming an exported entry point is not the same as being callable
// from outside: what four of them still owe is an operand a caller can spell.
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
	unbound := map[string]bool{}
	for _, entry := range rawAccess() {
		if entry.state == abiIncomplete {
			unbound[entry.stem] = true
		}
	}

	declared := 0
	pending := map[string]bool{}
	for _, family := range relbind.Declared().Families {
		if family.Census != "heap/index" {
			continue
		}
		declared++
		if family.Emitted() {
			continue
		}
		pending[family.Stem] = true
		if !strings.Contains(family.Pending, "w0-abi-incomplete") {
			t.Errorf("family %s carries a debt that is not tagged with what blocks it", family.Stem)
		}
		if !strings.Contains(family.Pending, "the owner") && !strings.Contains(family.Pending, "the authored") {
			t.Errorf("family %s carries a debt that does not name the statement that blocks it", family.Stem)
		}
	}
	if declared == 0 {
		t.Fatal("the corpus declares no indexed raw-access family")
	}
	for stem := range unbound {
		if !pending[stem] {
			t.Errorf("the specimen calls %s blocked and the corpus does not carry it as a debt", stem)
		}
	}
	for stem := range pending {
		if !unbound[stem] {
			t.Errorf("the corpus carries %s as a debt and the specimen does not say what blocks it", stem)
		}
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
	blocked := make([]string, 0, len(unbound))
	for _, entry := range rawAccess() {
		if entry.state == abiIncomplete {
			blocked = append(blocked, entry.plan)
		}
	}
	t.Logf("raw-access operations authored: %d, bound: %d, blocked on the owner's publication: %v", len(rawAccess()), len(rawAccess())-len(blocked), blocked)
}
