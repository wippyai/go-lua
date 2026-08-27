// Package issuance owns the sealed, domain-neutral machine which constructs
// rule occurrences from immutable Program rows. Domain packages contribute
// declarations; the compiler interprets only this vocabulary and its opcodes.
package issuance

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/schema/carrier"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	LawEntryShape schema.LawID = seal.SurfaceLawFloor + iota
	LawEntryIdentity
	LawOrdinalUnique
	LawOrdinalDense
	LawReferenceResolves
	LawReferenceKind
	LawProgramShape
	LawRelationAcyclic
	LawStageAcyclic
	LawFramingUnique
)

type Kind uint8

const (
	KindInvalid Kind = iota
	KindType
	KindRowSpace
	KindField
	KindRelation
	KindFamily
	KindOutput
	KindRequirement
	KindForm
	KindInput
	KindStage
)

func (kind Kind) valid() bool { return kind > KindInvalid && kind <= KindStage }

// ValueType is structural. DataType.Name supplies nominal meaning, preventing
// unrelated identities and enums from being compared or published together.
type ValueType uint8

const (
	ValueInvalid ValueType = iota
	ValueBool
	ValueUint
	ValueIdentity
	ValueRow
	ValueRange
	ValuePointRange
	ValueInputRange
	ValueStageRequestRange
	ValueRoute
	ValueEmissionRange
)

func (value ValueType) valid() bool { return value > ValueInvalid && value <= ValueEmissionRange }

type Cardinality uint8

const (
	CardinalityInvalid Cardinality = iota
	CardinalityOptional
	CardinalityOne
	CardinalityMany
)

func (cardinality Cardinality) valid() bool {
	return cardinality > CardinalityInvalid && cardinality <= CardinalityMany
}

// DataType is the complete nominal carrier of a register or declared output.
type DataType struct {
	Value       ValueType
	Name        schema.Key
	Space       schema.Key
	Relation    schema.Key
	Cardinality Cardinality
}

func (typ DataType) available() bool {
	if !typ.Value.valid() || !typ.Cardinality.valid() {
		return false
	}
	switch typ.Value {
	case ValueBool:
		return !typ.Name.Available() && !typ.Space.Available() && !typ.Relation.Available()
	case ValueUint, ValueIdentity:
		return typ.Name.Available() && !typ.Space.Available() && !typ.Relation.Available()
	case ValueRow:
		return !typ.Name.Available() && typ.Space.Available() && !typ.Relation.Available()
	case ValueRange:
		return !typ.Name.Available() && typ.Space.Available() && typ.Relation.Available()
	case ValuePointRange, ValueInputRange, ValueStageRequestRange, ValueRoute, ValueEmissionRange:
		return typ.Name.Available() && !typ.Space.Available() && !typ.Relation.Available()
	default:
		return false
	}
}

func BoolType() DataType {
	return DataType{Value: ValueBool, Cardinality: CardinalityOne}
}

func UintType(name schema.Key) DataType {
	return DataType{Value: ValueUint, Name: name, Cardinality: CardinalityOne}
}

func IdentityType(name schema.Key) DataType {
	return DataType{Value: ValueIdentity, Name: name, Cardinality: CardinalityOne}
}

type EmptyPolicy uint8

const (
	EmptyInvalid EmptyPolicy = iota
	EmptyRefuse
	EmptyEmitNone
)

func (policy EmptyPolicy) valid() bool {
	return policy == EmptyRefuse || policy == EmptyEmitNone
}

// InputKind is a closed projection from canonical occurrence geometry.
type InputKind uint8

const (
	InputInvalid InputKind = iota
	InputNone
	InputEntry
	InputFinish
	InputPredecessor
)

func (kind InputKind) valid() bool { return kind > InputInvalid && kind <= InputPredecessor }

// InputSource identifies the one owner proof from which an input point may be
// taken. Previous is an explicit predecessor in the sealed stage schedule;
// unlike a missing source it never guesses or fabricates a point.
type InputSource uint8

const (
	InputSourceInvalid InputSource = iota
	InputSourceNone
	InputSourceRelation
	InputSourceStage
	InputSourcePrevious
	// InputSourceRoute takes the input point from the route that reaches the
	// stage rather than from its position in the linear stage chain. A stage
	// standing on one route reads the state that route carries; taking the
	// previous stage instead would read a point the route itself feeds, which
	// is a cycle rather than a predecessor.
	InputSourceRoute
)

func (source InputSource) valid() bool {
	return source > InputSourceInvalid && source <= InputSourceRoute
}

// InputSelection is the complete point-selection law for an input source.
// The scheduler never infers this policy from an InputKind or stage name.
type InputSelection uint8

const (
	InputSelectionInvalid InputSelection = iota
	InputSelectionNone
	InputSelectionDriver
	InputSelectionOnly
	InputSelectionStage
	InputSelectionPrevious
	// InputSelectionRoute resolves the one point the declared route departs
	// from. The host owns the route-to-source mapping; the schedule never
	// falls back to the chain when that mapping has no answer.
	InputSelectionRoute
)

