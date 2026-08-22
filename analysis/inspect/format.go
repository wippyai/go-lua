package inspect

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func formatTarget(session *Session) string {
	var b strings.Builder
	writef(&b, "session.Fixture=%s", session.fixture)
	writef(&b, "compilation.Available=%t", session.compilation.Available())
	writef(&b, "session.CompileStatus.CompileComplete=%t", session.compileStatus == analysis.CompileComplete)
	if session.compilation.Available() {
		writef(&b, "compilation.Digest=%s", session.compilation.Digest())
		if table, ok := session.compilation.Structure(); ok {
			for category := structure.CategoryArm; category.Available(); category++ {
				count := table.Count(category)
				writef(&b, "compilation.Structure.Count(%d)=%d", uint8(category), count)
				for ordinal := uint16(1); ordinal <= uint16(count); ordinal++ {
					entry, entryOK := table.At(category, ordinal)
					if !entryOK || entry == nil {
						continue
					}
					writef(&b, "compilation.Structure.At(%d,%d).Key=%s", uint8(category), ordinal, entry.Key())
					writef(&b, "compilation.Structure.At(%d,%d).Spelling=%s", uint8(category), ordinal, entry.Spelling())
				}
			}
		}
	}
	if session.contract != nil && session.contract.ContentID().Available() {
		writef(&b, "contract.ContentID=%s", session.contract.ContentID())
		ops := session.contract.Operations
		writef(&b, "contract.Operations.OperationCount=%d", ops.OperationCount())
		writef(&b, "contract.Operations.SourceCount=%d", ops.SourceCount())
		writef(&b, "contract.Operations.BoundCount=%d", ops.BoundCount())
		for index := 0; index < ops.OperationCount(); index++ {
			op, ok := ops.OperationAt(index)
			if !ok {
				continue
			}
			writef(&b, "contract.Operations.OperationAt(%d)=%d", index, uint32(op))
			if id, idOK := session.contract.OperationContentID(op); idOK {
				writef(&b, "contract.OperationContentID(%d)=%s", uint32(op), id)
			}
			if anchor, anchorOK := ops.Anchor(op); anchorOK {
				writef(&b, "contract.Operations.Anchor(%d)=%s", uint32(op), anchor)
			}
		}
		protocols := session.contract.Protocols()
		writef(&b, "contract.Protocols.ProtocolCount=%d", protocols.ProtocolCount())
		for index := 0; index < protocols.ProtocolCount(); index++ {
			protocol, ok := protocols.ProtocolAt(index)
			if !ok {
				continue
			}
			writef(&b, "contract.Protocols.ProtocolAt(%d)=%d", index, uint32(protocol))
			writef(&b, "contract.Protocols.StateCount(%d)=%d", uint32(protocol), protocols.StateCount(protocol))
			writef(&b, "contract.Protocols.TransitionCount(%d)=%d", uint32(protocol), protocols.TransitionCount(protocol))
			for stateIndex := 0; stateIndex < protocols.StateCount(protocol); stateIndex++ {
				state, stateOK := protocols.StateAt(protocol, stateIndex)
				if !stateOK {
					continue
				}
				if name, nameOK := protocols.StateName(protocol, state); nameOK {
					writef(&b, "contract.Protocols.StateName(%d,%d)=%s", uint32(protocol), uint32(state), name)
				}
			}
		}
		writef(&b, "contract.ExactKeyCount=%d", session.contract.ExactKeyCount())
	}
	return b.String()
}

