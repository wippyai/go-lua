package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
)

// The mount directory column is the one place a Link states which programs it
// mounted and where. These laws state what makes it that: its key set is the
// Link's module directory exactly, so a consumer that resolves a mount
// through it can never be reading a mount the Link does not have and can
// never miss one it does.

func mountDirectoryAxis(t *testing.T) snapshot.Axis[identity.ContentID, cold.Program] {
	t.Helper()
	address, projected := composite.ProjectAxis[identity.ContentID, cold.Program](programmount.OutputKey)
	if !projected {
		t.Fatalf("the declared column %q projects no address", programmount.OutputKey)
	}
	return address
}

func TestMountDirectoryKeySetIsTheLinkModuleDirectory(t *testing.T) {
	linked := planLawLink(t)
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v/%v", status, plan)
	}
	defer plan.Close()

	published := plan.state.composition
	address := mountDirectoryAxis(t)

	project := linked.Project()
	if project == nil {
		t.Fatal("linked source publishes no project")
	}
	mounts := project.Mounts()
	if mounts.Count() == 0 {
		t.Fatal("the law needs a Link with at least one mount")
	}

	declared := make(map[identity.ContentID]struct{}, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		module, moduleOK := project.ModuleKey(shard)
		mounted, programOK := mounts.Program(shard)
		if !shardOK || !moduleOK || !programOK || mounted == nil {
			t.Fatalf("project mount %d is incomplete", index)
		}
		declared[module] = struct{}{}

		row, resolved := programmount.Mounted(&published, address, module)
		if !resolved {
			t.Fatalf("module %x is mounted by the Link and absent from the directory", module[:4])
		}
		if row.ModuleKey != module {
			t.Fatalf("directory row under %x carries module key %x", module[:4], row.ModuleKey[:4])
		}
		if row.ProgramID != mounted.ContentID() {
			t.Fatalf("directory row for %x names program %x, the Link mounts %x", module[:4], row.ProgramID[:4], mounted.ContentID())
		}
		if !row.Frozen.Published() {
			t.Fatalf("directory row for %x carries an unpublished cold value", module[:4])
		}
		if row.Frozen.Generation() != published.Generation() && !row.Frozen.Store().Available() {
			t.Fatalf("directory row for %x carries no store fence", module[:4])
		}
	}

	// The other direction: a key the Link does not mount is not merely absent
	// from the rows, it is outside the published universe, and the column says
	// so as a fact rather than as ignorance.
	foreign, derived := identity.DeriveContentID("analysis/program-mount-law/foreign", nil)
	if !derived {
		t.Fatal("probe identity")
	}
	if _, resolved := programmount.Mounted(&published, address, foreign); resolved {
		t.Fatal("the directory resolved a module the Link does not mount")
	}
	if _, status := snapshot.Read(&published, address, foreign); status == snapshot.ReadHit {
		t.Fatal("the directory published a row for a module the Link does not mount")
	}

	// Every published row is a mount the Link declares: the column publishes
	// no row the directory above did not account for.
	for module := range declared {
		if _, resolved := programmount.Mounted(&published, address, module); !resolved {
			t.Fatalf("declared module %x resolves to no row", module[:4])
		}
	}
}

// One program mounted at two module keys is one frozen value in two rows. The
// directory shares it rather than copying it, which is the property that makes
// compile-once reuse survive the move onto the published substrate.
func TestMountDirectorySharesOneColdPublicationAcrossMounts(t *testing.T) {
	linked := planLawLink(t)
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("schema unavailable")
	}
	artifacts, artifactsOK := compileProgramArtifacts(linked, receipt)
	if !artifactsOK || artifacts == nil || len(artifacts.mounts) == 0 {
		t.Fatal("program artifacts")
	}
	byProgram := make(map[identity.ContentID]cold.Program, len(artifacts.mounts))
	for _, mount := range artifacts.mounts {
		if !mount.program.Available() {
			t.Fatalf("mount %x carries no cold publication", mount.moduleKey[:4])
		}
		held, seen := byProgram[mount.programID]
		if !seen {
			byProgram[mount.programID] = mount.program
			continue
		}
		if held.Frozen.Store() != mount.program.Frozen.Store() {
			t.Fatalf("program %x is mounted twice against two cold stores", mount.programID[:4])
		}
		if held.ArtifactID != mount.program.ArtifactID {
			t.Fatalf("program %x is mounted twice against two artifacts", mount.programID[:4])
		}
	}
}
