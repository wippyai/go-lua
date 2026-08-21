package programschema

import (
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
)

// The accessors below are the complete typed binding of the root Program
// family catalog. Their definitions and slot/name values are owned solely by
// analysis/schema/program/catalog; the neutral family operations live in
// analysis/schema/program/family.
func ValuesFamily() programfamily.Family[Values] {
	return programfamily.New[Values](programcatalog.Values())
}
func ValuesMemberFamily() programfamily.Family[ValuesMember] {
	return programfamily.New[ValuesMember](programcatalog.ValuesMember())
}
func ExactScalarSummaryFamily() programfamily.Family[ExactScalarSummary] {
	return programfamily.New[ExactScalarSummary](programcatalog.ExactScalarSummary())
}
func ArithmeticSummaryFamily() programfamily.Family[ArithmeticSummary] {
	return programfamily.New[ArithmeticSummary](programcatalog.ArithmeticSummary())
}
func UnarySummaryFamily() programfamily.Family[UnarySummary] {
	return programfamily.New[UnarySummary](programcatalog.UnarySummary())
}
func PointFamily() programfamily.Family[Point] {
	return programfamily.New[Point](programcatalog.Point())
}
func PointDecisionFamily() programfamily.Family[PointDecision] {
	return programfamily.New[PointDecision](programcatalog.PointDecision())
}
func CallFamily() programfamily.Family[Call] {
	return programfamily.New[Call](programcatalog.Call())
}
func CallOperandFamily() programfamily.Family[CallOperand] {
	return programfamily.New[CallOperand](programcatalog.CallOperand())
}
func CallArgumentFamily() programfamily.Family[CallArgument] {
	return programfamily.New[CallArgument](programcatalog.CallArgument())
}
func CallTypeArgumentFamily() programfamily.Family[CallTypeArgument] {
	return programfamily.New[CallTypeArgument](programcatalog.CallTypeArgument())
}
func EnvironmentEdgeFamily() programfamily.Family[EnvironmentEdge] {
	return programfamily.New[EnvironmentEdge](programcatalog.EnvironmentEdge())
}
func EnvironmentResetFamily() programfamily.Family[EnvironmentReset] {
	return programfamily.New[EnvironmentReset](programcatalog.EnvironmentReset())
}
func StaticTypeValueFamily() programfamily.Family[StaticTypeValue] {
	return programfamily.New[StaticTypeValue](programcatalog.StaticTypeValue())
}
func StaticExpressionFamily() programfamily.Family[StaticExpression] {
	return programfamily.New[StaticExpression](programcatalog.StaticExpression())
}
func RegionFamily() programfamily.Family[Region] {
	return programfamily.New[Region](programcatalog.Region())
}
func RegionMemberFamily() programfamily.Family[RegionMember] {
	return programfamily.New[RegionMember](programcatalog.RegionMember())
}
func WTOEventFamily() programfamily.Family[WTOEvent] {
	return programfamily.New[WTOEvent](programcatalog.WTOEvent())
}
func BodyFamily() programfamily.Family[Body] {
	return programfamily.New[Body](programcatalog.Body())
}
func BodyEntryFamily() programfamily.Family[BodyEntry] {
	return programfamily.New[BodyEntry](programcatalog.BodyEntry())
}
func BodyRootFamily() programfamily.Family[BodyRoot] {
	return programfamily.New[BodyRoot](programcatalog.BodyRoot())
}
func OutcomeFamily() programfamily.Family[Outcome] {
	return programfamily.New[Outcome](programcatalog.Outcome())
}
func OutcomeReturnValueFamily() programfamily.Family[OutcomeReturnValue] {
	return programfamily.New[OutcomeReturnValue](programcatalog.OutcomeReturnValue())
}
func OutcomePointFamily() programfamily.Family[OutcomePoint] {
	return programfamily.New[OutcomePoint](programcatalog.OutcomePoint())
}
func FunctionBoundaryFamily() programfamily.Family[FunctionBoundary] {
	return programfamily.New[FunctionBoundary](programcatalog.FunctionBoundary())
}
func FunctionFormalFamily() programfamily.Family[FunctionFormal] {
	return programfamily.New[FunctionFormal](programcatalog.FunctionFormal())
}
func FunctionVarargFamily() programfamily.Family[FunctionVararg] {
	return programfamily.New[FunctionVararg](programcatalog.FunctionVararg())
}
func FunctionCaptureFamily() programfamily.Family[FunctionCapture] {
	return programfamily.New[FunctionCapture](programcatalog.FunctionCapture())
}
func StaticInputFamily() programfamily.Family[StaticInput] {
	return programfamily.New[StaticInput](programcatalog.StaticInput())
}
func LocalTransferFamily() programfamily.Family[LocalTransfer] {
	return programfamily.New[LocalTransfer](programcatalog.LocalTransfer())
}
func LocalTransferWriteFamily() programfamily.Family[LocalTransferWrite] {
	return programfamily.New[LocalTransferWrite](programcatalog.LocalTransferWrite())
}
func OccurrenceFamily() programfamily.Family[Occurrence] {
	return programfamily.New[Occurrence](programcatalog.Occurrence())
}
func OccurrencePointFamily() programfamily.Family[OccurrencePoint] {
	return programfamily.New[OccurrencePoint](programcatalog.OccurrencePoint())
}
func OccurrenceInputFamily() programfamily.Family[OccurrenceInput] {
	return programfamily.New[OccurrenceInput](programcatalog.OccurrenceInput())
}
func RuleOccurrenceFamily() programfamily.Family[RuleOccurrence] {
	return programfamily.New[RuleOccurrence](programcatalog.RuleOccurrence())
}
func CallResultFamily() programfamily.Family[CallResult] {
	return programfamily.New[CallResult](programcatalog.CallResult())
}
func ModuleImportFamily() programfamily.Family[ModuleImport] {
	return programfamily.New[ModuleImport](programcatalog.ModuleImport())
}
func ModuleRequestFamily() programfamily.Family[ModuleRequest] {
	return programfamily.New[ModuleRequest](programcatalog.ModuleRequest())
}
func ModuleEntryFamily() programfamily.Family[ModuleEntry] {
	return programfamily.New[ModuleEntry](programcatalog.ModuleEntry())
}
func ModuleEntryRootCellFamily() programfamily.Family[ModuleEntryRootCell] {
	return programfamily.New[ModuleEntryRootCell](programcatalog.ModuleEntryRootCell())
}
func ModuleEntryRootFunctionFamily() programfamily.Family[ModuleEntryRootFunction] {
	return programfamily.New[ModuleEntryRootFunction](programcatalog.ModuleEntryRootFunction())
}
func CallResultSlotFamily() programfamily.Family[CallResultSlot] {
	return programfamily.New[CallResultSlot](programcatalog.CallResultSlot())
}
func ModuleEntryMemberFamily() programfamily.Family[ModuleEntryMember] {
	return programfamily.New[ModuleEntryMember](programcatalog.ModuleEntryMember())
}
