package lint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirectoryReportsUnreadableFileAsDiagnostic(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "ok.lua")
	if err := os.WriteFile(good, []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing_target.lua"), filepath.Join(root, "broken.lua")); err != nil {
		t.Fatal(err)
	}
	input, err := LoadDirectory(root, nil)
	if err != nil {
		t.Fatalf("LoadDirectory must survive a broken symlink: %v", err)
	}
	if len(input.Entries) != 1 || input.Entries[0].Path != "ok.lua" {
		t.Fatalf("expected exactly the readable entry, got %#v", input.Entries)
	}
	if len(input.LoadFailures) != 1 || input.LoadFailures[0].Path != "broken.lua" {
		t.Fatalf("expected broken.lua load failure, got %#v", input.LoadFailures)
	}
	result, err := CheckProject(context.Background(), input)
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	found := false
	for _, item := range result.Diagnostics {
		if string(item.Code) == "lint.load.unreadable" && strings.Contains(item.Message, "broken.lua") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected lint.load.unreadable diagnostic for broken.lua, got %#v", result.Diagnostics)
	}
}

func TestLoadDirectoryExplicitEntryUnreadableFailsLoudly(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing_target.lua"), filepath.Join(root, "broken.lua")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDirectory(root, []string{"broken.lua"}); err == nil {
		t.Fatal("an explicitly requested unreadable entry must fail loudly")
	}
}
