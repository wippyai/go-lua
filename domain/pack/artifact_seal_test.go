package pack_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
)

// TestBranchOnlyArtifactSealsEmptyPackSchema pins the zero-factor Pack law:
// control-flow geometry with no Values or Call rows is a valid Program, not a
// malformed Pack publication. Pack must publish an empty sealed axis instead
// of inventing a value or rejecting the mount.
func TestBranchOnlyArtifactSealsEmptyPackSchema(t *testing.T) {
	target, _ := selectorLawContract(t)
	published, err := lower.Lower(lower.Source{Name: "pack_branch_only.lua", Text: []byte("if true then end\n")})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "pack_branch_only", Program: published}}})
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
		t.Fatal("branch-only mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, receipt.ExecutionSchemaID(), issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile branch-only artifact: %s", failure.Error())
	}
	cold := artifact.Program()
	catalog, catalogOK := programcatalog.CatalogID(cold.SchemaID)
	valueCount, valuesOK := programschema.ValuesFamily().Count(&cold.Frozen, catalog)
	callCount, callsOK := cold.CallCount()
	if !catalogOK || !valuesOK || !callsOK || valueCount != 0 || callCount != 0 {
		t.Fatalf("branch-only Pack denominator: values=%d/%t calls=%d/%t catalog=%t", valueCount, valuesOK, callCount, callsOK, catalogOK)
	}
	types, err := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{cold})
	if err != nil || types == nil {
		t.Fatalf("seal branch-only types: %v", err)
	}
	mounted := snapshottest.MustMount(t, artifact, module)
	statics, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: target}, types, []staticdomain.MountedProgram{{Program: mounted.Program, ModuleID: module, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal branch-only static: %v", err)
	}
	snapshot := snapshottest.MustLower(t, artifact)
	packMount, packMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !packMountOK {
		t.Fatal("branch-only Pack mount")
	}
	sealed, ok := packdomain.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{packMount})
	if !ok || sealed == nil || sealed.RootCount() != 0 {
		t.Fatalf("empty Pack seal: available=%t roots=%d", ok && sealed != nil, func() int {
			if sealed == nil {
				return -1
			}
			return sealed.RootCount()
		}())
	}
}
