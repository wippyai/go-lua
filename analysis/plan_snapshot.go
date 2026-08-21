package analysis

import (
	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Publication returns the immutable declaration-derived axis plan carried by
// this Plan. It is a general compile surface: consumers pair its declared
// output keys with Snapshot addresses and never rediscover slots by probing.
func (plan *Plan) Publication() (analysiscatalog.Publication, bool) {
	state, leased := plan.acquire()
	if !leased {
		return analysiscatalog.Publication{}, false
	}
	defer state.releaseLease()
	publication, published := state.compilation.Publication()
	if !published || !publication.Available() {
		return analysiscatalog.Publication{}, false
	}
	return publication, true
}

// Snapshot returns the immutable Link publication produced during Compile.
// It is the general read surface for Link-lifetime rows: callers read the
// schema-declared columns from this value and do not reopen the Link or ask a
// module-specific facade to reconstruct them. The returned value shares the
// sealed publication and copies no rows.
func (plan *Plan) Snapshot() (snapshot.Snapshot, bool) {
	state, leased := plan.acquire()
	if !leased {
		return snapshot.Snapshot{}, false
	}
	defer state.releaseLease()
	if !state.composition.Published() {
		return snapshot.Snapshot{}, false
	}
	return state.composition, true
}
