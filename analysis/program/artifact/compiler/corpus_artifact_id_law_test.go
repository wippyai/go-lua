package compiler

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// corpusFixturePath names one testdata/fixtures Lua source by its absolute
// location and its repository-relative name, the latter used as the report
// key so two runs can be diffed by fixture identity rather than path order.
type corpusFixturePath struct {
	absolute string
	relative string
}

// corpusFixturePaths walks the complete testdata/fixtures denominator. It is
// independent of the process working directory: package tests can run from a
// cache, a subdirectory, or a repository checkout with a different parent.
func corpusFixturePaths(t *testing.T) []corpusFixturePath {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve corpus fixture location")
	}
	repository := corpusRepositoryRoot(t, filepath.Dir(thisFile))
	root := filepath.Join(repository, "testdata", "fixtures")
	var fixtures []corpusFixturePath
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
		fixtures = append(fixtures, corpusFixturePath{absolute: path, relative: filepath.ToSlash(relative)})
		return nil
	}); err != nil {
		t.Fatalf("walk corpus fixtures: %v", err)
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].relative < fixtures[j].relative })
	return fixtures
}

func corpusRepositoryRoot(t testing.TB, start string) string {
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

// corpusCompileOutcome is one fixture's settled compile result: either the
// sealed ArtifactID, or the CompileFailure text when the current pipeline
// cannot seal that fixture. Comparing outcomes rather than requiring success
// makes the law a pure determinism check -- a fixture the pipeline cannot
// yet compile still owes an identical failure on every run -- without
// conflating it with compile-completeness, which is a different corpus law.
type corpusCompileOutcome struct {
	id      identity.ContentID
	failure string
}

// corpusCompileVariant produces one build's outcome for a lowered fixture
// Program. A migration commit passes two variants -- the pre-cut and
// post-cut compile paths -- to corpusArtifactIDReport and diffs the two
// reports as its A/B gate.
type corpusCompileVariant func(published *program.Program, grammar programartifact.GrammarIdentity) corpusCompileOutcome

// corpusArtifactIDReport compiles every parser-valid fixture in
// testdata/fixtures through one build variant and reports each fixture's
// outcome by its repository-relative path. A fixture that fails to lower is
// skipped here: parser validity is the lowering corpus law's subject, not
// this one's.
func corpusArtifactIDReport(t *testing.T, grammar programartifact.GrammarIdentity, build corpusCompileVariant) map[string]corpusCompileOutcome {
	t.Helper()
	report := make(map[string]corpusCompileOutcome)
	for _, fixture := range corpusFixturePaths(t) {
		source, err := os.ReadFile(fixture.absolute)
		if err != nil {
			t.Fatalf("read %s: %v", fixture.relative, err)
		}
		published, lowerErr := lower.Lower(lower.Source{Name: fixture.relative, Text: source})
		if lowerErr != nil || published == nil || !published.Available() {
			continue
		}
		report[fixture.relative] = build(published, grammar)
	}
	return report
}

func corpusCompileDetailedVariant(published *program.Program, grammar programartifact.GrammarIdentity) corpusCompileOutcome {
	artifact, failure := CompileDetailed(published, grammar, IssuanceDirectory{})
	if failure.Available() {
		return corpusCompileOutcome{failure: failure.Error()}
	}
	if artifact == nil || !artifact.Available() {
		return corpusCompileOutcome{failure: "artifact unavailable"}
	}
	return corpusCompileOutcome{id: artifact.ID()}
}

// TestCorpusArtifactIDIsCompileDeterministic is gate 1 from the population-
// algebra hostile review (finding 5): a byte-identical ArtifactID across two
// compiles of the same lowered Program, banked in memory over the complete
// fixture corpus rather than persisted to testdata. It is the harness a
// migration commit extends into an A/B by swapping in a second
// corpusCompileVariant and diffing the two reports.
func TestCorpusArtifactIDIsCompileDeterministic(t *testing.T) {
	grammar, ok := programartifact.NewGrammarIdentity(identity.ContentID{1}, programartifact.GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	first := corpusArtifactIDReport(t, grammar, corpusCompileDetailedVariant)
	second := corpusArtifactIDReport(t, grammar, corpusCompileDetailedVariant)
	if len(first) == 0 {
		t.Fatal("corpus produced no lowerable fixtures")
	}
	if len(first) != len(second) {
		t.Fatalf("compile report width changed across runs: %d vs %d", len(first), len(second))
	}
	sealed := 0
	for path, outcome := range first {
		other, ok := second[path]
		if !ok {
			t.Fatalf("outcome(%s) present on first compile, absent on second", path)
		}
		if other != outcome {
			t.Fatalf("compile(%s) not deterministic: id=%v failure=%q / id=%v failure=%q", path, outcome.id, outcome.failure, other.id, other.failure)
		}
		if outcome.failure == "" {
			if !outcome.id.Available() {
				t.Fatalf("compile(%s) reported success with no ArtifactID", path)
			}
			sealed++
		}
	}
	if sealed == 0 {
		t.Fatal("corpus produced no sealed artifacts")
	}
	t.Logf("corpus fixtures: %d lowerable, %d sealed", len(first), sealed)
}
