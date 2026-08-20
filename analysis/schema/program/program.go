package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Program is the immutable compiled content published for one program.
//
// Module placement is deliberately not part of this value. The same sealed
// content can be mounted at multiple module keys, while each mount row owns
// the key that places it in a Link directory.
//
// The frozen program is carried by value. That is what makes one compiled
// program shareable across every mount of it without a copy: a Frozen shares
// its published structure on assignment and admits no derivation, so the same
// value can sit in the row of two mounts, in two Links, for as long as any of
// them lives, and address the same content throughout.
type Program struct {
	Frozen     snapshot.Frozen
	ArtifactID identity.ContentID
	ProgramID  identity.ContentID
	SchemaID   identity.ContentID
}

// Available reports whether row names compiled program content. A row is
// available only when the frozen publication is sealed and every identity
// that authenticates it is present.
func (row Program) Available() bool {
	return row.Frozen.Published() && row.ArtifactID.Available() && row.ProgramID.Available() && row.SchemaID.Available()
}

// catalog is the identity this program's compiled publication is addressed under.
// It is derived rather than carried, so a row cannot be assembled with a
// catalog that disagrees with the declaration catalog it names.
func (row Program) catalog() (identity.ContentID, bool) {
	if !row.Available() {
		return identity.ContentID{}, false
	}
	return CatalogID(row.SchemaID)
}

// CallTargetCount is the sealed width of this program's call-target family.
func (row Program) CallTargetCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return CallTargetFamily().Count(&row.Frozen, catalog)
}

// CallTargetAt returns one closure-allocation-to-callable-body proof by its
// position in the emitted sequence.
func (row Program) CallTargetAt(index int) (CallTarget, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return CallTarget{}, false
	}
	return CallTargetFamily().At(&row.Frozen, catalog, index)
}

// CallCount is the sealed width of the authored-call family.
func (row Program) CallCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return CallFamily().Count(&row.Frozen, catalog)
}

// CallAt returns one authored-call row by emitted ordinal.
func (row Program) CallAt(index int) (Call, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return Call{}, false
	}
	return CallFamily().At(&row.Frozen, catalog, index)
}

