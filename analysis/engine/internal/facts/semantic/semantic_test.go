package semantic

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type semanticFactor uint64
type semanticKey uint64

const semanticColumn semanticFactor = 1

type semanticFixture struct {
	diagram  *diagram.Diagram[semanticFactor, semanticKey, uint8]
	values   *terminal.Arena[uint8]
	all      support.Mask
	atom     support.Mask
	notAtom  support.Mask
	atom2    support.Mask
	notAtom2 support.Mask
	ids      map[uint8]terminal.ID[uint8]
}

func newSemanticFixture(t testing.TB) semanticFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	all := regions.True()
	atom, ok := regions.Literal(1, true)
	if !ok {
		t.Fatal("atom mask creation failed")
	}
	notAtom, ok := regions.Not(atom)
	if !ok {
		t.Fatal("not-atom mask creation failed")
	}
	atom2, ok := regions.Literal(2, true)
	if !ok {
		t.Fatal("second atom mask creation failed")
	}
	notAtom2, ok := regions.Not(atom2)
	if !ok || !regions.Seal() {
		t.Fatal("support seal failed")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena creation failed")
	}
	ids := make(map[uint8]terminal.ID[uint8])
	for _, value := range []uint8{0, 1, 2, 3, 5, 7, 10, 20, 30, 40, 60} {
		id, admitted := values.Admit(value)
		if !admitted {
			t.Fatalf("terminal admission %d failed", value)
		}
		ids[value] = id
	}
	if !values.Seal() {
		t.Fatal("terminal seal failed")
	}
	facts, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{
		Factors:   []semanticFactor{semanticColumn},
		Terminals: values,
		Guards:    manager,
	})
	if !ok {
		t.Fatal("diagram creation failed")
	}
	return semanticFixture{diagram: facts, values: values, all: all, atom: atom, notAtom: notAtom, atom2: atom2, notAtom2: notAtom2, ids: ids}
}

func (fixture semanticFixture) root(t testing.TB, writes ...struct {
	when  support.Mask
	value uint8
}) diagram.Root[semanticFactor, semanticKey, uint8] {
	t.Helper()
	builder := fixture.diagram.Begin()
	root := fixture.diagram.Empty()
	for _, write := range writes {
		var ok bool
		root, ok = builder.Set(root, semanticColumn, 7, write.when, fixture.ids[write.value])
		if !ok {
			t.Fatalf("fact write %d failed", write.value)
		}
	}
	sealed, ok := builder.Seal(root)
	if !ok {
		t.Fatal("root seal failed")
	}
	return sealed
}

func (fixture semanticFixture) at(t testing.TB, plane Plane[semanticFactor, semanticKey, uint8], atom bool) (uint8, bool) {
	t.Helper()
	id, present, valid := fixture.diagram.At(plane.Root(), semanticColumn, 7, func(guard.Atom) bool { return atom })
	if !valid || !present {
		return 0, false
	}
	value, valid := fixture.values.Value(id)
	if !valid {
		t.Fatal("result terminal was not readable through the original semantic owner")
	}
	return value, true
}

func (fixture semanticFixture) at2(t testing.TB, plane Plane[semanticFactor, semanticKey, uint8], first, second bool) (uint8, bool) {
	t.Helper()
	id, present, valid := fixture.diagram.At(plane.Root(), semanticColumn, 7, func(atom guard.Atom) bool {
		if atom == 1 {
			return first
		}
		return second
	})
	if !valid || !present {
		return 0, false
	}
	value, valid := fixture.values.Value(id)
	if !valid {
		t.Fatal("result terminal was not readable through the original semantic owner")
	}
	return value, true
}

func max(left, right uint8) (uint8, bool) {
	if left >= right {
		return left, true
	}
	return right, true
}

func TestNewRejectsNonSoleFactorDiagram(t *testing.T) {
	fixture := newSemanticFixture(t)
	multi, ok := diagram.New(diagram.Config[semanticFactor, semanticKey, uint8]{
		Factors:   []semanticFactor{semanticColumn, semanticColumn + 1},
		Terminals: fixture.values,
		Guards:    fixture.diagram.Guards(),
	})
	if !ok {
		t.Fatal("multi-factor diagram setup")
	}
	if _, ok := New(multi, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		LessOrEq: func(left, right uint8) bool { return left <= right },
	}); ok {
		t.Fatal("semantic domain accepted a non-sole diagram")
	}
}

func splitFor(t testing.TB, left, right support.Mask) support.Split {
	t.Helper()
	split, ok := support.Three(left, right)
	if !ok {
		t.Fatal("support split")
	}
	return split
}

