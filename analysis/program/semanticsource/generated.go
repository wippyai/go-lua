// Code generated from program/relations/catalog.schema; DO NOT EDIT.

package semanticsource

const (
	OriginProgramSourceProvenance    Origin = 0x00010001
	OriginProgramSourceOrder         Origin = 0x00010002
	OriginProgramSourceKey           Origin = 0x00010003
	OriginProgramSourceExactKey      Origin = 0x00010004
	OriginProgramSourceControlFault  Origin = 0x00010005
	OriginProgramFlowLiterals        Origin = 0x00020001
	OriginProgramFlowValues          Origin = 0x00020002
	OriginProgramFlowLens            Origin = 0x00020003
	OriginProgramFlowStorage         Origin = 0x00020004
	OriginProgramFlowConstructors    Origin = 0x00020005
	OriginProgramFlowOperators       Origin = 0x00020006
	OriginProgramFlowFunction        Origin = 0x00020007
	OriginProgramFlowCall            Origin = 0x00020008
	OriginProgramFlowControl         Origin = 0x00020009
	OriginProgramFlowClaim           Origin = 0x0002000A
	OriginProgramFlowTypeValue       Origin = 0x0002000B
	OriginProgramFlowOutcome         Origin = 0x0002000C
	OriginProgramFlowTransfer        Origin = 0x0002000D
	OriginProgramFlowBody            Origin = 0x0002000E
	OriginProgramStatic              Origin = 0x00030001
	OriginProgramModuleImport        Origin = 0x00040001
	OriginProgramModuleEntry         Origin = 0x00040002
	OriginTargetContract             Origin = 0x00050001
	OriginTargetOperation            Origin = 0x00050002
	OriginTargetProtocol             Origin = 0x00050003
	OriginTargetBoot                 Origin = 0x00050004
	OriginTargetGsub                 Origin = 0x00050005
	OriginLinkProjectShardMount      Origin = 0x00060001
	OriginLinkProjectBaseApplication Origin = 0x00060002
	OriginLinkBoundary               Origin = 0x00070001
	OriginLinkModule                 Origin = 0x00080001
	OriginLinkStatic                 Origin = 0x00090001
	OriginLinkHost                   Origin = 0x000A0001
)

const (
	FacetLinkHostBoot                    Facet = 2
	FacetLinkHostEndpointTarget          Facet = 4
	FacetLinkHostExposure                Facet = 1
	FacetLinkHostMember                  Facet = 3
	FacetLinkModuleAnalysisRoot          Facet = 4
	FacetLinkModuleCache                 Facet = 1
	FacetLinkModuleInitGeneration        Facet = 5
	FacetLinkModuleInitOutcome           Facet = 6
	FacetLinkModuleInitTerminal          Facet = 7
	FacetLinkModuleRepresentative        Facet = 2
	FacetLinkModuleTransport             Facet = 3
	FacetProgramFlowArithmetic           Facet = 4
	FacetProgramFlowBitwise              Facet = 5
	FacetProgramFlowBodyRoots            Facet = 1
	FacetProgramFlowConcat               Facet = 6
	FacetProgramFlowConstructorField     Facet = 1
	FacetProgramFlowDirectCallBinding    Facet = 1
	FacetProgramFlowEquality             Facet = 7
	FacetProgramFlowFunctionCapture      Facet = 1
	FacetProgramFlowGenericFor           Facet = 1
	FacetProgramFlowIndexGet             Facet = 9
	FacetProgramFlowIndexSet             Facet = 10
	FacetProgramFlowLength               Facet = 3
	FacetProgramFlowOrder                Facet = 8
	FacetProgramFlowStorageAssign        Facet = 4
	FacetProgramFlowStorageBind          Facet = 7
	FacetProgramFlowStorageCell          Facet = 1
	FacetProgramFlowStorageGlobal        Facet = 2
	FacetProgramFlowStorageRead          Facet = 3
	FacetProgramFlowStorageVararg        Facet = 6
	FacetProgramFlowStorageWrite         Facet = 5
	FacetProgramFlowUnaryNumeric         Facet = 2
	FacetProgramFlowValueOccurrence      Facet = 1
	FacetProgramModuleEntryMember        Facet = 2
	FacetProgramModuleEntryRootCell      Facet = 1
	FacetProgramModuleEntryRootFunction  Facet = 3
	FacetProgramModuleRequest            Facet = 1
	FacetProgramStaticAnnotation         Facet = 8
	FacetProgramStaticCallTypeArguments  Facet = 3
	FacetProgramStaticCellDeclaredType   Facet = 4
	FacetProgramStaticClaimTarget        Facet = 5
	FacetProgramStaticFunctionContract   Facet = 2
	FacetProgramStaticPublication        Facet = 9
	FacetProgramStaticTypeRef            Facet = 10
	FacetProgramStaticTypeValueTarget    Facet = 6
	FacetProgramStaticTypeof             Facet = 7
	FacetTargetABI                       Facet = 1
	FacetTargetBinding                   Facet = 6
	FacetTargetBootBinding               Facet = 3
	FacetTargetBootEntry                 Facet = 1
	FacetTargetBootMetatableAttachment   Facet = 2
	FacetTargetCallback                  Facet = 4
	FacetTargetCallbackEffect            Facet = 11
	FacetTargetCallbackRelease           Facet = 12
	FacetTargetCallbackResult            Facet = 20
	FacetTargetFreshResult               Facet = 24
	FacetTargetOpaque                    Facet = 9
	FacetTargetOperationEffect           Facet = 10
	FacetTargetOutcome                   Facet = 13
	FacetTargetProduced                  Facet = 22
	FacetTargetProducedCapture           Facet = 23
	FacetTargetProtocolAcquisition       Facet = 2
	FacetTargetProtocolCallbackHolder    Facet = 6
	FacetTargetProtocolEscape            Facet = 5
	FacetTargetProtocolState             Facet = 1
	FacetTargetProtocolTransition        Facet = 3
	FacetTargetProtocolTransitionOutcome Facet = 4
	FacetTargetPublicationEffect         Facet = 25
	FacetTargetResultAlias               Facet = 21
	FacetTargetResume                    Facet = 7
	FacetTargetResumeOutcome             Facet = 17
	FacetTargetSpawn                     Facet = 8
	FacetTargetSpawnSibling              Facet = 18
	FacetTargetSubedge                   Facet = 3
	FacetTargetSubedgeArgumentOrigin     Facet = 19
	FacetTargetSuspension                Facet = 16
	FacetTargetTransfer                  Facet = 14
	FacetTargetTransferOutcome           Facet = 15
)

