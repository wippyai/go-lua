package semantic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type recurrenceFixture struct {
	manager *guard.Manager
	values  *terminal.Arena[uint8]
	domain  *Domain[semanticFactor, semanticKey, uint8]
	facts   *diagram.Diagram[semanticFactor, semanticKey, uint8]
	all     support.Mask
	ids     map[uint8]terminal.ID[uint8]
}

func newRecurrenceFixture(t testing.TB) recurrenceFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	all := regions.True()
	if !regions.Seal() {
		t.Fatal("support seal")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	ids := make(map[uint8]terminal.ID[uint8])
	for _, value := range []uint8{0, 1, 2, 4} {
		id, admitted := values.Admit(value)
		if !admitted {
			t.Fatalf("admit %d", value)
		}
		ids[value] = id
	}
	if !values.Seal() {
		t.Fatal("terminal seal")
	}
	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{
		Factors:   []semanticFactor{semanticColumn},
		Terminals: values,
		Guards:    manager,
	})
	if !ok {
		t.Fatal("diagram")
	}
	join := func(left, right uint8) (uint8, bool) { return left | right, true }
	domain, ok := New(facts, values, Operations[uint8]{
		Default:     0,
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join:        join,
		Widen:       join,
		Narrow:      func(_, right uint8) (uint8, bool) { return right, true },
		LessOrEq:    func(left, right uint8) bool { return left&right == left },
	})
	if !ok {
		t.Fatal("domain")
	}
	return recurrenceFixture{manager: manager, values: values, domain: domain, facts: facts, all: all, ids: ids}
}

func (fixture recurrenceFixture) plane(t testing.TB, key semanticKey, value uint8) Plane[semanticFactor, semanticKey, uint8] {
	t.Helper()
	builder := fixture.facts.Begin()
	if builder == nil {
		t.Fatal("plane builder")
	}
	root, written := builder.Set(fixture.facts.Empty(), semanticColumn, key, fixture.all, fixture.ids[value])
	if !written {
		t.Fatal("plane write")
	}
	root, written = builder.Seal(root)
	if !written {
		t.Fatal("plane seal")
	}
	plane, valid := fixture.domain.Plane(root)
	if !valid {
		t.Fatal("plane")
	}
	return plane
}

// population counts the whole sealed terminal universe of the owner.
func (fixture recurrenceFixture) population(t testing.TB) int {
	t.Helper()
	count := 0
	if !fixture.values.Every(func(uint8) bool { count++; return true }) {
		t.Fatal("sealed terminal audit failed")
	}
	return count
}

// A recurrence step whose semantic result did not advance must republish the
// exact predecessor root pointer and admit no terminal.  Canonical sealed
// interning is what makes the pointer answer an honest identity answer: an
// equal join output resolves to the terminal the predecessor already holds.
func TestJoinContributionsAtFixedPointRepublishesThePredecessorRoot(t *testing.T) {
	fixture := newRecurrenceFixture(t)
	scratch := diagram.NewSoleScratch[semanticKey, uint8]()
	regions := support.New(fixture.manager)
	if regions == nil {
		t.Fatal("regions")
	}
	report := func(semanticKey, support.Mask) bool { return true }
	covers := func(key semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
		return fixture.all, fixture.all, fixture.all, key == 7
	}

	left, right := fixture.plane(t, 7, 1), fixture.plane(t, 7, 2)
	first, ok := fixture.domain.JoinContributions(left, right, scratch, regions, report, covers)
	if !ok {
		t.Fatal("first join")
	}
	population := fixture.population(t)

	second, ok := fixture.domain.JoinContributions(first, right, scratch, regions, report, covers)
	if !ok {
		t.Fatal("fixed-point join")
	}
	if !fixture.domain.Same(first, second) {
		t.Fatal("fixed-point join changed the semantic plane")
	}
	if second.Root() != first.Root() {
		t.Fatal("fixed-point join republished a distinct root pointer for an unchanged plane")
	}
	if grown := fixture.population(t); grown != population {
		t.Fatalf("sealed terminal universe grew from %d to %d at a fixed point", population, grown)
	}
	third, ok := fixture.domain.JoinContributions(second, right, scratch, regions, report, covers)
	if !ok {
		t.Fatal("repeated fixed-point join")
	}
	if third.Root() != first.Root() {
		t.Fatal("repeated fixed-point join drifted from the predecessor root")
	}
}

