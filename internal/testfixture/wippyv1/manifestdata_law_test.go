package wippyv1_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/internal/testfixture"
	"github.com/wippyai/go-lua/internal/testfixture/wippyv1"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
)

// TestManifestDataIsNotStale is the anti-drift fence between the Go
// constructors, which carry the transcription trace and remain the source of
// truth, and the checked-in wire-encoded JSON fixture data derived from them.
// A constructor edit that is not followed by
// `go generate ./internal/testfixture/wippyv1` fails here instead of shipping
// silently divergent fixture data.
func TestManifestDataIsNotStale(t *testing.T) {
	repository := manifestDataLawRepositoryRoot(t)
	for _, module := range wippyv1.Modules() {
		t.Run(module.Name, func(t *testing.T) {
			encoded, err := manifestwire.Encode(module.Declaration())
			if err != nil {
				t.Fatalf("encode %s: %v", module.Name, err)
			}
			path := filepath.Join(repository, wippyv1.ManifestDataRelativePath(module.Name))
			checkedIn, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read checked-in manifest data %s: %v", path, err)
			}
			if !bytes.Equal(encoded, checkedIn) {
				t.Fatalf("%s manifest data is stale: run go generate ./internal/testfixture/wippyv1", module.Name)
			}
		})
	}
}

// TestManifestDataLoadsToTheConstructedManifest is the loader-side half of the
// anti-drift fence: what LoadManifestData reads back from the checked-in
// fixture re-encodes byte-identically to what the Go constructor built, so a
// consumer that reads the fixture data instead of calling the constructor
// observes the same manifest.
func TestManifestDataLoadsToTheConstructedManifest(t *testing.T) {
	repository := manifestDataLawRepositoryRoot(t)
	for _, module := range wippyv1.Modules() {
		t.Run(module.Name, func(t *testing.T) {
			loaded, err := wippyv1.LoadManifestData(repository, module.Name)
			if err != nil {
				t.Fatalf("load %s manifest data: %v", module.Name, err)
			}
			loadedEncoded, err := manifestwire.Encode(loaded)
			if err != nil {
				t.Fatalf("re-encode loaded %s: %v", module.Name, err)
			}
			constructedEncoded, err := manifestwire.Encode(module.Declaration())
			if err != nil {
				t.Fatalf("encode constructed %s: %v", module.Name, err)
			}
			if !bytes.Equal(loadedEncoded, constructedEncoded) {
				t.Fatalf("%s loaded manifest data does not round-trip to the constructed manifest", module.Name)
			}
		})
	}
}

// manifestDataLawRepositoryRoot locates the module root from this source
// file, so the fence is independent of the working directory a test runs in.
func manifestDataLawRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("wippyv1 manifest data law source location unavailable")
	}
	repository, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
