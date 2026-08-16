package guard

import (
	"runtime"
	"sync"
	"testing"
)

const (
	testA Atom = 11
	testB Atom = 29
	testC Atom = 47
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	manager, err := New([]Atom{testA, testB, testC})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return manager
}

func literal(t testing.TB, work *Work, atom Atom) Guard {
	t.Helper()
	guard, ok := work.Literal(atom)
	if !ok {
		t.Fatalf("Literal(%d) rejected a presealed atom", atom)
	}
	return guard
}

func TestInitialAtomUniverseIsCanonical(t *testing.T) {
	order := []Atom{testA, testB}
	manager, err := New(order)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	order[0] = testC
	work := manager.NewWork()
	if _, ok := work.Literal(testC); ok {
		t.Fatal("caller mutation introduced a new atom")
	}
	if _, err := New([]Atom{testA, testA}); err != ErrDuplicateAtom {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := New([]Atom{testB, testA}); err != ErrUnsortedAtom {
		t.Fatalf("unsorted error = %v", err)
	}
}

func TestWorkExactOperationsAndSeal(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	a, b, c := literal(t, work, testA), literal(t, work, testB), literal(t, work, testC)
	notA := work.Not(a)
	guard := work.Or(work.And(a, work.Not(b)), c)
	if got := work.Restrict(guard, testA, false); !work.Equivalent(got, c) {
		t.Fatalf("restriction = %#v, want c", got)
	}
	if got := work.Exists(work.And(a, b), testB); !work.Equivalent(got, a) {
		t.Fatalf("exists b. a and b = %#v, want a", got)
	}
	substituted := work.Substitute(guard, map[Atom]Guard{testA: work.Not(c)})
	expectedSubstitution := work.Or(work.And(work.Not(c), work.Not(b)), c)
	if !work.Equivalent(substituted, expectedSubstitution) {
		t.Fatal("substitution did not preserve its Boolean law")
	}
	renamed, ok := work.Rename(work.And(a, work.Not(b)), map[Atom]Atom{testA: testB, testB: testA})
	if !ok {
		t.Fatal("rename rejected its own atoms")
	}
	if expected := work.And(b, work.Not(a)); !work.Equivalent(renamed, expected) {
		t.Fatal("rename did not preserve its Boolean law")
	}
	work.Seal()
	if !manager.Valid(guard) || !manager.Valid(substituted) || !manager.Valid(renamed) {
		t.Fatal("sealed guards are not readable")
	}
	if !manager.Conflict(a, notA) {
		t.Fatal("sealed a and not a must conflict")
	}
}

func TestWorkReuseAfterSealPreservesPublishedGuards(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	prior := literal(t, work, testA)
	work.Seal()
	if !manager.Valid(prior) || work.Open() {
		t.Fatal("first sealed transaction did not publish a stable guard")
	}
	if !work.Begin() || work.Reset() || !work.Open() {
		t.Fatal("reusable Work did not obey its terminal-only linear cut")
	}
	if !work.Valid(prior) {
		t.Fatal("reused Work rejected a Guard published by its prior transaction")
	}
	next := literal(t, work, testB)
	work.Seal()
	if !manager.Valid(prior) || !manager.Valid(next) {
		t.Fatal("reusing Work invalidated an already-published Guard")
	}
}

func TestWorkReuseAfterDiscardClearsFailedCandidate(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	if !work.SetCheckpoint(func() bool { return false }) {
		t.Fatal("checkpoint install failed")
	}
	if _, ok := work.Literal(testA); ok {
		t.Fatal("cancelled candidate admitted a Guard")
	}
	work.Discard()
	if !work.Begin() || !work.Open() {
		t.Fatal("discarded Work did not reopen")
	}
	if got := literal(t, work, testC); !work.Valid(got) {
		t.Fatal("reopened Work rejected its new candidate")
	}
}

func TestWorkSealReuseReturnsEquivalentPublishedGuard(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	a := literal(t, work, testA)
	b := literal(t, work, testB)
	prior := work.And(a, work.Not(b))
	work.Seal()
	if !manager.Valid(prior) {
		t.Fatal("prior formula was not published")
	}
	if !work.Begin() {
		t.Fatal("published Work did not reopen")
	}
	repeatedA := literal(t, work, testA)
	repeatedB := literal(t, work, testB)
	repeated := work.And(repeatedA, work.Not(repeatedB))
	if !work.Valid(repeatedA) || !work.Valid(repeatedB) || !work.Valid(repeated) || !work.Equivalent(repeated, prior) {
		t.Fatal("successful Seal did not retain valid equivalent BDD results")
	}
	work.Seal()
}

func TestWorkSealClearsPublishedCachesButKeepsPriorRootValid(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	a := literal(t, work, testA)
	root := work.And(a, work.Not(literal(t, work, testB)))
	work.Seal()
	if !manager.Valid(root) || len(work.unique) != 0 || len(work.not) != 0 || len(work.applyCache) != 0 || len(work.ite) != 0 || len(work.restrict) != 0 || len(work.exists) != 0 || len(work.hashes) != 0 {
		t.Fatal("successful Seal retained published cache entries")
	}
	if !work.Begin() {
		t.Fatal("published Work did not reopen")
	}
	rebuilt := work.And(literal(t, work, testA), work.Not(literal(t, work, testB)))
	if !work.Equivalent(root, rebuilt) {
		t.Fatal("rebuilding equivalent formula changed semantics after cache clear")
	}
	work.Seal()
	if !manager.Valid(rebuilt) {
		t.Fatal("rebuilt equivalent formula did not publish")
	}
}

func TestWorkDiscardDoesNotPoisonLaterInterner(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	failed := literal(t, work, testA)
	work.Discard()
	if manager.Valid(failed) {
		t.Fatal("discarded candidate page became Manager-readable")
	}
	if !work.Begin() {
		t.Fatal("discarded Work did not reopen")
	}
	fresh := literal(t, work, testA)
	if fresh == failed {
		t.Fatal("discarded candidate poisoned the reusable interner")
	}
	work.Seal()
}

func TestWorkCloseDropsRetainedEpochReferences(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	_ = literal(t, work, testA)
	work.Seal()
	if work.unique == nil {
		t.Fatal("successful Seal did not retain its owner-local interner")
	}
	work.Close()
	if work.Open() || work.manager != nil || work.unique != nil || work.hashes != nil {
		t.Fatal("Close retained reusable Work state")
	}
	if work.Begin() {
		t.Fatal("closed Work reopened")
	}
}

func TestWorkLifecycleSeparatesPublishFromDiscard(t *testing.T) {
	var zero Work
	if zero.Open() || zero.Published() {
		t.Fatal("unowned zero Work reported a lifecycle")
	}
	manager := newTestManager(t)
	published := manager.NewWork()
	if !published.Open() || published.Published() {
		t.Fatal("new Work has the wrong lifecycle")
	}
	root := literal(t, published, testA)
	if !published.Valid(root) || manager.Valid(root) {
		t.Fatal("open Work visibility is wrong")
	}
	published.Seal()
	if published.Open() || !published.Published() {
		t.Fatal("Seal did not publish the Work")
	}
	if published.Valid(root) || !manager.Valid(root) {
		t.Fatal("published Work leaked candidate visibility or failed Manager visibility")
	}
	assertPanics(t, func() { published.Literal(testB) })
	published.Discard() // published is terminal; Discard cannot turn it into discarded.
	if !published.Published() {
		t.Fatal("Discard changed an already published outcome")
	}

	predecessor := manager.NewWork()
	if predecessor.Published() || !predecessor.Valid(root) {
		t.Fatal("same-manager Work did not accept a published predecessor")
	}
	other := newTestManager(t).NewWork()
	if other.Valid(root) {
		t.Fatal("foreign-manager Work accepted a published root")
	}

	discarded := manager.NewWork()
	dropped := literal(t, discarded, testB)
	discarded.Discard()
	if discarded.Open() || discarded.Published() || discarded.Valid(dropped) || manager.Valid(dropped) {
		t.Fatal("Discard did not produce a distinct unpublished terminal outcome")
	}
	assertPanics(t, func() { discarded.Seal() })
	allocations := testing.AllocsPerRun(1_000, func() {
		if published.Open() || !published.Published() || discarded.Open() || discarded.Published() {
			t.Fatal("terminal lifecycle changed")
		}
	})
	if allocations != 0 {
		t.Fatalf("lifecycle checks allocated %f times", allocations)
	}
}

func TestCrossGenerationSemanticCanonicality(t *testing.T) {
	manager := newTestManager(t)
	first := manager.NewWork()
	a1, b1, c1 := literal(t, first, testA), literal(t, first, testB), literal(t, first, testC)
	left := first.Or(first.And(a1, c1), b1)
	first.Seal()

	second := manager.NewWork()
	a2, b2, c2 := literal(t, second, testA), literal(t, second, testB), literal(t, second, testC)
	right := second.Or(b2, second.And(c2, a2))
	second.Seal()

	if comparison, ok := manager.Compare(left, right); !ok || comparison != 0 || !manager.Equivalent(left, right) {
		t.Fatalf("cross-generation equality = %d/%t", comparison, ok)
	}
	if !manager.Entails(left, right) || !manager.Entails(right, left) {
		t.Fatal("cross-generation equivalent guards lost entailment")
	}
}

func TestEntailsIdentityAndTerminalsHaveNoTraversalAllocation(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	region := literal(t, work, testA)
	work.Seal()

	if !manager.Entails(region, region) {
		t.Fatal("identical guard did not entail itself")
	}
	if !manager.Entails(manager.False(), region) || !manager.Entails(region, manager.True()) {
		t.Fatal("terminal entailment law failed")
	}
	if manager.Entails(manager.True(), region) || manager.Entails(region, manager.False()) || manager.Entails(manager.True(), manager.False()) {
		t.Fatal("terminal non-entailment law failed")
	}

	// Warm the assertion path, then prove the direct identity result avoids the
	// satisfiability stack and seen map entirely.
	_ = manager.Entails(region, region)
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !manager.Entails(region, region) {
			t.Fatal("identical guard did not entail itself")
		}
	}); allocations != 0 {
		t.Fatalf("identical Entails allocated %f times", allocations)
	}
}

