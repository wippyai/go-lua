package target

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

// SourceViews names every Target source family explicitly. The
// generated semantic-source schema remains the vocabulary authority; the
// owner-local viewFor switch only binds those definitions to typed rows.
type SourceViews struct {
	owner identity.ContentID

	contract, operation, abi, subedge, callback, binding, resume, spawn, opaque                             semanticsource.DigestView
	operationEffect, callbackEffect, publicationEffect, callbackRelease, outcome, transfer, transferOutcome semanticsource.DigestView
	suspension, resumeOutcome, spawnSibling, subedgeArgumentOrigin, callbackResult                          semanticsource.DigestView
	resultAlias, produced, producedCapture, freshResult                                                     semanticsource.DigestView
	protocol, protocolState, protocolAcquisition, protocolTransition, protocolTransitionOutcome             semanticsource.DigestView
	protocolEscape, protocolCallbackHolder                                                                  semanticsource.DigestView
	boot, bootEntry, bootMetatableAttachment, bootBinding, gsub                                             semanticsource.DigestView
}

func (views SourceViews) valid() bool {
	return semanticsource.FencedDigestViews(views.owner,
		views.contract, views.operation, views.abi, views.subedge, views.callback, views.binding, views.resume, views.spawn, views.opaque,
		views.operationEffect, views.callbackEffect, views.publicationEffect, views.callbackRelease, views.outcome, views.transfer, views.transferOutcome,
		views.suspension, views.resumeOutcome, views.spawnSibling, views.subedgeArgumentOrigin, views.callbackResult,
		views.resultAlias, views.produced, views.producedCapture, views.freshResult,
		views.protocol, views.protocolState, views.protocolAcquisition, views.protocolTransition, views.protocolTransitionOutcome,
		views.protocolEscape, views.protocolCallbackHolder, views.boot, views.bootEntry, views.bootMetatableAttachment,
		views.bootBinding, views.gsub,
	)
}

func (views SourceViews) OwnerID() identity.ContentID                { return views.owner }
func (views SourceViews) Contract() semanticsource.DigestView        { return views.contract }
func (views SourceViews) Operation() semanticsource.DigestView       { return views.operation }
func (views SourceViews) ABI() semanticsource.DigestView             { return views.abi }
func (views SourceViews) Subedge() semanticsource.DigestView         { return views.subedge }
func (views SourceViews) Callback() semanticsource.DigestView        { return views.callback }
func (views SourceViews) Binding() semanticsource.DigestView         { return views.binding }
func (views SourceViews) Resume() semanticsource.DigestView          { return views.resume }
func (views SourceViews) Spawn() semanticsource.DigestView           { return views.spawn }
func (views SourceViews) Opaque() semanticsource.DigestView          { return views.opaque }
func (views SourceViews) OperationEffect() semanticsource.DigestView { return views.operationEffect }
func (views SourceViews) CallbackEffect() semanticsource.DigestView  { return views.callbackEffect }
func (views SourceViews) PublicationEffect() semanticsource.DigestView {
	return views.publicationEffect
}
func (views SourceViews) CallbackRelease() semanticsource.DigestView { return views.callbackRelease }
func (views SourceViews) Outcome() semanticsource.DigestView         { return views.outcome }
func (views SourceViews) Transfer() semanticsource.DigestView        { return views.transfer }
func (views SourceViews) TransferOutcome() semanticsource.DigestView { return views.transferOutcome }
func (views SourceViews) Suspension() semanticsource.DigestView      { return views.suspension }
func (views SourceViews) ResumeOutcome() semanticsource.DigestView   { return views.resumeOutcome }
func (views SourceViews) SpawnSibling() semanticsource.DigestView    { return views.spawnSibling }
func (views SourceViews) SubedgeArgumentOrigin() semanticsource.DigestView {
	return views.subedgeArgumentOrigin
}
func (views SourceViews) CallbackResult() semanticsource.DigestView  { return views.callbackResult }
func (views SourceViews) ResultAlias() semanticsource.DigestView     { return views.resultAlias }
func (views SourceViews) Produced() semanticsource.DigestView        { return views.produced }
func (views SourceViews) ProducedCapture() semanticsource.DigestView { return views.producedCapture }
func (views SourceViews) FreshResult() semanticsource.DigestView     { return views.freshResult }
func (views SourceViews) Protocol() semanticsource.DigestView        { return views.protocol }
func (views SourceViews) ProtocolState() semanticsource.DigestView   { return views.protocolState }
func (views SourceViews) ProtocolAcquisition() semanticsource.DigestView {
	return views.protocolAcquisition
}
func (views SourceViews) ProtocolTransition() semanticsource.DigestView {
	return views.protocolTransition
}
func (views SourceViews) ProtocolTransitionOutcome() semanticsource.DigestView {
	return views.protocolTransitionOutcome
}
func (views SourceViews) ProtocolEscape() semanticsource.DigestView { return views.protocolEscape }
func (views SourceViews) ProtocolCallbackHolder() semanticsource.DigestView {
	return views.protocolCallbackHolder
}
func (views SourceViews) Boot() semanticsource.DigestView      { return views.boot }
func (views SourceViews) BootEntry() semanticsource.DigestView { return views.bootEntry }
func (views SourceViews) BootMetatableAttachment() semanticsource.DigestView {
	return views.bootMetatableAttachment
}
func (views SourceViews) BootBinding() semanticsource.DigestView { return views.bootBinding }
func (views SourceViews) Gsub() semanticsource.DigestView        { return views.gsub }

