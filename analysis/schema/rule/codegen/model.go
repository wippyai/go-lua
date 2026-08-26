// Package codegen owns the bounded, callback-free model used to take a
// sealed rule-plan catalog to a later direct-call emitter.
//
// The model is deliberately narrower than either input. It retains only the
// owner symbols and owner-qualified dense addresses needed by an emitter; it
// does not retain a schema, a runtime value, or a second digest. The digest on
// Model is the exact fence carried by ruleplan.Catalog.
package codegen

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// ProblemKind identifies the bounded verification gate that refused a model.
// These are generator diagnostics, not schema laws; the sealed schema and its
// rule-plan compiler remain the authority for declaration admission.
type ProblemKind uint8

const (
	ProblemNone ProblemKind = iota
	ProblemCatalog
	ProblemDigest
	ProblemAxis
	ProblemRule
	ProblemCandidate
	ProblemMember
	ProblemReducer
	ProblemForm
	ProblemOutputType
	ProblemSymbol
	ProblemCarry
)

// Failure is a stable, data-only explanation for a model verification refusal.
// Callers should use Kind and the bounded ordinals for policy decisions.
type Failure struct {
	Kind     ProblemKind
	Rule     uint32
	Axis     uint32
	Member   uint32
	Position uint32
	Message  string
}

func (failure Failure) Error() string { return failure.Message }

func failure(kind ProblemKind, rule, axis, member, position uint32, message string) error {
	return Failure{Kind: kind, Rule: rule, Axis: axis, Member: member, Position: position, Message: message}
}

// Axis is one owner metadata row resolved to the sealed dense axis ordinal.
// Metadata itself is not retained: the model carries only the axis identity
// needed by an emitter and the proof that the roster was resolved by position.
type Axis struct {
	ordinal uint32
	key     schema.Key
}

// Ordinal returns the sealed dense axis ordinal.
func (axis Axis) Ordinal() uint32 { return axis.ordinal }

// Key returns the authored axis key resolved from the sealed directory.
func (axis Axis) Key() schema.Key { return axis.key }

// Candidate is the relation used to enumerate one rule's dense candidates.
// Address is owner-qualified by ruleplan; Key and Subject are resolved from
// the metadata row for Address.Axis.
type Candidate struct {
	address  ruleplan.RelationAddr
	axis     schema.Key
	key      schema.Key
	subject  memberdefinition.GoType
	resolver memberdefinition.GoSymbol
	ordinal  memberdefinition.GoSymbol
	at       memberdefinition.GoSymbol
}

// Address returns the sealed owner-qualified candidate relation address.
func (candidate Candidate) Address() ruleplan.RelationAddr { return candidate.address }

// Axis returns the candidate relation's metadata owner axis.
func (candidate Candidate) Axis() schema.Key { return candidate.axis }

// Key returns the owner-issued relation key.
func (candidate Candidate) Key() schema.Key { return candidate.key }

// Subject returns the source type carried by the candidate relation.
func (candidate Candidate) Subject() memberdefinition.GoType { return candidate.subject }

// Resolver returns the owner direct-call symbol that resolves a mounted
// occurrence to one candidate value.
func (candidate Candidate) Resolver() memberdefinition.GoSymbol { return candidate.resolver }

// OrdinalSymbol returns the owner direct-call symbol that densifies a
// candidate value.
func (candidate Candidate) OrdinalSymbol() memberdefinition.GoSymbol { return candidate.ordinal }

// AtSymbol returns the owner direct-call symbol that reads one dense
// candidate value.
func (candidate Candidate) AtSymbol() memberdefinition.GoSymbol { return candidate.at }

// Join is one direct-call read input after dense plan verification. Relation
// and projection addresses remain explicit so a future emitter cannot rebuild
// keys or guess ordinals from the owner source.
type Join struct {
	Position          uint32
	Relation          ruleplan.RelationAddr
	RelationAxis      schema.Key
	Key               ruleplan.ProjectionAddr
	KeyAxis           schema.Key
	Predicate         ruleplan.ProjectionAddr
	PredicateAxis     schema.Key
	PredicatePresent  bool
	ReadAxis          uint32
	ReadAxisKey       schema.Key
	ReadForm          member.ReadForm
	Multiplicity      member.Multiplicity
	Cardinality       member.Multiplicity
	Denominator       ruleplan.DenominatorAddr
	ResultType        memberdefinition.GoType
	RelationResolver  memberdefinition.GoSymbol
	RelationOrdinal   memberdefinition.GoSymbol
	RelationAt        memberdefinition.GoSymbol
	RelationCandidate ruleplan.RelationAddr
	KeyAccessor       memberdefinition.GoSymbol
	PredicateAccessor memberdefinition.GoSymbol
	derivation        RelationDerivation
	derivationPresent bool
}

