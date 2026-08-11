package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/transaction"
)

func TestOptionsHaveOneMutuallyExclusiveMode(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want commandMode
		bad  bool
	}{
		{name: "prepare", args: []string{"-intent", "intent.json"}, want: modePrepare},
		{name: "replay", args: []string{"-lock", "cut.lock"}, want: modeReplay},
		{name: "apply", args: []string{"-lock", "cut.lock", "-apply"}, want: modeApply},
		{name: "discover", args: []string{"-discover", "pkg"}, want: modeDiscover},
		{name: "recovery inspect", args: []string{"-recovery", "inspect"}, want: modeRecover},
		{name: "complete", args: []string{"-recovery", "complete", "-lock", "cut.lock"}, want: modeRecover},
		{name: "mixed cut", args: []string{"-intent", "a", "-lock", "b"}, bad: true},
		{name: "mixed discovery", args: []string{"-discover", "pkg", "-intent", "a"}, bad: true},
		{name: "unsafe recovery", args: []string{"-recovery", "rollback", "-lock", "b"}, bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			options, err := parseOptions(test.args)
			if err != nil {
				t.Fatal(err)
			}
			got, err := options.mode()
			if test.bad {
				if err == nil {
					t.Fatalf("accepted %v as %s", test.args, got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("mode=%s err=%v want=%s", got, err, test.want)
			}
		})
	}
}

func TestIntentDecodeRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "intent.json")
	if err := os.WriteFile(path, []byte(`{"schema":2,"name":"x","operations":[],"actions":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readIntent(path); err == nil {
		t.Fatal("unknown action vocabulary was accepted")
	}
}

func TestArtifactCannotBecomeCutAuthority(t *testing.T) {
	root := t.TempDir()
	path, err := resolveArtifact(root, ".flashrefactor/locks/cut.json", artifactLock)
	if err != nil {
		t.Fatal(err)
	}
	intent := cutplan.Intent{Schema: cutplan.Version, Name: "x", Operations: []cutplan.Operation{{Footprint: cutplan.Footprint{Read: []string{".flashrefactor/locks/cut.json"}, Write: []string{".flashrefactor/locks/cut.json"}}}}}
	if err := rejectArtifactAuthority(root, intent, path); err == nil {
		t.Fatal("artifact gained source authority")
	}
}

func TestControlDirectoryCannotBecomeCutAuthority(t *testing.T) {
	intent := cutplan.Intent{Schema: cutplan.Version, Name: "x", Operations: []cutplan.Operation{{Footprint: cutplan.Footprint{Read: []string{".flashrefactor/transaction/journal.json"}, Write: []string{"source.go"}}}}}
	if err := rejectControlAuthority(intent); err == nil {
		t.Fatal("control directory gained source authority")
	}
}

func TestArtifactWritesAtomicallyInControlDirectory(t *testing.T) {
	root := t.TempDir()
	path, err := resolveArtifact(root, ".flashrefactor/reports/result.json", artifactReport)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("resolution wrote artifact directory: %v", err)
	}
	if err := writeAtomic(path, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "first\n" {
		t.Fatalf("artifact=%q err=%v", data, err)
	}
	if err := writeAtomic(path, []byte("second\n")); err == nil {
		t.Fatal("different artifact bytes overwrote immutable evidence")
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "first\n" {
		t.Fatalf("different artifact changed bytes: %q err=%v", data, err)
	}
	if _, err := resolveArtifact(root, "source.json", artifactReport); err == nil {
		t.Fatal("ordinary source location accepted as report artifact")
	}
}

func TestSafetyFailureRetainsExit125(t *testing.T) {
	if code := commandErrorCode(fmt.Errorf("gate: %w", transaction.ErrSafetyFailure)); code != 125 {
		t.Fatalf("safety code = %d, want 125", code)
	}
}