func TestWorkReadsSealedPriorAndCurrentLocalWithoutObserverAllocation(t *testing.T) {
	manager := newTestManager(t)
	priorWork := manager.NewWork()
	prior := literal(t, priorWork, testA)
	priorWork.Seal()

	current := manager.NewWork()
	local := literal(t, current, testA)
	if !current.Valid(prior) || !current.Valid(local) {
		t.Fatal("candidate did not accept sealed prior and current local guards")
	}
	if comparison, ok := current.Compare(prior, local); !ok || comparison != 0 || !current.Equivalent(prior, local) {
		t.Fatalf("mixed-generation Work equality = %d/%t", comparison, ok)
	}
	if current.Conflict(prior, local) || !current.Entails(prior, local) {
		t.Fatal("mixed-generation Work read laws failed")
	}
	// The first comparison may grow candidate scratch; warmed compatible checks
	// reuse it rather than allocating one observer map/stack per call.
	_, _ = current.Compare(prior, local)
	allocations := testing.AllocsPerRun(1_000, func() {
		if comparison, ok := current.Compare(prior, local); !ok || comparison != 0 {
			t.Fatal("warmed mixed-generation comparison changed")
		}
	})
	if allocations != 0 {
		t.Fatalf("warmed Work comparison allocated %f times", allocations)
	}
}

