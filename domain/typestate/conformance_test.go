package typestate

import "testing"

func testObligation(t *testing.T, states ...State) Obligation {
	t.Helper()
	obligation, ok := NewObligation(states...)
	if !ok {
		t.Fatal("NewObligation rejected valid states")
	}
	return obligation
}

func conformanceFixture() Definition {
	return Definition{
		Protocol:    "transaction",
		States:      []State{"active", "finished"},
		FinalStates: []State{"finished"},
		Transitions: []TransitionDecl{{From: "active", To: "finished"}},
	}
}

func TestDefinitionAdmitsAcquireAcceptsDeclaredStateAndObligation(t *testing.T) {
	def := conformanceFixture()
	if err := def.AdmitsAcquire("active", testObligation(t, "finished")); err != nil {
		t.Fatalf("AdmitsAcquire(active, finished) = %v, want admitted", err)
	}
	if err := def.AdmitsAcquire("active", Obligation{}); err != nil {
		t.Fatalf("AdmitsAcquire(active, no obligation) = %v, want admitted", err)
	}
}

func TestDefinitionAdmitsAcquireRejectsUndeclaredState(t *testing.T) {
	def := conformanceFixture()
	err := def.AdmitsAcquire("pending", testObligation(t, "finished"))
	if err == nil {
		t.Fatal("AdmitsAcquire(pending) = nil, want rejection")
	}
	if got, want := err.Error(), `protocol "transaction" does not declare acquire state "pending"`; got != want {
		t.Fatalf("AdmitsAcquire(pending) = %q, want %q", got, want)
	}
}

func TestDefinitionAdmitsAcquireRejectsNonFinalObligation(t *testing.T) {
	def := conformanceFixture()
	err := def.AdmitsAcquire("active", testObligation(t, "active"))
	if err == nil {
		t.Fatal("AdmitsAcquire(obligation active) = nil, want rejection")
	}
	if got, want := err.Error(), `protocol "transaction" does not declare obligation final state "active"`; got != want {
		t.Fatalf("AdmitsAcquire(obligation active) = %q, want %q", got, want)
	}
}

func TestDefinitionAdmitsTransitionAcceptsDeclaredEdge(t *testing.T) {
	def := conformanceFixture()
	if err := def.AdmitsTransition("active", "finished"); err != nil {
		t.Fatalf("AdmitsTransition(active -> finished) = %v, want admitted", err)
	}
}

func TestDefinitionAdmitsTransitionRejectsUndeclaredEndpoints(t *testing.T) {
	def := conformanceFixture()

	err := def.AdmitsTransition("active", "aborted")
	if err == nil {
		t.Fatal("AdmitsTransition(-> aborted) = nil, want rejection")
	}
	if got, want := err.Error(), `protocol "transaction" does not declare transition target state "aborted"`; got != want {
		t.Fatalf("AdmitsTransition(-> aborted) = %q, want %q", got, want)
	}

	err = def.AdmitsTransition("pending", "finished")
	if err == nil {
		t.Fatal("AdmitsTransition(pending ->) = nil, want rejection")
	}
	if got, want := err.Error(), `protocol "transaction" does not declare transition source state "pending"`; got != want {
		t.Fatalf("AdmitsTransition(pending ->) = %q, want %q", got, want)
	}
}

func TestDefinitionAdmitsTransitionRejectsMissingEdge(t *testing.T) {
	def := conformanceFixture()
	err := def.AdmitsTransition("finished", "active")
	if err == nil {
		t.Fatal("AdmitsTransition(finished -> active) = nil, want rejection")
	}
	if got, want := err.Error(), `protocol "transaction" does not declare transition "finished" -> "active"`; got != want {
		t.Fatalf("AdmitsTransition(finished -> active) = %q, want %q", got, want)
	}
}