func formatRows(session *Session) string {
	var b strings.Builder
	writef(&b, "session.Fixture=%s", session.fixture)
	writef(&b, "compilation.Available=%t", session.compilation.Available())
	writef(&b, "session.CompileStatus.CompileComplete=%t", session.compileStatus == analysis.CompileComplete)
	writef(&b, "session.SolveStatus.AnalyzeComplete=%t", session.solveStatus == analysis.AnalyzeComplete)
	solved := session.result
	if solved == nil {
		writef(&b, "result=unavailable")
	} else {
		writef(&b, "result.SourceID=%s", solved.SourceID())
		writef(&b, "result.ContentID=%s", solved.ContentID())
		writef(&b, "result.BodyCount=%d", solved.BodyCount())
		for index := 0; index < solved.BodyCount(); index++ {
			body, ok := solved.BodyAt(index)
			if !ok {
				continue
			}
			if id, idOK := body.ID(); idOK {
				writef(&b, "result.BodyAt(%d).ID=%s", index, id)
			}
			writef(&b, "result.BodyAt(%d).RootCount=%d", index, body.RootCount())
			for rootIndex := 0; rootIndex < body.RootCount(); rootIndex++ {
				root, rootOK := body.RootAt(rootIndex)
				if !rootOK {
					continue
				}
				if id, idOK := root.ID(); idOK {
					writef(&b, "result.BodyAt(%d).RootAt(%d).ID=%s", index, rootIndex, id)
				}
				writef(&b, "result.BodyAt(%d).RootAt(%d).Family=%d", index, rootIndex, uint8(root.Family()))
			}
		}
		writef(&b, "result.FamilyCount=%d", solved.FamilyCount())
		for familyIndex := 0; familyIndex < solved.FamilyCount(); familyIndex++ {
			family, familyOK := solved.FamilyAt(familyIndex)
			if !familyOK {
				continue
			}
			writef(&b, "result.FamilyAt(%d).Key=%s", familyIndex, family.Key())
			writef(&b, "result.FamilyAt(%d).ID=%s", familyIndex, family.ID())
			writef(&b, "result.FamilyAt(%d).ContractID=%s", familyIndex, family.ContractID())
			writef(&b, "result.FamilyAt(%d).QueryCount=%d", familyIndex, family.QueryCount())
		}
		writef(&b, "result.NativePublicationCount=%d", solved.NativePublicationCount())
	}
	for _, gap := range Gaps() {
		writef(&b, "unexposed.%s=%s", gap.Layer, gap.Accessor)
	}
	return b.String()
}

func formatRow(session *Session, id identity.ContentID) string {
	var b strings.Builder
	record, ok := session.Lookup(id)
	if !ok {
		writef(&b, "Lookup(%s)=false", id)
		return b.String()
	}
	writef(&b, "Lookup(%s).kind=%d", id, uint8(record.kind))
	switch record.kind {
	case rowBody:
		if solved := session.result; solved != nil {
			body, bodyOK := solved.BodyAt(record.body)
			if bodyOK {
				if bodyID, idOK := body.ID(); idOK {
					writef(&b, "result.BodyAt(%d).ID=%s", record.body, bodyID)
				}
				writef(&b, "result.BodyAt(%d).RootCount=%d", record.body, body.RootCount())
			}
		}
	case rowQuery:
		if solved := session.result; solved != nil {
			family, familyOK := solved.FamilyAt(record.queryFamily)
			if familyOK {
				query, queryOK := family.QueryAt(record.query)
				if queryOK {
					writef(&b, "result.FamilyAt(%d).Key=%s", record.queryFamily, family.Key())
					if site, siteOK := query.SiteID(); siteOK {
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).SiteID=%s", record.queryFamily, record.query, site)
					}
					if key, keyOK := query.PublicationKey(); keyOK {
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).PublicationKey=%s", record.queryFamily, record.query, key)
					}
					writef(&b, "result.FamilyAt(%d).QueryAt(%d).Status=%d", record.queryFamily, record.query, uint8(query.Status()))
					if point, pointOK := query.PointID(); pointOK {
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).PointID=%s", record.queryFamily, record.query, point)
					}
					if mount, mountOK := query.MountID(); mountOK {
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).MountID=%s", record.queryFamily, record.query, mount)
					}
					if contextID, contextOK := query.ContextID(); contextOK {
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).ContextID=%s", record.queryFamily, record.query, contextID)
					}
					if cell, cellOK := query.Cell(); cellOK {
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).Cell.Present=%t", record.queryFamily, record.query, cell.Present())
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).Cell.RowCount=%d", record.queryFamily, record.query, cell.RowCount())
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).Cell.ContentID=%s", record.queryFamily, record.query, cell.ContentID())
						writef(&b, "result.FamilyAt(%d).QueryAt(%d).Cell.ContractID=%s", record.queryFamily, record.query, cell.ContractID())
					}
				}
			}
		}
	case rowNative:
		if solved := session.result; solved != nil {
			row, rowOK := solved.NativePublicationAt(record.native)
			if rowOK {
				if nativeID, idOK := row.ID(); idOK {
					writef(&b, "result.NativePublicationAt(%d).ID=%s", record.native, nativeID)
				}
				writef(&b, "result.NativePublicationAt(%d).Kind=%s", record.native, row.Kind().String())
				writef(&b, "result.NativePublicationAt(%d).Lane=%s", record.native, row.Lane().String())
				writef(&b, "result.NativePublicationAt(%d).Family=%s", record.native, row.Family())
				writef(&b, "result.NativePublicationAt(%d).Trust=%s", record.native, row.Trust().String())
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
			}
		}
	case rowOperation:
		writef(&b, "contract.Operations.OperationAt(%d)=%d", record.operation, record.operation+1)
		writef(&b, "contract.OperationContentID=%s", record.id)
	}
	return b.String()
}

