// Code generated from program/internal/schema/relations/catalog.schema; DO NOT EDIT.

package semanticsource

import "sync"

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

var catalogDefinitions = [...]RelationDef{
	{token: Token{origin: OriginProgramSourceProvenance, facet: 0, revision: Revision(1), digest: 0xAF2E36A4300F43C7}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramSourceOrder, facet: 0, revision: Revision(1), digest: 0xC162BC2093FF81F8}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramSourceKey, facet: 0, revision: Revision(1), digest: 0x46505E8F950E53E3}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramSourceExactKey, facet: 0, revision: Revision(1), digest: 0xED588F2E4288F979}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramSourceControlFault, facet: 0, revision: Revision(1), digest: 0xFCC190FCAC560F5E}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowLiterals, facet: 0, revision: Revision(2), digest: 0x388B4E44DCDDBC03}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowValues, facet: 0, revision: Revision(5), digest: 0x71C20ECCCFDDEE38}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowValues, facet: FacetProgramFlowValueOccurrence, revision: Revision(5), digest: 0x3311000C1245B3B1}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowLens, facet: 0, revision: Revision(5), digest: 0x04DC8E860D1550BB}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: 0, revision: Revision(5), digest: 0x909D56FDE7C1AC2E}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: FacetProgramFlowStorageCell, revision: Revision(5), digest: 0xA00B50FFC73D7ADD}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: FacetProgramFlowStorageGlobal, revision: Revision(5), digest: 0xCCA507326E811886}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: FacetProgramFlowStorageRead, revision: Revision(5), digest: 0xF3C7875E80A01A0B}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: FacetProgramFlowStorageAssign, revision: Revision(5), digest: 0x637E440A2BFC5C62}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: FacetProgramFlowStorageWrite, revision: Revision(5), digest: 0x8B6993C29BF960E8}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: FacetProgramFlowStorageVararg, revision: Revision(5), digest: 0x0C9C4D0C914944F5}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowStorage, facet: FacetProgramFlowStorageBind, revision: Revision(5), digest: 0xD2DF5887186C9F5B}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowConstructors, facet: 0, revision: Revision(5), digest: 0x29A6921EE30D0B79}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowConstructors, facet: FacetProgramFlowConstructorField, revision: Revision(5), digest: 0x0F56681119BE8702}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: 0, revision: Revision(5), digest: 0xF08EABA2A39007E2}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowUnaryNumeric, revision: Revision(5), digest: 0x7C034CFCF566FAA7}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowLength, revision: Revision(5), digest: 0xDB0B00531905BEB0}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowArithmetic, revision: Revision(5), digest: 0x8A39D13910A11C13}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowBitwise, revision: Revision(5), digest: 0xBA829B91F48AA21A}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowConcat, revision: Revision(5), digest: 0x02FE0B874F52A843}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowEquality, revision: Revision(5), digest: 0x76DFFED19E549518}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowOrder, revision: Revision(5), digest: 0x8AF1A3AE15784D57}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowIndexGet, revision: Revision(5), digest: 0x5E34CD476F9EB775}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOperators, facet: FacetProgramFlowIndexSet, revision: Revision(5), digest: 0x48ABAA3164668F7D}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowFunction, facet: 0, revision: Revision(5), digest: 0xE33CF0ADFF389E87}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowFunction, facet: FacetProgramFlowFunctionCapture, revision: Revision(5), digest: 0xFDE8821EF95FB558}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowCall, facet: 0, revision: Revision(5), digest: 0x5B68F7FBCC43DF31}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowCall, facet: FacetProgramFlowDirectCallBinding, revision: Revision(5), digest: 0xA96C8BF446CE3DF2}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowControl, facet: 0, revision: Revision(3), digest: 0x8A3ED54E91949EE6}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowControl, facet: FacetProgramFlowGenericFor, revision: Revision(3), digest: 0x381B9CA4A9701AC0}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowClaim, facet: 0, revision: Revision(5), digest: 0xA24A5A6157F34F61}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowTypeValue, facet: 0, revision: Revision(3), digest: 0x9E422100EF297AC9}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowOutcome, facet: 0, revision: Revision(4), digest: 0xD4034D7710EC93BD}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowTransfer, facet: 0, revision: Revision(4), digest: 0x1792688976186D02}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowBody, facet: 0, revision: Revision(5), digest: 0xAF38C7285C9C38B7}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramFlowBody, facet: FacetProgramFlowBodyRoots, revision: Revision(5), digest: 0x372E7ECA475D2845}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: 0, revision: Revision(6), digest: 0xA8F3AC49EFCA32AB}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticFunctionContract, revision: Revision(6), digest: 0x5D04C0D7484D8FD6}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticCallTypeArguments, revision: Revision(6), digest: 0x36783B5E480803B8}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticCellDeclaredType, revision: Revision(6), digest: 0x8B416C3C45C465E0}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticClaimTarget, revision: Revision(6), digest: 0x199200127F106889}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticTypeValueTarget, revision: Revision(6), digest: 0x24BD76570B0777ED}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticTypeof, revision: Revision(6), digest: 0x880D381FA8E1D602}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticAnnotation, revision: Revision(6), digest: 0x5E116D27AEDFCA99}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticPublication, revision: Revision(6), digest: 0xF5ACBB9B6BB6492A}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramStatic, facet: FacetProgramStaticTypeRef, revision: Revision(6), digest: 0x78D2E516ACA8E187}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramModuleImport, facet: 0, revision: Revision(6), digest: 0xCA5EDF8F45FB34BA}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramModuleImport, facet: FacetProgramModuleRequest, revision: Revision(6), digest: 0x92ACAB9C7EBAB2A0}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramModuleEntry, facet: 0, revision: Revision(4), digest: 0x0475FDF1297D1AE0}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramModuleEntry, facet: FacetProgramModuleEntryRootCell, revision: Revision(4), digest: 0x0A56123B2816BCDC}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramModuleEntry, facet: FacetProgramModuleEntryMember, revision: Revision(4), digest: 0x15E8BE9C9EB4157E}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginProgramModuleEntry, facet: FacetProgramModuleEntryRootFunction, revision: Revision(4), digest: 0x2E89F865BF61EBAE}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetContract, facet: 0, revision: Revision(1), digest: 0x73BD5CEB13146543}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: 0, revision: Revision(6), digest: 0xE25E5E31273B6793}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetABI, revision: Revision(6), digest: 0x61AE49708E7CE109}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetSubedge, revision: Revision(6), digest: 0x9F6638178AFE7E44}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetCallback, revision: Revision(6), digest: 0xF181F5F1760BCD56}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetBinding, revision: Revision(6), digest: 0x0BBC7A81638BBFC6}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetResume, revision: Revision(6), digest: 0xFC7C734E11231D25}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetSpawn, revision: Revision(6), digest: 0x478119C62C460E8D}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetOpaque, revision: Revision(6), digest: 0x926390A9F09850DD}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetOperationEffect, revision: Revision(6), digest: 0xA6145AFEEE66F008}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetCallbackEffect, revision: Revision(6), digest: 0x81AB9B27385283A8}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetCallbackRelease, revision: Revision(6), digest: 0xFD615700E06ADE12}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetOutcome, revision: Revision(6), digest: 0xCFDFB63642BA5BBF}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetTransfer, revision: Revision(6), digest: 0xD566D26DE2B5A190}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetTransferOutcome, revision: Revision(6), digest: 0xD7A1B8E9BAFAC6F5}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetSuspension, revision: Revision(6), digest: 0xE43E7181C5D58A73}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetResumeOutcome, revision: Revision(6), digest: 0x702DC4AC58499280}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetSpawnSibling, revision: Revision(6), digest: 0x8A41C1D06B9BF5BE}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetSubedgeArgumentOrigin, revision: Revision(6), digest: 0x9691454ECB6B254C}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetCallbackResult, revision: Revision(6), digest: 0xB76FA2E8C4F5AB27}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetResultAlias, revision: Revision(6), digest: 0xBC2F5E3968A0446D}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetProduced, revision: Revision(6), digest: 0xD3616B19C4C0F0E6}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetProducedCapture, revision: Revision(6), digest: 0x75C9D98C3E782725}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetFreshResult, revision: Revision(6), digest: 0x4DA1F63E346D098B}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetOperation, facet: FacetTargetPublicationEffect, revision: Revision(6), digest: 0x4908A94326F2349D}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetProtocol, facet: 0, revision: Revision(7), digest: 0xE74076F2BD05710F}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetProtocol, facet: FacetTargetProtocolState, revision: Revision(7), digest: 0xDBCBEFAA498DA0AD}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetProtocol, facet: FacetTargetProtocolAcquisition, revision: Revision(7), digest: 0x3C088D97C0F53261}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetProtocol, facet: FacetTargetProtocolTransition, revision: Revision(7), digest: 0x2F9279E6EF4B773A}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetProtocol, facet: FacetTargetProtocolTransitionOutcome, revision: Revision(7), digest: 0x5E02B3132D240DFF}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetProtocol, facet: FacetTargetProtocolEscape, revision: Revision(7), digest: 0xDC431F0D102D97C4}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetProtocol, facet: FacetTargetProtocolCallbackHolder, revision: Revision(7), digest: 0x12BA788F83224E11}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetBoot, facet: 0, revision: Revision(2), digest: 0x6E54004135AA121F}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetBoot, facet: FacetTargetBootEntry, revision: Revision(2), digest: 0xB43F900A4512E618}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetBoot, facet: FacetTargetBootMetatableAttachment, revision: Revision(2), digest: 0x4259425CB5E7E14C}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetBoot, facet: FacetTargetBootBinding, revision: Revision(2), digest: 0x8A5E76DE0D229CF4}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginTargetGsub, facet: 0, revision: Revision(6), digest: 0x13F83842BC542F1F}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkProjectShardMount, facet: 0, revision: Revision(3), digest: 0x44B78A5A8B2481D8}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkProjectBaseApplication, facet: 0, revision: Revision(6), digest: 0x480B7289D297D8BE}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkBoundary, facet: 0, revision: Revision(11), digest: 0xBCE702CCCEC3D61C}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: 0, revision: Revision(11), digest: 0xC7524E74F2A5C74B}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: FacetLinkModuleCache, revision: Revision(11), digest: 0x4F851D9174C5D9F8}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: FacetLinkModuleRepresentative, revision: Revision(11), digest: 0x04140F960BD3F949}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: FacetLinkModuleTransport, revision: Revision(11), digest: 0x5B58C85DAE4BC27D}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: FacetLinkModuleAnalysisRoot, revision: Revision(11), digest: 0xD521D713D3BD249E}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: FacetLinkModuleInitGeneration, revision: Revision(11), digest: 0x68A186580DF1BE9B}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: FacetLinkModuleInitOutcome, revision: Revision(11), digest: 0x67D241150E75A6B7}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkModule, facet: FacetLinkModuleInitTerminal, revision: Revision(11), digest: 0x82724A26BD882E2A}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkStatic, facet: 0, revision: Revision(11), digest: 0x860B148E564262F6}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkHost, facet: 0, revision: Revision(11), digest: 0x2B4E599B59FCA66B}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkHost, facet: FacetLinkHostExposure, revision: Revision(11), digest: 0x2363649B555D0204}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkHost, facet: FacetLinkHostBoot, revision: Revision(11), digest: 0x2066258FE1A9BE76}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkHost, facet: FacetLinkHostMember, revision: Revision(11), digest: 0x91AE5D552CC3957F}, seal: relationDefinitionSeal},
	{token: Token{origin: OriginLinkHost, facet: FacetLinkHostEndpointTarget, revision: Revision(11), digest: 0x4C777F430F55EF0F}, seal: relationDefinitionSeal},
}

