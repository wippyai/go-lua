// Package denominator owns the denominator surface of the analyzer
// declaration table: the record one closed world is declared as, and the
// surface laws the declaration root seals it under.
//
// A denominator is the declared universe a totality claim is made against. A
// claim that something holds for every member, or that a member is absent, is
// only as good as the set it quantifies over, and that set has to be named,
// owned, and closed at a stated point. This surface is where it is named: one
// identity, the surface entry that owns it, the identity of the universe it
// describes, and the phase at which that universe stops admitting members.
//
// Nothing is resolved that cannot be. The owner is resolved against the
// already-sealed surface it names, so a denominator cannot be owned by a row
// that does not exist. The universe is a declared content identity whose own
// describing surface does not exist yet; it is form-validated here and
// resolved when that surface lands. Form-validating an identity is not
// resolving it, and this surface does not pretend otherwise.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package denominator

import (
	"bytes"
	"sort"

	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = seal.SurfaceLawFloor + iota
	LawDenominatorIdentity
	LawOwnerDeclared
	LawOwnerPhase
	LawOwnerResolves
	LawUniverseDeclared
	LawUniverseUnique
	LawPhaseDeclared
	LawRelationOwnerDeclared
	LawRelationFormDeclared
	LawRelationParentResolves
	LawRelationParentUnique
	LawRelationParentCycle
	LawRelationOwnerCycle
)

// Phase is the point at which a denominator's universe stops admitting
// members. It is the closed catalog of closure points the analyzer has.
type Phase uint8

const (
	PhaseInvalid Phase = iota
	// PhaseSeal closes the universe when the declaration table seals: its
	// members are declarations, and no later input can add one.
	PhaseSeal
	// PhasePublication closes the universe when the owning surface's facts are
	// published: its members are derived, and the fixpoint that derives them
	// must be closed before the set is total.
	PhasePublication
)

func (phase Phase) Available() bool { return phase == PhaseSeal || phase == PhasePublication }

// Owner is the surface entry a denominator's universe belongs to. It is a
// reference into the same declaration table, so it names a surface and one
// entry's authored key inside it.
type Owner struct {
	Surface schema.SurfaceKind
	Entry   schema.Key
}

func (owner Owner) Available() bool { return owner.Surface.Available() && owner.Entry.Available() }

// Entry is one admitted denominator declaration. It is immutable once built.
type Entry struct {
	key      schema.Key
	id       schema.EntryID
	owner    Owner
	universe identity.ContentID
	phase    Phase
}

// coordinatePrefix is the spelling one axis's coordinate world is named under.
// A closed world has exactly one spelling in the analyzer, and this one is the
// axis's own key under the name of what the world holds.
const coordinatePrefix = "coordinates/"

// Coordinate derives the closed world of one declared axis: the coordinate
// population that axis carries.
//
// Nothing here is authored. The identity is the axis's own key, the owner is
// the axis row itself, and the phase is publication because an axis's
// coordinates are derived by the solver, so the set is total only once the
// fixpoint that derives it has closed. A coordinate world therefore cannot
// disagree with the axis it describes, and an axis cannot acquire a world that
// quantifies over something else.
//
// The universe is the one thing this surface does not derive: it is the
// identity of the set description, which the axis surface holds and has
// already proved unique across its inventory, so two axes cannot present one
// closed world under two names. A source that names no axis, or no set
// description, yields no entry rather than a partially usable one.
func Coordinate(axis schema.Key, universe identity.ContentID) (*Entry, bool) {
	if !axis.Available() || !universe.Available() {
		return nil, false
	}
	key := schema.Key(coordinatePrefix + string(axis))
	entry := &Entry{
		key:      key,
		id:       schema.NewEntryID(schema.SurfaceKindDenominator, key),
		owner:    Owner{Surface: schema.SurfaceKindAxis, Entry: axis},
		universe: universe,
		phase:    PhasePublication,
	}
	return entry, entry.EntryAvailable()
}

func (entry *Entry) Key() schema.Key { return entry.key }

