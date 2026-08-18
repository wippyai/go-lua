package storage

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestStorageAuthorityStartsWithEmptyOwnedRanges(t *testing.T) {
	writer := New(nil, nil, nil, nil, nil, nil, nil, "storage.lua")
	if writer == nil || !writer.Clean() {
		t.Fatal("new storage writer was not clean")
	}
	if mark := writer.TargetMark(); mark.owner != writer || mark.target != 0 {
		t.Fatalf("initial TargetMark = %#v", mark)
	}
}

func TestStorageGlobalSelectionRequiresBinderAndCollectorAuthority(t *testing.T) {
	var writer Writer
	if _, err := writer.Global(bind.GlobalIdentity{}); err == nil {
		t.Fatal("Global accepted an unavailable storage authority")
	}
}

func TestStorageExpressionDispatchRejectsForeignSyntax(t *testing.T) {
	var writer Writer
	if err := writer.ScheduleExpression(nil, 1, programsource.Span{File: "storage.lua"}); err == nil {
		t.Fatal("ScheduleExpression accepted foreign syntax")
	}
}

func TestStorageLensConstructionRequiresCollectorAuthority(t *testing.T) {
	var writer Writer
	if _, err := writer.DotLens(programsource.Span{File: "storage.lua"}, 1, 2, programsource.Span{File: "storage.lua"}, "field"); err == nil {
		t.Fatal("DotLens accepted an unavailable collector")
	}
}

func TestTargetMarkRejectsZeroTargetAndRetainsNoScratch(t *testing.T) {
	var writer Writer
	if err := writer.RememberTarget(programsource.Span{File: "storage.lua"}, 0); err == nil {
		t.Fatal("RememberTarget accepted zero target")
	}
	if !writer.Clean() {
		t.Fatal("failed target admission left scratch state")
	}
}
