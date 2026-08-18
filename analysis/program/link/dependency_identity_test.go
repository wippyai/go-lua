package link_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	linkartifact "github.com/wippyai/go-lua/analysis/program/link/artifact"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

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