func (entry *Entry) ID() schema.EntryID { return entry.id }

func (entry *Entry) Owner() Owner { return entry.owner }

func (entry *Entry) Universe() identity.ContentID { return entry.universe }

func (entry *Entry) Phase() Phase { return entry.phase }

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the closed world it identifies is completely declared is
// the surface's own law, stated by Seal.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

// EntryContent writes this closed world's declared data: the surface entry that
// owns its universe, the declared identity of the universe itself, and the
// phase at which that universe stops admitting members. A totality claim is
// read against exactly these, so a denominator that moves any of them quantifies
// over a different set and the table digest says so.
func (entry *Entry) EntryContent(content *framing.Writer) error {
	if err := content.Uint(uint64(entry.owner.Surface)); err != nil {
		return err
	}
	if err := content.String(string(entry.owner.Entry)); err != nil {
		return err
	}
	if err := content.Bytes(entry.universe[:]); err != nil {
		return err
	}
	return content.Uint(uint64(entry.phase))
}

// RelationOwner identifies the immutable component that publishes one
// relation family. It is denominator vocabulary, not a runtime dispatch
// value: a relation declaration records the owner that is responsible for its
// published rows, and the denominator validates the owner graph at seal.
type RelationOwner uint8

const (
	RelationOwnerUnset RelationOwner = iota
	RelationOwnerProgramSource
	RelationOwnerProgramFlow
	RelationOwnerProgramStatic
	RelationOwnerProgramModule
	RelationOwnerTarget
	RelationOwnerLinkProject
	RelationOwnerLinkBoundary
	RelationOwnerLinkModule
	RelationOwnerLinkStatic
	RelationOwnerLinkHost
)

func (owner RelationOwner) Available() bool {
	return owner >= RelationOwnerProgramSource && owner <= RelationOwnerLinkHost
}

// Program reports whether owner is one of the four Program-interior owners:
// the group a cold Program component's own denominator rows are drawn from,
// as opposed to Target or a Link boundary owner.
func (owner RelationOwner) Program() bool {
	return owner >= RelationOwnerProgramSource && owner <= RelationOwnerProgramModule
}

// RelationForm states how one relation family exists at publication. The
// form is declaration data and is deliberately not an executable hook.
type RelationForm uint8

const (
	RelationFormUnset RelationForm = iota
	RelationFormAuthored
	RelationFormSealDerived
	RelationFormVirtualPredicate
)

func (form RelationForm) Available() bool {
	return form >= RelationFormAuthored && form <= RelationFormVirtualPredicate
}

// RelationSpec is the authored declaration of one semantic relation family.
// Parent identities are resolved only when the complete denominator surface
// seals, because a relation may refer to a row declared later in the same
// contribution.
type RelationSpec struct {
	Key     schema.Key
	Owner   RelationOwner
	Form    RelationForm
	Parents []schema.EntryID
}

// RelationEntry is one immutable relation-family declaration in the unified
// denominator surface. Its identity is the denominator surface's derivation
// of Key, exactly like the existing closed-world Entry.
type RelationEntry struct {
	key     schema.Key
	id      schema.EntryID
	owner   RelationOwner
	form    RelationForm
	parents []schema.EntryID
}

// NewRelation admits one relation declaration. Parent existence and graph
// laws are intentionally deferred to the unified surface seal.
func NewRelation(spec RelationSpec) (*RelationEntry, bool) {
	if !spec.Key.Available() || !spec.Owner.Available() || !spec.Form.Available() {
		return nil, false
	}
	entry := &RelationEntry{
		key:     spec.Key,
		id:      schema.NewEntryID(schema.SurfaceKindDenominator, spec.Key),
		owner:   spec.Owner,
		form:    spec.Form,
		parents: append([]schema.EntryID(nil), spec.Parents...),
	}
	return entry, entry.EntryAvailable()
}

func (entry *RelationEntry) Key() schema.Key { return entry.key }

func (entry *RelationEntry) ID() schema.EntryID { return entry.id }

