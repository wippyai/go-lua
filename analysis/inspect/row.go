package inspect

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

// formatRow renders the solved state held at one identity: the query row it
// belongs to, its published cell, and every factor column of that cell under
// the reduction outcome the fold concluded with.
func formatRow(session *Session, id identity.ContentID) string {
	var b strings.Builder
	writef(&b, "row.Identity=%s", id)
	record, ok := session.Lookup(id)
	if !ok {
		writef(&b, "session.Lookup(%s)=false", id)
		writeCoordinate(&b, session, id)
		return b.String()
	}
	writef(&b, "session.Lookup(%s).Kind=%s", id, record.kind.String())
	switch record.kind {
	case rowBody:
		if solved := session.result; solved != nil {
			body, bodyOK := solved.BodyAt(record.body)
			if bodyOK {
				if bodyID, idOK := body.ID(); idOK {
					writef(&b, "result.BodyAt(%d).ID=%s", record.body, bodyID)
				}
				writef(&b, "result.BodyAt(%d).RootCount=%d", record.body, body.RootCount())
				for rootIndex := 0; rootIndex < body.RootCount(); rootIndex++ {
					root, rootOK := body.RootAt(rootIndex)
					if !rootOK {
						continue
					}
					if rootID, idOK := root.ID(); idOK {
						writef(&b, "result.BodyAt(%d).RootAt(%d).ID=%s", record.body, rootIndex, rootID)
					}
					writef(&b, "result.BodyAt(%d).RootAt(%d).Family=%d", record.body, rootIndex, uint8(root.Family()))
				}
			}
		}
	case rowQuery:
		writeQueryRow(&b, session, record)
	case rowNative:
		if solved := session.result; solved != nil {
			row, rowOK := solved.NativePublicationAt(record.native)
			if rowOK {
				if nativeID, idOK := row.ID(); idOK {
					writef(&b, "result.NativePublicationAt(%d).ID=%s", record.native, nativeID)
				}
				writef(&b, "result.NativePublicationAt(%d).Kind=%s", record.native, row.Kind().String())
				writef(&b, "result.NativePublicationAt(%d).Lane=%s", record.native, row.Lane().String())
				writef(&b, "result.NativePublicationAt(%d).Trust=%s", record.native, row.Trust().String())
				writef(&b, "result.NativePublicationAt(%d).Family=%s", record.native, row.Family())
				writef(&b, "result.NativePublicationAt(%d).Exact=%t", record.native, row.Exact())
			}
		}
	case rowFinding:
		if report := session.report; report != nil && report.Available() {
			finding, findingOK := report.FindingAt(record.finding)
			if findingOK {
				if findingID, idOK := finding.ID(); idOK {
					writef(&b, "report.FindingAt(%d).ID=%s", record.finding, findingID)
				}
				if subject, subjectOK := finding.SubjectID(); subjectOK {
					writef(&b, "report.FindingAt(%d).SubjectID=%s", record.finding, subject)
				}
				writef(&b, "report.FindingAt(%d).Code=%s", record.finding, finding.Code())
				writef(&b, "report.FindingAt(%d).Render=%s", record.finding, compactSpace(finding.Render()))
			}
		}
	case rowOperation:
		writef(&b, "contract.Operations.OperationAt(%d)=%d", record.operation, record.operation+1)
		writef(&b, "contract.OperationContentID=%s", record.id)
	}
	return b.String()
}

func writeQueryRow(b *strings.Builder, session *Session, record rowRecord) {
	solved := session.result
	if solved == nil {
		writef(b, "result=unavailable")
		return
	}
	family, familyOK := solved.FamilyAt(record.queryFamily)
	if !familyOK {
		return
	}
	query, queryOK := family.QueryAt(record.query)
	if !queryOK {
		return
	}
	prefix := queryPrefix(record.queryFamily, record.query)
	writef(b, "result.FamilyAt(%d).Key=%s", record.queryFamily, family.Key())
	writef(b, "result.FamilyAt(%d).Codec=%s", record.queryFamily, semanticSpelling(family.Codec()))
	if site, siteOK := query.SiteID(); siteOK {
		writef(b, "%s.SiteID=%s", prefix, site)
	}
	if key, keyOK := query.PublicationKey(); keyOK {
		writef(b, "%s.PublicationKey=%s", prefix, key)
	}
	if point, pointOK := query.PointID(); pointOK {
		writef(b, "%s.PointID=%s", prefix, point)
	}
	if mount, mountOK := query.MountID(); mountOK {
		writef(b, "%s.MountID=%s", prefix, mount)
	}
	if context, contextOK := query.ContextID(); contextOK {
		writef(b, "%s.ContextID=%s", prefix, context)
	}
	writeCell(b, session, prefix, record.queryFamily, record.query)
	for _, gap := range solvedGaps() {
		writef(b, "unexposed.%s=%s", gap.Layer, gap.Accessor)
	}
}

// writeCoordinate answers an identity that is not a session row by searching
// every published answer's coordinate plane for it. A coordinate is a row of
// somebody's cell rather than a row of the session index, so the bisection
// over the admitted payload is the only place it can be found.
func writeCoordinate(b *strings.Builder, session *Session, id identity.ContentID) {
	solved := session.result
	if solved == nil {
		writef(b, "result=unavailable")
		return
	}
	found := false
	for familyIndex := 0; familyIndex < solved.FamilyCount(); familyIndex++ {
		family, familyOK := solved.FamilyAt(familyIndex)
		if !familyOK {
			continue
		}
		layout, layoutOK := session.plan.QueryResultLayout(family.Key())
		if !layoutOK {
			continue
		}
		for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
			view, refusal, viewOK := session.CellView(familyIndex, queryIndex)
			if !viewOK || refusal.Available() {
				continue
			}
			row, rowOK := view.Lookup(id)
			if !rowOK {
				continue
			}
			found = true
			prefix := queryPrefix(familyIndex, queryIndex)
			writef(b, "result.FamilyAt(%d).Key=%s", familyIndex, family.Key())
			writeRow(b, layout, prefix, coordinatePosition(view, id), row)
		}
	}
	if !found {
		writef(b, "plane.View.Lookup=false")
	}
}

// coordinatePosition is the row's position in the coordinate plane it was
// found in. The plane ascends, so the position is a bisection, not a scan.
func coordinatePosition(view plane.View, id identity.ContentID) int {
	low, high := 0, view.RowCount()
	for low < high {
		middle := int(uint(low+high) >> 1)
		row, ok := view.At(middle)
		if !ok {
			return -1
		}
		held := row.ID()
		switch {
		case held == id:
			return middle
		case lessIdentity(held, id):
			low = middle + 1
		default:
			high = middle
		}
	}
	return -1
}

func lessIdentity(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func queryPrefix(familyIndex, queryIndex int) string {
	return "result.FamilyAt(" + decimal(uint64(familyIndex)) + ").QueryAt(" + decimal(uint64(queryIndex)) + ")"
}
