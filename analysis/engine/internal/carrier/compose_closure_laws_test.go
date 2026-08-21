package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// closureFixture is the factor-free recurrence shape the closure laws are
// stated on: one carry-only Factor over a three-coordinate universe, so the
// only thing a transport can move is the state's own support region.
type closureFixture struct {
	manager     *guard.Manager
	composition *Composition
	whole       support.Mask
	literals    map[guard.Atom]support.Mask
}

func newClosureFixture(t testing.TB) closureFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("closure fixture support")
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("closure fixture support work")
	}
	literals := make(map[guard.Atom]support.Mask, 3)
	for _, atom := range []guard.Atom{1, 2, 3} {
		mask, literal := regions.Literal(atom, true)
		if !literal {
			t.Fatalf("closure fixture literal %d", atom)
		}
		literals[atom] = mask
	}
	if !regions.Seal() {
		t.Fatal("closure fixture literal seal")
	}
	composition, ok := attachTestComposition(t, []FactorOperation{&carryOnlyOperation{guards: manager}})
	if !ok {
		t.Fatal("closure fixture composition")
	}
	return closureFixture{manager: manager, composition: composition, whole: whole, literals: literals}
}

// substitution seals one relation over the fixture's complete scope from an
// explicit source-coordinate to target-coordinate map. Every scope coordinate
// must be named, which is exactly what ReindexBuilder.Seal demands.
func (fixture closureFixture) substitution(t testing.TB, mapping map[guard.Atom]guard.Atom, runtime bool) ReindexPlan {
	t.Helper()
	scope := fixture.composition.Scope()
	builder, ok := fixture.composition.NewReindex(scope, scope)
	if runtime {
		builder, ok = fixture.composition.NewRuntimeReindex(scope, scope)
	}
	if !ok || builder == nil {
		t.Fatalf("closure fixture relation builder runtime=%t", runtime)
	}
	for _, source := range []guard.Atom{1, 2, 3} {
		target, named := mapping[source]
		if !named {
			t.Fatalf("closure fixture relation leaves coordinate %d unnamed", source)
		}
		if source == target {
			if !builder.Identity(source) {
				t.Fatalf("closure fixture relation identity %d", source)
			}
			continue
		}
		expression, expressed := scope.Expr(fixture.literals[target])
		if !expressed || !builder.Set(source, expression) {
			t.Fatalf("closure fixture relation %d -> %d", source, target)
		}
	}
	plan, sealed := builder.Seal()
	if !sealed || !plan.Valid() {
		t.Fatalf("closure fixture relation seal runtime=%t", runtime)
	}
	return plan
}

func (fixture closureFixture) state(t testing.TB, region support.Mask) State {
	t.Helper()
	state, ok := NewState(fixture.composition, fixture.composition.Scope(), region)
	if !ok {
		t.Fatal("closure fixture state")
	}
	return state
}

// supportIdentity is the literal byte identity of one state's sole observable
// content. The factor-free shape carries no typed root distinction, so the
// canonical support formula identity is the whole state's byte image.
func supportIdentity(t testing.TB, state State) guard.FormulaID {
	t.Helper()
	id, ok := state.Support().Identity()
	if !ok || !id.Available() {
		t.Fatal("closure state identity")
	}
	return id
}

// TestComposedReindexAgreesWithSequentialTransport is the semiring product
// law the closure basis rests on: one composed relation moves a state exactly
// where applying its two operands in order moves it, byte for byte.
func TestComposedReindexAgreesWithSequentialTransport(t *testing.T) {
	fixture := newClosureFixture(t)
	first := fixture.substitution(t, map[guard.Atom]guard.Atom{1: 2, 2: 3, 3: 3}, false)
	second := fixture.substitution(t, map[guard.Atom]guard.Atom{1: 1, 2: 3, 3: 3}, false)
	composed, ok := fixture.composition.ComposeReindex(first, second)
	if !ok || !composed.Valid() {
		t.Fatal("cold composition")
	}
	origin := fixture.state(t, fixture.literals[1])
	work, ok := fixture.composition.NewWork()
	if !ok {
		t.Fatal("closure work")
	}
	once, onceOK := work.Reindex(origin, first)
	sequential, sequentialOK := work.Reindex(once, second)
	direct, directOK := work.Reindex(origin, composed)
	if !onceOK || !sequentialOK || !directOK {
		t.Fatalf("transports once=%t sequential=%t direct=%t", onceOK, sequentialOK, directOK)
	}
	if !work.EqualUnder(sequential, direct) {
		t.Fatal("composed transport disagrees with sequential transport")
	}
	if supportIdentity(t, sequential) != supportIdentity(t, direct) {
		t.Fatal("composed transport is not byte-identical to sequential transport")
	}
	// The law would hold vacuously if either operand were inert on this state.
	if supportIdentity(t, once) == supportIdentity(t, direct) || supportIdentity(t, origin) == supportIdentity(t, once) {
		t.Fatal("closure fixture relations do not move the state")
	}
}

