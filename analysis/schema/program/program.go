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
