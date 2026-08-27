// Package member owns the declaration-only vocabulary nested in an axis.
//
// Members are schema data, not executable handles. The package imports only
// common schema references, the shared carrier vocabulary, and the framing
// primitive; axis owns publication and sealing of a Catalog containing these
// declarations.
package member

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	"github.com/wippyai/go-lua/internal/framing"
)

// ReadForm is the finite cold read vocabulary. Runtime read implementations
// may be richer, but a member signature carries only this bounded declaration.
type ReadForm uint8

const (
	ReadFormInvalid ReadForm = iota
	ReadFormExact
	ReadFormSelected
	ReadFormSummary
	ReadFormComplete
)

// The short spellings match the established declaration vocabulary used by
// rule/program while keeping the nominal type owned by this package.
const (
	Exact    = ReadFormExact
	Selected = ReadFormSelected
	Summary  = ReadFormSummary
	Complete = ReadFormComplete
)

func (form ReadForm) Available() bool {
	return form >= ReadFormExact && form <= ReadFormComplete
}

// Multiplicity is the finite width vocabulary of one reducer input.
type Multiplicity uint8

const (
	MultiplicityInvalid Multiplicity = iota
	MultiplicityOptional
	MultiplicityOne
	MultiplicityMany
)

func (multiplicity Multiplicity) Available() bool {
	return multiplicity >= MultiplicityOptional && multiplicity <= MultiplicityMany
}

// Role is the finite role vocabulary of a projection.
type Role uint8

const (
	RoleInvalid Role = iota
	Key
	Predicate
	Destination
	// Attribute is a candidate-row column that is neither the join key, the
	// selection predicate, nor the write destination: a trigger context, a
	// body context, a transition, a settled outcome. Its projected local is a
	// declared vocabulary ordinal rather than a factor surface index, so a
	// Program can read a row it joins on without a fourth addressing mode.
	Attribute
	// Identity is a candidate-row column whose value is an owner-issued
	// content identity rather than a local. Every role above publishes a
	// local: a dense coordinate of some directory, or a declared vocabulary
	// ordinal. A local is the address of a row this analyzer minted, and a
	// uint32 carries one. An identity is not an address - it names a subject
	// the analyzer did not mint, a module, a body path, the semantic axis a
	// role is issued under - and no dense width carries one, so it is read
	// through the owner's identity surface instead of its local projection.
	Identity
)

func (role Role) Available() bool { return role >= Key && role <= Identity }

// Relation is one owner-issued relation declaration. Inputs retain authored
// carrier order; an empty input list is valid for a zero-input relation.
// CandidateProvider is the explicit authority whose typed candidate row
// supplies the runtime join. It is never inferred from a carrier's Go type or
// from same-axis placement. Its two arms are the same choice a rule states
// over its own candidate: an owner-qualified axis relation, or an issuance
// relation whose target rows are Program rows and which therefore publishes no
// axis directory at all.
type Relation struct {
	id                schema.EntryID
	Key               schema.Key
	Subject           carrier.Key
	Inputs            []carrier.Key
	CandidateProvider CandidateRef
	// Parent names the relation whose candidate row each of this relation's
	// rows hangs off. A relation that declares one is a nested ordered member
	// set - a bounded port list - addressed by (parent candidate, ordinal)
	// rather than by an occurrence.
	Parent RelationRef
	// Ordinal is the carrier that keys the nested member set. It is declared
	// exactly when Parent is: a parent with no ordinal carrier gives its
	// members no address, and an ordinal carrier with no parent keys nothing.
	Ordinal carrier.Key
	// PublishesKeyVector says rows of this directory carry an ordered dense
	// key vector of another axis: the coordinates the row was constructed
	// from. It is the span a whole-vector read over that other axis is taken
	// over when no directory there groups them, and a child Program consumes
	// it the way it consumes Parent - to know where the span comes from. The
	// accessors themselves stay in the owner's source; what survives here is
	// that they exist.
	PublishesKeyVector bool
	// Correspondences name the foreign axis relations whose candidate orders
	// enumerate the same subjects this relation's own order does. A relation
	// declares one for each foreign order a rule addressing it must reach; a
	// relation that correlates with nothing declares none.
	Correspondences []RelationRef
	// Addressing names the columns of this relation its own rows are
	// addressed by. A join onto this relation pairs against a column named
	// here, so which column that is stays the relation's own statement.
	Addressing Addressing
	// Keys are the ordered column vectors this relation's rows are published
	// under. A publication addresses a row by one of them, so which columns
	// form a key, and in what order, stays the relation's own statement.
	Keys []KeyVector
}

// ID returns the immutable identity issued to this relation by its owning
// axis. Construction rows intentionally return the unavailable zero value;
// only the catalog stored by axis.New carries an issued identity.
func (relation Relation) ID() schema.EntryID { return relation.id }

func (relation Relation) Available() bool {
	if !relation.Key.Available() || !relation.Subject.Available() || !relation.CandidateProvider.Available() {
		return false
	}
	if relation.Parent.Declared() != relation.Ordinal.Available() {
		return false
	}
	if relation.Parent.Declared() && !relation.Parent.Available() {
		return false
	}
	// A directory that publishes a key vector is not itself a nested member
	// set: it is the row a vector read of another axis is spanned by, and a
	// relation addressed under a parent takes its own span from there.
	if relation.PublishesKeyVector && relation.Parent.Declared() {
		return false
	}
	if !relation.Addressing.consistent(relation.Parent.Declared()) {
		return false
	}
	if !keyVectorsConsistent(relation.Keys) {
		return false
	}
	for _, correspondence := range relation.Correspondences {
		if !correspondence.Available() {
			return false
		}
	}
	for _, input := range relation.Inputs {
		if !input.Available() {
			return false
		}
	}
	return true
}

// Nested reports whether this relation is an ordinal-addressed member set of
// another relation's candidate rows.
func (relation Relation) Nested() bool {
	return relation.Parent.Available() && relation.Ordinal.Available()
}

// Projection is one owner-issued projection declaration. Relation names a
// relation in the same Catalog; the catalog validates that it exists.
// CandidateProvider repeats the related relation's explicit provider so the
// sealed projection retains its owner fence without a second lookup rule.
type Projection struct {
	id                schema.EntryID
	Key               schema.Key
	Relation          schema.Key
	Role              Role
	Result            carrier.Key
	CandidateProvider CandidateRef
}