func (entry *RelationEntry) Owner() RelationOwner { return entry.owner }

func (entry *RelationEntry) Form() RelationForm { return entry.form }

func (entry *RelationEntry) Parents() []schema.EntryID {
	if entry == nil {
		return nil
	}
	return append([]schema.EntryID(nil), entry.parents...)
}

func (entry *RelationEntry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

// EntryContent writes the relation declaration's data. Parent IDs are sorted
// for the content stream so a caller's input order cannot create a second
// spelling of the same declaration.
func (entry *RelationEntry) EntryContent(content *framing.Writer) error {
	if err := content.Uint(uint64(entry.owner)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.form)); err != nil {
		return err
	}
	parents := sortedEntryIDs(entry.parents)
	if err := content.Count(uint64(len(parents))); err != nil {
		return err
	}
	for _, parent := range parents {
		if err := content.Bytes(parent[:]); err != nil {
			return err
		}
	}
	return nil
}

// surface is the denominator contribution to the analyzer declaration root.
type surface struct {
	entries   []*Entry
	relations []*RelationEntry
}

// NewSurface hands one ordered set of closed-world declarations and, when
// supplied, one or more ordered sets of relation declarations to the table.
// The variadic relation argument preserves the existing one-argument caller
// while making both declaration kinds one SurfaceKindDenominator
// contribution.
func NewSurface(entries []*Entry, relationSets ...[]*RelationEntry) seal.Surface {
	var relations []*RelationEntry
	for _, relationSet := range relationSets {
		relations = append(relations, relationSet...)
	}
	return surface{entries: entries, relations: relations}
}

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindDenominator }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, 0, len(contribution.entries)+len(contribution.relations))
	for _, entry := range contribution.entries {
		entries = append(entries, entry)
	}
	for _, entry := range contribution.relations {
		entries = append(entries, entry)
	}
	return entries
}

