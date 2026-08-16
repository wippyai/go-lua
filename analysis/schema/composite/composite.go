// Package composite owns the composite surface of the analyzer declaration
// table: the record one relation is declared as, and the surface laws the
// declaration root seals it under.
//
// A composite is one declared relation over already-declared coordinate
// spaces. Its membership is a set of named roles, each ranging over one axis;
// its reads are declared cones, one per role; its result is a closed two-case
// output discipline; and its behaviour under the solver is a declared
// determinism, monotonicity, and reentrancy. Nothing here executes: a
// composite entry is data, and the executable half is bound elsewhere.
//
// The surface carries no type parameter. Unlike an axis or a rule, a composite
// declares no engine slot and instantiates no carrier, so there is nothing for
// a composition's input record to thread through it. That is the same fact the
// no-hidden-state law states from the other side: a composite owns no storage.
// Every coordinate space it touches - the axis a role ranges over, the axis a
// reducer writes, and every intermediate axis it routes through - is named,
// and every name must resolve to a declared axis entry. A composite that
// wanted private state would have to declare the axis that holds it, at which
// point the state is no longer private.
//
// # The deferred hot half
//
// The hot half of a composite - Frame and BindPair - is not built here; it
// awaits the store cut. The contract it will be built to is fixed, and is
// recorded here so the cold half is not designed against a moving target:
//
//	typed Frame, admitted write, callback-scoped.
//
// A typed Frame means the hot side receives the composite's roles already
// resolved to their axes' carrier coordinates, so a bound composite never
// asserts and never re-derives a coordinate from an ordinal. An admitted write
// means the only write a composite may perform is the one its declared output
// discipline admits - the reducer's output axis, or the capability's declared
// patch contracts - checked against this sealed declaration rather than
// trusted. Callback-scoped means the Frame is valid only for the duration of
// the callback it is handed to; it is never retained, never escapes, and never
// outlives the store's own view, so a composite cannot accumulate state across
// invocations by holding its Frame.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package composite

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawCompositeIdentity
	LawAxisPhase
	LawMembershipDeclared
	LawRoleUnique
	LawMembershipResolves
	LawConeForm
	LawDemandSource
	LawDemandMonotone
	LawOutputDiscipline
	LawReducerOutputAxis
	LawReducerDescent
	LawCapabilityContract
	LawDisciplineDeclared
	LawNoHiddenState
	LawCommutativity
	LawDependencyResolves
)

// ConeForm is the closed catalog of shapes a role's declared read may take.
type ConeForm uint8

const (
	ConeInvalid ConeForm = iota
	// ConeExact reads every coordinate of the role's axis.
	ConeExact
	// ConeSelected reads a declared selection of the role's axis.
	ConeSelected
	// ConeSummary reads one folded fact over the role's axis.
	ConeSummary
	// ConeDemand reads the point set on the role's axis that is derived from
	// another role's read. It is the one cone form whose extent is not known
	// until the composite runs, so it is the one form that names a source.
	ConeDemand
)

func (form ConeForm) Available() bool { return form >= ConeExact && form <= ConeDemand }

// Cone is one role's declared read. Source is the role whose read derives this
// cone's point set; exactly the demand form declares it, because it is the only
// form whose extent is derived rather than declared.
type Cone struct {
	Form   ConeForm
	Source schema.Key
}

func (cone Cone) Available() bool {
	if !cone.Form.Available() {
		return false
	}
	return (cone.Form == ConeDemand) == cone.Source.Available()
}

// Role is one member of a composite's membership: a name inside the composite,
// the axis it ranges over, and the read it declares over that axis.
type Role struct {
	Key  schema.Key
	Axis schema.Key
	Cone Cone
}

func (role Role) Available() bool {
	return role.Key.Available() && role.Axis.Available() && role.Cone.Available()
}

// Ordering is whether a composite's roles are positional or interchangeable.
type Ordering uint8

