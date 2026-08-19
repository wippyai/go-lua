package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// The synthetic-source links below share the corpus' single-module seal so
// one construction path serves both fixture text and inline law text.
func mustLink(t testing.TB, text string, contract *target.Contract) *link.Link {
	t.Helper()
	return fixtureSourceLink(t, contract, "analysis.lua", []byte(text))
}
