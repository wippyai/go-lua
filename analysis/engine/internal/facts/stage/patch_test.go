package stage

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type testFactor uint64
type testKey uint64

const testFactFactor testFactor = 1

type fixture struct {
	facts      *diagram.Diagram[testFactor, testKey, uint8]
	values     *terminal.Arena[uint8]
	all        support.Mask
	trueAtOne  support.Mask
	falseAtOne support.Mask
	config     Config[testKey, uint8]
}

func newFixture(t testing.TB) fixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	masks := support.New(manager)
	if masks == nil {
		t.Fatal("support work creation failed")
	}
	all := masks.True()
	trueAtOne, ok := masks.Literal(1, true)
	if !ok {
		t.Fatal("true support failed")
	}
	falseAtOne, ok := masks.Not(trueAtOne)
	if !ok || !masks.Seal() {
		t.Fatal("false support setup failed")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok || !values.Seal() {
		t.Fatal("terminal setup failed")
	}
	facts, ok := diagram.New(diagram.Config[testFactor, testKey, uint8]{
		Factors: []testFactor{testFactFactor}, Terminals: values, Guards: manager,
	})
	if !ok {
		t.Fatal("fact diagram failed")
	}
	return fixture{
		facts: facts, values: values, all: all, trueAtOne: trueAtOne, falseAtOne: falseAtOne,
		config: Config[testKey, uint8]{
			KeyEnd: 10, Default: 5,
			AdmitAt:  func(_ testKey, _ uint8) bool { return true },
			Equal:    func(left, right uint8) bool { return left == right },
			LessOrEq: func(left, right uint8) bool { return left <= right },
			Join: func(left, right uint8) (uint8, bool) {
				if left > right {
					return left, true
				}
				return right, true
			},
		},
	}
}

func atOne(value bool) func(guard.Atom) bool {
	return func(atom guard.Atom) bool { return atom == 1 && value }
}

func rootAt(t testing.TB, fixture fixture, root diagram.Root[testFactor, testKey, uint8], key testKey, valuation func(guard.Atom) bool) (uint8, bool) {
	t.Helper()
	id, present, valid := fixture.facts.At(root, testFactFactor, key, valuation)
	if !valid {
		t.Fatal("invalid published root")
	}
	if !present {
		return fixture.config.Default, true
	}
	value, valid := fixture.values.Value(id)
	return value, valid
}

func accept(patch *Patch[testFactor, testKey, uint8]) (diagram.Root[testFactor, testKey, uint8], KeyChanges[testKey], bool) {
	var changes KeyChanges[testKey]
	root, ok := patch.Accept(func(produced KeyChanges[testKey], _ *support.Work) bool {
		changes = produced
		return true
	})
	return root, changes, ok
}

func seed(t testing.TB, fixture fixture, writes func(*Patch[testFactor, testKey, uint8]) bool) diagram.Root[testFactor, testKey, uint8] {
	t.Helper()
	patch := Begin(fixture.facts, fixture.facts.Empty(), fixture.all, fixture.config)
	if patch == nil || !writes(patch) {
		t.Fatal("seed writes failed")
	}
	root, changed, ok := accept(patch)
	if !ok || changed.Count() == 0 {
		t.Fatal("seed accept failed")
	}
	return root
}

func TestPatchKeepsInputImmutableUntilAccept(t *testing.T) {
	fixture := newFixture(t)
	base := seed(t, fixture, func(patch *Patch[testFactor, testKey, uint8]) bool {
		return patch.Set(4, fixture.trueAtOne, 7)
	})
	patch := Begin(fixture.facts, base, fixture.all, fixture.config)
	if patch == nil || !patch.Set(4, fixture.trueAtOne, 9) {
		t.Fatal("candidate write failed")
	}
	if value, valid := rootAt(t, fixture, base, 4, atOne(true)); !valid || value != 7 {
		t.Fatalf("candidate mutated input = %d/%t, want 7/true", value, valid)
	}
	root, changed, ok := accept(patch)
	if !ok || changed.Count() == 0 {
		t.Fatal("candidate was not accepted as a change")
	}
	if !fixture.facts.Valid(root) {
		t.Fatal("accepted root was not sealed")
	}
	if value, valid := rootAt(t, fixture, root, 4, atOne(true)); !valid || value != 9 {
		t.Fatalf("accepted write = %d/%t, want 9/true", value, valid)
	}
	if value, valid := rootAt(t, fixture, base, 4, atOne(true)); !valid || value != 7 {
		t.Fatalf("accept mutated input = %d/%t, want 7/true", value, valid)
	}
}

