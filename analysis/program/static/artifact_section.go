package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// The Static artifact is a payload inside an owner-selected artifact stream.
// It deliberately has no domain/header or stream terminator of its own.  The
// The eight records are the same semantic records used by contentID; keeping the
// record framing here makes the persistence payload and the identity digest
// use one schema.
const (
	staticArtifactRecordTypes        uint64 = 1
	staticArtifactRecordReferences   uint64 = 2
	staticArtifactRecordDeclarations uint64 = 3
	staticArtifactRecordSignatures   uint64 = 4
	staticArtifactRecordContracts    uint64 = 5
	staticArtifactRecordOperators    uint64 = 6
	staticArtifactRecordOperands     uint64 = 7
	staticArtifactRecordPublications uint64 = 8

	staticArtifactUintWireMin = uint64(3)
	staticArtifactBoolWireMin = uint64(3)

	staticArtifactPrimitiveWireMin = staticArtifactUintWireMin
	staticArtifactLiteralWireMin   = staticArtifactUintWireMin * 3
	staticArtifactOptionalWireMin  = staticArtifactUintWireMin
	staticArtifactUnionWireMin     = staticArtifactUintWireMin * 3 // count + two terms
	staticArtifactGenericWireMin   = staticArtifactUintWireMin * 3 // base + count + one arg
	staticArtifactArrayWireMin     = staticArtifactUintWireMin + staticArtifactBoolWireMin
	staticArtifactMapWireMin       = staticArtifactUintWireMin*2 + staticArtifactBoolWireMin
	staticArtifactRecordWireMin    = staticArtifactBoolWireMin + staticArtifactUintWireMin
	staticArtifactFieldWireMin     = staticArtifactUintWireMin*2 + staticArtifactBoolWireMin

	staticArtifactReferenceWireMin       = staticArtifactUintWireMin * 6 // row + source count/key + canonical count
	staticArtifactAliasWireMin           = staticArtifactUintWireMin * 8
	staticArtifactTypeParamWireMin       = staticArtifactUintWireMin * 3
	staticArtifactInterfaceWireMin       = staticArtifactUintWireMin*2 + staticArtifactUintWireMin*4 + staticArtifactUintWireMin*2
	staticArtifactInterfaceMemberWireMin = staticArtifactUintWireMin * 8
	staticArtifactDeclaredTypeWireMin    = staticArtifactUintWireMin * 2

	staticArtifactTypeFunctionWireMin = staticArtifactUintWireMin * 10 // scope, three counts, variadic, coordinate, bool, returns count
	staticArtifactParameterWireMin    = staticArtifactUintWireMin * 6
	staticArtifactAssertionWireMin    = staticArtifactUintWireMin * 8

	staticArtifactContractFunctionWireMin = staticArtifactUintWireMin * 3
	staticArtifactContractCallWireMin     = staticArtifactUintWireMin
	staticArtifactTypeOfWireMin           = staticArtifactUintWireMin * 2
	staticArtifactKeyOfWireMin            = staticArtifactUintWireMin
	staticArtifactIndexAccessWireMin      = staticArtifactUintWireMin * 2
	staticArtifactConditionalWireMin      = staticArtifactUintWireMin * 4
	staticArtifactClaimWireMin            = staticArtifactUintWireMin * 2
	staticArtifactTypeValueWireMin        = staticArtifactUintWireMin
	staticArtifactAnnotationWireMin       = staticArtifactUintWireMin * 4
	staticArtifactPublicationWireMin      = staticArtifactUintWireMin * 3
)

var (
	errInvalidArtifactComponent = errors.New("program/static: invalid artifact component")
	errInvalidArtifactSection   = errors.New("program/static: invalid artifact section")
)

// WriteArtifactSection emits only the authored Static payload.  The caller
// owns Writer.Reset, Header, and Finish; this function is safe to place
// between another owner's root fields and suffix fields. It consumes the
// direct Static View and fails closed when that view is unavailable.
func WriteArtifactSection(writer *framing.Writer, view View) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	return writeArtifactViewContent(writer, view)
}

