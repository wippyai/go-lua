package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func TestCompactFormalApplyBoundaryPreservesMandatoryFrozenOrdinals(t *testing.T) {
	destinations := []formalApplyBoundaryDestination{{required: true}, {required: true}, {required: true}, {optional: true}}
	sources := []formalApplyBoundaryRoot{{destination: 2}, {destination: 0}, {destination: 1}}
	gotSources, gotDestinations, err := compactFormalApplyBoundary(sources, destinations)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotDestinations) != 3 {
		t.Fatalf("destinations = %d, want 3", len(gotDestinations))
	}
	if gotSources[0].destination != 2 || gotSources[1].destination != 0 || gotSources[2].destination != 1 {
		t.Fatalf("mandatory ordinals changed: %#v", gotSources)
	}
}

func TestFormalApplyBoundaryDestinationOwnsSchemaAcrossInterleavedSources(t *testing.T) {
	keys := keyspace.New()
	destinations := []formalApplyBoundaryDestination{
		{path: keys.FromPath(pathdom.Path{Root: "input-zero"})},
		{path: keys.FromPath(pathdom.Path{Root: "persistent-alias"})},
		{path: keys.FromPath(pathdom.Path{Root: "input-one"})},
	}
	// A persistent alias is interleaved between primary input destinations;
	// the exit bridge still names input zero by its sealed destination ordinal.
	sources := []formalApplyBoundaryRoot{
		{destination: 0}, {destination: 1}, {destination: 2}, {destination: 0}, {destination: 2},
	}
	assert := func(in []formalApplyBoundaryRoot) {
		t.Helper()
		rootMap, err := formalApplyBoundaryRootMap(in, destinations)
		if err != nil {
			t.Fatal(err)
		}
		for _, binding := range rootMap {
			if binding.To != destinations[binding.ToRoot].path {
				t.Fatalf("target %d schema = %#v, want %#v", binding.ToRoot, binding.To, destinations[binding.ToRoot].path)
			}
		}
	}
	assert(sources)
	assert([]formalApplyBoundaryRoot{sources[4], sources[3], sources[2], sources[1], sources[0]})
}
