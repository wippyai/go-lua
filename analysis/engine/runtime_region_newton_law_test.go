package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// newtonLawFixture is the synthetic factor-free recurrence shape: one carrier
// Composition with no Factor operation over a guard universe of the requested
// width, so a Region's whole observable content is its support region and its
// whole operator is its coordinate relations.
type newtonLawFixture struct {
	composition *carrier.Composition
	scope       carrier.Scope
	whole       support.Mask
	coordinates []guard.Atom
	literals    map[guard.Atom]support.Mask
}

func newNewtonLawFixture(t testing.TB, width int) newtonLawFixture {
	t.Helper()
	coordinates := make([]guard.Atom, width)
	for index := range coordinates {
		coordinates[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(coordinates)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("newton-law whole support")
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("newton-law support work")
	}
	literals := make(map[guard.Atom]support.Mask, width)
	for _, atom := range coordinates {
		mask, literal := regions.Literal(atom, true)
		if !literal {
			t.Fatalf("newton-law literal %d", atom)
		}
		literals[atom] = mask
	}
	if !regions.Seal() {
		t.Fatal("newton-law literal seal")
	}
	prepared, ok := carrier.PrepareComposition(nil, manager)
	if !ok || prepared == nil {
		t.Fatal("newton-law prepared composition")
	}
	composition, ok := prepared.Attach()
	if !ok || composition == nil {
		t.Fatal("newton-law composition")
	}
	return newtonLawFixture{composition: composition, scope: composition.Scope(), whole: whole, coordinates: coordinates, literals: literals}
}

// transport seals one relation over the fixture's complete scope from an
// explicit source-to-target coordinate map. It is the Region's back transport
// in the factor-free shape.
func (fixture newtonLawFixture) transport(t testing.TB, mapping map[guard.Atom]guard.Atom) carrier.ReindexPlan {
	t.Helper()
	builder, ok := fixture.composition.NewReindex(fixture.scope, fixture.scope)
	if !ok || builder == nil {
		t.Fatal("newton-law relation builder")
	}
	for _, source := range fixture.coordinates {
		target, named := mapping[source]
		if !named {
			t.Fatalf("newton-law relation leaves coordinate %d unnamed", source)
		}
		if source == target {
			if !builder.Identity(source) {
				t.Fatalf("newton-law relation identity %d", source)
			}
			continue
		}
		expression, expressed := fixture.scope.Expr(fixture.literals[target])
		if !expressed || !builder.Set(source, expression) {
			t.Fatalf("newton-law relation %d -> %d", source, target)
		}
	}
	plan, sealed := builder.Seal()
	if !sealed || !plan.Valid() {
		t.Fatal("newton-law relation seal")
	}
	return plan
}

// newtonLawOperator is the Region operator of the factor-free shape: one
// point fold whose base is the constant part and whose environment terms are
// the constant part or the previous iterate moved through the given
// relations. It is the same BeginPointRHSFold/AddPointFoldEnvironment/
// FinishPointRHSFold pass the executor runs at a recurrence head.
func newtonLawOperator(t testing.TB, work *carrier.Work, whole support.Mask, reference carrier.PointState, constant carrier.PointRHS, moved carrier.PointState, plans []carrier.ReindexPlan) carrier.PointRHS {
	t.Helper()
	if !work.BeginPointRHSFold(reference, constant) {
		t.Fatal("newton-law fold did not open")
	}
	for _, plan := range plans {
		term, ok := work.TransportPointState(moved, whole, plan, whole)
		if !ok {
			t.Fatal("newton-law fold term transport")
		}
		if !work.AddPointFoldEnvironment(term) {
			t.Fatal("newton-law fold term")
		}
	}
	result, _, ok := work.FinishPointRHSFold()
	if !ok || !result.Valid() {
		t.Fatal("newton-law fold did not finish")
	}
	return result
}

func newtonLawIdentity(t testing.TB, rhs carrier.PointRHS) guard.FormulaID {
	t.Helper()
	id, ok := rhs.Support().Identity()
	if !ok || !id.Available() {
		t.Fatal("newton-law point identity")
	}
	return id
}

// assertNewtonClosureEqualsKleeneFixpoint is the closure law itself. The
// Newton basis is the saturated set of the Region's back-transport powers;
// joining the constant part moved through every basis member, in one fold
// pass, must land byte-for-byte on the same head the Kleene iteration climbs
// to by applying the whole Region operator one step at a time. The law is
// checked at every height of the chain: only the fixpoint may agree, so a
// closure that under- or over-shoots is caught rather than accidentally
// matching a step it happened to land on.
func assertNewtonClosureEqualsKleeneFixpoint(t *testing.T, fixture newtonLawFixture, generators []carrier.ReindexPlan, wantBasis, wantRounds, wantHeight int) {
	t.Helper()
	newton, ok := saturateRegionNewton(fixture.composition, generators, false)
	if !ok || !newton.available() {
		t.Fatal("newton basis was not sealed for the factor-free shape")
	}
	if len(newton.basis) != wantBasis || newton.rounds != wantRounds {
		t.Fatalf("newton basis size=%d rounds=%d, want size=%d rounds=%d", len(newton.basis), newton.rounds, wantBasis, wantRounds)
	}
	constantState, ok := carrier.NewState(fixture.composition, fixture.scope, fixture.literals[1])
	if !ok {
		t.Fatal("newton-law constant state")
	}
	work, ok := fixture.composition.NewWork()
	if !ok {
		t.Fatal("newton-law work")
	}
	constantPoint, ok := work.EmptyPointState(constantState)
	if !ok {
		t.Fatal("newton-law constant point")
	}
	constant, ok := work.PointRHSFromPointState(constantPoint)
	if !ok {
		t.Fatal("newton-law constant RHS")
	}

	// The Kleene climb: one whole-Region-operator application per step, driven
	// to its fixpoint by byte equality of consecutive heads.
	iterates := []carrier.PointRHS{constant}
	for height := 1; ; height++ {
		if height > 32 {
			t.Fatal("newton-law Kleene chain did not settle")
		}
		previous, ok := work.EmptyPointState(iterates[height-1].State())
		if !ok {
			t.Fatal("newton-law previous iterate")
		}
		next := newtonLawOperator(t, work, fixture.whole, constantPoint, constant, previous, generators)
		iterates = append(iterates, next)
		if newtonLawIdentity(t, next) == newtonLawIdentity(t, iterates[height-1]) {
			break
		}
	}
	// The last iterate repeats its predecessor, so the chain height is the
	// number of steps that actually moved.
	height := len(iterates) - 2
	fixpoint := iterates[len(iterates)-1]

	// The Newton step: one fold pass over the whole saturated basis.
	closure := newtonLawOperator(t, work, fixture.whole, constantPoint, constant, constantPoint, newton.basis)

	if newtonLawIdentity(t, closure) != newtonLawIdentity(t, fixpoint) {
		t.Fatal("newton closure is not byte-identical to the Kleene fixpoint")
	}
	if !work.EqualUnder(closure.State(), fixpoint.State()) {
		t.Fatal("newton closure and the Kleene fixpoint are not the same carrier state")
	}
	// Every strictly lower height must disagree, or the law above proves
	// nothing about the closure actually closing.
	for lower := 0; lower < height; lower++ {
		if newtonLawIdentity(t, iterates[lower]) == newtonLawIdentity(t, closure) {
			t.Fatalf("newton closure already agreed with Kleene height %d", lower)
		}
	}
	if height != wantHeight {
		t.Fatalf("newton-law chain height=%d, want %d", height, wantHeight)
	}
}

// TestRegionNewtonClosureEqualsKleeneFixpoint drives the closure law on one
// generator whose powers are genuinely distinct, so the closure is more than
// the generator itself.
func TestRegionNewtonClosureEqualsKleeneFixpoint(t *testing.T) {
	fixture := newNewtonLawFixture(t, 3)
	// A shift: coordinate 1 becomes 2, 2 becomes 3, 3 is the fixed point.
	shift := fixture.transport(t, map[guard.Atom]guard.Atom{1: 2, 2: 3, 3: 3})
	assertNewtonClosureEqualsKleeneFixpoint(t, fixture, []carrier.ReindexPlan{shift}, 2, 2, 2)
}

// TestRegionNewtonClosureEqualsKleeneFixpointOverManyGenerators drives the
// same law on a Region with two back transports. This is the case the basis
// exists for: the Kleene operator joins both transports at every step, so its
// n-th iterate is the join over every interleaved word of length n, and the
// closure must contain every one of those words rather than only the powers
// of one generator.
func TestRegionNewtonClosureEqualsKleeneFixpointOverManyGenerators(t *testing.T) {
	fixture := newNewtonLawFixture(t, 4)
	// As coordinate maps these are (2,2,3,4) and (1,3,4,4). The monoid they
	// generate has eight members and closes after three squaring rounds, while
	// the Kleene chain over their join climbs three steps.
	first := fixture.transport(t, map[guard.Atom]guard.Atom{1: 2, 2: 2, 3: 3, 4: 4})
	second := fixture.transport(t, map[guard.Atom]guard.Atom{1: 1, 2: 3, 3: 4, 4: 4})
	assertNewtonClosureEqualsKleeneFixpoint(t, fixture, []carrier.ReindexPlan{first, second}, 8, 3, 3)
}

// TestRegionNewtonSaturationCollapsesOnCoordinateIdentity pins the first
// collapse: a relation that preserves every coordinate is its own closure, so
// the saturation composes nothing at all.
func TestRegionNewtonSaturationCollapsesOnCoordinateIdentity(t *testing.T) {
	fixture := newNewtonLawFixture(t, 3)
	identity, ok := fixture.composition.IdentityReindex(fixture.scope)
	if !ok {
		t.Fatal("newton-law identity relation")
	}
	newton, ok := saturateRegionNewton(fixture.composition, []carrier.ReindexPlan{identity}, false)
	if !ok || len(newton.basis) != 1 || newton.rounds != 0 {
		t.Fatalf("identity closure basis=%d rounds=%d", len(newton.basis), newton.rounds)
	}
}

// TestRegionNewtonSaturationClosesAtTheSquareOfAProjection pins the second
// collapse: a relation whose composition with itself introduces no new
// relation settles after exactly one squaring round.
func TestRegionNewtonSaturationClosesAtTheSquareOfAProjection(t *testing.T) {
	fixture := newNewtonLawFixture(t, 3)
	// Every coordinate reaches the fixed coordinate in one step, so the square
	// is the relation itself.
	collapse := fixture.transport(t, map[guard.Atom]guard.Atom{1: 3, 2: 3, 3: 3})
	newton, ok := saturateRegionNewton(fixture.composition, []carrier.ReindexPlan{collapse}, false)
	if !ok || len(newton.basis) != 1 || newton.rounds != 1 {
		t.Fatalf("idempotent closure basis=%d rounds=%d", len(newton.basis), newton.rounds)
	}
}

// TestRegionNewtonSaturationRefusesRelationsItCannotCompose pins the
// fail-closed edge: a relation that does not return to its own coordinate
// interface has no powers, so the Region simply carries no basis. Nothing is
// capped or approximated; Newton does not run there.
func TestRegionNewtonSaturationRefusesRelationsItCannotCompose(t *testing.T) {
	fixture := newNewtonLawFixture(t, 3)
	narrow, ok := fixture.composition.SealScope([]guard.Atom{1})
	if !ok {
		t.Fatal("newton-law narrow scope")
	}
	builder, ok := fixture.composition.NewReindex(narrow, fixture.scope)
	if !ok || builder == nil {
		t.Fatal("newton-law cross-scope builder")
	}
	if !builder.Identity(1) {
		t.Fatal("newton-law cross-scope coordinate")
	}
	crossing, sealed := builder.Seal()
	if !sealed {
		t.Fatal("newton-law cross-scope seal")
	}
	if newton, ok := saturateRegionNewton(fixture.composition, []carrier.ReindexPlan{crossing}, false); ok || newton.available() {
		t.Fatal("a relation that cannot compose with itself was sealed as a closure basis")
	}
	if newton, ok := saturateRegionNewton(fixture.composition, nil, false); ok || newton.available() {
		t.Fatal("an empty atom set was sealed as a closure basis")
	}
}