// CallForID resolves the unique authored Call carrying one existing
// owner-issued identity. The call family remains the sole authority; this is
// a cold scan and does not retain an inverse directory beside the publication.
func (row Program) CallForID(id identity.ContentID) (Call, bool) {
	if !row.Available() || !id.Available() {
		return Call{}, false
	}
	count, published := row.CallCount()
	if !published {
		return Call{}, false
	}
	var found Call
	for index := 0; index < count; index++ {
		candidate, held := row.CallAt(index)
		if !held || candidate.ID() != id {
			continue
		}
		if found.Available() {
			return Call{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

// CallResultCount is the sealed width of the authored Call output-geometry
// family. Statement Calls that discard their results have no row.
func (row Program) CallResultCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return CallResultFamily().Count(&row.Frozen, catalog)
}

// CallResultAt returns one reusable output geometry row by emitted ordinal.
func (row Program) CallResultAt(index int) (CallResult, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return CallResult{}, false
	}
	return CallResultFamily().At(&row.Frozen, catalog, index)
}

// CallResultForID resolves the unique output geometry for one existing Call
// identity. The family remains the sole cold authority; no inverse index is
// retained beside the publication.
func (row Program) CallResultForID(id identity.ContentID) (CallResult, bool) {
	if !row.Available() || !id.Available() {
		return CallResult{}, false
	}
	count, published := row.CallResultCount()
	if !published {
		return CallResult{}, false
	}
	var found CallResult
	for index := 0; index < count; index++ {
		candidate, held := row.CallResultAt(index)
		if !held || candidate.CallID() != id {
			continue
		}
		if found.Available() {
			return CallResult{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

// CallOperandFor resolves one operand in a call's published child range.
func (row Program) CallOperandFor(callIndex, childIndex int) (CallOperand, bool) {
	call, ok := row.CallAt(callIndex)
	if !ok || childIndex < 0 || childIndex >= call.OperandCount() {
		return CallOperand{}, false
	}
	offset, _, spanOK := call.OperandSpan()
	if !spanOK {
		return CallOperand{}, false
	}
	catalog, derived := row.catalog()
	if !derived {
		return CallOperand{}, false
	}
	return CallOperandFamily().At(&row.Frozen, catalog, int(offset)+childIndex)
}

// CallArgumentFor resolves one actual argument in a call's published child range.
func (row Program) CallArgumentFor(callIndex, childIndex int) (CallArgument, bool) {
	call, ok := row.CallAt(callIndex)
	if !ok || childIndex < 0 || childIndex >= call.ArgumentCount() {
		return CallArgument{}, false
	}
	offset, _, spanOK := call.ArgumentSpan()
	if !spanOK {
		return CallArgument{}, false
	}
	catalog, derived := row.catalog()
	if !derived {
		return CallArgument{}, false
	}
	return CallArgumentFamily().At(&row.Frozen, catalog, int(offset)+childIndex)
}

// CallArgumentForID resolves one actual argument by its parent Call identity
// and child position. The dense Call and CallArgument families remain the
// only authorities; no ingress-side lookup or copied argument row is kept.
func (row Program) CallArgumentForID(callID identity.ContentID, childIndex int) (CallArgument, bool) {
	if !row.Available() || !callID.Available() || childIndex < 0 {
		return CallArgument{}, false
	}
	count, published := row.CallCount()
	if !published {
		return CallArgument{}, false
	}
	var found CallArgument
	for callIndex := 0; callIndex < count; callIndex++ {
		call, held := row.CallAt(callIndex)
		if !held || call.ID() != callID {
			continue
		}
		argument, argumentOK := row.CallArgumentFor(callIndex, childIndex)
		if !argumentOK || found.Available() {
			return CallArgument{}, false
		}
		found = argument
	}
	return found, found.Available()
}

// CallTypeArgumentFor resolves one static type argument in a call's published child range.
func (row Program) CallTypeArgumentFor(callIndex, childIndex int) (CallTypeArgument, bool) {
	call, ok := row.CallAt(callIndex)
	if !ok || childIndex < 0 || childIndex >= call.TypeArgumentCount() {
		return CallTypeArgument{}, false
	}
	offset, _, spanOK := call.TypeArgumentSpan()
	if !spanOK {
		return CallTypeArgument{}, false
	}
	catalog, derived := row.catalog()
	if !derived {
		return CallTypeArgument{}, false
	}
	return CallTypeArgumentFamily().At(&row.Frozen, catalog, int(offset)+childIndex)
}

// CallOperandCount is the sealed width of the call-operand family.
func (row Program) CallOperandCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return CallOperandFamily().Count(&row.Frozen, catalog)
}

// CallOperandAt returns one call operand by emitted ordinal.
func (row Program) CallOperandAt(index int) (CallOperand, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return CallOperand{}, false
	}
	return CallOperandFamily().At(&row.Frozen, catalog, index)
}

// CallArgumentCount is the sealed width of the call-argument family.
func (row Program) CallArgumentCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return CallArgumentFamily().Count(&row.Frozen, catalog)
}

// CallArgumentAt returns one actual argument by emitted ordinal.
func (row Program) CallArgumentAt(index int) (CallArgument, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return CallArgument{}, false
	}
	return CallArgumentFamily().At(&row.Frozen, catalog, index)
}

// CallTypeArgumentCount is the sealed width of the call type-argument family.
func (row Program) CallTypeArgumentCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return CallTypeArgumentFamily().Count(&row.Frozen, catalog)
}

// CallTypeArgumentAt returns one static type argument by emitted ordinal.
func (row Program) CallTypeArgumentAt(index int) (CallTypeArgument, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return CallTypeArgument{}, false
	}
	return CallTypeArgumentFamily().At(&row.Frozen, catalog, index)
}

// BodyCount is the sealed width of the program's lexical-body family.
func (row Program) BodyCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return BodyFamily().Count(&row.Frozen, catalog)
}