// Two planes built by independent transactions over equal inputs carry one
// terminal identity per semantic value, so whole-plane equality is answered
// by identity rather than by a structural value walk.
func TestIndependentlyBuiltEqualPlanesShareTerminalIdentity(t *testing.T) {
	fixture := newRecurrenceFixture(t)
	regions := support.New(fixture.manager)
	if regions == nil {
		t.Fatal("regions")
	}
	report := func(semanticKey, support.Mask) bool { return true }
	covers := func(key semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
		return fixture.all, fixture.all, fixture.all, key == 7
	}
	build := func() Plane[semanticFactor, semanticKey, uint8] {
		t.Helper()
		left, right := fixture.plane(t, 7, 1), fixture.plane(t, 7, 2)
		plane, ok := fixture.domain.JoinContributions(left, right, diagram.NewSoleScratch[semanticKey, uint8](), regions, report, covers)
		if !ok {
			t.Fatal("independent join")
		}
		return plane
	}
	first := build()
	population := fixture.population(t)
	second := build()

	if !fixture.domain.Same(first, second) {
		t.Fatal("independent equal planes were not semantically equal")
	}
	firstID, present, valid := fixture.facts.At(first.Root(), semanticColumn, 7, func(guard.Atom) bool { return true })
	secondID, secondPresent, secondValid := fixture.facts.At(second.Root(), semanticColumn, 7, func(guard.Atom) bool { return true })
	if !valid || !present || !secondValid || !secondPresent {
		t.Fatal("plane read")
	}
	if firstID != secondID {
		t.Fatalf("independent equal planes carry distinct terminal identities %v and %v", firstID, secondID)
	}
	if grown := fixture.population(t); grown != population {
		t.Fatalf("sealed terminal universe grew from %d to %d for an already interned value", population, grown)
	}
}

// EqualUnder must agree with the semantic verdict on planes that reached the
// same value along different routes; a shared root is the fast answer, never
// a different answer.
func TestEqualUnderAgreesWithSemanticVerdictAcrossRoutes(t *testing.T) {
	fixture := newRecurrenceFixture(t)
	regions := support.New(fixture.manager)
	if regions == nil {
		t.Fatal("regions")
	}
	report := func(semanticKey, support.Mask) bool { return true }
	covers := func(key semanticKey) (support.Mask, support.Mask, support.Mask, bool) {
		return fixture.all, fixture.all, fixture.all, key == 7
	}
	one, two, four := fixture.plane(t, 7, 1), fixture.plane(t, 7, 2), fixture.plane(t, 7, 4)

	direct, ok := fixture.domain.JoinContributions(fixture.plane(t, 7, 1), fixture.plane(t, 7, 2), diagram.NewSoleScratch[semanticKey, uint8](), regions, report, covers)
	if !ok {
		t.Fatal("direct join")
	}
	routed, ok := fixture.domain.JoinContributions(two, one, diagram.NewSoleScratch[semanticKey, uint8](), regions, report, covers)
	if !ok {
		t.Fatal("routed join")
	}
	scratch := diagram.NewSoleScratch[semanticKey, uint8]()
	if !fixture.domain.EqualUnder(direct, routed, fixture.all, scratch) {
		t.Fatal("commuted join routes disagreed under EqualUnder")
	}
	if !fixture.domain.Same(direct, routed) {
		t.Fatal("commuted join routes disagreed under Same")
	}
	directID, present, valid := fixture.facts.At(direct.Root(), semanticColumn, 7, func(guard.Atom) bool { return true })
	routedID, routedPresent, routedValid := fixture.facts.At(routed.Root(), semanticColumn, 7, func(guard.Atom) bool { return true })
	if !valid || !present || !routedValid || !routedPresent {
		t.Fatal("route read")
	}
	if directID != routedID {
		t.Fatalf("commuted join routes carry distinct terminal identities %v and %v", directID, routedID)
	}
	advanced, ok := fixture.domain.JoinContributions(direct, four, diagram.NewSoleScratch[semanticKey, uint8](), regions, report, covers)
	if !ok {
		t.Fatal("advancing join")
	}
	if fixture.domain.EqualUnder(direct, advanced, fixture.all, scratch) || fixture.domain.Same(direct, advanced) {
		t.Fatal("an advanced plane was reported equal to its predecessor")
	}
	if advanced.Root() == direct.Root() {
		t.Fatal("an advanced plane retained the predecessor root pointer")
	}
}
