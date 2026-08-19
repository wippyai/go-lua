package static

import (
	"errors"

	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
	"github.com/wippyai/go-lua/internal/framing"
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

	staticArtifactClaimWireMin      = staticArtifactUintWireMin * 2
	staticArtifactTypeValueWireMin  = staticArtifactUintWireMin
	staticArtifactAnnotationWireMin = staticArtifactUintWireMin * 4
)

var (
	errInvalidArtifactComponent = errors.New("program/static: invalid artifact component")
	errInvalidArtifactSection   = errors.New("program/static: invalid artifact section")
)

// WriteArtifactSection emits only the authored Static payload.  The caller
// owns Writer.Reset, Header, and Finish; this function is safe to place
// between another owner's root fields and suffix fields. It consumes the
// direct Static View and fails closed when that view is unavailable.
func WriteArtifactSection(writer *framing.Writer, view staticquery.View) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	return writeArtifactViewContent(writer, view)
}

// writeArtifactViewContent is the publication-facing entry to Static's one
// canonical authored-row writer. It consumes the immutable authored stores
// directly; no aggregate artifact representation is constructed at this
// boundary.

func writeArtifactViewContent(writer *framing.Writer, view staticquery.View) error {
	snapshot, ok := view.Snapshot()
	if !ok {
		return errInvalidArtifactComponent
	}
	types, references, declarations, signatures, contracts, operators, operands, publications := snapshot.Tables()
	return writeArtifactContent(writer, types, references, declarations, signatures,
		contracts, operators, operands, publications)
}

// writeArtifactContent is shared by the identity digest and the public
// payload boundary.  The digest computes the identity before Component's
// snapshot field is assigned, so it must use this schema writer directly
// rather than the publication-time availability guard above.
func writeArtifactContent(
	writer *framing.Writer,
	types statictypes.Table,
	references staticrefs.Table,
	declarations staticdecl.Table,
	signatures staticsig.Table,
	contracts staticcontracts.Table,
	operators staticoperators.Table,
	operands staticoperands.Table,
	publications staticpubs.Table,
) error {
	if err := writer.Record(staticArtifactRecordTypes); err != nil {
		return err
	}
	if err := statictypes.WriteContent(writer, types); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordReferences); err != nil {
		return err
	}
	if err := staticrefs.WriteContent(writer, references); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordDeclarations); err != nil {
		return err
	}
	if err := staticdecl.WriteContent(writer, declarations); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordSignatures); err != nil {
		return err
	}
	if err := staticsig.WriteContent(writer, signatures); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordContracts); err != nil {
		return err
	}
	if err := staticcontracts.WriteContent(writer, contracts); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordOperators); err != nil {
		return err
	}
	if err := staticoperators.WriteContent(writer, operators); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordOperands); err != nil {
		return err
	}
	if err := staticoperands.WriteContent(writer, operands); err != nil {
		return err
	}
	if err := writer.Record(staticArtifactRecordPublications); err != nil {
		return err
	}
	if err := staticpubs.WriteContent(writer, publications); err != nil {
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
	typesInput, err := statictypes.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.Types = typesInput
	if err := decoder.record(staticArtifactRecordReferences); err != nil {
		return Input{}, err
	}
	referencesInput, err := staticrefs.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.References = referencesInput
	if err := decoder.record(staticArtifactRecordDeclarations); err != nil {
		return Input{}, err
	}
	declarationsInput, err := staticdecl.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.Declarations = declarationsInput
	if err := decoder.record(staticArtifactRecordSignatures); err != nil {
		return Input{}, err
	}
	signaturesInput, err := staticsig.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.Signatures = signaturesInput
	if err := decoder.record(staticArtifactRecordContracts); err != nil {
		return Input{}, err
	}
	contractsInput, err := staticcontracts.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.Contracts = contractsInput
	if err := decoder.record(staticArtifactRecordOperators); err != nil {
		return Input{}, err
	}
	operatorsInput, err := staticoperators.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.Operators = operatorsInput
	if err := decoder.record(staticArtifactRecordOperands); err != nil {
		return Input{}, err
	}
	operandsInput, err := staticoperands.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.Operands = operandsInput
	if err := decoder.record(staticArtifactRecordPublications); err != nil {
		return Input{}, err
	}
	publicationsInput, err := staticpubs.Decode(decoder.reader)
	if err != nil {
		return Input{}, err
	}
	input.Publications = publicationsInput
	return input, nil
}
