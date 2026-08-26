package contribution

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/invocation"
)

// Handle is the store/generation-local opaque identity issued by this state
// directory. Contribution rows consume it; they cannot derive one from an
// address, an ordinal, or a durable ContentID.
//
// The local slot is private. It only supplies deterministic ordering inside
// one directory authority and is never exposed as an invocation identity.
type Handle struct {
	authority *handleAuthority
	slot      uint64
}

type handleAuthority struct {
	fence binding.Fence
}

// Available reports whether handle was issued by a live directory authority.
func (handle Handle) Available() bool {
	return handle.authority != nil && handle.slot != 0 && handle.authority.fence.Available()
}

// ValidFor reports whether handle belongs to one exact runtime fence.
func (handle Handle) ValidFor(fence binding.Fence) bool {
	return handle.Available() && fence.Available() && handle.authority.fence.Same(fence)
}

// Same compares exact opaque local identity. Handles from distinct directory
// authorities never compare equal even when their private slots coincide.
func (handle Handle) Same(other Handle) bool {
	return handle.Available() && other.Available() && handle.authority == other.authority && handle.slot == other.slot
}

// SameDirectory reports whether two handles share one immutable directory
// authority. It is a namespace check, not an invocation identity comparison.
func (handle Handle) SameDirectory(other Handle) bool {
	return handle.Available() && other.Available() && handle.authority == other.authority
}

// CompareHandles orders handles only inside one directory authority. Foreign
// or unavailable handles are refused rather than ordered by pointer or by a
// cross-store fallback.
func CompareHandles(left, right Handle) (int, bool) {
	if !left.Available() || !right.Available() || left.authority != right.authority {
		return 0, false
	}
	if left.slot < right.slot {
		return -1, true
	}
	if left.slot > right.slot {
		return 1, true
	}
	return 0, true
}

// Directory interns exact structural addresses into immutable successors. It
// is the sole owner allowed to issue contribution Handle values.
type Directory struct {
	state *directoryState
}

type directoryState struct {
	parent    *directoryState
	authority *handleAuthority
	entries   []directoryEntry
	sealed    bool
}

type directoryEntry struct {
	address invocation.InvocationAddress
	handle  Handle
}

// NewDirectory creates an empty directory for one exact runtime fence. It
// does not issue an invocation handle.
func NewDirectory(fence binding.Fence) (Directory, bool) {
	if !fence.Available() {
		return Directory{}, false
	}
	result := Directory{state: &directoryState{authority: &handleAuthority{fence: fence}, entries: make([]directoryEntry, 0), sealed: true}}
	return result, result.Available()
}

// Available reports whether the directory retains one complete immutable
// authority and sealed entry vector.
func (directory Directory) Available() bool {
	return directory.state != nil && directory.state.sealed && directory.state.authority != nil && directory.state.authority.fence.Available() && directory.state.entries != nil
}

// Same reports exact immutable directory-root identity.
func (directory Directory) Same(other Directory) bool {
	return directory.Available() && other.Available() && directory.state == other.state
}

// SuccessorOf proves one direct immutable directory successor.
func (directory Directory) SuccessorOf(base Directory) bool {
	return directory.Available() && base.Available() && !directory.Same(base) && directory.state.parent == base.state && directory.state.authority == base.state.authority
}

// Fence returns the exact runtime authority captured by the directory.
func (directory Directory) Fence() binding.Fence {
	if !directory.Available() {
		return binding.Fence{}
	}
	return directory.state.authority.fence
}

// Len reports the number of interned structural addresses.
func (directory Directory) Len() int {
	if !directory.Available() {
		return 0
	}
	return len(directory.state.entries)
}

// Intern adopts an already-authenticated structural address and returns an
// immutable successor plus its opaque local handle. Repeated addresses reuse
// both the existing root and handle; no digest or semantic identity is made.
func (directory Directory) Intern(address invocation.InvocationAddress) (Directory, Handle, bool) {
	if !directory.Available() || !address.ValidFor(directory.Fence()) {
		return Directory{}, Handle{}, false
	}
	for _, candidate := range directory.state.entries {
		if candidate.address.Same(address) {
			return directory, candidate.handle, true
		}
	}
	if uint64(len(directory.state.entries)) == ^uint64(0) {
		return Directory{}, Handle{}, false
	}
	slot := uint64(len(directory.state.entries) + 1)
	if slot == 0 {
		return Directory{}, Handle{}, false
	}
	handle := Handle{authority: directory.state.authority, slot: slot}
	entries := make([]directoryEntry, len(directory.state.entries), len(directory.state.entries)+1)
	copy(entries, directory.state.entries)
	entries = append(entries, directoryEntry{address: address, handle: handle})
	next := Directory{state: &directoryState{parent: directory.state, authority: directory.state.authority, entries: entries, sealed: true}}
	if !next.Available() {
		return Directory{}, Handle{}, false
	}
	return next, handle, true
}

// Resolve returns the exact structural address retained for one handle. A
// predecessor directory cannot resolve a handle introduced by its successor.
func (directory Directory) Resolve(handle Handle) (invocation.InvocationAddress, bool) {
	if !directory.Available() || !handle.ValidFor(directory.Fence()) || handle.authority != directory.state.authority {
		return invocation.InvocationAddress{}, false
	}
	for _, candidate := range directory.state.entries {
		if candidate.handle.Same(handle) {
			return candidate.address, true
		}
	}
	return invocation.InvocationAddress{}, false
}

// Contains reports whether this immutable directory root retains handle.
func (directory Directory) Contains(handle Handle) bool {
	_, ok := directory.Resolve(handle)
	return ok
}

// internBatch interns all addresses against one base directory and publishes
// at most one direct successor.  Handles are assigned only by this directory
// owner; no digest, ordinal, or intermediate directory is returned.
func (directory Directory) internBatch(addresses []invocation.InvocationAddress) (Directory, []Handle, bool) {
	if !directory.Available() || len(addresses) == 0 {
		return Directory{}, nil, false
	}
	entries := append([]directoryEntry(nil), directory.state.entries...)
	handles := make([]Handle, len(addresses))
	changed := false
	for index, address := range addresses {
		if !address.ValidFor(directory.Fence()) {
			return Directory{}, nil, false
		}
		found := false
		for _, candidate := range entries {
			if candidate.address.Same(address) {
				handles[index] = candidate.handle
				found = true
				break
			}
		}
		if found {
			continue
		}
		if uint64(len(entries)) == ^uint64(0) {
			return Directory{}, nil, false
		}
		slot := uint64(len(entries) + 1)
		if slot == 0 {
			return Directory{}, nil, false
		}
		handle := Handle{authority: directory.state.authority, slot: slot}
		entries = append(entries, directoryEntry{address: address, handle: handle})
		handles[index] = handle
		changed = true
	}
	if !changed {
		return directory, handles, true
	}
	next := Directory{state: &directoryState{
		parent:    directory.state,
		authority: directory.state.authority,
		entries:   entries,
		sealed:    true,
	}}
	if !next.Available() {
		return Directory{}, nil, false
	}
	return next, handles, true
}
