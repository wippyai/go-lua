package resultalias_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	resultaliasprogram "github.com/wippyai/go-lua/domain/value/resultalias/program"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// The subject carries one call of each result geometry the rule must decide
// about: an aliasing member, a fresh member, a discarded statement call, a
// method call, and a nullary call.
const aliasSubject = "local m = setmetatable({}, {})\n" +
	"local co = coroutine.create(function() end)\n" +
	"print(\"x\")\n" +
	"local upper = (\"a\"):upper()\n" +
	"local now = os.time()\n" +
	"return m, co, upper, now\n"

const ruleKey = schema.Key("value-callresult-resultalias")

// TestResultAliasIssuesExactlyTheResultSlotOperandsItSeals is the declared
// admissibility law of this rule. Value seals one ResultAlias operand per
// mounted call that owns a fixed valued result-zero slot, and construction
// resolves that operand for every placement the artifact carries. The two sets
// are therefore one set: a placement without an operand is a construction the
// owner cannot answer, and a sealed operand without a placement is a row the
// transfer never runs.
func TestResultAliasIssuesExactlyTheResultSlotOperandsItSeals(t *testing.T) {
	targetContract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(targetContract, "result_alias_issuance.lua", []byte(aliasSubject))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !mountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{mount}), []programmount.MountedArtifact{mount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal schemas heap=%s value=%s", heapFailure, valueFailure)
	}

	spec := resultaliasprogram.RuleEntry()
	if len(spec.Issues) != 1 || !spec.Issues[0].Available() {
		t.Fatalf("rule declares %d issuances, want the one result-slot subscription", len(spec.Issues))
	}

	placed := make(map[identity.ContentID]int)
	key, walked := composite.WalkSealedPlacements([]programmount.MountedArtifact{mount}, func(placedKey schema.Key, placedMount, point, occurrence identity.ContentID) bool {
		if placedKey != ruleKey {
			return true
		}
		if placedMount != module {
			t.Errorf("placement of %q names mount %v, want the sealed module", ruleKey, placedMount)
			return false
		}
		placed[occurrence]++
		return true
	})
	if !walked {
		t.Fatalf("sealed placement walk refused at %q", key)
	}
	if len(placed) == 0 {
		t.Fatalf("no %q placement was issued for a subject that calls an aliasing member", ruleKey)
	}

	// One occurrence identity can be published by more than one compiled
	// family - a call and its activation share the identity Value keys its
	// result-slot directory by - so the law is stated over identities rather
	// than over rows.
	canonical := snapshot.Program()
	occurrenceCount, published := canonical.OccurrenceCount()
	if !published {
		t.Fatal("occurrence family is unpublished")
	}
	operands := make(map[identity.ContentID]bool)
	calls := 0
	for index := 0; index < occurrenceCount; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		if !occurrenceOK {
			t.Fatalf("occurrence %d", index)
		}
		if occurrence.Kind() == programschema.OccurrenceCall {
			calls++
		}
		row, rowOK := values.MountedCallResultSlotFor(module, occurrence.ID(), 0)
		if rowOK && values.OwnsMountedCallResultSlot(row) {
			operands[occurrence.ID()] = true
			continue
		}
		if _, seen := operands[occurrence.ID()]; !seen {
			operands[occurrence.ID()] = false
		}
	}
	sealed := 0
	for occurrence, operand := range operands {
		if operand {
			sealed++
		}
		if operand != (placed[occurrence] == 1) {
			t.Fatalf("occurrence %v seals a result-slot operand=%t and carries %d %q placements; the requirement and the sealed operand set must be one set",
				occurrence, operand, placed[occurrence], ruleKey)
		}
	}
	if calls == 0 || sealed == 0 || sealed >= calls {
		t.Fatalf("subject calls = %d, of which %d seal a result-slot operand; the law needs both kinds present", calls, sealed)
	}
	if len(placed) != sealed {
		t.Fatalf("%q placements = %d, sealed result-slot operands = %d", ruleKey, len(placed), sealed)
	}
}
