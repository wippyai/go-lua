// Package typeauthority owns the sealed projection from Program static type
// terms to existing typ semantics. It is deliberately neither an AST reader
// nor a second type language: every selector resolves back to one exact
// Program ContentID and one already-sealed Program Term.
package typeauthority

import (
	"bytes"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

// StaticTypeRef is the portable identity issued by this Link-local authority
// for one authored Static type term. Static.StaticTypeRef is intentionally an
// owner-bound hot capability and carries only its local Term; the authority
// therefore owns the cross-owner pair used by Lookup, persistence, and
// runtime bindings.
type StaticTypeRef struct {
	owner keyspace.ContentID
	root  keyspace.Term
}

func (ref StaticTypeRef) Valid() bool               { return ref.owner.Available() && ref.root != 0 }
func (ref StaticTypeRef) Owner() keyspace.ContentID { return ref.owner }
func (ref StaticTypeRef) Root() keyspace.Term       { return ref.root }

// Selector is a Link-authority-local dense static type coordinate. Zero is
// invalid. It is deliberately small enough for recurrent fact hot paths; its
// portable identity is recovered only at the artifact boundary through Ref.
type Selector uint32

// FamilyRef identifies a direct sealed static-union family. Its members are
// existing Program type roots; no process-global discovery catalog or copied
// typ union determines family identity.
type FamilyRef struct{ root Selector }

func (f FamilyRef) Root() Selector { return f.root }
func (f FamilyRef) Valid() bool    { return f.root != 0 }

type entry struct {
	ref     StaticTypeRef
	program *program.Program
}

type materializationState uint8

const (
	materializationCold materializationState = iota
	materializationWorking
	materializationReady
)

// Authority is immutable after Seal. Its only mutable data is the cold,
// memoized existing typ.Type projection; selectors, their portable references,
// and family arities are entirely sealed Program facts.
type Authority struct {
	source   *link.Link
	linkID   keyspace.ContentID
	entries  []entry // Selector is one-based into entries.
	byRef    map[StaticTypeRef]Selector
	programs map[keyspace.ContentID]*program.Program

	mu        sync.Mutex
	states    []materializationState
	types     []typ.Type
	params    []*typ.TypeParam
	recursive []*typ.Recursive

	familyMu sync.Mutex
	families map[Selector]familyResult
}

// Seal constructs one canonical selector universe for Link's complete finite
// Program set. Link already owns the source-module authority; this pass merely
// gives every existing static type root a dense hot-path coordinate. No AST is
// read and no new structural type form is invented.
func Seal(source *link.Link) (*Authority, bool) {
	if source == nil || !source.ContentID().Available() {
		return nil, false
	}
	mounts := source.Project().Mounts()
	byProgram := make(map[keyspace.ContentID]*program.Program, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			return nil, false
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil || !p.ContentID().Available() {
			return nil, false
		}
		id := p.ContentID()
		if prior, duplicate := byProgram[id]; duplicate && prior != p {
			// Equal Program content may be decoded into distinct pointers. One
			// canonical pointer is enough because all queries are immutable.
			continue
		}
		byProgram[id] = p
	}
	ids := make([]keyspace.ContentID, 0, len(byProgram))
	for id := range byProgram {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })

	entries := make([]entry, 0)
	for _, id := range ids {
		p := byProgram[id]
		staticTypes := p.Static().StaticTypes()
		for index := 0; index < staticTypes.Count(); index++ {
			hot, ok := staticTypes.At(index)
			root := hot.Term()
			if !ok || root == 0 {
				return nil, false
			}
			entries = append(entries, entry{ref: StaticTypeRef{owner: id, root: root}, program: p})
		}
	}
	if uint64(len(entries)) >= uint64(^uint32(0)) {
		return nil, false
	}
	authority := &Authority{
		source:    source,
		linkID:    source.ContentID(),
		entries:   entries,
		byRef:     make(map[StaticTypeRef]Selector, len(entries)),
		programs:  byProgram,
		states:    make([]materializationState, len(entries)),
		types:     make([]typ.Type, len(entries)),
		params:    make([]*typ.TypeParam, len(entries)),
		recursive: make([]*typ.Recursive, len(entries)),
	}
	for index, current := range authority.entries {
		selector := Selector(index + 1)
		if _, duplicate := authority.byRef[current.ref]; duplicate {
			return nil, false
		}
		authority.byRef[current.ref] = selector
	}
	return authority, true
}

