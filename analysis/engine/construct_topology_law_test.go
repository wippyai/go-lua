package engine

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// TestConstructedTopologyPublishesInjectiveAddressTables proves the current
// ConstructProgram result publishes one graph-owned locator for each Point,
// member and Query identity, with no observation row entering that directory.
func TestConstructedTopologyPublishesInjectiveAddressTables(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 6, nil, nil)
	directory := fixture.graph.directory
	if directory == nil || len(directory.entries) == 0 {
		t.Fatal("constructed program published no address table")
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
			t.Fatalf("identity %x has %d address roles", id[:4], roles)
		}
	}
	for _, id := range fixture.observationIDs {
		if _, ok := directory.resolve(id); ok {
			t.Fatalf("observation %x entered the structural address table", id[:4])
		}
	}
}

// TestConstructedMountedMemberFoldsIntoItsMountedPoint proves every sealed
// member is owned by the graph and contributes to exactly the Group output
// Point that ConstructProgram published for it.
func TestConstructedMountedMemberFoldsIntoItsMountedPoint(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 8, nil, nil)
	graph := fixture.graph.graph
	members := 0
	for index := 0; index < graph.GroupCount(); index++ {
		group, ok := graph.HyperedgeAt(index)
		if !ok || !graph.OwnsGroup(group) || !graph.OwnsPoint(group.Output()) || group.MemberCount() == 0 {
			t.Fatalf("group %d is not a graph-owned mounted fold", index)
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !graph.OwnsMember(member) || member.WriteCount() != 1 {
				t.Fatalf("group %d member %d is not one exact published write", index, memberIndex)
			}
			members++
		}
	}
	if members != len(fixture.graph.members) {
		t.Fatalf("mounted member fold count=%d committed=%d", members, len(fixture.graph.members))
	}
}

// TestConstructedScheduleCoversEveryDeclaredPointExactlyOnce proves the
// immutable schedule is a total ordering of the committed Point plane.
func TestConstructedScheduleCoversEveryDeclaredPointExactlyOnce(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 12, nil, nil)
	graph, ranked := fixture.graph.graph, fixture.graph.graph.Schedule()
	if ranked == nil || ranked.NodeCount() != graph.PointCount() {
		t.Fatal("constructed topology published no total schedule")
	}
	seen := make(map[composition.Key]struct{}, graph.PointCount())
	for index := 0; index < ranked.EventCount(); index++ {
		event, ok := ranked.EventAt(index)
		if !ok || event.Kind != schedule.EventNode {
			continue
		}
		point, pointOK := graph.PointAt(event.Node)
		if !pointOK || !graph.OwnsPoint(point) {
			t.Fatalf("schedule event %d has no committed Point", index)
		}
		if _, duplicate := seen[point.Key()]; duplicate {
			t.Fatalf("Point %v appeared twice in the schedule", point.Key())
		}
		seen[point.Key()] = struct{}{}
	}
	if len(seen) != graph.PointCount() {
		t.Fatalf("schedule covered %d/%d committed Points", len(seen), graph.PointCount())
	}
}

// TestConstructedTopologyRefusesDuplicateIdentity proves a sealed program's
// observation table rejects a repeated public identity instead of publishing a
// second ordinal for it.
func TestConstructedTopologyRefusesDuplicateIdentity(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	rows := append([]ProgramObservationAdmission(nil), fixture.observations...)
	rows = append(rows, rows[0])
	if _, failure, ok := fixture.graph.Seal(rows); ok || !failure.Available() {
		t.Fatalf("duplicate identity sealed: ok=%t failure=%v", ok, failure)
	}
}

// TestConstructedTopologyRefusesInadmissibleIssuance proves ConstructProgram
// refuses an inventory with no mounted artifact rows and publishes no partial
// geometry.
func TestConstructedTopologyRefusesInadmissibleIssuance(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	program, refusal, ok := ConstructProgram(ProgramDeclaration{Binding: fixture.binding})
	if ok || program != nil || !refusal.LoweringFailure().Available() {
		t.Fatalf("empty admission published program=%v refusal=%v", program, refusal)
	}
}

// TestConstructedTopologyRefusesScheduleViolation keeps the rank/edge fence
// at the scheduler boundary: invalid dense endpoints cannot become a partial
// committed schedule.
func TestConstructedTopologyRefusesScheduleViolation(t *testing.T) {
	if got, err := schedule.Prepare(2, []schedule.Edge{{From: 2, To: 0}}); got != nil || !errors.Is(err, schedule.ErrInvalidEdge) {
		t.Fatalf("invalid topology edge produced schedule=%v err=%v", got, err)
	}
}

