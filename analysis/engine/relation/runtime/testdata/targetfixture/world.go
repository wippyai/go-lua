package targetfixture

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// World is the one target-runtime fixture result. It exposes only the mounted
// witness, geometry, and immutable bootstrap root required by Solve.
type World struct {
	mounted witness.Mounted
	view    geometry.Geometry
	base    database.Version
}

func (value World) Mounted() witness.Mounted { return value.mounted }
func (value World) View() geometry.Geometry  { return value.view }
func (value World) Base() database.Version   { return value.base }

// Solve executes the production target runtime over this fixture's sole root.
func (value World) Solve() (terminal.Result, bool) {
	return runtime.Solve(value.mounted, value.base, value.view)
}

// Snapshot projects a terminal target root into the canonical snapshot API.
func (value World) Snapshot(result terminal.Result) (snapshot.Projection, bool) {
	return snapshot.Publish(result, value.view)
}
