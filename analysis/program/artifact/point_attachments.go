package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// PointAttachmentRow is one immutable Site-to-LocalWTO point relation copied
// from the canonical Flow schedule. It carries only the two parent-issued
// identities needed by artifact consumers; no Program, Flow, or transformer
// proof crosses the artifact boundary.
type PointAttachmentRow struct {
	site  identity.ContentID
	point identity.ContentID
}

func (row PointAttachmentRow) Available() bool {
	return row.site.Available() && row.point.Available()
}

func (row PointAttachmentRow) SiteID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.site
}

func (row PointAttachmentRow) PointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}
