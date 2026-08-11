package guard

import "testing"

func sealedGuard(t testing.TB, manager *Manager, build func(*Work) Guard) Guard {
	t.Helper()
	work := manager.NewWork()
	root := build(work)
	work.Seal()
	return root
}

func TestReindexForgetAndSimultaneousSwap(t *testing.T) {
	manager, err := New([]Atom{testA, testB})
	if err != nil {
		t.Fatal(err)
	}
	all := manager.AllScope()
	onlyA, ok := manager.SealScope([]Atom{testA})
	if !ok {
		t.Fatal("target scope")
	}
	input := sealedGuard(t, manager, func(work *Work) Guard {
		return work.And(literal(t, work, testA), literal(t, work, testB))
	})
	forget, ok := manager.NewReindex(all, onlyA)
	if !ok || !forget.Identity(testA) || !forget.Forget(testB) {
		t.Fatal("forget builder")
	}
	forgetPlan, ok := forget.Seal()
	if !ok {
		t.Fatal("forget plan")
	}
	work := manager.NewWork()
	forgotten, ok := work.Reindex(input, forgetPlan)
	if !ok {
		t.Fatal("forget reindex")
	}
	work.Seal()
	expectedA := sealedGuard(t, manager, func(work *Work) Guard { return literal(t, work, testA) })
	if !manager.Equivalent(forgotten, expectedA) {
		t.Fatal("forget did not existentially close b")
	}

	swap, ok := manager.NewReindex(all, all)
	if !ok {
		t.Fatal("swap builder")
	}
	b := sealedGuard(t, manager, func(work *Work) Guard { return literal(t, work, testB) })
	a := sealedGuard(t, manager, func(work *Work) Guard { return literal(t, work, testA) })
	bExpr, ok := all.Expr(b)
	if !ok {
		t.Fatal("b expression")
	}
	aExpr, ok := all.Expr(a)
	if !ok || !swap.Set(testA, bExpr) || !swap.Set(testB, aExpr) {
		t.Fatal("swap map")
	}
	swapPlan, ok := swap.Seal()
	if !ok {
		t.Fatal("swap plan")
	}
	swapInput := sealedGuard(t, manager, func(work *Work) Guard {
		return work.And(literal(t, work, testA), work.Not(literal(t, work, testB)))
	})
	work = manager.NewWork()
	swapped, ok := work.Reindex(swapInput, swapPlan)
	if !ok {
		t.Fatal("swap reindex")
	}
	work.Seal()
	expectedSwap := sealedGuard(t, manager, func(work *Work) Guard {
		return work.And(literal(t, work, testB), work.Not(literal(t, work, testA)))
	})
	if !manager.Equivalent(swapped, expectedSwap) {
		t.Fatal("swap was not simultaneous")
	}
}

func TestReindexCompositionMatchesSequentialTransport(t *testing.T) {
	manager, err := New([]Atom{testA, testB})
	if err != nil {
		t.Fatal(err)
	}
	firstScope := manager.AllScope()
	middleScope, ok := manager.SealScope([]Atom{testA, testB})
	if !ok {
		t.Fatal("middle scope")
	}
	lastScope, ok := manager.SealScope([]Atom{testA})
	if !ok {
		t.Fatal("last scope")
	}
	first, ok := manager.NewReindex(firstScope, middleScope)
	if !ok || !first.Identity(testA) || !first.Identity(testB) {
		t.Fatal("first plan")
	}
	firstPlan, ok := first.Seal()
	if !ok {
		t.Fatal("seal first")
	}
	second, ok := manager.NewReindex(middleScope, lastScope)
	if !ok || !second.Identity(testA) || !second.Forget(testB) {
		t.Fatal("second plan")
	}
	secondPlan, ok := second.Seal()
	if !ok {
		t.Fatal("seal second")
	}
	composed, ok := manager.ComposeReindex(firstPlan, secondPlan)
	if !ok {
		t.Fatal("compose")
	}
	input := sealedGuard(t, manager, func(work *Work) Guard {
		return work.And(literal(t, work, testA), literal(t, work, testB))
	})
	firstWork := manager.NewWork()
	middle, ok := firstWork.Reindex(input, firstPlan)
	if !ok {
		t.Fatal("first transport")
	}
	firstWork.Seal()
	secondWork := manager.NewWork()
	sequential, ok := secondWork.Reindex(middle, secondPlan)
	if !ok {
		t.Fatal("second transport")
	}
	secondWork.Seal()
	composedWork := manager.NewWork()
	direct, ok := composedWork.Reindex(input, composed)
	if !ok {
		t.Fatal("composed transport")
	}
	composedWork.Seal()
	if !manager.Equivalent(sequential, direct) {
		t.Fatal("composed relation differs from sequential transport")
	}
}