func joinFor(t testing.TB, domain *Domain[semanticFactor, semanticKey, uint8], left, right Plane[semanticFactor, semanticKey, uint8], leftSupport, rightSupport support.Mask) (Plane[semanticFactor, semanticKey, uint8], bool) {
	t.Helper()
	return domain.JoinUnder(left, right, splitFor(t, leftSupport, rightSupport), diagram.NewSoleScratch[semanticKey, uint8](), nil, nil)
}

func widenAllKeysFor(t testing.TB, domain *Domain[semanticFactor, semanticKey, uint8], left, right Plane[semanticFactor, semanticKey, uint8], leftSupport, rightSupport support.Mask) (Plane[semanticFactor, semanticKey, uint8], bool) {
	t.Helper()
	return domain.WidenUnderKeys(left, right, splitFor(t, leftSupport, rightSupport), diagram.NewSoleScratch[semanticKey, uint8](), nil, nil, func(semanticKey) bool { return true })
}

func narrowFor(t testing.TB, domain *Domain[semanticFactor, semanticKey, uint8], left, right Plane[semanticFactor, semanticKey, uint8], leftSupport, rightSupport support.Mask) (Plane[semanticFactor, semanticKey, uint8], bool) {
	t.Helper()
	return domain.NarrowUnder(left, right, splitFor(t, leftSupport, rightSupport), diagram.NewSoleScratch[semanticKey, uint8](), nil, nil)
}

func intersectFor(t testing.TB, left, right support.Mask) support.Mask {
	t.Helper()
	work := support.New(left.Manager())
	if work == nil {
		t.Fatal("support work")
	}
	result, ok := work.And(left, right)
	if !ok || !work.Seal() {
		t.Fatal("support intersection")
	}
	return result
}

func TestFusedJoinKeepsSimultaneousLeftRightAndOverlapRegionsExact(t *testing.T) {
	fixture := newSemanticFixture(t)
	leftOnly := intersectFor(t, fixture.atom, fixture.notAtom2)
	rightOnly := intersectFor(t, fixture.notAtom, fixture.atom2)
	overlap := intersectFor(t, fixture.atom, fixture.atom2)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(_, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain")
	}
	left, ok := domain.Plane(fixture.root(t,
		struct {
			when  support.Mask
			value uint8
		}{leftOnly, 10},
		struct {
			when  support.Mask
			value uint8
		}{overlap, 20},
	))
	if !ok {
		t.Fatal("left")
	}
	right, ok := domain.Plane(fixture.root(t,
		struct {
			when  support.Mask
			value uint8
		}{rightOnly, 30},
		struct {
			when  support.Mask
			value uint8
		}{overlap, 40},
	))
	if !ok {
		t.Fatal("right")
	}
	joined, ok := joinFor(t, domain, left, right, fixture.atom, fixture.atom2)
	if !ok {
		t.Fatal("join")
	}
	for _, check := range []struct {
		first, second bool
		want          uint8
		present       bool
	}{
		{true, false, 10, true},
		{false, true, 30, true},
		{true, true, 40, true},
		{false, false, 0, false},
	} {
		got, present := fixture.at2(t, joined, check.first, check.second)
		if present != check.present || present && got != check.want {
			t.Fatalf("joined(%t,%t) = %d/%t, want %d/%t", check.first, check.second, got, present, check.want, check.present)
		}
	}
}

func TestPreserveRejectsOverlapMismatch(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(_, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain")
	}
	left, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 10}))
	if !ok {
		t.Fatal("left")
	}
	right, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 20}))
	if !ok {
		t.Fatal("right")
	}
	if _, ok := domain.PreserveUnder(left, right, splitFor(t, fixture.all, fixture.all), diagram.NewSoleScratch[semanticKey, uint8](), nil, nil); ok {
		t.Fatal("preserve accepted an overlap mismatch")
	}
}

func TestJoinPreservesOneSidedSupportAndCombinesOverlap(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  5,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	left, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 10}))
	if !ok {
		t.Fatal("left carrier failed")
	}
	right, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 20}))
	if !ok {
		t.Fatal("right carrier failed")
	}
	joined, ok := joinFor(t, domain, left, right, fixture.all, fixture.atom)
	if !ok {
		t.Fatal("join failed")
	}
	if got, present := fixture.at(t, joined, true); !present || got != 20 {
		t.Fatalf("overlap join = %d/%t, want 20/true", got, present)
	}
	if got, present := fixture.at(t, joined, false); !present || got != 10 {
		t.Fatalf("one-sided join = %d/%t, want retained 10/true", got, present)
	}
}