// ID returns the immutable identity issued to this projection by its owning
// axis. Construction rows intentionally return the unavailable zero value.
func (projection Projection) ID() schema.EntryID { return projection.id }

func (projection Projection) Available() bool {
	return projection.Key.Available() && projection.Relation.Available() && projection.Role.Available() && projection.Result.Available() && projection.CandidateProvider.Available()
}

// ReducerInput is one ordered axis read in a reducer's cold signature.
type ReducerInput struct {
	Axis         schema.EntryReference
	Carrier      carrier.Key
	Form         ReadForm
	Multiplicity Multiplicity
	// Tag is the carrier naming which member of a selection the invocation
	// folds. A Summary's tag IS its selection projection, so a Summary read
	// always carries one. A Selected read carries one exactly when the join it
	// reads declares a Predicate, which only the reading Program states, so
	// this declaration leaves it optional and the rule's plan settles it.
	Tag carrier.Key
	// Route is the carrier of the route join's Destination projection: the
	// coordinate this invocation writes to. A routed fold is indexed by that
	// coordinate, so it receives it as a value rather than resolving it from a
	// plan of its own. Whether an input is routed is the reading Program's
	// statement - a route join is named by an output, not by this row - so this
	// declaration leaves it optional and the rule's plan settles it.
	Route carrier.Key
}

func (input ReducerInput) Available() bool {
	if !axisReferenceAvailable(input.Axis) || !input.Carrier.Available() ||
		!input.Form.Available() || !input.Multiplicity.Available() {
		return false
	}
	switch input.Form {
	case ReadFormSelected:
		return true
	case ReadFormSummary:
		return input.Tag.Available() && !input.Route.Available()
	default:
		return !input.Tag.Available() && !input.Route.Available()
	}
}

// ReducerOutput is one ordered axis publication in a reducer's cold
// signature.
type ReducerOutput struct {
	Axis    schema.EntryReference
	Carrier carrier.Key
}

func (output ReducerOutput) Available() bool {
	return axisReferenceAvailable(output.Axis) && output.Carrier.Available()
}

// Reducer is one owner-issued reducer declaration. Inputs and Outputs retain
// authored order and each row carries its complete bounded signature.
type Reducer struct {
	id      schema.EntryID
	Key     schema.Key
	Inputs  []ReducerInput
	Outputs []ReducerOutput
	// Structural marks a fold that publishes no fact. A fold's declared
	// outputs are the facts it publishes, and a structural publication has
	// none: its whole result is the disposition of the branch it was invoked
	// for, which is what a ReductionOutcome already is.
	//
	// It is declared on the row rather than inferred from an empty output
	// list, so an ordinary reducer that simply lost its output is still
	// refused instead of silently reading as structural.
	Structural bool
}

// ID returns the immutable identity issued to this reducer by its owning
// axis. Construction rows intentionally return the unavailable zero value.
func (reducer Reducer) ID() schema.EntryID { return reducer.id }

func (reducer Reducer) Available() bool {
	if !reducer.Key.Available() || (len(reducer.Outputs) == 0) != reducer.Structural {
		return false
	}
	for _, input := range reducer.Inputs {
		if !input.Available() {
			return false
		}
	}
	for _, output := range reducer.Outputs {
		if !output.Available() {
			return false
		}
	}
	return true
}

// CarryTransform is one owner-issued typed transform that may be applied to
// a carried fact.  The carrier pair is part of the member signature: a plan
// can therefore prove that a transform belongs to the factor it is carrying
// without importing an executable function or a domain package.
type CarryTransform struct {
	id        schema.EntryID
	Key       schema.Key
	Candidate carrier.Key
	Input     carrier.Key
	Output    carrier.Key
}

// ID returns the immutable identity issued to this carry transform by its
// owning axis. Construction rows intentionally return the unavailable zero
// value.
func (transform CarryTransform) ID() schema.EntryID { return transform.id }

func (transform CarryTransform) Available() bool {
	return transform.Key.Available() && transform.Candidate.Available() && transform.Input.Available() && transform.Output.Available() && transform.Input == transform.Output
}

func axisReferenceAvailable(reference schema.EntryReference) bool {
	return reference.Surface == schema.SurfaceKindAxis && reference.Key.Available()
}

// RelationRef names one relation member on an axis.
type RelationRef struct {
	Axis   schema.EntryReference
	Member schema.Key
}

func (reference RelationRef) Available() bool {
	return axisReferenceAvailable(reference.Axis) && reference.Member.Available()
}

func (reference RelationRef) Declared() bool {
	return reference.Axis.Declared() || reference.Member.Available()
}

func (reference RelationRef) EntryReference() schema.EntryReference { return reference.Axis }

func (reference RelationRef) AxisReference() schema.EntryReference { return reference.Axis }

// ProjectionRef names one projection member on an axis.
type ProjectionRef struct {
	Axis   schema.EntryReference
	Member schema.Key
}

func (reference ProjectionRef) Available() bool {
	return axisReferenceAvailable(reference.Axis) && reference.Member.Available()
}

func (reference ProjectionRef) Declared() bool {
	return reference.Axis.Declared() || reference.Member.Available()
}

func (reference ProjectionRef) EntryReference() schema.EntryReference { return reference.Axis }

func (reference ProjectionRef) AxisReference() schema.EntryReference { return reference.Axis }

// ReducerRef names one reducer member on an axis.
type ReducerRef struct {
	Axis   schema.EntryReference
	Member schema.Key
}

func (reference ReducerRef) Available() bool {
	return axisReferenceAvailable(reference.Axis) && reference.Member.Available()
}

func (reference ReducerRef) Declared() bool {
	return reference.Axis.Declared() || reference.Member.Available()
}

func (reference ReducerRef) EntryReference() schema.EntryReference { return reference.Axis }

func (reference ReducerRef) AxisReference() schema.EntryReference { return reference.Axis }

// CarryTransformRef names one owner-issued carry transform member on an
// axis.  It is deliberately the same owner-qualified shape as the other
// member references so a transform cannot be laundered from a foreign axis.
type CarryTransformRef struct {
	Axis   schema.EntryReference
	Member schema.Key
}

func (reference CarryTransformRef) Available() bool {
	return axisReferenceAvailable(reference.Axis) && reference.Member.Available()
}

func (reference CarryTransformRef) Declared() bool {
	return reference.Axis.Declared() || reference.Member.Available()
}