// writeArtifactViewContent is the publication-facing entry to Static's one
// canonical authored-row writer. A construction View keeps the lifecycle
// fence held for the complete write, so an expired copied View cannot emit a
// payload after its owner has been consumed. Published Views use their
// immutable authored stores directly. No aggregate artifact representation is
// constructed at this boundary.
func writeArtifactViewContent(writer *framing.Writer, view View) error {
	if view.state != nil {
		state := view.state
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.phase != draftClaimed || state.component == nil || !state.component.contentID.Available() {
			return errInvalidArtifactComponent
		}
		component := state.component
		return writeArtifactContent(writer,
			component.types, component.references, component.declarations,
			component.signatures, component.contracts, component.operators,
			component.operands, component.publications)
	}
	component := view.component
	if component == nil || !component.contentID.Available() {
		return errInvalidArtifactComponent
	}
	return writeArtifactContent(writer,
		component.types, component.references, component.declarations,
		component.signatures, component.contracts, component.operators,
		component.operands, component.publications)
}

// writeArtifactContent is shared by the identity digest and the public
// payload boundary.  The digest computes the identity before Component's
// snapshot field is assigned, so it must use this schema writer directly
// rather than the publication-time availability guard above.
func writeArtifactContent(
	writer *framing.Writer,
	types typeStore,
	references referenceStore,
	declarations declarationStore,
	signatures signatureStore,
	contracts contractsStore,
	operators operatorsStore,
	operands operandsStore,
	publications []publicationRow,
) error {
	if err := writer.Record(staticArtifactRecordTypes); err != nil {
		return err
	}
	if err := writeTypesContent(writer, types); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordReferences); err != nil {
		return err
	}
	if err := writeReferencesContent(writer, references); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordDeclarations); err != nil {
		return err
	}
	if err := writeDeclarationsContent(writer, declarations); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordSignatures); err != nil {
		return err
	}
	if err := writeSignaturesContent(writer, signatures); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordContracts); err != nil {
		return err
	}
	if err := writeContractsContent(writer, contracts); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordOperators); err != nil {
		return err
	}
	if err := writeOperatorsContent(writer, operators); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordOperands); err != nil {
		return err
	}
	if err := writeOperandsContent(writer, operands); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordPublications); err != nil {
		return err
	}
	if err := writePublicationsContent(writer, publications); err != nil {
		return err
	}
	return nil
}

// ReadArtifactSection consumes only the authored Static payload.  Counts stay
// zero intentionally: the root assembler injects the already sealed Program
// family denominators before calling Build.  The decoder allocates only the
// returned Input slices; all indexes, inverse rows, containment proofs,
// direct-return evidence, CSR state, and receipts are rebuilt or supplied by
// their owning construction boundary.
func ReadArtifactSection(reader *framing.Reader) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	probe := staticArtifactDecoder{reader: reader}
	if err := probe.preflightSection(); err != nil {
		return Input{}, err
	}
	var input Input
	decoder := staticArtifactDecoder{reader: reader, preflighted: true}
	if err := decoder.record(staticArtifactRecordTypes); err != nil {
		return Input{}, err
	}
	if err := decoder.types(&input.Types); err != nil {
		return Input{}, err
	}
	if err := decoder.record(staticArtifactRecordReferences); err != nil {
		return Input{}, err
	}
	if err := decoder.references(&input.References); err != nil {
		return Input{}, err
	}
	if err := decoder.record(staticArtifactRecordDeclarations); err != nil {
		return Input{}, err
	}
	if err := decoder.declarations(&input.Declarations); err != nil {
		return Input{}, err
	}
	if err := decoder.record(staticArtifactRecordSignatures); err != nil {
		return Input{}, err
	}
	if err := decoder.signatures(&input.Signatures); err != nil {
		return Input{}, err
	}
	if err := decoder.record(staticArtifactRecordContracts); err != nil {
		return Input{}, err
	}
	if err := decoder.contracts(&input.Contracts); err != nil {
		return Input{}, err
	}
	if err := decoder.record(staticArtifactRecordOperators); err != nil {
		return Input{}, err
	}
	if err := decoder.operators(&input.Operators); err != nil {
		return Input{}, err
	}
	if err := decoder.record(staticArtifactRecordOperands); err != nil {
		return Input{}, err
	}
	if err := decoder.operands(&input.Operands); err != nil {
		return Input{}, err
	}
	if err := decoder.record(staticArtifactRecordPublications); err != nil {
		return Input{}, err
	}
	if err := decoder.publications(&input.Publications); err != nil {
		return Input{}, err
	}
	return input, nil
}

