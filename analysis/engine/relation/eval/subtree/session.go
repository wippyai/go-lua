package subtree

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Session is the solve-local capability for one exact mounted root.  It owns
// no reconstructed plan and no row cache: every physical choice used by
// Evaluate is carried by the ApplyReplay/CorrelatedSubtree witnesses or by
// the root-bound reader.
//
// ReadScratch is intentionally supplied by the caller.  This makes the
// state arena part of the session binding and prevents a reader from being
// rebound through a foreign geometry manager.  A Session and its scratch are
// serial capabilities; callers must not use one session concurrently.
type Session struct {
	mounted  witness.Mounted
	root     database.Version
	geometry geometry.Geometry
	scratch  *store.ReadScratch
	sealed   bool
}

// New binds one exact mounted runtime, committed root, geometry, and read
// scratch.  The root and geometry must name the same complete mount and the
// scratch manager must be the geometry's manager.  No arrangement is
// reconstructed or selected here.
func New(mounted witness.Mounted, root database.Version, view geometry.Geometry, scratch *store.ReadScratch) (Session, bool) {
	if !mounted.Available() || !root.Available() || !view.Available() || scratch == nil || !scratch.Available() {
		return Session{}, false
	}
	if !view.ValidFor(mounted) || scratch.Manager() != view.Manager() {
		return Session{}, false
	}
	plan := mounted.Arrangement()
	if !plan.Available() || !root.Mounted().Same(mounted) || !root.Fence().Same(mounted.RuntimeFence()) || root.MountedDigest() != mounted.Digest() || root.ArrangementDigest() != plan.Digest() {
		return Session{}, false
	}
	if root.Arrangement().Digest() != plan.Digest() {
		return Session{}, false
	}
	value := Session{mounted: mounted, root: root, geometry: view, scratch: scratch, sealed: true}
	return value, value.Available()
}

// NewOwned binds a session and allocates scratch from the exact geometry
// manager.  New remains the explicit-scratch constructor used by callers that
// already own a reusable read shell.
func NewOwned(mounted witness.Mounted, root database.Version, view geometry.Geometry) (Session, bool) {
	if !view.Available() {
		return Session{}, false
	}
	scratch := store.NewReadScratch(view.Manager())
	if scratch == nil {
		return Session{}, false
	}
	return New(mounted, root, view, scratch)
}

// Available reports whether the constructor's complete binding still holds.
// It intentionally does not walk a subtree or any mounted extent vectors.
func (session Session) Available() bool {
	if !session.sealed || !session.mounted.Available() || !session.root.Available() || !session.geometry.ValidFor(session.mounted) || session.scratch == nil || !session.scratch.Available() || session.scratch.Manager() != session.geometry.Manager() {
		return false
	}
	plan := session.mounted.Arrangement()
	return plan.Available() && session.root.Mounted().Same(session.mounted) && session.root.Fence().Same(session.mounted.RuntimeFence()) && session.root.MountedDigest() == session.mounted.Digest() && session.root.ArrangementDigest() == plan.Digest() && session.root.Arrangement().Digest() == plan.Digest()
}

// Mounted returns the exact mount capability retained by the session.
func (session Session) Mounted() witness.Mounted {
	if !session.Available() {
		return witness.Mounted{}
	}
	return session.mounted
}

// Root returns the exact committed root retained by the session.
func (session Session) Root() database.Version {
	if !session.Available() {
		return database.Version{}
	}
	return session.root
}

// Geometry returns the exact solve-local geometry retained by the session.
func (session Session) Geometry() geometry.Geometry {
	if !session.Available() {
		return geometry.Geometry{}
	}
	return session.geometry
}