func (selection InputSelection) valid() bool {
	return selection > InputSelectionInvalid && selection <= InputSelectionRoute
}

type StageTransport uint8

const (
	StageTransportInvalid StageTransport = iota
	StageTransportAll
	StageTransportAllExceptTargetWrites
	StageTransportWritesOfStages
	StageTransportAllExceptWritesOfStages
)

func (transport StageTransport) valid() bool {
	return transport > StageTransportInvalid && transport <= StageTransportAllExceptWritesOfStages
}

type StageConstructor uint8

const (
	StageConstructorInvalid StageConstructor = iota
	StageConstructorPassthrough
	StageConstructorFramed
)

func (constructor StageConstructor) valid() bool {
	return constructor == StageConstructorPassthrough || constructor == StageConstructorFramed
}

type StageEdgeSource uint8

const (
	StageEdgeSourceInvalid StageEdgeSource = iota
	StageEdgeSourceBase
	StageEdgeSourcePrevious
	StageEdgeSourceStage
	StageEdgeSourceBeforeStage
	// StageEdgeSourceRoute transfers into the stage from the point its route
	// departs from, placing the stage on that route instead of in the linear
	// chain at its base. It is the transfer half of InputSourceRoute, and only
	// a stage that carries a route in its identity may declare it.
	StageEdgeSourceRoute
)

func (source StageEdgeSource) valid() bool {
	return source > StageEdgeSourceInvalid && source <= StageEdgeSourceRoute
}

// StageEdge states one explicit transport source. Previous names the active
// node immediately before the target; BeforeStage names the active node
// immediately before the referenced stage group.
type StageEdge struct {
	Source       StageEdgeSource
	Stage        schema.Key
	Transport    StageTransport
	WriterStages []schema.Key
	Framing      string
}

func cloneEdges(edges []StageEdge) []StageEdge {
	cloned := make([]StageEdge, len(edges))
	for index, edge := range edges {
		cloned[index] = edge
		cloned[index].WriterStages = append([]schema.Key(nil), edge.WriterStages...)
	}
	return cloned
}

// Opcode is the only switch vocabulary permitted in the generic executor.
// Relation predicates have implicit Current and Item binders; those opcodes
// are rejected in every other phase, giving iteration an exact lexical scope.
type Opcode uint8

const (
	OpInvalid Opcode = iota
	OpCurrent
	OpItem
	OpItemIndex
	OpLiteral
	OpRead
	OpFollow
	OpAt
	OpCount
	OpEqual
	OpEqualIfPresent
	OpGreater
	OpLess
	OpAnd
	OpOr
	OpNot
	OpPresent
	OpExactlyOne
	OpOnly
	OpRequirePresent
	OpSelection
	OpRuleKey
	OpWritesKey
	OpProjectPoints
	OpPoint
	OpRoute
	OpInput
	OpRequestStage
	OpEmit
)

func (opcode Opcode) valid() bool { return opcode > OpInvalid && opcode <= OpEmit }

// Instruction is immutable SSA. Args are ordered operands; unused slots are
// zero. Ref names the declaration consumed by the opcode.
type Instruction struct {
	Op      Opcode
	Out     uint16
	Args    [6]uint16
	Ref     schema.Key
	Aux     schema.Key
	Type    DataType
	Literal uint64
}

type Program []Instruction

func cloneProgram(program Program) Program { return append(Program(nil), program...) }

// registerWidth is the sealed height of one program's register file. The
// machine numbers registers densely from one and register zero is its "no
// operand" spelling, so the file a program needs is exactly its highest
// output ordinal plus that reserved slot.
func registerWidth(program Program) int {
	width := 1
	for _, instruction := range program {
		if int(instruction.Out)+1 > width {
			width = int(instruction.Out) + 1
		}
	}
	return width
}

// OutputBinding authenticates a requirement selection against a separately
// declared output ABI. Proof must be the requirement admission register.
type OutputBinding struct {
	Output   schema.Key
	Register uint16
	Proof    uint16
}

type JoinField struct {
	Source  schema.Key
	Target  schema.Key
	Missing JoinMissing
}

// JoinMissing makes optional-key semantics explicit. NoEdge means an absent
// source or target key contributes no relation edge; it never fabricates a
// row, Unknown value, or refusal.
type JoinMissing uint8

const (
	JoinMissingInvalid JoinMissing = iota
	JoinMissingNoEdge
)

