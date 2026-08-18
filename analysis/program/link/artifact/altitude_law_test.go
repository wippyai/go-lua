package artifact

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The codec is a package of its own because persistence is one altitude below
// the sealed Link: it reads Link through the published child authorities and
// reopens one through the ordinary Seal. Making it a package puts that
// direction under the compiler rather than under review - a Link authority
// cannot name the codec, because naming it would close a cycle.
//
// The two laws below state the direction and keep it from being vacuous.

const (
	linkPackage     = "github.com/wippyai/go-lua/analysis/program/link"
	artifactPackage = linkPackage + "/artifact"
)

// TestLinkAuthoritiesNameNoArtifactCodec states the direction over every Link
// authority: neither the sealed root nor any child names the codec, so no Link
// judgment can be written in terms of how a Link is stored. The scan is over
// published sources; an external test package sits at the consumer altitude
// and is free to name both.
func TestLinkAuthoritiesNameNoArtifactCodec(t *testing.T) {
	fileset := token.NewFileSet()
	root := linkTreeRoot(t)
	scanned := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && filepath.Base(path) == "artifact" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		scanned++
		for _, spec := range parsed.Imports {
			if strings.Trim(spec.Path.Value, `"`) == artifactPackage {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s imports the artifact codec; a Link authority never names its persistence", filepath.ToSlash(relative))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("Link tree has no sources")
	}
}

// TestArtifactCodecIsWrittenInTheSealedLink keeps the direction law honest: the
// codec names the sealed Link, so the closure above constrains two packages
// that actually meet rather than two unrelated ones.
func TestArtifactCodecIsWrittenInTheSealedLink(t *testing.T) {
	fileset := token.NewFileSet()
	naming := 0
	sources, err := filepath.Glob(filepath.Join(codecDirectory(t), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), parseErr)
		}
		for _, spec := range parsed.Imports {
			if strings.Trim(spec.Path.Value, `"`) == linkPackage {
				naming++
			}
		}
	}
	if naming == 0 {
		t.Fatal("no codec source names the sealed Link; the direction law would be vacuous")
	}
}

func codecDirectory(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("codec source location unavailable")
	}
	return filepath.Dir(current)
}

func linkTreeRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(codecDirectory(t))
}