// RelationDerivation is the sealed direct-call descriptor for a dependent
// relation. StaticAxes are resolved against the composition roster in source
// order; Build, Count, and At remain source references for the emitter to
// call directly and compile-check against State and the relation Subject.
type RelationDerivation struct {
	state      memberdefinition.GoType
	build      memberdefinition.GoSymbol
	count      memberdefinition.GoSymbol
	at         memberdefinition.GoSymbol
	staticAxes []Axis
}

func (derivation RelationDerivation) State() memberdefinition.GoType   { return derivation.state }
func (derivation RelationDerivation) Build() memberdefinition.GoSymbol { return derivation.build }
func (derivation RelationDerivation) Count() memberdefinition.GoSymbol { return derivation.count }
func (derivation RelationDerivation) At() memberdefinition.GoSymbol    { return derivation.at }
func (derivation RelationDerivation) StaticAxisCount() int             { return len(derivation.staticAxes) }
func (derivation RelationDerivation) StaticAxisAt(index int) (Axis, bool) {
	if index < 0 || index >= len(derivation.staticAxes) {
		return Axis{}, false
	}
	return derivation.staticAxes[index], true
}

// Derivation returns the optional relation construction descriptor. A normal
// candidate-directory join returns false and must not be treated as a
// zero-value derivation.
func (join Join) Derivation() (RelationDerivation, bool) {
	if !join.derivationPresent {
		return RelationDerivation{}, false
	}
	return cloneRelationDerivation(join.derivation), true
}

// PredicateAddress returns the optional owner-qualified predicate projection
// address. The bool distinguishes an absent predicate from zero address.
func (join Join) PredicateAddress() (ruleplan.ProjectionAddr, bool) {
	return join.Predicate, join.PredicatePresent
}

// ReducerInput is one joined input in the implementation call signature.
type ReducerInput struct {
	Join         uint32
	Type         memberdefinition.GoType
	Form         member.ReadForm
	Multiplicity member.Multiplicity
	Tag          memberdefinition.GoType
	Tagged       bool
	// Route is the destination coordinate of an input whose join a routed
	// output writes through, resolved from that output's Destination
	// projection. Routed says the plan named this join as a route join.
	Route  memberdefinition.GoType
	Routed bool
}

// Output is one reducer output mapped to a sealed frame column. Type is the
// owner-resolved output carrier type used by a future direct call.
type Output struct {
	address             ruleplan.OutputAddr
	destination         ruleplan.ProjectionAddr
	destinationAxis     schema.Key
	outputAxis          schema.Key
	mode                program.OutputMode
	slot                uint32
	routeJoin           uint32
	routeJoinPresent    bool
	valueType           memberdefinition.GoType
	destinationAccessor memberdefinition.GoSymbol
}

// Address returns the sealed output frame address.
func (output Output) Address() ruleplan.OutputAddr { return output.address }

// DestinationAddress returns the owner-qualified destination projection.
func (output Output) DestinationAddress() ruleplan.ProjectionAddr { return output.destination }

// DestinationAxis returns the metadata owner of the destination projection.
func (output Output) DestinationAxis() schema.Key { return output.destinationAxis }

// OutputAxis returns the metadata owner of the published output column.
func (output Output) OutputAxis() schema.Key { return output.outputAxis }

// Mode returns the sealed output mode.
func (output Output) Mode() program.OutputMode { return output.mode }

// RouteJoin returns the selected join that supplies a bounded route address.
// Exact and structural outputs return false; a routed output cannot be built
// without this sealed correspondence.
func (output Output) RouteJoin() (uint32, bool) { return output.routeJoin, output.routeJoinPresent }

// Slot returns the reducer value slot.
func (output Output) Slot() uint32 { return output.slot }

// ValueType returns the resolved output carrier type.
func (output Output) ValueType() memberdefinition.GoType { return output.valueType }

// DestinationAccessor returns the owner direct-call symbol for the output's
// destination projection.
func (output Output) DestinationAccessor() memberdefinition.GoSymbol {
	return output.destinationAccessor
}

// ReducerOutcomeType is the one disposition every reducer concludes with. It
// is a constant of this package rather than a row an owner authors: a reducer
// that returned a boolean, or an enum of its own, would be stating a
// disposition its consumers - the fold executor, the activation relation, and
// the Delta path - have no member for. An emitter binds the second result of
// every direct reducer call to this type.
var ReducerOutcomeType = memberdefinition.GoType{
	PackagePath: "github.com/wippyai/go-lua/analysis/schema/structure",
	Name:        "ReductionOutcome",
}

