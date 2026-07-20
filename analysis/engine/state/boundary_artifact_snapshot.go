package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// ProjectedWorld returns the artifact's complete, root-reachable final world
// and ordered roots for read-only conversion at a function boundary. Registry
// and keyspace authority must be the exact authorities that produced the
// artifact.
//
// State is persistent: its exported updates return a new State. The roots
// slice is copied here, so neither result grants mutable ownership of the
// artifact. Every enabled lane in the returned State was projected through
// the catalog-owned boundary policy used by ProjectBoundary.
func (a BoundaryArtifact) ProjectedWorld(reg *axis.Registry, keys *keyspace.KeySpace) (State, BoundaryRoots, error) {
	if reg == nil || a.reg != reg || keys == nil || !keys.Valid() || a.keys != keys {
		return State{}, nil, fmt.Errorf("state: projected boundary world requires the artifact authorities")
	}
	roots := append(BoundaryRoots(nil), a.roots...)
	return a.world.Snapshot(), roots, nil
}
