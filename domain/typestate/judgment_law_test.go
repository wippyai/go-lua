package typestate

import "testing"

// connectionProtocol and transactionProtocol are the two machines the
// declared-resource-lifecycle fixture's host manifest declares. The laws below
// judge that fixture's own cases against them, so a law here states the same
// contract the fixture states rather than a restatement of this package's code.
func connectionProtocol() Definition {
	return Definition{
		Protocol:    "connection",
		States:      []State{"open", "closed"},
		FinalStates: []State{"closed"},
		Transitions: []TransitionDecl{{From: "open", To: "closed"}},
	}
}

func transactionProtocol() Definition {
	return Definition{
		Protocol:    "transaction",
		States:      []State{"active", "committed"},
		FinalStates: []State{"committed"},
		Transitions: []TransitionDecl{{From: "active", To: "committed"}},
	}
}

func mustState(t *testing.T, states ...State) StateSet {
	t.Helper()
	set, ok := NewStateSet(states...)
	if !ok {
		t.Fatalf("NewStateSet(%v) refused a valid set", states)
	}
	return set
}

func mustExactly(t *testing.T, state State) Abstract {
	t.Helper()
	solved, ok := Exactly(state)
	if !ok {
		t.Fatalf("Exactly(%q) refused a declared state", state)
	}
	return solved
}

func mustObligation(t *testing.T, states ...State) Obligation {
	t.Helper()
	obligation, ok := NewObligation(states...)
	if !ok {
		t.Fatalf("NewObligation(%v) refused a valid obligation", states)
	}
	return obligation
}

// TestJoinIsTheStateUnionAndUnknownAbsorbs states the merge law: two
// control-flow predecessors that solve different states reach their successor
// with both live, and a predecessor that proves nothing makes the merge prove
// nothing. This is what leak_on_some_path's exit depends on.
func TestJoinIsTheStateUnionAndUnknownAbsorbs(t *testing.T) {
	open, closed := mustExactly(t, "open"), mustExactly(t, "closed")
	merged := open.Join(closed)
	if merged.IsUnknown() {
		t.Fatal("joining two proven states lost the proof")
	}
	if got := merged.States(); got != mustState(t, "open", "closed") {
		t.Fatalf("join = %v, want the union of both states", got.List())
	}
	if !open.Join(Unknown()).IsUnknown() || !Unknown().Join(open).IsUnknown() {
		t.Fatal("an unproven predecessor did not absorb the merge")
	}
	if got := open.Join(Unreachable()); got != open {
		t.Fatalf("join with the unreachable element = %v, want the other operand unchanged", got.States().List())
	}
	if !open.LessOrEqual(merged) || !closed.LessOrEqual(merged) || !merged.LessOrEqual(Unknown()) {
		t.Fatal("join is not an upper bound of its operands")
	}
	if merged.LessOrEqual(open) {
		t.Fatal("a two-state merge compared below one of its members")
	}
}

// TestJoinIsCommutativeAssociativeAndIdempotent states the lattice laws the
// engine's fixed point relies on for termination and for order independence.
func TestJoinIsCommutativeAssociativeAndIdempotent(t *testing.T) {
	elements := []Abstract{
		Unreachable(), Unknown(),
		mustExactly(t, "open"), mustExactly(t, "closed"),
		Possibly(mustState(t, "open", "closed")),
	}
	for _, left := range elements {
		if got := left.Join(left); got != left {
			t.Fatalf("join is not idempotent at %v", left.States().List())
		}
		for _, right := range elements {
			if left.Join(right) != right.Join(left) {
				t.Fatal("join is not commutative")
			}
			for _, third := range elements {
				if left.Join(right).Join(third) != left.Join(right.Join(third)) {
					t.Fatal("join is not associative")
				}
			}
		}
	}
}

