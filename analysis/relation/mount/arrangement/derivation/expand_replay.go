package derivation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ExpandWatcher is one Input occurrence below an Expand-left child.  The
// watcher is mount evidence: PathOccurrence selects the authored occurrence,
// StopFrame selects the exact Expand boundary, and Leaf/Range carry the two
// physical accesses issued for that Input.  A later evaluator may therefore
// replay the child at one fixed epoch without reopening the algebra or
// rediscovering a relation range.
type ExpandWatcher struct {
	path     uint32
	stop     uint32
	boundary identity.ContentID
	leaf     sealedAccess
	range_   sealedAccess
}

func newExpandWatcher(path, stop uint32, boundary identity.ContentID, leaf, range_ sealedAccess) (ExpandWatcher, bool) {
	value := ExpandWatcher{path: path, stop: stop, boundary: boundary, leaf: leaf, range_: range_}
	return value, value.Available()
}

func (value ExpandWatcher) Available() bool {
	return value.boundary.Available() && value.leaf.available() && value.range_.available() &&
		value.leaf.access.key == (model.KeyID{}) && value.range_.access.key == (model.KeyID{}) &&
		value.range_.access.relation == value.leaf.access.relation && len(value.range_.access.columns) == 0
}

// PathOccurrence is the stable authored Input occurrence to replay.
func (value ExpandWatcher) PathOccurrence() uint32 {
	if !value.Available() {
		return 0
	}
	return value.path
}

// StopFrame is the frame ordinal of the Expand boundary in PathOccurrence.
// Frames after this ordinal belong to the Expand-left child and are replayed
// at the fixed successor epoch; frames before it are ascended once after the
// dependent join is evaluated.
func (value ExpandWatcher) StopFrame() uint32 {
	if !value.Available() {
		return 0
	}
	return value.stop
}

// StopFrameDigest is the logical identity of the sealed Expand boundary.
func (value ExpandWatcher) StopFrameDigest() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.boundary
}

// Leaf is the exact mounted authored row-vector access for this Input.
func (value ExpandWatcher) Leaf() SiblingAccess {
	if !value.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: value.leaf}
}

// Range is the exact mounted relation-cofiber access for this Input.
func (value ExpandWatcher) Range() SiblingAccess {
	if !value.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: value.range_}
}

func (value ExpandWatcher) digest() (identity.ContentID, bool) {
	if !value.Available() {
		return identity.ContentID{}, false
	}
	parts := [][]byte{contentBytes(value.boundary), contentBytes(value.leaf.physical), contentBytes(value.range_.physical)}
	appendUint32(&parts, value.path)
	appendUint32(&parts, value.stop)
	parts = append(parts, accessDigest(value.leaf.access), accessDigest(value.range_.access))
	return identity.DeriveContentID(pathDigestDomain+"/expand-watcher", parts...)
}

// ExpandAnchor is the unique order-driving C occurrence for one Expand-left
// child.  It is deliberately separate from a generic watcher: the anchor is
// the only occurrence whose candidate RowID postings seed the keyed
// successor recompute, while all other watchers supply fixed-epoch child
// state.
type ExpandAnchor struct {
	path   uint32
	leaf   sealedAccess
	range_ sealedAccess
}

func newExpandAnchor(path uint32, leaf, range_ sealedAccess) (ExpandAnchor, bool) {
	value := ExpandAnchor{path: path, leaf: leaf, range_: range_}
	return value, value.Available()
}

func (value ExpandAnchor) Available() bool {
	return value.leaf.available() && value.range_.available() &&
		value.leaf.access.key == (model.KeyID{}) && value.range_.access.key == (model.KeyID{}) &&
		value.range_.access.relation == value.leaf.access.relation && len(value.range_.access.columns) == 0
}

// PathOccurrence identifies the sole C anchor occurrence.
func (value ExpandAnchor) PathOccurrence() uint32 {
	if !value.Available() {
		return 0
	}
	return value.path
}

// Access is the exact mounted C row-vector access used as the replay anchor.
func (value ExpandAnchor) Access() SiblingAccess {
	if !value.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: value.leaf}
}

// Range is the exact mounted C relation-cofiber access used to authenticate
// the candidate successor range.
func (value ExpandAnchor) Range() SiblingAccess {
	if !value.Available() {
		return SiblingAccess{}
	}
	return SiblingAccess{value: value.range_}
}

func (value ExpandAnchor) digest() (identity.ContentID, bool) {
	if !value.Available() {
		return identity.ContentID{}, false
	}
	parts := [][]byte{contentBytes(value.leaf.physical), contentBytes(value.range_.physical), accessDigest(value.leaf.access), accessDigest(value.range_.access)}
	appendUint32(&parts, value.path)
	return identity.DeriveContentID(pathDigestDomain+"/expand-anchor", parts...)
}

