package programlower_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/programlower"
	"github.com/wippyai/go-lua/compiler/parse"
)

// frozenFixtureLuaFiles is the complete parser-valid fixture denominator at
// the canonical Program cut. Changing it is a target-contract change, not a
// way to hide an unlowered source family.
const frozenFixtureLuaFiles = 1176

var sourcePositionSuffix = regexp.MustCompile(` at [0-9]+:[0-9]+$`)

type fixtureFailureCluster struct {
	summary string
	paths   []string
}

// TestFrozenFixtureCorpusLowersAndSeals is intentionally black-box: it knows
// only the public parser, binder, and Program lowerer. Lower seals before it
// returns, so a pass proves the complete parser -> bind -> Program -> Seal
// path for every checked-in fixture source, including expected-diagnostic
// cases that later analysis must diagnose rather than reject here.
func TestFrozenFixtureCorpusLowersAndSeals(t *testing.T) {
	t.Helper()
	fixtures := frozenFixturePaths(t)
	if len(fixtures) != frozenFixtureLuaFiles {
		t.Fatalf(
			"frozen fixture denominator = %d Lua files; want exactly %d (corpus changes require an explicit target-contract update)",
			len(fixtures), frozenFixtureLuaFiles,
		)
	}

	failures := make(map[string]*fixtureFailureCluster)
	for _, fixture := range fixtures {
		source, err := os.ReadFile(fixture.absolute)
		if err == nil {
			stmts, parseErr := parse.ParseString(string(source), fixture.relative)
			err = parseErr
			if err == nil {
				_, err = programlower.Lower(
					fixture.relative,
					stmts,
					bind.BindChunk(stmts, bind.Options{}),
				)
			}
		}
		if err == nil {
			continue
		}
		cluster := fixtureFailureSummary(err)
		if failures[cluster] == nil {
			failures[cluster] = &fixtureFailureCluster{summary: cluster}
		}
		failures[cluster].paths = append(failures[cluster].paths, fixture.relative)
	}
	if len(failures) == 0 {
		return
	}

	clusters := make([]*fixtureFailureCluster, 0, len(failures))
	failed := 0
	for _, cluster := range failures {
		sort.Strings(cluster.paths)
		failed += len(cluster.paths)
		clusters = append(clusters, cluster)
	}
	sort.Slice(clusters, func(i, j int) bool {
		if len(clusters[i].paths) != len(clusters[j].paths) {
			return len(clusters[i].paths) > len(clusters[j].paths)
		}
		return clusters[i].summary < clusters[j].summary
	})

	var report strings.Builder
	fmt.Fprintf(&report, "%d/%d fixture Lua files failed parser -> bind -> Program -> Seal:\n", failed, len(fixtures))
	for _, cluster := range clusters {
		fmt.Fprintf(&report, "  %d x %s", len(cluster.paths), cluster.summary)
		for _, path := range cluster.paths[:min(len(cluster.paths), 3)] {
			fmt.Fprintf(&report, "\n    %s", path)
		}
		report.WriteByte('\n')
	}
	t.Fatal(report.String())
}

type fixturePath struct {
	absolute string
	relative string
}

func frozenFixturePaths(t *testing.T) []fixturePath {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve fixture census test location")
	}
	repository := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	root := filepath.Join(repository, "testdata", "fixtures")
	var fixtures []fixturePath
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		relative, err := filepath.Rel(repository, path)
		if err != nil {
			return err
		}
		fixtures = append(fixtures, fixturePath{
			absolute: path,
			relative: filepath.ToSlash(relative),
		})
		return nil
	}); err != nil {
		t.Fatalf("walk frozen fixture corpus: %v", err)
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].relative < fixtures[j].relative })
	return fixtures
}

func fixtureFailureSummary(err error) string {
	summary := sourcePositionSuffix.ReplaceAllString(err.Error(), "")
	if strings.HasPrefix(summary, "programlower: unsupported typed function parameter ") {
		return "programlower: unsupported typed function parameter"
	}
	return summary
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
