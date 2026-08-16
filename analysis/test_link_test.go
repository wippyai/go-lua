package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// The synthetic-source links below share the harness' single-module seal so
// one construction path serves both fixture text and inline law text.
func directFieldHostileLink(t testing.TB, text string) *link.Link {
	t.Helper()
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	return corpusHarnessSourceLink(t, contract, "analysis_test.lua", []byte(text))
}

func mustLink(t testing.TB, text string, contract *target.Contract) *link.Link {
	t.Helper()
	return corpusHarnessSourceLink(t, contract, "analysis.lua", []byte(text))
}
