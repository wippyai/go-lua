package main

import (
	"errors"

	"github.com/wippyai/go-lua/analysis"
	relsnapshot "github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// RelationMount is everything the relation engine needs to answer for one
// compiled corpus fixture, and therefore the exact contract the production
// constructor owes this driver.
//
// Mounted, Base and View are the three arguments of the relation runtime's
// Solve. Address is the correspondence without which the two engines'
// answers are not in one address space and only their cardinalities could be
// compared: it states, for one projected column and one projected logical
// row, the published family and query site the old engine answers that same
// question under.
type RelationMount struct {
	Mounted witness.Mounted
	Base    database.Version
	View    geometry.Geometry
	Address func(relsnapshot.Column, relsnapshot.RowKey) (family string, site string, ok bool)
}

// ErrConstructorUnavailable is the seam's refusal while the production
// constructor does not exist. It is reported as its own divergence class for
// every fixture it fires on, so the corpus catalogue states exactly how much
// of the corpus the new engine cannot yet be asked about.
var ErrConstructorUnavailable = errors.New(
	"relation constructor unavailable: no production path carries a compiled corpus fixture into the relation engine")

// MountRelationFixture is the one seam of this driver.
//
// It must carry one compiled corpus fixture into the relation engine and hand
// back the mount that engine solves over. Everything downstream of it -- the
// Solve, the canonical projection the answer is published through, the
// comparison, the corpus walk and the report -- is built and exercised
// against the committed runtime; this function is the only thing standing
// between the driver and a full-corpus differential run.
//
// Filling it needs three things that no production package supplies today:
//
//  1. a relation Declaration for a compiled corpus fixture. Every Declaration
//     in the tree is hand-authored by a target fixture; nothing translates the
//     compiled artifact behind an analysis.Plan into one.
//  2. the capability factories that Declaration is specialized against -- a
//     witness inventory bound to the fixture's certificate, binding and
//     algebra registries for its declared types, a lineage authority and a
//     geometry factory. Only test worlds implement them today.
//  3. the Address correspondence above. The relation engine publishes under
//     owner-issued relation, row and column identities; the old engine
//     publishes under family keys and query-site identities. Until one of the
//     two states the other's address, an answer from either side is not
//     comparable to the other.
//
// Until those exist the seam refuses by name. It never returns an empty mount
// and it never lets a fixture pass silently: an unanswerable fixture is a
// recorded divergence class, not a skipped one.
func MountRelationFixture(_ *analysis.Plan, _ testfixture.CorpusProject) (RelationMount, error) {
	return RelationMount{}, ErrConstructorUnavailable
}

// Available reports whether a mount carries every authority the driver needs.
func (mount RelationMount) Available() bool {
	return mount.Mounted.Available() && mount.Base.Available() &&
		mount.Address != nil && mount.View.ValidFor(mount.Mounted)
}