func (reference CarryTransformRef) EntryReference() schema.EntryReference { return reference.Axis }

func (reference CarryTransformRef) AxisReference() schema.EntryReference { return reference.Axis }

// Catalog is the ordered declaration catalog nested in one axis. The slices
// retain authored order within each finite kind. An empty catalog is the
// absence value; any catalog with members must pass Complete. An authority-
// only catalog may also be issued by a non-axis owner, which lets a surface
// publish a carrier vocabulary without importing the axis member rows.
type Catalog struct {
	// Authorities are carriers issued by this catalog's enclosing owner.  A
	// raw catalog carries zero authority identities; Catalog.Issue fills them
	// once, and only once.
	Authorities []carrier.Authority
	// CarrierRefs are imported carrier aliases.  The Use spelling belongs to
	// this catalog; Ref names the authority owned by another schema entry.  The
	// binding itself is never issued by this catalog.
	CarrierRefs     []carrier.Binding
	Relations       []Relation
	Projections     []Projection
	Reducers        []Reducer
	CarryTransforms []CarryTransform
	// Selections are the operations this axis publishes produced rows
	// through. A read whose rows are produced rather than enumerated names one
	// of these instead of pairing against a column that does not exist yet.
	Selections []Selection
}

// Issue owner-qualifies one raw declaration catalog and returns the immutable
// copy an axis stores. A catalog can cross this boundary exactly once: every
// input row must still carry the unavailable identity it had at construction,
// and every issued identity must be unique across all member kinds.
//
// The owner is deliberately a schema reference rather than a key so issuance
// cannot accidentally use a foreign surface or a name-derived convention.
func (catalog Catalog) Issue(owner schema.EntryReference) (Catalog, bool) {
	if !owner.Available() || !catalog.raw() || !catalog.Complete() {
		return Catalog{}, false
	}
	// Ordinary member identities are axis-owned. A non-axis owner may issue a
	// carrier catalog (authorities and/or imports), but never an empty catalog
	// or ordinary member rows.
	if owner.Surface != schema.SurfaceKindAxis &&
		(catalog.MemberCount() != 0 || !catalog.HasCarriers()) {
		return Catalog{}, false
	}
	for _, reference := range catalog.CarrierRefs {
		// A local authority is not imported through the reference list.  This
		// also keeps the owner-qualified local resolution unambiguous.
		if reference.Ref.Owner == owner {
			return Catalog{}, false
		}
	}
	issued := catalog.Clone()
	for index := range issued.Authorities {
		authority, authorityOK := carrier.Issue(owner, issued.Authorities[index])
		if !authorityOK {
			return Catalog{}, false
		}
		issued.Authorities[index] = authority
	}
	for index := range issued.Relations {
		if issued.Relations[index].ID().Available() {
			return Catalog{}, false
		}
		id := IssueID(owner, issued.Relations[index].Key)
		if !id.Available() {
			return Catalog{}, false
		}
		issued.Relations[index].id = id
	}
	for index := range issued.Projections {
		if issued.Projections[index].ID().Available() {
			return Catalog{}, false
		}
		id := IssueID(owner, issued.Projections[index].Key)
		if !id.Available() {
			return Catalog{}, false
		}
		issued.Projections[index].id = id
	}
	for index := range issued.Reducers {
		if issued.Reducers[index].ID().Available() {
			return Catalog{}, false
		}
		id := IssueID(owner, issued.Reducers[index].Key)
		if !id.Available() {
			return Catalog{}, false
		}
		issued.Reducers[index].id = id
	}
	for index := range issued.CarryTransforms {
		if issued.CarryTransforms[index].ID().Available() {
			return Catalog{}, false
		}
		id := IssueID(owner, issued.CarryTransforms[index].Key)
		if !id.Available() {
			return Catalog{}, false
		}
		issued.CarryTransforms[index].id = id
	}
	for index := range issued.Selections {
		if issued.Selections[index].ID().Available() {
			return Catalog{}, false
		}
		id := IssueID(owner, issued.Selections[index].Key)
		if !id.Available() {
			return Catalog{}, false
		}
		issued.Selections[index].id = id
	}
	return issued, true
}

// Issued reports whether every row carries the exact identity its owner
// issued. Complete already enforces one unique key across every member kind;
// because IssueID is owner-plus-key issuance, those exact IDs are unique too.
// It is used at the axis seal boundary to keep a manually altered or foreign
// row from entering the sealed catalog.
func (catalog Catalog) Issued(owner schema.EntryReference) bool {
	if !owner.Available() || !catalog.Complete() {
		return false
	}
	if owner.Surface != schema.SurfaceKindAxis &&
		(catalog.MemberCount() != 0 || !catalog.HasCarriers()) {
		return false
	}
	for _, reference := range catalog.CarrierRefs {
		if reference.Ref.Owner == owner {
			return false
		}
	}
	for _, authority := range catalog.Authorities {
		raw := carrier.Authority{Carrier: authority.Carrier, Capability: authority.Capability}
		issued, issuedOK := carrier.Issue(owner, raw)
		if !authority.ID().Available() || !issuedOK || authority.ID() != issued.ID() {
			return false
		}
	}
	check := func(key schema.Key, id schema.EntryID) bool {
		if !id.Available() || id != IssueID(owner, key) {
			return false
		}
		return true
	}
	for _, relation := range catalog.Relations {
		if !check(relation.Key, relation.ID()) {
			return false
		}
	}
	for _, projection := range catalog.Projections {
		if !check(projection.Key, projection.ID()) {
			return false
		}
	}
	for _, reducer := range catalog.Reducers {
		if !check(reducer.Key, reducer.ID()) {
			return false
		}
	}
	for _, transform := range catalog.CarryTransforms {
		if !check(transform.Key, transform.ID()) {
			return false
		}
	}
	for _, selection := range catalog.Selections {
		if !check(selection.Key, selection.ID()) {
			return false
		}
	}
	return true
}

// NewCatalog admits and deep-copies one complete declaration catalog. A fully
// empty catalog is the sole absence value; every declared member occurrence
// must resolve through exactly one local authority or imported binding.
func NewCatalog(authorities []carrier.Authority, references []carrier.Binding, relations []Relation, projections []Projection, reducers []Reducer, transforms []CarryTransform) (Catalog, bool) {
	catalog := Catalog{
		Authorities:     cloneAuthorities(authorities),
		CarrierRefs:     cloneCarrierRefs(references),
		Relations:       cloneRelations(relations),
		Projections:     append([]Projection(nil), projections...),
		Reducers:        cloneReducers(reducers),
		CarryTransforms: append([]CarryTransform(nil), transforms...),
	}
	if !catalog.raw() || !catalog.Complete() {
		return Catalog{}, false
	}
	return catalog, true
}

