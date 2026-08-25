package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// A routed rule declares two projections over one member of its derived
// relation: the join key it OBSERVES the fact at, and the output destination
// it PUBLISHES the reduced fact at. The schema admits declarations whose two
// projections name different coordinates, so the write target is what the
// output's destination projection states and never what the join key happens
// to name.
//
// The laws below are stated over the declaration, which is where the emitter
// answers: one clause of one declaration moves, and exactly the write half of
// the emitted family moves with it. The identity case, where both projections
// answer the same coordinate - which is Call's dispatch relation - is pinned
// as one admitted declaration of that contract rather than as the contract.

const (
	elsewhereDestinationKey schema.Key = "placement/return-escape/route-elsewhere"
	identityDestinationKey  schema.Key = "placement/return-escape/route-key-as-destination"
)

// destinationProjectionRoster is the member-set roster with two further
// destination projections of the routed relation: one that projects a
// coordinate of its own, and one that projects the very accessor the join key
// projects.
func destinationProjectionRoster(t testing.TB) definition.Roster {
	t.Helper()
	provider := member.AxisRelationCandidate(member.RelationRef{
		Axis: memberSetValueAxisRef(), Member: "value/return-boundary/candidates",
	})
	placement := memberSetPlacementDefinition()
	placement.Projections = append(placement.Projections,
		definition.Projection{
			Name: "RouteElsewhere", Key: elsewhereDestinationKey, Relation: "Routes",
			Role: member.Destination, Result: "PlacementKey",
			Accessor:          specimenMethod("Elsewhere", "Route", -1),
			CandidateProvider: provider,
		},
		definition.Projection{
			Name: "RouteKeyAsDestination", Key: identityDestinationKey, Relation: "Routes",
			Role: member.Destination, Result: "PlacementKey",
			Accessor:          specimenMethod("Key", "Route", -1),
			CandidateProvider: provider,
		},
	)
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "membersetvalue", Name: "membersetvalue", Base: memberSetValueDefinition()},
		definition.Source{
			Package: "membersetplacement", Name: "membersetplacement",
			Base:          placement,
			Contributions: []definition.Contribution{memberSetPlacementContribution()},
		},
	)
	if !rosterOK {
		t.Fatal("destination-projection roster is not admissible")
	}
	return roster
}

// renderDestination emits the member-set family with one clause changed: the
// projection its routed output publishes at.
func renderDestination(t testing.TB, destination schema.Key) string {
	t.Helper()
	spec := memberSetSpec()
	spec.Program.Fold.Outputs[0].Destination = member.ProjectionRef{
		Axis: memberSetPlacementAxisRef(), Member: destination,
	}
	target := memberSetTarget()
	target.Spec = spec
	source, err := Render(target, destinationProjectionRoster(t))
	if err != nil {
		t.Fatalf("a routed declaration publishing at %q did not emit: %v", string(destination), err)
	}
	return string(source)
}

// declaredDestination is the projection the member-set specimen publishes at
// before any of these laws move it.
func declaredDestination() schema.Key { return "placement/return-escape/route-destination" }

// TestARoutedRowStagesAtTheDestinationItsOutputProjects is the K != D law at
// the emitter. The declaration observes at one projection of a member and
// publishes at another; the emitted worker therefore resolves two dense
// coordinates through the routed axis directory - the key's, which the member
// is observed at, and the destination's, which the fact is staged at - and
// hands both to the one authenticated route member. Nothing reconstructs the
// destination from the key, the tag, or the member.
func TestARoutedRowStagesAtTheDestinationItsOutputProjects(t *testing.T) {
	source := renderDestination(t, elsewhereDestinationKey)

	if !strings.Contains(source, "routeElsewhere, routeElsewhereOK := selected.Elsewhere()") {
		t.Fatalf("the worker does not take the output's own destination projection:\n%s", source)
	}
	if !strings.Contains(source, "destinationDense, destinationDenseOK := lane.family.placementSchema.KeyIndex(routeElsewhere)") {
		t.Fatalf("the destination is not normalized through the routed axis directory:\n%s", source)
	}
	if !strings.Contains(source, "dense, denseOK := lane.family.placementSchema.KeyIndex(routeKey)") {
		t.Fatalf("the observed member is not resolved from the join's key projection:\n%s", source)
	}
	if !strings.Contains(source, "member, memberOK := lane.family.plane.RouteMember(uint32(dense), uint32(destinationDense), uint64(routeTag))") {
		t.Fatalf("the staged write target is not the destination's own coordinate:\n%s", source)
	}
	if !strings.Contains(source, "routes[index] = routeElsewhere") {
		t.Fatalf("the fold is handed a route carrier that is not the projected destination:\n%s", source)
	}
	if strings.Contains(source, "KeyIndex(routeKey)\n\t\tif !destinationDenseOK") ||
		strings.Contains(source, "RouteMember(uint32(dense), uint32(dense)") {
		t.Fatalf("the emitted worker publishes at the coordinate it observed:\n%s", source)
	}
	if strings.Contains(source, "_ = routeElsewhere\n") {
		t.Fatalf("the worker projects a destination and discards it:\n%s", source)
	}
}