type staticArtifactDecoder struct {
	reader        *framing.Reader
	probing       bool
	preflighted   bool
	lastTermCount int
	lastKeyCount  int
}

func (decoder *staticArtifactDecoder) preflightSection() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordTypes); err != nil {
		return err
	}
	if err := probe.types(nil); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordReferences); err != nil {
		return err
	}
	if err := probe.references(nil); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordDeclarations); err != nil {
		return err
	}
	if err := probe.declarations(nil); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordSignatures); err != nil {
		return err
	}
	if err := probe.signatures(nil); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordContracts); err != nil {
		return err
	}
	if err := probe.contracts(nil); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordOperators); err != nil {
		return err
	}
	if err := probe.operators(nil); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordOperands); err != nil {
		return err
	}
	if err := probe.operands(nil); err != nil {
		return err
	}
	if err := probe.record(staticArtifactRecordPublications); err != nil {
		return err
	}
	if err := probe.publications(nil); err != nil {
		return err
	}
	return nil
}

// probeReader makes a value copy of framing.Reader. Reader's semantic
// methods advance only that copy, so a complete collection can be checked
// without touching the live stream or allocating any decoded rows. The
// subsequent real pass consumes the same bytes and uses the same row schema.
func (decoder *staticArtifactDecoder) probeReader() (staticArtifactDecoder, error) {
	if decoder == nil || decoder.reader == nil {
		return staticArtifactDecoder{}, framing.ErrMalformed
	}
	reader := *decoder.reader
	probe := staticArtifactDecoder{reader: &reader, probing: true}
	return probe, nil
}

func (decoder *staticArtifactDecoder) preflightTypes() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.types(nil)
}

func (decoder *staticArtifactDecoder) preflightReferences() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.references(nil)
}

func (decoder *staticArtifactDecoder) preflightDeclarations() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.declarations(nil)
}

func (decoder *staticArtifactDecoder) preflightSignatures() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.signatures(nil)
}

func (decoder *staticArtifactDecoder) preflightContracts() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.contracts(nil)
}

func (decoder *staticArtifactDecoder) preflightOperators() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.operators(nil)
}

func (decoder *staticArtifactDecoder) preflightOperands() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.operands(nil)
}

func (decoder *staticArtifactDecoder) preflightPublications() error {
	probe, err := decoder.probeReader()
	if err != nil {
		return err
	}
	return probe.publications(nil)
}

func (decoder *staticArtifactDecoder) record(want uint64) error {
	if decoder == nil || decoder.reader == nil {
		return framing.ErrMalformed
	}
	got, err := decoder.reader.Record()
	if err != nil {
		return err
	}
	if got != want {
		return errInvalidArtifactSection
	}
	return nil
}

// count is the allocation gate for every decoded collection.  The ordinal
// ceiling is deliberately stricter than a machine's int/uint32 ceiling, and
// the remaining-byte floor ensures an adversarial arity cannot reserve more
// rows than the unread canonical stream can possibly contain.
func (decoder *staticArtifactDecoder) count(rowMinimum uint64) (int, error) {
	value, err := decoder.reader.Count()
	if err != nil {
		return 0, err
	}
	if value > uint64(keyspace.MaxTermOrdinal) ||
		value > uint64(^uint(0)>>1) ||
		rowMinimum == 0 || value > uint64(decoder.reader.Remaining())/rowMinimum {
		return 0, errInvalidArtifactSection
	}
	return int(value), nil
}

func (decoder *staticArtifactDecoder) uint32() (uint32, error) {
	value, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if value > uint64(^uint32(0)) {
		return 0, errInvalidArtifactSection
	}
	return uint32(value), nil
}

func (decoder *staticArtifactDecoder) enum(max uint64) (uint8, error) {
	value, err := decoder.reader.Uint()
	if err != nil {
		return 0, err
	}
	if value == 0 || value > max {
		return 0, errInvalidArtifactSection
	}
	return uint8(value), nil
}

func (decoder *staticArtifactDecoder) term() (keyspace.Term, error) {
	value, err := decoder.uint32()
	if err != nil {
		return 0, err
	}
	term := keyspace.Term(value)
	if term != 0 && keyspace.TermFamily(term) == keyspace.FamilyInvalid {
		return 0, errInvalidArtifactSection
	}
	return term, nil
}

