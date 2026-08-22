package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// Content identity is a semantic artifact boundary, not a cached lowerer
// detail.  Exercise the frozen corpus twice so every currently reachable
// Program relation must be safe to encode and deterministic on reconstruction.
func TestFrozenFixtureCorpusHasStableProgramContentID(t *testing.T) {
	for _, fixture := range frozenFixturePaths(t) {
		source, err := os.ReadFile(fixture.absolute)
		if err != nil {
			t.Fatalf("read %s: %v", fixture.relative, err)
		}
		first, err := programlower.Lower(programlower.Source{Name: fixture.relative, Text: source})
		if err != nil {
			t.Fatalf("first lower %s: %v", fixture.relative, err)
		}
		second, err := programlower.Lower(programlower.Source{Name: fixture.relative, Text: source})
		if err != nil {
			t.Fatalf("second lower %s: %v", fixture.relative, err)
		}
		left, right := first.ContentID(), second.ContentID()
		if !left.Available() || left != right {
			t.Fatalf("ContentID(%s) = %v / %v; want equal available semantic IDs", fixture.relative, left, right)
		}
	}
}

func TestProgramContentIDTracksSourceSemanticsAndCoordinates(t *testing.T) {
	base, err := programlower.Lower(programlower.Source{Name: "identity.lua", Text: []byte("local x = 1\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	value, err := programlower.Lower(programlower.Source{Name: "identity.lua", Text: []byte("local x = 2\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	span, err := programlower.Lower(programlower.Source{Name: "identity.lua", Text: []byte("\nlocal x = 1\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := programlower.Lower(programlower.Source{Name: "other.lua", Text: []byte("local x = 1\nreturn x")})
	if err != nil {
		t.Fatal(err)
	}
	for label, got := range map[string][32]byte{
		"base": base.ContentID(), "value": value.ContentID(), "span": span.ContentID(), "name": renamed.ContentID(),
	} {
		if got == ([32]byte{}) {
			t.Fatalf("%s ContentID unavailable", label)
		}
	}
	if base.ContentID() == value.ContentID() || base.ContentID() == span.ContentID() || base.ContentID() == renamed.ContentID() {
		t.Fatal("semantic payload, source coordinates, and source identity must each affect ContentID")
	}
}

// frozenFixtureLuaFiles is the complete parser-valid fixture denominator at
// the canonical Program cut. Changing it is a target-contract change, not a
// way to hide an unlowered source family.
const frozenFixtureLuaFiles = 1178

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
			var loweredErr error
			lowered, loweredErr := programlower.Lower(programlower.Source{Name: fixture.relative, Text: source})
			err = loweredErr
			if err == nil {
				_, err = programCountRows(lowered)
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

// programCountRows states the denominator column a lowered Program root owes:
// exactly the three cold owners the root freezes. The ProgramModule family is
// not among them; the root holds no Module component, and those derived
// cardinalities are first sealed at the artifact boundary, which this black-box
// lowering census never reaches.
func programCountRows(p *program.Program) (denominator.CountRows, error) {
	if p == nil {
		return denominator.CountRows{}, fmt.Errorf("nil Program")
	}
	rows := p.CountRows()
	if !denominator.GeneratedCountRowsCompleteForOwners(rows,
		denominator.RelationOwnerProgramSource,
		denominator.RelationOwnerProgramFlow,
		denominator.RelationOwnerProgramStatic,
	) {
		return denominator.CountRows{}, fmt.Errorf("Program denominator rows unavailable or incomplete")
	}
	return rows, nil
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
	repository := repositoryRoot(t, filepath.Dir(thisFile))
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

// repositoryRoot is intentionally independent of the process working
// directory: package tests can run from a cache, a subdirectory, or a
// repository checkout with a different parent path.
func repositoryRoot(t *testing.T, start string) string {
	t.Helper()
	for directory := filepath.Clean(start); ; {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && !info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatalf("cannot locate repository root from %q", start)
		}
		directory = parent
	}
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