type Spec struct {
	Key     schema.Key
	Kind    Kind
	Ordinal uint16
	// Authorities are raw owner-local carrier declarations. New copies and
	// issues them under the entry's own issuance reference; callers must never
	// supply an already-issued authority here.
	Authorities  []carrier.Authority
	Space        schema.Key
	Target       schema.Key
	Type         DataType
	Cardinality  Cardinality
	Joins        []JoinField
	Program      Program
	Result       uint16
	Outputs      []OutputBinding
	Requires     []schema.Key
	Subject      schema.Key
	Emissions    []uint16
	Input        InputKind
	InputSource  InputSource
	Selection    InputSelection
	Source       schema.Key
	Parameters   []DataType
	Base         uint16
	Identity     []uint16
	Constructor  StageConstructor
	Order        uint16
	Node         uint16
	Dependencies []uint16
	Predecessors []schema.Key
	Edges        []StageEdge
	Framing      string
	Native       bool
	// InputCount is the ordered number of input-range operands trailing a
	// stage request's structural parameters. The request instruction has six
	// argument cells, so the width is sealed rather than inferred at runtime.
	InputCount uint8
	Empty      EmptyPolicy
}

type Entry struct {
	key           schema.Key
	id            schema.EntryID
	kind          Kind
	ordinal       uint16
	authorities   []carrier.Authority
	space         schema.Key
	target        schema.Key
	typ           DataType
	cardinality   Cardinality
	joins         []JoinField
	program       Program
	result        uint16
	outputs       []OutputBinding
	requires      []schema.Key
	subject       schema.Key
	emissions     []uint16
	input         InputKind
	inputSource   InputSource
	selection     InputSelection
	source        schema.Key
	parameters    []DataType
	base          uint16
	identity      []uint16
	constructor   StageConstructor
	order         uint16
	node          uint16
	dependencies  []uint16
	predecessors  []schema.Key
	edges         []StageEdge
	framing       string
	native        bool
	inputCount    uint8
	empty         EmptyPolicy
	registerWidth int
}

func New(spec Spec) (*Entry, bool) {
	inputCount := spec.InputCount
	authorities, authoritiesOK := issueAuthorities(spec.Key, spec.Kind, spec.Authorities)
	if !authoritiesOK {
		return nil, false
	}
	entry := &Entry{
		key: spec.Key, id: schema.NewEntryID(schema.SurfaceKindIssuance, spec.Key),
		kind: spec.Kind, ordinal: spec.Ordinal, authorities: authorities, space: spec.Space, target: spec.Target,
		typ: spec.Type, cardinality: spec.Cardinality,
		joins: append([]JoinField(nil), spec.Joins...), program: cloneProgram(spec.Program),
		result: spec.Result, outputs: append([]OutputBinding(nil), spec.Outputs...),
		requires: append([]schema.Key(nil), spec.Requires...), subject: spec.Subject,
		emissions: append([]uint16(nil), spec.Emissions...), input: spec.Input,
		inputSource: spec.InputSource, selection: spec.Selection, source: spec.Source,
		parameters: append([]DataType(nil), spec.Parameters...), base: spec.Base,
		identity:    append([]uint16(nil), spec.Identity...),
		constructor: spec.Constructor, order: spec.Order, node: spec.Node,
		dependencies: append([]uint16(nil), spec.Dependencies...),
		predecessors: append([]schema.Key(nil), spec.Predecessors...), edges: cloneEdges(spec.Edges),
		framing: spec.Framing, native: spec.Native, inputCount: inputCount, empty: spec.Empty,
		registerWidth: registerWidth(spec.Program),
	}
	return entry, entry.EntryAvailable() && entry.declarationComplete()
}

func (entry *Entry) Key() schema.Key    { return entry.key }
func (entry *Entry) ID() schema.EntryID { return entry.id }
func (entry *Entry) Kind() Kind         { return entry.kind }
func (entry *Entry) Ordinal() uint16    { return entry.ordinal }
func (entry *Entry) AuthorityCount() int {
	if entry == nil {
		return 0
	}
	return len(entry.authorities)
}
func (entry *Entry) AuthorityAt(index int) (carrier.Authority, bool) {
	if entry == nil || index < 0 || index >= len(entry.authorities) {
		return carrier.Authority{}, false
	}
	return entry.authorities[index], true
}