type staticArtifactTermConstraint uint8

const (
	staticArtifactAnyTerm staticArtifactTermConstraint = iota + 1
	staticArtifactStaticNodeTerm
	staticArtifactTypeRefTerm
	staticArtifactTypeParamTerm
	staticArtifactTypeFieldTerm
)

func validDecodedTerm(term keyspace.Term, constraint staticArtifactTermConstraint) bool {
	family := keyspace.TermFamily(term)
	if family == keyspace.FamilyInvalid {
		return false
	}
	switch constraint {
	case staticArtifactAnyTerm:
		return true
	case staticArtifactStaticNodeTerm:
		return staticNodeFamily(family)
	case staticArtifactTypeRefTerm:
		return family == keyspace.FamilyTypeRef
	case staticArtifactTypeParamTerm:
		return family == keyspace.FamilyTypeParam
	case staticArtifactTypeFieldTerm:
		return family == keyspace.FamilyTypeField
	default:
		return false
	}
}

func (decoder *staticArtifactDecoder) constrainedTerm(constraint staticArtifactTermConstraint) (keyspace.Term, error) {
	term, err := decoder.term()
	if err != nil {
		return 0, err
	}
	if !validDecodedTerm(term, constraint) {
		return 0, errInvalidArtifactSection
	}
	return term, nil
}

func (decoder *staticArtifactDecoder) key() (keyspace.Key, error) {
	value, err := decoder.uint32()
	if err != nil {
		return 0, err
	}
	if value == 0 {
		return 0, errInvalidArtifactSection
	}
	return keyspace.Key(value), nil
}

func (decoder *staticArtifactDecoder) boolean() (bool, error) {
	return decoder.reader.Bool()
}

func (decoder *staticArtifactDecoder) coordinate() (source.Coordinate, error) {
	startLine, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	startColumn, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	endLine, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	endColumn, err := decoder.uint32()
	if err != nil {
		return source.Coordinate{}, err
	}
	coordinate, ok := source.CoordinateFromParts(startLine, startColumn, endLine, endColumn)
	if !ok {
		return source.Coordinate{}, errInvalidArtifactSection
	}
	return coordinate, nil
}

func (decoder *staticArtifactDecoder) termSequenceConstraint(minimum int, constraint staticArtifactTermConstraint) ([]keyspace.Term, error) {
	if !decoder.probing && !decoder.preflighted {
		probe, err := decoder.probeReader()
		if err != nil {
			return nil, err
		}
		if _, err := probe.termSequenceConstraint(minimum, constraint); err != nil {
			return nil, err
		}
	}
	count, err := decoder.count(staticArtifactUintWireMin)
	if err != nil {
		return nil, err
	}
	if count < minimum {
		return nil, errInvalidArtifactSection
	}
	decoder.lastTermCount = count
	var terms []keyspace.Term
	if !decoder.probing {
		terms = make([]keyspace.Term, count)
	}
	for index := 0; index < count; index++ {
		term, readErr := decoder.constrainedTerm(constraint)
		if readErr != nil {
			return nil, readErr
		}
		if !decoder.probing {
			terms[index] = term
		}
	}
	return terms, nil
}

