package runtime

import (
	relationruntime "github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	admission "github.com/wippyai/go-lua/analysis/program/relationadmission"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
)

// Solve runs the one production transition for an admitted relation.  Ready
// is the sealed handoff from admission; no declarations, factories, or
// replacement state may enter after this boundary.  A successful solve is
// immediately published through the existing canonical snapshot builder.
func Solve(ready admission.Ready) (canonical.Snapshot, bool) {
	if !ready.Available() {
		return canonical.Snapshot{}, false
	}

	result, solved := relationruntime.Solve(ready.Mounted(), ready.Base(), ready.Geometry())
	if !solved {
		return canonical.Snapshot{}, false
	}

	projection, projected := snapshot.Publish(result, ready.Geometry())
	if !projected {
		return canonical.Snapshot{}, false
	}
	published := projection.Snapshot()
	if !published.Published() {
		return canonical.Snapshot{}, false
	}
	return published, true
}
