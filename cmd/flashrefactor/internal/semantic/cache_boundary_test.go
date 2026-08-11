package semantic

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

type recordingLoader struct {
	requests []LoadRequest
}

func (loader *recordingLoader) Load(ctx context.Context, request LoadRequest) (LoadResult, error) {
	loader.requests = append(loader.requests, LoadRequest{
		Root: request.Root, Scratch: request.Scratch, Environment: cloneStrings(request.Environment),
		BuildFlags: cloneStrings(request.BuildFlags), Patterns: cloneStrings(request.Patterns),
	})
	return packagesLoader{}.Load(ctx, request)
}

func TestPackagesEnvironmentRetainsInheritedGoCache(t *testing.T) {
	inherited := filepath.Join(t.TempDir(), "bounded-go-cache")
	t.Setenv("GOCACHE", inherited)
	t.Setenv("GOPACKAGESDRIVER", "unexpected-driver")
	values := environmentValues(t, packagesEnvironment())
	if got := values["GOCACHE"]; got != inherited {
		t.Fatalf("GOCACHE=%q, want inherited %q", got, inherited)
	}
	if got := values["GOPACKAGESDRIVER"]; got != "off" {
		t.Fatalf("GOPACKAGESDRIVER=%q, want off", got)
	}
}

func TestSessionScratchIsSeparateFromGoActionCache(t *testing.T) {
	root, parent := testWorkspace(t), t.TempDir()
	inherited := filepath.Join(t.TempDir(), "bounded-go-cache")
	t.Setenv("GOCACHE", inherited)
	session, err := NewSession(Config{Root: root, Flashrefactor: "test", CacheParent: parent})
	if err != nil {
		t.Fatal(err)
	}
	scratch := session.ScratchPath()
	if scratch == "" || scratch == inherited || filepath.Dir(scratch) != parent || !strings.HasPrefix(filepath.Base(scratch), "flashrefactor-scratch-") {
		t.Fatalf("session scratch=%q, inherited action cache=%q", scratch, inherited)
	}
	if _, err := os.Stat(scratch); err != nil {
		t.Fatalf("session scratch missing: %v", err)
	}
	if got := environmentValues(t, packagesEnvironment())["GOCACHE"]; got != inherited {
		t.Fatalf("session scratch replaced inherited action cache: %q", got)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("session scratch survived close: %v", err)
	}
}

func TestSessionFreezesOneSemanticLoadContext(t *testing.T) {
	root := testWorkspace(t)
	loader := &recordingLoader{}
	session, err := NewSession(Config{Root: root, Flashrefactor: "test", Loader: loader})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	source, err := testCollect(session, context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "changed-after-source"))
	target, err := testCollectVirtual(session, context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(loader.requests) != 2 {
		t.Fatalf("loads=%d, want source and target", len(loader.requests))
	}
	if !bytes.Equal([]byte(strings.Join(loader.requests[0].Environment, "\x00")), []byte(strings.Join(loader.requests[1].Environment, "\x00"))) ||
		!bytes.Equal([]byte(strings.Join(loader.requests[0].BuildFlags, "\x00")), []byte(strings.Join(loader.requests[1].BuildFlags, "\x00"))) ||
		!bytes.Equal([]byte(strings.Join(loader.requests[0].Patterns, "\x00")), []byte(strings.Join(loader.requests[1].Patterns, "\x00"))) {
		t.Fatalf("source and target used different frozen load contexts: %#v %#v", loader.requests[0], loader.requests[1])
	}
	if source.Toolchain != target.Toolchain || source.Authority != target.Authority {
		t.Fatalf("frozen source/target evidence drifted: %#v %#v", source.Toolchain, target.Toolchain)
	}
	if got := loader.requests[0].BuildFlags; len(got) != 2 || got[0] != "-buildvcs=false" || got[1] != "-trimpath" {
		t.Fatalf("semantic build flags=%#v", got)
	}
}

func TestSessionRejectsLocationBoundGoInputs(t *testing.T) {
	root := testWorkspace(t)
	for name, set := range map[string]struct{ key, value string }{
		"workfile": {"GOWORK", filepath.Join(t.TempDir(), "go.work")},
		"modfile":  {"GOFLAGS", "-modfile=elsewhere.mod"},
		"overlay":  {"GOFLAGS", "-overlay elsewhere.json"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(set.key, set.value)
			if _, err := NewSession(Config{Root: root, Flashrefactor: "test"}); err == nil {
				t.Fatal("location-bound Go input accepted")
			}
		})
	}
}