func (decoder *staticArtifactDecoder) types(output *TypesInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightTypes(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactPrimitiveWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Primitive = make([]Primitive, count)
	}
	for index := 0; index < count; index++ {
		kind, err := decoder.enum(uint64(PrimitiveSelf))
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Primitive[index].Kind = PrimitiveKind(kind)
		}
	}

	count, err = decoder.count(staticArtifactLiteralWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Literal = make([]Literal, count)
	}
	for index := 0; index < count; index++ {
		kind, err := decoder.enum(uint64(keyspace.LiteralString))
		if err != nil {
			return err
		}
		exact, err := decoder.uint32()
		if err != nil {
			return err
		}
		floatBits, err := decoder.reader.Uint()
		if err != nil {
			return err
		}
		switch keyspace.LiteralKind(kind) {
		case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
			if exact == 0 || floatBits != 0 {
				return errInvalidArtifactSection
			}
		case keyspace.LiteralFloat:
			if exact != 0 {
				return errInvalidArtifactSection
			}
		default:
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Literal[index] = Literal{
				Kind:      keyspace.LiteralKind(kind),
				Exact:     keyspace.Key(exact),
				FloatBits: floatBits,
			}
		}
	}

	count, err = decoder.count(staticArtifactOptionalWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Optional = make([]Optional, count)
	}
	for index := 0; index < count; index++ {
		inner, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Optional[index] = Optional{Inner: inner}
		}
	}

	union, err := decoder.unions()
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Union = union
	}
	intersection, err := decoder.intersections()
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Intersection = intersection
	}

	count, err = decoder.count(staticArtifactGenericWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Generic = make([]Generic, count)
	}
	for index := 0; index < count; index++ {
		base, err := decoder.constrainedTerm(staticArtifactTypeRefTerm)
		if err != nil {
			return err
		}
		args, err := decoder.termSequenceConstraint(1, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Generic[index] = Generic{Base: base, Args: args}
		}
	}

	count, err = decoder.count(staticArtifactArrayWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Array = make([]Array, count)
	}
	for index := 0; index < count; index++ {
		element, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		readOnly, err := decoder.boolean()
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Array[index] = Array{Element: element, ReadOnly: readOnly}
		}
	}

	count, err = decoder.count(staticArtifactMapWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Map = make([]Map, count)
	}
	for index := 0; index < count; index++ {
		key, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		value, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		readOnly, err := decoder.boolean()
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Map[index] = Map{Key: key, Value: value, ReadOnly: readOnly}
		}
	}

	count, err = decoder.count(staticArtifactRecordWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Record = make([]Record, count)
	}
	for index := 0; index < count; index++ {
		readOnly, err := decoder.boolean()
		if err != nil {
			return err
		}
		fields, err := decoder.termSequenceConstraint(0, staticArtifactTypeFieldTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Record[index] = Record{Fields: fields, ReadOnly: readOnly}
		}
	}

	count, err = decoder.count(staticArtifactFieldWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Field = make([]Field, count)
	}
	for index := 0; index < count; index++ {
		key, err := decoder.key()
		if err != nil {
			return err
		}
		typ, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		optional, err := decoder.boolean()
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Field[index] = Field{Key: key, Type: typ, Optional: optional}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) unions() ([]Union, error) {
	count, err := decoder.count(staticArtifactUnionWireMin)
	if err != nil {
		return nil, err
	}
	var rows []Union
	if !decoder.probing {
		rows = make([]Union, count)
	}
	for index := 0; index < count; index++ {
		members, err := decoder.termSequenceConstraint(2, staticArtifactStaticNodeTerm)
		if err != nil {
			return nil, err
		}
		if !decoder.probing {
			rows[index] = Union{Members: members}
		}
	}
	return rows, nil
}

func (decoder *staticArtifactDecoder) intersections() ([]Intersection, error) {
	count, err := decoder.count(staticArtifactUnionWireMin)
	if err != nil {
		return nil, err
	}
	var rows []Intersection
	if !decoder.probing {
		rows = make([]Intersection, count)
	}
	for index := 0; index < count; index++ {
		members, err := decoder.termSequenceConstraint(2, staticArtifactStaticNodeTerm)
		if err != nil {
			return nil, err
		}
		if !decoder.probing {
			rows[index] = Intersection{Members: members}
		}
	}
	return rows, nil
}

