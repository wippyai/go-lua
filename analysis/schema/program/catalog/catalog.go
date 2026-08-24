// Package catalog is the one declaration source for the cold Program family
// catalog. It deliberately knows nothing about the row types in its parent
// package: a Definition is only an opaque slot/name pair that the parent
// binds to a typed Family.
package catalog

import "github.com/wippyai/go-lua/analysis/identity"

// Definition names one column in the cold Program publication. Its fields are
// private so callers can consume a declaration but cannot manufacture a
// second, unregistered slot/name pair.
type Definition struct {
	slot uint32
	name string
}

// Slot is the stable snapshot slot occupied by the declaration.
func (definition Definition) Slot() uint32 { return definition.slot }

// Name is the stable family name used to derive the family's denominator.
func (definition Definition) Name() string { return definition.name }

// Valid reports whether the declaration has a family name. Every definition
// in the manifest is valid; this is useful to consumers that accept a
// definition as an opaque value.
func (definition Definition) Valid() bool { return definition.name != "" }

const catalogDomain = "analysis/cold-catalog/v1"

// CatalogID is the identity a compiled program's cold publication is sealed
// under, derived from the runtime schema identity the program was compiled
// against. An unavailable runtime schema derives no catalog.
func CatalogID(runtimeSchema identity.ContentID) (identity.ContentID, bool) {
	if !runtimeSchema.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(catalogDomain, runtimeSchema[:])
}

// Denominator is the identity of this family's ordinal universe inside one
// cold catalog. A definition with no name or a catalog with no identity is not
// a declaration that can be addressed.
func (definition Definition) Denominator(catalogID identity.ContentID) (identity.ContentID, bool) {
	if !definition.Valid() || !catalogID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(catalogDomain+"/"+definition.name, catalogID[:])
}

const (
	callTargetIndex = iota
	heapAllocationIndex
	heapFieldIndex
	valuesIndex
	valuesMemberIndex
	heapIndexIndex
	exactScalarSummaryIndex
	arithmeticSummaryIndex
	unarySummaryIndex
	pointIndex
	pointDecisionIndex
	callIndex
	callOperandIndex
	callArgumentIndex
	callTypeArgumentIndex
	environmentEdgeIndex
	environmentResetIndex
	staticTypeValueIndex
	staticExpressionIndex
	regionIndex
	regionMemberIndex
	wtoEventIndex
	bodyIndex
	bodyEntryIndex
	bodyRootIndex
	outcomeIndex
	outcomeReturnValueIndex
	outcomePointIndex
	functionBoundaryIndex
	functionFormalIndex
	functionVarargIndex
	functionCaptureIndex
	staticInputIndex
	localTransferIndex
	localTransferWriteIndex
	occurrenceIndex
	occurrencePointIndex
	occurrenceInputIndex
	ruleOccurrenceIndex
	diagnosticObservationIndex
	diagnosticEvidenceIndex
	diagnosticPathIndex
	staticTypeNodeIndex
	staticTypeNodeUnionMemberIndex
	staticTypeNodeIntersectionMemberIndex
	staticTypeNodeGenericArgumentIndex
	staticTypeNodeAliasParameterIndex
	staticTypeNodeInterfaceExtendIndex
	staticTypeNodeInterfaceMemberIndex
	staticTypeNodeTypeFunctionTypeParameterIndex
	staticTypeNodeTypeFunctionParameterIndex
	staticTypeNodeTypeFunctionReturnIndex
	staticTypeNodeRecordFieldIndex
	staticTypeNodeReferenceSourceKeyIndex
	staticTypeNodeReferenceCanonicalKeyIndex
	storageCellLifetimeIndex
	subjectLivenessSpanIndex
	subjectYieldBoundaryIndex
	callResultIndex
	subjectEventIndex
	subjectAliasRouteScopeIndex
	subjectAliasRouteScopeMemberIndex
	subjectAliasCandidateIndex
	moduleImportIndex
	moduleRequestIndex
	moduleEntryIndex
	moduleEntryRootCellIndex
	moduleEntryMemberIndex
	moduleEntryRootFunctionIndex
	callResultSlotIndex
	manifestSize
)