var (
	catalogSchemaOnce sync.Once
	catalogSchema     Schema
)

func CatalogSchema() Schema {
	catalogSchemaOnce.Do(func() {
		catalogSchema = issuedSchema(catalogDefinitions[:]...)
	})
	return catalogSchema
}

// catalogDefinition is generated from the sole catalog source. It returns a
// value copy from private immutable storage, so repeated hot lookups neither
// materialize Schema.Definitions nor recompute token identities.
func catalogDefinition(origin Origin, facet Facet) (RelationDef, bool) {
	switch origin {
	case OriginProgramSourceProvenance:
		switch facet {
		case 0:
			return catalogDefinitions[0], true
		}
	case OriginProgramSourceOrder:
		switch facet {
		case 0:
			return catalogDefinitions[1], true
		}
	case OriginProgramSourceKey:
		switch facet {
		case 0:
			return catalogDefinitions[2], true
		}
	case OriginProgramSourceExactKey:
		switch facet {
		case 0:
			return catalogDefinitions[3], true
		}
	case OriginProgramSourceControlFault:
		switch facet {
		case 0:
			return catalogDefinitions[4], true
		}
	case OriginProgramFlowLiterals:
		switch facet {
		case 0:
			return catalogDefinitions[5], true
		}
	case OriginProgramFlowValues:
		switch facet {
		case 0:
			return catalogDefinitions[6], true
		case FacetProgramFlowValueOccurrence:
			return catalogDefinitions[7], true
		}
	case OriginProgramFlowLens:
		switch facet {
		case 0:
			return catalogDefinitions[8], true
		}
	case OriginProgramFlowStorage:
		switch facet {
		case 0:
			return catalogDefinitions[9], true
		case FacetProgramFlowStorageCell:
			return catalogDefinitions[10], true
		case FacetProgramFlowStorageGlobal:
			return catalogDefinitions[11], true
		case FacetProgramFlowStorageRead:
			return catalogDefinitions[12], true
		case FacetProgramFlowStorageAssign:
			return catalogDefinitions[13], true
		case FacetProgramFlowStorageWrite:
			return catalogDefinitions[14], true
		case FacetProgramFlowStorageVararg:
			return catalogDefinitions[15], true
		case FacetProgramFlowStorageBind:
			return catalogDefinitions[16], true
		}
	case OriginProgramFlowConstructors:
		switch facet {
		case 0:
			return catalogDefinitions[17], true
		case FacetProgramFlowConstructorField:
			return catalogDefinitions[18], true
		}
	case OriginProgramFlowOperators:
		switch facet {
		case 0:
			return catalogDefinitions[19], true
		case FacetProgramFlowUnaryNumeric:
			return catalogDefinitions[20], true
		case FacetProgramFlowLength:
			return catalogDefinitions[21], true
		case FacetProgramFlowArithmetic:
			return catalogDefinitions[22], true
		case FacetProgramFlowBitwise:
			return catalogDefinitions[23], true
		case FacetProgramFlowConcat:
			return catalogDefinitions[24], true
		case FacetProgramFlowEquality:
			return catalogDefinitions[25], true
		case FacetProgramFlowOrder:
			return catalogDefinitions[26], true
		case FacetProgramFlowIndexGet:
			return catalogDefinitions[27], true
		case FacetProgramFlowIndexSet:
			return catalogDefinitions[28], true
		}
	case OriginProgramFlowFunction:
		switch facet {
		case 0:
			return catalogDefinitions[29], true
		case FacetProgramFlowFunctionCapture:
			return catalogDefinitions[30], true
		}
	case OriginProgramFlowCall:
		switch facet {
		case 0:
			return catalogDefinitions[31], true
		case FacetProgramFlowDirectCallBinding:
			return catalogDefinitions[32], true
		}
	case OriginProgramFlowControl:
		switch facet {
		case 0:
			return catalogDefinitions[33], true
		case FacetProgramFlowGenericFor:
			return catalogDefinitions[34], true
		}
	case OriginProgramFlowClaim:
		switch facet {
		case 0:
			return catalogDefinitions[35], true
		}
	case OriginProgramFlowTypeValue:
		switch facet {
		case 0:
			return catalogDefinitions[36], true
		}
	case OriginProgramFlowOutcome:
		switch facet {
		case 0:
			return catalogDefinitions[37], true
		}
	case OriginProgramFlowTransfer:
		switch facet {
		case 0:
			return catalogDefinitions[38], true
		}
	case OriginProgramFlowBody:
		switch facet {
		case 0:
			return catalogDefinitions[39], true
		case FacetProgramFlowBodyRoots:
			return catalogDefinitions[40], true
		}
	case OriginProgramStatic:
		switch facet {
		case 0:
			return catalogDefinitions[41], true
		case FacetProgramStaticFunctionContract:
			return catalogDefinitions[42], true
		case FacetProgramStaticCallTypeArguments:
			return catalogDefinitions[43], true
		case FacetProgramStaticCellDeclaredType:
			return catalogDefinitions[44], true
		case FacetProgramStaticClaimTarget:
			return catalogDefinitions[45], true
		case FacetProgramStaticTypeValueTarget:
			return catalogDefinitions[46], true
		case FacetProgramStaticTypeof:
			return catalogDefinitions[47], true
		case FacetProgramStaticAnnotation:
			return catalogDefinitions[48], true
		case FacetProgramStaticPublication:
			return catalogDefinitions[49], true
		case FacetProgramStaticTypeRef:
			return catalogDefinitions[50], true
		}
	case OriginProgramModuleImport:
		switch facet {
		case 0:
			return catalogDefinitions[51], true
		case FacetProgramModuleRequest:
			return catalogDefinitions[52], true
		}
	case OriginProgramModuleEntry:
		switch facet {
		case 0:
			return catalogDefinitions[53], true
		case FacetProgramModuleEntryRootCell:
			return catalogDefinitions[54], true
		case FacetProgramModuleEntryMember:
			return catalogDefinitions[55], true
		case FacetProgramModuleEntryRootFunction:
			return catalogDefinitions[56], true
		}
	case OriginTargetContract:
		switch facet {
		case 0:
			return catalogDefinitions[57], true
		}
	case OriginTargetOperation:
		switch facet {
		case 0:
			return catalogDefinitions[58], true
		case FacetTargetABI:
			return catalogDefinitions[59], true
		case FacetTargetSubedge:
			return catalogDefinitions[60], true
		case FacetTargetCallback:
			return catalogDefinitions[61], true
		case FacetTargetBinding:
			return catalogDefinitions[62], true
		case FacetTargetResume:
			return catalogDefinitions[63], true
		case FacetTargetSpawn:
			return catalogDefinitions[64], true
		case FacetTargetOpaque:
			return catalogDefinitions[65], true
		case FacetTargetOperationEffect:
			return catalogDefinitions[66], true
		case FacetTargetCallbackEffect:
			return catalogDefinitions[67], true
		case FacetTargetCallbackRelease:
			return catalogDefinitions[68], true
		case FacetTargetOutcome:
			return catalogDefinitions[69], true
		case FacetTargetTransfer:
			return catalogDefinitions[70], true
		case FacetTargetTransferOutcome:
			return catalogDefinitions[71], true
		case FacetTargetSuspension:
			return catalogDefinitions[72], true
		case FacetTargetResumeOutcome:
			return catalogDefinitions[73], true
		case FacetTargetSpawnSibling:
			return catalogDefinitions[74], true
		case FacetTargetSubedgeArgumentOrigin:
			return catalogDefinitions[75], true
		case FacetTargetCallbackResult:
			return catalogDefinitions[76], true
		case FacetTargetResultAlias:
			return catalogDefinitions[77], true
		case FacetTargetProduced:
			return catalogDefinitions[78], true
		case FacetTargetProducedCapture:
			return catalogDefinitions[79], true
		case FacetTargetFreshResult:
			return catalogDefinitions[80], true
		case FacetTargetPublicationEffect:
			return catalogDefinitions[81], true
		}
	case OriginTargetProtocol:
		switch facet {
		case 0:
			return catalogDefinitions[82], true
		case FacetTargetProtocolState:
			return catalogDefinitions[83], true
		case FacetTargetProtocolAcquisition:
			return catalogDefinitions[84], true
		case FacetTargetProtocolTransition:
			return catalogDefinitions[85], true
		case FacetTargetProtocolTransitionOutcome:
			return catalogDefinitions[86], true
		case FacetTargetProtocolEscape:
			return catalogDefinitions[87], true
		case FacetTargetProtocolCallbackHolder:
			return catalogDefinitions[88], true
		}
	case OriginTargetBoot:
		switch facet {
		case 0:
			return catalogDefinitions[89], true
		case FacetTargetBootEntry:
			return catalogDefinitions[90], true
		case FacetTargetBootMetatableAttachment:
			return catalogDefinitions[91], true
		case FacetTargetBootBinding:
			return catalogDefinitions[92], true
		}
	case OriginTargetGsub:
		switch facet {
		case 0:
			return catalogDefinitions[93], true
		}
	case OriginLinkProjectShardMount:
		switch facet {
		case 0:
			return catalogDefinitions[94], true
		}
	case OriginLinkProjectBaseApplication:
		switch facet {
		case 0:
			return catalogDefinitions[95], true
		}
	case OriginLinkBoundary:
		switch facet {
		case 0:
			return catalogDefinitions[96], true
		}
	case OriginLinkModule:
		switch facet {
		case 0:
			return catalogDefinitions[97], true
		case FacetLinkModuleCache:
			return catalogDefinitions[98], true
		case FacetLinkModuleRepresentative:
			return catalogDefinitions[99], true
		case FacetLinkModuleTransport:
			return catalogDefinitions[100], true
		case FacetLinkModuleAnalysisRoot:
			return catalogDefinitions[101], true
		case FacetLinkModuleInitGeneration:
			return catalogDefinitions[102], true
		case FacetLinkModuleInitOutcome:
			return catalogDefinitions[103], true
		case FacetLinkModuleInitTerminal:
			return catalogDefinitions[104], true
		}
	case OriginLinkStatic:
		switch facet {
		case 0:
			return catalogDefinitions[105], true
		}
	case OriginLinkHost:
		switch facet {
		case 0:
			return catalogDefinitions[106], true
		case FacetLinkHostExposure:
			return catalogDefinitions[107], true
		case FacetLinkHostBoot:
			return catalogDefinitions[108], true
		case FacetLinkHostMember:
			return catalogDefinitions[109], true
		case FacetLinkHostEndpointTarget:
			return catalogDefinitions[110], true
		}
	}
	return RelationDef{}, false
}
