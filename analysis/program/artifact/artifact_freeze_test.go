package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestArtifactFreezePublishesOneAvailableIdentity(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-freeze.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() || !artifact.ID().Available() {
		t.Fatalf("freeze result = artifact:%v failure:%s", artifact != nil, failure.Error())
	}
}
