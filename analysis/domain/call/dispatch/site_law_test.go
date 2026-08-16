package dispatch_test

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	dispatchdomain "github.com/wippyai/go-lua/analysis/domain/call/dispatch"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/domain/type/authority"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/schemaadapter"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
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
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"require"}}},
		Input:    target.ValuesSpec{Tail: target.ValuesClosed},
		Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:  target.RowSpec{Tail: target.RowClosed},
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
	receipt, receiptOK := grammar.Global()
	if !receiptOK {
		t.Fatal("program schema receipt")
	}
	mounts := linked.Project().Mounts()
	artifacts := make([]*programartifact.Artifact, mounts.Count())
	heapMounts := make([]heapdomain.ArtifactMount, mounts.Count())
	valueMounts := make([]valuedomain.ArtifactMount, mounts.Count())
	packMounts := make([]packdomain.ArtifactMount, mounts.Count())
	staticMounts := make([]staticdomain.MountedArtifact, mounts.Count())
	callMounts := make([]calldomain.MountedArtifact, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		published, programOK := mounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := mounts.ProgramID(shard)
		if !shardOK || !programOK || published == nil || !moduleOK || !programIDOK {
			t.Fatalf("site fixture mount %d", index)
		}
		artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile site fixture artifact %d: %s", index, failure.Error())
		}
		var heapOK, valueOK, packOK bool
		artifacts[index] = artifact
		heapMounts[index], heapOK = heapdomain.NewArtifactMount(artifact, module, programID)
		valueMounts[index], valueOK = valuedomain.NewArtifactMount(artifact, module, programID)
		packMounts[index], packOK = packdomain.NewArtifactMount(artifact, module, programID)
		staticMounts[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
		callMounts[index] = calldomain.MountedArtifact{ModuleKey: module, Artifact: artifact}
		if !heapOK || !valueOK || !packOK {
			t.Fatalf("site fixture artifact mounts %d", index)
		}
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, heapMounts)
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, valueMounts)
	types, typesErr := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || typesErr != nil || types == nil {
		t.Fatalf("site fixture schemas heap=%s value=%s types=%v", heapFailure, valueFailure, typesErr)
	}
	statics, _, staticErr := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, staticMounts)
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
