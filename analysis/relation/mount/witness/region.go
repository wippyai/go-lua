package witness

import "github.com/wippyai/go-lua/analysis/identity"

// Region is the neutral finite-region law consumed by mount. Mount never
// stores or interprets a concrete support/guard representation; a physical
// owner supplies an immutable implementation and tests may supply a finite
// one. Identity is the only digest projection, while Conjoin and Entails are
// the complete algebra.
type Region interface {
	Identity() (identity.ContentID, bool)
	Conjoin(Region) (Region, bool)
	Entails(Region) bool
}