const (
	OrderingInvalid Ordering = iota
	// OrderingOrdered is a positional membership: permuting the roles changes
	// the relation.
	OrderingOrdered
	// OrderingCommutative is an interchangeable membership: permuting the roles
	// leaves the relation unchanged.
	OrderingCommutative
)

func (ordering Ordering) Available() bool {
	return ordering == OrderingOrdered || ordering == OrderingCommutative
}

// Determinism is whether a composite's result is a function of its declared
// reads alone.
type Determinism uint8

const (
	DeterminismInvalid Determinism = iota
	DeterminismDeterministic
	DeterminismNondeterministic
)

func (determinism Determinism) Available() bool {
	return determinism == DeterminismDeterministic || determinism == DeterminismNondeterministic
}

// Monotonicity is whether a composite's result only grows as its declared
// reads grow.
type Monotonicity uint8

const (
	MonotonicityInvalid Monotonicity = iota
	MonotonicityMonotone
	MonotonicityNonMonotone
)

func (monotonicity Monotonicity) Available() bool {
	return monotonicity == MonotonicityMonotone || monotonicity == MonotonicityNonMonotone
}

// Reentrancy is whether a composite admits a nested invocation of itself while
// one is already in flight.
type Reentrancy uint8

const (
	ReentrancyInvalid Reentrancy = iota
	ReentrancyReentrant
	ReentrancyExclusive
)

func (reentrancy Reentrancy) Available() bool {
	return reentrancy == ReentrancyReentrant || reentrancy == ReentrancyExclusive
}

// Discipline is the composite's declared behaviour under the solver.
type Discipline struct {
	Determinism  Determinism
	Monotonicity Monotonicity
	Reentrancy   Reentrancy
}

func (discipline Discipline) Available() bool {
	return discipline.Determinism.Available() && discipline.Monotonicity.Available() &&
		discipline.Reentrancy.Available()
}

// OutputKind is the discriminant of the closed two-case output discipline.
type OutputKind uint8

const (
	OutputInvalid OutputKind = iota
	// OutputReducer folds the composite's reads onto one declared output axis,
	// under a declared rank descent.
	OutputReducer
	// OutputCapability hands the composite's result to the store as a set of
	// per-role patch contracts closed and committed under declared identities.
	OutputCapability
)

func (kind OutputKind) Available() bool {
	return kind == OutputReducer || kind == OutputCapability
}

// Reducer is the folding case of the output discipline. Descent names the rank
// components of the output axis that strictly descend, most significant first;
// it is the composite's own termination declaration. The components are
// form-validated here and resolved against the output axis's published rank
// when that axis is bound, because a declaration table holds no bound algebra.
type Reducer struct {
	Axis    schema.Key
	Descent []uint16
}

func (reducer Reducer) Available() bool {
	if !reducer.Axis.Available() || len(reducer.Descent) == 0 {
		return false
	}
	for index := 1; index < len(reducer.Descent); index++ {
		if reducer.Descent[index] <= reducer.Descent[index-1] {
			return false
		}
	}
	return true
}

// Absent reports the one admissible undeclared shape.
func (reducer Reducer) Absent() bool { return !reducer.Axis.Available() && len(reducer.Descent) == 0 }

// Patch is one role's declared write contract in the capability case.
type Patch struct {
	Role     schema.Key
	Contract identity.ContentID
}

func (patch Patch) Available() bool { return patch.Role.Available() && patch.Contract.Available() }

// Capability is the store-handoff case of the output discipline: one patch
// contract per role, the closure the patches are applied under, and the commit
// contract the closure is sealed by. The three identities name contracts whose
// own surface does not exist yet, so they are declared and form-validated here
// and resolved when that surface lands. Form-validating an identity is not
// resolving it, and this surface does not pretend otherwise.
type Capability struct {
	Patches []Patch
	Closure identity.ContentID
	Commit  identity.ContentID
}

func (capability Capability) Available() bool {
	if len(capability.Patches) == 0 || !capability.Closure.Available() || !capability.Commit.Available() {
		return false
	}
	for _, patch := range capability.Patches {
		if !patch.Available() {
			return false
		}
	}
	return true
}

