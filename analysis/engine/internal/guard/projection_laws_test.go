package guard

import "testing"

func TestReindexProjectionProofIsExact(t *testing.T) {
	manager := newTestManager(t)
	all := manager.AllScope()

	identity, ok := manager.NewReindex(all, all)
	if !ok || !identity.Identity(testA) || !identity.Identity(testB) || !identity.Identity(testC) {
		t.Fatal("identity relation construction")
	}
	identityPlan, ok := identity.Seal()
	if !ok || !identityPlan.PureProjection() {
		t.Fatal("identity relation did not seal as a pure projection")
	}
	for _, atom := range []Atom{testA, testB, testC} {
		action, valid := identityPlan.ProjectionAction(atom)
		if !valid || !action.RetainsCoordinate() || action.ForgetsCoordinate() {
			t.Fatalf("identity projection action for %d = %#v/%t", atom, action, valid)
		}
	}

	forget, ok := manager.NewReindex(all, all)
	if !ok || !forget.Identity(testA) || !forget.Forget(testB) || !forget.Identity(testC) {
		t.Fatal("forget relation construction")
	}
	forgetPlan, ok := forget.Seal()
	if !ok || !forgetPlan.PureProjection() {
		t.Fatal("forget relation did not seal as a pure projection")
	}
	if action, valid := forgetPlan.ProjectionAction(testA); !valid || !action.RetainsCoordinate() {
		t.Fatal("retained projection action was not reported")
	}
	if action, valid := forgetPlan.ProjectionAction(testB); !valid || !action.ForgetsCoordinate() {
		t.Fatal("forgotten projection action was not reported")
	}

	work := manager.NewWork()
	expressionRoot, valid := work.Literal(testA)
	if !valid {
		t.Fatal("same-coordinate set expression")
	}
	work.Seal()
	expression, valid := all.Expr(expressionRoot)
	if !valid {
		t.Fatal("same-coordinate set expression scope")
	}
	set, ok := manager.NewReindex(all, all)
	if !ok || !set.Set(testA, expression) || !set.Identity(testB) || !set.Identity(testC) {
		t.Fatal("set relation construction")
	}
	setPlan, ok := set.Seal()
	if !ok || setPlan.PureProjection() {
		t.Fatal("Set relation was incorrectly accepted as a pure projection")
	}
	if _, valid := setPlan.ProjectionAction(testA); valid {
		t.Fatal("general Set relation exposed a projection action")
	}

	mixedWork := manager.NewWork()
	mixedExpressionRoot, valid := mixedWork.Literal(testB)
	if !valid {
		t.Fatal("mixed set expression")
	}
	mixedWork.Seal()
	mixedExpression, valid := all.Expr(mixedExpressionRoot)
	if !valid {
		t.Fatal("mixed set expression scope")
	}
	mixed, ok := manager.NewReindex(all, all)
	if !ok || !mixed.Identity(testA) || !mixed.Set(testB, mixedExpression) || !mixed.Forget(testC) {
		t.Fatal("mixed relation construction")
	}
	mixedPlan, ok := mixed.Seal()
	if !ok || mixedPlan.PureProjection() {
		t.Fatal("mixed Set relation was incorrectly accepted as a pure projection")
	}
}

func TestComposedReindexConservativelyDropsProjectionProof(t *testing.T) {
	manager := newTestManager(t)
	all := manager.AllScope()
	firstBuilder, ok := manager.NewReindex(all, all)
	if !ok || !firstBuilder.Identity(testA) || !firstBuilder.Forget(testB) || !firstBuilder.Identity(testC) {
		t.Fatal("first projection construction")
	}
	first, ok := firstBuilder.Seal()
	if !ok || !first.PureProjection() {
		t.Fatal("first projection seal")
	}
	secondBuilder, ok := manager.NewReindex(all, all)
	if !ok || !secondBuilder.Identity(testA) || !secondBuilder.Identity(testB) || !secondBuilder.Identity(testC) {
		t.Fatal("second projection construction")
	}
	second, ok := secondBuilder.Seal()
	if !ok || !second.PureProjection() {
		t.Fatal("second projection seal")
	}
	composed, ok := manager.ComposeReindex(first, second)
	if !ok {
		t.Fatal("compose projection relations")
	}
	if composed.PureProjection() {
		t.Fatal("composition claimed an unproven pure projection")
	}
}