func TestJoinDoesNotCarryAcrossDisjointOuterSupport(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  5,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	left, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 10}))
	if !ok {
		t.Fatal("left plane")
	}
	right, ok := domain.Plane(fixture.diagram.Empty())
	if !ok {
		t.Fatal("right plane")
	}
	joined, ok := joinFor(t, domain, left, right, fixture.atom, fixture.notAtom)
	if !ok {
		t.Fatal("join failed")
	}
	if _, present := fixture.at(t, joined, false); present {
		t.Fatal("left fact escaped its disjoint outer support")
	}
	if got, present := fixture.at(t, joined, true); !present || got != 10 {
		t.Fatalf("left fact at its support = %d/%t, want 10/true", got, present)
	}
}

func TestJoinRejectsResultThatIsNotAnUpperBound(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     func(left, _ uint8) (uint8, bool) { return left, true },
		Widen:    max,
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	left, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 10}))
	if !ok {
		t.Fatal("left plane")
	}
	right, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 20}))
	if !ok {
		t.Fatal("right plane")
	}
	if _, ok := joinFor(t, domain, left, right, fixture.all, fixture.all); ok {
		t.Fatal("Join accepted a result below its right operand")
	}
	split, ok := support.Three(fixture.all, fixture.all)
	if !ok {
		t.Fatal("split")
	}
	if _, ok := domain.JoinUnder(left, right, split, diagram.NewSoleScratch[semanticKey, uint8](), nil, nil); ok {
		t.Fatal("JoinUnder accepted a result below its right operand")
	}
}

func TestWidenOfEqualOverlappingRootsPreservesDenotation(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	plane, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 10}))
	if !ok {
		t.Fatal("carrier creation failed")
	}
	widened, ok := widenAllKeysFor(t, domain, plane, plane, fixture.all, fixture.all)
	if !ok {
		t.Fatal("equal-root widen failed")
	}
	if !domain.EqualUnder(plane, widened, fixture.all, diagram.NewSoleScratch[semanticKey, uint8]()) {
		t.Fatal("equal-root widen changed the plane denotation")
	}
	if got, present := fixture.at(t, widened, true); !present || got != 10 {
		t.Fatalf("widen result = %d/%t, want 10/true", got, present)
	}
}

func TestNarrowOfEqualOverlappingRootsPreservesDenotation(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, _ uint8) (uint8, bool) { return left, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	plane, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 10}))
	if !ok {
		t.Fatal("carrier creation failed")
	}
	narrowed, ok := narrowFor(t, domain, plane, plane, fixture.all, fixture.all)
	if !ok {
		t.Fatal("equal-root narrow failed")
	}
	if !domain.EqualUnder(plane, narrowed, fixture.all, diagram.NewSoleScratch[semanticKey, uint8]()) {
		t.Fatal("equal-root narrow changed the plane denotation")
	}
}

func TestFusedJoinOfEqualPlanesPreservesDenotation(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(_, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain")
	}
	plane, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 10}))
	if !ok {
		t.Fatal("plane")
	}
	scratch := diagram.NewSoleScratch[semanticKey, uint8]()
	joined, ok := domain.JoinUnder(plane, plane, splitFor(t, fixture.all, fixture.all), scratch, nil, nil)
	if !ok {
		t.Fatal("fused join failed")
	}
	if !domain.EqualUnder(plane, joined, fixture.all, diagram.NewSoleScratch[semanticKey, uint8]()) {
		t.Fatal("fused join of equal planes changed the denotation")
	}
}

func TestNarrowRequiresSubsetAndDropsLeftOnlySupport(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default: 0,
		Equal:   func(left, right uint8) bool { return left == right },
		Join:    max,
		Widen:   max,
		Narrow: func(left, right uint8) (uint8, bool) {
			if right < left {
				return right, true
			}
			return left, true
		},
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	left, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 10}))
	if !ok {
		t.Fatal("left carrier failed")
	}
	right, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 5}))
	if !ok {
		t.Fatal("right carrier failed")
	}
	narrowed, ok := narrowFor(t, domain, left, right, fixture.all, fixture.atom)
	if !ok {
		t.Fatal("subset narrow failed")
	}
	if got, present := fixture.at(t, narrowed, true); !present || got != 5 {
		t.Fatalf("narrow overlap = %d/%t, want 5/true", got, present)
	}
	if _, present := fixture.at(t, narrowed, false); present {
		t.Fatal("narrow retained left-only support")
	}
	if _, ok := narrowFor(t, domain, right, left, fixture.atom, fixture.all); ok {
		t.Fatal("narrow accepted non-subset support")
	}
	tooWide, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 20}))
	if !ok {
		t.Fatal("non-refining right plane failed")
	}
	if _, ok := narrowFor(t, domain, left, tooWide, fixture.all, fixture.atom); ok {
		t.Fatal("narrow accepted subset support with a non-refining terminal")
	}
}