// raw reports whether a catalog still consists solely of construction rows.
// It is intentionally not exported: callers cross the owner boundary through
// Catalog.Issue, while generated declarations remain ordinary struct values.
func (catalog Catalog) raw() bool {
	for _, authority := range catalog.Authorities {
		if authority.ID().Available() {
			return false
		}
	}
	for _, relation := range catalog.Relations {
		if relation.ID().Available() {
			return false
		}
	}
	for _, projection := range catalog.Projections {
		if projection.ID().Available() {
			return false
		}
	}
	for _, reducer := range catalog.Reducers {
		if reducer.ID().Available() {
			return false
		}
	}
	for _, transform := range catalog.CarryTransforms {
		if transform.ID().Available() {
			return false
		}
	}
	for _, selection := range catalog.Selections {
		if selection.ID().Available() {
			return false
		}
	}
	return true
}

func cloneAuthorities(authorities []carrier.Authority) []carrier.Authority {
	if authorities == nil {
		return nil
	}
	return append([]carrier.Authority(nil), authorities...)
}

func cloneCarrierRefs(references []carrier.Binding) []carrier.Binding {
	if references == nil {
		return nil
	}
	return append([]carrier.Binding(nil), references...)
}
func cloneCarriers(carriers []carrier.Key) []carrier.Key {
	if carriers == nil {
		return nil
	}
	return append([]carrier.Key(nil), carriers...)
}

func cloneRelations(relations []Relation) []Relation {
	if relations == nil {
		return nil
	}
	clone := make([]Relation, len(relations))
	for index, relation := range relations {
		clone[index] = Relation{
			id:                 relation.id,
			Key:                relation.Key,
			Subject:            relation.Subject,
			Inputs:             cloneCarriers(relation.Inputs),
			CandidateProvider:  relation.CandidateProvider,
			Parent:             relation.Parent,
			Ordinal:            relation.Ordinal,
			PublishesKeyVector: relation.PublishesKeyVector,
			Correspondences:    cloneCorrespondences(relation.Correspondences),
			Addressing:         relation.Addressing,
			Keys:               cloneKeyVectors(relation.Keys),
		}
	}
	return clone
}

func cloneReducerInputs(inputs []ReducerInput) []ReducerInput {
	if inputs == nil {
		return nil
	}
	return append([]ReducerInput(nil), inputs...)
}

func cloneReducerOutputs(outputs []ReducerOutput) []ReducerOutput {
	if outputs == nil {
		return nil
	}
	return append([]ReducerOutput(nil), outputs...)
}

func cloneReducers(reducers []Reducer) []Reducer {
	if reducers == nil {
		return nil
	}
	clone := make([]Reducer, len(reducers))
	for index, reducer := range reducers {
		clone[index] = Reducer{
			id:         reducer.id,
			Key:        reducer.Key,
			Inputs:     cloneReducerInputs(reducer.Inputs),
			Outputs:    cloneReducerOutputs(reducer.Outputs),
			Structural: reducer.Structural,
		}
	}
	return clone
}

// Clone returns an independent declaration copy, preserving nil slices.
// WithSelections returns this catalog extended by the operations that publish
// its produced rows. It is separate from construction so an axis that
// publishes none keeps the catalog it already had.
func (catalog Catalog) WithSelections(selections []Selection) (Catalog, bool) {
	if !catalog.raw() {
		return Catalog{}, false
	}
	extended := catalog.Clone()
	extended.Selections = append([]Selection(nil), selections...)
	if !extended.raw() || !extended.Complete() {
		return Catalog{}, false
	}
	return extended, true
}

func (catalog Catalog) Clone() Catalog {
	clone := Catalog{
		Authorities:     cloneAuthorities(catalog.Authorities),
		CarrierRefs:     cloneCarrierRefs(catalog.CarrierRefs),
		Relations:       cloneRelations(catalog.Relations),
		Projections:     append([]Projection(nil), catalog.Projections...),
		Reducers:        cloneReducers(catalog.Reducers),
		CarryTransforms: append([]CarryTransform(nil), catalog.CarryTransforms...),
		Selections:      append([]Selection(nil), catalog.Selections...),
	}
	return clone
}

// HasMembers reports whether this catalog contains any member or carrier
// declaration.
func (catalog Catalog) HasMembers() bool { return catalog.MemberCount() != 0 || catalog.HasCarriers() }

// HasCarriers reports whether the catalog declares the carrier authority
// vocabulary, independently of whether it has any relation members.
func (catalog Catalog) HasCarriers() bool {
	return len(catalog.Authorities) != 0 || len(catalog.CarrierRefs) != 0
}

func (catalog Catalog) MemberCount() int {
	return len(catalog.Relations) + len(catalog.Projections) + len(catalog.Reducers) +
		len(catalog.CarryTransforms) + len(catalog.Selections)
}

