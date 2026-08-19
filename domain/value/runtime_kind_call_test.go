package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestRuntimeKindCallUsesMountedCallGeometry seals a real plain unary call
// and verifies that Value issues the operand from the existing ingress Call
// and CallArgument rows. The test intentionally checks the detached API, not
// a second call catalog or a reconstructed Program occurrence.
func TestRuntimeKindCallUsesMountedCallGeometry(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "runtime_kind_call.lua", Text: []byte("local function send(x) return x end\nsend({})\n")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: allocationMembershipContract(t), Modules: []linkproject.Module{{Name: "runtime_kind_call", Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := composite.Global()
	if !grammarOK {
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
	artifact, failure := composite.CompileArtifactDetailed(program, grammar)
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

	for index := 0; index < snapshot.OccurrenceCount(); index++ {
		occurrence, occurrenceOK := snapshot.OccurrenceAt(index)
		if !occurrenceOK || occurrence.Kind() != uint8(programartifact.OccurrenceCall) {
			continue
		}
		operand, operandOK := values.RuntimeKindCall(module, occurrence.ID())
		if !operandOK || !values.OwnsRuntimeKindCall(operand) {
			continue
		}
		result, input, endpointsOK := operand.Endpoints()
		mountedModule, mountedOccurrence, joinOK := operand.CallOccurrence()
		if !endpointsOK || !joinOK || mountedModule != module || mountedOccurrence != occurrence.ID() {
			t.Fatalf("runtime-kind call endpoint join = module %x occurrence %x", mountedModule, mountedOccurrence)
		}
		if _, resultOK := values.CoordinateIndex(result); !resultOK {
			t.Fatal("runtime-kind result coordinate")
		}
		if _, inputOK := values.CoordinateIndex(input); !inputOK {
			t.Fatal("runtime-kind input coordinate")
		}
		return
	}
	t.Fatal("plain unary call did not issue a RuntimeKindCall operand")
}
