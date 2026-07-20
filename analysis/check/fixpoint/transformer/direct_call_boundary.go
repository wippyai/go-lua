package transformer

import "github.com/wippyai/go-lua/analysis/symbol"

// DirectCallBoundary is the exact callee namespace order needed to bind
// RootCapture and RootGlobal terms from the caller relation. Parameters are
// already ordered by the call site's value-source list.
type DirectCallBoundary struct {
	Captures []symbol.ID
	Globals  []symbol.ID
	Ambients []AmbientRoot
}
