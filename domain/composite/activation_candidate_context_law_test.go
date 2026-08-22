package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// Every admitted mounted activation candidate carries the execution-context
// edge its body route runs on. The tuple names one sealed directory
// Transition whose endpoints are the trigger module's Context and the body
// module's Context. For a body that lives in the trigger's own module that
// edge is the directory's canonical reflexive local edge, which Seal issues
// for every Context; the producer resolves it rather than leaving the tuple
// for a later consumer to infer.
func TestMountedActivationCandidatesCarryTheirDirectoryEdge(t *testing.T) {
	record := mountedRecord(t, "activation-candidate-context", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("sealed rule binding")
	}
	contexts := record.Source.ContextDirectory()
	if !contexts.Available() {
		t.Fatal("sealed execution-context directory")
	}
	_, activations, failed := rules.MountedAdmissions(record.Artifacts, contexts)
	if failed.Available() {
		t.Fatalf("mounted admissions refused: %s", failed)
	}
	observed := 0
	for index, admit := range activations {
		triggerContexts := lawModuleContexts(t, contexts, admit.Mount)
		for candidateIndex, candidate := range admit.Candidates {
			observed++
			from, fromOK := contexts.Context(candidate.FromContextID)
			to, toOK := contexts.Context(candidate.ToContextID)
			transition, transitionOK := contexts.Transition(candidate.FromContextID, candidate.ToContextID)
			if !fromOK || !toOK || !transitionOK || transition.ID() != candidate.TransitionID {
				t.Fatalf("activation %d candidate %d carries a tuple the directory does not resolve: from=%t to=%t transition=%t transitionID=%t",
					index, candidateIndex, fromOK, toOK, transitionOK, transitionOK && transition.ID() == candidate.TransitionID)
			}
			if from.ModuleKey() != admit.Mount {
				t.Fatalf("activation %d candidate %d departs a foreign module context", index, candidateIndex)
			}
			if to.ModuleKey() != candidate.Mount {
				t.Fatalf("activation %d candidate %d arrives outside its body module", index, candidateIndex)
			}
			if candidate.Mount != admit.Mount {
				continue
			}
			// The intra-module route is the reflexive local edge, so both
			// endpoints are one of the trigger module's own contexts.
			if candidate.FromContextID != candidate.ToContextID {
				t.Fatalf("activation %d candidate %d took a cross-context edge for a body in its own module", index, candidateIndex)
			}
			reflexive := false
			for _, context := range triggerContexts {
				reflexive = reflexive || context.ID() == candidate.FromContextID
			}
			if !reflexive {
				t.Fatalf("activation %d candidate %d names a reflexive edge outside its module's contexts", index, candidateIndex)
			}
		}
	}
	if observed == 0 {
		t.Fatal("fixture admitted no activation candidate")
	}
}

func lawModuleContexts(t *testing.T, directory executioncontext.Directory, module identity.ContentID) []executioncontext.Context {
	t.Helper()
	rows, ok := directory.ContextsForModule(module)
	if !ok || len(rows) == 0 {
		t.Fatalf("module %x holds no sealed execution context", module[:4])
	}
	return rows
}
