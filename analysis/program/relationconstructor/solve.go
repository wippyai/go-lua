package relationconstructor

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Solved is one constructed relation program and the answer it settled on.
//
// It keeps the three authorities the runtime solved over beside the terminal
// result and the projection published from that same result. Publishing
// through the view that solved is the whole reason this value exists: a
// projection taken against a second view would address its rows in a geometry
// no part of this answer was computed in.
type Solved struct {
	mounted    witness.Mounted
	view       geometry.Geometry
	base       database.Version
	result     terminal.Result
	projection snapshot.Projection
	sealed     bool
}

// Solve runs the relation runtime over one mounted execution and publishes the
// terminal root it settles on.
//
// The arguments are exactly the runtime's own, so this adds no planning step
// and no second solve path. It fails closed: a refused solve or a projection
// that will not publish yields the unavailable zero rather than an answer whose
// rows are silently absent.
func Solve(mounted witness.Mounted, base database.Version, view geometry.Geometry) (Solved, bool) {
	if !mounted.Available() || !base.Available() || !view.ValidFor(mounted) {
		return Solved{}, false
	}
	result, ok := runtime.Solve(mounted, base, view)
	if !ok || !result.Available() {
		return Solved{}, false
	}
	projection, ok := snapshot.Publish(result, view)
	if !ok || !projection.Available() {
		return Solved{}, false
	}
	value := Solved{mounted: mounted, view: view, base: base, result: result, projection: projection, sealed: true}
	if !value.Available() {
		return Solved{}, false
	}
	return value, true
}

// Available reports whether this answer still carries every authority it was
// sealed with.
func (solved Solved) Available() bool {
	return solved.sealed && solved.mounted.Available() && solved.base.Available() &&
		solved.view.ValidFor(solved.mounted) && solved.result.Available() && solved.projection.Available()
}

// Mounted returns the mounted execution the answer was solved over.
func (solved Solved) Mounted() witness.Mounted {
	if !solved.Available() {
		return witness.Mounted{}
	}
	return solved.mounted
}

// View returns the physical scope authority the answer was solved and
// published through.
func (solved Solved) View() geometry.Geometry {
	if !solved.Available() {
		return geometry.Geometry{}
	}
	return solved.view
}

// Base returns the immutable root the solve started from.
func (solved Solved) Base() database.Version {
	if !solved.Available() {
		return database.Version{}
	}
	return solved.base
}

// Result returns the terminal result, including the root the solve settled on
// and the evaluation and publication counts it reached.
func (solved Solved) Result() terminal.Result {
	if !solved.Available() {
		return terminal.Result{}
	}
	return solved.result
}

// Projection returns the published snapshot of the settled root.
func (solved Solved) Projection() snapshot.Projection {
	if !solved.Available() {
		return snapshot.Projection{}
	}
	return solved.projection
}