// ReducerCall is the direct-call signature of one owner reducer. Candidate is
// optional and precedes Inputs when present. When CandidatePresent is false,
// CandidateConstant is true: the call has no candidate carrier and its
// candidate guard is the literal true rather than an inferred relation value.
//
// The call returns the declared output carriers followed by Outcome. Outcome
// is filled from ReducerOutcomeType by Build, so no owner declaration can
// substitute a second disposition vocabulary for it.
type ReducerCall struct {
	Address           ruleplan.ReducerAddr
	Axis              schema.Key
	Key               schema.Key
	Rule              schema.Key
	Implementation    memberdefinition.GoSymbol
	Candidate         memberdefinition.GoType
	CandidatePresent  bool
	CandidateConstant bool
	Inputs            []ReducerInput
	Outputs           []Output
	Outcome           memberdefinition.GoType
}

// OutcomeType returns the sealed disposition type this call's last result is
// bound to.
func (call ReducerCall) OutcomeType() memberdefinition.GoType { return call.Outcome }

// Carry is the direct-call descriptor for one optional whole-output carry.
// The transform address and owner key are retained together: the address is
// the sealed dense ordinal consumed by an emitter, while the key/axis pair
// fences that ordinal to the exact owner-issued member. Implementation is a
// symbol descriptor, never a callback or a runtime receipt. The direct call
// receives the retained candidate and input fact in that order (a receiver
// symbol uses candidate.Method(input)); the output is the declared output
// carrier plus the symbol's validity result.
type Carry struct {
	input            uint32
	mode             program.CarryMode
	transform        ruleplan.CarryTransformAddr
	transformPresent bool
	transformAxis    schema.Key
	transformKey     schema.Key
	candidate        memberdefinition.GoType
	inputType        memberdefinition.GoType
	outputType       memberdefinition.GoType
	implementation   memberdefinition.GoSymbol
}

// Input returns the explicit sealed carry input ordinal.
func (carry Carry) Input() uint32 { return carry.input }

// Mode returns the carry disposition.
func (carry Carry) Mode() program.CarryMode { return carry.mode }

// TransformAddress returns the dense transform member address. The bool is
// false for identity carries and therefore distinguishes a valid zero address
// from an absent transform.
func (carry Carry) TransformAddress() (ruleplan.CarryTransformAddr, bool) {
	return carry.transform, carry.transformPresent
}

// Transform is a compatibility spelling for TransformAddress.
func (carry Carry) Transform() (ruleplan.CarryTransformAddr, bool) {
	return carry.TransformAddress()
}

// TransformOrdinal returns the exact dense transform member ordinal.
func (carry Carry) TransformOrdinal() (uint32, bool) {
	if !carry.transformPresent {
		return 0, false
	}
	return carry.transform.Member, true
}

// TransformAxis returns the owner axis that issued the transform member.
func (carry Carry) TransformAxis() schema.Key { return carry.transformAxis }

// TransformKey returns the exact owner-issued transform member key.
func (carry Carry) TransformKey() schema.Key { return carry.transformKey }

// CandidateType returns the transform candidate relationship type.
func (carry Carry) CandidateType() memberdefinition.GoType { return carry.candidate }

// InputType returns the transformed carry input type.
func (carry Carry) InputType() memberdefinition.GoType { return carry.inputType }

// OutputType returns the transformed carry output type.
func (carry Carry) OutputType() memberdefinition.GoType { return carry.outputType }

// Implementation returns the direct owner symbol for the transform. Identity
// carries return the zero symbol.
func (carry Carry) Implementation() memberdefinition.GoSymbol { return carry.implementation }

// CandidateArgument returns the optional candidate type and whether the
// implementation receives that argument.
func (call ReducerCall) CandidateArgument() (memberdefinition.GoType, bool) {
	return call.Candidate, call.CandidatePresent
}

// CandidateGuard reports the bounded guard used by the generated call.
func (call ReducerCall) CandidateGuard() bool { return call.CandidateConstant }

// InputCount returns the number of joined input arguments.
func (call ReducerCall) InputCount() int { return len(call.Inputs) }

// InputAt returns one joined input argument.
func (call ReducerCall) InputAt(index int) (ReducerInput, bool) {
	if index < 0 || index >= len(call.Inputs) {
		return ReducerInput{}, false
	}
	return call.Inputs[index], true
}

