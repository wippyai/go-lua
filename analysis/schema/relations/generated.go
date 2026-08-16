// Code generated from catalog.schema; DO NOT EDIT.

package relations

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

func catalogDefinition(origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.RelationDef {
	definition, _ := semanticsource.Declare(origin, facet)
	return definition
}

// CatalogToken resolves one generated schema name for cold claims/codegen only.
func CatalogToken(name string) (semanticsource.Token, bool) {
	switch name {
	case "ProgramSourceProvenance@-":
		definition := catalogDefinition(semanticsource.OriginProgramSourceProvenance, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramSourceOrder@-":
		definition := catalogDefinition(semanticsource.OriginProgramSourceOrder, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramSourceKey@-":
		definition := catalogDefinition(semanticsource.OriginProgramSourceKey, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramSourceExactKey@-":
		definition := catalogDefinition(semanticsource.OriginProgramSourceExactKey, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramSourceControlFault@-":
		definition := catalogDefinition(semanticsource.OriginProgramSourceControlFault, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowLiterals@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowLiterals, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowValues@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowValues, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowValues@ProgramFlowValueOccurrence":
		definition := catalogDefinition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowLens@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowLens, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@ProgramFlowStorageCell":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@ProgramFlowStorageGlobal":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@ProgramFlowStorageRead":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@ProgramFlowStorageAssign":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@ProgramFlowStorageWrite":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@ProgramFlowStorageVararg":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowStorage@ProgramFlowStorageBind":
		definition := catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageBind)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowConstructors@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowConstructors, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowConstructors@ProgramFlowConstructorField":
		definition := catalogDefinition(semanticsource.OriginProgramFlowConstructors, semanticsource.FacetProgramFlowConstructorField)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowUnaryNumeric":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowLength":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowArithmetic":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowBitwise":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowConcat":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowEquality":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowOrder":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowIndexGet":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOperators@ProgramFlowIndexSet":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowFunction@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowFunction, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowFunction@ProgramFlowFunctionCapture":
		definition := catalogDefinition(semanticsource.OriginProgramFlowFunction, semanticsource.FacetProgramFlowFunctionCapture)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowCall@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowCall, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowCall@ProgramFlowDirectCallBinding":
		definition := catalogDefinition(semanticsource.OriginProgramFlowCall, semanticsource.FacetProgramFlowDirectCallBinding)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowControl@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowControl, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowControl@ProgramFlowGenericFor":
		definition := catalogDefinition(semanticsource.OriginProgramFlowControl, semanticsource.FacetProgramFlowGenericFor)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowClaim@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowClaim, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowTypeValue@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowTypeValue, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowOutcome@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowOutcome, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowTransfer@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowTransfer, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowBody@-":
		definition := catalogDefinition(semanticsource.OriginProgramFlowBody, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramFlowBody@ProgramFlowBodyRoots":
		definition := catalogDefinition(semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@-":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticFunctionContract":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticFunctionContract)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticCallTypeArguments":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCallTypeArguments)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticCellDeclaredType":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCellDeclaredType)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticClaimTarget":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticClaimTarget)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticTypeValueTarget":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticTypeof":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeof)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticAnnotation":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticPublication":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramStatic@ProgramStaticTypeRef":
		definition := catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramModuleImport@-":
		definition := catalogDefinition(semanticsource.OriginProgramModuleImport, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramModuleImport@ProgramModuleRequest":
		definition := catalogDefinition(semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramModuleEntry@-":
		definition := catalogDefinition(semanticsource.OriginProgramModuleEntry, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramModuleEntry@ProgramModuleEntryRootCell":
		definition := catalogDefinition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootCell)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramModuleEntry@ProgramModuleEntryMember":
		definition := catalogDefinition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryMember)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "ProgramModuleEntry@ProgramModuleEntryRootFunction":
		definition := catalogDefinition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootFunction)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetContract@-":
		definition := catalogDefinition(semanticsource.OriginTargetContract, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@-":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetABI":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetSubedge":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetCallback":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetBinding":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetResume":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetSpawn":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetOpaque":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetOperationEffect":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetCallbackEffect":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetCallbackRelease":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetOutcome":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetTransfer":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetTransferOutcome":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetSuspension":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetResumeOutcome":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResumeOutcome)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetSpawnSibling":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawnSibling)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetSubedgeArgumentOrigin":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedgeArgumentOrigin)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetCallbackResult":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackResult)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetResultAlias":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResultAlias)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetProduced":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetProducedCapture":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProducedCapture)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetFreshResult":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetFreshResult)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetOperation@TargetPublicationEffect":
		definition := catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetProtocol@-":
		definition := catalogDefinition(semanticsource.OriginTargetProtocol, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetProtocol@TargetProtocolState":
		definition := catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetProtocol@TargetProtocolAcquisition":
		definition := catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetProtocol@TargetProtocolTransition":
		definition := catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetProtocol@TargetProtocolTransitionOutcome":
		definition := catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetProtocol@TargetProtocolEscape":
		definition := catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetProtocol@TargetProtocolCallbackHolder":
		definition := catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetBoot@-":
		definition := catalogDefinition(semanticsource.OriginTargetBoot, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetBoot@TargetBootEntry":
		definition := catalogDefinition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootEntry)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetBoot@TargetBootMetatableAttachment":
		definition := catalogDefinition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootMetatableAttachment)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetBoot@TargetBootBinding":
		definition := catalogDefinition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootBinding)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "TargetGsub@-":
		definition := catalogDefinition(semanticsource.OriginTargetGsub, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkProjectShardMount@-":
		definition := catalogDefinition(semanticsource.OriginLinkProjectShardMount, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkProjectBaseApplication@-":
		definition := catalogDefinition(semanticsource.OriginLinkProjectBaseApplication, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkBoundary@-":
		definition := catalogDefinition(semanticsource.OriginLinkBoundary, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@-":
		definition := catalogDefinition(semanticsource.OriginLinkModule, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@LinkModuleCache":
		definition := catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@LinkModuleRepresentative":
		definition := catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@LinkModuleTransport":
		definition := catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@LinkModuleAnalysisRoot":
		definition := catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@LinkModuleInitGeneration":
		definition := catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@LinkModuleInitOutcome":
		definition := catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkModule@LinkModuleInitTerminal":
		definition := catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkStatic@-":
		definition := catalogDefinition(semanticsource.OriginLinkStatic, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkHost@-":
		definition := catalogDefinition(semanticsource.OriginLinkHost, 0)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkHost@LinkHostExposure":
		definition := catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkHost@LinkHostBoot":
		definition := catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkHost@LinkHostMember":
		definition := catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	case "LinkHost@LinkHostEndpointTarget":
		definition := catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget)
		return definition.Token(), definition != (semanticsource.RelationDef{})
	default:
		return semanticsource.Token{}, false
	}
}

// CatalogName is the exact inverse of CatalogToken for issued catalog tokens.
func CatalogName(token semanticsource.Token) (string, bool) {
	if token == catalogDefinition(semanticsource.OriginProgramSourceProvenance, 0).Token() {
		return "ProgramSourceProvenance@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramSourceOrder, 0).Token() {
		return "ProgramSourceOrder@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramSourceKey, 0).Token() {
		return "ProgramSourceKey@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramSourceExactKey, 0).Token() {
		return "ProgramSourceExactKey@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramSourceControlFault, 0).Token() {
		return "ProgramSourceControlFault@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowLiterals, 0).Token() {
		return "ProgramFlowLiterals@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowValues, 0).Token() {
		return "ProgramFlowValues@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence).Token() {
		return "ProgramFlowValues@ProgramFlowValueOccurrence", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowLens, 0).Token() {
		return "ProgramFlowLens@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, 0).Token() {
		return "ProgramFlowStorage@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell).Token() {
		return "ProgramFlowStorage@ProgramFlowStorageCell", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal).Token() {
		return "ProgramFlowStorage@ProgramFlowStorageGlobal", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead).Token() {
		return "ProgramFlowStorage@ProgramFlowStorageRead", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign).Token() {
		return "ProgramFlowStorage@ProgramFlowStorageAssign", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite).Token() {
		return "ProgramFlowStorage@ProgramFlowStorageWrite", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg).Token() {
		return "ProgramFlowStorage@ProgramFlowStorageVararg", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageBind).Token() {
		return "ProgramFlowStorage@ProgramFlowStorageBind", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowConstructors, 0).Token() {
		return "ProgramFlowConstructors@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowConstructors, semanticsource.FacetProgramFlowConstructorField).Token() {
		return "ProgramFlowConstructors@ProgramFlowConstructorField", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, 0).Token() {
		return "ProgramFlowOperators@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric).Token() {
		return "ProgramFlowOperators@ProgramFlowUnaryNumeric", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength).Token() {
		return "ProgramFlowOperators@ProgramFlowLength", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic).Token() {
		return "ProgramFlowOperators@ProgramFlowArithmetic", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise).Token() {
		return "ProgramFlowOperators@ProgramFlowBitwise", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat).Token() {
		return "ProgramFlowOperators@ProgramFlowConcat", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality).Token() {
		return "ProgramFlowOperators@ProgramFlowEquality", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder).Token() {
		return "ProgramFlowOperators@ProgramFlowOrder", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet).Token() {
		return "ProgramFlowOperators@ProgramFlowIndexGet", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet).Token() {
		return "ProgramFlowOperators@ProgramFlowIndexSet", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowFunction, 0).Token() {
		return "ProgramFlowFunction@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowFunction, semanticsource.FacetProgramFlowFunctionCapture).Token() {
		return "ProgramFlowFunction@ProgramFlowFunctionCapture", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowCall, 0).Token() {
		return "ProgramFlowCall@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowCall, semanticsource.FacetProgramFlowDirectCallBinding).Token() {
		return "ProgramFlowCall@ProgramFlowDirectCallBinding", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowControl, 0).Token() {
		return "ProgramFlowControl@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowControl, semanticsource.FacetProgramFlowGenericFor).Token() {
		return "ProgramFlowControl@ProgramFlowGenericFor", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowClaim, 0).Token() {
		return "ProgramFlowClaim@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowTypeValue, 0).Token() {
		return "ProgramFlowTypeValue@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowOutcome, 0).Token() {
		return "ProgramFlowOutcome@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowTransfer, 0).Token() {
		return "ProgramFlowTransfer@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowBody, 0).Token() {
		return "ProgramFlowBody@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots).Token() {
		return "ProgramFlowBody@ProgramFlowBodyRoots", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, 0).Token() {
		return "ProgramStatic@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticFunctionContract).Token() {
		return "ProgramStatic@ProgramStaticFunctionContract", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCallTypeArguments).Token() {
		return "ProgramStatic@ProgramStaticCallTypeArguments", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCellDeclaredType).Token() {
		return "ProgramStatic@ProgramStaticCellDeclaredType", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticClaimTarget).Token() {
		return "ProgramStatic@ProgramStaticClaimTarget", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget).Token() {
		return "ProgramStatic@ProgramStaticTypeValueTarget", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeof).Token() {
		return "ProgramStatic@ProgramStaticTypeof", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation).Token() {
		return "ProgramStatic@ProgramStaticAnnotation", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication).Token() {
		return "ProgramStatic@ProgramStaticPublication", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef).Token() {
		return "ProgramStatic@ProgramStaticTypeRef", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramModuleImport, 0).Token() {
		return "ProgramModuleImport@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest).Token() {
		return "ProgramModuleImport@ProgramModuleRequest", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramModuleEntry, 0).Token() {
		return "ProgramModuleEntry@-", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootCell).Token() {
		return "ProgramModuleEntry@ProgramModuleEntryRootCell", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryMember).Token() {
		return "ProgramModuleEntry@ProgramModuleEntryMember", true
	}
	if token == catalogDefinition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootFunction).Token() {
		return "ProgramModuleEntry@ProgramModuleEntryRootFunction", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetContract, 0).Token() {
		return "TargetContract@-", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, 0).Token() {
		return "TargetOperation@-", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI).Token() {
		return "TargetOperation@TargetABI", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge).Token() {
		return "TargetOperation@TargetSubedge", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback).Token() {
		return "TargetOperation@TargetCallback", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding).Token() {
		return "TargetOperation@TargetBinding", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume).Token() {
		return "TargetOperation@TargetResume", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn).Token() {
		return "TargetOperation@TargetSpawn", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque).Token() {
		return "TargetOperation@TargetOpaque", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect).Token() {
		return "TargetOperation@TargetOperationEffect", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect).Token() {
		return "TargetOperation@TargetCallbackEffect", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease).Token() {
		return "TargetOperation@TargetCallbackRelease", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome).Token() {
		return "TargetOperation@TargetOutcome", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer).Token() {
		return "TargetOperation@TargetTransfer", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome).Token() {
		return "TargetOperation@TargetTransferOutcome", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension).Token() {
		return "TargetOperation@TargetSuspension", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResumeOutcome).Token() {
		return "TargetOperation@TargetResumeOutcome", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawnSibling).Token() {
		return "TargetOperation@TargetSpawnSibling", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedgeArgumentOrigin).Token() {
		return "TargetOperation@TargetSubedgeArgumentOrigin", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackResult).Token() {
		return "TargetOperation@TargetCallbackResult", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResultAlias).Token() {
		return "TargetOperation@TargetResultAlias", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced).Token() {
		return "TargetOperation@TargetProduced", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProducedCapture).Token() {
		return "TargetOperation@TargetProducedCapture", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetFreshResult).Token() {
		return "TargetOperation@TargetFreshResult", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect).Token() {
		return "TargetOperation@TargetPublicationEffect", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetProtocol, 0).Token() {
		return "TargetProtocol@-", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState).Token() {
		return "TargetProtocol@TargetProtocolState", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition).Token() {
		return "TargetProtocol@TargetProtocolAcquisition", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition).Token() {
		return "TargetProtocol@TargetProtocolTransition", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome).Token() {
		return "TargetProtocol@TargetProtocolTransitionOutcome", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape).Token() {
		return "TargetProtocol@TargetProtocolEscape", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder).Token() {
		return "TargetProtocol@TargetProtocolCallbackHolder", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetBoot, 0).Token() {
		return "TargetBoot@-", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootEntry).Token() {
		return "TargetBoot@TargetBootEntry", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootMetatableAttachment).Token() {
		return "TargetBoot@TargetBootMetatableAttachment", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootBinding).Token() {
		return "TargetBoot@TargetBootBinding", true
	}
	if token == catalogDefinition(semanticsource.OriginTargetGsub, 0).Token() {
		return "TargetGsub@-", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkProjectShardMount, 0).Token() {
		return "LinkProjectShardMount@-", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkProjectBaseApplication, 0).Token() {
		return "LinkProjectBaseApplication@-", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkBoundary, 0).Token() {
		return "LinkBoundary@-", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, 0).Token() {
		return "LinkModule@-", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache).Token() {
		return "LinkModule@LinkModuleCache", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative).Token() {
		return "LinkModule@LinkModuleRepresentative", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport).Token() {
		return "LinkModule@LinkModuleTransport", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot).Token() {
		return "LinkModule@LinkModuleAnalysisRoot", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration).Token() {
		return "LinkModule@LinkModuleInitGeneration", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome).Token() {
		return "LinkModule@LinkModuleInitOutcome", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal).Token() {
		return "LinkModule@LinkModuleInitTerminal", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkStatic, 0).Token() {
		return "LinkStatic@-", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkHost, 0).Token() {
		return "LinkHost@-", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure).Token() {
		return "LinkHost@LinkHostExposure", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot).Token() {
		return "LinkHost@LinkHostBoot", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember).Token() {
		return "LinkHost@LinkHostMember", true
	}
	if token == catalogDefinition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget).Token() {
		return "LinkHost@LinkHostEndpointTarget", true
	}
	return "", false
}

var (
	canonicalSchemaOnce sync.Once
	canonicalSchema     *Schema
	canonicalSchemaErr  error
)

// CanonicalSchema returns the one immutable generated relation schema. The
// schema exposes only detached snapshots, so callers cannot mutate the
// cached authority. Initialization errors are cached and fail closed.
func CanonicalSchema() (*Schema, error) {
	canonicalSchemaOnce.Do(func() {
		canonicalSchema, canonicalSchemaErr = buildCanonicalSchema()
	})
	if canonicalSchemaErr != nil || canonicalSchema == nil {
		return nil, canonicalSchemaErr
	}
	return canonicalSchema, nil
}

func buildCanonicalSchema() (*Schema, error) {
	definition := catalogDefinition
	parents := func(definitions ...semanticsource.RelationDef) []semanticsource.Token {
		tokens := make([]semanticsource.Token, len(definitions))
		for index, definition := range definitions {
			tokens[index] = definition.Token()
		}
		return tokens
	}
	return Seal([]Row{
		{Definition: definition(semanticsource.OriginProgramSourceProvenance, 0), Owner: OwnerProgramSource, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramSourceOrder, 0), Owner: OwnerProgramSource, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramSourceProvenance, 0))},
		{Definition: definition(semanticsource.OriginProgramSourceKey, 0), Owner: OwnerProgramSource, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramSourceProvenance, 0))},
		{Definition: definition(semanticsource.OriginProgramSourceExactKey, 0), Owner: OwnerProgramSource, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramSourceKey, 0))},
		{Definition: definition(semanticsource.OriginProgramSourceControlFault, 0), Owner: OwnerProgramSource, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramSourceProvenance, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowLiterals, 0), Owner: OwnerProgramSource, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramFlowValues, 0), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence))},
		{Definition: definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), Owner: OwnerProgramFlow, Form: FormVirtualPredicate, Parents: parents(definition(semanticsource.OriginProgramFlowLiterals, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg), definition(semanticsource.OriginProgramFlowConstructors, 0), definition(semanticsource.OriginProgramFlowOperators, 0), definition(semanticsource.OriginProgramFlowFunction, 0), definition(semanticsource.OriginProgramFlowCall, 0), definition(semanticsource.OriginProgramFlowClaim, 0), definition(semanticsource.OriginProgramFlowTypeValue, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowLens, 0), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), definition(semanticsource.OriginProgramSourceKey, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, 0), Owner: OwnerProgramFlow, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell), Owner: OwnerProgramFlow, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell), definition(semanticsource.OriginProgramSourceKey, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell), definition(semanticsource.OriginProgramFlowLens, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell), definition(semanticsource.OriginProgramFlowLens, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell))},
		{Definition: definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageBind), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell), definition(semanticsource.OriginProgramFlowValues, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowConstructors, 0), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), definition(semanticsource.OriginProgramFlowValues, 0), definition(semanticsource.OriginProgramSourceKey, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowConstructors, semanticsource.FacetProgramFlowConstructorField), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowConstructors, 0), definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), definition(semanticsource.OriginProgramFlowLens, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, 0), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead))},
		{Definition: definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowOperators, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite))},
		{Definition: definition(semanticsource.OriginProgramFlowFunction, 0), Owner: OwnerProgramFlow, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramFlowFunction, semanticsource.FacetProgramFlowFunctionCapture), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowFunction, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell))},
		{Definition: definition(semanticsource.OriginProgramFlowCall, 0), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), definition(semanticsource.OriginProgramFlowValues, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowCall, semanticsource.FacetProgramFlowDirectCallBinding), Owner: OwnerProgramFlow, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramFlowCall, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowControl, 0), Owner: OwnerProgramFlow, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramFlowControl, semanticsource.FacetProgramFlowGenericFor), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowControl, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowClaim, 0), Owner: OwnerProgramFlow, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence))},
		{Definition: definition(semanticsource.OriginProgramFlowTypeValue, 0), Owner: OwnerProgramFlow, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramFlowOutcome, 0), Owner: OwnerProgramFlow, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramFlowCall, 0), definition(semanticsource.OriginProgramFlowOperators, 0), definition(semanticsource.OriginProgramFlowControl, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowTransfer, 0), Owner: OwnerProgramFlow, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramFlowCall, 0), definition(semanticsource.OriginProgramFlowOutcome, 0))},
		{Definition: definition(semanticsource.OriginProgramFlowBody, 0), Owner: OwnerProgramSource, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots), Owner: OwnerProgramSource, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramFlowBody, 0))},
		{Definition: definition(semanticsource.OriginProgramStatic, 0), Owner: OwnerProgramStatic, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticFunctionContract), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramStatic, 0), definition(semanticsource.OriginProgramFlowFunction, 0))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCallTypeArguments), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramStatic, 0), definition(semanticsource.OriginProgramFlowCall, 0))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCellDeclaredType), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramStatic, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticClaimTarget), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramStatic, 0), definition(semanticsource.OriginProgramFlowClaim, 0))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramStatic, 0), definition(semanticsource.OriginProgramFlowTypeValue, 0))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeof), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowValues, 0), definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign), definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef))},
		{Definition: definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef), Owner: OwnerProgramStatic, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramStatic, 0))},
		{Definition: definition(semanticsource.OriginProgramModuleImport, 0), Owner: OwnerProgramModule, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginProgramFlowCall, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell))},
		{Definition: definition(semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest), Owner: OwnerProgramModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramModuleImport, 0), definition(semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), definition(semanticsource.OriginProgramFlowLiterals, 0), definition(semanticsource.OriginProgramSourceExactKey, 0))},
		{Definition: definition(semanticsource.OriginProgramModuleEntry, 0), Owner: OwnerProgramModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramFlowControl, 0), definition(semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots))},
		{Definition: definition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootCell), Owner: OwnerProgramModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramModuleEntry, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell))},
		{Definition: definition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryMember), Owner: OwnerProgramModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramModuleEntry, 0), definition(semanticsource.OriginProgramFlowConstructors, semanticsource.FacetProgramFlowConstructorField))},
		{Definition: definition(semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootFunction), Owner: OwnerProgramModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramModuleEntry, 0), definition(semanticsource.OriginProgramFlowFunction, 0))},
		{Definition: definition(semanticsource.OriginTargetContract, 0), Owner: OwnerTarget, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginTargetOperation, 0), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetContract, 0))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque), Owner: OwnerTarget, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginTargetOperation, 0))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension), Owner: OwnerTarget, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResumeOutcome), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawnSibling), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedgeArgumentOrigin), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackResult), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResultAlias), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProducedCapture), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetFreshResult), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect))},
		{Definition: definition(semanticsource.OriginTargetProtocol, 0), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetContract, 0), definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetProtocol, 0))},
		{Definition: definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetProtocol, 0), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetProtocol, 0), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState), definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape), Owner: OwnerTarget, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginTargetProtocol, 0), definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetProtocol, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginTargetBoot, 0), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetContract, 0))},
		{Definition: definition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootEntry), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetBoot, 0))},
		{Definition: definition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootMetatableAttachment), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetBoot, 0))},
		{Definition: definition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootBinding), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetBoot, 0), definition(semanticsource.OriginTargetBoot, semanticsource.FacetTargetBootEntry))},
		{Definition: definition(semanticsource.OriginTargetGsub, 0), Owner: OwnerTarget, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome))},
		{Definition: definition(semanticsource.OriginLinkProjectShardMount, 0), Owner: OwnerLinkProject, Form: FormAuthored},
		{Definition: definition(semanticsource.OriginLinkProjectBaseApplication, 0), Owner: OwnerLinkProject, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkProjectShardMount, 0), definition(semanticsource.OriginProgramFlowCall, 0), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet), definition(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet), definition(semanticsource.OriginProgramFlowControl, semanticsource.FacetProgramFlowGenericFor))},
		{Definition: definition(semanticsource.OriginLinkBoundary, 0), Owner: OwnerLinkBoundary, Form: FormVirtualPredicate, Parents: parents(definition(semanticsource.OriginLinkProjectBaseApplication, 0), definition(semanticsource.OriginTargetOperation, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransfer), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetTransferOutcome), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResumeOutcome), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawnSibling), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedgeArgumentOrigin), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackResult), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetResultAlias), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetProducedCapture), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetFreshResult), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect), definition(semanticsource.OriginTargetProtocol, 0), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape), definition(semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder), definition(semanticsource.OriginTargetGsub, 0))},
		{Definition: definition(semanticsource.OriginLinkModule, 0), Owner: OwnerLinkModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkProjectShardMount, 0), definition(semanticsource.OriginProgramModuleImport, 0))},
		{Definition: definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache), Owner: OwnerLinkModule, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginLinkModule, 0))},
		{Definition: definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative), Owner: OwnerLinkModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache))},
		{Definition: definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport), Owner: OwnerLinkModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkBoundary, 0), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot))},
		{Definition: definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot), Owner: OwnerLinkModule, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginLinkModule, 0), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache))},
		{Definition: definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration), Owner: OwnerLinkModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkModule, 0), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport), definition(semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots))},
		{Definition: definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome), Owner: OwnerLinkModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitGeneration), definition(semanticsource.OriginProgramFlowOutcome, 0))},
		{Definition: definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitTerminal), Owner: OwnerLinkModule, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleInitOutcome))},
		{Definition: definition(semanticsource.OriginLinkStatic, 0), Owner: OwnerLinkStatic, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication))},
		{Definition: definition(semanticsource.OriginLinkHost, 0), Owner: OwnerLinkHost, Form: FormAuthored, Parents: parents(definition(semanticsource.OriginTargetContract, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI))},
		{Definition: definition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure), Owner: OwnerLinkHost, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkHost, 0))},
		{Definition: definition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot), Owner: OwnerLinkHost, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkModule, 0), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache), definition(semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative), definition(semanticsource.OriginTargetBoot, 0), definition(semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal), definition(semanticsource.OriginProgramSourceKey, 0))},
		{Definition: definition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember), Owner: OwnerLinkHost, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure), definition(semanticsource.OriginProgramFlowLens, 0), definition(semanticsource.OriginProgramSourceKey, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding))},
		{Definition: definition(semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget), Owner: OwnerLinkHost, Form: FormSealDerived, Parents: parents(definition(semanticsource.OriginLinkHost, 0), definition(semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding))},
	})
}
