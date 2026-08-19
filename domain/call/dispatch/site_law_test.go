package dispatch_test

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	calldomain "github.com/wippyai/go-lua/domain/call"
	dispatchdomain "github.com/wippyai/go-lua/domain/call/dispatch"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	"github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type siteLawFixture struct {
	algebra *calldomain.Algebra
	heaps   heapdomain.Schema
	values  *valuedomain.Schema
	packs   *packdomain.Schema
	apps    []identity.ContentID
}

func newSiteLawFixture(t testing.TB) siteLawFixture {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "dispatch_site_law.lua", Text: []byte(`return require("module")`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{
		{Name: "dispatch_site_alpha", Program: program},
		{Name: "dispatch_site_beta", Program: program},
	}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("program schema receipt")
	}
	mounts := linked.Project().Mounts()
	artifacts := make([]*programartifact.Artifact, mounts.Count())
	heapMounts := make([]heapdomain.ArtifactMount, mounts.Count())
	valueMounts := make([]valuedomain.ArtifactMount, mounts.Count())
	packMounts := make([]packdomain.ArtifactMount, mounts.Count())
	staticMounts := make([]staticdomain.MountedProgram, mounts.Count())
	callMounts := make([]calldomain.MountedArtifact, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		published, programOK := mounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := mounts.ProgramID(shard)
		if !shardOK || !programOK || published == nil || !moduleOK || !programIDOK {
			t.Fatalf("site fixture mount %d", index)
		}
		artifact, failure := composite.CompileArtifactDetailed(published, receipt)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile site fixture artifact %d: %s", index, failure.Error())
		}
		var heapOK, valueOK, packOK bool
		artifacts[index] = artifact
		heapMounts[index], heapOK = heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		valueMounts[index], valueOK = valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		packMounts[index], packOK = packdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		staticMounts[index] = staticdomain.MountedProgram{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}
		callMounts[index] = calldomain.MountedArtifact{Program: snapshottest.MustMount(t, artifact, module), Snapshot: snapshottest.MustLower(t, artifact)}
		if !heapOK || !valueOK || !packOK {
			t.Fatalf("site fixture artifact mounts %d", index)
		}
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, heapMounts)
	structural, structuralOK := composite.StructureVocabulary()
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, valueMounts, structural)
	types, typesErr := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || typesErr != nil || types == nil {
		t.Fatalf("site fixture schemas heap=%s value=%s types=%v", heapFailure, valueFailure, typesErr)
	}
	statics, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, staticMounts)
	if staticErr != nil || statics == nil {
		t.Fatalf("site fixture Static: %v", staticErr)
	}
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, packMounts)
	algebra, algebraOK := calldomain.NewWithMountedArtifacts(linked, callMounts)
	if !packsOK || packs == nil || !algebraOK || algebra == nil {
		t.Fatal("site fixture Pack/Call")
	}
	apps := make([]identity.ContentID, algebra.MountedCallCount())
	for index := range apps {
		mounted, mountedOK := algebra.MountedCallAtHandle(index)
		application, _, _, _, _, identityOK := algebra.MountedCallIdentity(mounted)
		if !mountedOK || !identityOK {
			t.Fatalf("site fixture mounted call %d", index)
		}
		apps[index] = application
	}
	return siteLawFixture{algebra: algebra, heaps: heaps, values: values, packs: packs, apps: apps}
}

func TestSiteRejectsForgedCrossMountRequireSeed(t *testing.T) {
	fixture := newSiteLawFixture(t)
	if len(fixture.apps) < 2 {
		t.Fatalf("mounted applications=%d, want at least two", len(fixture.apps))
	}
	left, leftOK := dispatchdomain.NewSiteForTest(fixture.algebra, fixture.values, fixture.heaps, fixture.packs, fixture.apps[0])
	right, rightOK := dispatchdomain.NewSiteForTest(fixture.algebra, fixture.values, fixture.heaps, fixture.packs, fixture.apps[1])
	leftSeed := dispatchdomain.SiteRequireSeedForTest(left)
	rightSeed := dispatchdomain.SiteRequireSeedForTest(right)
	if !leftOK || !rightOK || leftSeed == rightSeed {
		t.Fatal("site fixture did not issue distinct mount-scoped require seeds")
	}
	forged := dispatchdomain.SiteWithRequireSeedForTest(left, rightSeed)
	if dispatchdomain.SiteValidForTest(forged) {
		t.Fatal("forged cross-mount require seed crossed the authoritative site fence")
	}
}