// LinkID identifies the exact Link whose dense selector coordinates this
// Authority owns. It is for cache fencing only; portable selector identity is
// always Ref's Program ContentID plus Program Term.
func (a *Authority) LinkID() keyspace.ContentID {
	if a == nil {
		return keyspace.ContentID{}
	}
	return a.linkID
}

// Link returns the exact sealed Link owner. LinkID is only a replay/cache
// identity: independently sealed same-content Links retain distinct authority
// capabilities and must not cross a live semantic boundary.
func (a *Authority) Link() *link.Link {
	if a == nil {
		return nil
	}
	return a.source
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

// Ref returns Selector's portable Program-owned identity.
func (a *Authority) Ref(selector Selector) (StaticTypeRef, bool) {
	entry, ok := a.entry(selector)
	if !ok {
		return StaticTypeRef{}, false
	}
	return entry.ref, true
}

// Lookup admits only a portable reference present in this exact sealed Link.
func (a *Authority) Lookup(ref StaticTypeRef) (Selector, bool) {
	if a == nil || !ref.Valid() {
		return 0, false
	}
	selector, ok := a.byRef[ref]
	if !ok {
		return 0, false
	}
	entry, ok := a.entry(selector)
	if !ok || entry.program == nil || entry.program.ContentID() != ref.Owner() {
		return 0, false
	}
	hot, ok := entry.program.Static().StaticTypes().Ref(ref.Root())
	return selector, ok && hot.Term() == ref.Root()
}

// Find resolves an artifact-origin Program coordinate by re-minting the
// canonical typed reference through this Authority's sealed Program owner.
// It is the sole raw artifact boundary; all semantic consumers retain the
// resulting StaticTypeRef or Selector.
func (a *Authority) Find(owner keyspace.ContentID, root keyspace.Term) (Selector, bool) {
	if a == nil || !owner.Available() || root == 0 {
		return 0, false
	}
	p := a.programs[owner]
	if p == nil || p.ContentID() != owner {
		return 0, false
	}
	if _, ok := p.Static().StaticTypes().Ref(root); !ok {
		return 0, false
	}
	return a.Lookup(StaticTypeRef{owner: owner, root: root})
}

// Resolve projects one portable authored reference through this exact sealed
// authority.  It is a cold boundary: the returned graph is ownership-isolated
// and is never the authority's memoized construction graph.
func (a *Authority) Resolve(ref StaticTypeRef) (typ.Type, bool) {
	selector, ok := a.Lookup(ref)
	if !ok {
		return nil, false
	}
	return a.Materialize(selector)
}

// Family exposes a direct Program union as a finite family. The returned
// family has no separate ID: its root selector is the exact static union root.
func (a *Authority) Family(root Selector) (FamilyRef, bool) {
	if _, ok := a.familyArms(root); !ok {
		return FamilyRef{}, false
	}
	return FamilyRef{root: root}, true
}

func (a *Authority) FamilyArity(family FamilyRef) int {
	arms, ok := a.familyArms(family.root)
	if !ok {
		return 0
	}
	return len(arms)
}

// FamilyArm returns the exact existing static member selector. It never
// materializes a typ union or consults a global variant catalog.
func (a *Authority) FamilyArm(family FamilyRef, index int) (Selector, bool) {
	arms, ok := a.familyArms(family.root)
	if !ok || index < 0 || index >= len(arms) {
		return 0, false
	}
	return arms[index], true
}

func (a *Authority) entry(selector Selector) (entry, bool) {
	if a == nil || selector == 0 || uint64(selector) > uint64(len(a.entries)) {
		return entry{}, false
	}
	return a.entries[uint32(selector)-1], true
}