// CarrierAuthority resolves one owner-issued local authority by its carrier
// key. The shared name is the schema-level authority-provider contract used
// by cross-surface validation.
// The value is returned by copy, so callers cannot mutate sealed storage.
func (entry *Entry) CarrierAuthority(key carrier.Key) (carrier.Authority, bool) {
	if entry == nil || !key.Available() {
		return carrier.Authority{}, false
	}
	var result carrier.Authority
	found := 0
	for _, authority := range entry.authorities {
		if authority.Carrier == key {
			result = authority
			found++
		}
	}
	if found != 1 {
		return carrier.Authority{}, false
	}
	return result, result.Available() && result.Issued()
}
func (entry *Entry) Space() schema.Key              { return entry.space }
func (entry *Entry) Target() schema.Key             { return entry.target }
func (entry *Entry) Type() DataType                 { return entry.typ }
func (entry *Entry) Cardinality() Cardinality       { return entry.cardinality }
func (entry *Entry) Result() uint16                 { return entry.result }
func (entry *Entry) Input() InputKind               { return entry.input }
func (entry *Entry) InputSource() InputSource       { return entry.inputSource }
func (entry *Entry) InputSelection() InputSelection { return entry.selection }
func (entry *Entry) Source() schema.Key             { return entry.source }
func (entry *Entry) Framing() string                { return entry.framing }
func (entry *Entry) Native() bool                   { return entry.native }
func (entry *Entry) InputCount() uint8              { return entry.inputCount }
func (entry *Entry) EmptyPolicy() EmptyPolicy       { return entry.empty }
func (entry *Entry) Joins() []JoinField             { return append([]JoinField(nil), entry.joins...) }
func (entry *Entry) ProgramLen() int                { return len(entry.program) }
func (entry *Entry) RegisterWidth() int             { return entry.registerWidth }
func (entry *Entry) Outputs() []OutputBinding       { return append([]OutputBinding(nil), entry.outputs...) }
func (entry *Entry) Requires() []schema.Key         { return append([]schema.Key(nil), entry.requires...) }
func (entry *Entry) Subject() schema.Key            { return entry.subject }
func (entry *Entry) Emissions() []uint16            { return append([]uint16(nil), entry.emissions...) }
func (entry *Entry) Parameters() []DataType         { return append([]DataType(nil), entry.parameters...) }
func (entry *Entry) BaseParameter() uint16          { return entry.base }
func (entry *Entry) IdentityParameters() []uint16 {
	return append([]uint16(nil), entry.identity...)
}

// InstructionAt reads one sealed instruction in place. An Instruction is an
// immutable value, so the sealed sequence is read by ordinal rather than
// handed out as a slice: an interpreter walks the program once per row it
// evaluates, and copying the whole program at each of those walks is the one
// cost this accessor exists to remove.
func (entry *Entry) InstructionAt(index int) (Instruction, bool) {
	if entry == nil || index < 0 || index >= len(entry.program) {
		return Instruction{}, false
	}
	return entry.program[index], true
}
func (entry *Entry) Constructor() StageConstructor {
	return entry.constructor
}
func (entry *Entry) Order() uint16         { return entry.order }
func (entry *Entry) NodeParameter() uint16 { return entry.node }
func (entry *Entry) DependencyParameters() []uint16 {
	return append([]uint16(nil), entry.dependencies...)
}
func (entry *Entry) Predecessors() []schema.Key {
	return append([]schema.Key(nil), entry.predecessors...)
}
func (entry *Entry) Edges() []StageEdge { return cloneEdges(entry.edges) }

func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available() &&
		entry.kind.valid() && entry.ordinal != 0 && entry.authoritiesComplete()
}

// issueAuthorities crosses the only authority issuance boundary owned by an
// issuance Entry. It intentionally accepts raw values only: an already-issued
// authority, a duplicate carrier key, or a duplicate resulting identity is a
// malformed declaration rather than something to repair or look up.
func issueAuthorities(key schema.Key, kind Kind, raw []carrier.Authority) ([]carrier.Authority, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	if kind != KindType || !key.Available() {
		return nil, false
	}
	owner := schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: key}
	issued := append([]carrier.Authority(nil), raw...)
	seenCarriers := make(map[carrier.Key]struct{}, len(issued))
	seenIDs := make(map[schema.EntryID]struct{}, len(issued))
	for index, authority := range issued {
		if authority.ID().Available() || !authority.Available() {
			return nil, false
		}
		if _, duplicate := seenCarriers[authority.Carrier]; duplicate {
			return nil, false
		}
		seenCarriers[authority.Carrier] = struct{}{}
		issuedAuthority, ok := carrier.Issue(owner, authority)
		if !ok || !issuedAuthority.Issued() {
			return nil, false
		}
		if _, duplicate := seenIDs[issuedAuthority.ID()]; duplicate {
			return nil, false
		}
		seenIDs[issuedAuthority.ID()] = struct{}{}
		issued[index] = issuedAuthority
	}
	return issued, true
}

// authoritiesComplete authenticates the private authority identity against
// this Entry's owner. Reconstructing the raw value and asking carrier.Issue
// for the expected identity catches copied authorities from another owner,
// field drift, duplicate keys, duplicate IDs, and raw/unissued mutations.
func (entry *Entry) authoritiesComplete() bool {
	if entry == nil {
		return false
	}
	if len(entry.authorities) != 0 && entry.kind != KindType {
		return false
	}
	if len(entry.authorities) == 0 {
		return true
	}
	owner := schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: entry.key}
	seenCarriers := make(map[carrier.Key]struct{}, len(entry.authorities))
	seenIDs := make(map[schema.EntryID]struct{}, len(entry.authorities))
	for _, authority := range entry.authorities {
		if !authority.Available() || !authority.Issued() {
			return false
		}
		if _, duplicate := seenCarriers[authority.Carrier]; duplicate {
			return false
		}
		seenCarriers[authority.Carrier] = struct{}{}
		if _, duplicate := seenIDs[authority.ID()]; duplicate {
			return false
		}
		seenIDs[authority.ID()] = struct{}{}
		expected, ok := carrier.Issue(owner, carrier.Authority{Carrier: authority.Carrier, Capability: authority.Capability})
		if !ok || authority.ID() != expected.ID() {
			return false
		}
	}
	return true
}

