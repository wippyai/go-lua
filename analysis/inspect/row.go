package inspect

import (
	"encoding/hex"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/analysis/schema"
)

type rowKind uint8

const (
	rowNone rowKind = iota
	rowBody
	rowQuery
	rowNative
	rowFinding
	rowOperation
)

// rowRecord is the sealed scalar index of one inspectable identity. Lookup
// copies this value; it does not allocate.
type rowRecord struct {
	kind         rowKind
	id           identity.ContentID
	alt          identity.ContentID
	family       schema.Key
	status       result.QueryStatus
	present      bool
	rows         uint64
	cell         identity.ContentID
	point        identity.ContentID
	mount        identity.ContentID
	context      identity.ContentID
	nativeKind   result.NativePublicationKind
	nativeLane   result.NativePublicationLane
	nativeFamily string
	code         anadiag.DiagnosticCode
	roots        int
	operation    int
	body         int
	queryFamily  int
	query        int
	native       int
	finding      int
}

func (session *Session) index() {
	if session == nil {
		return
	}
	session.records = session.records[:0]
	session.byID = make(map[identity.ContentID]int)
	if session.contract != nil {
		ops := session.contract.Operations
		for index := 0; index < ops.OperationCount(); index++ {
			op, ok := ops.OperationAt(index)
			if !ok {
				continue
			}
			id, idOK := session.contract.OperationContentID(op)
			if !idOK || !id.Available() {
				continue
			}
			session.addRecord(rowRecord{kind: rowOperation, id: id, operation: index})
		}
	}
	if solved := session.result; solved != nil {
		for index := 0; index < solved.BodyCount(); index++ {
			body, ok := solved.BodyAt(index)
			if !ok {
				continue
			}
			id, idOK := body.ID()
			if !idOK || !id.Available() {
				continue
			}
			session.addRecord(rowRecord{kind: rowBody, id: id, body: index, roots: body.RootCount()})
		}
		for familyIndex := 0; familyIndex < solved.FamilyCount(); familyIndex++ {
			family, familyOK := solved.FamilyAt(familyIndex)
			if !familyOK {
				continue
			}
			for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
				query, queryOK := family.QueryAt(queryIndex)
				if !queryOK {
					continue
				}
				site, siteOK := query.SiteID()
				if !siteOK || !site.Available() {
					continue
				}
				record := rowRecord{
					kind:        rowQuery,
					id:          site,
					family:      family.Key(),
					status:      query.Status(),
					queryFamily: familyIndex,
					query:       queryIndex,
				}
				if key, keyOK := query.PublicationKey(); keyOK {
					record.alt = key
				}
				if point, pointOK := query.PointID(); pointOK {
					record.point = point
				}
				if mount, mountOK := query.MountID(); mountOK {
					record.mount = mount
				}
				if contextID, contextOK := query.ContextID(); contextOK {
					record.context = contextID
				}
				if cell, cellOK := query.Cell(); cellOK {
					record.present = cell.Present()
					record.rows = cell.RowCount()
					record.cell = cell.ContentID()
				}
				session.addRecord(record)
				if record.alt.Available() && record.alt != record.id {
					alias := record
					alias.id = record.alt
					session.addRecord(alias)
				}
			}
		}
		for index := 0; index < solved.NativePublicationCount(); index++ {
			row, ok := solved.NativePublicationAt(index)
			if !ok {
				continue
			}
			id, idOK := row.ID()
			if !idOK || !id.Available() {
				continue
			}
			session.addRecord(rowRecord{
				kind:         rowNative,
				id:           id,
				native:       index,
				nativeKind:   row.Kind(),
				nativeLane:   row.Lane(),
				nativeFamily: row.Family(),
			})
		}
	}
	if report := session.report; report != nil && report.Available() {
		for index := 0; index < report.FindingCount(); index++ {
			finding, ok := report.FindingAt(index)
			if !ok {
				continue
			}
			id, idOK := finding.ID()
			if !idOK || !id.Available() {
				continue
			}
			record := rowRecord{kind: rowFinding, id: id, finding: index, code: finding.Code()}
			if subject, subjectOK := finding.SubjectID(); subjectOK {
				record.alt = subject
			}
			session.addRecord(record)
			if record.alt.Available() && record.alt != record.id {
				alias := record
				alias.id = record.alt
				session.addRecord(alias)
			}
		}
	}
}

func (session *Session) addRecord(record rowRecord) {
	if session == nil || !record.id.Available() {
		return
	}
	if _, exists := session.byID[record.id]; exists {
		return
	}
	session.byID[record.id] = len(session.records)
	session.records = append(session.records, record)
}

// Lookup returns the indexed row for id. After Open it copies sealed scalars
// and does not allocate.
func (session *Session) Lookup(id identity.ContentID) (rowRecord, bool) {
	if session == nil || session.byID == nil || !id.Available() {
		return rowRecord{}, false
	}
	index, ok := session.byID[id]
	if !ok || index < 0 || index >= len(session.records) {
		return rowRecord{}, false
	}
	return session.records[index], true
}

// ParseContentID admits a lower-case hex identity.
func ParseContentID(text string) (identity.ContentID, bool) {
	var id identity.ContentID
	if len(text) != hex.EncodedLen(len(id)) {
		return identity.ContentID{}, false
	}
	n, err := hex.Decode(id[:], []byte(text))
	if err != nil || n != len(id) || !id.Available() {
		return identity.ContentID{}, false
	}
	return id, true
}