// Complete reports whether the catalog is empty or a valid, closed member
// declaration catalog.
func (catalog Catalog) Complete() bool {
	if !catalog.carrierCatalogComplete() {
		return false
	}
	relations := make(map[schema.Key]struct{}, len(catalog.Relations))
	keys := make(map[schema.Key]struct{}, catalog.MemberCount()+len(catalog.Authorities))
	for _, relation := range catalog.Relations {
		if !relation.Available() {
			return false
		}
		if _, duplicate := keys[relation.Key]; duplicate {
			return false
		}
		keys[relation.Key] = struct{}{}
		relations[relation.Key] = struct{}{}
	}
	for _, relation := range catalog.Relations {
		if !catalog.carrierOccurrenceComplete(relation.Subject) {
			return false
		}
		for _, input := range relation.Inputs {
			if !catalog.carrierOccurrenceComplete(input) {
				return false
			}
		}
		if relation.Nested() && !catalog.carrierOccurrenceComplete(relation.Ordinal) {
			return false
		}
		if !correspondencesComplete(relation) {
			return false
		}
		if !relation.Nested() {
			continue
		}
		// A nested member set is addressed from a base row, so its parent must
		// be a relation this same catalog declares and cannot be the set
		// itself: a relation addressed by its own rows has no base row.
		if relation.Parent.Member == relation.Key {
			return false
		}
		if _, held := relations[relation.Parent.Member]; !held {
			return false
		}
		// The emitted owner resolves a parent ordinal through the parent
		// relation's directory on this same owner, so a parent on another axis
		// names an ordinal this owner has no directory for. A rule that must
		// address a nested set from a foreign candidate states a
		// correspondence and keeps its member set at home.
		if relation.Parent.Axis.Key != relation.CandidateProvider.AxisRelation.Axis.Key {
			return false
		}
	}
	columns := make(map[schema.Key]schema.Key, len(catalog.Projections))
	for _, projection := range catalog.Projections {
		if !projection.Available() {
			return false
		}
		if _, duplicate := keys[projection.Key]; duplicate {
			return false
		}
		if _, relationExists := relations[projection.Relation]; !relationExists {
			return false
		}
		keys[projection.Key] = struct{}{}
		columns[projection.Key] = projection.Relation
		if !catalog.carrierOccurrenceComplete(projection.Result) {
			return false
		}
	}
	// An addressing coordinate is an ordinary column, so every one a relation
	// names is a projection this catalog declares over that same relation. A
	// relation that named a foreign column would be addressed through rows it
	// does not own.
	for _, relation := range catalog.Relations {
		for _, column := range relation.Addressing.Columns() {
			owner, declared := columns[column]
			if !declared || owner != relation.Key {
				return false
			}
		}
		// A key vector addresses rows of the relation that declares it, so
		// every column it names is a column of that same relation. A key over a
		// foreign column would publish rows through an address their own
		// relation does not hold.
		for _, key := range relation.Keys {
			if _, duplicate := keys[key.Name]; duplicate {
				return false
			}
			keys[key.Name] = struct{}{}
			for _, column := range key.Columns {
				owner, declared := columns[column]
				if !declared || owner != relation.Key {
					return false
				}
			}
		}
	}
	// A selection publishes rows into a relation this catalog declares and
	// stamps them with a projection over that same relation, so the reading
	// rule joins produced rows exactly as it joins enumerated ones. Where the
	// relation also names its tag coordinate the two must agree: one column,
	// one authority.
	for _, selection := range catalog.Selections {
		if !selection.Available() {
			return false
		}
		if _, duplicate := keys[selection.Key]; duplicate {
			return false
		}
		keys[selection.Key] = struct{}{}
		if _, relationExists := relations[selection.Relation]; !relationExists {
			return false
		}
		owner, declared := columns[selection.Tag]
		if !declared || owner != selection.Relation {
			return false
		}
	}
	for _, relation := range catalog.Relations {
		if !relation.Addressing.Tag.Available() {
			continue
		}
		for _, selection := range catalog.Selections {
			if selection.Relation == relation.Key && selection.Tag != relation.Addressing.Tag {
				return false
			}
		}
	}
	for _, reducer := range catalog.Reducers {
		if !reducer.Available() {
			return false
		}
		if _, duplicate := keys[reducer.Key]; duplicate {
			return false
		}
		keys[reducer.Key] = struct{}{}
		for _, input := range reducer.Inputs {
			if !catalog.carrierOccurrenceComplete(input.Carrier) ||
				(input.Tag.Available() && !catalog.carrierOccurrenceComplete(input.Tag)) ||
				(input.Route.Available() && !catalog.carrierOccurrenceComplete(input.Route)) {
				return false
			}
		}
		for _, output := range reducer.Outputs {
			if !catalog.carrierOccurrenceComplete(output.Carrier) {
				return false
			}
		}
	}
	for _, transform := range catalog.CarryTransforms {
		if !transform.Available() {
			return false
		}
		if _, duplicate := keys[transform.Key]; duplicate {
			return false
		}
		keys[transform.Key] = struct{}{}
		if !catalog.carrierOccurrenceComplete(transform.Candidate) ||
			!catalog.carrierOccurrenceComplete(transform.Input) ||
			!catalog.carrierOccurrenceComplete(transform.Output) {
			return false
		}
	}
	return true
}

// Available reports whether this is a complete authored catalog. Empty
// absence is not an available member catalog.
func (catalog Catalog) Available() bool { return catalog.HasMembers() && catalog.Complete() }

// carrierCatalogComplete enforces the alias namespace shared by local
// authorities and imports.  An alias must resolve to exactly one declaration;
// a local/import collision would make every later carrier occurrence
// ambiguous, even when both declarations individually look complete.
func (catalog Catalog) carrierCatalogComplete() bool {
	aliases := make(map[carrier.Key]struct{}, len(catalog.Authorities)+len(catalog.CarrierRefs))
	for _, authority := range catalog.Authorities {
		if !authority.Available() {
			return false
		}
		if _, duplicate := aliases[authority.Carrier]; duplicate {
			return false
		}
		aliases[authority.Carrier] = struct{}{}
	}
	for _, binding := range catalog.CarrierRefs {
		if !binding.Available() {
			return false
		}
		if _, duplicate := aliases[binding.Use]; duplicate {
			return false
		}
		aliases[binding.Use] = struct{}{}
	}
	return true
}

func (catalog Catalog) carrierOccurrenceComplete(use carrier.Key) bool {
	if !use.Available() {
		return false
	}
	_, ok := catalog.resolveCarrier(use)
	return ok
}

func (catalog Catalog) resolveCarrier(use carrier.Key) (carrier.Binding, bool) {
	if !use.Available() {
		return carrier.Binding{}, false
	}
	var resolved carrier.Binding
	found := 0
	for _, authority := range catalog.Authorities {
		if authority.Carrier != use {
			continue
		}
		resolved = carrier.Binding{Use: use, Ref: carrier.Ref{Carrier: use}}
		found++
	}
	for _, binding := range catalog.CarrierRefs {
		if binding.Use != use {
			continue
		}
		resolved = binding
		found++
	}
	return resolved, found == 1
}

// ResolveCarrier resolves one occurrence against exactly one local authority
// or imported reference.  Local authorities are qualified with owner only at
// resolution time; the raw catalog does not bake an enclosing owner into its
// authored data.
func (catalog Catalog) ResolveCarrier(owner schema.EntryReference, use carrier.Key) (carrier.Binding, bool) {
	if owner.Surface != schema.SurfaceKindAxis || !owner.Key.Available() {
		return carrier.Binding{}, false
	}
	if !catalog.HasCarriers() {
		return carrier.Binding{}, false
	}
	binding, ok := catalog.resolveCarrier(use)
	if !ok {
		return carrier.Binding{}, false
	}
	if binding.Ref.Owner == (schema.EntryReference{}) {
		binding.Ref.Owner = owner
	}
	return binding, true
}

