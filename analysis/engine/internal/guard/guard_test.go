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
