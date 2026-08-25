package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// The carry of a publication is indexed by the thing that publishes.
//
// An exact rule publishes at one coordinate, so it has one image to carry and
// one transition to carry it through, and the transition is a method on the
// candidate. A routed rule publishes at N derived destinations, so it has N of
// each, and asking which of them is "the" carry has no answer. The carry is
// therefore taken per published row: one closure over the image at that row's
// own destination, issued by the row of the derived relation that produced the
// route. The exact form is the one-row case of the same statement, and an
// identity carry is the trivial closure at every row - which is why it needs no
// vector at all and emits the plain routed fold.
//
// The laws below are stated over the declaration, because that is where the
// difference lives: one clause names who issues the transition, and the emitted
// family either takes a closure per row or is refused by name.

// routedCarryTransform is a transition issued by the ROUTE relation's subject:
// the row a routed publication publishes at.
func routedCarryTransform() definition.CarryTransform {
	return definition.CarryTransform{
		Name: "RouteCarry", Key: "transform/placement/route",
		Candidate: "RouteCarrier", Input: "PlacementFact", Output: "PlacementFact",
		Implementation: specimenMethod("Aged", "Route", 0),
	}
}

// candidateCarryTransform is the same transition issued by the CANDIDATE: the
// shape a routed publication has no answer for.
func candidateCarryTransform() definition.CarryTransform {
	return definition.CarryTransform{
		Name: "CandidateCarry", Key: "transform/placement/candidate",
		Candidate: "ReturnBoundaryCarrier", Input: "PlacementFact", Output: "PlacementFact",
		Implementation: specimenMethod("Aged", "ReturnBoundary", 0),
	}
}

// foreignFactCarryTransform maps something other than the written fact.
func foreignFactCarryTransform() definition.CarryTransform {
	return definition.CarryTransform{
		Name: "ForeignCarry", Key: "transform/placement/foreign",
		Candidate: "RouteCarrier", Input: "ValueFactCarrier", Output: "ValueFactCarrier",
		Implementation: specimenMethod("Aged", "Route", 0),
	}
}

// routedCarryRoster is the member-set roster with the three carry transitions
// declared on the written axis.
func routedCarryRoster(t testing.TB) definition.Roster {
	t.Helper()
	placement := memberSetPlacementDefinition()
	placement.CarryTransforms = []definition.CarryTransform{
		routedCarryTransform(), candidateCarryTransform(), foreignFactCarryTransform(),
	}
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "membersetvalue", Name: "membersetvalue", Base: memberSetValueDefinition()},
		definition.Source{
			Package: "membersetplacement", Name: "membersetplacement",
			Base:          placement,
			Contributions: []definition.Contribution{memberSetPlacementContribution()},
		},
	)
	if !rosterOK {
		t.Fatal("routed-carry roster is not admissible")
	}
	return roster
}

// renderRoutedCarry emits the member-set family with one clause changed: the
// carry its routed output declares.
func renderRoutedCarry(t testing.TB, carry *program.CarryDecl) (string, error) {
	t.Helper()
	spec := memberSetSpec()
	spec.Program.Carry = carry
	target := memberSetTarget()
	target.Spec = spec
	source, err := Render(target, routedCarryRoster(t))
	return string(source), err
}

func routedCarryDecl(mode program.CarryMode, transform string, input uint64) *program.CarryDecl {
	decl := &program.CarryDecl{Input: program.InputRef(input), Mode: mode}
	if mode == program.CarryTransform {
		decl.Transform = member.CarryTransformRef{Axis: memberSetPlacementAxisRef(), Member: schema.Key(transform)}
	}
	return decl
}