// ExpandReplay is the complete mount-time replay program for one Expand
// boundary.  It contains no evaluator or algebra callback.  The fixed set of
// watcher paths and the unique C anchor are enough for the runtime to perform
// one keyed successor recompute at Next, preserving cofibers, scope, and
// authored encounter order without output deduplication or RowID exclusion.
type ExpandReplay struct {
	emit     uint32
	anchor   ExpandAnchor
	watchers []ExpandWatcher
	digest   identity.ContentID
	sealed   bool
}

func newExpandReplay(emit uint32, anchor ExpandAnchor, watchers []ExpandWatcher) (ExpandReplay, bool) {
	if !anchor.Available() || len(watchers) == 0 || emit != anchor.PathOccurrence() {
		return ExpandReplay{}, false
	}
	copyOf := append([]ExpandWatcher(nil), watchers...)
	anchorCount := 0
	minimum := ^uint32(0)
	for index, watcher := range copyOf {
		if !watcher.Available() {
			return ExpandReplay{}, false
		}
		for _, prior := range copyOf[:index] {
			if prior.PathOccurrence() == watcher.PathOccurrence() {
				return ExpandReplay{}, false
			}
		}
		if watcher.PathOccurrence() < minimum {
			minimum = watcher.PathOccurrence()
		}
		if watcher.PathOccurrence() == anchor.PathOccurrence() {
			anchorCount++
		}
	}
	if anchorCount != 1 || minimum != anchor.PathOccurrence() || emit != minimum {
		return ExpandReplay{}, false
	}
	value := ExpandReplay{emit: emit, anchor: anchor, watchers: copyOf, sealed: true}
	digest, ok := value.recomputeDigest()
	if !ok {
		return ExpandReplay{}, false
	}
	value.digest = digest
	return value, value.Available()
}

func (value ExpandReplay) Available() bool {
	if !value.sealed || !value.anchor.Available() || value.watchers == nil || len(value.watchers) == 0 || !value.digest.Available() || value.emit != value.anchor.PathOccurrence() {
		return false
	}
	anchorCount := 0
	minimum := ^uint32(0)
	anchorMatch := false
	for index, watcher := range value.watchers {
		if !watcher.Available() {
			return false
		}
		for _, prior := range value.watchers[:index] {
			if prior.PathOccurrence() == watcher.PathOccurrence() {
				return false
			}
		}
		if watcher.PathOccurrence() < minimum {
			minimum = watcher.PathOccurrence()
		}
		if watcher.PathOccurrence() == value.anchor.PathOccurrence() {
			anchorCount++
			anchorMatch = watcher.leaf.equal(value.anchor.leaf) && watcher.range_.equal(value.anchor.range_)
		}
	}
	if anchorCount != 1 || !anchorMatch || minimum != value.anchor.PathOccurrence() || value.emit != minimum {
		return false
	}
	return true
}

func (value ExpandReplay) recomputeDigest() (identity.ContentID, bool) {
	if !value.anchor.Available() || value.watchers == nil || len(value.watchers) == 0 {
		return identity.ContentID{}, false
	}
	parts := make([][]byte, 0, 2+len(value.watchers))
	appendUint32(&parts, value.emit)
	anchorDigest, ok := value.anchor.digest()
	if !ok {
		return identity.ContentID{}, false
	}
	parts = append(parts, contentBytes(anchorDigest))
	for _, watcher := range value.watchers {
		digest, digestOK := watcher.digest()
		if !digestOK {
			return identity.ContentID{}, false
		}
		parts = append(parts, contentBytes(digest))
	}
	return identity.DeriveContentID(pathDigestDomain+"/expand-replay", parts...)
}

// EmitOccurrence is the one canonical occurrence through which this replay
// is emitted. It is always the order-driving C anchor.
func (value ExpandReplay) EmitOccurrence() uint32 {
	if !value.Available() {
		return 0
	}
	return value.emit
}

func (value ExpandReplay) Anchor() ExpandAnchor {
	if !value.Available() {
		return ExpandAnchor{}
	}
	return value.anchor
}

func (value ExpandReplay) WatcherCount() int {
	if !value.Available() {
		return 0
	}
	return len(value.watchers)
}

func (value ExpandReplay) WatcherAt(index int) (ExpandWatcher, bool) {
	if !value.Available() || index < 0 || index >= len(value.watchers) {
		return ExpandWatcher{}, false
	}
	return value.watchers[index], true
}

func (value ExpandReplay) Digest() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.digest
}
