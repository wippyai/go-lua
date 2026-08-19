package link_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkartifact "github.com/wippyai/go-lua/analysis/program/link/artifact"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func mountedProgram(t *testing.T, linked *link.Link, name string) *program.Program {
	t.Helper()
	if linked == nil || linked.Project() == nil {
		t.Fatal("link project unavailable")
	}
	mounts := linked.Project().Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		got, nameOK := mounts.Name(shard)
		published, publishedOK := mounts.Program(shard)
		if shardOK && nameOK && publishedOK && got == name {
			return published
		}
	}
	t.Fatalf("mount %q unavailable", name)
	return nil
}

func TestMountedProgramContentSurvivesActorAndCallerDeltas(t *testing.T) {
	first := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"provider"}, Member: []string{"first"}}
	second := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"provider"}, Member: []string{"second"}}
	contract := contract(t, first, second)
	main, dependency := source(t, `require("dependency")`), source(t, `return 1`)
	actors, aliases, roots, entries := moduleCacheDeployment(t, main, dependency)
	base, err := link.Seal(&link.Spec{
		Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}},
		Module:           linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "first", Binding: first}},
	})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := link.Seal(&link.Spec{
		Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}},
		Module:           linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "second", Binding: second}},
	})
	if err != nil {
		t.Fatal(err)
	}
	actorName := "other-actor"
	otherActors := []linkmodule.ActorSpec{{Name: actorName}}
	otherAliases := append([]linkmodule.ModuleCacheAliasClassSpec(nil), aliases...)
	otherRoots := append([]linkmodule.AnalysisRootSpec(nil), roots...)
	for index := range otherAliases {
		otherAliases[index].Actor = actorName
	}
	for index := range otherRoots {
		otherRoots[index].Actor = actorName
	}
	actor, err := link.Seal(&link.Spec{
		Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}, {Name: "dependency", Program: dependency}},
		Module:           linkmodule.Spec{Actors: otherActors, ModuleCacheAliases: otherAliases, AnalysisRoots: otherRoots, ModuleCacheEntries: entries},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "first", Binding: first}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if base.ContentID() == caller.ContentID() || base.ContentID() == actor.ContentID() {
		t.Fatal("caller or actor delta did not change Link identity")
	}
	if mountedProgram(t, base, "main") != main || mountedProgram(t, caller, "main") != main || mountedProgram(t, actor, "main") != main ||
		mountedProgram(t, base, "dependency") != dependency || mountedProgram(t, caller, "dependency") != dependency || mountedProgram(t, actor, "dependency") != dependency {
		t.Fatal("caller or actor delta replaced a mounted Program")
	}
	if main.ContentID() != mountedProgram(t, caller, "main").ContentID() || main.ContentID() != mountedProgram(t, actor, "main").ContentID() ||
		dependency.ContentID() != mountedProgram(t, caller, "dependency").ContentID() || dependency.ContentID() != mountedProgram(t, actor, "dependency").ContentID() {
		t.Fatal("caller or actor delta changed mounted Program content")
	}
}

func TestDependencyDigestIsCanonicalAndReplayable(t *testing.T) {
	first := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"provider"}, Member: []string{"first"}}
	second := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"provider"}, Member: []string{"second"}}
	contract := contract(t, first, second)
	main, schema := source(t, ``), source(t, `local schema = 1`)
	left, err := link.Seal(&link.Spec{
		Target:           contract,
		Modules:          []linkproject.Module{{Name: "main", Program: main}, {Name: "schema", Program: schema}},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "first", Binding: first}, {Identity: "second", Binding: second}},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := link.Seal(&link.Spec{
		Target:           contract,
		Modules:          []linkproject.Module{{Name: "schema", Program: schema}, {Name: "main", Program: main}},
		EndpointRequests: []linkboundary.EndpointRequest{{Identity: "second", Binding: second}, {Identity: "first", Binding: first}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if left.ContentID() != right.ContentID() {
		t.Fatal("authority permutation changed Link identity")
	}
	leftBytes, err := linkartifact.Encode(left)
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, err := linkartifact.Encode(right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatal("authority permutation changed artifact bytes")
	}
	replayed, err := linkartifact.Decode(leftBytes, contract, artifactProgramPool(main, schema))
	if err != nil || replayed.ContentID() != left.ContentID() {
		t.Fatalf("artifact replay = %v/%v", replayed, err)
	}
}

func TestDependencyDigestTracksActualAuthoritiesAndFailsClosed(t *testing.T) {
	first := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"provider"}, Member: []string{"first"}}
	second := vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"provider"}, Member: []string{"second"}}
	contract := contract(t, first, second)
	main := source(t, ``)
	base, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "first", Binding: first}}})
	if err != nil {
		t.Fatal(err)
	}
	changedProvider, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "second", Binding: second}}})
	if err != nil {
		t.Fatal(err)
	}
	if base.ContentID() == changedProvider.ContentID() {
		t.Fatal("resolved EndpointTarget operation did not change Link identity")
	}
	changedSchema, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: source(t, `local changed = 1`)}}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "first", Binding: first}}})
	if err != nil {
		t.Fatal(err)
	}
	if base.ContentID() == changedSchema.ContentID() {
		t.Fatal("changed static namespace schema did not change Link identity")
	}
	cacheMain, cacheDependency := source(t, `require("dependency")`), source(t, `return 1`)
	actors, aliases, roots, entries := moduleCacheDeployment(t, cacheMain, cacheDependency)
	cacheBase, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: cacheMain}, {Name: "dependency", Program: cacheDependency}}, Module: linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "first", Binding: first}}})
	if err != nil {
		t.Fatal(err)
	}
	roots[0].Instance = "cache-dependency"
	cacheChanged, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: cacheMain}, {Name: "dependency", Program: cacheDependency}}, Module: linkmodule.Spec{Actors: actors, ModuleCacheAliases: aliases, AnalysisRoots: roots, ModuleCacheEntries: entries}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "first", Binding: first}}})
	if err != nil {
		t.Fatal(err)
	}
	if cacheBase.ContentID() == cacheChanged.ContentID() {
		t.Fatal("changed deployment binding world did not change Link identity")
	}
	// A malformed endpoint cannot produce a provider dependency; sealing stops
	// before identity publication instead of accepting a caller claim.
	if linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: main}}, EndpointRequests: []linkboundary.EndpointRequest{{Identity: "missing", Binding: vocabulary.BindingSpec{Namespace: vocabulary.BindingProvider, Owner: []string{"missing"}, Member: []string{"operation"}}}}}); err == nil || linked != nil {
		t.Fatal("unknown endpoint binding admitted")
	}
}