// Absent reports the one admissible undeclared shape.
func (capability Capability) Absent() bool {
	return len(capability.Patches) == 0 && !capability.Closure.Available() && !capability.Commit.Available()
}

// Output is the closed two-case union. The discriminant selects exactly one
// case, and the other case must be absent: a record carrying both is not a
// union, and the surface says so rather than reading the discriminant and
// ignoring the rest.
type Output struct {
	Kind       OutputKind
	Reducer    Reducer
	Capability Capability
}

func (output Output) Available() bool {
	switch output.Kind {
	case OutputReducer:
		return output.Reducer.Available() && output.Capability.Absent()
	case OutputCapability:
		return output.Capability.Available() && output.Reducer.Absent()
	default:
		return false
	}
}

// Spec is the authored declaration of one composite.
type Spec struct {
	// Key is the composite's authored identity and its diagnostic name, so a
	// composite has exactly one spelling in the analyzer. It derives the entry
	// identity a verdict carries.
	Key schema.Key
	// Roles is the composite's membership, in declaration order. Ordering says
	// whether that order is part of the relation.
	Roles    []Role
	Ordering Ordering
	Output   Output
	// Discipline is the composite's declared behaviour under the solver.
	Discipline Discipline
	// Intermediates are the axes this composite routes through that are neither
	// a role axis nor the output axis. Naming one is what keeps a composite
	// free of hidden storage: an intermediate must be a declared axis entry,
	// owned and written like any other.
	Intermediates []schema.Key
	// Dependencies are the composite entries this one depends on, by their
	// authored keys. An edge must resolve to a declared composite and may not
	// be a self-edge.
	Dependencies []schema.Key
}

// Entry is one admitted composite declaration. It is immutable once built.
type Entry struct {
	key schema.Key
	id  schema.EntryID

	roles         []Role
	ordering      Ordering
	output        Output
	discipline    Discipline
	intermediates []schema.Key
	dependencies  []schema.Key
}

// New admits one authored declaration. A rejected spec returns false rather
// than a partially usable entry.
func New(spec Spec) (*Entry, bool) {
	if !specAdmissible(spec) {
		return nil, false
	}
	entry := &Entry{
		key:           spec.Key,
		id:            schema.NewEntryID(schema.SurfaceKindComposite, spec.Key),
		roles:         append([]Role(nil), spec.Roles...),
		ordering:      spec.Ordering,
		output:        spec.Output,
		discipline:    spec.Discipline,
		intermediates: append([]schema.Key(nil), spec.Intermediates...),
		dependencies:  append([]schema.Key(nil), spec.Dependencies...),
	}
	entry.output.Reducer.Descent = append([]uint16(nil), spec.Output.Reducer.Descent...)
	entry.output.Capability.Patches = append([]Patch(nil), spec.Output.Capability.Patches...)
	return entry, entry.EntryAvailable() && entry.membershipComplete() && entry.disciplineComplete()
}

func specAdmissible(spec Spec) bool {
	if !spec.Key.Available() || !spec.Ordering.Available() || !spec.Discipline.Available() {
		return false
	}
	if len(spec.Roles) == 0 || !spec.Output.Available() {
		return false
	}
	seen := make(map[schema.Key]bool, len(spec.Roles))
	for _, role := range spec.Roles {
		if !role.Available() || seen[role.Key] {
			return false
		}
		seen[role.Key] = true
	}
	for _, role := range spec.Roles {
		if role.Cone.Form != ConeDemand {
			continue
		}
		if role.Cone.Source == role.Key || !seen[role.Cone.Source] {
			return false
		}
		// A demand cone makes the read set of one role depend on another role's
		// read, so the composite is a fixpoint over its own demand. Only a
		// monotone composite has one.
		if spec.Discipline.Monotonicity != MonotonicityMonotone {
			return false
		}
	}
	if spec.Output.Kind == OutputCapability && !capabilityCoversRoles(spec.Output.Capability, spec.Roles) {
		return false
	}
	for _, intermediate := range spec.Intermediates {
		if !intermediate.Available() || intermediate == spec.Output.Reducer.Axis {
			return false
		}
		for _, role := range spec.Roles {
			if role.Axis == intermediate {
				return false
			}
		}
	}
	for _, dependency := range spec.Dependencies {
		if !dependency.Available() || dependency == spec.Key {
			return false
		}
	}
	if spec.Output.Kind == OutputReducer {
		for _, role := range spec.Roles {
			if role.Axis == spec.Output.Reducer.Axis {
				return false
			}
		}
	}
	return symmetricRoles(spec.Roles) == (spec.Ordering == OrderingCommutative)
}