func (entry *Entry) declarationComplete() bool {
	if !entry.EntryAvailable() {
		return false
	}
	program := len(entry.program) != 0
	switch entry.kind {
	case KindType, KindRowSpace:
		return entry.typ == (DataType{}) && entry.emptyState()
	case KindField:
		return entry.space.Available() && scalar(entry.typ) &&
			(entry.cardinality == CardinalityOptional || entry.cardinality == CardinalityOne) &&
			!program && entry.fieldStateEmpty()
	case KindRelation:
		return entry.space.Available() && entry.target.Available() &&
			entry.cardinality.valid() && len(entry.joins) != 0 && program &&
			entry.result != 0 && entry.relationStateEmpty()
	case KindFamily:
		return entry.space.Available() && program && entry.result != 0 &&
			entry.requirementStateEmpty()
	case KindOutput:
		return entry.typ.available() && !program && entry.outputStateEmpty()
	case KindRequirement:
		return entry.space.Available() && program && entry.result != 0 &&
			validOutputs(entry.outputs) && entry.requirementStateEmpty()
	case KindForm:
		return program && len(entry.emissions) != 0 && entry.empty.valid() &&
			entry.subject.Available() && validKeys(entry.requires) &&
			containsKey(entry.requires, entry.subject) && entry.formStateEmpty()
	case KindInput:
		return entry.input.valid() && entry.inputSource.valid() && entry.selection.valid() &&
			validInputSource(entry.input, entry.inputSource, entry.selection, entry.source) &&
			!program && entry.inputStateEmpty()
	case KindStage:
		return entry.constructor.valid() && validStageFraming(entry.constructor, entry.framing) &&
			validTypes(entry.parameters) && entry.order != 0 &&
			validStageSchedule(entry.parameters, entry.base, entry.identity, entry.node, entry.dependencies, entry.predecessors) &&
			entry.inputCount <= uint8(len(Instruction{}.Args)) && validEdges(entry.edges) && (!entry.native || entry.inputCount != 0) && !program && entry.stageStateEmpty()
	default:
		return false
	}
}

func (entry *Entry) emptyState() bool {
	return !entry.space.Available() && !entry.target.Available() &&
		entry.cardinality == CardinalityInvalid && len(entry.joins) == 0 &&
		len(entry.program) == 0 && entry.result == 0 && len(entry.outputs) == 0 && len(entry.requires) == 0 &&
		len(entry.emissions) == 0 && entry.input == InputInvalid &&
		len(entry.parameters) == 0 && entry.base == 0 && len(entry.identity) == 0 && len(entry.edges) == 0 &&
		entry.constructor == StageConstructorInvalid && entry.framing == "" && !entry.native && entry.inputCount == 0 && entry.empty == EmptyInvalid &&
		entry.nonFormInputStageStateEmpty()
}

func (entry *Entry) fieldStateEmpty() bool {
	return !entry.target.Available() && len(entry.joins) == 0 && entry.result == 0 &&
		len(entry.outputs) == 0 && len(entry.requires) == 0 && len(entry.emissions) == 0 &&
		entry.input == InputInvalid && len(entry.parameters) == 0 && entry.base == 0 && len(entry.identity) == 0 &&
		len(entry.edges) == 0 && entry.constructor == StageConstructorInvalid &&
		entry.framing == "" && !entry.native && entry.inputCount == 0 && entry.empty == EmptyInvalid && entry.nonFormInputStageStateEmpty()
}

func (entry *Entry) relationStateEmpty() bool {
	return entry.typ == (DataType{}) && len(entry.outputs) == 0 && len(entry.requires) == 0 &&
		len(entry.emissions) == 0 && entry.input == InputInvalid &&
		len(entry.parameters) == 0 && entry.base == 0 && len(entry.identity) == 0 && len(entry.edges) == 0 &&
		entry.constructor == StageConstructorInvalid && entry.framing == "" && !entry.native && entry.inputCount == 0 && entry.empty == EmptyInvalid &&
		entry.nonFormInputStageStateEmpty()
}

func (entry *Entry) outputStateEmpty() bool {
	return !entry.space.Available() && !entry.target.Available() &&
		entry.cardinality == CardinalityInvalid && len(entry.joins) == 0 &&
		entry.result == 0 && len(entry.outputs) == 0 && len(entry.requires) == 0 && len(entry.emissions) == 0 &&
		entry.input == InputInvalid && len(entry.parameters) == 0 && entry.base == 0 && len(entry.identity) == 0 &&
		len(entry.edges) == 0 && entry.constructor == StageConstructorInvalid &&
		entry.framing == "" && !entry.native && entry.inputCount == 0 && entry.empty == EmptyInvalid && entry.nonFormInputStageStateEmpty()
}