// viewFor binds one generated definition to its owner-local typed view. The
// catalog owns the relation list and this switch owns only typed projection;
// no second source vocabulary or cross-owner registry is created.
func (views SourceViews) viewFor(token semanticsource.Token) (semanticsource.DigestView, bool) {
	if token.Origin() == semanticsource.OriginTargetContract && token.Facet() == 0 {
		return views.contract, true
	}
	if token.Origin() == semanticsource.OriginTargetOperation {
		switch token.Facet() {
		case 0:
			return views.operation, true
		case semanticsource.FacetTargetABI:
			return views.abi, true
		case semanticsource.FacetTargetSubedge:
			return views.subedge, true
		case semanticsource.FacetTargetCallback:
			return views.callback, true
		case semanticsource.FacetTargetBinding:
			return views.binding, true
		case semanticsource.FacetTargetResume:
			return views.resume, true
		case semanticsource.FacetTargetSpawn:
			return views.spawn, true
		case semanticsource.FacetTargetOpaque:
			return views.opaque, true
		case semanticsource.FacetTargetOperationEffect:
			return views.operationEffect, true
		case semanticsource.FacetTargetCallbackEffect:
			return views.callbackEffect, true
		case semanticsource.FacetTargetPublicationEffect:
			return views.publicationEffect, true
		case semanticsource.FacetTargetCallbackRelease:
			return views.callbackRelease, true
		case semanticsource.FacetTargetOutcome:
			return views.outcome, true
		case semanticsource.FacetTargetTransfer:
			return views.transfer, true
		case semanticsource.FacetTargetTransferOutcome:
			return views.transferOutcome, true
		case semanticsource.FacetTargetSuspension:
			return views.suspension, true
		case semanticsource.FacetTargetResumeOutcome:
			return views.resumeOutcome, true
		case semanticsource.FacetTargetSpawnSibling:
			return views.spawnSibling, true
		case semanticsource.FacetTargetSubedgeArgumentOrigin:
			return views.subedgeArgumentOrigin, true
		case semanticsource.FacetTargetCallbackResult:
			return views.callbackResult, true
		case semanticsource.FacetTargetResultAlias:
			return views.resultAlias, true
		case semanticsource.FacetTargetProduced:
			return views.produced, true
		case semanticsource.FacetTargetProducedCapture:
			return views.producedCapture, true
		case semanticsource.FacetTargetFreshResult:
			return views.freshResult, true
		}
	}
	if token.Origin() == semanticsource.OriginTargetProtocol {
		switch token.Facet() {
		case 0:
			return views.protocol, true
		case semanticsource.FacetTargetProtocolState:
			return views.protocolState, true
		case semanticsource.FacetTargetProtocolAcquisition:
			return views.protocolAcquisition, true
		case semanticsource.FacetTargetProtocolTransition:
			return views.protocolTransition, true
		case semanticsource.FacetTargetProtocolTransitionOutcome:
			return views.protocolTransitionOutcome, true
		case semanticsource.FacetTargetProtocolEscape:
			return views.protocolEscape, true
		case semanticsource.FacetTargetProtocolCallbackHolder:
			return views.protocolCallbackHolder, true
		}
	}
	if token.Origin() == semanticsource.OriginTargetBoot {
		switch token.Facet() {
		case 0:
			return views.boot, true
		case semanticsource.FacetTargetBootEntry:
			return views.bootEntry, true
		case semanticsource.FacetTargetBootMetatableAttachment:
			return views.bootMetatableAttachment, true
		case semanticsource.FacetTargetBootBinding:
			return views.bootBinding, true
		}
	}
	if token.Origin() == semanticsource.OriginTargetGsub && token.Facet() == 0 {
		return views.gsub, true
	}
	return semanticsource.DigestView{}, false
}

// Publications projects the Target-owned source rows through the sealed
// ProgramSchema. Target owns row cardinalities; the schema owns relation
// membership and canonical enumeration.
func (views SourceViews) Publications(schema semanticsource.ProgramSchema) []semanticsource.Publication {
	if !views.Valid() {
		return nil
	}
	return semanticsource.OriginPublications(schema, func(token semanticsource.Token) (int, bool) {
		row, found := views.viewFor(token)
		if !found {
			return 0, false
		}
		return row.Count(), true
	},
		semanticsource.OriginTargetContract, semanticsource.OriginTargetOperation,
		semanticsource.OriginTargetProtocol, semanticsource.OriginTargetBoot, semanticsource.OriginTargetGsub)
}

func (views SourceViews) Valid() bool {
	return views.owner.Available() && views.valid()
}
func (c *Contract) SourceViews() (SourceViews, bool) {
	if c == nil || !c.sealed || len(c.operations) == 0 || c.opaque != Operation(len(c.operations)) || !c.sourceViews.Valid() || c.sourceViews.OwnerID() != c.ContentID() {
		return SourceViews{}, false
	}
	return c.sourceViews, true
}

func (c *Contract) semanticSourceReady() bool {
	return c != nil && c.sealed && len(c.operations) != 0 && c.opaque == Operation(len(c.operations)) && c.ContentID().Available()
}