func (row Program) BodyAt(index int) (Body, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return Body{}, false
	}
	return BodyFamily().At(&row.Frozen, catalog, index)
}

func (row Program) BodyEntryFor(bodyIndex, childIndex int) (BodyEntry, bool) {
	body, ok := row.BodyAt(bodyIndex)
	if !ok || childIndex < 0 || childIndex >= body.EntryCount() {
		return BodyEntry{}, false
	}
	offset, _, spanOK := body.EntrySpan()
	if !spanOK {
		return BodyEntry{}, false
	}
	catalog, derived := row.catalog()
	if !derived {
		return BodyEntry{}, false
	}
	child, held := BodyEntryFamily().At(&row.Frozen, catalog, int(offset)+childIndex)
	return child, held && child.BodyID() == body.ID()
}

func (row Program) BodyRootFor(bodyIndex, childIndex int) (BodyRoot, bool) {
	body, ok := row.BodyAt(bodyIndex)
	if !ok || childIndex < 0 || childIndex >= body.RootCount() {
		return BodyRoot{}, false
	}
	offset, _, spanOK := body.RootSpan()
	if !spanOK {
		return BodyRoot{}, false
	}
	catalog, derived := row.catalog()
	if !derived {
		return BodyRoot{}, false
	}
	child, held := BodyRootFamily().At(&row.Frozen, catalog, int(offset)+childIndex)
	return child, held && child.BodyID() == body.ID()
}

func (row Program) OutcomeCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return OutcomeFamily().Count(&row.Frozen, catalog)
}

func (row Program) OutcomeAt(index int) (Outcome, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return Outcome{}, false
	}
	return OutcomeFamily().At(&row.Frozen, catalog, index)
}

func (row Program) BodyOutcomeFor(bodyIndex, childIndex int) (Outcome, bool) {
	body, ok := row.BodyAt(bodyIndex)
	if !ok || childIndex < 0 || childIndex >= body.OutcomeCount() {
		return Outcome{}, false
	}
	offset, _, spanOK := body.OutcomeSpan()
	if !spanOK {
		return Outcome{}, false
	}
	outcome, held := row.OutcomeAt(int(offset) + childIndex)
	return outcome, held && outcome.BodyID() == body.ID()
}

func (row Program) OutcomeReturnValueFor(outcomeIndex, childIndex int) (OutcomeReturnValue, bool) {
	outcome, ok := row.OutcomeAt(outcomeIndex)
	if !ok || childIndex < 0 || childIndex >= outcome.ReturnValueCount() {
		return OutcomeReturnValue{}, false
	}
	offset, _, spanOK := outcome.ReturnValueSpan()
	if !spanOK {
		return OutcomeReturnValue{}, false
	}
	catalog, derived := row.catalog()
	if !derived {
		return OutcomeReturnValue{}, false
	}
	child, held := OutcomeReturnValueFamily().At(&row.Frozen, catalog, int(offset)+childIndex)
	return child, held && child.OutcomeID() == outcome.ID()
}

func (row Program) OutcomePointFor(outcomeIndex, childIndex int) (OutcomePoint, bool) {
	outcome, ok := row.OutcomeAt(outcomeIndex)
	if !ok || childIndex < 0 || childIndex >= outcome.PointCount() {
		return OutcomePoint{}, false
	}
	offset, _, spanOK := outcome.PointSpan()
	if !spanOK {
		return OutcomePoint{}, false
	}
	catalog, derived := row.catalog()
	if !derived {
		return OutcomePoint{}, false
	}
	child, held := OutcomePointFamily().At(&row.Frozen, catalog, int(offset)+childIndex)
	return child, held && child.OutcomeID() == outcome.ID()
}

// ExactScalarSummaryCount is the sealed width of this program's exact scalar
// summary family.
func (row Program) ExactScalarSummaryCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return ExactScalarSummaryFamily().Count(&row.Frozen, catalog)
}