// TestUnknownProvesAndRefutesNothing is the soundness fence: an unproven state
// never certifies a requirement and never convicts one either.
func TestUnknownProvesAndRefutesNothing(t *testing.T) {
	if Unknown().Proves("open") || Unknown().Refutes("open") {
		t.Fatal("the unproven element answered a question about a state")
	}
	if !Unknown().States().Empty() {
		t.Fatal("the unproven element published a state set as if it were a proof")
	}
	merged := Possibly(mustState(t, "open", "closed"))
	if merged.Proves("open") || merged.Proves("closed") {
		t.Fatal("a set containing a state was read as a proof of that state")
	}
	if merged.Refutes("open") || merged.Refutes("closed") {
		t.Fatal("a set containing a state refuted it")
	}
}

// TestOpaqueEscapeDischargesEveryProof states the escape law: an operation
// declared to hand the resource somewhere unfollowed leaves nothing proven,
// which is why declared-resource-lifecycle's opaque_handoff reports nothing.
func TestOpaqueEscapeDischargesEveryProof(t *testing.T) {
	definition := connectionProtocol()
	for _, solved := range []Abstract{
		mustExactly(t, "open"), mustExactly(t, "closed"),
		Possibly(mustState(t, "open", "closed")), Unknown(), Unreachable(),
	} {
		if !definition.Escape(solved).IsUnknown() {
			t.Fatalf("escape from %v kept a proof", solved.States().List())
		}
	}
	if JudgeExit(definition.Escape(mustExactly(t, "open")), mustObligation(t, "closed")) != VerdictAbstain {
		t.Fatal("an escaped resource was judged as a leak")
	}
}

// TestStepMovesOnlyTheDeclaredEdge states the transition law. A member the
// edge admits moves; one it does not is left in place, so double_commit's
// second commit does not also lose the committed state that convicts it.
func TestStepMovesOnlyTheDeclaredEdge(t *testing.T) {
	definition := connectionProtocol()
	if got := definition.Step(mustExactly(t, "open"), "open", "closed"); !got.Proves("closed") {
		t.Fatalf("close from open = %v, want a proof of closed", got.States().List())
	}
	if got := definition.Step(mustExactly(t, "closed"), "open", "closed"); !got.Proves("closed") {
		t.Fatalf("close from closed = %v, want the state left in place", got.States().List())
	}
	if got := definition.Step(Possibly(mustState(t, "open", "closed")), "open", "closed"); !got.Proves("closed") {
		t.Fatalf("close over a merged state = %v, want every member at closed", got.States().List())
	}
	if !definition.Step(Unknown(), "open", "closed").IsUnknown() {
		t.Fatal("stepping an unproven state manufactured a proof")
	}
	if got := definition.Step(mustExactly(t, "open"), "closed", "open"); !got.Proves("open") {
		t.Fatal("an undeclared edge moved the state")
	}
}

// TestRequirementJudgmentMatchesTheDeclaredResourceFixture states the
// requirement cases the declared-resource-lifecycle fixture authors:
// use_after_close and alias_close_propagates convict, query_unknown_state is
// unproven, and the clean-path fixture's query-while-open conforms.
func TestRequirementJudgmentMatchesTheDeclaredResourceFixture(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		solved Abstract
		want   Verdict
	}{
		{name: "query after close", solved: mustExactly(t, "closed"), want: VerdictInvalidRequirement},
		{name: "query on an unknown parameter", solved: Unknown(), want: VerdictUnprovenRequirement},
		{name: "query while open", solved: mustExactly(t, "open"), want: VerdictConforms},
		{name: "query on a merged state", solved: Possibly(mustState(t, "open", "closed")), want: VerdictUnprovenRequirement},
		{name: "query at an unreachable point", solved: Unreachable(), want: VerdictAbstain},
	} {
		if got := JudgeRequirement(testCase.solved, "open"); got != testCase.want {
			t.Fatalf("%s = %s, want %s", testCase.name, got.Spelling(), testCase.want.Spelling())
		}
	}
}

