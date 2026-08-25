package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// branch_publication_law_test.go states the structural publication channel: a
// row whose output is an activation row set publishes the ORDINALS of the
// branches it settled, and nothing else.
//
// The ordinal is the only address a cold member set's rows have - it is what
// the relation's own Ordinal carrier names them by - so the channel mints no
// coordinate, names no Factor and resolves no member. Which mounted activation
// member an ordinal stands for was settled once, cold, by the engine that
// mounted it.

// TestAStructuralInvocationPublishesTheBranchesItSettled is the positive law.
// A structural row has no output slots at all, so its Concrete disposition
// carries no patch, and the branch set is the whole of what it published.
func TestAStructuralInvocationPublishesTheBranchesItSettled(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	for _, branch := range []int{2, 0, 5} {
		if !run.Activate(&ticket, branch) {
			t.Fatalf("branch %d was not staged", branch)
		}
	}
	if !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("a structural invocation did not submit")
	}
	disposition, patches, branches, drained := run.Consume()
	if !drained || disposition != structure.Concrete {
		t.Fatalf("structural drain = %v/%t", disposition, drained)
	}
	if len(patches) != 0 {
		t.Fatalf("a structural row staged %d patches", len(patches))
	}
	// The order is the order they were settled in. Canonical order is the
	// engine's own concern, over members rather than ordinals.
	if len(branches) != 3 || branches[0] != 2 || branches[1] != 0 || branches[2] != 5 {
		t.Fatalf("published branches = %v, want the three settled ordinals in settle order", branches)
	}
}

// TestOneBranchSettlesOnce is the fence that keeps the channel single-valued.
// Two dispositions for one branch is a disagreement no order between them
// could resolve, so the second staging is refused where it is made rather than
// silently replacing or duplicating the first.
func TestOneBranchSettlesOnce(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	if !run.Activate(&ticket, 4) {
		t.Fatal("the first staging of a branch was refused")
	}
	if run.Activate(&ticket, 4) {
		t.Fatal("one branch settled twice")
	}
	if run.Activate(&ticket, -1) {
		t.Fatal("a negative ordinal addressed a branch")
	}
	if !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("submit")
	}
	if _, _, branches, drained := run.Consume(); !drained || len(branches) != 1 {
		t.Fatalf("published branches = %d, want the one branch that settled", len(branches))
	}
}

// TestADispositionThatSettlesNothingPublishesNoBranch states that the branch
// set is a property of the Concrete disposition. An invocation that concluded
// no selection concluded no branch either, so anything staged before that
// conclusion is discarded rather than published under it.
func TestADispositionThatSettlesNothingPublishesNoBranch(t *testing.T) {
	for _, outcome := range []structure.ReductionOutcome{structure.NoCandidate, structure.NoSelection} {
		fixture := newExecutionFixture(t)
		run := NewRun(0, 0)
		ticket := issueExecution(t, run, fixture)
		if !run.Activate(&ticket, 1) {
			t.Fatal("staging")
		}
		if !run.Submit(&ticket, outcome) {
			t.Fatalf("submit %v", outcome)
		}
		disposition, _, branches, drained := run.Consume()
		if !drained || disposition != outcome || len(branches) != 0 {
			t.Fatalf("outcome %v published %d branches", outcome, len(branches))
		}
	}
}

// TestAnAbortedInvocationPublishesNoBranch keeps the fail-closed edge whole: an
// abandoned invocation publishes nothing through either channel.
func TestAnAbortedInvocationPublishesNoBranch(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	if !run.Activate(&ticket, 3) || !run.Abort() {
		t.Fatal("stage and abort")
	}
	if _, _, _, drained := run.Consume(); drained {
		t.Fatal("an aborted invocation drained a result")
	}
	next := issueExecution(t, run, fixture)
	if !run.Submit(&next, structure.Concrete) {
		t.Fatal("submit")
	}
	if _, _, branches, drained := run.Consume(); !drained || len(branches) != 0 {
		t.Fatalf("a fresh invocation inherited %d branches", len(branches))
	}
}

// TestTheBranchSetIsNotStagedOutsideALiveInvocation states the same lifetime
// fence every other publication is held to: the channel is reachable only
// through a live ticket.
func TestTheBranchSetIsNotStagedOutsideALiveInvocation(t *testing.T) {
	fixture := newExecutionFixture(t)
	run := NewRun(0, 0)
	ticket := issueExecution(t, run, fixture)
	if !run.Submit(&ticket, structure.Concrete) {
		t.Fatal("submit")
	}
	if run.Activate(&ticket, 0) {
		t.Fatal("a submitted invocation staged another branch")
	}
	var foreign Ticket
	if run.Activate(&foreign, 0) {
		t.Fatal("a zero ticket staged a branch")
	}
}