// ExactScalarSummaryAt returns one exact scalar proof by its emitted ordinal.
func (row Program) ExactScalarSummaryAt(index int) (ExactScalarSummary, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return ExactScalarSummary{}, false
	}
	return ExactScalarSummaryFamily().At(&row.Frozen, catalog, index)
}

// ArithmeticSummaryCount is the sealed width of this program's arithmetic
// summary family.
func (row Program) ArithmeticSummaryCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return ArithmeticSummaryFamily().Count(&row.Frozen, catalog)
}

// ArithmeticSummaryAt returns one arithmetic proof by its emitted ordinal.
func (row Program) ArithmeticSummaryAt(index int) (ArithmeticSummary, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return ArithmeticSummary{}, false
	}
	return ArithmeticSummaryFamily().At(&row.Frozen, catalog, index)
}

// UnarySummaryCount is the sealed width of this program's unary summary
// family.
func (row Program) UnarySummaryCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return UnarySummaryFamily().Count(&row.Frozen, catalog)
}

// UnarySummaryAt returns one unary proof by its emitted ordinal.
func (row Program) UnarySummaryAt(index int) (UnarySummary, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return UnarySummary{}, false
	}
	return UnarySummaryFamily().At(&row.Frozen, catalog, index)
}

// StaticInputCount is the sealed width of the authored static-input family.
func (row Program) StaticInputCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return StaticInputFamily().Count(&row.Frozen, catalog)
}

// StaticInputAt returns one authored static input by emitted ordinal.
func (row Program) StaticInputAt(index int) (StaticInput, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return StaticInput{}, false
	}
	return StaticInputFamily().At(&row.Frozen, catalog, index)
}

// StaticExpressionCount is the sealed width of the authored static-expression
// family.
func (row Program) StaticExpressionCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return StaticExpressionFamily().Count(&row.Frozen, catalog)
}

// StaticExpressionAt returns one authored static expression by emitted
// ordinal.
func (row Program) StaticExpressionAt(index int) (StaticExpression, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return StaticExpression{}, false
	}
	return StaticExpressionFamily().At(&row.Frozen, catalog, index)
}

// StaticTypeValueCount is the sealed width of the authored TypeValue family.
func (row Program) StaticTypeValueCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return StaticTypeValueFamily().Count(&row.Frozen, catalog)
}

// StaticTypeValueAt returns one authored TypeValue by emitted ordinal.
func (row Program) StaticTypeValueAt(index int) (StaticTypeValue, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return StaticTypeValue{}, false
	}
	return StaticTypeValueFamily().At(&row.Frozen, catalog, index)
}

// FunctionBoundaryCount is the sealed width of the callable-interface
// family. Child formals, varargs, and captures are read through the spans on
// each boundary rather than through retained nested slices.
func (row Program) FunctionBoundaryCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return FunctionBoundaryFamily().Count(&row.Frozen, catalog)
}

func (row Program) FunctionBoundaryAt(index int) (FunctionBoundary, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return FunctionBoundary{}, false
	}
	return FunctionBoundaryFamily().At(&row.Frozen, catalog, index)
}

func (row Program) FunctionFormalCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return FunctionFormalFamily().Count(&row.Frozen, catalog)
}

func (row Program) FunctionFormalAt(index int) (FunctionFormal, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return FunctionFormal{}, false
	}
	return FunctionFormalFamily().At(&row.Frozen, catalog, index)
}

func (row Program) FunctionVarargCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return FunctionVarargFamily().Count(&row.Frozen, catalog)
}

func (row Program) FunctionVarargAt(index int) (FunctionVararg, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return FunctionVararg{}, false
	}
	return FunctionVarargFamily().At(&row.Frozen, catalog, index)
}

func (row Program) FunctionCaptureCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return FunctionCaptureFamily().Count(&row.Frozen, catalog)
}

func (row Program) FunctionCaptureAt(index int) (FunctionCapture, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return FunctionCapture{}, false
	}
	return FunctionCaptureFamily().At(&row.Frozen, catalog, index)
}

