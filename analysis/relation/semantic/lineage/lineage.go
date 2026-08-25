package lineage

import (
	"bytes"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

const (
	authorityIdentityTag = "analysis/relation/semantic/lineage/authority/v1"
	joinIdentityTag      = "analysis/relation/semantic/lineage/join/v1"
)

// Authority is the one proof-sidecar join authority for a mounted solve.
//
// Join is an ACI operation over immutable references.  It is intentionally
// not a value-lattice operation: lineage never supplies a semantic result,
// sees a scope mask, or participates in scheduling.  A false result is a
// hard refusal; callers must not substitute a local token or last writer.
type Authority interface {
	Fence() binding.Fence
	Owner() model.OwnerID
	Identity() identity.ContentID
	Validate(model.LineageRef) bool
	Join(model.LineageRef, model.LineageRef) (model.LineageRef, bool)
}

// Factory binds the immutable lineage namespace to one mounted runtime.  The
// factory itself is independent of the runtime fence; Bind creates a fresh
// solve-local arena while preserving the namespace and content identities.
type Factory interface {
	Bind(binding.Fence) (Authority, bool)
}

// NewFactory creates a lineage factory for one dedicated owner namespace.
// The owner must be reserved exclusively for lineage output; source/domain
// publishers must use different owners.  The owner is part of the output
// identity fence, while the runtime fence is deliberately not hashed into
// joined lineage references.
func NewFactory(owner model.OwnerID) (Factory, bool) {
	if !owner.Available() {
		return nil, false
	}
	ownerContent := owner.Content()
	authorityIdentity, ok := identity.DeriveContentID(authorityIdentityTag, ownerContent[:])
	if !ok {
		return nil, false
	}
	return factory{owner: owner, identity: authorityIdentity}, true
}

type factory struct {
	owner    model.OwnerID
	identity identity.ContentID
}

func (value factory) Bind(fence binding.Fence) (Authority, bool) {
	if !value.owner.Available() || !value.identity.Available() || !fence.Available() {
		return nil, false
	}
	return &authority{
		fence:    fence,
		owner:    value.owner,
		identity: value.identity,
		nodes:    make(map[identity.ContentID]node),
	}, true
}

// atom is the complete identity of one owner-issued proof.  Keeping both
// owner and content prevents equal content tokens from different authorities
// from collapsing during normalization.
type atom struct {
	owner   identity.ContentID
	content identity.ContentID
}

func newAtom(ref model.LineageRef) (atom, bool) {
	if !ref.Available() {
		return atom{}, false
	}
	owner := ref.Owner().Content()
	content := ref.Content()
	if !owner.Available() || !content.Available() {
		return atom{}, false
	}
	return atom{owner: owner, content: content}, true
}

// node is immutable once inserted.  Its atom slice is never returned to a
// caller; Join copies it before normalization, so the arena remains
// append-only and defensive against caller mutation.
type node struct {
	atoms []atom
}

type authority struct {
	fence    binding.Fence
	owner    model.OwnerID
	identity identity.ContentID

	mu    sync.RWMutex
	nodes map[identity.ContentID]node
}

func (value *authority) Fence() binding.Fence {
	if value == nil {
		return binding.Fence{}
	}
	return value.fence
}

func (value *authority) Owner() model.OwnerID {
	if value == nil {
		return model.OwnerID{}
	}
	return value.owner
}

func (value *authority) Identity() identity.ContentID {
	if value == nil {
		return identity.ContentID{}
	}
	return value.identity
}

// Validate accepts structurally valid foreign-owner refs as opaque atoms.
// Same-owner refs are capabilities into this authority's arena and therefore
// must resolve to one immutable normalized node.  This prevents a caller
// from fabricating an authority-owned ref or importing one from another
// mounted authority without an explicit import operation.
func (value *authority) Validate(ref model.LineageRef) bool {
	if value == nil || !value.fence.Available() || !value.owner.Available() || !value.identity.Available() {
		return false
	}
	_, ok := newAtom(ref)
	if !ok {
		return false
	}
	if ref.Owner() != value.owner {
		return true
	}

	value.mu.RLock()
	defer value.mu.RUnlock()
	stored, ok := value.nodes[ref.Content()]
	if !ok || !validNode(stored) {
		return false
	}
	derived, ok := deriveJoinContent(stored.atoms)
	return ok && derived == ref.Content()
}

// Join returns the canonical union of left and right.  A foreign-owner ref
// is an atomic witness.  A same-owner ref must already be known by this
// authority.  Newly derived refs are hash-consed in the append-only arena.
// The runtime fence is an admission property of the authority, not part of
// the joined content identity; callers must compare it with their base state
// before applying a transaction.
func (value *authority) Join(left, right model.LineageRef) (model.LineageRef, bool) {
	if value == nil || !value.fence.Available() || !value.owner.Available() || !value.identity.Available() {
		return model.LineageRef{}, false
	}

	value.mu.Lock()
	defer value.mu.Unlock()

	leftAtoms, ok := value.atomsLocked(left)
	if !ok {
		return model.LineageRef{}, false
	}
	rightAtoms, ok := value.atomsLocked(right)
	if !ok {
		return model.LineageRef{}, false
	}
	atoms := normalizeAtoms(leftAtoms, rightAtoms)
	if len(atoms) == 0 {
		return model.LineageRef{}, false
	}
	if len(atoms) == 1 {
		return issueAtom(atoms[0])
	}

	content, ok := deriveJoinContent(atoms)
	if !ok {
		return model.LineageRef{}, false
	}
	if stored, exists := value.nodes[content]; exists {
		if !validNode(stored) || !sameAtoms(stored.atoms, atoms) {
			// A digest collision or an internally inconsistent cache is a hard
			// refusal.  Never return whichever node was inserted first.
			return model.LineageRef{}, false
		}
		return model.IssueLineageRef(value.owner, content)
	}

	ref, ok := model.IssueLineageRef(value.owner, content)
	if !ok {
		return model.LineageRef{}, false
	}
	copyOf := append([]atom(nil), atoms...)
	if !validNode(node{atoms: copyOf}) {
		return model.LineageRef{}, false
	}
	value.nodes[content] = node{atoms: copyOf}
	return ref, true
}

// atomsLocked expands one reference while the authority write lock is held.
// The returned slice is private to the caller and is always copied or
// normalized before arena insertion.
func (value *authority) atomsLocked(ref model.LineageRef) ([]atom, bool) {
	if !ref.Available() {
		return nil, false
	}
	if ref.Owner() != value.owner {
		atomValue, ok := newAtom(ref)
		if !ok {
			return nil, false
		}
		return []atom{atomValue}, true
	}
	stored, ok := value.nodes[ref.Content()]
	if !ok || !validNode(stored) {
		return nil, false
	}
	derived, ok := deriveJoinContent(stored.atoms)
	if !ok || derived != ref.Content() {
		return nil, false
	}
	return append([]atom(nil), stored.atoms...), true
}

func issueAtom(value atom) (model.LineageRef, bool) {
	owner, ok := model.IssueOwnerID(value.owner)
	if !ok {
		return model.LineageRef{}, false
	}
	return model.IssueLineageRef(owner, value.content)
}

func deriveJoinContent(atoms []atom) (identity.ContentID, bool) {
	if len(atoms) == 0 {
		return identity.ContentID{}, false
	}
	parts := make([][]byte, 0, len(atoms)*2)
	for _, value := range atoms {
		if !value.owner.Available() || !value.content.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, value.owner[:], value.content[:])
	}
	return identity.DeriveContentID(joinIdentityTag, parts...)
}

func normalizeAtoms(left, right []atom) []atom {
	result := make([]atom, 0, len(left)+len(right))
	result = append(result, left...)
	result = append(result, right...)
	sort.Slice(result, func(left, right int) bool {
		return compareAtoms(result[left], result[right]) < 0
	})
	write := 0
	for _, value := range result {
		if write == 0 || result[write-1] != value {
			result[write] = value
			write++
		}
	}
	return result[:write]
}

func compareAtoms(left, right atom) int {
	if ownerOrder := bytes.Compare(left.owner[:], right.owner[:]); ownerOrder != 0 {
		return ownerOrder
	}
	return bytes.Compare(left.content[:], right.content[:])
}

func sameAtoms(left, right []atom) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validNode(value node) bool {
	if len(value.atoms) < 2 {
		return false
	}
	for index, atomValue := range value.atoms {
		if !atomValue.owner.Available() || !atomValue.content.Available() {
			return false
		}
		if index > 0 && compareAtoms(value.atoms[index-1], atomValue) >= 0 {
			return false
		}
	}
	return true
}
