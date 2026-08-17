package guard

import "testing"

// coordinateIdentityPlan seals the complete identity relation between two
// separately issued scopes carrying the same coordinates.
func coordinateIdentityPlan(t *testing.T, manager *Manager, source, target Scope, atoms ...Atom) Reindex {
	t.Helper()
	builder, ok := manager.NewReindex(source, target)
	if !ok {
		t.Fatal("coordinate identity builder")
	}
	for _, atom := range atoms {
		if !builder.Identity(atom) {
			t.Fatalf("identity entry for atom %d", atom)
		}
	}
	plan, ok := builder.Seal()
	if !ok || !plan.CoordinateIdentity() {
		t.Fatal("coordinate identity seal")
	}
	return plan
}

// TestComposeReindexCarriesCoordinateIdentityThroughIssuedScopes proves that
// composing two coordinate-identical relations yields the coordinate identity
// Seal would derive for the same source and target rank vectors, and that the
// composite is a full Identity exactly when its source and target are the one
// issued scope.
func TestComposeReindexCarriesCoordinateIdentityThroughIssuedScopes(t *testing.T) {
	manager := newTestManager(t)
	first, ok := manager.SealScope([]Atom{testA, testB})
	if !ok {
		t.Fatal("first scope")
	}
	second, ok := manager.SealScope([]Atom{testA, testB})
	if !ok {
		t.Fatal("second scope")
	}
	third, ok := manager.SealScope([]Atom{testA, testB})
	if !ok {
		t.Fatal("third scope")
	}
	if first.Same(second) || second.Same(third) || first.Same(third) {
		t.Fatal("scopes were not separately issued")
	}

	forward := coordinateIdentityPlan(t, manager, first, second, testA, testB)
	onward := coordinateIdentityPlan(t, manager, second, third, testA, testB)
	composite, ok := manager.ComposeReindex(forward, onward)
	if !ok || !composite.Valid() {
		t.Fatal("coordinate identity composition")
	}
	if !composite.CoordinateIdentity() {
		t.Fatal("composition of two coordinate identities lost CoordinateIdentity")
	}
	if composite.Identity() {
		t.Fatal("composition across separately issued scopes claimed full Identity")
	}
	if !composite.Source().Same(first) || !composite.Target().Same(third) {
		t.Fatal("composition did not carry its outer scopes")
	}

	backward := coordinateIdentityPlan(t, manager, second, first, testA, testB)
	closed, ok := manager.ComposeReindex(forward, backward)
	if !ok || !closed.Valid() {
		t.Fatal("closed coordinate identity composition")
	}
	if !closed.CoordinateIdentity() || !closed.Identity() {
		t.Fatal("composition returning to its issued source scope lost Identity")
	}
}

// TestComposeReindexDeniesCoordinateIdentityToForgetfulRelation proves the
// derived proof is exact: an existentially forgotten coordinate is not a
// coordinate identity, and composing it with one cannot become one.
func TestComposeReindexDeniesCoordinateIdentityToForgetfulRelation(t *testing.T) {
	manager := newTestManager(t)
	first, ok := manager.SealScope([]Atom{testA, testB})
	if !ok {
		t.Fatal("first scope")
	}
	second, ok := manager.SealScope([]Atom{testA, testB})
	if !ok {
		t.Fatal("second scope")
	}
	third, ok := manager.SealScope([]Atom{testA, testB})
	if !ok {
		t.Fatal("third scope")
	}
	builder, ok := manager.NewReindex(first, second)
	if !ok || !builder.Identity(testA) || !builder.Forget(testB) {
		t.Fatal("forgetful relation construction")
	}
	forgetful, ok := builder.Seal()
	if !ok || forgetful.CoordinateIdentity() {
		t.Fatal("forgetful relation claimed CoordinateIdentity")
	}
	onward := coordinateIdentityPlan(t, manager, second, third, testA, testB)
	composite, ok := manager.ComposeReindex(forgetful, onward)
	if !ok || !composite.Valid() {
		t.Fatal("forgetful composition")
	}
	if composite.CoordinateIdentity() || composite.Identity() {
		t.Fatal("composition through a forgotten coordinate claimed an identity")
	}
}