// OutputCount returns the number of reducer output arguments.
func (call ReducerCall) OutputCount() int { return len(call.Outputs) }

// OutputAt returns one reducer output argument.
func (call ReducerCall) OutputAt(index int) (Output, bool) {
	if index < 0 || index >= len(call.Outputs) {
		return Output{}, false
	}
	return call.Outputs[index], true
}

// Rule is one present plan row lowered to a direct reducer call. Absent plans
// are not emitted and therefore do not appear in Model.
type Rule struct {
	ordinal      uint32
	candidate    Candidate
	joins        []Join
	reducer      ReducerCall
	outputs      []Output
	carry        Carry
	carryPresent bool
}

// Ordinal returns the sealed dense rule ordinal.
func (rule Rule) Ordinal() uint32 { return rule.ordinal }

// Candidate returns this rule's owner-qualified candidate relation.
func (rule Rule) Candidate() Candidate { return rule.candidate }

// JoinCount returns the number of ordered joins.
func (rule Rule) JoinCount() int { return len(rule.joins) }

// JoinAt returns one ordered join.
func (rule Rule) JoinAt(index int) (Join, bool) {
	if index < 0 || index >= len(rule.joins) {
		return Join{}, false
	}
	return rule.joins[index], true
}

// Reducer returns the direct-call reducer signature.
func (rule Rule) Reducer() ReducerCall { return cloneReducerCall(rule.reducer) }

// Carry returns the optional direct-call carry descriptor.
func (rule Rule) Carry() (Carry, bool) { return rule.carry, rule.carryPresent }

// OutputCount returns the number of reducer output columns.
func (rule Rule) OutputCount() int { return len(rule.outputs) }

// OutputAt returns one ordered output column.
func (rule Rule) OutputAt(index int) (Output, bool) {
	if index < 0 || index >= len(rule.outputs) {
		return Output{}, false
	}
	return rule.outputs[index], true
}

// Model is the immutable composition-wide direct-call model. Digest is copied
// verbatim from ruleplan.Catalog; no digest is derived here.
type Model struct {
	digest identity.ContentID
	axes   []Axis
	rules  []Rule
}

// Available reports whether the model carries a valid catalog fence and a
// complete metadata roster. A model with no present rules is valid when the
// sealed catalog contains only absent plans.
func (model Model) Available() bool {
	return model.digest.Available() && model.axes != nil && model.rules != nil
}

// Digest returns the exact sealed catalog digest.
func (model Model) Digest() identity.ContentID { return model.digest }

// AxisCount returns the number of metadata rows resolved into the model.
func (model Model) AxisCount() int { return len(model.axes) }

// AxisAt returns one resolved axis row by sealed ordinal order.
func (model Model) AxisAt(index int) (Axis, bool) {
	if index < 0 || index >= len(model.axes) {
		return Axis{}, false
	}
	return model.axes[index], true
}

// Count returns the number of present generated rules.
func (model Model) Count() int { return len(model.rules) }

// At returns one present rule by model position.
func (model Model) At(index int) (Rule, bool) {
	if index < 0 || index >= len(model.rules) {
		return Rule{}, false
	}
	return cloneRule(model.rules[index]), true
}

func newOutput(compiled ruleplan.Output, outputAxis, destinationAxis schema.Key, typ memberdefinition.GoType, destinationAccessor memberdefinition.GoSymbol) Output {
	return Output{
		address: compiled.Address, destination: compiled.Destination,
		destinationAxis: destinationAxis, outputAxis: outputAxis,
		mode: compiled.Mode, slot: compiled.Slot,
		routeJoin: compiled.RouteJoin, routeJoinPresent: compiled.RouteJoinPresent,
		valueType:           typ,
		destinationAccessor: destinationAccessor,
	}
}

func cloneReducerCall(call ReducerCall) ReducerCall {
	call.Inputs = append([]ReducerInput(nil), call.Inputs...)
	call.Outputs = append([]Output(nil), call.Outputs...)
	return call
}

func cloneRule(rule Rule) Rule {
	rule.joins = append([]Join(nil), rule.joins...)
	for index := range rule.joins {
		rule.joins[index].derivation = cloneRelationDerivation(rule.joins[index].derivation)
	}
	rule.reducer = cloneReducerCall(rule.reducer)
	rule.outputs = append([]Output(nil), rule.outputs...)
	return rule
}

func cloneRelationDerivation(derivation RelationDerivation) RelationDerivation {
	derivation.staticAxes = append([]Axis(nil), derivation.staticAxes...)
	return derivation
}