// ResolveLocalCarrier answers only the local arm.  It is used by axis
// signature admission, where an imported reference is expressly forbidden.
func (catalog Catalog) ResolveLocalCarrier(owner schema.EntryReference, use carrier.Key) (carrier.Authority, bool) {
	if owner.Surface != schema.SurfaceKindAxis || !owner.Key.Available() || !use.Available() {
		return carrier.Authority{}, false
	}
	if _, resolved := catalog.resolveCarrier(use); !resolved {
		return carrier.Authority{}, false
	}
	var authority carrier.Authority
	found := 0
	for _, candidate := range catalog.Authorities {
		if candidate.Carrier == use {
			authority = candidate
			found++
		}
	}
	return authority, found == 1
}

func (catalog Catalog) RelationCount() int       { return len(catalog.Relations) }
func (catalog Catalog) ProjectionCount() int     { return len(catalog.Projections) }
func (catalog Catalog) ReducerCount() int        { return len(catalog.Reducers) }
func (catalog Catalog) CarryTransformCount() int { return len(catalog.CarryTransforms) }

func (catalog Catalog) AuthorityCount() int { return len(catalog.Authorities) }

func (catalog Catalog) CarrierRefCount() int { return len(catalog.CarrierRefs) }

func (catalog Catalog) AuthorityAt(index int) (carrier.Authority, bool) {
	if index < 0 || index >= len(catalog.Authorities) {
		return carrier.Authority{}, false
	}
	return catalog.Authorities[index], true
}

func (catalog Catalog) CarrierRefAt(index int) (carrier.Binding, bool) {
	if index < 0 || index >= len(catalog.CarrierRefs) {
		return carrier.Binding{}, false
	}
	return catalog.CarrierRefs[index], true
}

// Authority resolves exactly one local authority by its authored carrier
// alias. A local/import collision or duplicate declaration is not a lookup;
// it is an unavailable authority.
func (catalog Catalog) Authority(key carrier.Key) (carrier.Authority, bool) {
	if _, resolved := catalog.resolveCarrier(key); !resolved {
		return carrier.Authority{}, false
	}
	var result carrier.Authority
	found := 0
	for _, authority := range catalog.Authorities {
		if authority.Carrier == key {
			result = authority
			found++
		}
	}
	if found != 1 || !result.Available() {
		return carrier.Authority{}, false
	}
	return result, true
}

// CarrierAuthority exposes one local authority for structural cross-surface
// validation. The carrier package owns the returned value and identity.
func (catalog Catalog) CarrierAuthority(key carrier.Key) (carrier.Authority, bool) {
	return catalog.Authority(key)
}

// CarrierRef resolves exactly one imported alias by its authored carrier
// spelling. A local/import collision or duplicate declaration is unavailable.
func (catalog Catalog) CarrierRef(key carrier.Key) (carrier.Binding, bool) {
	if _, resolved := catalog.resolveCarrier(key); !resolved {
		return carrier.Binding{}, false
	}
	var result carrier.Binding
	found := 0
	for _, binding := range catalog.CarrierRefs {
		if binding.Use == key {
			result = binding
			found++
		}
	}
	if found != 1 || !result.Available() {
		return carrier.Binding{}, false
	}
	return result, true
}

func (catalog Catalog) RelationAt(index int) (Relation, bool) {
	if index < 0 || index >= len(catalog.Relations) {
		return Relation{}, false
	}
	relation := catalog.Relations[index]
	relation.Inputs = cloneCarriers(relation.Inputs)
	return relation, true
}

func (catalog Catalog) ProjectionAt(index int) (Projection, bool) {
	if index < 0 || index >= len(catalog.Projections) {
		return Projection{}, false
	}
	return catalog.Projections[index], true
}

func (catalog Catalog) ReducerAt(index int) (Reducer, bool) {
	if index < 0 || index >= len(catalog.Reducers) {
		return Reducer{}, false
	}
	reducer := catalog.Reducers[index]
	reducer.Inputs = cloneReducerInputs(reducer.Inputs)
	reducer.Outputs = cloneReducerOutputs(reducer.Outputs)
	return reducer, true
}

func (catalog Catalog) CarryTransformAt(index int) (CarryTransform, bool) {
	if index < 0 || index >= len(catalog.CarryTransforms) {
		return CarryTransform{}, false
	}
	return catalog.CarryTransforms[index], true
}

func (catalog Catalog) Relation(key schema.Key) (Relation, bool) {
	for _, relation := range catalog.Relations {
		if relation.Key == key {
			relation.Inputs = cloneCarriers(relation.Inputs)
			return relation, true
		}
	}
	return Relation{}, false
}

func (catalog Catalog) Projection(key schema.Key) (Projection, bool) {
	for _, projection := range catalog.Projections {
		if projection.Key == key {
			return projection, true
		}
	}
	return Projection{}, false
}

func (catalog Catalog) Reducer(key schema.Key) (Reducer, bool) {
	for _, reducer := range catalog.Reducers {
		if reducer.Key == key {
			return Reducer{
				id:         reducer.id,
				Key:        reducer.Key,
				Inputs:     cloneReducerInputs(reducer.Inputs),
				Outputs:    cloneReducerOutputs(reducer.Outputs),
				Structural: reducer.Structural,
			}, true
		}
	}
	return Reducer{}, false
}

func (catalog Catalog) CarryTransform(key schema.Key) (CarryTransform, bool) {
	for _, transform := range catalog.CarryTransforms {
		if transform.Key == key {
			return transform, true
		}
	}
	return CarryTransform{}, false
}

// RelationOrdinal resolves a relation's dense ordinal in declaration order.
func (catalog Catalog) RelationOrdinal(key schema.Key) (uint32, bool) {
	for index, relation := range catalog.Relations {
		if relation.Key == key {
			return uint32(index), true
		}
	}
	return 0, false
}

// ProjectionOrdinal resolves a projection's dense ordinal in declaration
// order.
func (catalog Catalog) ProjectionOrdinal(key schema.Key) (uint32, bool) {
	for index, projection := range catalog.Projections {
		if projection.Key == key {
			return uint32(index), true
		}
	}
	return 0, false
}

