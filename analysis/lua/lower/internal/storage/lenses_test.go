package storage

import (
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestStorageLensConstructionRequiresCollectorAuthority(t *testing.T) {
	var writer Writer
	if _, err := writer.DotLens(programsource.Span{File: "storage.lua"}, 1, 2, programsource.Span{File: "storage.lua"}, "field"); err == nil {
		t.Fatal("DotLens accepted an unavailable collector")
	}
}
