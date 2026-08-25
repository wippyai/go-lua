// Package member owns the declaration-only vocabulary nested in an axis.
//
// Members are schema data, not executable handles. The package deliberately
// imports only the common schema references and the framing primitive; axis
// owns publication and sealing of a Catalog containing these declarations.
package member

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

// Carrier is the nominal name of one owner-issued coordinate carrier. It is
// deliberately distinct from schema.Key: a member signature's coordinate
// vocabulary must not be silently substituted with an unrelated schema key.
type Carrier schema.Key

func (carrier Carrier) Available() bool { return schema.Key(carrier).Available() }

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
	Key               schema.Key
	Subject           Carrier
	Inputs            []Carrier
	CandidateProvider CandidateRef
	// Parent names the relation whose candidate row each of this relation's
	// rows hangs off. A relation that declares one is a nested ordered member
	// set - a bounded port list - addressed by (parent candidate, ordinal)
	// rather than by an occurrence.
	Parent RelationRef
	// Ordinal is the carrier that keys the nested member set. It is declared
	// exactly when Parent is: a parent with no ordinal carrier gives its
	// members no address, and an ordinal carrier with no parent keys nothing.
	Ordinal Carrier
	// Correspondences name the foreign axis relations whose candidate orders
	// enumerate the same subjects this relation's own order does. A relation
	// declares one for each foreign order a rule addressing it must reach; a
	// relation that correlates with nothing declares none.
	Correspondences []RelationRef
}

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
	Key               schema.Key
	Relation          schema.Key
	Role              Role
	Result            Carrier
	CandidateProvider CandidateRef
}

func (projection Projection) Available() bool {
	return projection.Key.Available() && projection.Relation.Available() && projection.Role.Available() && projection.Result.Available() && projection.CandidateProvider.Available()
}

// ReducerInput is one ordered axis read in a reducer's cold signature.
type ReducerInput struct {
	Axis         schema.EntryReference
	Carrier      Carrier
	Form         ReadForm
	Multiplicity Multiplicity
	// Tag is the carrier naming which member of a selection the invocation
	// folds. A Summary's tag IS its selection projection, so a Summary read
	// always carries one. A Selected read carries one exactly when the join it
	// reads declares a Predicate, which only the reading Program states, so
	// this declaration leaves it optional and the rule's plan settles it.
	Tag Carrier
	// Route is the carrier of the route join's Destination projection: the
	// coordinate this invocation writes to. A routed fold is indexed by that
	// coordinate, so it receives it as a value rather than resolving it from a
	// plan of its own. Whether an input is routed is the reading Program's
	// statement - a route join is named by an output, not by this row - so this
	// declaration leaves it optional and the rule's plan settles it.
	Route Carrier
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
	Carrier Carrier
}

func (output ReducerOutput) Available() bool {
	return axisReferenceAvailable(output.Axis) && output.Carrier.Available()
}

// Reducer is one owner-issued reducer declaration. Inputs and Outputs retain
// authored order and each row carries its complete bounded signature.
type Reducer struct {
	Key     schema.Key
	Inputs  []ReducerInput
	Outputs []ReducerOutput
}

func (reducer Reducer) Available() bool {
	if !reducer.Key.Available() || len(reducer.Outputs) == 0 {
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
	Key       schema.Key
	Candidate Carrier
	Input     Carrier
	Output    Carrier
}

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
// retain authored order within each finite kind. A zero catalog is the
// legacy-absence state during migration; any catalog with members must pass
// Complete.
type Catalog struct {
	Relations       []Relation
	Projections     []Projection
	Reducers        []Reducer
	CarryTransforms []CarryTransform
}

// NewCatalog admits and deep-copies one declaration catalog. Empty input is
// the legacy-absence value and is accepted; every nonempty input must be
// complete and internally closed.
func NewCatalog(relations []Relation, projections []Projection, reducers []Reducer, transforms []CarryTransform) (Catalog, bool) {
	catalog := Catalog{
		Relations:       cloneRelations(relations),
		Projections:     append([]Projection(nil), projections...),
		Reducers:        cloneReducers(reducers),
		CarryTransforms: append([]CarryTransform(nil), transforms...),
	}
	if !catalog.Complete() {
		return Catalog{}, false
	}
	return catalog, true
}

func cloneCarriers(carriers []Carrier) []Carrier {
	if carriers == nil {
		return nil
	}
	return append([]Carrier(nil), carriers...)
}

func cloneRelations(relations []Relation) []Relation {
	if relations == nil {
		return nil
	}
	clone := make([]Relation, len(relations))
	for index, relation := range relations {
		clone[index] = Relation{
			Key:               relation.Key,
			Subject:           relation.Subject,
			Inputs:            cloneCarriers(relation.Inputs),
			CandidateProvider: relation.CandidateProvider,
			Parent:            relation.Parent,
			Ordinal:           relation.Ordinal,
			Correspondences:   cloneCorrespondences(relation.Correspondences),
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
			Key:     reducer.Key,
			Inputs:  cloneReducerInputs(reducer.Inputs),
			Outputs: cloneReducerOutputs(reducer.Outputs),
		}
	}
	return clone
}

// Clone returns an independent declaration copy, preserving nil slices.
func (catalog Catalog) Clone() Catalog {
	clone := Catalog{
		Relations:       cloneRelations(catalog.Relations),
		Projections:     append([]Projection(nil), catalog.Projections...),
		Reducers:        cloneReducers(catalog.Reducers),
		CarryTransforms: append([]CarryTransform(nil), catalog.CarryTransforms...),
	}
	return clone
}

// HasMembers reports whether this catalog has crossed the migration ratchet
// from a legacy omitted catalog to an authored member declaration.
func (catalog Catalog) HasMembers() bool { return catalog.MemberCount() != 0 }

func (catalog Catalog) MemberCount() int {
	return len(catalog.Relations) + len(catalog.Projections) + len(catalog.Reducers) + len(catalog.CarryTransforms)
}

// Complete reports whether the catalog is empty (legacy absence) or a valid,
// closed member declaration catalog.
func (catalog Catalog) Complete() bool {
	relations := make(map[schema.Key]struct{}, len(catalog.Relations))
	keys := make(map[schema.Key]struct{}, catalog.MemberCount())
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
	}
	for _, reducer := range catalog.Reducers {
		if !reducer.Available() {
			return false
		}
		if _, duplicate := keys[reducer.Key]; duplicate {
			return false
		}
		keys[reducer.Key] = struct{}{}
	}
	for _, transform := range catalog.CarryTransforms {
		if !transform.Available() {
			return false
		}
		if _, duplicate := keys[transform.Key]; duplicate {
			return false
		}
		keys[transform.Key] = struct{}{}
	}
	return true
}

// Available reports whether this is a complete authored catalog. Legacy
// absence is not an available member catalog.
func (catalog Catalog) Available() bool { return catalog.HasMembers() && catalog.Complete() }

func (catalog Catalog) RelationCount() int       { return len(catalog.Relations) }
func (catalog Catalog) ProjectionCount() int     { return len(catalog.Projections) }
func (catalog Catalog) ReducerCount() int        { return len(catalog.Reducers) }
func (catalog Catalog) CarryTransformCount() int { return len(catalog.CarryTransforms) }

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
				Key:     reducer.Key,
				Inputs:  cloneReducerInputs(reducer.Inputs),
				Outputs: cloneReducerOutputs(reducer.Outputs),
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
	return nil
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