// The reducer call-shape vocabulary is the member definition layer's, not this
// package's. It is aliased here so an emitter reads one name set, and so the
// ordering rule has exactly one statement below the two derivations that need
// it: this one, which resolves carriers through dense plan addresses, and
// Definition.ReducerSignature, which resolves them by declared name.
type (
	ReducerArgumentRole = memberdefinition.ArgumentRole
	ReducerArgument     = memberdefinition.Argument
)

const (
	ReducerArgumentCandidate = memberdefinition.ArgumentCandidate
	ReducerArgumentRoute     = memberdefinition.ArgumentRoute
	ReducerArgumentTag       = memberdefinition.ArgumentTag
	ReducerArgumentFact      = memberdefinition.ArgumentFact
	ReducerArgumentVector    = memberdefinition.ArgumentVector
)

// SummaryVectorType is the view a WHOLE-VECTOR read delivers its cells
// through. It is a constant of this package for the same reason the outcome
// type is: the execution layer already materialized those cells in the order
// it sealed, and an owner that named a container of its own would be asking
// for a second copy of a vector the read boundary already owns. An emitter
// instantiates it at the input's declared fact carrier.
//
// It is what a Summary delivery is handed as, wherever it is consumed: the
// fold of a whole denominator and the Build of a relation derived over one see
// the same view, because a many-valued input is ONE argument. Which of the two
// views one input takes is definition.ManyValuedView's answer.
var SummaryVectorType = memberdefinition.GoType{
	PackagePath: "github.com/wippyai/go-lua/analysis/engine/execution",
	Name:        "SummaryVector",
}

// SelectionCellType is the view a SELECTED delivery is handed through: the
// cells a selection answered, each paired with the owner-issued tag that
// selection correlated it by. It is a distinct view from the vector above
// because it carries a distinct fact - a selection establishes a tag per cell
// and a whole-vector read establishes none, so a consumer of a selection reads
// what its own read proved rather than a view that drops it. It is delivered
// as a slice of this type, one entry per observed member.
var SelectionCellType = memberdefinition.GoType{
	PackagePath: "github.com/wippyai/go-lua/analysis/engine/execution",
	Name:        "SelectedCell",
}

// Arguments derives the complete parameter vector of this reducer's direct
// call. It is the one statement of the call shape: the emitter emits this
// vector, and the laws that fence the shape read this vector, so an emitter
// cannot drift from the contract by construction.
//
// The vector is carrier values only - the optional candidate carrier, then for
// each declared input its route coordinate when the input is routed, its tag
// carrier when the input is tagged, and the input itself: one fact carrier, or
// one view over that carrier when the sealed read is many-valued. Nothing else
// is ever a parameter. In particular the owner
// schema, the derived route plan, and the projections a fold consults are NOT
// passed: they are the sealed state of the installed Family that calls this
// reducer, bound once when the owner installs it and immutable thereafter.
// That is what keeps this signature from growing plumbing - a fold that needs
// more owner knowledge takes it from its Family, and the call shape is a
// function of the declaration alone.
//
// Which inputs are many-valued is read off the sealed plan's own multiplicity,
// never off a row an owner could set independently of the read it describes.
func (call ReducerCall) Arguments() []ReducerArgument {
	inputs := make([]memberdefinition.ArgumentInput, len(call.Inputs))
	for index, input := range call.Inputs {
		inputs[index] = memberdefinition.ArgumentInput{Route: input.Route, Routed: input.Routed, Tag: input.Tag, Tagged: input.Tagged, Fact: input.Type}
		if input.Multiplicity == member.MultiplicityMany {
			// Which view a many-valued delivery takes is the read's Form, and
			// that choice has one statement. This derivation resolves carriers
			// through plan addresses rather than by name, but it is the same
			// call shape, so it asks the same question rather than answering
			// it a second way.
			view, slice, viewOK := memberdefinition.ManyValuedView(input.Form, SelectionCellType, SummaryVectorType)
			if !viewOK {
				return nil
			}
			inputs[index].Vector, inputs[index].Slice, inputs[index].Many = view, slice, true
		}
	}
	return memberdefinition.ComposeArguments(call.Candidate, call.CandidatePresent, inputs)
}

// Results derives the complete result vector: the declared output carriers in
// slot order, followed by the sealed disposition. A reducer concludes exactly
// one disposition and it is always the last result.
func (call ReducerCall) Results() []memberdefinition.GoType {
	results := make([]memberdefinition.GoType, 0, len(call.Outputs)+1)
	for _, output := range call.Outputs {
		results = append(results, output.ValueType())
	}
	return append(results, call.Outcome)
}