func (entry *Entry) requirementStateEmpty() bool {
	return !entry.target.Available() && entry.typ == (DataType{}) &&
		entry.cardinality == CardinalityInvalid && len(entry.joins) == 0 &&
		len(entry.requires) == 0 && len(entry.emissions) == 0 && entry.input == InputInvalid &&
		len(entry.parameters) == 0 && entry.base == 0 && len(entry.identity) == 0 && len(entry.edges) == 0 &&
		entry.constructor == StageConstructorInvalid && entry.framing == "" && !entry.native && entry.inputCount == 0 && entry.empty == EmptyInvalid &&
		entry.nonFormInputStageStateEmpty()
}

func (entry *Entry) formStateEmpty() bool {
	return !entry.space.Available() && !entry.target.Available() &&
		entry.typ == (DataType{}) && entry.cardinality == CardinalityInvalid &&
		len(entry.joins) == 0 && entry.result == 0 && len(entry.outputs) == 0 &&
		entry.input == InputInvalid && entry.inputSource == InputSourceInvalid && entry.selection == InputSelectionInvalid && !entry.source.Available() &&
		len(entry.parameters) == 0 && entry.base == 0 && len(entry.identity) == 0 && len(entry.edges) == 0 &&
		entry.constructor == StageConstructorInvalid && entry.order == 0 && entry.node == 0 && !entry.native && entry.inputCount == 0 &&
		len(entry.dependencies) == 0 && len(entry.predecessors) == 0 && entry.framing == ""
}

func (entry *Entry) inputStateEmpty() bool {
	return !entry.space.Available() && !entry.target.Available() &&
		entry.typ == (DataType{}) && entry.cardinality == CardinalityInvalid &&
		len(entry.joins) == 0 && entry.result == 0 && len(entry.outputs) == 0 && len(entry.requires) == 0 &&
		len(entry.emissions) == 0 && !entry.subject.Available() && len(entry.parameters) == 0 && entry.base == 0 && len(entry.identity) == 0 &&
		len(entry.edges) == 0 && entry.constructor == StageConstructorInvalid &&
		entry.order == 0 && entry.node == 0 && len(entry.dependencies) == 0 && len(entry.predecessors) == 0 &&
		entry.framing == "" && !entry.native && entry.inputCount == 0 && entry.empty == EmptyInvalid
}

func (entry *Entry) stageStateEmpty() bool {
	return !entry.space.Available() && !entry.target.Available() &&
		entry.typ == (DataType{}) && entry.cardinality == CardinalityInvalid &&
		len(entry.joins) == 0 && entry.result == 0 && len(entry.outputs) == 0 && len(entry.requires) == 0 &&
		len(entry.emissions) == 0 && !entry.subject.Available() && entry.input == InputInvalid &&
		entry.inputSource == InputSourceInvalid && entry.selection == InputSelectionInvalid && !entry.source.Available() &&
		entry.empty == EmptyInvalid
}

func (entry *Entry) nonFormInputStageStateEmpty() bool {
	return !entry.subject.Available() && entry.inputSource == InputSourceInvalid && entry.selection == InputSelectionInvalid &&
		!entry.source.Available() && entry.order == 0 && entry.node == 0 &&
		len(entry.dependencies) == 0 && len(entry.predecessors) == 0
}

func validStageFraming(constructor StageConstructor, framing string) bool {
	return constructor == StageConstructorPassthrough && framing == "" ||
		constructor == StageConstructorFramed && framing != ""
}

func validInputSource(kind InputKind, source InputSource, selection InputSelection, reference schema.Key) bool {
	switch source {
	case InputSourceNone:
		return kind == InputNone && selection == InputSelectionNone && !reference.Available()
	case InputSourceRelation:
		return kind != InputNone && (selection == InputSelectionDriver || selection == InputSelectionOnly) && reference.Available()
	case InputSourceStage:
		return kind == InputFinish && selection == InputSelectionStage && reference.Available()
	case InputSourcePrevious:
		return kind == InputFinish && selection == InputSelectionPrevious && !reference.Available()
	case InputSourceRoute:
		return kind == InputPredecessor && selection == InputSelectionRoute && !reference.Available()
	default:
		return false
	}
}