// Seal states the denominator surface's own laws over the indexed view. Every
// owner is resolved against the surface it names, in the same table this
// surface is being sealed into.
func (contribution surface) Seal(view seal.View, sealed seal.Sealed) schema.SealFailure {
	universes := make(map[identity.ContentID]schema.EntryID, view.Count())
	relations := make(map[schema.EntryID]*RelationEntry, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		if !rowOK || row == nil {
			return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		if relation, relationOK := row.(*RelationEntry); relationOK {
			if relation == nil {
				return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
			}
			if !relation.key.Available() || relation.id != schema.NewEntryID(schema.SurfaceKindDenominator, relation.key) {
				return failure(relation.id, LawDenominatorIdentity, schema.DispositionMalformed)
			}
			if !relation.owner.Available() {
				return failure(relation.id, LawRelationOwnerDeclared, schema.DispositionIncomplete)
			}
			if !relation.form.Available() {
				return failure(relation.id, LawRelationFormDeclared, schema.DispositionIncomplete)
			}
			if _, duplicate := relations[relation.id]; duplicate {
				return failure(relation.id, seal.LawEntryUnique, schema.DispositionDuplicate)
			}
			relations[relation.id] = relation
			continue
		}
		entry, entryOK := row.(*Entry)
		if !entryOK || entry == nil {
			return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of this entry's key, so an entry cannot travel under another
		// surface's identity.
		if !entry.key.Available() || entry.id != schema.NewEntryID(schema.SurfaceKindDenominator, entry.key) {
			return failure(entry.id, LawDenominatorIdentity, schema.DispositionMalformed)
		}
		if !entry.owner.Available() {
			return failure(entry.id, LawOwnerDeclared, schema.DispositionIncomplete)
		}
		// The catalog order is the reference order. An owner at or above this
		// surface has not been sealed yet, so the reference cannot be resolved
		// and is not admitted as though it had been.
		if entry.owner.Surface >= schema.SurfaceKindDenominator {
			return failure(entry.id, LawOwnerPhase, schema.DispositionMalformed)
		}
		owning, owningOK := sealed.Surface(entry.owner.Surface)
		if !owningOK {
			return failure(entry.id, LawOwnerPhase, schema.DispositionIncomplete)
		}
		if _, declared := owning.ByID(schema.NewEntryID(entry.owner.Surface, entry.owner.Entry)); !declared {
			return failure(entry.id, LawOwnerResolves, schema.DispositionIncomplete)
		}
		if !entry.universe.Available() {
			return failure(entry.id, LawUniverseDeclared, schema.DispositionIncomplete)
		}
		// Two denominators over one universe description are one closed world
		// under two names, and a totality claim would then depend on which name
		// it was made under.
		if prior, duplicate := universes[entry.universe]; duplicate {
			return failure(prior, LawUniverseUnique, schema.DispositionDuplicate)
		}
		universes[entry.universe] = entry.id
		if !entry.phase.Available() {
			return failure(entry.id, LawPhaseDeclared, schema.DispositionIncomplete)
		}
	}
	if relationFailure, failed := validateRelations(relations); failed {
		return relationFailure
	}
	return schema.SealFailure{}
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return seal.SurfaceLawFailure(schema.SurfaceKindDenominator, entry, law, disposition)
}

func validateRelations(relations map[schema.EntryID]*RelationEntry) (schema.SealFailure, bool) {
	ids := make([]schema.EntryID, 0, len(relations))
	for id := range relations {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return bytes.Compare(ids[left][:], ids[right][:]) < 0 })
	for _, id := range ids {
		relation := relations[id]
		parents := relation.parents
		seen := make(map[schema.EntryID]struct{}, len(parents))
		for _, parent := range parents {
			if !parent.Available() {
				return failure(id, LawRelationParentResolves, schema.DispositionIncomplete), true
			}
			if _, exists := relations[parent]; !exists {
				return failure(id, LawRelationParentResolves, schema.DispositionIncomplete), true
			}
			if _, duplicate := seen[parent]; duplicate {
				return failure(id, LawRelationParentUnique, schema.DispositionDuplicate), true
			}
			seen[parent] = struct{}{}
		}
	}
	if id, cyclic := relationSelfCycle(relations, ids); cyclic {
		return failure(id, LawRelationParentCycle, schema.DispositionMalformed), true
	}
	if id, crossOwner := crossOwnerRelationCycle(relations, ids); crossOwner {
		return failure(id, LawRelationParentCycle, schema.DispositionMalformed), true
	}
	if id, cyclic := relationOwnerCycle(relations, ids); cyclic {
		return failure(id, LawRelationOwnerCycle, schema.DispositionMalformed), true
	}
	return schema.SealFailure{}, false
}

func relationSelfCycle(relations map[schema.EntryID]*RelationEntry, ids []schema.EntryID) (schema.EntryID, bool) {
	for _, id := range ids {
		for _, parent := range relations[id].parents {
			if parent == id {
				return id, true
			}
		}
	}
	return schema.EntryID{}, false
}

// crossOwnerRelationCycle rejects a parent SCC that crosses component
// ownership. A recursive SCC inside one owner is permitted; it is a local
// relation recursion, not a cycle in the owner dependency graph.
func crossOwnerRelationCycle(relations map[schema.EntryID]*RelationEntry, ids []schema.EntryID) (schema.EntryID, bool) {
	for _, component := range relationSCCs(relations, ids) {
		if len(component) < 2 {
			continue
		}
		owner := relations[component[0]].owner
		for _, id := range component[1:] {
			if relations[id].owner != owner {
				return component[0], true
			}
		}
	}
	return schema.EntryID{}, false
}

func relationSCCs(relations map[schema.EntryID]*RelationEntry, ids []schema.EntryID) [][]schema.EntryID {
	index := 0
	indices := make(map[schema.EntryID]int, len(ids))
	lowlink := make(map[schema.EntryID]int, len(ids))
	onStack := make(map[schema.EntryID]bool, len(ids))
	stack := make([]schema.EntryID, 0, len(ids))
	components := make([][]schema.EntryID, 0)
	var visit func(schema.EntryID)
	visit = func(id schema.EntryID) {
		indices[id] = index
		lowlink[id] = index
		index++
		stack = append(stack, id)
		onStack[id] = true
		parents := sortedEntryIDs(relations[id].parents)
		for _, parent := range parents {
			if _, seen := indices[parent]; !seen {
				visit(parent)
				if lowlink[parent] < lowlink[id] {
					lowlink[id] = lowlink[parent]
				}
			} else if onStack[parent] && indices[parent] < lowlink[id] {
				lowlink[id] = indices[parent]
			}
		}
		if lowlink[id] != indices[id] {
			return
		}
		component := make([]schema.EntryID, 0)
		for {
			last := len(stack) - 1
			member := stack[last]
			stack = stack[:last]
			onStack[member] = false
			component = append(component, member)
			if member == id {
				break
			}
		}
		sort.Slice(component, func(left, right int) bool { return bytes.Compare(component[left][:], component[right][:]) < 0 })
		components = append(components, component)
	}
	for _, id := range ids {
		if _, seen := indices[id]; !seen {
			visit(id)
		}
	}
	return components
}

func relationOwnerCycle(relations map[schema.EntryID]*RelationEntry, ids []schema.EntryID) (schema.EntryID, bool) {
	edges := make(map[RelationOwner]map[RelationOwner]struct{})
	witness := make(map[RelationOwner]map[RelationOwner]schema.EntryID)
	owners := make(map[RelationOwner]struct{})
	for _, id := range ids {
		relation := relations[id]
		owners[relation.owner] = struct{}{}
		for _, parent := range relation.parents {
			parentRelation := relations[parent]
			if parentRelation.owner == relation.owner {
				continue
			}
			if edges[relation.owner] == nil {
				edges[relation.owner] = make(map[RelationOwner]struct{})
				witness[relation.owner] = make(map[RelationOwner]schema.EntryID)
			}
			edges[relation.owner][parentRelation.owner] = struct{}{}
			if prior, exists := witness[relation.owner][parentRelation.owner]; !exists || bytes.Compare(id[:], prior[:]) < 0 {
				witness[relation.owner][parentRelation.owner] = id
			}
		}
	}
	orderedOwners := make([]RelationOwner, 0, len(owners))
	for owner := range owners {
		orderedOwners = append(orderedOwners, owner)
	}
	sort.Slice(orderedOwners, func(left, right int) bool { return orderedOwners[left] < orderedOwners[right] })
	const (
		ownerUnseen uint8 = iota
		ownerVisiting
		ownerComplete
	)
	state := make(map[RelationOwner]uint8, len(owners))
	var visit func(RelationOwner) (schema.EntryID, bool)
	visit = func(owner RelationOwner) (schema.EntryID, bool) {
		switch state[owner] {
		case ownerVisiting:
			return schema.EntryID{}, true
		case ownerComplete:
			return schema.EntryID{}, false
		}
		state[owner] = ownerVisiting
		parents := make([]RelationOwner, 0, len(edges[owner]))
		for parent := range edges[owner] {
			parents = append(parents, parent)
		}
		sort.Slice(parents, func(left, right int) bool { return parents[left] < parents[right] })
		for _, parent := range parents {
			if id, cyclic := visit(parent); cyclic {
				if id.Available() {
					return id, true
				}
				return witness[owner][parent], true
			}
		}
		state[owner] = ownerComplete
		return schema.EntryID{}, false
	}
	for _, owner := range orderedOwners {
		if id, cyclic := visit(owner); cyclic {
			if id.Available() {
				return id, true
			}
			return schema.EntryID{}, true
		}
	}
	return schema.EntryID{}, false
}

func sortedEntryIDs(ids []schema.EntryID) []schema.EntryID {
	ordered := append([]schema.EntryID(nil), ids...)
	sort.Slice(ordered, func(left, right int) bool { return bytes.Compare(ordered[left][:], ordered[right][:]) < 0 })
	return ordered
}
