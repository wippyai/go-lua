package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestArtifactEnvelopeCanonicalDependenciesRejectDuplicates(t *testing.T) {
	id := identity.ContentID{1}
	ordered, err := canonicalDependencies(Metadata{Dependencies: []Dependency{{Name: "z", ID: id}, {Name: "a", ID: id}}})
	if err != nil || len(ordered) != 2 || ordered[0].Name != "a" || ordered[1].Name != "z" {
		t.Fatalf("canonical dependencies = %#v, %v", ordered, err)
	}
	if _, err := canonicalDependencies(Metadata{Dependencies: []Dependency{{Name: "same", ID: id}, {Name: "same", ID: id}}}); err == nil {
		t.Fatal("duplicate dependency names were admitted")
	}
}
