package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/internal/framing"
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
// direct-return evidence, CSR state, and other derived state are rebuilt or
// supplied by their owning construction boundary.
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