// ReducerOrdinal resolves a reducer's dense ordinal in declaration order.
func (catalog Catalog) ReducerOrdinal(key schema.Key) (uint32, bool) {
	for index, reducer := range catalog.Reducers {
		if reducer.Key == key {
			return uint32(index), true
		}
	}
	return 0, false
}

// CarryTransformOrdinal resolves a carry transform's dense ordinal in
// declaration order.
func (catalog Catalog) CarryTransformOrdinal(key schema.Key) (uint32, bool) {
	for index, transform := range catalog.CarryTransforms {
		if transform.Key == key {
			return uint32(index), true
		}
	}
	return 0, false
}

// References returns a deep copy of all provider and reducer axis references
// in canonical member order. The relation member key is resolved against the
// referenced axis by the seal/plan owner; only the common axis reference is a
// root dependency here.
func (catalog Catalog) References() schema.EntryReferences {
	var references schema.EntryReferences
	for _, carrier := range catalog.CarrierRefs {
		references = append(references, carrier.Ref.Owner)
	}
	for _, relation := range catalog.Relations {
		references = append(references, relation.CandidateProvider.References()...)
		for _, correspondence := range relation.Correspondences {
			references = append(references, correspondence.EntryReference())
		}
	}
	for _, projection := range catalog.Projections {
		references = append(references, projection.CandidateProvider.References()...)
	}
	for _, reducer := range catalog.Reducers {
		for _, input := range reducer.Inputs {
			references = append(references, input.Axis)
		}
		for _, output := range reducer.Outputs {
			references = append(references, output.Axis)
		}
	}
	return references.Clone()
}

const (
	contentRecordCatalog        uint64 = 1
	contentRecordRelation       uint64 = 2
	contentRecordProjection     uint64 = 3
	contentRecordReducer        uint64 = 4
	contentRecordInput          uint64 = 5
	contentRecordRelationInput  uint64 = 6
	contentRecordOutput         uint64 = 7
	contentRecordCarryTransform uint64 = 8
	contentRecordProvider       uint64 = 9
	// contentRecordNestedSet is written only by a relation that declares a
	// nested ordered member set. An ordinary relation emits the exact stream
	// it emitted before the nested form existed, so adding the form remints
	// no declaration that does not use it.
	contentRecordNestedSet uint64 = 10
	// contentRecordIssuedProvider is written only by a relation or projection
	// whose candidate authority is an issued Program row. An axis-relation
	// provider emits the exact reference it emitted before the choice existed,
	// so stating the choice remints no catalog that keeps the arm it had.
	contentRecordIssuedProvider uint64 = 11
	// contentRecordCorrespondence is written only by a relation that states
	// one. A relation that correlates with nothing emits the exact stream it
	// emitted before the statement existed, so adding the form remints no
	// declaration that does not use it.
	contentRecordCorrespondence uint64 = 12

	// contentRecordAddressing carries the columns a relation states its own
	// rows are addressed by. It is a tagged trailing extension of the relation
	// record: a relation that declares no addressing emits nothing, so its
	// canonical stream is exactly the stream it had before the coordinates
	// could be named.
	contentRecordAddressing uint64 = 13

	// contentRecordKeyVector carries one ordered column vector a relation states its
	// rows are published under. It is a tagged trailing collection on the
	// relation record: a relation that declares no key emits nothing, so its
	// canonical stream is exactly the stream it had before keys could be named.
	contentRecordKeyVector uint64 = 15

	// contentRecordSelection carries one operation that publishes a relation's
	// produced rows. It is a tagged trailing collection: an axis that publishes
	// none emits nothing, so its canonical stream is exactly the stream it had
	// before produced rows could be named.
	contentRecordSelection uint64 = 14

	// contentRecordCarrierCatalog is a tagged trailing extension. Legacy
	// catalogs without carrier authorities/imports retain their exact stream;
	// migrated catalogs append the complete carrier vocabulary here.
	contentRecordCarrierCatalog uint64 = 16
	contentRecordAuthority      uint64 = 17
	contentRecordCarrierRef     uint64 = 18
)