// manifest is the complete append-only declaration. The explicit slot
// numbers are intentional: they make the historical wire addresses visible
// at the one source of truth and avoid deriving any address in another file.
var manifest = [...]Definition{
	callTargetIndex:                              {slot: 0, name: "call-target"},
	heapAllocationIndex:                          {slot: 1, name: "heap-allocation"},
	heapFieldIndex:                               {slot: 2, name: "heap-field"},
	valuesIndex:                                  {slot: 3, name: "values"},
	valuesMemberIndex:                            {slot: 4, name: "values-member"},
	heapIndexIndex:                               {slot: 5, name: "heap-index"},
	exactScalarSummaryIndex:                      {slot: 6, name: "exact-scalar-summary"},
	arithmeticSummaryIndex:                       {slot: 7, name: "arithmetic-summary"},
	unarySummaryIndex:                            {slot: 8, name: "unary-summary"},
	pointIndex:                                   {slot: 9, name: "point"},
	pointDecisionIndex:                           {slot: 10, name: "point-decision"},
	callIndex:                                    {slot: 11, name: "call"},
	callOperandIndex:                             {slot: 12, name: "call-operand"},
	callArgumentIndex:                            {slot: 13, name: "call-argument"},
	callTypeArgumentIndex:                        {slot: 14, name: "call-type-argument"},
	environmentEdgeIndex:                         {slot: 15, name: "environment-edge"},
	environmentResetIndex:                        {slot: 16, name: "environment-reset"},
	staticTypeValueIndex:                         {slot: 17, name: "static-type-value"},
	staticExpressionIndex:                        {slot: 18, name: "static-expression"},
	regionIndex:                                  {slot: 19, name: "region"},
	regionMemberIndex:                            {slot: 20, name: "region-member"},
	wtoEventIndex:                                {slot: 21, name: "wto-event"},
	bodyIndex:                                    {slot: 22, name: "body"},
	bodyEntryIndex:                               {slot: 23, name: "body-entry"},
	bodyRootIndex:                                {slot: 24, name: "body-root"},
	outcomeIndex:                                 {slot: 25, name: "outcome"},
	outcomeReturnValueIndex:                      {slot: 26, name: "outcome-return-value"},
	outcomePointIndex:                            {slot: 27, name: "outcome-point"},
	functionBoundaryIndex:                        {slot: 28, name: "function-boundary"},
	functionFormalIndex:                          {slot: 29, name: "function-formal"},
	functionVarargIndex:                          {slot: 30, name: "function-vararg"},
	functionCaptureIndex:                         {slot: 31, name: "function-capture"},
	staticInputIndex:                             {slot: 32, name: "static-input"},
	localTransferIndex:                           {slot: 33, name: "local-transfer"},
	localTransferWriteIndex:                      {slot: 34, name: "local-transfer-write"},
	occurrenceIndex:                              {slot: 35, name: "occurrence"},
	occurrencePointIndex:                         {slot: 36, name: "occurrence-point"},
	occurrenceInputIndex:                         {slot: 37, name: "occurrence-input"},
	ruleOccurrenceIndex:                          {slot: 38, name: "rule-occurrence"},
	diagnosticObservationIndex:                   {slot: 39, name: "diagnostic-observation"},
	diagnosticEvidenceIndex:                      {slot: 40, name: "diagnostic-evidence"},
	diagnosticPathIndex:                          {slot: 41, name: "diagnostic-path"},
	staticTypeNodeIndex:                          {slot: 42, name: "static-type-node"},
	staticTypeNodeUnionMemberIndex:               {slot: 43, name: "static-type-node-union-member"},
	staticTypeNodeIntersectionMemberIndex:        {slot: 44, name: "static-type-node-intersection-member"},
	staticTypeNodeGenericArgumentIndex:           {slot: 45, name: "static-type-node-generic-argument"},
	staticTypeNodeAliasParameterIndex:            {slot: 46, name: "static-type-node-alias-parameter"},
	staticTypeNodeInterfaceExtendIndex:           {slot: 47, name: "static-type-node-interface-extend"},
	staticTypeNodeInterfaceMemberIndex:           {slot: 48, name: "static-type-node-interface-member"},
	staticTypeNodeTypeFunctionTypeParameterIndex: {slot: 49, name: "static-type-node-type-function-type-parameter"},
	staticTypeNodeTypeFunctionParameterIndex:     {slot: 50, name: "static-type-node-type-function-parameter"},
	staticTypeNodeTypeFunctionReturnIndex:        {slot: 51, name: "static-type-node-type-function-return"},
	staticTypeNodeRecordFieldIndex:               {slot: 52, name: "static-type-node-record-field"},
	staticTypeNodeReferenceSourceKeyIndex:        {slot: 53, name: "static-type-node-reference-source-key"},
	staticTypeNodeReferenceCanonicalKeyIndex:     {slot: 54, name: "static-type-node-reference-canonical-key"},
	storageCellLifetimeIndex:                     {slot: 55, name: "storage-cell-lifetime"},
	// The live-range plane replaces the per-pair subject-liveness family and
	// takes its column: a catalog identity is derived from the schema it was
	// compiled against, so no snapshot addresses slot 56 under both shapes.
	subjectLivenessSpanIndex:          {slot: 56, name: "subject-liveness-span"},
	subjectYieldBoundaryIndex:         {slot: 57, name: "subject-yield-boundary"},
	callResultIndex:                   {slot: 58, name: "call-result"},
	subjectEventIndex:                 {slot: 59, name: "subject-event"},
	subjectAliasRouteScopeIndex:       {slot: 60, name: "subject-alias-route-scope"},
	subjectAliasRouteScopeMemberIndex: {slot: 61, name: "subject-alias-route-scope-member"},
	subjectAliasCandidateIndex:        {slot: 62, name: "subject-alias-candidate"},
	moduleImportIndex:                 {slot: 63, name: "module-import"},
	moduleRequestIndex:                {slot: 64, name: "module-request"},
	moduleEntryIndex:                  {slot: 65, name: "module-entry"},
	moduleEntryRootCellIndex:          {slot: 66, name: "module-entry-root-cell"},
	moduleEntryMemberIndex:            {slot: 67, name: "module-entry-member"},
	moduleEntryRootFunctionIndex:      {slot: 68, name: "module-entry-root-function"},
	callResultSlotIndex:               {slot: 69, name: "call-result-slot"},
}

