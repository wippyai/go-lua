package pathevidence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestPathValueDependencyMapsEveryValuesRootVocabulary(t *testing.T) {
	keys := keyspace.New()
	resolver, ok := keys.FromResolverKey(11, 2, []segment.Segment{{Kind: segment.SegmentField, Name: "field"}})
	if !ok {
		t.Fatal("resolver key")
	}
	stable, ok := keys.FromStableSymbol(12, nil)
	if !ok {
		t.Fatal("stable key")
	}
	ret := mustStateKey(t, keys, pathdom.PathKey("ret[3].field"))

	for _, test := range []struct {
		name string
		path keyspace.Key
		want statekey.Value
	}{
		{name: "resolver suffix", path: resolver, want: statekey.SymbolValue(11)},
		{name: "stable", path: stable, want: statekey.SymbolValue(12)},
		{name: "return suffix", path: ret, want: statekey.ReturnSlot(3)},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependency, found := PathValueDependency(keys, test.path)
			got, concrete := dependency.Concrete()
			if !found || !concrete || got != test.want {
				t.Fatalf("dependency = %v/%v/%v, want %v", got, concrete, found, test.want)
			}
			if _, formalRoot := dependency.Formal(); formalRoot {
				t.Fatal("concrete path reported a formal root")
			}
		})
	}
}

func TestCoordinateValueDependenciesPreserveFormalVocabularyAndSuffixRoot(t *testing.T) {
	keys := keyspace.New()
	var owner lexicalidentity.StableLexicalBodyID
	owner[0], owner[len(owner)-1] = 3, 9
	input := formal.NewRoot(owner, 19, formal.Input)
	output := formal.NewRoot(owner, 19, formal.Output)

	for _, root := range []formal.Root{input, output} {
		base, ok := keys.InternFormalRoot(root)
		if !ok {
			t.Fatalf("formal root %v", root)
		}
		child, ok := keys.AppendSegment(base, segment.Segment{Kind: segment.SegmentField, Name: "nested"})
		if !ok {
			t.Fatalf("formal suffix %v", root)
		}
		var got []statekey.ValueDependency
		VisitCoordinateValueDependencies(RefinementCoordinate(child), keys, func(dependency statekey.ValueDependency) {
			got = append(got, dependency)
		})
		if len(got) != 1 {
			t.Fatalf("dependencies(%v) = %d, want 1", root, len(got))
		}
		formalRoot, isFormal := got[0].Formal()
		if !isFormal || formalRoot != root {
			t.Fatalf("formal dependency = %v/%v, want %v", formalRoot, isFormal, root)
		}
		if _, concrete := got[0].Concrete(); concrete {
			t.Fatal("formal path reported a concrete cell")
		}
	}
	if statekey.FormalDependency(input) == statekey.FormalDependency(output) {
		t.Fatal("IN and OUT dependencies collapsed")
	}
}
