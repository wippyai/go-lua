package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("point construction failed: %s", failure.Error())
	}
	program := artifact.Program()
	catalog, catalogOK := programschema.CatalogID(program.SchemaID)
	eventCount, eventsPublished := programschema.WTOEventFamily().Count(&program.Frozen, catalog)
	regionCount, regionsPublished := programschema.RegionFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !eventsPublished || !regionsPublished || eventCount == 0 || regionCount == 0 {
		t.Fatalf("WTO schedule = events:%d regions:%d", eventCount, regionCount)
	}
}
