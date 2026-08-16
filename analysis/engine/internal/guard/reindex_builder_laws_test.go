package guard

import "testing"

func TestReindexBuilderIncompleteSealKeepsCandidateOpen(t *testing.T) {
	manager := newTestManager(t)
	all := manager.AllScope()
	builder, ok := manager.NewReindex(all, all)
	if !ok || !builder.Identity(testA) {
		t.Fatal("identity construction")
	}
	if builder.work == nil || !builder.work.Open() {
		t.Fatal("identity did not create an open builder work")
	}
	unsealed := builder.entries[0].low
	if manager.Valid(unsealed) {
		t.Fatal("candidate entry was published before Seal")
	}
	if _, ok := builder.Seal(); ok {
		t.Fatal("incomplete relation sealed")
	}
	if builder.sealed || builder.work == nil || !builder.work.Open() {
		t.Fatal("incomplete Seal closed the builder candidate")
	}
	if builder.entries[0].low != unsealed {
		t.Fatal("incomplete Seal changed an admitted entry")
	}
	if !builder.Forget(testB) || !builder.Identity(testC) {
		t.Fatal("completion after incomplete Seal")
	}
	plan, ok := builder.Seal()
	if !ok || !plan.Valid() || !builder.sealed || !builder.work.Published() {
		t.Fatal("completed relation did not publish once")
	}
	if !manager.Valid(plan.value.entries[0].low) {
		t.Fatal("published plan retained an unsealed entry")
	}
}

func TestReindexBuilderBatchesMixedEntriesAndSealsPlanOwnership(t *testing.T) {
	manager := newTestManager(t)
	all := manager.AllScope()
	expressionWork := manager.NewWork()
	expressionRoot := literal(t, expressionWork, testB)
	expressionWork.Seal()
	expression, ok := all.Expr(expressionRoot)
	if !ok {
		t.Fatal("sealed Set expression")
	}
	builder, ok := manager.NewReindex(all, all)
	if !ok || !builder.Forget(testA) || !builder.Set(testB, expression) || !builder.Identity(testC) {
		t.Fatal("mixed relation construction")
	}
	if builder.work == nil || !builder.work.Open() {
		t.Fatal("Set/Identity did not share an open builder work")
	}
	if manager.Valid(builder.entries[1].low) || manager.Valid(builder.entries[2].low) {
		t.Fatal("builder-owned entries were published before Seal")
	}
	plan, ok := builder.Seal()
	if !ok || !plan.Valid() || !builder.work.Published() {
		t.Fatal("mixed relation did not publish")
	}
	for index, entry := range plan.value.entries {
		if !manager.Valid(entry.low) || !manager.Valid(entry.high) {
			t.Fatalf("plan entry %d escaped without sealed ownership", index)
		}
	}
}

func TestReindexBuildersOwnIsolatedCandidateWorks(t *testing.T) {
	manager := newTestManager(t)
	all := manager.AllScope()
	first, ok := manager.NewReindex(all, all)
	if !ok || !first.Identity(testA) || !first.Forget(testB) || !first.Forget(testC) {
		t.Fatal("first relation construction")
	}
	second, ok := manager.NewReindex(all, all)
	if !ok || !second.Identity(testA) {
		t.Fatal("second relation construction")
	}
	if first.work == nil || second.work == nil || first.work == second.work {
		t.Fatal("builders shared candidate work")
	}
	firstPlan, ok := first.Seal()
	if !ok || !firstPlan.Valid() {
		t.Fatal("first relation seal")
	}
	if !second.work.Open() || second.sealed {
		t.Fatal("sealing one builder affected another")
	}
	if !second.Forget(testB) || !second.Forget(testC) {
		t.Fatal("second relation completion")
	}
	if _, ok := second.Seal(); !ok {
		t.Fatal("second relation seal")
	}
}

func TestReindexBuilderAllForgetNeedsNoWork(t *testing.T) {
	manager := newTestManager(t)
	all := manager.AllScope()
	builder, ok := manager.NewReindex(all, all)
	if !ok || !builder.Forget(testA) || !builder.Forget(testB) || !builder.Forget(testC) {
		t.Fatal("all-Forget relation construction")
	}
	if builder.work != nil {
		t.Fatal("Forget allocated a builder Work")
	}
	plan, ok := builder.Seal()
	if !ok || !plan.Valid() {
		t.Fatal("all-Forget relation seal")
	}
	if builder.work != nil {
		t.Fatal("all-Forget relation allocated Work at Seal")
	}
}
