// Package observation owns the immutable address of one Call-dispatch
// observation row.  The admission walk lives in composite; this package owns
// only the row identity so no consumer can mint a second address vocabulary.
package observation

import "github.com/wippyai/go-lua/analysis/identity"

const domain = "wippy.analysis.call.dispatch-observation.v1\x00"

// ID derives the owner-qualified address of one mounted application
// occurrence and execution Context.  Every coordinate is required: an
// incomplete tuple is refused instead of acquiring a fallback address.
func ID(linkID, applicationID, mount, occurrence, contextID identity.ContentID) (identity.ContentID, bool) {
	if !linkID.Available() || !applicationID.Available() || !mount.Available() ||
		!occurrence.Available() || !contextID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(domain, linkID[:], applicationID[:], mount[:], occurrence[:], contextID[:])
}