func TestPatchStrongAndWeakUpdatesRespectMasks(t *testing.T) {
	fixture := newFixture(t)
	base := seed(t, fixture, func(patch *Patch[testFactor, testKey, uint8]) bool {
		if !patch.Set(4, fixture.trueAtOne, 7) {
			t.Error("first disjoint seed write failed")
			return false
		}
		if !patch.Set(4, fixture.falseAtOne, 9) {
			t.Error("second disjoint seed write failed")
			return false
		}
		return true
	})
	patch := Begin(fixture.facts, base, fixture.all, fixture.config)
	if patch == nil ||
		!patch.WeakJoin(4, fixture.trueAtOne, 12) ||
		!patch.Set(4, fixture.falseAtOne, 3) {
		t.Fatal("candidate updates failed")
	}
	root, changed, ok := accept(patch)
	if !ok || changed.Count() == 0 {
		t.Fatal("candidate updates were not published as a change")
	}
	if value, valid := rootAt(t, fixture, root, 4, atOne(true)); !valid || value != 12 {
		t.Fatalf("weak high branch = %d/%t, want 12/true", value, valid)
	}
	if value, valid := rootAt(t, fixture, root, 4, atOne(false)); !valid || value != 3 {
		t.Fatalf("strong low branch = %d/%t, want 3/true", value, valid)
	}
}

func TestPatchDefaultResultsRemainSparse(t *testing.T) {
	fixture := newFixture(t)
	patch := Begin(fixture.facts, fixture.facts.Empty(), fixture.all, fixture.config)
	if patch == nil ||
		!patch.Set(4, fixture.trueAtOne, fixture.config.Default) ||
		!patch.WeakJoin(4, fixture.falseAtOne, 2) { // max(Default=5, 2) == Default
		t.Fatal("default-preserving writes failed")
	}
	root, changed, ok := accept(patch)
	if !ok || changed.Count() != 0 {
		t.Fatal("default-only patch reported a change")
	}
	if count, valid := fixture.facts.Count(root); !valid || count != 0 {
		t.Fatalf("default result retained a sparse column: %d/%t", count, valid)
	}
	if value, valid := rootAt(t, fixture, root, 4, atOne(true)); !valid || value != fixture.config.Default {
		t.Fatalf("default result = %d/%t, want %d/true", value, valid, fixture.config.Default)
	}
}

func TestPatchWeakJoinRejectsNonUpperBoundResult(t *testing.T) {
	fixture := newFixture(t)
	base := seed(t, fixture, func(patch *Patch[testFactor, testKey, uint8]) bool {
		return patch.Set(4, fixture.trueAtOne, 7)
	})
	bad := fixture.config
	bad.Join = func(left, _ uint8) (uint8, bool) { return left, true }
	patch := Begin(fixture.facts, base, fixture.all, bad)
	if patch == nil {
		t.Fatal("patch creation failed")
	}
	if patch.WeakJoin(4, fixture.trueAtOne, 12) {
		t.Fatal("weak join accepted a result below its incoming operand")
	}
	root, changed, ok := accept(patch)
	if !ok || changed.Count() != 0 || !fixture.facts.Equal(base, root) {
		t.Fatal("rejected weak join changed or invalidated the candidate")
	}
}

func TestPatchRejectsWritesOutsideCapturedSupport(t *testing.T) {
	fixture := newFixture(t)
	patch := Begin(fixture.facts, fixture.facts.Empty(), fixture.trueAtOne, fixture.config)
	if patch == nil {
		t.Fatal("patch creation failed")
	}
	if patch.Set(4, fixture.falseAtOne, 7) {
		t.Fatal("strong write outside captured support was accepted")
	}
	if patch.WeakJoin(4, fixture.falseAtOne, 7) {
		t.Fatal("weak write outside captured support was accepted")
	}
	if !patch.Set(4, fixture.trueAtOne, 7) {
		t.Fatal("contained write was rejected")
	}
}