func TestWorkEntailsReusesTraversalScratchAndHonorsCheckpoint(t *testing.T) {
	manager := newTestManager(t)
	builder := manager.NewWork()
	a, b := literal(t, builder, testA), literal(t, builder, testB)
	premise := builder.And(a, b)
	conclusion := a
	builder.Seal()

	reader := manager.NewWork()
	if !reader.Entails(premise, conclusion) || reader.Entails(conclusion, premise) {
		t.Fatal("reusable Work entailment law failed")
	}
	// Warm the non-identity traversal so its Work-owned seen map and stack have
	// reached their stable capacity before measuring the hot read.
	if !reader.Entails(premise, conclusion) {
		t.Fatal("warmed entailment changed")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !reader.Entails(premise, conclusion) {
			t.Fatal("reused entailment changed")
		}
	}); allocations != 0 {
		t.Fatalf("reused Work entailment allocated %f times", allocations)
	}

	if !reader.SetCheckpoint(func() bool { return false }) {
		t.Fatal("checkpoint install failed")
	}
	if reader.Entails(premise, conclusion) {
		t.Fatal("cancelled entailment succeeded")
	}
	if !reader.SetCheckpoint(nil) || !reader.Entails(premise, conclusion) {
		t.Fatal("reader did not recover after cancellation")
	}

	foreignManager := newTestManager(t)
	foreignWork := foreignManager.NewWork()
	foreign := literal(t, foreignWork, testA)
	foreignWork.Seal()
	if reader.Entails(premise, foreign) {
		t.Fatal("foreign manager entailment crossed the Work authority fence")
	}
}

