// Package provenance publishes the four-owner fence shared by every Flow
// sibling. A Flow projection is meaningful only for one exact committed
// Source/Flow/Static/Module quartet; this type is that quartet as a value.
package provenance

import "github.com/wippyai/go-lua/analysis/identity"

// Provenance is the explicit four-owner fence for a published Flow
// projection. It is not a composite build ID and carries no owner pointer.
type Provenance struct {
	Source identity.ContentID
	Flow   identity.ContentID
	Static identity.ContentID
	Module identity.ContentID
}

// Available reports whether all four owner identities are present.
func (p Provenance) Available() bool {
	return p.Source.Available() && p.Flow.Available() && p.Static.Available() && p.Module.Available()
}