// LocalTransferCount is the sealed width of the Program's local-transfer
// family.
func (row Program) LocalTransferCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return LocalTransferFamily().Count(&row.Frozen, catalog)
}

// LocalTransferAt returns one local transfer by emitted ordinal.
func (row Program) LocalTransferAt(index int) (LocalTransfer, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return LocalTransfer{}, false
	}
	return LocalTransferFamily().At(&row.Frozen, catalog, index)
}

// LocalTransferWriteCount is the sealed width of the local-transfer write
// child family.
func (row Program) LocalTransferWriteCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return LocalTransferWriteFamily().Count(&row.Frozen, catalog)
}

// LocalTransferWriteAt returns one factor key by its dense child ordinal.
func (row Program) LocalTransferWriteAt(index int) (LocalTransferWrite, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return LocalTransferWrite{}, false
	}
	return LocalTransferWriteFamily().At(&row.Frozen, catalog, index)
}

// LocalTransferWriteFor resolves one factor key owned by a local transfer.
func (row Program) LocalTransferWriteFor(transferIndex, childIndex int) (LocalTransferWrite, bool) {
	transfer, ok := row.LocalTransferAt(transferIndex)
	if !ok || childIndex < 0 || childIndex >= transfer.WritesCount() {
		return LocalTransferWrite{}, false
	}
	offset, _, spanOK := transfer.WriteSpan()
	if !spanOK {
		return LocalTransferWrite{}, false
	}
	return row.LocalTransferWriteAt(int(offset) + childIndex)
}

func (row Program) FunctionFormalFor(boundaryIndex, childIndex int) (FunctionFormal, bool) {
	boundary, ok := row.FunctionBoundaryAt(boundaryIndex)
	if !ok || childIndex < 0 || childIndex >= boundary.FormalCount() {
		return FunctionFormal{}, false
	}
	offset, _, spanOK := boundary.FormalSpan()
	if !spanOK {
		return FunctionFormal{}, false
	}
	formal, held := row.FunctionFormalAt(int(offset) + childIndex)
	return formal, held && formal.Available()
}

func (row Program) FunctionVarargFor(boundaryIndex, childIndex int) (FunctionVararg, bool) {
	boundary, ok := row.FunctionBoundaryAt(boundaryIndex)
	if !ok || childIndex < 0 || childIndex >= 1 || !boundary.HasVararg() {
		return FunctionVararg{}, false
	}
	offset, count, spanOK := boundary.VarargSpan()
	if !spanOK || count != 1 {
		return FunctionVararg{}, false
	}
	vararg, held := row.FunctionVarargAt(int(offset) + childIndex)
	return vararg, held && vararg.Available()
}

func (row Program) FunctionCaptureFor(boundaryIndex, childIndex int) (FunctionCapture, bool) {
	boundary, ok := row.FunctionBoundaryAt(boundaryIndex)
	if !ok || childIndex < 0 || childIndex >= boundary.CaptureCount() {
		return FunctionCapture{}, false
	}
	offset, _, spanOK := boundary.CaptureSpan()
	if !spanOK {
		return FunctionCapture{}, false
	}
	capture, held := row.FunctionCaptureAt(int(offset) + childIndex)
	return capture, held && capture.InnerBodyID() == boundary.BodyID()
}

// FunctionBoundaryForBody resolves the callable boundary owned by one Body
// by bounded lookup in the canonical family. It deliberately retains no
// body-to-boundary inverse beside the sealed publication.
func (row Program) FunctionBoundaryForBody(bodyID identity.ContentID) (FunctionBoundary, bool) {
	if !bodyID.Available() {
		return FunctionBoundary{}, false
	}
	count, published := row.FunctionBoundaryCount()
	if !published {
		return FunctionBoundary{}, false
	}
	var found FunctionBoundary
	for index := 0; index < count; index++ {
		candidate, ok := row.FunctionBoundaryAt(index)
		if !ok || candidate.BodyID() != bodyID {
			continue
		}
		if found.Available() {
			return FunctionBoundary{}, false
		}
		found = candidate
	}
	return found, found.Available()
}
