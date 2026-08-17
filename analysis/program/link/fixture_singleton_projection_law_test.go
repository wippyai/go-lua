package link_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/internal/testfixture"
	"github.com/wippyai/go-lua/analysis/targetprofile"
)

// loadFixtureCorpus enumerates the checked-in corpus once per package run. The
// repository root is derived from this source file, so the census is
// independent of the working directory a test runs in, and the walk is held by
// the test package rather than repeated per test function.
var loadFixtureCorpus = sync.OnceValues(func() (*testfixture.Corpus, error) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errLinkTestSourceLocation
	}
	repository, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		return nil, err
	}
	return testfixture.LoadCorpus(repository)
})

var errLinkTestSourceLocation = errors.New("link test source location unavailable")

func fixtureCorpus(t *testing.T) *testfixture.Corpus {
	t.Helper()
	corpus, err := loadFixtureCorpus()
	if err != nil {
		t.Fatalf("load frozen corpus: %v", err)
	}
	return corpus
}

// These are deliberately separate from the corpus denominator gate: each
// keeps an individually runnable proof for a source shape that once exposed a
// missing Program-to-Link projection.
func TestRecurrenceExitArmRetainsCanonicalReadProjection(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	project, err := fixtureCorpus(t).Project("soundness/recurrence-exit-arm")
	if err != nil {
		t.Fatalf("fixture project: %v", err)
	}
	if _, err := testfixture.SealCorpusProject(contract, project); err != nil {
		t.Fatalf("seal recurrence-exit-arm through canonical Program-to-Link projection: %v", err)
	}
}

func TestBackwardGotoSealsCanonicalPackProjection(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	project, err := fixtureCorpus(t).Project("functions/goto-backward")
	if err != nil {
		t.Fatalf("fixture project: %v", err)
	}
	if _, err := testfixture.SealCorpusProject(contract, project); err != nil {
		t.Fatalf("seal goto-backward through canonical Program-to-Link projection: %v", err)
	}
}
