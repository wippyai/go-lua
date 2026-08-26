package step

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Session is the concrete, serial evaluator context for one committed root.
// It contains only capabilities already sealed by mount/state. The evaluator
// cannot install another plan, substitute a root, or ask a domain how to
// execute a rule.
type Session struct {
	mounted  witness.Mounted
	root     database.Version
	geometry geometry.Geometry
	door     publish.Door
	scratch  *store.ReadScratch
	sealed   bool
}

// New binds the evaluator to one exact mounted root and its solve-local
// geometry. Scratch is born here from the sealed guard manager rather than
// accepted from callers, so a foreign physical arena cannot be smuggled into
// a read or publication operation.
func New(mounted witness.Mounted, root database.Version, view geometry.Geometry) (Session, bool) {
	if !mounted.Available() || !root.Available() || !view.Available() {
		return Session{}, false
	}
	plan := mounted.Arrangement()
	if !plan.Available() || !root.Mounted().Same(mounted) || root.MountedDigest() != mounted.Digest() || root.ArrangementDigest() != plan.Digest() || !root.Fence().Same(mounted.RuntimeFence()) || !view.ValidFor(mounted) {
		return Session{}, false
	}
	manager := view.Manager()
	if manager == nil {
		return Session{}, false
	}
	scratch := store.NewReadScratch(manager)
	if scratch == nil || !scratch.Available() {
		return Session{}, false
	}
	door, ok := publish.New(mounted, view)
	if !ok || !door.Available() {
		return Session{}, false
	}
	result := Session{
		mounted:  mounted,
		root:     root,
		geometry: view,
		door:     door,
		scratch:  scratch,
		sealed:   true,
	}
	return result, result.Available()
}

// Available is an O(1) admission fence. New establishes all cross-owner
// agreement once; evaluation redeems that immutable context rather than
// re-planning or rescanning the mounted schema for every node.
func (session Session) Available() bool {
	return session.sealed && session.mounted.Available() && session.root.Available() && session.geometry.ValidFor(session.mounted) && session.door.Available() && session.scratch != nil && session.scratch.Available() && session.root.MountedDigest() == session.mounted.Digest() && session.root.ArrangementDigest() == session.mounted.Arrangement().Digest() && session.root.Fence().Same(session.mounted.RuntimeFence()) && session.door.Fence().Same(session.mounted.RuntimeFence())
}

// Mounted returns the exact immutable mount capability retained by this
// serial session.
func (session Session) Mounted() witness.Mounted {
	if !session.Available() {
		return witness.Mounted{}
	}
	return session.mounted
}

// Root returns the exact committed database version redeemed by this session.
func (session Session) Root() database.Version {
	if !session.Available() {
		return database.Version{}
	}
	return session.root
}
