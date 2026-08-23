// Package typeauthority owns the detached static-type directory assembled
// from canonical sealed Program rows. Programs and Links are construction
// inputs to the compiler, never data retained by this package's authority.
package typeauthority

import (
	"bytes"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// StaticTypeRef is the exact canonical Program node coordinate. It
// carries no dense Term compatibility token: the node is the Program-issued
// static reference content identity used by every mounted substitution.
type StaticTypeRef struct {
	owner identity.ContentID
	node  identity.ContentID
}

func (ref StaticTypeRef) Valid() bool                { return ref.owner.Available() && ref.node.Available() }
func (ref StaticTypeRef) Owner() identity.ContentID  { return ref.owner }
func (ref StaticTypeRef) NodeID() identity.ContentID { return ref.node }

// Selector is a dense authority-local static-type coordinate. Zero is
// invalid. It is only a hot lookup token; it is not a source term.
type Selector uint32

type entry struct {
	ref        StaticTypeRef
	projection ReferenceProjection
}

// Authority is immutable after SealProgramRows. All semantic materialization
// is delegated to the detached artifact authority; no Program, Flow, Link, or
// authored Term is reachable from this object.
type Authority struct {
	linkID        identity.ContentID
	entries       []entry
	byRef         map[StaticTypeRef]Selector
	byReferenceID map[identity.ContentID]Selector
	runtimeInputs []RuntimeInput
	artifact      *artifactAuthority
}

// SealProgramRows constructs the selector directory directly from canonical
// Program rows. Rows are sorted by owner and row identity so the
// selector assignment is deterministic and independent of mount order.
//
// qualified is the Link target's published type vocabulary, already read into
// this domain's types. A Program reference that names a qualified type by
// canonical path is resolved through it; a Link whose target publishes none
// carries an empty vocabulary and refuses every such reference by name.
func SealProgramRows(linkID identity.ContentID, programs []programschema.Program, qualified []QualifiedType) (*Authority, error) {
	if !linkID.Available() {
		return nil, errors.New("typeauthority: unavailable program link identity")
	}
	artifact, err := sealPrograms(programs, true, qualified)
	if err != nil {
		return nil, err
	}
	type rowEntry struct{ owner, node identity.ContentID }
	rows := make([]rowEntry, 0)
	for _, view := range artifact.views {
		count, countOK := view.StaticTypeNodeCount()
		if !countOK {
			return nil, errors.New("typeauthority: unavailable program static graph")
		}
		for i := 0; i < count; i++ {
			row, ok := view.StaticTypeNodeAt(i)
			if !ok || !row.Available() {
				return nil, errors.New("typeauthority: malformed program row")
			}
			rows = append(rows, rowEntry{owner: row.Owner(), node: row.ID()})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if cmp := bytes.Compare(rows[i].owner[:], rows[j].owner[:]); cmp != 0 {
			return cmp < 0
		}
		return bytes.Compare(rows[i].node[:], rows[j].node[:]) < 0
	})
	// An unannotated Program lawfully contributes no static type nodes. The
	// program set itself is still non-empty and owner-validated by
	// SealPrograms; an empty selector directory is therefore a complete
	// authority, not an unavailable one.
	if uint64(len(rows)) >= uint64(^uint32(0)) {
		return nil, errors.New("typeauthority: invalid program selector range")
	}
	a := &Authority{
		linkID: linkID, artifact: artifact, entries: make([]entry, len(rows)),
		byRef:         make(map[StaticTypeRef]Selector, len(rows)),
		byReferenceID: make(map[identity.ContentID]Selector, len(rows)),
		runtimeInputs: make([]RuntimeInput, len(rows)),
	}
	for i, item := range rows {
		ref := StaticTypeRef{owner: item.owner, node: item.node}
		projection, runtimeInput, projectionOK := bindReferenceProjection(a, ref, artifact)
		if !projectionOK {
			return nil, errors.New("typeauthority: static reference projection unavailable")
		}
		a.entries[i] = entry{ref: ref, projection: projection}
		if !projection.open {
			a.runtimeInputs[i] = runtimeInput
		}
		a.byRef[ref] = Selector(i + 1)
		a.byReferenceID[item.node] = Selector(i + 1)
	}
	artifact.releaseProjectionGraphs()
	return a, nil
}

func (a *Authority) runtimeInput(ref StaticTypeRef, semantic identity.ContentID) (RuntimeInput, bool) {
	if a == nil || !ref.Valid() || !semantic.Available() {
		return RuntimeInput{}, false
	}
	selector, ok := a.byRef[ref]
	if !ok || selector == 0 || uint64(selector) > uint64(len(a.runtimeInputs)) {
		return RuntimeInput{}, false
	}
	input := a.runtimeInputs[uint32(selector)-1]
	if input.authority != a {
		return RuntimeInput{}, false
	}
	inputIdentity, identityOK := input.CanonicalIdentity()
	return input, identityOK && inputIdentity == semantic
}

func (a *Authority) releaseRuntimeInputs() {
	if a != nil {
		a.runtimeInputs = nil
	}
}

func (a *Authority) LinkID() identity.ContentID {
	if a == nil {
		return identity.ContentID{}
	}
	return a.linkID
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
func (a *Authority) FindByReferenceID(id identity.ContentID) (StaticTypeRef, bool) {
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

func (a *Authority) entry(selector Selector) (entry, bool) {
	if a == nil || selector == 0 || uint64(selector) > uint64(len(a.entries)) {
		return entry{}, false
	}
	return a.entries[uint32(selector)-1], true
}
