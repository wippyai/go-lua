package dispatch

import (
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// SiteForTest exposes the package-private site only to external law tests.
// The alias and helpers are test-build symbols; production callers still
// receive sites only through Rule's owner-fenced constructors.
type SiteForTest = site

func NewSiteForTest(algebra *calldomain.Algebra, values *valuedomain.Schema, heaps heapdomain.Schema, packs *packdomain.Schema, applicationID identity.ContentID) (SiteForTest, bool) {
	return newSite(algebra, values, heaps, packs, applicationID)
}

func SiteRequireSeedForTest(bound SiteForTest) identity.ContentID { return bound.requireSeedID }

func SiteWithRequireSeedForTest(bound SiteForTest, seed identity.ContentID) SiteForTest {
	bound.requireSeedID = seed
	return bound
}

func SiteValidForTest(bound SiteForTest) bool { return bound.valid() }
