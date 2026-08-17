package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestPointConstructionPublishesLocalWTOSchedule(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "point-construction.lua", Text: []byte(`
local function loop(limit: number)
  local value = 0
  while value < limit do value = value + 1 end
  return value
end
return loop
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("point construction failed: %s", failure.Error())
	}
	if artifact.WTOEventCount() == 0 || artifact.RegionCount() == 0 {
		t.Fatalf("WTO schedule = events:%d regions:%d", artifact.WTOEventCount(), artifact.RegionCount())
	}
}