// TestConstructedTopologyRefusesUnsitedDeclaration proves the one public
// construction entry point rejects a declaration whose mount plane is absent.
func TestConstructedTopologyRefusesUnsitedDeclaration(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	if program, _, ok := ConstructProgram(ProgramDeclaration{Binding: fixture.binding}); ok || program != nil {
		t.Fatal("unsited declaration crossed ConstructProgram")
	}
}

// TestConstructedOwnerDeclaredTablesArePublished keeps the owner-declared
// query/observation row cardinality tied to the graph's sealed tables.
func TestConstructedOwnerDeclaredTablesArePublished(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 4, nil, nil)
	if len(fixture.graph.queries) != fixture.graph.graph.QueryCount() || len(fixture.solver.runtime.observations) != len(fixture.observations) {
		t.Fatalf("owner tables queries=%d/%d observations=%d/%d", len(fixture.graph.queries), fixture.graph.graph.QueryCount(), len(fixture.solver.runtime.observations), len(fixture.observations))
	}
}

// TestConstructedOwnerDeclaredRefusesDuplicateQueryIdentity delegates the
// duplicate-row refusal to the sealed observation table, where the same
// identity fence applies without retaining a construction workspace.
func TestConstructedOwnerDeclaredRefusesDuplicateQueryIdentity(t *testing.T) {
	fixture := newObservedReceiptQueryMatrixFixture(t, 1, nil, nil)
	rows := append([]ProgramObservationAdmission(nil), fixture.observations...)
	rows = append(rows, rows[0])
	if _, failure, ok := fixture.graph.Seal(rows); ok || !failure.Available() {
		t.Fatal("duplicate declared row crossed the program table")
	}
}

// TestConstructedOwnerDeclaredRefusesPointCollision proves a published
// identity cannot simultaneously occupy two structural directory planes.
func TestConstructedOwnerDeclaredRefusesPointCollision(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	for id := range fixture.graph.directory.entries {
		roles := 0
		if _, ok := fixture.graph.directory.point(id); ok {
			roles++
		}
		if _, ok := fixture.graph.directory.member(id); ok {
			roles++
		}
		if _, ok := fixture.graph.directory.query(id); ok {
			roles++
		}
		if roles != 1 {
			t.Fatalf("directory collision for %x", id[:4])
		}
	}
}

// TestConstructedActivationPlaneIsTotalOverRegisteredTriggers keeps the
// activation transport law beside construction: every admitted vector is
// bound to the same sealed activation slot and exact Factor identities.
func TestConstructedActivationPlaneIsTotalOverRegisteredTriggers(t *testing.T) {
	owner := newActivationTransportLawOwner(t, 3, 971_100)
	binding := openActivationTransportLawBinding(t, owner)
	issuer, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, []AnyFactorRef{owner.factors[0].Ref().Any()}, owner.factors[1].Ref().Any())
	if !ok || issuer == nil || !binding.Seal() || len(issuer.imports) != 1 || !issuer.export.Available() {
		t.Fatal("activation plane did not publish one registered trigger transport")
	}
}

// TestConstructedActivationPlaneRefusesIncompleteCandidateSets proves an
// activation plane with no imported Factor cannot issue a body candidate.
func TestConstructedActivationPlaneRefusesIncompleteCandidateSets(t *testing.T) {
	owner := newActivationTransportLawOwner(t, 2, 971_200)
	binding := openActivationTransportLawBinding(t, owner)
	if issuer, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, nil, owner.factors[1].Ref().Any()); ok || issuer != nil {
		t.Fatal("empty activation transport vector admitted")
	}
	if binding.Poisoned() {
		t.Fatal("a rejected candidate poisoned the still-open activation binding")
	}
}

// TestConstructedActivationPlaneRefusesUnshapedTrigger proves an unavailable
// trigger reference cannot be smuggled into the sealed candidate plane.
func TestConstructedActivationPlaneRefusesUnshapedTrigger(t *testing.T) {
	owner := newActivationTransportLawOwner(t, 2, 971_300)
	binding := openActivationTransportLawBinding(t, owner)
	if _, ok := BindMountedActivationCandidateIssuer(binding, owner.rule, []AnyFactorRef{AnyFactorRef{}}, owner.factors[1].Ref().Any()); ok {
		t.Fatal("unavailable trigger transport admitted")
	}
}

// TestConstructedQueryTableIsAddressedInDeclaredOrder delegates query address
// order to the current sealed table binder.
func TestConstructedQueryTableIsAddressedInDeclaredOrder(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 7, nil, nil)
	bound := make(map[composition.Key]runtimeQuery, len(fixture.solver.runtime.queries))
	for _, row := range fixture.solver.runtime.queries {
		bound[row.query().Key()] = row
	}
	rows, ok := bindProgramQueryTable(fixture.addressed, fixture.graph.graph, bound)
	if !ok || len(rows) != len(fixture.addressed) {
		t.Fatalf("query table rows=%d ok=%t addressed=%d", len(rows), ok, len(fixture.addressed))
	}
}