func TestSessionCanonicalScratchNeverEntersSemanticWorkspace(t *testing.T) {
	for _, parentKind := range []string{"symlinked", "relative"} {
		t.Run(parentKind, func(t *testing.T) {
			root := testWorkspace(t)
			var configured, wantParent string
			switch parentKind {
			case "symlinked":
				wantParent = filepath.Join(root, "scratch-real")
				if err := os.Mkdir(wantParent, 0o700); err != nil {
					t.Fatal(err)
				}
				configured = filepath.Join(root, "scratch-link")
				if err := os.Symlink(wantParent, configured); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			case "relative":
				wantParent = filepath.Join(root, "scratch-relative")
				if err := os.Mkdir(wantParent, 0o700); err != nil {
					t.Fatal(err)
				}
				previous, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(root); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chdir(previous) })
				configured = "scratch-relative"
			}
			session, err := NewSession(Config{Root: root, Flashrefactor: "test", CacheParent: configured})
			if err != nil {
				t.Fatal(err)
			}
			scratch := session.ScratchPath()
			if filepath.Dir(scratch) != wantParent || strings.HasPrefix(scratch, configured+string(filepath.Separator)) {
				t.Fatalf("scratch=%q was not canonicalized from parent=%q", scratch, configured)
			}
			qualificationWriteScratch(t, scratch, "leak/leak.go", "package leak\n\nconst Hidden = 1\n")
			request := SymbolRequest{Object: goneMethod, Role: cutplan.ObjectSource}
			snapshot, err := testCollect(session, context.Background(), []SymbolRequest{request}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range snapshot.Workspace.Files() {
				if strings.Contains(file.Path, "flashrefactor-scratch-") || strings.Contains(file.Path, "leak/leak.go") {
					t.Fatalf("scratch entered semantic workspace: %#v", file)
				}
			}
			shadow, _, cleanup, err := session.virtualWorkspace([]VirtualFile{{Path: "pkg/gone/g.go", Content: []byte("package gone\n\ntype T struct{}\n")}})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			relative, err := filepath.Rel(root, scratch)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(shadow, relative)); !os.IsNotExist(err) {
				t.Fatalf("scratch entered virtual shadow: %v", err)
			}
			if err := session.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(scratch); !os.IsNotExist(err) {
				t.Fatalf("canonical scratch survived close: %v", err)
			}
		})
	}
}

func TestSessionScratchDoesNotSuppressWorkspaceAliases(t *testing.T) {
	for _, aliasKind := range []string{"root", "self"} {
		t.Run(aliasKind, func(t *testing.T) {
			root := testWorkspace(t)
			parent := filepath.Join(root, "scratch-parent")
			if err := os.Mkdir(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			session, err := NewSession(Config{Root: root, Flashrefactor: "test", CacheParent: parent})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			alias := filepath.Join(root, "workspace-alias")
			target := root
			if aliasKind == "self" {
				target = alias
			}
			if err := os.Symlink(target, alias); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			_, _, cleanup, err := session.virtualWorkspace([]VirtualFile{{Path: "pkg/gone/g.go", Content: []byte("package gone\n")}})
			if cleanup != nil {
				defer cleanup()
			}
			if err == nil {
				t.Fatalf("workspace %s alias was silently omitted", aliasKind)
			}
			if aliasKind == "root" && !strings.Contains(err.Error(), "rejects symlink") {
				t.Fatalf("root alias failed for the wrong reason: %v", err)
			}
		})
	}
}

func TestCleanupCreatedScratchReportsBothFailures(t *testing.T) {
	canonicalize := errors.New("canonicalization failed")
	cleanup := errors.New("cleanup failed")
	var removed string
	err := cleanupCreatedScratch("/scratch/created", canonicalize, func(path string) error {
		removed = path
		return cleanup
	})
	if removed != "/scratch/created" {
		t.Fatalf("cleanup path=%q", removed)
	}
	if !errors.Is(err, canonicalize) || !errors.Is(err, cleanup) {
		t.Fatalf("combined cleanup error lost a cause: %v", err)
	}
	if !strings.Contains(err.Error(), "remove created semantic scratch /scratch/created") {
		t.Fatalf("combined cleanup error omits cleanup context: %v", err)
	}
}

func qualificationWriteScratch(t *testing.T, root, path, source string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func environmentValues(t *testing.T, environment []string) map[string]string {
	t.Helper()
	values := make(map[string]string, len(environment))
	for _, value := range environment {
		key, content, found := strings.Cut(value, "=")
		if !found || key == "" {
			t.Fatalf("malformed environment entry %q", value)
		}
		if _, exists := values[key]; exists {
			t.Fatalf("duplicate environment key %q", key)
		}
		values[key] = content
	}
	return values
}