func TestMuExistentiallyClosesSupportAndTypedCofactors(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	root := fixture.root(t,
		struct {
			when  support.Mask
			value uint8
		}{fixture.notAtom, 1},
		struct {
			when  support.Mask
			value uint8
		}{fixture.atom, 2},
	)
	input, ok := domain.Plane(root)
	if !ok {
		t.Fatal("input carrier failed")
	}
	closed, ok := domain.Mu(input, fixture.all, 1)
	if !ok {
		t.Fatal("Mu failed")
	}
	for _, valuation := range []bool{false, true} {
		if got, present := fixture.at(t, closed, valuation); !present || got != 2 {
			t.Fatalf("Mu cofactor at %t = %d/%t, want 2/true", valuation, got, present)
		}
	}
}

func TestJoinConfluenceAcrossParenthesization(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  0,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	makePlane := func(value uint8) Plane[semanticFactor, semanticKey, uint8] {
		plane, accepted := domain.Plane(fixture.root(t, struct {
			when  support.Mask
			value uint8
		}{fixture.all, value}))
		if !accepted {
			t.Fatal("carrier failed")
		}
		return plane
	}
	first, second, third := makePlane(1), makePlane(2), makePlane(3)
	left, ok := joinFor(t, domain, first, second, fixture.all, fixture.all)
	if !ok {
		t.Fatal("first parenthesization stage failed")
	}
	left, ok = joinFor(t, domain, left, third, fixture.all, fixture.all)
	if !ok {
		t.Fatal("first parenthesization failed")
	}
	right, ok := joinFor(t, domain, second, third, fixture.all, fixture.all)
	if !ok {
		t.Fatal("second parenthesization stage failed")
	}
	right, ok = joinFor(t, domain, first, right, fixture.all, fixture.all)
	if !ok {
		t.Fatal("second parenthesization failed")
	}
	for _, valuation := range []bool{false, true} {
		leftValue, leftPresent := fixture.at(t, left, valuation)
		rightValue, rightPresent := fixture.at(t, right, valuation)
		if !leftPresent || !rightPresent || leftValue != rightValue || leftValue != 3 {
			t.Fatalf("join confluence at %t = %d/%t versus %d/%t, want 3/true", valuation, leftValue, leftPresent, rightValue, rightPresent)
		}
	}
}

func TestNonBottomDefaultParticipatesInOverlapOperations(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  5,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	left, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 10}))
	if !ok {
		t.Fatal("left plane")
	}
	right, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 7}))
	if !ok {
		t.Fatal("right plane")
	}
	joined, ok := joinFor(t, domain, left, right, fixture.all, fixture.all)
	if !ok {
		t.Fatal("non-bottom join failed")
	}
	if got, present := fixture.at(t, joined, false); !present || got != 7 {
		t.Fatalf("Default join right = %d/%t, want 7/true", got, present)
	}
	if got, present := fixture.at(t, joined, true); !present || got != 10 {
		t.Fatalf("left join right = %d/%t, want 10/true", got, present)
	}

	defaultLeft, ok := domain.Plane(fixture.diagram.Empty())
	if !ok {
		t.Fatal("empty left plane")
	}
	narrowRight, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 3}))
	if !ok {
		t.Fatal("narrow right plane")
	}
	narrowed, ok := narrowFor(t, domain, defaultLeft, narrowRight, fixture.all, fixture.all)
	if !ok {
		t.Fatal("narrow from typed Default was rejected")
	}
	for _, valuation := range []bool{false, true} {
		if got, present := fixture.at(t, narrowed, valuation); !present || got != 3 {
			t.Fatalf("Default narrow at %t = %d/%t, want 3/true", valuation, got, present)
		}
	}
}

func TestMuCarriesOneSidedSupportCofactorWithoutInventingDefault(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  5,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	input, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 10}))
	if !ok {
		t.Fatal("input plane")
	}
	closed, ok := domain.Mu(input, fixture.atom, 1)
	if !ok {
		t.Fatal("Mu failed")
	}
	for _, valuation := range []bool{false, true} {
		if got, present := fixture.at(t, closed, valuation); !present || got != 10 {
			t.Fatalf("one-sided Mu at %t = %d/%t, want 10/true", valuation, got, present)
		}
	}
}

