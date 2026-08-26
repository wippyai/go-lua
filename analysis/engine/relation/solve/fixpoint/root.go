package fixpoint

import "github.com/wippyai/go-lua/analysis/engine/relation/state/database"

// RootMode identifies the only two inputs a solve work item may carry.
//
// FullRoot is the initial evaluation of a committed database root. LaterRoot
// is a database publication delta and is evaluated semi-naively. Keeping the
// distinction in the value, rather than inferring it from a revision number,
// prevents a later root from accidentally becoming a full rescan.
type RootMode uint8

const (
	RootInvalid RootMode = iota
	FullRoot
	LaterRoot
)

// Root is the immutable state boundary carried by one work item. It is
// either a complete committed root or one authenticated successor delta; it
// is never a mutable store and never a second row representation.
type Root struct {
	mode  RootMode
	full  database.Version
	delta database.Delta
}

// Full constructs the initial full-root input.
func Full(version database.Version) (Root, bool) {
	if !version.Available() {
		return Root{}, false
	}
	return Root{mode: FullRoot, full: version}, true
}

// Later constructs a semi-naive successor input from one authenticated
// database delta.
func Later(delta database.Delta) (Root, bool) {
	if !delta.Available() {
		return Root{}, false
	}
	return Root{mode: LaterRoot, delta: delta}, true
}

// Available reports whether the root carries exactly one valid input form.
func (root Root) Available() bool {
	switch root.mode {
	case FullRoot:
		return root.full.Available() && !root.delta.Available()
	case LaterRoot:
		return root.delta.Available() && !root.full.Available()
	default:
		return false
	}
}

// Mode returns the explicit full/later distinction.
func (root Root) Mode() RootMode {
	if !root.Available() {
		return RootInvalid
	}
	return root.mode
}

// FullVersion returns the complete root, only for FullRoot.
func (root Root) FullVersion() (database.Version, bool) {
	if !root.Available() || root.mode != FullRoot {
		return database.Version{}, false
	}
	return root.full, true
}

// Delta returns the authenticated successor, only for LaterRoot.
func (root Root) Delta() (database.Delta, bool) {
	if !root.Available() || root.mode != LaterRoot {
		return database.Delta{}, false
	}
	return root.delta, true
}

// Revision returns the committed successor revision represented by root.
func (root Root) Revision() uint64 {
	if !root.Available() {
		return 0
	}
	if root.mode == FullRoot {
		return root.full.Revision()
	}
	return root.delta.Next().Revision()
}

// BaseRevision returns the predecessor revision. A full root has no
// predecessor and returns zero.
func (root Root) BaseRevision() uint64 {
	if !root.Available() || root.mode == FullRoot {
		return 0
	}
	return root.delta.Base().Revision()
}

// Same reports exact immutable root identity. Revision numbers are not
// identity: two roots can share a revision while belonging to different
// mounted histories.
func (root Root) Same(other Root) bool {
	if !root.Available() || !other.Available() || root.mode != other.mode {
		return false
	}
	switch root.mode {
	case FullRoot:
		return root.full.Same(other.full)
	case LaterRoot:
		return root.delta.Base().Same(other.delta.Base()) && root.delta.Next().Same(other.delta.Next())
	default:
		return false
	}
}