// TestTransitionJudgmentMatchesTheDeclaredResourceFixture states the
// transition cases: double_commit convicts, the correct ordering conforms, and
// an unproven source state is withheld rather than convicted or certified.
func TestTransitionJudgmentMatchesTheDeclaredResourceFixture(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		solved Abstract
		want   Verdict
	}{
		{name: "second commit", solved: mustExactly(t, "committed"), want: VerdictInvalidTransition},
		{name: "first commit", solved: mustExactly(t, "active"), want: VerdictConforms},
		{name: "commit on an unknown transaction", solved: Unknown(), want: VerdictUnprovenTransition},
		{name: "commit on a merged state", solved: Possibly(mustState(t, "active", "committed")), want: VerdictUnprovenTransition},
	} {
		if got := JudgeTransition(testCase.solved, "active"); got != testCase.want {
			t.Fatalf("%s = %s, want %s", testCase.name, got.Spelling(), testCase.want.Spelling())
		}
	}
	_ = transactionProtocol()
}

// TestExitJudgmentMatchesTheDeclaredResourceFixture states the obligation
// cases the resource fixtures author. The two reporting verdicts are exactly
// the two messages those fixtures expect: one names the single remaining
// state, the other cannot name one.
func TestExitJudgmentMatchesTheDeclaredResourceFixture(t *testing.T) {
	obligation := mustObligation(t, "closed")
	for _, testCase := range []struct {
		name   string
		solved Abstract
		want   Verdict
	}{
		{name: "leak on the pcall error path", solved: mustExactly(t, "open"), want: VerdictUnreleasedState},
		{name: "leak on one branch only", solved: Possibly(mustState(t, "open", "closed")), want: VerdictUnreleasedNonFinal},
		{name: "closed on every path", solved: mustExactly(t, "closed"), want: VerdictConforms},
		{name: "handed to an opaque callee", solved: Unknown(), want: VerdictAbstain},
		{name: "unreachable exit", solved: Unreachable(), want: VerdictAbstain},
	} {
		if got := JudgeExit(testCase.solved, obligation); got != testCase.want {
			t.Fatalf("%s = %s, want %s", testCase.name, got.Spelling(), testCase.want.Spelling())
		}
	}
	if JudgeExit(mustExactly(t, "open"), Obligation{}) != VerdictAbstain {
		t.Fatal("a protocol with no obligation reported a leak")
	}
}

// TestChannelLifecycleJudgmentMatchesTheChannelFixture states the channel
// fixture's cases against the same kernel. send is a requirement on open and
// close is a transition from open, so the two channel codes are two
// publications of verdicts this package already decides.
func TestChannelLifecycleJudgmentMatchesTheChannelFixture(t *testing.T) {
	channel := Definition{
		Protocol:    "channel",
		States:      []State{"open", "closed"},
		Transitions: []TransitionDecl{{From: "open", To: "closed"}},
	}
	if err := channel.Validate(); err != nil {
		t.Fatalf("the channel machine is malformed: %v", err)
	}
	closed := mustExactly(t, "closed")
	if got := JudgeRequirement(closed, "open"); got != VerdictInvalidRequirement {
		t.Fatalf("send after close = %s, want %s", got.Spelling(), VerdictInvalidRequirement.Spelling())
	}
	if got := JudgeTransition(closed, "open"); got != VerdictInvalidTransition {
		t.Fatalf("close after close = %s, want %s", got.Spelling(), VerdictInvalidTransition.Spelling())
	}
	open := mustExactly(t, "open")
	if JudgeRequirement(open, "open") != VerdictConforms || JudgeTransition(open, "open") != VerdictConforms {
		t.Fatal("the clean channel sequence did not conform")
	}
	escaped := channel.Escape(closed)
	if JudgeRequirement(escaped, "open") != VerdictUnprovenRequirement {
		t.Fatal("a send after an escaped close was convicted on an unproven state")
	}
	// A channel declares no final state, so no exit obligation exists and the
	// clean-path fixture's channels are not leaks.
	if JudgeExit(open, Obligation{}) != VerdictAbstain {
		t.Fatal("a channel with no declared obligation reported a leak")
	}
}

