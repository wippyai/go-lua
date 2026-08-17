package activation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	calldomain "github.com/wippyai/go-lua/domain/call"
	activation "github.com/wippyai/go-lua/domain/call/activation"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestTargetBatchCatalogRejectsBodyFromAnotherArtifact(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	program, err := lower.Lower(lower.Source{
		Name: "target_batch_law.lua",
		Text: []byte(`
local function callee() return 1 end
local function invoke(value) return value() end
invoke(callee)
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "target_batch_law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Global()
	if !ok {
		t.Fatal("program schema receipt")
	}
	mounts := linked.Project().Mounts()
	shard, ok := mounts.At(0)
	if !ok {
		t.Fatal("mounted shard")
	}
	mounted, ok := mounts.Program(shard)
	if !ok || mounted == nil {
		t.Fatal("mounted program")
	}
	moduleKey, ok := linked.Project().ModuleKey(shard)
	if !ok {
		t.Fatal("module key")
	}
	artifact, failure := composite.CompileArtifactDetailed(mounted, receipt)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	calls, ok := calldomain.NewWithMountedArtifacts(linked, []calldomain.MountedArtifact{{ModuleKey: moduleKey, Artifact: artifact}})
	if !ok || calls == nil || calls.Bodies().Count() == 0 {
		t.Fatal("call body fixture")
	}
	body, ok := calls.Bodies().At(0)
	if !ok {
		t.Fatal("call body")
	}
	path, ok := body.BodyPath()
	if !ok {
		t.Fatal("body path")
	}
	role, ok := body.RoleID()
	if !ok {
		t.Fatal("body role")
	}
	if catalog, admitted := activation.NewTargetBatchCatalog([]activation.MountedTargetBatch{{
		Artifact:  artifact,
		ModuleKey: moduleKey,
		Rows:      []activation.TargetBatchRow{{Body: body, BodyPath: path, Role: role}},
	}}); !admitted || catalog == nil {
		t.Fatal("matching artifact/body receipt was rejected")
	}

	foreignProgram, err := lower.Lower(lower.Source{
		Name: "target_batch_law_foreign.lua",
		Text: []byte("return 2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignArtifact, foreignFailure := composite.CompileArtifactDetailed(foreignProgram, receipt)
	if foreignFailure.Available() || foreignArtifact == nil || !foreignArtifact.Available() {
		t.Fatalf("compile foreign artifact: %s", foreignFailure.Error())
	}
	bodyArtifactID, bodyArtifactOK := body.ArtifactID()
	if !bodyArtifactOK || bodyArtifactID == foreignArtifact.ID() {
		t.Fatal("foreign artifact did not provide a mismatched artifact identity")
	}
	if catalog, admitted := activation.NewTargetBatchCatalog([]activation.MountedTargetBatch{{
		Artifact:  foreignArtifact,
		ModuleKey: moduleKey,
		Rows:      []activation.TargetBatchRow{{Body: body, BodyPath: path, Role: role}},
	}}); admitted || catalog != nil {
		t.Fatal("body receipt crossed the mounted artifact identity fence")
	}
}
