package guard

import (
	"sync"
	"testing"
)

func TestDecomposePreservesExactTerminalsAndOrderedDecisions(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	falseView, ok := work.Decompose(work.False())
	if !ok || !falseView.Terminal || falseView.Value {
		t.Fatalf("false decomposition = %#v/%t", falseView, ok)
	}
	trueView, ok := work.Decompose(work.True())
	if !ok || !trueView.Terminal || !trueView.Value {
		t.Fatalf("true decomposition = %#v/%t", trueView, ok)
	}

	a, b, c := literal(t, work, testA), literal(t, work, testB), literal(t, work, testC)
	root := work.Or(work.And(a, c), work.And(work.Not(a), b))
	view, ok := work.Decompose(root)
	if !ok || view.Terminal || view.Atom != testA {
		t.Fatalf("root decomposition = %#v/%t", view, ok)
	}
	if !work.Equivalent(view.Low, b) || !work.Equivalent(view.High, c) {
		t.Fatalf("root branches do not retain the canonical low/high meaning: %#v", view)
	}
	assertDecisionOrder(t, work, root)

	work.Seal()
	view, ok = manager.Decompose(root)
	if !ok || view.Terminal || view.Atom != testA || !manager.Equivalent(view.Low, b) || !manager.Equivalent(view.High, c) {
		t.Fatalf("sealed decomposition changed = %#v/%t", view, ok)
	}
	if _, ok := manager.Decompose(Guard{}); ok {
		t.Fatal("manager accepted an invalid Guard")
	}
}

func TestFoldPostorderReconstructsExactGuard(t *testing.T) {
	manager := newTestManager(t)
	build := manager.NewWork()
	a, b, c := literal(t, build, testA), literal(t, build, testB), literal(t, build, testC)
	root := build.Or(build.And(a, c), build.And(build.Not(a), b))
	build.Seal()

	reconstruct := manager.NewWork()
	values := make(map[Guard]Guard)
	var order []Atom
	completed, valid := manager.Fold(root, func(original Guard, view Decomposition) bool {
		if view.Terminal {
			if view.Value {
				values[original] = reconstruct.True()
			} else {
				values[original] = reconstruct.False()
			}
			return true
		}
		low, lowOK := values[view.Low]
		high, highOK := values[view.High]
		if !lowOK || !highOK {
			t.Fatalf("Fold visited parent before a successor: %#v", view)
		}
		atom, atomOK := reconstruct.Literal(view.Atom)
		if !atomOK {
			t.Fatalf("Fold exposed an atom outside the Manager universe: %d", view.Atom)
		}
		// A decision is exactly (atom ∧ high) ∨ (¬atom ∧ low). Rebuild it
		// through the public Boolean algebra rather than its BDD representation.
		values[original] = reconstruct.Or(
			reconstruct.And(atom, high),
			reconstruct.And(reconstruct.Not(atom), low),
		)
		order = append(order, view.Atom)
		return true
	})
	if !completed || !valid {
		t.Fatalf("Fold = %t/%t", completed, valid)
	}
	if want := []Atom{testB, testC, testA}; !sameAtoms(order, want) {
		t.Fatalf("Fold decision order = %v, want canonical postorder %v", order, want)
	}
	rebuilt, ok := values[root]
	if !ok {
		t.Fatal("Fold omitted its root")
	}
	reconstruct.Seal()
	if !manager.Equivalent(root, rebuilt) {
		t.Fatal("decomposition/fold reconstruction changed Guard semantics")
	}
}

func TestWorkFoldReadsSealedAndCandidateGuardsWithoutPublication(t *testing.T) {
	manager := newTestManager(t)
	prior := manager.NewWork()
	b := literal(t, prior, testB)
	prior.Seal()

	current := manager.NewWork()
	a, c := literal(t, current, testA), literal(t, current, testC)
	root := current.Or(current.And(a, b), current.And(current.Not(a), c))
	seen := 0
	completed, valid := current.Fold(root, func(_ Guard, _ Decomposition) bool {
		seen++
		return true
	})
	if !completed || !valid || seen != 5 { // false, true, b, c, and the reduced a root
		t.Fatalf("candidate Fold = %t/%t with %d nodes", completed, valid, seen)
	}
	if _, valid := manager.Fold(root, func(Guard, Decomposition) bool { return true }); valid {
		t.Fatal("manager folded an unsealed candidate Guard")
	}
	foreign := newTestManager(t).NewWork()
	if _, valid := foreign.Fold(root, func(Guard, Decomposition) bool { return true }); valid {
		t.Fatal("foreign Work folded a candidate Guard")
	}

	stopped := 0
	completed, valid = current.Fold(root, func(Guard, Decomposition) bool {
		stopped++
		return false
	})
	if completed || !valid || stopped != 1 {
		t.Fatalf("short Fold = %t/%t with %d visits", completed, valid, stopped)
	}
}

func TestFoldIsIterativeAndConcurrentForSealedGuards(t *testing.T) {
	const count = 8_192
	order := ascending(count)
	manager, err := New(order)
	if err != nil {
		t.Fatal(err)
	}
	work := manager.NewWork()
	root := work.True()
	for index := len(order) - 1; index >= 0; index-- {
		root = work.And(literal(t, work, order[index]), root)
	}
	work.Seal()

	const workers = 12
	var group sync.WaitGroup
	errors := make(chan string, workers)
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < 4; iteration++ {
				visited := 0
				completed, valid := manager.Fold(root, func(_ Guard, _ Decomposition) bool {
					visited++
					return true
				})
				if !completed || !valid || visited != count+2 {
					errors <- "sealed Fold changed its exact DAG traversal"
					return
				}
			}
		}()
	}
	group.Wait()
	close(errors)
	for message := range errors {
		t.Error(message)
	}
}

func assertDecisionOrder(t testing.TB, work *Work, root Guard) {
	t.Helper()
	completed, valid := work.Fold(root, func(_ Guard, view Decomposition) bool {
		if view.Terminal {
			return true
		}
		for _, child := range [...]Guard{view.Low, view.High} {
			childView, ok := work.Decompose(child)
			if !ok {
				t.Fatal("Fold exposed an unreadable child")
			}
			if !childView.Terminal && childView.Atom <= view.Atom {
				t.Fatalf("decision order %d -> %d is not canonical", view.Atom, childView.Atom)
			}
		}
		return true
	})
	if !completed || !valid {
		t.Fatalf("Fold order check = %t/%t", completed, valid)
	}
}

func sameAtoms(left, right []Atom) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
