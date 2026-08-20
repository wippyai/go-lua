package runtimekind_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
	"github.com/wippyai/go-lua/domain/value/runtimekind"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// lawPrincipals and lawAuthorities satisfy the rule's own need interfaces
// structurally, so the declaration can be read without a composition root.
type lawPrincipals struct{}

func (lawPrincipals) ValuePrincipal() *valueowner.SchemaFragment { return nil }
func (lawPrincipals) CallPrincipal() *callowner.SchemaFragment   { return nil }

type lawAuthorities struct{}

func (lawAuthorities) ValueAuthority() *valueowner.HotOwner { return nil }
func (lawAuthorities) CallAuthority() *callowner.HotOwner   { return nil }

// TestRuntimeKindRuleSubscribesToEveryOccurrenceFamilyItSeals is the placement
// half of this rule's denominator. Value seals a RuntimeKindCall operand for
// two occurrence families: the strict unary plain call itself and the guarded
// arm of that call's predicate. A family the owner seals an operand for but
// the declaration does not name is never placed, so the sealed row is dead
// weight and the transfer it feeds never runs. The required set is derived
// from the sealed schema rather than restated, so a third sealed family is
// held to the same law.
func TestRuntimeKindRuleSubscribesToEveryOccurrenceFamilyItSeals(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(contract, "runtime_kind_issuance.lua",
		[]byte("local function classify(value: string | number): string\n  if type(value) == \"string\" then\n    return value\n  end\n  return \"\"\nend\nreturn classify\n"))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	grammar, grammarOK := composite.Global()
	artifactGrammar, artifactGrammarOK := composite.ArtifactGrammar(grammar)
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory()
	if !grammarOK || !artifactGrammarOK || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounts.ProgramID(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, artifactGrammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshot, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshot, module, programID)
	if !heapMountOK || !valueMountOK {
		t.Fatal("artifact mounts")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	structural, structuralOK := composite.StructureVocabulary()
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal schemas heap=%s value=%s", heapFailure, valueFailure)
	}

	spec := runtimekind.RuleEntry[lawPrincipals, lawAuthorities]()
	declared := make(map[schema.Key]rule.Issuance, len(spec.Issues))
	for _, issuance := range spec.Issues {
		if _, duplicate := declared[issuance.Occurrence]; duplicate {
			t.Fatalf("rule declares the occurrence family %q twice", issuance.Occurrence)
		}
		if !issuance.Form.Available() || !issuance.Input.Available() || !issuance.Stage.Available() || !issuance.Requirement.Available() {
			t.Fatalf("issuance for %q is incomplete: %+v", issuance.Occurrence, issuance)
		}
		declared[issuance.Occurrence] = issuance
	}

	// Value's computation directory is keyed by occurrence identity, and more
	// than one compiled family can resolve the same sealed row. The law is
	// therefore stated over the sealed rows: each one must be reachable
	// through at least one declared subscription, otherwise the compiler
	// places nothing for it and the operand is dead weight.
	covered := make(map[identity.ContentID]bool)
	refinements := 0
	canonical := snapshot.Program()
	occurrenceCount, occurrencesPublished := canonical.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for index := 0; index < occurrenceCount; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		if !occurrenceOK {
			t.Fatalf("occurrence %d", index)
		}
		row, found := values.RuntimeKindCall(module, occurrence.ID())
		if !found || !values.OwnsRuntimeKindCall(row) {
			continue
		}
		id, idOK := row.ID()
		entry, entryOK := structural.At(structure.CategoryOccurrenceKind, uint16(occurrence.Kind()))
		if !idOK || !entryOK || !entry.Key().Available() {
			t.Fatalf("sealed runtime-kind row at occurrence %d has no identity or declared family", index)
		}
		if _, subscribed := declared[entry.Key()]; subscribed {
			covered[id] = true
		} else if _, seen := covered[id]; !seen {
			covered[id] = false
		}
		if _, _, _, refinement := row.Refinement(); refinement {
			refinements++
		}
	}
	if len(covered) < 2 || refinements == 0 {
		t.Fatalf("sealed runtime-kind operands = %d rows / %d guarded arms, want the call row and its guarded arms", len(covered), refinements)
	}
	for id, reachable := range covered {
		if !reachable {
			t.Fatalf("Value seals runtime-kind operand %v that no declared subscription places; declared=%v", id, declared)
		}
	}
}
