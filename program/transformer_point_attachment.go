package program

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
)

// pointAttachmentReceipt is the sole cold Program projection from exact
// Causal Sites to the parent-issued LocalWTO points that attach them. A Site
// may attach to more than one phase point; the receipt preserves the parent
// event order and never derives a point from a Term or a Site identity.
type pointAttachmentReceipt struct {
	owner  *Program
	rows   []pointAttachmentRow
	bySite map[keyspace.ContentID][]uint32
}

type pointAttachmentRow struct {
	site  flow.Site
	point flow.WTOPoint
}

func buildPointAttachmentReceipt(owner *Program) (*pointAttachmentReceipt, bool) {
	if owner == nil || owner.flow == nil || !owner.id.Available() {
		return nil, false
	}
	receipt := &pointAttachmentReceipt{owner: owner, bySite: make(map[keyspace.ContentID][]uint32)}
	seenPoints := make(map[keyspace.ContentID]struct{})
	wto := owner.Flow().Local().WTO()
	for eventIndex := 0; eventIndex < wto.EventCount(); eventIndex++ {
		event, eventOK := wto.EventAt(eventIndex)
		if !eventOK || !event.Available() || event.Kind() != flow.WTOEventPoint {
			continue
		}
		point, pointOK := event.Point()
		if !pointOK || !point.Available() || !point.PathID().Available() {
			return nil, false
		}
		if _, duplicate := seenPoints[point.PathID()]; duplicate {
			return nil, false
		}
		seenPoints[point.PathID()] = struct{}{}
		for siteIndex := 0; siteIndex < point.SiteCount(); siteIndex++ {
			site, siteOK := point.SiteAt(siteIndex)
			if !siteOK || !site.Available() || !site.ContextID().Available() {
				return nil, false
			}
			rowIndex := len(receipt.rows)
			if uint64(rowIndex) > uint64(^uint32(0)) {
				return nil, false
			}
			for _, prior := range receipt.bySite[site.ContextID()] {
				if int(prior) >= len(receipt.rows) || receipt.rows[prior].point.PathID() == point.PathID() {
					return nil, false
				}
			}
			receipt.rows = append(receipt.rows, pointAttachmentRow{site: site, point: point})
			receipt.bySite[site.ContextID()] = append(receipt.bySite[site.ContextID()], uint32(rowIndex))
		}
	}
	return receipt, receipt.valid(owner)
}

func (receipt *pointAttachmentReceipt) valid(owner *Program) bool {
	return receipt != nil && receipt.owner == owner && owner != nil && receipt.bySite != nil
}

// PointAttachments is the exact owner-fenced multi-valued Site-to-WTOPoint
// receipt. It is a view over the one publication sidecar; no Flow scan or
// coordinate reconstruction occurs at query time.
type PointAttachments struct {
	input TransformerInput
	site  flow.Site
}

func (input TransformerInput) PointAttachments(site flow.Site) PointAttachments {
	if !input.Available() || !input.OwnsSite(site) {
		return PointAttachments{}
	}
	return PointAttachments{input: input, site: site}
}

func (attachments PointAttachments) Available() bool {
	return attachments.input.Available() && attachments.input.OwnsSite(attachments.site) &&
		attachments.input.pointAttachments == attachments.input.owner.pointAttachments
}

func (attachments PointAttachments) Count() int {
	if !attachments.Available() {
		return 0
	}
	return len(attachments.input.pointAttachments.bySite[attachments.site.ContextID()])
}

func (attachments PointAttachments) At(index int) (PointAttachment, bool) {
	if !attachments.Available() || index < 0 {
		return PointAttachment{}, false
	}
	rows := attachments.input.pointAttachments.bySite[attachments.site.ContextID()]
	if index >= len(rows) || int(rows[index]) >= len(attachments.input.pointAttachments.rows) {
		return PointAttachment{}, false
	}
	attachment := PointAttachment{input: attachments.input, site: attachments.site, index: rows[index]}
	return attachment, attachment.Available()
}

// PointAttachment is one immutable Site-to-parent-phase-point proof.
type PointAttachment struct {
	input TransformerInput
	site  flow.Site
	index uint32
}

func (attachment PointAttachment) Available() bool {
	if !attachment.input.Available() || !attachment.input.OwnsSite(attachment.site) ||
		attachment.input.pointAttachments != attachment.input.owner.pointAttachments || int(attachment.index) >= len(attachment.input.pointAttachments.rows) {
		return false
	}
	row := attachment.input.pointAttachments.rows[attachment.index]
	if !attachment.input.OwnsSite(row.site) || !row.site.Equal(attachment.site) || !row.point.Available() || !row.point.PathID().Available() {
		return false
	}
	indices := attachment.input.pointAttachments.bySite[attachment.site.ContextID()]
	for _, index := range indices {
		if index == attachment.index {
			return true
		}
	}
	return false
}

func (attachment PointAttachment) Site() (flow.Site, bool) {
	if !attachment.Available() {
		return flow.Site{}, false
	}
	return attachment.site, true
}

func (attachment PointAttachment) Point() (flow.WTOPoint, bool) {
	if !attachment.Available() {
		return flow.WTOPoint{}, false
	}
	return attachment.input.pointAttachments.rows[attachment.index].point, true
}

func (attachment PointAttachment) PointPathID() keyspace.ContentID {
	point, ok := attachment.Point()
	if !ok {
		return keyspace.ContentID{}
	}
	return point.PathID()
}

func (input TransformerInput) OwnsPointAttachment(attachment PointAttachment) bool {
	return input.Available() && attachment.input == input && attachment.Available()
}
