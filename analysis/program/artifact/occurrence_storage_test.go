package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestArtifactOccurrenceStorageCatalogRetainsReadAndWriteRows(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "occurrence-storage.lua", Text: []byte(`
local function run(value: number): number
  local result = value
  result = result + 1
  return result
end
return run
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
		t.Fatalf("storage compilation failed: %s", failure.Error())
	}
	read, write := 0, 0
	for index := 0; index < artifact.OccurrenceCount(); index++ {
		row, rowOK := artifact.OccurrenceAt(index)
		if !rowOK {
			t.Fatalf("OccurrenceAt(%d)", index)
		}
		switch row.Kind() {
		case programartifact.OccurrenceStorageRead:
			read++
		case programartifact.OccurrenceStorageWrite:
			write++
		}
	}
	if read == 0 || write == 0 {
		t.Fatalf("storage occurrence counts = read:%d write:%d", read, write)
	}
}