// TestChangingTheDestinationProjectionMovesOnlyTheWriteTarget states the
// separation between the two projections at the declaration. One clause
// changes - which projection the output publishes at - and the write target
// moves with it, while the coordinate the row observes at, the tag that names
// that member, and the support the selection is observed under do not move:
// they are the join's own and belong to the read.
func TestChangingTheDestinationProjectionMovesOnlyTheWriteTarget(t *testing.T) {
	declared := renderDestination(t, declaredDestination())
	moved := renderDestination(t, elsewhereDestinationKey)
	if declared == moved {
		t.Fatal("moving the routed output onto another destination projection emitted the same family")
	}

	for _, observation := range []string{
		"routeKey, routeKeyOK := selected.Key()",
		"routeTag, routeTagOK := selected.Predicate()",
		"dense, denseOK := lane.family.placementSchema.KeyIndex(routeKey)",
		"uint64(routeTag))",
		"members[index] = member",
		"status := row.read1.Observe(ticket, &lane.read1, members, cells)",
	} {
		if !strings.Contains(declared, observation) || !strings.Contains(moved, observation) {
			t.Fatalf("the observed support coordinate moved with the destination: %q", observation)
		}
	}

	// The write target is the destination coordinate the member is resolved
	// at, so moving the projection must move exactly that resolution and leave
	// the member's own line - which names both halves - standing.
	movedTarget := "destinationDense, destinationDenseOK := lane.family.placementSchema.KeyIndex(routeElsewhere)"
	if !strings.Contains(moved, movedTarget) || strings.Contains(declared, movedTarget) {
		t.Fatalf("the write target did not move with the destination projection:\n%s", moved)
	}
	member := "member, memberOK := lane.family.plane.RouteMember(uint32(dense), uint32(destinationDense), uint64(routeTag))"
	if !strings.Contains(declared, member) || !strings.Contains(moved, member) {
		t.Fatalf("the two declarations do not resolve their member under one contract:\n%s", moved)
	}

	for _, line := range linesOnlyIn(declared, moved) {
		if !strings.Contains(line, "estination") && !strings.Contains(line, "routeElsewhere") {
			t.Fatalf("changing the destination projection changed a line that is not the write target: %q", line)
		}
	}
}

// TestAnIdentityDestinationIsOneCaseOfTheContract pins Call's shape. A
// declaration whose destination projection answers the same coordinate its
// join key projects - which is what the dispatch relation does - is one
// admitted case of the two-projection contract, not the contract itself. The
// emitted worker projects that destination on its own, normalizes it on its
// own, and hands it to the route member as the write coordinate, exactly as it
// does for a declaration whose projections differ. The two coordinates being
// equal is then a fact about the owner's relation and about nothing the
// emitter did.
func TestAnIdentityDestinationIsOneCaseOfTheContract(t *testing.T) {
	source := renderDestination(t, identityDestinationKey)

	if !strings.Contains(source, "routeKeyAsDestination, routeKeyAsDestinationOK := selected.Key()") {
		t.Fatalf("an identity destination is not projected on its own:\n%s", source)
	}
	if !strings.Contains(source, "destinationDense, destinationDenseOK := lane.family.placementSchema.KeyIndex(routeKeyAsDestination)") {
		t.Fatalf("an identity destination reuses the member's dense coordinate:\n%s", source)
	}
	if !strings.Contains(source, "member, memberOK := lane.family.plane.RouteMember(uint32(dense), uint32(destinationDense), uint64(routeTag))") {
		t.Fatalf("an identity destination is not carried to the write half of the member:\n%s", source)
	}
	if !strings.Contains(source, "routes[index] = routeKeyAsDestination") {
		t.Fatalf("an identity destination is not the route carrier the fold receives:\n%s", source)
	}
	if strings.Contains(source, "_ = routeKeyAsDestination\n") {
		t.Fatalf("an identity destination is projected and discarded:\n%s", source)
	}

	distinct := renderDestination(t, elsewhereDestinationKey)
	for _, shape := range []string{
		"dense, denseOK := lane.family.placementSchema.KeyIndex(routeKey)",
		"member, memberOK := lane.family.plane.RouteMember(uint32(dense), uint32(destinationDense), uint64(routeTag))",
	} {
		if !strings.Contains(source, shape) || !strings.Contains(distinct, shape) {
			t.Fatalf("the identity case and the distinct case do not emit one contract: %q", shape)
		}
	}
}

// linesOnlyIn answers the lines of right that left does not hold anywhere. It
// is a membership difference rather than an alignment: these laws ask what a
// declaration change introduced, not where it landed in the file.
func linesOnlyIn(left, right string) []string {
	held := map[string]struct{}{}
	for _, line := range strings.Split(left, "\n") {
		held[strings.TrimSpace(line)] = struct{}{}
	}
	var changed []string
	for _, line := range strings.Split(right, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if _, present := held[trimmed]; !present {
			changed = append(changed, trimmed)
		}
	}
	return changed
}