func formatWhy(session *Session, id identity.ContentID, scoped bool) string {
	var b strings.Builder
	writeDiagnostics(&b, "compile", session.compileDiag)
	writeDiagnostics(&b, "solve", session.solveDiag)
	if report := session.report; report != nil && report.Available() {
		writef(&b, "report.FindingCount=%d", report.FindingCount())
		for index := 0; index < report.FindingCount(); index++ {
			finding, ok := report.FindingAt(index)
			if !ok {
				continue
			}
			if scoped {
				match := false
				if findingID, idOK := finding.ID(); idOK && findingID == id {
					match = true
				}
				if subject, subjectOK := finding.SubjectID(); subjectOK && subject == id {
					match = true
				}
				if !match {
					continue
				}
			}
			writef(&b, "report.FindingAt(%d).Code=%s", index, finding.Code())
			writef(&b, "report.FindingAt(%d).Message=%s", index, finding.Message())
			writef(&b, "report.FindingAt(%d).Render=%s", index, compactSpace(finding.Render()))
		}
	}
	if scoped {
		if _, ok := session.Lookup(id); !ok {
			writef(&b, "Lookup(%s)=false", id)
		}
	}
	return b.String()
}

func formatPublish(session *Session) string {
	var b strings.Builder
	solved := session.result
	if solved == nil {
		writef(&b, "result=unavailable")
	} else {
		writef(&b, "result.ContentID=%s", solved.ContentID())
		writef(&b, "result.NativePublicationAvailable=%t", solved.NativePublicationAvailable())
		writef(&b, "result.NativePublicationCount=%d", solved.NativePublicationCount())
		for index := 0; index < solved.NativePublicationCount(); index++ {
			row, ok := solved.NativePublicationAt(index)
			if !ok {
				continue
			}
			if id, idOK := row.ID(); idOK {
				writef(&b, "result.NativePublicationAt(%d).ID=%s", index, id)
			}
			writef(&b, "result.NativePublicationAt(%d).Kind=%s", index, row.Kind().String())
			writef(&b, "result.NativePublicationAt(%d).Lane=%s", index, row.Lane().String())
			writef(&b, "result.NativePublicationAt(%d).Family=%s", index, row.Family())
			writef(&b, "result.NativePublicationAt(%d).Exact=%t", index, row.Exact())
			if provenance, provenanceOK := row.Provenance(); provenanceOK {
				writef(&b, "result.NativePublicationAt(%d).Provenance.PointID=%s", index, provenance.PointID())
				writef(&b, "result.NativePublicationAt(%d).Provenance.MountID=%s", index, provenance.MountID())
			}
		}
		for familyIndex := 0; familyIndex < solved.FamilyCount(); familyIndex++ {
			family, familyOK := solved.FamilyAt(familyIndex)
			if !familyOK {
				continue
			}
			writef(&b, "result.FamilyAt(%d).Key=%s", familyIndex, family.Key())
			for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
				query, queryOK := family.QueryAt(queryIndex)
				if !queryOK {
					continue
				}
				if site, siteOK := query.SiteID(); siteOK {
					writef(&b, "result.FamilyAt(%d).QueryAt(%d).SiteID=%s", familyIndex, queryIndex, site)
				}
				writef(&b, "result.FamilyAt(%d).QueryAt(%d).Status=%d", familyIndex, queryIndex, uint8(query.Status()))
				if cell, cellOK := query.Cell(); cellOK {
					writef(&b, "result.FamilyAt(%d).QueryAt(%d).Cell.Present=%t", familyIndex, queryIndex, cell.Present())
					writef(&b, "result.FamilyAt(%d).QueryAt(%d).Cell.RowCount=%d", familyIndex, queryIndex, cell.RowCount())
					writef(&b, "result.FamilyAt(%d).QueryAt(%d).Cell.ContentID=%s", familyIndex, queryIndex, cell.ContentID())
				}
			}
		}
	}
	if report := session.report; report != nil && report.Available() {
		writef(&b, "report.FindingCount=%d", report.FindingCount())
		for index := 0; index < report.FindingCount(); index++ {
			finding, ok := report.FindingAt(index)
			if !ok {
				continue
			}
			writef(&b, "report.FindingAt(%d).Render=%s", index, compactSpace(finding.Render()))
		}
	}
	return b.String()
}