func TestWorkEntailsImmediateCasesAcrossSealedAndCandidateRoots(t *testing.T) {
	manager := newTestManager(t)
	priorWork := manager.NewWork()
	prior := literal(t, priorWork, testA)
	priorWork.Seal()

	current := manager.NewWork()
	local := literal(t, current, testA)
	if !current.Valid(prior) || !current.Valid(local) {
		t.Fatal("Work did not admit sealed predecessor and current candidate")
	}

	// Same-handle identity is valid for both an immutable predecessor and the
	// current candidate.  The terminal cases cover the exact Boolean boundary
	// without entering the product traversal.
	if !current.Entails(prior, prior) || !current.Entails(local, local) ||
		!current.Entails(manager.False(), local) || !current.Entails(local, manager.True()) {
		t.Fatal("immediate Work entailment cases failed")
	}
	if current.Entails(manager.True(), local) || current.Entails(local, manager.False()) {
		t.Fatal("immediate Work non-entailment cases failed")
	}

	// Distinct sealed/candidate handles still use the exact product traversal;
	// warming it must preserve the allocation-free reusable-scratch contract.
	if !current.Entails(prior, local) || !current.Entails(local, prior) {
		t.Fatal("mixed-generation Work entailment failed")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !current.Entails(prior, local) || !current.Entails(prior, prior) || !current.Entails(local, local) {
			t.Fatal("warmed Work entailment changed")
		}
	}); allocations != 0 {
		t.Fatalf("warmed Work entailment allocated %f times", allocations)
	}
}

func TestForeignAndUnsealedGuardsReject(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	local := literal(t, work, testA)
	if manager.Valid(local) {
		t.Fatal("unsealed guard was readable")
	}
	work.Discard()
	if manager.Valid(local) {
		t.Fatal("discarded guard was readable")
	}
	other := newTestManager(t)
	otherWork := other.NewWork()
	foreign := literal(t, otherWork, testA)
	otherWork.Seal()
	if _, ok := manager.Compare(manager.True(), foreign); ok {
		t.Fatal("foreign guard compared as local")
	}
}

func assertPanics(t testing.TB, action func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("operation on terminal Work did not panic")
		}
	}()
	action()
}

func TestForeignWorkNeverReadsUnsealedPages(t *testing.T) {
	for _, transition := range []struct {
		name string
		seal bool
	}{
		{name: "seal", seal: true},
		{name: "discard", seal: false},
	} {
		t.Run(transition.name, func(t *testing.T) {
			order := ascending(3_000)
			manager, err := New(order)
			if err != nil {
				t.Fatal(err)
			}
			owner := manager.NewWork()
			candidate := literal(t, owner, order[0])
			foreign := manager.NewWork()
			started := make(chan struct{})
			stop := make(chan struct{})
			var readers sync.WaitGroup
			readers.Add(1)
			go func() {
				defer readers.Done()
				close(started)
				for {
					select {
					case <-stop:
						return
					default:
					}
					_, _ = foreign.Compare(candidate, manager.True())
					_ = foreign.Valid(candidate)
					_ = foreign.Equivalent(candidate, manager.True())
					_ = foreign.Conflict(candidate, manager.True())
					_ = foreign.Entails(candidate, manager.True())
				}
			}()
			<-started

			for index := 1; index < len(order); index++ {
				_ = literal(t, owner, order[index])
				if index%64 == 0 {
					runtime.Gosched()
				}
			}
			if transition.seal {
				owner.Seal()
				if !foreign.Valid(candidate) {
					t.Fatal("foreign Work rejected sealed predecessor")
				}
			} else {
				owner.Discard()
				if foreign.Valid(candidate) {
					t.Fatal("foreign Work accepted discarded predecessor")
				}
			}
			close(stop)
			readers.Wait()
		})
	}
}