// TestComposeRuntimeReindexSealsAfterWorkOpens pins the runtime-safe compose
// admission: the cold surface still refuses once an evaluator holds Work, and
// its runtime counterpart seals the same relation, which still obeys the
// sequential-transport law.
func TestComposeRuntimeReindexSealsAfterWorkOpens(t *testing.T) {
	fixture := newClosureFixture(t)
	first := fixture.substitution(t, map[guard.Atom]guard.Atom{1: 2, 2: 3, 3: 3}, false)
	second := fixture.substitution(t, map[guard.Atom]guard.Atom{1: 1, 2: 3, 3: 3}, false)
	origin := fixture.state(t, fixture.literals[1])
	work, ok := fixture.composition.NewWork()
	if !ok {
		t.Fatal("closure work")
	}
	if refused, ok := fixture.composition.ComposeReindex(first, second); ok || refused.Valid() {
		t.Fatal("cold compose sealed a relation after work opened")
	}
	composed, ok := fixture.composition.ComposeRuntimeReindex(first, second)
	if !ok || !composed.Valid() {
		t.Fatal("runtime compose refused after work opened")
	}
	once, onceOK := work.Reindex(origin, first)
	sequential, sequentialOK := work.Reindex(once, second)
	direct, directOK := work.Reindex(origin, composed)
	if !onceOK || !sequentialOK || !directOK {
		t.Fatalf("transports once=%t sequential=%t direct=%t", onceOK, sequentialOK, directOK)
	}
	if !work.EqualUnder(sequential, direct) || supportIdentity(t, sequential) != supportIdentity(t, direct) {
		t.Fatal("runtime composed transport disagrees with sequential transport")
	}
}

// TestComposedReindexCarriesOneRelationIdentity pins that the published
// relation identity is a property of the relation and not of the order the
// relation was built in: two ways of reaching the same composite agree.
func TestComposedReindexCarriesOneRelationIdentity(t *testing.T) {
	fixture := newClosureFixture(t)
	shift := fixture.substitution(t, map[guard.Atom]guard.Atom{1: 2, 2: 3, 3: 3}, false)
	square, ok := fixture.composition.ComposeReindex(shift, shift)
	if !ok {
		t.Fatal("square")
	}
	cube, ok := fixture.composition.ComposeReindex(square, shift)
	if !ok {
		t.Fatal("cube")
	}
	squareID, squareOK := square.RelationIdentity()
	cubeID, cubeOK := cube.RelationIdentity()
	shiftID, shiftOK := shift.RelationIdentity()
	if !squareOK || !cubeOK || !shiftOK {
		t.Fatalf("relation identities square=%t cube=%t shift=%t", squareOK, cubeOK, shiftOK)
	}
	if shiftID == squareID {
		t.Fatal("distinct relations share one identity")
	}
	// This shift saturates at its square: every coordinate has already reached
	// the fixed coordinate 3 after two steps.
	if squareID != cubeID {
		t.Fatal("equal relations reached by different compositions carry different identities")
	}
	if !square.SelfComposable() || !shift.SelfComposable() {
		t.Fatal("scope endomorphism is not reported as self-composable")
	}
}

// TestFactorFreeMergeScopeReportsEmptySelection publishes the emptiness the
// recurrence binder previously had to approximate with the pre-seal target
// count it handed to SealWidening.
func TestFactorFreeMergeScopeReportsEmptySelection(t *testing.T) {
	fixture := newClosureFixture(t)
	empty, ok := fixture.composition.SealWidening(nil)
	if !ok {
		t.Fatal("empty widening selection")
	}
	if !empty.FactorFree() {
		t.Fatal("empty widening selection is not reported factor-free")
	}
	if fixture.composition.AllMergeScope().FactorFree() {
		t.Fatal("the complete join selection is reported factor-free")
	}
	if (MergeScope{}).FactorFree() {
		t.Fatal("an unsealed selection is reported factor-free")
	}
}
