package storage

import (
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestTargetMarkRejectsZeroTargetAndRetainsNoScratch(t *testing.T) {
	var writer Writer
	if err := writer.RememberTarget(programsource.Span{File: "storage.lua"}, 0); err == nil {
		t.Fatal("RememberTarget accepted zero target")
	}
	if !writer.Clean() {
		t.Fatal("failed target admission left scratch state")
	}
}