func validStageSchedule(parameters []DataType, base uint16, identity []uint16, node uint16, dependencies []uint16, predecessors []schema.Key) bool {
	if !validKeys(predecessors) || base == 0 || int(base) > len(parameters) {
		return false
	}
	baseType := parameters[base-1]
	if baseType.Value != ValuePointRange || baseType.Name != TypePoint ||
		(baseType.Cardinality != CardinalityOne && baseType.Cardinality != CardinalityMany) {
		return false
	}
	seenIdentity := make(map[uint16]struct{}, len(identity))
	baseIncluded := false
	nodeIncluded := node == 0
	for _, parameter := range identity {
		if parameter == 0 || int(parameter) > len(parameters) {
			return false
		}
		if _, duplicate := seenIdentity[parameter]; duplicate {
			return false
		}
		if !validStageIdentityType(parameters[parameter-1], parameter == base) {
			return false
		}
		seenIdentity[parameter] = struct{}{}
		baseIncluded = baseIncluded || parameter == base
		nodeIncluded = nodeIncluded || parameter == node
	}
	if len(identity) == 0 || !baseIncluded || !nodeIncluded {
		return false
	}
	if node == 0 {
		return len(dependencies) == 0
	}
	if int(node) > len(parameters) || parameters[node-1].Value != ValueIdentity {
		return false
	}
	seen := make(map[uint16]struct{}, len(dependencies))
	for _, dependency := range dependencies {
		if dependency == 0 || int(dependency) > len(parameters) ||
			parameters[dependency-1] != parameters[node-1] || dependency == node {
			return false
		}
		if _, duplicate := seen[dependency]; duplicate {
			return false
		}
		seen[dependency] = struct{}{}
	}
	return len(dependencies) != 0
}

func validStageIdentityType(typ DataType, base bool) bool {
	if typ.Cardinality != CardinalityOne && !(base && typ.Cardinality == CardinalityMany) {
		return false
	}
	switch typ.Value {
	case ValueBool, ValueUint, ValueIdentity:
		return true
	case ValuePointRange:
		return typ.Name == TypePoint
	default:
		return false
	}
}

func scalar(typ DataType) bool {
	return typ.Cardinality == CardinalityOne &&
		(typ.Value == ValueBool || typ.Value == ValueUint || typ.Value == ValueIdentity) &&
		typ.available()
}

func validTypes(types []DataType) bool {
	for _, typ := range types {
		if !typ.available() {
			return false
		}
	}
	return true
}

func validOutputs(outputs []OutputBinding) bool {
	seen := make(map[schema.Key]struct{}, len(outputs))
	for _, output := range outputs {
		if !output.Output.Available() || output.Register == 0 || output.Proof == 0 {
			return false
		}
		if _, duplicate := seen[output.Output]; duplicate {
			return false
		}
		seen[output.Output] = struct{}{}
	}
	return true
}