func formatDiff(left, right *Session) string {
	if left == nil || right == nil {
		return ""
	}
	var b strings.Builder
	leftIDs := sessionIDs(left)
	rightIDs := sessionIDs(right)
	for _, id := range leftIDs {
		if _, ok := right.Lookup(id); ok {
			continue
		}
		writef(&b, "diff.only[%s]=%s", left.fixture, id)
	}
	for _, id := range rightIDs {
		if _, ok := left.Lookup(id); ok {
			continue
		}
		writef(&b, "diff.only[%s]=%s", right.fixture, id)
	}
	if left.result != nil && right.result != nil && left.result.ContentID() != right.result.ContentID() {
		writef(&b, "diff.result.ContentID[%s]=%s", left.fixture, left.result.ContentID())
		writef(&b, "diff.result.ContentID[%s]=%s", right.fixture, right.result.ContentID())
	}
	return b.String()
}

func sessionIDs(session *Session) []identity.ContentID {
	if session == nil {
		return nil
	}
	ids := make([]identity.ContentID, 0, len(session.records))
	seen := make(map[identity.ContentID]struct{}, len(session.records))
	for _, record := range session.records {
		if !record.id.Available() {
			continue
		}
		if _, duplicate := seen[record.id]; duplicate {
			continue
		}
		seen[record.id] = struct{}{}
		ids = append(ids, record.id)
	}
	return ids
}

func writeDiagnostics(b *strings.Builder, origin string, diagnostics anadiag.AnalyzeDiagnostics) {
	writef(b, "%s.AnalyzeDiagnostics.Phase=%s", origin, diagnostics.Phase.String())
	writef(b, "%s.AnalyzeDiagnostics.Reason=%s", origin, diagnostics.Reason.String())
	if diagnostics.Engine.Failure.Available() {
		writef(b, "%s.AnalyzeDiagnostics.Engine.Failure.Reason=%d", origin, uint8(diagnostics.Engine.Failure.Reason()))
		failure := diagnostics.Engine.Failure.Failure()
		if failure.Available() {
			writef(b, "%s.AnalyzeDiagnostics.Engine.Failure.Failure=%s", origin, failure.String())
		}
	}
}

func writef(b *strings.Builder, format string, args ...any) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, format, args...)
}

func compactSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
