package dispatch

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/program/keyspace"
)

// SiteForTest exposes the package-private site only to external law tests.
// The alias and helpers are test-build symbols; production callers still
// receive sites only through Rule's owner-fenced constructors.
type SiteForTest = site

func NewSiteForTest(algebra *calldomain.Algebra, values *valuedomain.Schema, heaps heapdomain.Schema, packs *packdomain.Schema, applicationID keyspace.ContentID) (SiteForTest, bool) {
	return newSite(algebra, values, heaps, packs, applicationID)
}

func SiteRequireSeedForTest(bound SiteForTest) keyspace.ContentID { return bound.requireSeedID }

func SiteWithRequireSeedForTest(bound SiteForTest, seed keyspace.ContentID) SiteForTest {
	bound.requireSeedID = seed
	return bound
}

func SiteValidForTest(bound SiteForTest) bool { return bound.valid() }