// TestRoutedCarryIsTakenOverEachPublishedRow states the form: a routed
// publication whose carry is issued by the route relation's own subject takes
// one closure per published row, assigned from the row the relation answered,
// and folds through the routed carry.
func TestRoutedCarryIsTakenOverEachPublishedRow(t *testing.T) {
	source, err := renderRoutedCarry(t, routedCarryDecl(program.CarryTransform, "transform/placement/route", 0))
	if err != nil {
		t.Fatalf("a routed declaration carrying per row did not emit: %v", err)
	}
	for _, want := range []string{
		"[]execution.RouteCarry[",
		"make([]execution.RouteCarry[",
		"carries := lane.carries[:count]",
		"carries[index] = selected.Aged",
		"execution.FoldSelectedRouteCarry(ticket, row.write, &lane.write, cells, members, routes, carries,",
		"count > len(lane.carries)",
		"if !carryPresent || carryMode != program.CarryTransform {",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("the emitted routed carry does not state %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "execution.FoldSelectedRoute(ticket") {
		t.Fatalf("a carrying routed family still folds through the uncarried routed fold:\n%s", source)
	}
}

// TestRoutedIdentityCarryIsTheTrivialClosure states that an identity carry is a
// declared answer and not a vector: a routed write already leaves every
// coordinate it did not select exactly as its predecessor left it, so the
// trivial closure is what the plain routed fold already performs. The emitted
// fence still restates the declared mode, because a plan row that arrived
// carrying something else is not this declaration.
func TestRoutedIdentityCarryIsTheTrivialClosure(t *testing.T) {
	source, err := renderRoutedCarry(t, routedCarryDecl(program.CarryIdentity, "", 0))
	if err != nil {
		t.Fatalf("a routed declaration with an identity carry did not emit: %v", err)
	}
	if !strings.Contains(source, "execution.FoldSelectedRoute(ticket") {
		t.Fatalf("an identity carry did not emit the plain routed fold:\n%s", source)
	}
	if strings.Contains(source, "RouteCarry[") || strings.Contains(source, "FoldSelectedRouteCarry") {
		t.Fatalf("an identity carry emitted a per-row closure vector:\n%s", source)
	}
	if !strings.Contains(source, "if !carryPresent || carryMode != program.CarryIdentity {") {
		t.Fatalf("the emitted fence does not restate the declared identity carry:\n%s", source)
	}
}

// TestRoutedPublicationWithNoCarryFencesTheAbsence states the third declared
// answer: a routed rule that names no carry refuses a plan row that carries.
func TestRoutedPublicationWithNoCarryFencesTheAbsence(t *testing.T) {
	source, err := renderRoutedCarry(t, nil)
	if err != nil {
		t.Fatalf("a routed declaration with no carry did not emit: %v", err)
	}
	if !strings.Contains(source, "if carryPresent || carryMode != 0 {") {
		t.Fatalf("the emitted fence does not refuse an undeclared carry:\n%s", source)
	}
}

// TestRoutedCarryRefusesADeclarationItCannotIndex states the three shapes that
// have no per-row closure. A transition issued by the CANDIDATE is the one the
// form exists to answer, and it is refused by its own name rather than by a
// generic mismatch: a candidate that publishes at derived destinations has one
// image and one transition per row, not one of each.
func TestRoutedCarryRefusesADeclarationItCannotIndex(t *testing.T) {
	cases := []struct {
		name   string
		carry  *program.CarryDecl
		clause string
	}{
		{
			name:   "indexed-by-the-candidate",
			carry:  routedCarryDecl(program.CarryTransform, "transform/placement/candidate", 0),
			clause: "a routed carry indexed by the candidate",
		},
		{
			name:   "does-not-map-the-written-fact",
			carry:  routedCarryDecl(program.CarryTransform, "transform/placement/foreign", 0),
			clause: "a routed carry that does not map the written fact",
		},
		{
			name:   "at-an-input-the-route-is-not-read-at",
			carry:  routedCarryDecl(program.CarryTransform, "transform/placement/route", 3),
			clause: "a routed carry at an input its route is not read at",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source, err := renderRoutedCarry(t, test.carry)
			if err == nil {
				t.Fatalf("a routed carry %s emitted a family:\n%s", test.name, source)
			}
			if !strings.Contains(err.Error(), test.clause) {
				t.Fatalf("the refusal does not name its clause %q: %v", test.clause, err)
			}
		})
	}
}
