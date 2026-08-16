package pack

import (
	"bytes"
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/identity"
)

// algebra is the immutable class fence for one Pack schema. It owns no type
// language and no Class vocabulary: every class remains an opaque value issued
// by Static's one ClassSet. Sealed entries provide stable hot ordinals, while
// a ClassSet-derived union remains valid by the same owner fence without
// extending Pack's entry table.
//
// A separate algebra exists per Pack schema because a recurrence may only use
// the endpoint classes its own sealed relation can observe. It is not an
// application cache, a type resolver, or a second static authority.
type algebra struct {
	classes *static.ClassSet
	id      identity.ContentID
	entries []classEntry
	index   map[identity.ContentID]uint32 // cold artifact identity, one-based
	ordinal map[identity.ContentID]uint32 // hot stable-class handle, one-based
	offsets []nat
}

type classEntry struct {
	class static.Class
	id    identity.ContentID
}

// newAlgebraWithOffsets is schema construction only. Callers must enumerate
// every statically required class and Head/Tail offset before the algebra is
// frozen; there is intentionally no post-seal interning path.
func newAlgebraWithOffsets(classes *static.ClassSet, admitted []static.Class, offsets []nat) (*algebra, bool) {
	if classes == nil || !classes.ContentID().Available() {
		return nil, false
	}
	frozenOffsets, ok := freezeOffsets(offsets)
	if !ok {
		return nil, false
	}
	owner := &algebra{
		classes: classes,
		index:   make(map[identity.ContentID]uint32),
		ordinal: make(map[identity.ContentID]uint32),
		offsets: frozenOffsets,
	}
	if !owner.add(classes.AnyValue()) {
		return nil, false
	}
	if nilClass := classes.Nil(); !owner.add(nilClass) {
		return nil, false
	}
	for _, class := range admitted {
		if !owner.add(class) {
			return nil, false
		}
	}
	sort.Slice(owner.entries, func(left, right int) bool {
		return bytes.Compare(owner.entries[left].id[:], owner.entries[right].id[:]) < 0
	})
	owner.index = make(map[identity.ContentID]uint32, len(owner.entries))
	owner.ordinal = make(map[identity.ContentID]uint32, len(owner.entries))
	for index, entry := range owner.entries {
		if _, duplicate := owner.index[entry.id]; duplicate {
			return nil, false
		}
		ordinal := uint32(index + 1)
		owner.index[entry.id] = ordinal
		owner.ordinal[entry.id] = ordinal
	}
	owner.id = algebraID(classes.ContentID(), owner.entries, owner.offsets)
	return owner, owner.id.Available()
}

func (owner *algebra) add(class static.Class) bool {
	if owner == nil || owner.classes == nil {
		return false
	}
	id, ok := owner.classes.Identity(class)
	if !ok {
		return false
	}
	if _, exists := owner.index[id]; exists {
		return true
	}
	owner.entries = append(owner.entries, classEntry{class: class, id: id})
	ordinal := uint32(len(owner.entries))
	owner.index[id] = ordinal
	owner.ordinal[id] = ordinal
	return true
}

func (owner *algebra) valid() bool {
	return owner != nil && owner.classes != nil && owner.id.Available() && len(owner.entries) > 0 && len(owner.offsets) > 0
}

func (owner *algebra) admits(class static.Class) bool {
	return owner != nil && owner.valid() && owner.classes.Owns(class)
}

func (owner *algebra) containsClass(class static.Class) bool {
	if owner == nil || owner.classes == nil {
		return false
	}
	return owner.classes.Owns(class)
}

func (owner *algebra) equalClass(left, right static.Class) bool {
	return owner != nil && owner.admits(left) && owner.admits(right) && owner.classes.Equal(left, right)
}

func (owner *algebra) lessClass(left, right static.Class) bool {
	return owner != nil && owner.admits(left) && owner.admits(right) && owner.classes.LessOrEq(left, right)
}

func (owner *algebra) joinClass(left, right static.Class) (static.Class, bool) {
	if owner == nil || !owner.admits(left) || !owner.admits(right) {
		return static.Class{}, false
	}
	joined := owner.classes.Join(left, right)
	return joined, owner.admits(joined)
}

func (owner *algebra) classRank(class static.Class) uint64 {
	if owner == nil {
		return 0
	}
	return owner.classes.Rank(class)
}

// classIdentity is the Static-owned identity of a class.  It deliberately
// does not fall back to Fingerprint: a derived class is not necessarily in
// Pack's sealed entry table, and truncating its fingerprint into an ordinal
// would turn distinct derived descriptors into equal Pack values.
func (owner *algebra) classIdentity(class static.Class) (identity.ContentID, bool) {
	if owner == nil || !owner.admits(class) {
		return identity.ContentID{}, false
	}
	return owner.classes.Identity(class)
}

func algebraID(classes identity.ContentID, entries []classEntry, offsets []nat) (id identity.ContentID) {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.pack/class-fence\x00\x01"))
	_, _ = h.Write(classes[:])
	for _, entry := range entries {
		_, _ = h.Write(entry.id[:])
	}
	for _, offset := range offsets {
		var count [8]byte
		width := uint64(len(offset.limbs))
		for index := len(count) - 1; index >= 0; index-- {
			count[index] = byte(width)
			width >>= 8
		}
		_, _ = h.Write(count[:])
		for _, limb := range offset.limbs {
			_, _ = h.Write([]byte{byte(limb >> 24), byte(limb >> 16), byte(limb >> 8), byte(limb)})
		}
	}
	copy(id[:], h.Sum(nil))
	return id
}
