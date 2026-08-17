package program

import "github.com/wippyai/go-lua/analysis/program/semanticsource"

// programSemanticSourceDefinitions snapshots the complete generated
// declaration table supplied by the sealed schema. The schema is the sole
// denominator; this function only filters it to Program-owned origins.
func programSemanticSourceDefinitions(schema semanticsource.ProgramSchema) ([]semanticsource.RelationDef, bool) {
	if schema == nil || schema.Count() == 0 {
		return nil, false
	}
	definitions := make([]semanticsource.RelationDef, 0, programSemanticSourcePublicationCount)
	for index := 0; index < schema.Count(); index++ {
		definition, ok := schema.DefinitionAt(index)
		if !ok {
			return nil, false
		}
		switch definition.Token().Origin() {
		case semanticsource.OriginProgramSourceProvenance,
			semanticsource.OriginProgramSourceOrder,
			semanticsource.OriginProgramSourceKey,
			semanticsource.OriginProgramSourceExactKey,
			semanticsource.OriginProgramSourceControlFault,
			semanticsource.OriginProgramFlowLiterals,
			semanticsource.OriginProgramFlowValues,
			semanticsource.OriginProgramFlowLens,
			semanticsource.OriginProgramFlowStorage,
			semanticsource.OriginProgramFlowConstructors,
			semanticsource.OriginProgramFlowOperators,
			semanticsource.OriginProgramFlowFunction,
			semanticsource.OriginProgramFlowCall,
			semanticsource.OriginProgramFlowControl,
			semanticsource.OriginProgramFlowClaim,
			semanticsource.OriginProgramFlowTypeValue,
			semanticsource.OriginProgramFlowOutcome,
			semanticsource.OriginProgramFlowTransfer,
			semanticsource.OriginProgramFlowBody,
			semanticsource.OriginProgramStatic,
			semanticsource.OriginProgramModuleImport,
			semanticsource.OriginProgramModuleEntry:
			definitions = append(definitions, definition)
		}
	}
	return definitions, len(definitions) == programSemanticSourcePublicationCount
}

// programOwnerCount maps one generated semantic-source row to the cardinality
// measured by its canonical typed owner. The generated token is the only
// dispatch vocabulary; no owner row is copied into Program.
func programOwnerCount(token semanticsource.Token, sourceCounts [8]int, flowCounts [33]int, staticCounts [10]int, moduleCounts [6]int) (int, bool) {
	switch token.Origin() {
	case semanticsource.OriginProgramSourceProvenance:
		return sourceCounts[0], token.Facet() == 0
	case semanticsource.OriginProgramSourceOrder:
		return sourceCounts[1], token.Facet() == 0
	case semanticsource.OriginProgramSourceKey:
		return sourceCounts[2], token.Facet() == 0
	case semanticsource.OriginProgramSourceExactKey:
		return sourceCounts[3], token.Facet() == 0
	case semanticsource.OriginProgramSourceControlFault:
		return sourceCounts[4], token.Facet() == 0
	case semanticsource.OriginProgramFlowLiterals:
		return sourceCounts[5], token.Facet() == 0
	case semanticsource.OriginProgramFlowBody:
		switch token.Facet() {
		case 0:
			return sourceCounts[6], true
		case semanticsource.FacetProgramFlowBodyRoots:
			return sourceCounts[7], true
		}
	case semanticsource.OriginProgramFlowValues:
		if token.Facet() == 0 {
			return flowCounts[0], true
		}
		if token.Facet() == semanticsource.FacetProgramFlowValueOccurrence {
			return flowCounts[1], true
		}
	case semanticsource.OriginProgramFlowLens:
		return flowCounts[2], token.Facet() == 0
	case semanticsource.OriginProgramFlowStorage:
		if token.Facet() <= semanticsource.FacetProgramFlowStorageBind {
			return flowCounts[3+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowConstructors:
		if token.Facet() <= semanticsource.FacetProgramFlowConstructorField {
			return flowCounts[11+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowOperators:
		if token.Facet() <= semanticsource.FacetProgramFlowIndexSet {
			return flowCounts[13+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowFunction:
		if token.Facet() <= semanticsource.FacetProgramFlowFunctionCapture {
			return flowCounts[23+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowCall:
		if token.Facet() <= semanticsource.FacetProgramFlowDirectCallBinding {
			return flowCounts[25+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowControl:
		if token.Facet() <= semanticsource.FacetProgramFlowGenericFor {
			return flowCounts[27+int(token.Facet())], true
		}
	case semanticsource.OriginProgramFlowClaim:
		return flowCounts[29], token.Facet() == 0
	case semanticsource.OriginProgramFlowTypeValue:
		return flowCounts[30], token.Facet() == 0
	case semanticsource.OriginProgramFlowOutcome:
		return flowCounts[31], token.Facet() == 0
	case semanticsource.OriginProgramFlowTransfer:
		return flowCounts[32], token.Facet() == 0
	case semanticsource.OriginProgramStatic:
		switch token.Facet() {
		case 0:
			return staticCounts[0], true
		case semanticsource.FacetProgramStaticFunctionContract:
			return staticCounts[1], true
		case semanticsource.FacetProgramStaticCallTypeArguments:
			return staticCounts[2], true
		case semanticsource.FacetProgramStaticCellDeclaredType:
			return staticCounts[3], true
		case semanticsource.FacetProgramStaticClaimTarget:
			return staticCounts[4], true
		case semanticsource.FacetProgramStaticTypeValueTarget:
			return staticCounts[5], true
		case semanticsource.FacetProgramStaticTypeof:
			return staticCounts[6], true
		case semanticsource.FacetProgramStaticAnnotation:
			return staticCounts[7], true
		case semanticsource.FacetProgramStaticPublication:
			return staticCounts[8], true
		case semanticsource.FacetProgramStaticTypeRef:
			return staticCounts[9], true
		}
	case semanticsource.OriginProgramModuleImport:
		if token.Facet() <= semanticsource.FacetProgramModuleRequest {
			return moduleCounts[int(token.Facet())], true
		}
	case semanticsource.OriginProgramModuleEntry:
		if token.Facet() <= semanticsource.FacetProgramModuleEntryRootFunction {
			return moduleCounts[2+int(token.Facet())], true
		}
	}
	return 0, false
}
