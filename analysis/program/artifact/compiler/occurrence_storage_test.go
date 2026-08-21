package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("storage compilation failed: %s", failure.Error())
	}
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	read, write := 0, 0
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK {
			t.Fatalf("OccurrenceAt(%d)", index)
		}
		switch row.Kind() {
		case programschema.OccurrenceStorageRead:
			read++
		case programschema.OccurrenceStorageWrite:
			write++
		}
	}
	if read == 0 || write == 0 {
		t.Fatalf("storage occurrence counts = read:%d write:%d", read, write)
	}
}
