package target

import (
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// SemanticSourceView is one detached Target-owned source-family receipt. Its
// rows are identities traversed from the typed Contract projections; the
// owner identity fences the receipt to this exact Contract replay.
type SemanticSourceView struct {
	owner   keyspace.ContentID
	digests []keyspace.ContentID
}

func (view SemanticSourceView) valid() bool {
	if !view.owner.Available() {
		return false
	}
	for _, digest := range view.digests {
		if !digest.Available() {
			return false
		}
	}
	return true
}

// OwnerID returns the sealed Contract identity that owns this detached view.
func (view SemanticSourceView) OwnerID() keyspace.ContentID { return view.owner }

// Count reports the detached typed row count, including zero. A malformed
// owner receipt never exposes a partial count.
func (view SemanticSourceView) Count() int {
	if !view.valid() {
		return 0
	}
	return len(view.digests)
}

// DigestAt returns one canonical typed-row identity. Both the detached count
// and backing slice bound the access, so a count/row-length mismatch cannot
// leak a phantom row.
func (view SemanticSourceView) DigestAt(index int) (keyspace.ContentID, bool) {
	if !view.valid() || index < 0 || index >= len(view.digests) {
		return keyspace.ContentID{}, false
	}
	digest := view.digests[index]
	return digest, true
}

// Digests returns an owner-independent copy of the canonical typed identities.
func (view SemanticSourceView) Digests() []keyspace.ContentID {
	if !view.valid() {
		return nil
	}
	return append([]keyspace.ContentID(nil), view.digests...)
}

// SemanticSourceCursor walks one exact Target view. It remains owner-bound by
// retaining the view's sealed Contract identity and never erases its family.
type SemanticSourceCursor struct {
	view  SemanticSourceView
	index int
}

func (view SemanticSourceView) Cursor() SemanticSourceCursor {
	return SemanticSourceCursor{view: view}
}

func (cursor *SemanticSourceCursor) Next() (keyspace.ContentID, bool) {
	if cursor == nil || !cursor.view.valid() || cursor.index < 0 || cursor.index >= len(cursor.view.digests) {
		return keyspace.ContentID{}, false
	}
	digest := cursor.view.digests[cursor.index]
	cursor.index++
	return digest, digest.Available()
}

// SemanticSourceViews names every Target source family explicitly. The
// generated semantic-source schema remains the vocabulary authority; the
// owner-local viewFor switch only binds those definitions to typed receipts.
type SemanticSourceViews struct {
	owner keyspace.ContentID

	contract, operation, abi, subedge, callback, binding, resume, spawn, opaque                 SemanticSourceView
	operationEffect, callbackEffect, callbackRelease, outcome, transfer, transferOutcome        SemanticSourceView
	suspension, resumeOutcome, spawnSibling, subedgeArgumentOrigin, callbackResult              SemanticSourceView
	resultAlias, produced, producedCapture, freshResult                                         SemanticSourceView
	protocol, protocolState, protocolAcquisition, protocolTransition, protocolTransitionOutcome SemanticSourceView
	protocolEscape, protocolCallbackHolder                                                      SemanticSourceView
	boot, bootEntry, bootMetatableAttachment, bootBinding, gsub                                 SemanticSourceView
}

func (views SemanticSourceViews) valid() bool {
	if !views.owner.Available() {
		return false
	}
	for _, view := range views.all() {
		if !view.valid() || view.owner != views.owner {
			return false
		}
	}
	return true
}

func (views SemanticSourceViews) all() []SemanticSourceView {
	return []SemanticSourceView{
		views.contract, views.operation, views.abi, views.subedge, views.callback, views.binding, views.resume, views.spawn, views.opaque,
		views.operationEffect, views.callbackEffect, views.callbackRelease, views.outcome, views.transfer, views.transferOutcome,
		views.suspension, views.resumeOutcome, views.spawnSibling, views.subedgeArgumentOrigin, views.callbackResult,
		views.resultAlias, views.produced, views.producedCapture, views.freshResult,
		views.protocol, views.protocolState, views.protocolAcquisition, views.protocolTransition, views.protocolTransitionOutcome,
		views.protocolEscape, views.protocolCallbackHolder, views.boot, views.bootEntry, views.bootMetatableAttachment,
		views.bootBinding, views.gsub,
	}
}

func (views SemanticSourceViews) OwnerID() keyspace.ContentID         { return views.owner }
func (views SemanticSourceViews) Contract() SemanticSourceView        { return views.contract }
func (views SemanticSourceViews) Operation() SemanticSourceView       { return views.operation }
func (views SemanticSourceViews) ABI() SemanticSourceView             { return views.abi }
func (views SemanticSourceViews) Subedge() SemanticSourceView         { return views.subedge }
func (views SemanticSourceViews) Callback() SemanticSourceView        { return views.callback }
func (views SemanticSourceViews) Binding() SemanticSourceView         { return views.binding }
func (views SemanticSourceViews) Resume() SemanticSourceView          { return views.resume }
func (views SemanticSourceViews) Spawn() SemanticSourceView           { return views.spawn }
func (views SemanticSourceViews) Opaque() SemanticSourceView          { return views.opaque }
func (views SemanticSourceViews) OperationEffect() SemanticSourceView { return views.operationEffect }
func (views SemanticSourceViews) CallbackEffect() SemanticSourceView  { return views.callbackEffect }
func (views SemanticSourceViews) CallbackRelease() SemanticSourceView { return views.callbackRelease }
func (views SemanticSourceViews) Outcome() SemanticSourceView         { return views.outcome }
func (views SemanticSourceViews) Transfer() SemanticSourceView        { return views.transfer }
func (views SemanticSourceViews) TransferOutcome() SemanticSourceView { return views.transferOutcome }
func (views SemanticSourceViews) Suspension() SemanticSourceView      { return views.suspension }
func (views SemanticSourceViews) ResumeOutcome() SemanticSourceView   { return views.resumeOutcome }
func (views SemanticSourceViews) SpawnSibling() SemanticSourceView    { return views.spawnSibling }
func (views SemanticSourceViews) SubedgeArgumentOrigin() SemanticSourceView {
	return views.subedgeArgumentOrigin
}
func (views SemanticSourceViews) CallbackResult() SemanticSourceView  { return views.callbackResult }
func (views SemanticSourceViews) ResultAlias() SemanticSourceView     { return views.resultAlias }
func (views SemanticSourceViews) Produced() SemanticSourceView        { return views.produced }
func (views SemanticSourceViews) ProducedCapture() SemanticSourceView { return views.producedCapture }
func (views SemanticSourceViews) FreshResult() SemanticSourceView     { return views.freshResult }
func (views SemanticSourceViews) Protocol() SemanticSourceView        { return views.protocol }
func (views SemanticSourceViews) ProtocolState() SemanticSourceView   { return views.protocolState }
func (views SemanticSourceViews) ProtocolAcquisition() SemanticSourceView {
	return views.protocolAcquisition
}
func (views SemanticSourceViews) ProtocolTransition() SemanticSourceView {
	return views.protocolTransition
}
func (views SemanticSourceViews) ProtocolTransitionOutcome() SemanticSourceView {
	return views.protocolTransitionOutcome
}
func (views SemanticSourceViews) ProtocolEscape() SemanticSourceView { return views.protocolEscape }
func (views SemanticSourceViews) ProtocolCallbackHolder() SemanticSourceView {
	return views.protocolCallbackHolder
}
func (views SemanticSourceViews) Boot() SemanticSourceView      { return views.boot }
func (views SemanticSourceViews) BootEntry() SemanticSourceView { return views.bootEntry }
func (views SemanticSourceViews) BootMetatableAttachment() SemanticSourceView {
	return views.bootMetatableAttachment
}
func (views SemanticSourceViews) BootBinding() SemanticSourceView { return views.bootBinding }
func (views SemanticSourceViews) Gsub() SemanticSourceView        { return views.gsub }

// viewFor binds one generated definition to its owner-local typed view. The
// catalog owns the relation list and this switch owns only typed projection;
// no second source vocabulary or cross-owner registry is created.
func (views SemanticSourceViews) viewFor(token semanticsource.Token) (SemanticSourceView, bool) {
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
	return SemanticSourceView{}, false
}

// SemanticSourceReceipt is a detached owner-bound source publication. The
// receipt preserves the Contract identity and all typed row digests without
// retaining a Contract pointer or any hot Target tables.
type SemanticSourceReceipt struct {
	owner keyspace.ContentID
	views SemanticSourceViews
}

func (receipt SemanticSourceReceipt) Publications() []semanticsource.Publication {
	if !receipt.Valid() {
		return nil
	}
	views, ok := receipt.Views()
	if !ok {
		return nil
	}
	schema := semanticsource.CatalogSchema()
	rows := make([]semanticsource.Publication, 0, 36)
	for index := 0; index < schema.Count(); index++ {
		definition, defined := schema.DefinitionAt(index)
		if !defined {
			return nil
		}
		origin := definition.Token().Origin()
		if origin != semanticsource.OriginTargetContract && origin != semanticsource.OriginTargetOperation && origin != semanticsource.OriginTargetProtocol && origin != semanticsource.OriginTargetBoot && origin != semanticsource.OriginTargetGsub {
			continue
		}
		view, found := views.viewFor(definition.Token())
		if !found || !view.valid() {
			return nil
		}
		publication, err := semanticsource.SealPublication(definition, view.Count())
		if err != nil {
			return nil
		}
		rows = append(rows, publication)
	}
	return rows
}

func (receipt SemanticSourceReceipt) Valid() bool {
	return receipt.owner.Available() && receipt.views.valid() && receipt.views.owner == receipt.owner
}
func (receipt SemanticSourceReceipt) OwnerID() keyspace.ContentID { return receipt.owner }
func (receipt SemanticSourceReceipt) Views() (SemanticSourceViews, bool) {
	if !receipt.Valid() {
		return SemanticSourceViews{}, false
	}
	return receipt.views, true
}

func (c *Contract) SemanticSourceReceipt() (SemanticSourceReceipt, bool) {
	if c == nil || !c.sealed || len(c.operations) == 0 || c.opaque != Operation(len(c.operations)) || !c.semanticReceipt.Valid() || c.semanticReceipt.OwnerID() != c.ContentID() {
		return SemanticSourceReceipt{}, false
	}
	return c.semanticReceipt, true
}

func (c *Contract) SemanticSourceViews() (SemanticSourceViews, bool) {
	receipt, ok := c.SemanticSourceReceipt()
	if !ok {
		return SemanticSourceViews{}, false
	}
	return receipt.Views()
}

func (c *Contract) semanticSourceReady() bool {
	return c != nil && c.sealed && len(c.operations) != 0 && c.opaque == Operation(len(c.operations)) && c.ContentID().Available()
}