func validKeys(keys []schema.Key) bool {
	seen := make(map[schema.Key]struct{}, len(keys))
	for _, key := range keys {
		if !key.Available() {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}

func validEdges(edges []StageEdge) bool {
	type edgeIdentity struct {
		source StageEdgeSource
		stage  schema.Key
	}
	seen := make(map[edgeIdentity]struct{}, len(edges))
	for _, edge := range edges {
		stageRequired := edge.Source == StageEdgeSourceStage || edge.Source == StageEdgeSourceBeforeStage
		if !edge.Source.valid() || edge.Stage.Available() != stageRequired || !edge.Transport.valid() || edge.Framing == "" {
			return false
		}
		stageDependent := edge.Transport == StageTransportWritesOfStages || edge.Transport == StageTransportAllExceptWritesOfStages
		if stageDependent != (len(edge.WriterStages) != 0) ||
			!validKeys(edge.WriterStages) {
			return false
		}
		identity := edgeIdentity{source: edge.Source, stage: edge.Stage}
		if _, duplicate := seen[identity]; duplicate {
			return false
		}
		seen[identity] = struct{}{}
	}
	return true
}

func writeDataType(content *framing.Writer, typ DataType) error {
	if err := content.Uint(uint64(typ.Value)); err != nil {
		return err
	}
	if err := content.String(string(typ.Name)); err != nil {
		return err
	}
	if err := content.String(string(typ.Space)); err != nil {
		return err
	}
	if err := content.String(string(typ.Relation)); err != nil {
		return err
	}
	return content.Uint(uint64(typ.Cardinality))
}

func (entry *Entry) EntryContent(content *framing.Writer) error {
	if entry == nil || !entry.EntryAvailable() {
		return errors.New("issuance: incomplete entry authority")
	}
	if err := content.Uint(uint64(entry.kind)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.ordinal)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.authorities))); err != nil {
		return err
	}
	for _, authority := range entry.authorities {
		id := authority.ID()
		if err := content.Bytes(id[:]); err != nil {
			return err
		}
		if err := content.String(string(authority.Carrier)); err != nil {
			return err
		}
		if err := content.Uint(uint64(authority.Capability)); err != nil {
			return err
		}
	}
	if err := content.String(string(entry.space)); err != nil {
		return err
	}
	if err := content.String(string(entry.target)); err != nil {
		return err
	}
	if err := writeDataType(content, entry.typ); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.cardinality)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.joins))); err != nil {
		return err
	}
	for _, join := range entry.joins {
		if err := content.String(string(join.Source)); err != nil {
			return err
		}
		if err := content.String(string(join.Target)); err != nil {
			return err
		}
		if err := content.Uint(uint64(join.Missing)); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(entry.program))); err != nil {
		return err
	}
	for _, instruction := range entry.program {
		if err := content.Uint(uint64(instruction.Op)); err != nil {
			return err
		}
		if err := content.Uint(uint64(instruction.Out)); err != nil {
			return err
		}
		for _, argument := range instruction.Args {
			if err := content.Uint(uint64(argument)); err != nil {
				return err
			}
		}
		if err := content.String(string(instruction.Ref)); err != nil {
			return err
		}
		if err := content.String(string(instruction.Aux)); err != nil {
			return err
		}
		if err := writeDataType(content, instruction.Type); err != nil {
			return err
		}
		if err := content.Uint(instruction.Literal); err != nil {
			return err
		}
	}
	if err := content.Uint(uint64(entry.result)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.outputs))); err != nil {
		return err
	}
	for _, output := range entry.outputs {
		if err := content.String(string(output.Output)); err != nil {
			return err
		}
		if err := content.Uint(uint64(output.Register)); err != nil {
			return err
		}
		if err := content.Uint(uint64(output.Proof)); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(entry.requires))); err != nil {
		return err
	}
	for _, required := range entry.requires {
		if err := content.String(string(required)); err != nil {
			return err
		}
	}
	if err := content.String(string(entry.subject)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.emissions))); err != nil {
		return err
	}
	for _, emission := range entry.emissions {
		if err := content.Uint(uint64(emission)); err != nil {
			return err
		}
	}
	if err := content.Uint(uint64(entry.input)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.inputSource)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.selection)); err != nil {
		return err
	}
	if err := content.String(string(entry.source)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.parameters))); err != nil {
		return err
	}
	for _, parameter := range entry.parameters {
		if err := writeDataType(content, parameter); err != nil {
			return err
		}
	}
	if err := content.Uint(uint64(entry.base)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.identity))); err != nil {
		return err
	}
	for _, parameter := range entry.identity {
		if err := content.Uint(uint64(parameter)); err != nil {
			return err
		}
	}
	if err := content.Uint(uint64(entry.constructor)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.order)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.node)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.dependencies))); err != nil {
		return err
	}
	for _, dependency := range entry.dependencies {
		if err := content.Uint(uint64(dependency)); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(entry.predecessors))); err != nil {
		return err
	}
	for _, predecessor := range entry.predecessors {
		if err := content.String(string(predecessor)); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(entry.edges))); err != nil {
		return err
	}
	for _, edge := range entry.edges {
		if err := content.Uint(uint64(edge.Source)); err != nil {
			return err
		}
		if err := content.String(string(edge.Stage)); err != nil {
			return err
		}
		if err := content.Uint(uint64(edge.Transport)); err != nil {
			return err
		}
		if err := content.Count(uint64(len(edge.WriterStages))); err != nil {
			return err
		}
		for _, stage := range edge.WriterStages {
			if err := content.String(string(stage)); err != nil {
				return err
			}
		}
		if err := content.String(edge.Framing); err != nil {
			return err
		}
	}
	if err := content.String(entry.framing); err != nil {
		return err
	}
	if err := content.Bool(entry.native); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.inputCount)); err != nil {
		return err
	}
	return content.Uint(uint64(entry.empty))
}

type Surface struct{ entries []*Entry }

func NewSurface(entries []*Entry) *Surface {
	return &Surface{entries: append([]*Entry(nil), entries...)}
}

func (*Surface) Kind() schema.SurfaceKind { return schema.SurfaceKindIssuance }

func (surface *Surface) Entries() []schema.Entry {
	if surface == nil {
		return nil
	}
	rows := make([]schema.Entry, len(surface.entries))
	for index, entry := range surface.entries {
		rows[index] = entry
	}
	return rows
}

type Table struct{ entries map[schema.Key]*Entry }

func NewTable(view seal.View) (Table, bool) {
	if view.Kind() != schema.SurfaceKindIssuance {
		return Table{}, false
	}
	entries := make(map[schema.Key]*Entry, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, ok := view.At(position)
		entry, typed := row.(*Entry)
		if !ok || !typed || entry == nil || !entry.EntryAvailable() {
			return Table{}, false
		}
		entries[entry.key] = entry
	}
	return Table{entries: entries}, true
}

func (table Table) Entry(key schema.Key, kind Kind) (*Entry, bool) {
	entry := table.entries[key]
	return entry, entry != nil && entry.kind == kind
}

// Entries returns one declaration kind in its sealed ordinal order. The
// caller receives exact immutable declaration pointers, never projected enum
// values or copied semantic fields.
func (table Table) Entries(kind Kind) []*Entry {
	if table.entries == nil || !kind.valid() {
		return nil
	}
	entries := make([]*Entry, 0)
	for _, entry := range table.entries {
		if entry != nil && entry.kind == kind {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].ordinal < entries[right].ordinal })
	return entries
}

var _ schema.Entry = (*Entry)(nil)
var _ seal.Surface = (*Surface)(nil)