// TestVerdictCatalogIsClosedAndDense states the vocabulary law: every declared
// verdict has a distinct dense ordinal and a distinct spelling, so a
// declaration surface can key a published variant by ordinal.
func TestVerdictCatalogIsClosedAndDense(t *testing.T) {
	catalog := Catalog()
	if len(catalog) == 0 {
		t.Fatal("the verdict catalog is empty")
	}
	spellings := make(map[string]Verdict, len(catalog))
	for index, verdict := range catalog {
		if !verdict.Available() {
			t.Fatalf("catalog member %d is not a declared verdict", index)
		}
		if got := verdict.Ordinal(); got != uint16(index)+1 {
			t.Fatalf("verdict %s has ordinal %d at position %d", verdict.Spelling(), got, index)
		}
		spelling := verdict.Spelling()
		if spelling == "" {
			t.Fatalf("verdict at ordinal %d has no spelling", verdict.Ordinal())
		}
		if previous, duplicate := spellings[spelling]; duplicate {
			t.Fatalf("verdicts %d and %d share the spelling %q", previous.Ordinal(), verdict.Ordinal(), spelling)
		}
		spellings[spelling] = verdict
	}
	if VerdictInvalid.Available() || VerdictInvalid.Spelling() != "" || VerdictInvalid.Ordinal() != 0 {
		t.Fatal("the zero value is a declared verdict")
	}
	if VerdictConforms.Reports() || VerdictAbstain.Reports() {
		t.Fatal("a clean or withheld answer is published as a finding")
	}
	for _, verdict := range []Verdict{
		VerdictInvalidRequirement, VerdictUnprovenRequirement,
		VerdictInvalidTransition, VerdictUnprovenTransition,
		VerdictUnreleasedState, VerdictUnreleasedNonFinal,
	} {
		if !verdict.Reports() {
			t.Fatalf("verdict %s is a finding but is not published as one", verdict.Spelling())
		}
	}
}

// TestObligationIsTheCanonicalStateSet states that the obligation role and the
// solved-state role share one set representation, so an obligation authored in
// any order compares equal and is testable against a solved state directly.
func TestObligationIsTheCanonicalStateSet(t *testing.T) {
	forward := mustObligation(t, "closed", "aborted")
	reverse := mustObligation(t, "aborted", "closed")
	if forward != reverse {
		t.Fatal("obligation identity depends on authored order")
	}
	if forward.FinalStates() != mustState(t, "aborted", "closed") {
		t.Fatal("an obligation and a state set built from the same members differ")
	}
	if !forward.SatisfiedBy("closed") || forward.SatisfiedBy("open") || forward.SatisfiedBy("") {
		t.Fatal("obligation membership is not the set's membership")
	}
	if !(Obligation{}).Empty() || len((Obligation{}).FinalStateList()) != 0 {
		t.Fatal("the zero obligation is not the empty obligation")
	}
	if _, ok := NewObligation("closed", ""); ok {
		t.Fatal("an obligation admitted an unnamed state")
	}
}

// TestRequirementConformanceIsDecidedByTheStateMachine states the declaration
// law for the read arm: a requirement may name any declared state and nothing
// else, and admitting one neither adds an edge nor discharges an obligation.
func TestRequirementConformanceIsDecidedByTheStateMachine(t *testing.T) {
	definition := connectionProtocol()
	for _, state := range definition.States {
		if err := definition.AdmitsRequire(state); err != nil {
			t.Fatalf("declared state %q was refused as a requirement: %v", state, err)
		}
	}
	for _, state := range []State{"draining", ""} {
		if err := definition.AdmitsRequire(state); err == nil {
			t.Fatalf("undeclared state %q was admitted as a requirement", state)
		}
	}
	// A requirement is not a self-edge: admitting one must not make the state
	// machine allow a transition it never declared.
	if definition.AllowsTransition("open", "open") || definition.AllowsTransition("closed", "closed") {
		t.Fatal("the state machine allows a self transition")
	}
	// A required state that is also final is still only a read, so an exit
	// obligation is unchanged by it.
	if err := definition.AdmitsRequire("closed"); err != nil {
		t.Fatalf("a final state was refused as a requirement: %v", err)
	}
	if JudgeExit(mustExactly(t, "open"), mustObligation(t, "closed")) != VerdictUnreleasedState {
		t.Fatal("requiring a state changed the exit obligation")
	}
}