func TestLargeGuardRestrictsWithExactBooleanMeaning(t *testing.T) {
	const count = 600
	order := ascending(count)
	manager, err := New(order)
	if err != nil {
		t.Fatal(err)
	}
	build := manager.NewWork()
	root := manager.True()
	for index := len(order) - 1; index >= 0; index-- {
		root = build.And(literal(t, build, order[index]), root)
	}
	build.Seal()

	change := manager.NewWork()
	result := change.Restrict(root, order[1], true)
	if falseBranch := change.Restrict(root, order[1], false); !change.Equivalent(falseBranch, manager.False()) {
		t.Fatal("restriction admits the false branch")
	}
	if trueBranch := change.Restrict(root, order[1], true); !change.Equivalent(trueBranch, result) {
		t.Fatal("restriction did not eliminate the fixed atom")
	}
	change.Seal()
	if !manager.Valid(result) {
		t.Fatal("sealed changed-path result is unreadable")
	}
}

func TestDeepSealedReadAndConcurrentAccess(t *testing.T) {
	const count = 100_000
	order := ascending(count)
	manager, err := New(order)
	if err != nil {
		t.Fatal(err)
	}
	work := manager.NewWork()
	deep := manager.True()
	for index := len(order) - 1; index >= 0; index-- {
		deep = work.And(literal(t, work, order[index]), deep)
	}
	work.Seal()

	change := manager.NewWork()
	prefix := change.Restrict(deep, order[1], true)
	change.Seal()
	if relation, ok := manager.Compare(deep, prefix); !ok || relation == 0 {
		t.Fatalf("deep Compare = %d/%t", relation, ok)
	}
	if !manager.Entails(deep, prefix) || manager.Conflict(deep, prefix) {
		t.Fatal("deep sealed read laws failed")
	}

	var group sync.WaitGroup
	for worker := 0; worker < 12; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < 3; iteration++ {
				if comparison, ok := manager.Compare(deep, prefix); !ok || comparison == 0 {
					t.Error("concurrent Compare failed")
				}
				if !manager.Entails(deep, prefix) || manager.Conflict(deep, prefix) {
					t.Error("concurrent sealed read failed")
				}
			}
		}()
	}
	group.Wait()
	allocations := testing.AllocsPerRun(1_000, func() {
		if !manager.Equivalent(deep, deep) {
			t.Fatal("identical sealed guard lost equality")
		}
		if comparison, ok := manager.Compare(deep, deep); !ok || comparison != 0 {
			t.Fatal("identical sealed guard lost canonical comparison")
		}
	})
	if allocations != 0 {
		t.Fatalf("sealed equality/read allocated %f times", allocations)
	}
}

func TestWarmedConstructionCacheHitsAllocateNothing(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	a, b := literal(t, work, testA), literal(t, work, testB)
	and := work.And(a, b)
	allocations := testing.AllocsPerRun(1_000, func() {
		if got := work.And(a, b); !work.Equivalent(got, and) {
			t.Fatal("warmed And changed its Boolean meaning")
		}
	})
	if allocations != 0 {
		t.Fatalf("warmed construction allocated %f times", allocations)
	}
}

func ascending(count int) []Atom {
	order := make([]Atom, count)
	for index := range order {
		order[index] = Atom(index + 1)
	}
	return order
}

func BenchmarkWorkAndHit(b *testing.B) {
	manager, _ := New([]Atom{testA, testB})
	work := manager.NewWork()
	a, bGuard := literal(b, work, testA), literal(b, work, testB)
	_ = work.And(a, bGuard)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_ = work.And(a, bGuard)
	}
}