func TestPatchNoOpHasExactUnchangedResult(t *testing.T) {
	fixture := newFixture(t)
	base := seed(t, fixture, func(patch *Patch[testFactor, testKey, uint8]) bool {
		return patch.Set(4, fixture.trueAtOne, 7)
	})
	patch := Begin(fixture.facts, base, fixture.all, fixture.config)
	if patch == nil || !patch.Set(4, fixture.trueAtOne, 7) {
		t.Fatal("no-op write failed")
	}
	root, changed, ok := accept(patch)
	if !ok || changed.Count() != 0 {
		t.Fatal("no-op patch did not report unchanged")
	}
	if !fixture.facts.Equal(base, root) {
		t.Fatal("no-op patch changed the semantic root")
	}
}

func TestPatchAcceptAndDiscardAreOneShotAndDoNotExposeCandidates(t *testing.T) {
	fixture := newFixture(t)
	base := fixture.facts.Empty()
	patch := Begin(fixture.facts, base, fixture.all, fixture.config)
	if patch == nil || !patch.Set(4, fixture.trueAtOne, 7) {
		t.Fatal("candidate write failed")
	}
	if value, valid := rootAt(t, fixture, base, 4, atOne(true)); !valid || value != fixture.config.Default {
		t.Fatalf("candidate escaped before disposition: %d/%t", value, valid)
	}
	if !patch.Discard() {
		t.Fatal("candidate discard failed")
	}
	if value, valid := rootAt(t, fixture, base, 4, atOne(true)); !valid || value != fixture.config.Default {
		t.Fatal("discard emitted a write into the predecessor")
	}
	if patch.Discard() || patch.Set(4, fixture.trueAtOne, 9) {
		t.Fatal("discarded patch remained open")
	}

	accepted := Begin(fixture.facts, base, fixture.all, fixture.config)
	if accepted == nil || !accepted.Set(4, fixture.trueAtOne, 7) {
		t.Fatal("accepted candidate write failed")
	}
	if _, _, ok := accept(accepted); !ok {
		t.Fatal("candidate accept failed")
	}
	if _, _, ok := accept(accepted); ok || accepted.Set(4, fixture.trueAtOne, 9) {
		t.Fatal("accepted patch remained open")
	}
}

func TestPatchRejectsOutOfRangeKeys(t *testing.T) {
	fixture := newFixture(t)
	patch := Begin(fixture.facts, fixture.facts.Empty(), fixture.all, fixture.config)
	if patch == nil {
		t.Fatal("patch creation failed")
	}
	if patch.Set(10, fixture.trueAtOne, 7) || patch.WeakJoin(10, fixture.trueAtOne, 7) {
		t.Fatal("out-of-range key was admitted")
	}
}

func TestBeginRejectsMultiFactorDiagram(t *testing.T) {
	fixture := newFixture(t)
	multi, ok := diagram.New(diagram.Config[testFactor, testKey, uint8]{
		Factors: []testFactor{1, 2}, Terminals: fixture.values, Guards: fixture.all.Manager(),
	})
	if !ok {
		t.Fatal("multi-factor diagram setup")
	}
	if patch := Begin(multi, multi.Empty(), fixture.all, fixture.config); patch != nil {
		t.Fatal("stage admitted a multi-factor change authority")
	}
}

// BenchmarkPatchBegin reports the real candidate construction cost. It is
// intentionally not required to be allocation-free: terminal work and an FDD
// builder are candidate state. The stage package must not add a dynamic read
// map on top of that fixed cost.
func BenchmarkPatchBegin(b *testing.B) {
	fixture := newFixture(b)
	base := fixture.facts.Empty()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		patch := Begin(fixture.facts, base, fixture.all, fixture.config)
		if patch == nil {
			b.Fatal("patch creation failed")
		}
		if !patch.Discard() {
			b.Fatal("patch discard failed")
		}
	}
}