// WriteContent writes the catalog's canonical declaration stream. Collection
// and member-kind boundaries are explicit, and all authored order is retained.
func (catalog Catalog) WriteContent(content *framing.Writer) error {
	if err := content.Record(contentRecordCatalog); err != nil {
		return err
	}
	if err := content.Count(uint64(len(catalog.Relations))); err != nil {
		return err
	}
	for _, relation := range catalog.Relations {
		if err := content.Record(contentRecordRelation); err != nil {
			return err
		}
		if err := content.String(string(relation.Key)); err != nil {
			return err
		}
		if err := content.Bytes(entryBytes(relation.ID())); err != nil {
			return err
		}
		if err := content.String(string(relation.Subject)); err != nil {
			return err
		}
		if err := writeCandidateProvider(content, relation.CandidateProvider); err != nil {
			return err
		}
		if err := content.Count(uint64(len(relation.Inputs))); err != nil {
			return err
		}
		for _, input := range relation.Inputs {
			if err := content.Record(contentRecordRelationInput); err != nil {
				return err
			}
			if err := content.String(string(input)); err != nil {
				return err
			}
		}
		for _, correspondence := range relation.Correspondences {
			if err := content.Record(contentRecordCorrespondence); err != nil {
				return err
			}
			if err := writeRelationReference(content, correspondence); err != nil {
				return err
			}
		}
		if relation.Addressing.Declared() {
			if err := content.Record(contentRecordAddressing); err != nil {
				return err
			}
			for _, column := range [...]schema.Key{
				relation.Addressing.Address, relation.Addressing.Parent,
				relation.Addressing.Ordinal, relation.Addressing.Tag,
				relation.Addressing.Occurrence,
			} {
				if err := content.String(string(column)); err != nil {
					return err
				}
			}
		}
		for _, key := range relation.Keys {
			if err := content.Record(contentRecordKeyVector); err != nil {
				return err
			}
			if err := content.String(string(key.Name)); err != nil {
				return err
			}
			for _, column := range key.Columns {
				if err := content.String(string(column)); err != nil {
					return err
				}
			}
		}
		if !relation.Nested() {
			continue
		}
		if err := content.Record(contentRecordNestedSet); err != nil {
			return err
		}
		if err := writeRelationReference(content, relation.Parent); err != nil {
			return err
		}
		if err := content.String(string(relation.Ordinal)); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(catalog.Projections))); err != nil {
		return err
	}
	for _, projection := range catalog.Projections {
		if err := content.Record(contentRecordProjection); err != nil {
			return err
		}
		if err := content.String(string(projection.Key)); err != nil {
			return err
		}
		if err := content.Bytes(entryBytes(projection.ID())); err != nil {
			return err
		}
		if err := content.String(string(projection.Relation)); err != nil {
			return err
		}
		if err := content.Uint(uint64(projection.Role)); err != nil {
			return err
		}
		if err := content.String(string(projection.Result)); err != nil {
			return err
		}
		if err := writeCandidateProvider(content, projection.CandidateProvider); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(catalog.Reducers))); err != nil {
		return err
	}
	for _, reducer := range catalog.Reducers {
		if err := content.Record(contentRecordReducer); err != nil {
			return err
		}
		if err := content.String(string(reducer.Key)); err != nil {
			return err
		}
		if err := content.Bytes(entryBytes(reducer.ID())); err != nil {
			return err
		}
		if err := content.Count(uint64(len(reducer.Inputs))); err != nil {
			return err
		}
		for _, input := range reducer.Inputs {
			if err := content.Record(contentRecordInput); err != nil {
				return err
			}
			if err := content.Uint(uint64(input.Axis.Surface)); err != nil {
				return err
			}
			if err := content.String(string(input.Axis.Key)); err != nil {
				return err
			}
			if err := content.String(string(input.Carrier)); err != nil {
				return err
			}
			if err := content.Uint(uint64(input.Form)); err != nil {
				return err
			}
			if err := content.Uint(uint64(input.Multiplicity)); err != nil {
				return err
			}
			if err := content.String(string(input.Tag)); err != nil {
				return err
			}
		}
		if err := content.Count(uint64(len(reducer.Outputs))); err != nil {
			return err
		}
		for _, output := range reducer.Outputs {
			if err := content.Record(contentRecordOutput); err != nil {
				return err
			}
			if err := content.Uint(uint64(output.Axis.Surface)); err != nil {
				return err
			}
			if err := content.String(string(output.Axis.Key)); err != nil {
				return err
			}
			if err := content.String(string(output.Carrier)); err != nil {
				return err
			}
		}
	}
	if err := content.Count(uint64(len(catalog.CarryTransforms))); err != nil {
		return err
	}
	for _, transform := range catalog.CarryTransforms {
		if err := content.Record(contentRecordCarryTransform); err != nil {
			return err
		}
		if err := content.String(string(transform.Key)); err != nil {
			return err
		}
		if err := content.Bytes(entryBytes(transform.ID())); err != nil {
			return err
		}
		if err := content.String(string(transform.Candidate)); err != nil {
			return err
		}
		if err := content.String(string(transform.Input)); err != nil {
			return err
		}
		if err := content.String(string(transform.Output)); err != nil {
			return err
		}
	}
	if catalog.HasCarriers() {
		if err := content.Record(contentRecordCarrierCatalog); err != nil {
			return err
		}
		if err := content.Count(uint64(len(catalog.Authorities))); err != nil {
			return err
		}
		for _, authority := range catalog.Authorities {
			if err := content.Record(contentRecordAuthority); err != nil {
				return err
			}
			if err := content.String(string(authority.Carrier)); err != nil {
				return err
			}
			if err := content.Bytes(entryBytes(authority.ID())); err != nil {
				return err
			}
			if err := content.Uint(uint64(authority.Capability)); err != nil {
				return err
			}
		}
		if err := content.Count(uint64(len(catalog.CarrierRefs))); err != nil {
			return err
		}
		for _, binding := range catalog.CarrierRefs {
			if err := content.Record(contentRecordCarrierRef); err != nil {
				return err
			}
			if err := content.String(string(binding.Use)); err != nil {
				return err
			}
			if err := content.Uint(uint64(binding.Ref.Owner.Surface)); err != nil {
				return err
			}
			if err := content.String(string(binding.Ref.Owner.Key)); err != nil {
				return err
			}
			if err := content.String(string(binding.Ref.Carrier)); err != nil {
				return err
			}
		}
	}
	if len(catalog.Selections) != 0 {
		if err := content.Count(uint64(len(catalog.Selections))); err != nil {
			return err
		}
		for _, selection := range catalog.Selections {
			if err := content.Record(contentRecordSelection); err != nil {
				return err
			}
			if err := content.String(string(selection.Key)); err != nil {
				return err
			}
			if err := content.Bytes(entryBytes(selection.ID())); err != nil {
				return err
			}
			if err := content.String(string(selection.Relation)); err != nil {
				return err
			}
			if err := content.String(string(selection.Tag)); err != nil {
				return err
			}
		}
	}
	return nil
}

// SelectionCount is the number of operations this axis publishes rows through.
func (catalog Catalog) SelectionCount() int { return len(catalog.Selections) }

// SelectionAt returns one selection at its declaration position.
func (catalog Catalog) SelectionAt(index int) (Selection, bool) {
	if index < 0 || index >= len(catalog.Selections) {
		return Selection{}, false
	}
	return catalog.Selections[index], true
}

// Selection resolves one selection by the key its owner declared it under.
func (catalog Catalog) Selection(key schema.Key) (Selection, bool) {
	for _, selection := range catalog.Selections {
		if selection.Key == key {
			return selection, true
		}
	}
	return Selection{}, false
}

// writeCandidateProvider emits whichever arm the provider states. The axis arm
// writes the exact relation reference it wrote before the choice existed, so a
// catalog that keeps the arm it already had is not reminted; the issued arm
// writes its own record.
func writeCandidateProvider(content *framing.Writer, provider CandidateRef) error {
	if !provider.Issued() {
		return writeRelationReference(content, provider.AxisRelation)
	}
	if err := content.Record(contentRecordIssuedProvider); err != nil {
		return err
	}
	return content.String(string(provider.IssuedRow))
}

func writeRelationReference(content *framing.Writer, reference RelationRef) error {
	if err := content.Record(contentRecordProvider); err != nil {
		return err
	}
	if err := content.Uint(uint64(reference.Axis.Surface)); err != nil {
		return err
	}
	if err := content.String(string(reference.Axis.Key)); err != nil {
		return err
	}
	return content.String(string(reference.Member))
}

// EntryContent is the canonical-content spelling used by parent declarations.
func (catalog Catalog) EntryContent(content *framing.Writer) error {
	return catalog.WriteContent(content)
}
