package value_test

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestRuntimeKindNamesUsesTheSealedVocabulary verifies the semantic result
// of the Value projection through a real mounted schema. The expected names
// are read from the same sealed structural table that fed Value sealing; no
// runtime-kind strings are restated by this test or by the Value domain.
func TestRuntimeKindNamesUsesTheSealedVocabulary(t *testing.T) {
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	structural, structuralOK := composite.StructureVocabulary(compilation)
	contract, contractErr := testfixture.StandardLibraryTarget()
	if contractErr != nil {
		t.Fatal(contractErr)
	}
	linked, linkErr := testfixture.SealSource(contract, "runtime_kind_names.lua", []byte("return {}\n"))
	if linkErr != nil {
		t.Fatal(linkErr)
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := mounts.ProgramID(shard)
	if !compilationOK || !grammar.Available() || !issuanceOK || !structuralOK || mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("runtime-kind names fixture mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile runtime-kind names artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshot, module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshot, module, programID)
	if !heapMountOK || !valueMountOK {
		t.Fatal("runtime-kind names artifact mounts")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("runtime-kind names heap seal: %s", heapFailure)
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount}, structural)
	if valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("runtime-kind names value seal: %s", valueFailure)
	}

	all, allOK := values.RuntimeKindNames(values.Top())
	if !allOK {
		t.Fatal("runtime-kind projection of Top")
	}
	atoms, atomsOK := values.Atoms(all)
	if !atomsOK || len(atoms) != structural.Count(structure.CategoryRuntimeKind) {
		t.Fatalf("runtime-kind names atoms = %d, want %d", len(atoms), structural.Count(structure.CategoryRuntimeKind))
	}

	seen := make(map[string]bool, len(atoms))
	for _, atom := range atoms {
		scalar, scalarOK := values.ExactScalar(mustSingleton(t, values, atom))
		if !scalarOK {
			t.Fatal("runtime-kind result atom is not an exact scalar")
		}
		literal, literalOK := scalar.Literal()
		if !literalOK || literal.Kind != keyspace.LiteralString {
			t.Fatalf("runtime-kind result literal = %#v, want string", literal)
		}
		if _, keyed := atom.ExactKey(); keyed {
			t.Fatal("computed runtime-kind result acquired authored key identity")
		}
		seen[literal.String] = true
	}
	for kind := runtimekind.Invalid + 1; kind < runtimekind.Count; kind++ {
		entry, entryOK := structural.At(structure.CategoryRuntimeKind, uint16(kind))
		if !entryOK || !seen[entry.Spelling()] {
			t.Fatalf("sealed runtime-kind spelling %q missing from Value result", entry.Spelling())
		}
	}

	bottom, bottomOK := values.RuntimeKindNames(values.Bottom())
	if !bottomOK || !bottom.IsBottom() {
		t.Fatal("runtime-kind projection of Bottom did not remain Bottom")
	}
	var allocation *valuedomain.AllocationResult
	for index := 0; index < heaps.KeyCount(); index++ {
		key, keyOK := heaps.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		allocation, _ = values.AllocationResultFor(key)
		if allocation != nil {
			break
		}
	}
	if allocation == nil {
		t.Fatal("allocation result")
	}
	knownTable, knownOK := allocation.Fresh()
	if !knownOK {
		t.Fatal("known table Value")
	}
	tableNames, tableNamesOK := values.RuntimeKindNames(knownTable)
	if !tableNamesOK {
		t.Fatal("runtime-kind projection of known table")
	}
	if values.RuntimeKinds(tableNames) != runtimekind.Bit(runtimekind.String) {
		t.Fatalf("runtime-kind result Value has kinds %#x, want String", values.RuntimeKinds(tableNames))
	}
}

func mustSingleton(t *testing.T, schema *valuedomain.Schema, atom valuedomain.Atom) valuedomain.Value {
	t.Helper()
	value, valueOK := schema.Singleton(atom)
	if !valueOK {
		t.Fatal("runtime-kind atom does not belong to fixture schema")
	}
	return value
}
