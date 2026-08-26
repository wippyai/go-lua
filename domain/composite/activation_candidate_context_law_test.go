package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// activationDeclaringRuleKeys is the set of rule keys whose declaration states
// an activation, derived from the sealed catalog. Renaming a rule, or
// declaring a second activation, changes the set without any law below being
// edited: no law here spells a rule.
func activationDeclaringRuleKeys(t *testing.T, compilation Compilation) map[schema.Key]struct{} {
	t.Helper()
	keys := make(map[schema.Key]struct{}, RuleCount(compilation))
	for position := 0; position < RuleCount(compilation); position++ {
		key, keyOK := RuleKeyAt(compilation, position)
		if !keyOK || !key.Available() {
			t.Fatalf("rule key at position %d", position)
		}
		template, templateOK := templateForKey(compilation.catalog, key)
		if !templateOK || template == nil {
			t.Fatalf("rule %q names no sealed template", key)
		}
		if template.Program().ActivationRole.Available() {
			keys[key] = struct{}{}
		}
	}
	if len(keys) == 0 {
		t.Fatal("the sealed rule inventory declares no activation")
	}
	return keys
}

// activationPlacementCount counts the occurrences the artifact placed for the
// rules that declare an activation. It is the artifact's own statement of how
// many triggers exist, taken before any program is constructed.
func activationPlacementCount(t *testing.T, record LinkInputs, declaring map[schema.Key]struct{}) int {
	t.Helper()
	placed := 0
	for _, mount := range record.Artifacts {
		program := mount.Snapshot.Program()
		count, published := program.RuleOccurrenceCount()
		if !published {
			t.Fatal("cold rule-occurrence family")
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.RuleOccurrenceAt(index)
			if !rowOK {
				t.Fatalf("rule occurrence %d is not published", index)
			}
			if _, declares := declaring[row.Key()]; declares {
				placed++
			}
		}
	}
	return placed
}

// Every committed mounted activation candidate carries the execution-context
// edge its body route runs on. The tuple names one sealed directory
// Transition whose endpoints are the trigger module's Context and the body
// module's Context. For a body that lives in the trigger's own module that
// edge is the directory's canonical reflexive local edge, which Seal issues
// for every Context; the producer resolves it rather than leaving the tuple
// for a later consumer to infer.
//
// The law is read off the constructed program, which is where the candidates
// exist: the issuance states them, the construction admits them, and the
// committed program enumerates them.
func TestMountedActivationCandidatesCarryTheirDirectoryEdge(t *testing.T) {
	record := mountedRecord(t, "activation-candidate-context", "local function identity(value) return value end; return identity(1)")
	bound := materializerBinding(t, record)
	committed, _ := queryCanonicalProgram(t, record, bound)
	contexts := record.Source.ContextDirectory()
	if !contexts.Available() {
		t.Fatal("sealed execution-context directory")
	}
	observed := 0
	for index := 0; index < committed.ActivationCount(); index++ {
		activation, activationOK := committed.ActivationAt(index)
		if !activationOK {
			t.Fatalf("committed activation %d is not enumerable", index)
		}
		triggerContexts := lawModuleContexts(t, contexts, activation.Mount())
		for candidateIndex := 0; candidateIndex < activation.CandidateCount(); candidateIndex++ {
			observed++
			candidate, candidateOK := activation.CandidateAt(candidateIndex)
			if !candidateOK {
				t.Fatalf("activation %d candidate %d is not enumerable", index, candidateIndex)
			}
			from, fromOK := contexts.Context(candidate.FromContextID)
			to, toOK := contexts.Context(candidate.ToContextID)
			transition, transitionOK := contexts.Transition(candidate.FromContextID, candidate.ToContextID)
			if !fromOK || !toOK || !transitionOK || transition.ID() != candidate.TransitionID {
				t.Fatalf("activation %d candidate %d carries a tuple the directory does not resolve: from=%t to=%t transition=%t transitionID=%t",
					index, candidateIndex, fromOK, toOK, transitionOK, transitionOK && transition.ID() == candidate.TransitionID)
			}
			// The committed row resolves the same edge on its own, against the
			// directory the program was committed under, so a consumer reads
			// an authenticated transition rather than a tuple to join later.
			committedTransition, committedTransitionOK := activation.CandidateTransition(candidateIndex)
			if !committedTransitionOK || committedTransition.ID() != transition.ID() {
				t.Fatalf("activation %d candidate %d does not resolve its edge against the committed directory", index, candidateIndex)
			}
			if from.ModuleKey() != activation.Mount() {
				t.Fatalf("activation %d candidate %d departs a foreign module context", index, candidateIndex)
			}
			if to.ModuleKey() != candidate.Mount {
				t.Fatalf("activation %d candidate %d arrives outside its body module", index, candidateIndex)
			}
			if candidate.Mount != activation.Mount() {
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
		t.Fatal("fixture committed no activation candidate")
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