// capabilityCoversRoles reports whether the capability declares exactly one
// patch contract per declared role.
func capabilityCoversRoles(capability Capability, roles []Role) bool {
	if len(capability.Patches) != len(roles) {
		return false
	}
	declared := make(map[schema.Key]bool, len(roles))
	for _, role := range roles {
		declared[role.Key] = true
	}
	patched := make(map[schema.Key]bool, len(capability.Patches))
	for _, patch := range capability.Patches {
		if !declared[patch.Role] || patched[patch.Role] {
			return false
		}
		patched[patch.Role] = true
	}
	return true
}

// symmetricRoles reports whether the declaration distinguishes its roles at
// all. A declaration is data: two roles that carry the same axis, the same cone
// form, and the same cone source are indistinguishable in it, so permuting them
// yields the identical declaration and the relation it declares is necessarily
// commutative in them. An author who means an asymmetric relation over one axis
// must give the roles distinguishing structure - a different cone form, or a
// demand sourced from the other side - rather than rely on a distinction the
// declaration does not carry.
func symmetricRoles(roles []Role) bool {
	if len(roles) < 2 {
		return false
	}
	first := roles[0]
	for _, role := range roles[1:] {
		if role.Axis != first.Axis || role.Cone.Form != first.Cone.Form || role.Cone.Source != first.Cone.Source {
			return false
		}
	}
	return true
}

func (entry *Entry) Key() schema.Key { return entry.key }

func (entry *Entry) ID() schema.EntryID { return entry.id }

func (entry *Entry) Ordering() Ordering { return entry.ordering }

func (entry *Entry) Output() Output { return entry.output }

func (entry *Entry) Discipline() Discipline { return entry.discipline }

func (entry *Entry) RoleCount() int { return len(entry.roles) }

func (entry *Entry) RoleAt(index int) (Role, bool) {
	if index < 0 || index >= len(entry.roles) {
		return Role{}, false
	}
	return entry.roles[index], true
}

// Role resolves one member by its authored identity inside this composite.
func (entry *Entry) Role(key schema.Key) (Role, bool) {
	for _, role := range entry.roles {
		if role.Key == key {
			return role, true
		}
	}
	return Role{}, false
}

func (entry *Entry) IntermediateCount() int { return len(entry.intermediates) }

func (entry *Entry) IntermediateAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(entry.intermediates) {
		return "", false
	}
	return entry.intermediates[index], true
}

func (entry *Entry) DependencyCount() int { return len(entry.dependencies) }

func (entry *Entry) DependencyAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(entry.dependencies) {
		return "", false
	}
	return entry.dependencies[index], true
}

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the relation it identifies is completely declared is the
// surface's own law, stated by Seal, so an incomplete composite is reported as
// the incomplete composite it is rather than as an unidentifiable row.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

func (entry *Entry) membershipComplete() bool {
	if len(entry.roles) == 0 {
		return false
	}
	for _, role := range entry.roles {
		if !role.Available() {
			return false
		}
	}
	return entry.output.Available()
}

func (entry *Entry) disciplineComplete() bool {
	return entry.discipline.Available() && entry.ordering.Available()
}

// surface is the composite contribution to the analyzer declaration root.
type surface struct{ entries []*Entry }

// NewSurface hands one ordered set of composite declarations to the table.
func NewSurface(entries []*Entry) schema.Surface { return surface{entries: entries} }

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindComposite }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.entries))
	for index, entry := range contribution.entries {
		entries[index] = entry
	}
	return entries
}

