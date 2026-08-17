package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestProgramArtifactBodyRootsAreDenseExecutableSemanticRows(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-root-plane.lua", Text: []byte(`
local function roots(flag: boolean)
  local first = 1
  if flag then first = 2 end
  return first
end
return roots(true)
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile root fixture: %s", failure.Error())
	}
	seen := make(map[identity.ContentID]struct{})
	rootCount := 0
	for bodyIndex := 0; bodyIndex < artifact.BodyCount(); bodyIndex++ {
		body, bodyOK := artifact.BodyAt(bodyIndex)
		if !bodyOK {
			t.Fatalf("BodyAt(%d)", bodyIndex)
		}
		for rootIndex := 0; rootIndex < body.RootCount(); rootIndex++ {
			root, rootOK := body.RootAt(rootIndex)
			if !rootOK || !root.Available() || !root.ID().Available() || root.Family() == keyspace.FamilyInvalid {
				t.Fatalf("body %d root %d is not an exact scalar row", bodyIndex, rootIndex)
			}
			if _, duplicate := seen[root.ID()]; duplicate {
				t.Fatalf("duplicate root identity %v", root.ID())
			}
			seen[root.ID()] = struct{}{}
			rootCount++
		}
		if _, ok := body.RootAt(body.RootCount()); ok {
			t.Fatalf("body %d accepted out-of-range root", bodyIndex)
		}
	}
	if rootCount < 2 {
		t.Fatalf("fixture emitted %d executable roots, want multiple", rootCount)
	}
}
