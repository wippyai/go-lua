// Package typeauthority owns the detached static-type directory assembled
// from sealed ProgramArtifact rows.  Programs and Links are construction
// inputs to the compiler, never data retained by this package's authority.
package typeauthority

import (
	"bytes"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/keyspace"
)

// StaticTypeRef is the exact detached ProgramArtifact node coordinate.  It
// carries no dense Term compatibility token: the node is the Program-issued
// static reference content identity used by every mounted substitution.
type StaticTypeRef struct {
	owner keyspace.ContentID
	node  keyspace.ContentID
}

func (ref StaticTypeRef) Valid() bool                { return ref.owner.Available() && ref.node.Available() }
func (ref StaticTypeRef) Owner() keyspace.ContentID  { return ref.owner }
func (ref StaticTypeRef) NodeID() keyspace.ContentID { return ref.node }

// Selector is a dense authority-local static-type coordinate. Zero is
// invalid. It is only a hot lookup token; it is not a source term.
type Selector uint32

type entry struct {
	ref StaticTypeRef
}

// Authority is immutable after SealArtifactRows. All semantic materialization
// is delegated to the detached ArtifactAuthority; no Program, Flow, Link, or
// authored Term is reachable from this object.
type Authority struct {
	linkID        keyspace.ContentID
	entries       []entry
	byRef         map[StaticTypeRef]Selector
	byReferenceID map[keyspace.ContentID]Selector
	artifact      *ArtifactAuthority
}

// SealArtifactRows constructs the selector directory directly from reusable
// ProgramArtifact rows. Rows are sorted by owner and row identity so the
// selector assignment is deterministic and independent of mount order.
func SealArtifactRows(linkID keyspace.ContentID, artifacts []*programartifact.Artifact) (*Authority, error) {
	if !linkID.Available() {
		return nil, errors.New("typeauthority: unavailable artifact link identity")
	}
	artifact, err := SealArtifacts(artifacts)
	if err != nil {
		return nil, err
	}
	type rowEntry struct{ owner, node keyspace.ContentID }
	rows := make([]rowEntry, 0)
	for _, item := range artifacts {
		if item == nil || !item.Available() {
			return nil, errors.New("typeauthority: unavailable artifact")
		}
		owner := item.CompileKey().ProgramID()
		for i := 0; i < item.StaticTypeNodeCount(); i++ {
			row, ok := item.StaticTypeNodeAt(i)
			if !ok || !row.Available() || row.Owner() != owner {
				return nil, errors.New("typeauthority: malformed artifact row")
			}
			rows = append(rows, rowEntry{owner: owner, node: row.ID()})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if cmp := bytes.Compare(rows[i].owner[:], rows[j].owner[:]); cmp != 0 {
			return cmp < 0
		}
		return bytes.Compare(rows[i].node[:], rows[j].node[:]) < 0
	})
	// An unannotated Program lawfully contributes no static type nodes. The
	// artifact set itself is still non-empty and owner-validated by
	// SealArtifacts; an empty selector directory is therefore a complete
	// authority, not an unavailable one.
	if uint64(len(rows)) >= uint64(^uint32(0)) {
		return nil, errors.New("typeauthority: invalid artifact selector range")
	}
	a := &Authority{
		linkID: linkID, artifact: artifact, entries: make([]entry, len(rows)),
		byRef:         make(map[StaticTypeRef]Selector, len(rows)),
		byReferenceID: make(map[keyspace.ContentID]Selector, len(rows)),
	}
	for i, item := range rows {
		ref := StaticTypeRef{owner: item.owner, node: item.node}
		a.entries[i] = entry{ref: ref}
		a.byRef[ref] = Selector(i + 1)
		a.byReferenceID[item.node] = Selector(i + 1)
	}
	return a, nil
}

func (a *Authority) LinkID() keyspace.ContentID {
	if a == nil {
		return keyspace.ContentID{}
	}
	return a.linkID
}

// ArtifactBacked is retained as a compatibility predicate for callers while
// the migration is completed. It is always true for a valid Authority.
func (a *Authority) ArtifactBacked() bool { return a != nil && a.artifact != nil }

// DetachConstructionAuthority is an explicit lifecycle fence. Construction
// state is never retained, so detachment is a validity check only.
func (a *Authority) DetachConstructionAuthority() bool {
	return a != nil && a.artifact != nil && a.linkID.Available()
}

func (a *Authority) Count() int {
	if a == nil {
		return 0
	}
	return len(a.entries)
}

func (a *Authority) At(index int) (Selector, bool) {
	if a == nil || index < 0 || index >= len(a.entries) {
		return 0, false
	}
	return Selector(index + 1), true
}

func (a *Authority) Ref(selector Selector) (StaticTypeRef, bool) {
	entry, ok := a.entry(selector)
	if !ok {
		return StaticTypeRef{}, false
	}
	return entry.ref, true
}

func (a *Authority) Lookup(ref StaticTypeRef) (Selector, bool) {
	if a == nil || !ref.Valid() {
		return 0, false
	}
	selector, ok := a.byRef[ref]
	if !ok {
		return 0, false
	}
	entry, ok := a.entry(selector)
	return selector, ok && entry.ref == ref && entry.ref.NodeID().Available()
}

// FindByReferenceID admits the term-free artifact reference issued by the
// ProgramArtifact compiler. No Program lookup or authored-term reconstruction
// is permitted here.
func (a *Authority) FindByReferenceID(id keyspace.ContentID) (StaticTypeRef, bool) {
	if a == nil || !id.Available() {
		return StaticTypeRef{}, false
	}
	selector, ok := a.byReferenceID[id]
	if !ok {
		return StaticTypeRef{}, false
	}
	entry, ok := a.entry(selector)
	return entry.ref, ok && entry.ref.Valid()
}

func (a *Authority) Resolve(ref StaticTypeRef) (typ.Type, bool) {
	selector, ok := a.Lookup(ref)
	if !ok {
		return nil, false
	}
	return a.Materialize(selector)
}

func (a *Authority) Materialize(selector Selector) (typ.Type, bool) {
	if a == nil || a.artifact == nil {
		return nil, false
	}
	entry, ok := a.entry(selector)
	if !ok || !entry.ref.NodeID().Available() {
		return nil, false
	}
	value, ok := a.artifact.Resolve(entry.ref.NodeID())
	if !ok || value == nil || typ.ValidateStaticGenericRecurrence(value) != nil {
		return nil, false
	}
	return value, true
}

func (a *Authority) entry(selector Selector) (entry, bool) {
	if a == nil || selector == 0 || uint64(selector) > uint64(len(a.entries)) {
		return entry{}, false
	}
	return a.entries[uint32(selector)-1], true
}
