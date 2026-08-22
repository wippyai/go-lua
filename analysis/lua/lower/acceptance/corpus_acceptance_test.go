package acceptance_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// loadFixtureCorpus enumerates the checked-in corpus once per package run
// through the one canonical walker. Every consumer of the frozen fixture set
// in this package reads through this census rather than re-walking
// testdata/fixtures with a private filter.
var loadFixtureCorpus = sync.OnceValues(func() (*testfixture.Corpus, error) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errAcceptanceTestSourceLocation
	}
	repository, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		return nil, err
	}
	return testfixture.LoadCorpus(repository)
})

var errAcceptanceTestSourceLocation = errors.New("acceptance test source location unavailable")

func fixtureCorpus(t *testing.T) *testfixture.Corpus {
	t.Helper()
	corpus, err := loadFixtureCorpus()
	if err != nil {
		t.Fatalf("load frozen corpus: %v", err)
	}
	return corpus
}

type fixtureSource struct {
	path string
	text []byte
}

// frozenFixtureSources reads every file the canonical corpus census declares,
// through the corpus project's own SourceText accessor. No path outside the
// census-declared file set is read or counted.
func frozenFixtureSources(t *testing.T) []fixtureSource {
	t.Helper()
	var sources []fixtureSource
	for _, project := range fixtureCorpus(t).Projects() {
		for index := 0; index < project.FileCount(); index++ {
			relative, ok := project.FileAt(index)
			if !ok {
				t.Fatalf("fixture project %s has malformed file index %d", project.Name(), index)
			}
			text, err := project.SourceText(filepath.Base(relative))
			if err != nil {
				t.Fatalf("read fixture %s: %v", relative, err)
			}
			sources = append(sources, fixtureSource{
				path: filepath.ToSlash(filepath.Join("testdata", "fixtures", relative)),
				text: text,
			})
		}
	}
	return sources
}

// Content identity is a semantic artifact boundary, not a cached lowerer
// detail.  Exercise the frozen corpus twice so every currently reachable
// Program relation must be safe to encode and deterministic on reconstruction.
func TestFrozenFixtureCorpusHasStableProgramContentID(t *testing.T) {
	for _, fixture := range frozenFixtureSources(t) {
		first, err := programlower.Lower(programlower.Source{Name: fixture.path, Text: fixture.text})
		if err != nil {
			t.Fatalf("first lower %s: %v", fixture.path, err)
		}
		second, err := programlower.Lower(programlower.Source{Name: fixture.path, Text: fixture.text})
		if err != nil {
			t.Fatalf("second lower %s: %v", fixture.path, err)
		}
		left, right := first.ContentID(), second.ContentID()
		if !left.Available() || left != right {
			t.Fatalf("ContentID(%s) = %v / %v; want equal available semantic IDs", fixture.path, left, right)
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
	fixtures := frozenFixtureSources(t)
	if len(fixtures) != testfixture.FrozenLuaFileCount {
		t.Fatalf(
			"frozen fixture denominator = %d Lua files; want exactly %d (corpus changes require an explicit target-contract update)",
			len(fixtures), testfixture.FrozenLuaFileCount,
		)
	}
	failures := make(map[string]*fixtureFailureCluster)
	for _, fixture := range fixtures {
		lowered, err := programlower.Lower(programlower.Source{Name: fixture.path, Text: fixture.text})
		if err == nil {
			_, err = programCountRows(lowered)
		}
		if err == nil {
			continue
		}
		cluster := fixtureFailureSummary(err)
		if failures[cluster] == nil {
			failures[cluster] = &fixtureFailureCluster{summary: cluster}
		}
		failures[cluster].paths = append(failures[cluster].paths, fixture.path)
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
