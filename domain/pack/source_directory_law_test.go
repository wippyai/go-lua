package pack_test

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
)

func sealedPackSchema(t testing.TB, name, source string) *packdomain.Schema {
	t.Helper()
	target, _ := selectorLawContract(t)
	published, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: name, Program: published}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Build()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !receiptOK || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("pack source mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, receipt.ExecutionSchemaID(), issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile pack source artifact: %s", failure.Error())
	}
	types, err := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()})
	if err != nil || types == nil {
		t.Fatalf("seal pack source types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: target}, types, []staticdomain.MountedProgram{{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal pack source static: %v", err)
	}
	snapshot := snapshottest.MustLower(t, artifact)
	packMount, packMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !packMountOK {
		t.Fatal("pack source mount snapshot")
	}
	sealed, ok := packdomain.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{packMount})
	if !ok || sealed == nil {
		t.Fatal("seal pack source schema")
	}
	return sealed
}

func TestSourceDirectoryIsDenseAndInvertible(t *testing.T) {
	schema := sealedPackSchema(t, "source-directory", "return 1\n")
	if schema.SourceCount() == 0 {
		t.Fatal("literal fixture issued no Pack sources")
	}
	seen := make(map[uint32]struct{}, schema.SourceCount())
	for index := 0; index < schema.SourceCount(); index++ {
		source, sourceOK := schema.SourceAt(index)
		if !sourceOK {
			t.Fatalf("source at %d", index)
		}
		ordinal, ordinalOK := schema.SourceOrdinal(source)
		if !ordinalOK || int(ordinal) != index {
			t.Fatalf("source ordinal = %d/%t, want %d", ordinal, ordinalOK, index)
		}
		if _, duplicate := seen[ordinal]; duplicate {
			t.Fatalf("duplicate source ordinal %d", ordinal)
		}
		seen[ordinal] = struct{}{}
		root, fact, resultOK := source.Result()
		rootIndex, indexOK := schema.RootIndex(root)
		factAgain, factOutcome := packdomain.SourceFact(source)
		if !resultOK || !indexOK || factOutcome != structure.Concrete || !schema.Admit(root, fact) || !schema.Admit(root, factAgain) || schema.Fingerprint(fact) != schema.Fingerprint(factAgain) {
			t.Fatalf("source result index=%d admit=%t", rootIndex, schema.Admit(root, fact))
		}
		module, occurrence, identityOK := source.Occurrence()
		resolved, resolvedOK := schema.SourceForMountedOccurrence(module, occurrence)
		resolvedOrdinal, resolvedOrdinalOK := schema.SourceOrdinal(resolved)
		if !identityOK || !resolvedOK || !resolvedOrdinalOK || resolvedOrdinal != ordinal {
			t.Fatalf("mounted source lookup ordinal=%d/%t want=%d", resolvedOrdinal, resolvedOrdinalOK, ordinal)
		}
	}
	if _, ok := schema.SourceAt(schema.SourceCount()); ok {
		t.Fatal("out-of-range source admitted")
	}
	if _, ok := schema.SourceOrdinal(packdomain.Source{}); ok {
		t.Fatal("zero source admitted an ordinal")
	}
	if _, ok := schema.SourceForMountedOccurrence(identity.ContentID{}, identity.ContentID{}); ok {
		t.Fatal("unavailable occurrence admitted a source")
	}
}