// DefinitionCount is the number of declarations in the complete manifest.
func DefinitionCount() int { return manifestSize }

// DefinitionAt enumerates the complete manifest without exposing its backing
// array.
func DefinitionAt(index int) (Definition, bool) {
	if index < 0 || index >= len(manifest) {
		return Definition{}, false
	}
	return manifest[index], true
}

// Manifest returns a copy of the complete declaration manifest.
func Manifest() []Definition {
	result := make([]Definition, len(manifest))
	copy(result, manifest[:])
	return result
}

func CallTarget() Definition                { return manifest[callTargetIndex] }
func HeapAllocation() Definition            { return manifest[heapAllocationIndex] }
func HeapField() Definition                 { return manifest[heapFieldIndex] }
func Values() Definition                    { return manifest[valuesIndex] }
func ValuesMember() Definition              { return manifest[valuesMemberIndex] }
func HeapIndex() Definition                 { return manifest[heapIndexIndex] }
func ExactScalarSummary() Definition        { return manifest[exactScalarSummaryIndex] }
func ArithmeticSummary() Definition         { return manifest[arithmeticSummaryIndex] }
func UnarySummary() Definition              { return manifest[unarySummaryIndex] }
func Point() Definition                     { return manifest[pointIndex] }
func PointDecision() Definition             { return manifest[pointDecisionIndex] }
func Call() Definition                      { return manifest[callIndex] }
func CallOperand() Definition               { return manifest[callOperandIndex] }
func CallArgument() Definition              { return manifest[callArgumentIndex] }
func CallTypeArgument() Definition          { return manifest[callTypeArgumentIndex] }
func EnvironmentEdge() Definition           { return manifest[environmentEdgeIndex] }
func EnvironmentReset() Definition          { return manifest[environmentResetIndex] }
func StaticTypeValue() Definition           { return manifest[staticTypeValueIndex] }
func StaticExpression() Definition          { return manifest[staticExpressionIndex] }
func Region() Definition                    { return manifest[regionIndex] }
func RegionMember() Definition              { return manifest[regionMemberIndex] }
func WTOEvent() Definition                  { return manifest[wtoEventIndex] }
func Body() Definition                      { return manifest[bodyIndex] }
func BodyEntry() Definition                 { return manifest[bodyEntryIndex] }
func BodyRoot() Definition                  { return manifest[bodyRootIndex] }
func Outcome() Definition                   { return manifest[outcomeIndex] }
func OutcomeReturnValue() Definition        { return manifest[outcomeReturnValueIndex] }
func OutcomePoint() Definition              { return manifest[outcomePointIndex] }
func FunctionBoundary() Definition          { return manifest[functionBoundaryIndex] }
func FunctionFormal() Definition            { return manifest[functionFormalIndex] }
func FunctionVararg() Definition            { return manifest[functionVarargIndex] }
func FunctionCapture() Definition           { return manifest[functionCaptureIndex] }
func StaticInput() Definition               { return manifest[staticInputIndex] }
func LocalTransfer() Definition             { return manifest[localTransferIndex] }
func LocalTransferWrite() Definition        { return manifest[localTransferWriteIndex] }
func Occurrence() Definition                { return manifest[occurrenceIndex] }
func OccurrencePoint() Definition           { return manifest[occurrencePointIndex] }
func OccurrenceInput() Definition           { return manifest[occurrenceInputIndex] }
func RuleOccurrence() Definition            { return manifest[ruleOccurrenceIndex] }
func DiagnosticObservation() Definition     { return manifest[diagnosticObservationIndex] }
func DiagnosticEvidence() Definition        { return manifest[diagnosticEvidenceIndex] }
func DiagnosticPath() Definition            { return manifest[diagnosticPathIndex] }
func StaticTypeNode() Definition            { return manifest[staticTypeNodeIndex] }
func StaticTypeNodeUnionMember() Definition { return manifest[staticTypeNodeUnionMemberIndex] }
func StaticTypeNodeIntersectionMember() Definition {
	return manifest[staticTypeNodeIntersectionMemberIndex]
}
func StaticTypeNodeGenericArgument() Definition { return manifest[staticTypeNodeGenericArgumentIndex] }
func StaticTypeNodeAliasParameter() Definition  { return manifest[staticTypeNodeAliasParameterIndex] }
func StaticTypeNodeInterfaceExtend() Definition { return manifest[staticTypeNodeInterfaceExtendIndex] }
func StaticTypeNodeInterfaceMember() Definition { return manifest[staticTypeNodeInterfaceMemberIndex] }
func StaticTypeNodeTypeFunctionTypeParameter() Definition {
	return manifest[staticTypeNodeTypeFunctionTypeParameterIndex]
}
func StaticTypeNodeTypeFunctionParameter() Definition {
	return manifest[staticTypeNodeTypeFunctionParameterIndex]
}
func StaticTypeNodeTypeFunctionReturn() Definition {
	return manifest[staticTypeNodeTypeFunctionReturnIndex]
}
func StaticTypeNodeRecordField() Definition { return manifest[staticTypeNodeRecordFieldIndex] }
func StaticTypeNodeReferenceSourceKey() Definition {
	return manifest[staticTypeNodeReferenceSourceKeyIndex]
}
func StaticTypeNodeReferenceCanonicalKey() Definition {
	return manifest[staticTypeNodeReferenceCanonicalKeyIndex]
}
func StorageCellLifetime() Definition    { return manifest[storageCellLifetimeIndex] }
func SubjectYieldBoundary() Definition   { return manifest[subjectYieldBoundaryIndex] }
func SubjectLivenessSpan() Definition    { return manifest[subjectLivenessSpanIndex] }
func CallResult() Definition             { return manifest[callResultIndex] }
func SubjectEvent() Definition           { return manifest[subjectEventIndex] }
func SubjectAliasRouteScope() Definition { return manifest[subjectAliasRouteScopeIndex] }
func SubjectAliasRouteScopeMember() Definition {
	return manifest[subjectAliasRouteScopeMemberIndex]
}
func SubjectAliasCandidate() Definition   { return manifest[subjectAliasCandidateIndex] }
func ModuleImport() Definition            { return manifest[moduleImportIndex] }
func ModuleRequest() Definition           { return manifest[moduleRequestIndex] }
func ModuleEntry() Definition             { return manifest[moduleEntryIndex] }
func ModuleEntryRootCell() Definition     { return manifest[moduleEntryRootCellIndex] }
func ModuleEntryMember() Definition       { return manifest[moduleEntryMemberIndex] }
func ModuleEntryRootFunction() Definition { return manifest[moduleEntryRootFunctionIndex] }
func CallResultSlot() Definition          { return manifest[callResultSlotIndex] }