func (decoder *staticArtifactDecoder) references(output *ReferencesInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightReferences(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactReferenceWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeRef = make([]TypeRef, count)
	}
	for index := 0; index < count; index++ {
		resolution, err := decoder.enum(uint64(TypeRefCanonicalPath))
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		root, err := decoder.term()
		if err != nil {
			return err
		}
		sourceKeys, err := decoder.keys()
		if err != nil {
			return err
		}
		sourceKeyCount := decoder.lastKeyCount
		canonicalKeys, err := decoder.keysAllowEmpty()
		if err != nil {
			return err
		}
		canonicalKeyCount := decoder.lastKeyCount
		switch TypeRefResolution(resolution) {
		case TypeRefUnresolved:
			if target != 0 || canonicalKeyCount != 0 {
				return errInvalidArtifactSection
			}
		case TypeRefDeclaration:
			if !staticrole.TypeReferenceTargetFamily(keyspace.TermFamily(target)) || canonicalKeyCount != 0 {
				return errInvalidArtifactSection
			}
		case TypeRefCanonicalPath:
			if target != 0 || canonicalKeyCount == 0 {
				return errInvalidArtifactSection
			}
		default:
			return errInvalidArtifactSection
		}
		if sourceKeyCount == 1 && root != 0 {
			return errInvalidArtifactSection
		}
		if sourceKeyCount > 1 && keyspace.TermFamily(root) != keyspace.FamilyCell {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeRef[index] = TypeRef{
				Resolution: TypeRefResolution(resolution),
				Target:     target,
				Root:       root,
				Source:     sourceKeys,
				Canonical:  canonicalKeys,
			}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) keys() ([]keyspace.Key, error) {
	return decoder.keysWithMinimum(1)
}

func (decoder *staticArtifactDecoder) keysAllowEmpty() ([]keyspace.Key, error) {
	return decoder.keysWithMinimum(0)
}

func (decoder *staticArtifactDecoder) keysWithMinimum(minimum int) ([]keyspace.Key, error) {
	if !decoder.probing && !decoder.preflighted {
		probe, err := decoder.probeReader()
		if err != nil {
			return nil, err
		}
		if _, err := probe.keysWithMinimum(minimum); err != nil {
			return nil, err
		}
	}
	count, err := decoder.count(staticArtifactUintWireMin)
	if err != nil {
		return nil, err
	}
	if count < minimum {
		return nil, errInvalidArtifactSection
	}
	decoder.lastKeyCount = count
	var keys []keyspace.Key
	if !decoder.probing {
		keys = make([]keyspace.Key, count)
	}
	for index := 0; index < count; index++ {
		key, readErr := decoder.key()
		if readErr != nil {
			return nil, readErr
		}
		if !decoder.probing {
			keys[index] = key
		}
	}
	return keys, nil
}

func (decoder *staticArtifactDecoder) declarations(output *DeclarationsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightDeclarations(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactAliasWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Alias = make([]TypeAlias, count)
	}
	for index := 0; index < count; index++ {
		owner, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || !validDecodedTerm(target, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		coordinate, err := decoder.coordinate()
		if err != nil || coordinate == (source.Coordinate{}) {
			if err != nil {
				return err
			}
			return errInvalidArtifactSection
		}
		params, err := decoder.termSequenceConstraint(0, staticArtifactTypeParamTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Alias[index] = TypeAlias{Owner: owner, Target: target, Name: name, NameCoordinate: coordinate, Params: params}
		}
	}

	count, err = decoder.count(staticArtifactTypeParamWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeParam = make([]TypeParam, count)
	}
	for index := 0; index < count; index++ {
		owner, err := decoder.term()
		if err != nil {
			return err
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		constraint, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.TypeParameterOwnerFamily(keyspace.TermFamily(owner)) {
			return errInvalidArtifactSection
		}
		if !staticrole.TypeParameterOwnerFamily(keyspace.TermFamily(owner)) || (constraint != 0 && !validDecodedTerm(constraint, staticArtifactStaticNodeTerm)) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeParam[index] = TypeParam{Owner: owner, Name: name, Constraint: constraint}
		}
	}

	count, err = decoder.count(staticArtifactInterfaceWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Interface = make([]Interface, count)
	}
	for index := 0; index < count; index++ {
		owner, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errInvalidArtifactSection
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		coordinate, err := decoder.coordinate()
		if err != nil || coordinate == (source.Coordinate{}) {
			if err != nil {
				return err
			}
			return errInvalidArtifactSection
		}
		extends, err := decoder.termSequenceConstraint(0, staticArtifactTypeRefTerm)
		if err != nil {
			return err
		}
		memberCount, err := decoder.count(staticArtifactInterfaceMemberWireMin)
		if err != nil {
			return err
		}
		var members []InterfaceMember
		if !decoder.probing {
			members = make([]InterfaceMember, memberCount)
		}
		for memberIndex := 0; memberIndex < memberCount; memberIndex++ {
			kind, err := decoder.enum(uint64(InterfaceMethod))
			if err != nil {
				return err
			}
			field, err := decoder.term()
			if err != nil {
				return err
			}
			memberName, err := decoder.uint32()
			if err != nil {
				return err
			}
			coordinate, err := decoder.coordinate()
			if err != nil {
				return err
			}
			signature, err := decoder.term()
			if err != nil {
				return err
			}
			member := InterfaceMember{
				Kind:           InterfaceMemberKind(kind),
				Field:          field,
				Name:           keyspace.Key(memberName),
				NameCoordinate: coordinate,
				Signature:      signature,
			}
			if member.Kind == InterfaceField {
				if keyspace.TermFamily(field) != keyspace.FamilyTypeField || memberName != 0 || coordinate != (source.Coordinate{}) || signature != 0 {
					return errInvalidArtifactSection
				}
			} else if member.Kind == InterfaceMethod {
				if field != 0 || memberName == 0 || coordinate == (source.Coordinate{}) || keyspace.TermFamily(signature) != keyspace.FamilyTypeFunction {
					return errInvalidArtifactSection
				}
			}
			if !decoder.probing {
				members[memberIndex] = member
			}
		}
		if !decoder.probing {
			output.Interface[index] = Interface{Owner: owner, Name: name, NameCoordinate: coordinate, Extends: extends, Members: members}
		}
	}

	count, err = decoder.count(staticArtifactDeclaredTypeWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.DeclaredType = make([]DeclaredType, count)
	}
	for index := 0; index < count; index++ {
		cell, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(cell) != keyspace.FamilyCell || !validDecodedTerm(target, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.DeclaredType[index] = DeclaredType{Cell: cell, Target: target}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) signatures(output *SignaturesInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightSignatures(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactTypeFunctionWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeFunction = make([]TypeFunction, count)
	}
	for index := 0; index < count; index++ {
		scope, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) {
			return errInvalidArtifactSection
		}
		typeParams, err := decoder.termSequenceConstraint(0, staticArtifactTypeParamTerm)
		if err != nil {
			return err
		}
		parameterCount, err := decoder.count(staticArtifactParameterWireMin)
		if err != nil {
			return err
		}
		var parameters []Parameter
		if !decoder.probing {
			parameters = make([]Parameter, parameterCount)
		}
		for parameterIndex := 0; parameterIndex < parameterCount; parameterIndex++ {
			name, err := decoder.uint32()
			if err != nil {
				return err
			}
			coordinate, err := decoder.coordinate()
			if err != nil {
				return err
			}
			typ, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
			if err != nil {
				return err
			}
			if (name == 0) != (coordinate == (source.Coordinate{})) {
				return errInvalidArtifactSection
			}
			if !decoder.probing {
				parameters[parameterIndex] = Parameter{Name: keyspace.Key(name), NameCoordinate: coordinate, Type: typ}
			}
		}
		variadic, err := decoder.term()
		if err != nil {
			return err
		}
		variadicCoordinate, err := decoder.coordinate()
		if err != nil {
			return err
		}
		if (variadic == 0) != (variadicCoordinate == (source.Coordinate{})) {
			return errInvalidArtifactSection
		}
		if variadic != 0 && !validDecodedTerm(variadic, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		returnsKnown, err := decoder.boolean()
		if err != nil {
			return err
		}
		returns, err := decoder.termSequenceConstraint(0, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		returnsCount := decoder.lastTermCount
		if !returnsKnown && returnsCount != 0 {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeFunction[index] = TypeFunction{
				Scope: scope, TypeParams: typeParams, Parameters: parameters,
				Variadic: variadic, VariadicCoordinate: variadicCoordinate,
				ReturnsKnown: returnsKnown, Returns: returns,
			}
		}
	}

	count, err = decoder.count(staticArtifactAssertionWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeAsserts = make([]TypeAsserts, count)
	}
	for index := 0; index < count; index++ {
		name, err := decoder.key()
		if err != nil {
			return err
		}
		coordinate, err := decoder.coordinate()
		if err != nil || coordinate == (source.Coordinate{}) {
			if err != nil {
				return err
			}
			return errInvalidArtifactSection
		}
		bound, err := decoder.boolean()
		if err != nil {
			return err
		}
		parameter, err := decoder.uint32()
		if err != nil {
			return err
		}
		if !bound && parameter != 0 {
			return errInvalidArtifactSection
		}
		narrow, err := decoder.term()
		if err != nil {
			return err
		}
		if narrow != 0 && !validDecodedTerm(narrow, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeAsserts[index] = TypeAsserts{Name: name, ParamCoordinate: coordinate, Bound: bound, Param: parameter, Narrow: narrow}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) contracts(output *ContractsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightContracts(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactContractFunctionWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Function = make([]FunctionContract, count)
	}
	for index := 0; index < count; index++ {
		typeParams, err := decoder.termSequenceConstraint(0, staticArtifactTypeParamTerm)
		if err != nil {
			return err
		}
		returnsKnown, err := decoder.boolean()
		if err != nil {
			return err
		}
		returns, err := decoder.termSequenceConstraint(0, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		returnsCount := decoder.lastTermCount
		if !returnsKnown && returnsCount != 0 {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Function[index] = FunctionContract{TypeParams: typeParams, ReturnsKnown: returnsKnown, Returns: returns}
		}
	}

	count, err = decoder.count(staticArtifactContractCallWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Call = make([]CallContract, count)
	}
	for index := 0; index < count; index++ {
		typeArguments, err := decoder.termSequenceConstraint(0, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Call[index] = CallContract{TypeArguments: typeArguments}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) operators(output *OperatorsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightOperators(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactTypeOfWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeOf = make([]TypeOf, count)
	}
	for index := 0; index < count; index++ {
		scope, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) {
			return errInvalidArtifactSection
		}
		operand, err := decoder.term()
		if err != nil {
			return err
		}
		// TypeOf's operand is a cross-owner Flow value occurrence. Reject
		// static nodes, storage handles, and Module Import terms at decode time;
		// Build performs the counted ordinal check after root denominators are
		// injected.
		if !flowrole.ValueOccurrenceFamily(keyspace.TermFamily(operand)) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeOf[index] = TypeOf{Scope: scope, Operand: operand}
		}
	}

	count, err = decoder.count(staticArtifactKeyOfWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.KeyOf = make([]KeyOf, count)
	}
	for index := 0; index < count; index++ {
		inner, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.KeyOf[index] = KeyOf{Inner: inner}
		}
	}

	count, err = decoder.count(staticArtifactIndexAccessWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.IndexAccess = make([]IndexAccess, count)
	}
	for index := 0; index < count; index++ {
		object, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		indexTerm, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.IndexAccess[index] = IndexAccess{Object: object, Index: indexTerm}
		}
	}

	count, err = decoder.count(staticArtifactConditionalWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Conditional = make([]Conditional, count)
	}
	for index := 0; index < count; index++ {
		check, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		extends, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		then, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		elseTerm, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Conditional[index] = Conditional{Check: check, Extends: extends, Then: then, Else: elseTerm}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) operands(output *OperandsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightOperands(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactClaimWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Claim = make([]ClaimTarget, count)
	}
	var previous keyspace.Term
	for index := 0; index < count; index++ {
		claim, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(claim) != keyspace.FamilyValueClaim ||
			(index != 0 && keyspace.TermOrdinal(claim) <= keyspace.TermOrdinal(previous)) {
			return errInvalidArtifactSection
		}
		if !validDecodedTerm(target, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		previous = claim
		if !decoder.probing {
			output.Claim[index] = ClaimTarget{Claim: claim, Target: target}
		}
	}

	count, err = decoder.count(staticArtifactTypeValueWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeValue = make([]TypeValueTarget, count)
	}
	for index := 0; index < count; index++ {
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(target) != keyspace.FamilyTypePrimitive && keyspace.TermFamily(target) != keyspace.FamilyTypeRef {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeValue[index] = TypeValueTarget{Target: target}
		}
	}

	count, err = decoder.count(staticArtifactAnnotationWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Annotation = make([]Annotation, count)
	}
	for index := 0; index < count; index++ {
		scope, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		values, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) ||
			!staticrole.AnnotationTargetFamily(keyspace.TermFamily(target)) ||
			keyspace.TermFamily(values) != keyspace.FamilyValues {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Annotation[index] = Annotation{Scope: scope, Target: target, Name: name, Values: values}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) publications(output *PublicationsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightPublications(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactPublicationWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Type = make([]Publication, count)
	}
	for index := 0; index < count; index++ {
		assign, err := decoder.term()
		if err != nil {
			return err
		}
		pair, err := decoder.uint32()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(assign) != keyspace.FamilyAssign || keyspace.TermFamily(target) != keyspace.FamilyTypeRef {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Type[index] = Publication{Assign: assign, Pair: pair, Target: target}
		}
	}
	return nil
}