// Seal states the composite surface's own laws over the indexed view. Every
// axis a composite names is resolved against the already-sealed axis surface,
// so a relation over a coordinate space that does not exist is rejected here
// rather than discovered at bind.
func (contribution surface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	// A composite resolves its membership against the axis inventory, so the
	// axis surface must be sealed below it. The catalog order is the bind phase
	// order; stating the composite surface's position states the phase.
	if schema.SurfaceKindAxis >= schema.SurfaceKindComposite {
		return failure(schema.EntryID{}, LawAxisPhase, schema.DispositionMalformed)
	}
	axes, axesOK := sealed.Surface(schema.SurfaceKindAxis)
	if !axesOK {
		return failure(schema.EntryID{}, LawAxisPhase, schema.DispositionIncomplete)
	}
	keys := make(map[schema.Key]schema.EntryID, view.Count())
	entries := make([]*Entry, 0, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil {
			return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		entries = append(entries, entry)
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of this entry's key, so an entry cannot travel under another
		// surface's identity.
		if !entry.key.Available() || entry.id != schema.NewEntryID(schema.SurfaceKindComposite, entry.key) {
			return failure(entry.id, LawCompositeIdentity, schema.DispositionMalformed)
		}
		keys[entry.key] = entry.id
		if verdict := entry.sealMembership(axes); verdict.Available() {
			return verdict
		}
		if verdict := entry.sealOutput(axes); verdict.Available() {
			return verdict
		}
		if !entry.discipline.Available() || !entry.ordering.Available() {
			return failure(entry.id, LawDisciplineDeclared, schema.DispositionIncomplete)
		}
		if verdict := entry.sealIntermediates(axes); verdict.Available() {
			return verdict
		}
		if symmetricRoles(entry.roles) != (entry.ordering == OrderingCommutative) {
			return failure(entry.id, LawCommutativity, schema.DispositionMalformed)
		}
	}
	// Dependency edges resolve against the sealed inventory, so a composite
	// cannot declare an edge to a composite that is not in this table.
	for _, entry := range entries {
		for _, dependency := range entry.dependencies {
			if dependency == entry.key {
				return failure(entry.id, LawDependencyResolves, schema.DispositionMalformed)
			}
			if _, declared := keys[dependency]; !declared {
				return failure(entry.id, LawDependencyResolves, schema.DispositionIncomplete)
			}
		}
	}
	return schema.SealFailure{}
}

func (entry *Entry) sealMembership(axes schema.View) schema.SealFailure {
	if len(entry.roles) == 0 {
		return failure(entry.id, LawMembershipDeclared, schema.DispositionIncomplete)
	}
	roles := make(map[schema.Key]bool, len(entry.roles))
	for _, role := range entry.roles {
		if !role.Key.Available() || !role.Axis.Available() {
			return failure(entry.id, LawMembershipDeclared, schema.DispositionIncomplete)
		}
		if roles[role.Key] {
			return failure(entry.id, LawRoleUnique, schema.DispositionDuplicate)
		}
		roles[role.Key] = true
		if !axisDeclared(axes, role.Axis) {
			return failure(entry.id, LawMembershipResolves, schema.DispositionIncomplete)
		}
	}
	for _, role := range entry.roles {
		if !role.Cone.Available() {
			return failure(entry.id, LawConeForm, schema.DispositionMalformed)
		}
		if role.Cone.Form != ConeDemand {
			continue
		}
		if role.Cone.Source == role.Key {
			return failure(entry.id, LawDemandSource, schema.DispositionMalformed)
		}
		if !roles[role.Cone.Source] {
			return failure(entry.id, LawDemandSource, schema.DispositionIncomplete)
		}
		// A demand cone is a fixpoint over the composite's own read set: the
		// points it reads are derived from a read that the composite's own
		// result may extend. Only a monotone composite reaches a fixpoint over
		// that, so declaring one without monotonicity is malformed.
		if entry.discipline.Monotonicity != MonotonicityMonotone {
			return failure(entry.id, LawDemandMonotone, schema.DispositionMalformed)
		}
	}
	return schema.SealFailure{}
}

func (entry *Entry) sealOutput(axes schema.View) schema.SealFailure {
	if !entry.output.Kind.Available() {
		return failure(entry.id, LawOutputDiscipline, schema.DispositionIncomplete)
	}
	reducer, capability := !entry.output.Reducer.Absent(), !entry.output.Capability.Absent()
	if reducer == capability {
		return failure(entry.id, LawOutputDiscipline, schema.DispositionMalformed)
	}
	if (entry.output.Kind == OutputReducer) != reducer {
		return failure(entry.id, LawOutputDiscipline, schema.DispositionMalformed)
	}
	switch entry.output.Kind {
	case OutputReducer:
		if !entry.output.Reducer.Axis.Available() || !axisDeclared(axes, entry.output.Reducer.Axis) {
			return failure(entry.id, LawReducerOutputAxis, schema.DispositionIncomplete)
		}
		// A reducer that writes an axis it also reads is a fixpoint over
		// itself, wearing the shape of a one-way fold. Every role of a
		// composite is a read, so the output axis must differ from all of them.
		for _, role := range entry.roles {
			if role.Axis == entry.output.Reducer.Axis {
				return failure(entry.id, LawReducerOutputAxis, schema.DispositionMalformed)
			}
		}
		if !entry.output.Reducer.Available() {
			return failure(entry.id, LawReducerDescent, schema.DispositionMalformed)
		}
	case OutputCapability:
		if !entry.output.Capability.Closure.Available() || !entry.output.Capability.Commit.Available() {
			return failure(entry.id, LawCapabilityContract, schema.DispositionIncomplete)
		}
		for _, patch := range entry.output.Capability.Patches {
			if !patch.Available() {
				return failure(entry.id, LawCapabilityContract, schema.DispositionIncomplete)
			}
		}
		if !capabilityCoversRoles(entry.output.Capability, entry.roles) {
			return failure(entry.id, LawCapabilityContract, schema.DispositionMalformed)
		}
	}
	return schema.SealFailure{}
}

// sealIntermediates states the no-hidden-state law. A composite owns no
// storage: this record has no place to put any. What it can do is route
// through a coordinate space, and every such space must be a declared axis
// entry with its own owner and writer, so the state is visible where state is
// declared. An intermediate that repeats a role axis or the output axis is not
// an intermediate; it is that axis under a second name, and naming it twice
// hides which declaration governs it.
func (entry *Entry) sealIntermediates(axes schema.View) schema.SealFailure {
	for _, intermediate := range entry.intermediates {
		if !intermediate.Available() {
			return failure(entry.id, LawNoHiddenState, schema.DispositionIncomplete)
		}
		if !axisDeclared(axes, intermediate) {
			return failure(entry.id, LawNoHiddenState, schema.DispositionIncomplete)
		}
		if entry.output.Kind == OutputReducer && intermediate == entry.output.Reducer.Axis {
			return failure(entry.id, LawNoHiddenState, schema.DispositionDuplicate)
		}
		for _, role := range entry.roles {
			if role.Axis == intermediate {
				return failure(entry.id, LawNoHiddenState, schema.DispositionDuplicate)
			}
		}
	}
	return schema.SealFailure{}
}

// axisDeclared resolves one authored axis key against the sealed axis surface.
// The composite surface never sees an axis's own record: it derives the axis
// surface's identity for the key it was handed and asks the sealed view, so a
// reference is resolved against the same table it is being sealed into.
func axisDeclared(axes schema.View, key schema.Key) bool {
	if !key.Available() {
		return false
	}
	id := schema.NewEntryID(schema.SurfaceKindAxis, key)
	if !id.Available() {
		return false
	}
	_, declared := axes.ByID(id)
	return declared
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return schema.SurfaceLawFailure(schema.SurfaceKindComposite, entry, law, disposition)
}
