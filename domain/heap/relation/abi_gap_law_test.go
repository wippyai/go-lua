package relation_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	calldomain "github.com/wippyai/go-lua/domain/call"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

// operation is one semantic operation the authored indexed raw-access plans
// state, and the owner entry point a binding would carry it through.
//
// This table is the hostile specimen for the raw-access ABI gap. It is written
// to be uncomfortable: it names every operation the plan declares, says which
// owner mathematics answers it, and says plainly which ones no binding can
// name. Half a surface is worse than a named hole, so the hole is named.
type operation struct {
	plan      string
	entry     string
	reachable bool
}

// rawAccess is the eleven operations the two indexed raw-access plans author.
// The route mathematics is the sealed topology's own traversal and it is
// reachable. The payload-tail expansion, the semantic-source expansion and
// both reductions are reachable only through symbols domain/heap/index does
// not export and through the operand type of the protocol this engine
// replaces, so no binding can name them without the owner publishing them.
func rawAccess() []operation {
	return []operation{
		{plan: "raw-get/key-routes", entry: "the dynamic key of the legacy Index operand", reachable: false},
		{plan: "raw-get/call-routes", entry: "(*index.Topology).VisitReceiverCallDemand", reachable: true},
		{plan: "raw-get/heap-routes", entry: "(*index.Topology).VisitReceiver", reachable: true},
		{plan: "raw-get/pack-routes", entry: "index.visitSelectedPayloads over an unexported payload descriptor", reachable: false},
		{plan: "raw-get/source-routes", entry: "index.catalog.sourceTags over the unexported rawSourceTag", reachable: false},
		{plan: "raw-get/result", entry: "(*index.RawGetRule).reduce over the legacy Index operand", reachable: false},
		{plan: "raw-set/key-routes", entry: "the dynamic key of the legacy Index operand", reachable: false},
		{plan: "raw-set/heap-routes", entry: "(*index.Topology).VisitReceiver", reachable: true},
		{plan: "raw-set/pack-routes", entry: "index.payloadForWrite over an unexported payload descriptor", reachable: false},
		{plan: "raw-set/source-routes", entry: "index.catalog.sourceTags over the unexported rawSourceTag", reachable: false},
		{plan: "raw-set/commit", entry: "(*index.RawSetRule).mutateRoute over the legacy Index operand", reachable: false},
	}
}

// TestTheReachableRawAccessMathematicsIsReallyReachable states the gap is
// about publication and not about the mathematics. The owner's route
// traversals exist, are exported, and answer through the same typed frame
// every other family answers through, so what is missing for the rest is an
// owner statement and never a lowering this layer could invent.
func TestTheReachableRawAccessMathematicsIsReallyReachable(t *testing.T) {
	fixture := relationfixture.New(t)
	visited := 0
	if !fixture.Topology.VisitReceiver(fixture.Receiver, nil, func(indexdomain.Route) bool {
		visited++
		return true
	}) {
		t.Fatal("the sealed topology refused the receiver its own program allocates")
	}
	if visited == 0 {
		t.Fatal("the sealed topology observed no route for a receiver that denotes a table root")
	}
	demanded := fixture.Topology.VisitReceiverCallDemand(fixture.Receiver, func(calldomain.Key, uint64) bool { return true })
	if !demanded {
		t.Fatal("the sealed topology refused the call demand of its own receiver")
	}
}

// TestEveryUnreachableRawAccessOperationIsCarriedAsANamedDebt states that the
// corpus carries the gap rather than routing around it. An unreachable
// operation is not bound, is not approximated, and is not quietly dropped: it
// is one declared row whose reason names the owner and the tag.
func TestEveryUnreachableRawAccessOperationIsCarriedAsANamedDebt(t *testing.T) {
	unreachable := 0
	for _, entry := range rawAccess() {
		if !entry.reachable {
			unreachable++
		}
	}
	if unreachable == 0 {
		t.Fatal("the specimen claims a gap and lists no unreachable operation")
	}

	rows := 0
	for _, family := range relbind.Declared().Families {
		if family.Census != "heap/index" {
			continue
		}
		rows++
		if family.Emitted() {
			t.Errorf("family %s is bound while the raw-access gap is open; close the specimen with it", family.Stem)
			continue
		}
		if !strings.Contains(family.Pending, "w0-abi-gap") {
			t.Errorf("family %s carries a debt that is not tagged as an ABI gap", family.Stem)
		}
		if !strings.Contains(family.Pending, "domain/heap/index") {
			t.Errorf("family %s carries a debt that does not name the owner that must publish", family.Stem)
		}
	}
	if rows != 2 {
		t.Fatalf("the corpus declares %d indexed raw-access rows, and the census states two", rows)
	}
	t.Logf("raw-access operations authored: %d, reachable: %d, unreachable: %d", len(rawAccess()), len(rawAccess())-unreachable, unreachable)
}