func TestMuJoinsTypedDefaultOnlyWhenBothCofactorsSupported(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  5,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		Narrow:   func(left, right uint8) (uint8, bool) { return right, true },
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	input, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 10}))
	if !ok {
		t.Fatal("input plane")
	}
	closed, ok := domain.Mu(input, fixture.all, 1)
	if !ok {
		t.Fatal("Mu failed")
	}
	for _, valuation := range []bool{false, true} {
		if got, present := fixture.at(t, closed, valuation); !present || got != 10 {
			t.Fatalf("Default/explicit Mu at %t = %d/%t, want 10/true", valuation, got, present)
		}
	}
}

func TestSummaryFoldsSymbolicLeavesAndRespectsOuterSupport(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:  5,
		Equal:    func(left, right uint8) bool { return left == right },
		Join:     max,
		Widen:    max,
		LessOrEq: func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	input, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 10}))
	if !ok {
		t.Fatal("input plane")
	}
	if got, present := domain.Summary(input, 7); !present || got != 10 {
		t.Fatalf("unrestricted summary = %d/%t, want 10/true", got, present)
	}
	if got, present := domain.SummaryUnder(input, 7, fixture.atom); !present || got != 10 {
		t.Fatalf("supported summary = %d/%t, want 10/true", got, present)
	}
	if _, present := domain.SummaryUnder(input, 7, fixture.notAtom); !present {
		t.Fatal("supported Default summary was absent")
	}
}

func TestPartitionKeyPreservesStoredAndAbsentTerminals(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:     5,
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join:        max,
		Widen:       max,
		LessOrEq:    func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain")
	}
	storedRoot := fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.all, 5})
	stored, ok := domain.Plane(storedRoot)
	if !ok {
		t.Fatal("stored plane")
	}
	type piece struct {
		value   uint8
		present bool
		region  support.Mask
	}
	pieces := make([]piece, 0, 2)
	if !domain.PartitionKey(stored, 7, fixture.all, func(value uint8, present bool, region support.Mask) bool {
		pieces = append(pieces, piece{value: value, present: present, region: region})
		return true
	}) || len(pieces) != 1 || !pieces[0].present || pieces[0].value != 5 || !pieces[0].region.Equal(fixture.all) {
		t.Fatal("stored Default was not retained as present")
	}
	empty, ok := domain.Empty()
	if !ok {
		t.Fatal("empty plane")
	}
	pieces = pieces[:0]
	if !domain.PartitionKey(empty, 7, fixture.all, func(value uint8, present bool, region support.Mask) bool {
		pieces = append(pieces, piece{value: value, present: present, region: region})
		return true
	}) || len(pieces) != 1 || pieces[0].present || pieces[0].value != 5 || !pieces[0].region.Equal(fixture.all) {
		t.Fatal("sparse Default was not retained as absent")
	}
}

func TestPartitionRetainsOneJointSymbolicTerminalTuple(t *testing.T) {
	fixture := newSemanticFixture(t)
	domain, ok := New(fixture.diagram, fixture.values, Operations[uint8]{
		Default:     5,
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
		Join:        max,
		Widen:       max,
		LessOrEq:    func(left, right uint8) bool { return left <= right },
	})
	if !ok {
		t.Fatal("domain creation failed")
	}
	input, ok := domain.Plane(fixture.root(t, struct {
		when  support.Mask
		value uint8
	}{fixture.atom, 10}))
	if !ok {
		t.Fatal("input plane")
	}
	cells := make([]support.Mask, 0, 2)
	if !domain.Partition(input, fixture.all, func(cell support.Mask) bool {
		cells = append(cells, cell)
		return true
	}) || len(cells) != 2 {
		t.Fatalf("partition cells = %d, want two exact atom cells", len(cells))
	}
	for _, valuation := range []bool{false, true} {
		matches := 0
		for _, cell := range cells {
			if cell.Matches(func(guard.Atom) bool { return valuation }) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("valuation %t belongs to %d cells, want one", valuation, matches)
		}
	}
	if !domain.EqualAt(input, func(guard.Atom) bool { return true }, input, func(guard.Atom) bool { return true }) {
		t.Fatal("equal representative rows were rejected")
	}
	if domain.EqualAt(input, func(guard.Atom) bool { return true }, input, func(guard.Atom) bool { return false }) {
		t.Fatal("different representative rows were accepted")
	}
	if _, ok := domain.FingerprintAt(input, func(guard.Atom) bool { return true }); !ok {
		t.Fatal("row fingerprint was unavailable")
	}
}
