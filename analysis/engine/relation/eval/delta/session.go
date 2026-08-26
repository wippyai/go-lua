package delta

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// Session is the solve-local capability for one authenticated Later root.
// It retains no logical expression and no mutable state.  The successor root
// is the read base for full siblings and the publication predecessor for
// applications produced by this differential evaluation.
type Session struct {
	mounted   witness.Mounted
	root      fixpoint.Root
	delta     database.Delta
	geometry  geometry.Geometry
	door      publish.Door
	scratch   *store.ReadScratch
	execution arrangement.Execution
	sealed    bool
}

// New binds one Later root to the exact mounted execution and solve-local
// geometry.  Full roots are intentionally refused: full evaluation belongs
// to eval/step and must not silently enter the differential path.
func New(mounted witness.Mounted, root fixpoint.Root, view geometry.Geometry) (Session, bool) {
	if !mounted.Available() || !root.Available() || root.Mode() != fixpoint.LaterRoot || !view.ValidFor(mounted) {
		return Session{}, false
	}
	deltaValue, ok := root.Delta()
	if !ok || !deltaValue.Available() {
		return Session{}, false
	}
	next := deltaValue.Next()
	if !ownsVersion(mounted, next) || !ownsVersion(mounted, deltaValue.Base()) || !next.SuccessorOf(deltaValue.Base()) {
		return Session{}, false
	}
	plan := mounted.Arrangement()
	if !plan.Available() || next.ArrangementDigest() != plan.Digest() || next.MountedDigest() != mounted.Digest() || !next.Fence().Same(mounted.RuntimeFence()) {
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
	execution := plan.Execution()
	if !execution.Available() || !execution.Fence().Same(mounted.Fence()) {
		return Session{}, false
	}
	result := Session{mounted: mounted, root: root, delta: deltaValue, geometry: view, door: door, scratch: scratch, execution: execution, sealed: true}
	return result, result.Available()
}

func ownsVersion(mounted witness.Mounted, version database.Version) bool {
	if !mounted.Available() || !version.Available() || !version.Mounted().Same(mounted) || !version.Fence().Same(mounted.RuntimeFence()) {
		return false
	}
	plan := mounted.Arrangement()
	execution := version.Arrangement().Execution()
	return plan.Available() && version.MountedDigest() == mounted.Digest() && version.ArrangementDigest() == plan.Digest() && execution.Available() && execution.Digest() == plan.Execution().Digest() && execution.Fence().Same(mounted.Fence())
}

// Available is the constant-time session admission fence established by New.
func (session Session) Available() bool {
	if !session.sealed || !session.mounted.Available() || !session.root.Available() || session.root.Mode() != fixpoint.LaterRoot || !session.delta.Available() || !session.geometry.ValidFor(session.mounted) || !session.door.Available() || session.scratch == nil || !session.scratch.Available() || !session.execution.Available() {
		return false
	}
	base, next := session.delta.Base(), session.delta.Next()
	rootDelta, ok := session.root.Delta()
	return base.Available() && next.Available() && next.SuccessorOf(base) && ownsVersion(session.mounted, base) && ownsVersion(session.mounted, next) && ok && rootDelta.Base().Same(base) && rootDelta.Next().Same(next)
}

// Mounted returns the exact mount capability retained by this session.
func (session Session) Mounted() witness.Mounted {
	if !session.Available() {
		return witness.Mounted{}
	}
	return session.mounted
}

// Root returns the exact Later input redeemed by this session.
func (session Session) Root() fixpoint.Root {
	if !session.Available() {
		return fixpoint.Root{}
	}
	return session.root
}

// Delta returns the exact input transition redeemed by this session.
func (session Session) Delta() database.Delta {
	if !session.Available() {
		return database.Delta{}
	}
	return session.delta
}

// Successor returns the committed successor observed by this Later session.
// It is the read base for stable Next-side siblings and the publication
// predecessor for applications produced by this evaluation.
func (session Session) Successor() database.Version {
	if !session.Available() {
		return database.Version{}
	}
	return session.delta.Next()
}

// Execution returns the exact arrangement execution redeemed by Session.
func (session Session) Execution() arrangement.Execution {
	if !session.Available() {
		return arrangement.Execution{}
	}
	return session.execution
}