// revisionForOrigin is the generated compatibility fence for the token
// alphabet. It deliberately does not enumerate facets; the sealed relations
// table is the sole complete denominator.
func revisionForOrigin(origin Origin) (Revision, bool) {
	switch origin {
	case OriginProgramSourceProvenance:
		return Revision(1), true
	case OriginProgramSourceOrder:
		return Revision(1), true
	case OriginProgramSourceKey:
		return Revision(1), true
	case OriginProgramSourceExactKey:
		return Revision(1), true
	case OriginProgramSourceControlFault:
		return Revision(1), true
	case OriginProgramFlowLiterals:
		return Revision(2), true
	case OriginProgramFlowValues:
		return Revision(5), true
	case OriginProgramFlowLens:
		return Revision(5), true
	case OriginProgramFlowStorage:
		return Revision(5), true
	case OriginProgramFlowConstructors:
		return Revision(5), true
	case OriginProgramFlowOperators:
		return Revision(5), true
	case OriginProgramFlowFunction:
		return Revision(5), true
	case OriginProgramFlowCall:
		return Revision(5), true
	case OriginProgramFlowControl:
		return Revision(3), true
	case OriginProgramFlowClaim:
		return Revision(5), true
	case OriginProgramFlowTypeValue:
		return Revision(3), true
	case OriginProgramFlowOutcome:
		return Revision(4), true
	case OriginProgramFlowTransfer:
		return Revision(4), true
	case OriginProgramFlowBody:
		return Revision(5), true
	case OriginProgramStatic:
		return Revision(6), true
	case OriginProgramModuleImport:
		return Revision(6), true
	case OriginProgramModuleEntry:
		return Revision(4), true
	case OriginTargetContract:
		return Revision(1), true
	case OriginTargetOperation:
		return Revision(6), true
	case OriginTargetProtocol:
		return Revision(7), true
	case OriginTargetBoot:
		return Revision(2), true
	case OriginTargetGsub:
		return Revision(6), true
	case OriginLinkProjectShardMount:
		return Revision(3), true
	case OriginLinkProjectBaseApplication:
		return Revision(6), true
	case OriginLinkBoundary:
		return Revision(11), true
	case OriginLinkModule:
		return Revision(11), true
	case OriginLinkStatic:
		return Revision(11), true
	case OriginLinkHost:
		return Revision(11), true
	default:
		return 0, false
	}
}
